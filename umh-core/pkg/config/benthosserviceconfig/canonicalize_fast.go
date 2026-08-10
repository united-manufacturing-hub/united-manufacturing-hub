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

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
)

// intFromInt64 and intFromUint64 pick the Go type yaml's resolver picks: int when
// the value fits, otherwise int64 or uint64. The wider branches only matter on a
// 32-bit build.
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

	// ParseInt still accepts it, so yaml keeps it signed; only past MaxInt64 does
	// it fall through to ParseUint.
	if v <= math.MaxInt64 {
		return int64(v)
	}

	return v
}

// checkString rejects the strings yaml's block-scalar emitter does not round-trip:
//
//	"\nSELECT 1"    -> "SELECT 1"    leading newline dropped
//	"  a\n    b\n"  -> "a\n  b\n"    in a sequence element, the common indent is
//	                                 stripped from every line
//
// The second depends on position, which the walk cannot see, so decline every
// multi-line string starting with a space. Tabs are safe: YAML forbids them as
// indentation, so those strings get quoted instead of blocked.
//
// Every string reaching the fast path must pass through here, including map keys
// and the elements of []string and map[string]string. Those used to be copied
// verbatim, which let the exact values this rejects through unchecked.
func checkString(s string) (out interface{}, ok bool, unsupported string) {
	if strings.HasPrefix(s, "\n") {
		return nil, false, "string(leading newline)"
	}

	if strings.HasPrefix(s, " ") && strings.Contains(s, "\n") {
		return nil, false, "string(multiline, leading space)"
	}

	return s, true, ""
}

// normalizeValue reproduces what a yaml.Marshal + yaml.Unmarshal round-trip does to
// a generic value, without serialising to text. The third return value names the
// shape it gave up on, so the caller can report what is still missing.
//
// SAFETY: the result must be indistinguishable from the round-trip. A wrong answer
// makes two equal configs look different and re-applies the config forever. Anything
// this cannot reproduce exactly returns ok=false and the caller falls back to the
// real round-trip. Being slow is acceptable; being wrong is not.
func normalizeValue(v interface{}) (out interface{}, ok bool, unsupported string) {
	switch t := v.(type) {
	case nil:
		return nil, true, ""
	case bool, int:
		return t, true, ""
	case string:
		return checkString(t)
	case float64:
		// yaml writes floats as a plain, untagged scalar, so float64(1000) becomes
		// "1000" and reads back as int. Resolve the emitted text the way yaml does:
		// ParseInt, then ParseUint, otherwise it stays a float.
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
		// Round-trips through a 32-bit decimal form, so the result is not float64(t).
		// Reproducible, but nothing in the config path produces a float32.
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
		// Structs, pointers, custom types: yaml encodes these via their tags, and
		// replicating that would be a second marshaller.
		return nil, false, fmt.Sprintf("%T", v)
	}
}

var reportedFallbackTypes sync.Map // map[string]struct{}, one report per type

// reportFallback surfaces a type the fast path cannot handle. A single declining
// value makes its whole config section fall back, so this is the only signal that
// the optimization has stopped applying somewhere.
//
// WARN, not DEBUG: the default level is Info, so a DEBUG line is invisible exactly
// where this matters. It fires at most once per type per process and means a whole
// config section went back to the slow path, which is a reconcile budget concern.
//
// Sentry is best-effort on top: its warning path shares one process-wide two-hour
// debounce across every warning, and the type is marked before the send, so a
// debounced report is never retried. The log line is the dependable signal.
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
			"Correct but slower; add the case to normalizeValue in canonicalize_fast.go.",
		unsupported)

	// Passing log rather than nil: ReportIssueWithContext swaps nil for a no-op
	// logger, which silently drops its own half of the report.
	sentry.ReportIssueWithContext(
		fmt.Errorf("canonicalize fast path unsupported type: %s", unsupported),
		sentry.IssueTypeWarning, log,
		map[string]interface{}{"trigger": "canonicalize_fallback_" + unsupported})
}
