// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package benthosserviceconfig

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
)

// intFromInt64 and intFromUint64 pick the type yaml's resolver picks: int when it
// fits, otherwise int64 or uint64. The wider branches only matter on 32-bit.
func intFromInt64(v int64) interface{} {
	if v > math.MaxInt || v < math.MinInt {
		return v
	}

	return int(v)
}

func intFromUint64(v uint64) interface{} {
	if v <= math.MaxInt {
		return int(v)
	}

	// Still signed to yaml; only past MaxInt64 does its resolver reach ParseUint.
	if v <= math.MaxInt64 {
		return int64(v)
	}

	return v
}

// checkString rejects the strings yaml's block-scalar emitter does not round-trip.
// A leading break is swallowed ("\nSELECT 1" -> "SELECT 1"); U+2028 and U+2029 count
// as breaks too, hence decoding the first rune rather than matching a byte. U+0085
// stays accepted: a break to the parser, but the emitter quotes it.
//
// A leading space loses the common indent inside a sequence element, a leading tab
// makes yaml emit output it cannot read back. Both need a multi-line string, which
// is what makes yaml reach for a block scalar at all.
//
// Every string on the fast path passes through here, map keys and the elements of
// []string and map[string]string included.
func checkString(s string) (out interface{}, ok bool, unsupported string) {
	if r, _ := utf8.DecodeRuneInString(s); r == '\n' || r == '\u2028' || r == '\u2029' {
		return nil, false, declinedLeadingBreak
	}

	if (strings.HasPrefix(s, " ") || strings.HasPrefix(s, "\t")) && strings.Contains(s, "\n") {
		return nil, false, declinedLeadingWhitespace
	}

	return s, true, ""
}

// normalizeValue reproduces a yaml.Marshal + yaml.Unmarshal round-trip without
// serialising. The third return value names the shape it gave up on.
//
// SAFETY: the result must be indistinguishable from the round-trip. A wrong answer
// makes two equal configs look different and re-applies the config forever, so
// anything it cannot reproduce exactly returns ok=false and falls back. Being slow
// is acceptable; being wrong is not.
func normalizeValue(v interface{}) (out interface{}, ok bool, unsupported string) {
	switch t := v.(type) {
	case nil:
		return nil, true, ""
	case bool, int:
		return t, true, ""
	case string:
		return checkString(t)
	case float64:
		// float64(1000) is emitted as a plain "1000" and reads back as int, so resolve
		// the emitted text the way yaml does.
		text := strconv.FormatFloat(t, 'g', -1, 64)
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return intFromInt64(i), true, ""
		}

		if u, err := strconv.ParseUint(text, 10, 64); err == nil {
			return u, true, ""
		}

		return t, true, ""
	case int8:
		return int(t), true, ""
	case int16:
		return int(t), true, ""
	case int32:
		return int(t), true, ""
	case int64:
		return intFromInt64(t), true, ""
	case uint:
		return intFromUint64(uint64(t)), true, ""
	case uint8:
		return int(t), true, ""
	case uint16:
		return int(t), true, ""
	case uint32:
		return intFromUint64(uint64(t)), true, ""
	case uint64:
		return intFromUint64(t), true, ""
	case float32:
		// Round-trips via a 32-bit decimal form, so the result is not float64(t).
		// Reproducible, but no config path produces one.
		return nil, false, "float32"
	case []interface{}:
		res := make([]interface{}, len(t))

		for i, e := range t {
			n, k, bad := normalizeValue(e)
			if !k {
				return nil, false, bad
			}

			res[i] = n
		}

		return res, true, ""
	case []string:
		res := make([]interface{}, len(t))

		for i, e := range t {
			n, k, bad := checkString(e)
			if !k {
				return nil, false, bad
			}

			res[i] = n
		}

		return res, true, ""
	case []map[string]interface{}:
		res := make([]interface{}, len(t))

		for i, e := range t {
			n, k, bad := normalizeValue(e)
			if !k {
				return nil, false, bad
			}

			res[i] = n
		}

		return res, true, ""
	case map[string]interface{}:
		res := make(map[string]interface{}, len(t))

		for key, e := range t {
			if _, k, bad := checkString(key); !k {
				return nil, false, "map key " + bad
			}

			n, k, bad := normalizeValue(e)
			if !k {
				return nil, false, bad
			}

			res[key] = n
		}

		return res, true, ""
	case map[string]string:
		res := make(map[string]interface{}, len(t))

		for key, e := range t {
			if _, k, bad := checkString(key); !k {
				return nil, false, "map key " + bad
			}

			n, k, bad := checkString(e)
			if !k {
				return nil, false, bad
			}

			res[key] = n
		}

		return res, true, ""
	case map[interface{}]interface{}:
		res := make(map[string]interface{}, len(t))

		for key, e := range t {
			ks, isStr := key.(string)
			if !isStr {
				return nil, false, fmt.Sprintf("map key %T", key)
			}

			if _, k, bad := checkString(ks); !k {
				return nil, false, "map key " + bad
			}

			n, k, bad := normalizeValue(e)
			if !k {
				return nil, false, bad
			}

			res[ks] = n
		}

		return res, true, ""
	default:
		// Structs, pointers, custom types: yaml goes through their tags, and
		// replicating that would be a second marshaller.
		return nil, false, fmt.Sprintf("%T", v)
	}
}

// declinedLeadingBreak and declinedLeadingWhitespace name the two value shapes
// checkString refuses. They are ordinary configuration - an indented SQL block or
// a script pasted with its leading whitespace - so they are reported differently
// from a Go type the walk has no case for.
const (
	declinedLeadingBreak      = "string(leading line break)"
	declinedLeadingWhitespace = "string(multiline, leading whitespace)"
)

// isDeclinedShape reports whether reason names a value the walk refuses on purpose
// rather than a type it cannot handle. HasSuffix because a map key arrives as
// "map key " + reason.
func isDeclinedShape(reason string) bool {
	return strings.HasSuffix(reason, declinedLeadingBreak) ||
		strings.HasSuffix(reason, declinedLeadingWhitespace)
}

var reportedFallbacks sync.Map // map[string]struct{}, one report per section and reason

// reportFallback surfaces a section that went back to the slow path. One declining
// value costs the round-trip for everything around it, so this is the only signal
// that the optimization stopped applying somewhere. It fires at most once per
// section and reason per process.
//
// Two very different things end up here, and only one is a defect:
//
//   - A refused value shape. The round-trip result depends on where the value sits
//     in the document, which the walk cannot see, so refusing is the only correct
//     answer and the config is fine. Info, and no Sentry issue: an engineer has
//     nothing to do, and a customer with an indented SQL query would otherwise file
//     one for us on every deployment.
//   - A Go type with no case in normalizeValue. That means something new reached the
//     config path and the walk silently stopped applying to it. WARN plus Sentry,
//     because it is actionable and nobody would otherwise notice.
func reportFallback(section, reason string) {
	if reason == "" {
		return
	}

	if _, alreadyReported := reportedFallbacks.LoadOrStore(section+"/"+reason, struct{}{}); alreadyReported {
		return
	}

	log := logger.For(logger.ComponentBenthosConfig)

	if isDeclinedShape(reason) {
		log.Infof(
			"Benthos config canonicalization took the YAML round-trip for the %s section: "+
				"it holds a %s, which cannot be normalized without knowing where it sits in "+
				"the document. Correct, only slower.",
			section, reason)

		return
	}

	log.Warnf(
		"Benthos config canonicalization fell back to the YAML round-trip for the %s "+
			"section: unsupported type %q. Correct but slower; add the case to "+
			"normalizeValue in canonicalize_walk.go.",
		section, reason)

	// log, not nil: ReportIssueWithContext swaps nil for a no-op logger.
	sentry.ReportIssueWithContext(
		fmt.Errorf("canonicalize fast path unsupported type: %s", reason),
		sentry.IssueTypeWarning, log,
		map[string]interface{}{"trigger": "canonicalize_fallback_" + reason, "section": section})
}
