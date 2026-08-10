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

// fastNormalize reproduces what a yaml.Marshal + yaml.Unmarshal round-trip does to
// a generic value, without serialising to text.
//
// canonicalize exists so a config built in Go compares equal to the same config
// decoded from benthos.yaml. The round-trip achieves that by re-encoding, which is
// correct but serialises and re-parses the whole config on both sides of every
// comparison: a CPU profile under a 1.2 MB config attributed 49.8% of all umh-core
// CPU time to it.
//
// SAFETY: the result must be indistinguishable from the round-trip. A wrong answer
// makes two equal configs look different and reintroduces the endless re-apply loop
// canonicalization was added to fix (ENG-5357). Anything this cannot reproduce
// exactly returns ok=false and the caller falls back to the real round-trip. Being
// slow is acceptable; being wrong is not.
func fastNormalize(v interface{}) (out interface{}, ok bool) {
	out, ok, _ = normalizeValue(v)

	return out, ok
}

// intFromInt64 and intFromUint64 pick the same Go type yaml.v3's resolver picks for
// an integer scalar: ParseInt first, then ParseUint, kept as int when it fits. The
// wider branches are unreachable on a 64-bit build and exist so a 32-bit one does
// not truncate.
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

	// Above MaxInt but still a valid int64: ParseInt accepts it, so yaml keeps it
	// signed. Only values past MaxInt64 fall through to ParseUint and stay uint64.
	if v <= math.MaxInt64 {
		return int64(v)
	}

	return v
}

// normalizeValue is fastNormalize plus the type that made it give up, so the
// caller can report which shape is still missing.
func normalizeValue(v interface{}) (out interface{}, ok bool, unsupported string) {
	switch t := v.(type) {
	case nil:
		return nil, true, ""
	case bool, int:
		return t, true, ""
	case string:
		// yaml's block-scalar emitter drops a leading newline, so "\nSELECT 1" comes
		// back as "SELECT 1". Reproducing that loss would mean reimplementing the
		// emitter's block-scalar decision, so decline instead. Being more faithful
		// than the round-trip is still a mismatch.
		if strings.HasPrefix(t, "\n") {
			return nil, false, "string(leading newline)"
		}

		return t, true, ""
	case float64:
		// yaml emits floats as a plain, untagged scalar via FormatFloat('g', -1, 64),
		// so float64(1000) is written as "1000" and the resolver reads it back as
		// int(1000). Resolve the emitted text the same way: ParseInt, then ParseUint,
		// otherwise leave it a float. Forms carrying an exponent or a dot fail both
		// parses and stay float64.
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
		// yaml writes float32 via its shortest 32-bit decimal form and resolves that
		// text back, so the result is not float64(t). Replicable, but yaml.Unmarshal
		// never produces a float32 and no config builder uses one, so leave it to the
		// fallback until the telemetry below says otherwise.
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
			res[i] = e
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
			res[key] = e
		}

		return res, true, ""
	case map[interface{}]interface{}:
		res := make(map[string]interface{}, len(t))

		for key, e := range t {
			ks, isStr := key.(string)
			if !isStr {
				return nil, false, fmt.Sprintf("map key %T", key)
			}

			n, k, bad := normalizeValue(e)
			if !k {
				return nil, false, bad
			}

			res[ks] = n
		}

		return res, true, ""
	default:
		// Structs, pointers, time.Time, custom types: yaml would encode these via
		// their tags, and replicating that here would be a second marshaller.
		return nil, false, fmt.Sprintf("%T", v)
	}
}

var reportedFallbackTypes sync.Map // map[string]struct{}, one report per type

// reportFallback surfaces a type the fast path cannot handle, so the missing case
// can be added rather than silently costing a full round-trip forever. A single
// declining value makes its whole config section fall back, so this is the only
// signal that the fix has stopped applying somewhere.
//
// Logged at DEBUG because it is a developer TODO, not something a customer can act
// on: the fallback is correct, just slow.
//
// KNOWN LIMITATION: sentry.ReportIssueWithContext shares one process-wide two-hour
// debounce across every IssueTypeWarning (pkg/sentry/report_internal.go). If an
// unrelated warning fired recently this report is dropped, and since the type is
// marked before we get here it is never retried. The log line is the reliable
// signal; Sentry is best-effort.
func reportFallback(unsupported string) {
	if unsupported == "" {
		return
	}

	if _, alreadyReported := reportedFallbackTypes.LoadOrStore(unsupported, struct{}{}); alreadyReported {
		return
	}

	zap.S().Debugf(
		"benthos config canonicalization fell back to the YAML round-trip: unsupported type %q. "+
			"Correct but slow; add the case to normalizeValue in canonicalize_fast.go.",
		unsupported)

	sentry.ReportIssueWithContext(
		fmt.Errorf("canonicalize fast path unsupported type: %s", unsupported),
		sentry.IssueTypeWarning, nil,
		map[string]interface{}{"trigger": "canonicalize_fallback_" + unsupported})
}
