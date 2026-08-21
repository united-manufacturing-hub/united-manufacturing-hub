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

	"go.uber.org/zap"

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
		return nil, false, "string(leading line break)"
	}

	if (strings.HasPrefix(s, " ") || strings.HasPrefix(s, "\t")) && strings.Contains(s, "\n") {
		return nil, false, "string(multiline, leading whitespace)"
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

var reportedFallbackTypes sync.Map // map[string]struct{}, one report per type

// reportFallback surfaces a type the fast path cannot handle. One declining value
// sends its whole config section back to the slow path, so this is the only signal
// that the optimization stopped applying somewhere.
//
// WARN, not DEBUG: the default level is Info, which would hide it exactly where it
// matters. Fires once per type per process. Sentry on top is best-effort - its
// warning path shares one process-wide two-hour debounce and the type is marked
// before the send, so a debounced report is never retried.
func reportFallback(unsupported string) {
	if unsupported == "" {
		return
	}

	if _, alreadyReported := reportedFallbackTypes.LoadOrStore(unsupported, struct{}{}); alreadyReported {
		return
	}

	log := zap.S()

	log.Warnf(
		"benthos config canonicalization fell back to the YAML round-trip: unsupported type %q. "+
			"Correct but slower; add the case to normalizeValue in canonicalize_walk.go.",
		unsupported)

	// log, not nil: ReportIssueWithContext swaps nil for a no-op logger.
	sentry.ReportIssueWithContext(
		fmt.Errorf("canonicalize fast path unsupported type: %s", unsupported),
		sentry.IssueTypeWarning, log,
		map[string]interface{}{"trigger": "canonicalize_fallback_" + unsupported})
}
