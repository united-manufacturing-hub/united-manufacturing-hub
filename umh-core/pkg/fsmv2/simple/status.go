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

package simple

import (
	"encoding/json"
	"fmt"
)

// Status wraps a developer's poll result with the tick's degraded/reason verdict.
// Those two fields are the Health value from health.go, flattened in here: on a
// good poll they come from the developer's Health function; on a poll error the
// framework sets them directly (Degraded, a reason, and a zero Result) without
// calling Health. simple owns this type (unlike the opaque developer TStatus), so
// it can carry the verdict on both paths. The fsmv2 state reads Degraded/Reason
// off the snapshot, and the fsmv1 adapter reads them through the HealthReporter
// interface.
//
// Result is flattened to the top JSON level (see MarshalJSON) so CSE delta sync
// keeps per-field granularity and native consumers see the same schema as an
// unwrapped status.
type Status[T any] struct {
	// Result is the developer's poll result. Flattened to top level via
	// MarshalJSON; it must marshal to a JSON object.
	Result T `json:"-"`
	// Reason is the human-readable explanation for the verdict.
	Reason string `json:"reason"`
	// Degraded is true when the polled target is unhealthy or the poll failed.
	Degraded bool `json:"degraded"`
}

// HealthVerdict returns the carried verdict. It satisfies the adapter's
// HealthReporter interface so the fsmv1 side reads the verdict without importing
// simple or knowing the developer's status type.
func (s Status[T]) HealthVerdict() (degraded bool, reason string) {
	return s.Degraded, s.Reason
}

// statusVerdictFields is the JSON shape of the verdict, used by MarshalJSON and
// UnmarshalJSON to (de)serialize without recursing through Status's custom
// marshaler.
type statusVerdictFields struct {
	Reason   string `json:"reason"`
	Degraded bool   `json:"degraded"`
}

// MarshalJSON hoists Result's fields to the top level alongside the verdict
// fields, mirroring fsmv2.Observation's flattening so CSE delta sync compares
// per developer field. A developer field colliding with "reason"/"degraded" is
// rejected.
func (s Status[T]) MarshalJSON() ([]byte, error) {
	resultBytes, err := json.Marshal(s.Result)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	merged := make(map[string]json.RawMessage)
	if err := json.Unmarshal(resultBytes, &merged); err != nil {
		return nil, fmt.Errorf("simple.Status: Result must marshal to a JSON object, got: %s", string(resultBytes))
	}

	verdictBytes, err := json.Marshal(statusVerdictFields{Reason: s.Reason, Degraded: s.Degraded})
	if err != nil {
		return nil, fmt.Errorf("marshal verdict: %w", err)
	}

	var verdictMap map[string]json.RawMessage
	if err := json.Unmarshal(verdictBytes, &verdictMap); err != nil {
		return nil, fmt.Errorf("unmarshal verdict to map: %w", err)
	}

	for k, v := range verdictMap {
		if _, exists := merged[k]; exists {
			return nil, fmt.Errorf("simple.Status: Result field %q collides with verdict field", k)
		}

		merged[k] = v
	}

	return json.Marshal(merged)
}

// UnmarshalJSON reverses the flat JSON back into Result and the verdict fields.
func (s *Status[T]) UnmarshalJSON(data []byte) error {
	var v statusVerdictFields
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("unmarshal verdict fields: %w", err)
	}

	s.Reason = v.Reason
	s.Degraded = v.Degraded

	// Result's fields coexist at the same level; json.Unmarshal ignores the
	// verdict keys for a struct target.
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("unmarshal result: %w", err)
	}

	s.Result = result

	return nil
}
