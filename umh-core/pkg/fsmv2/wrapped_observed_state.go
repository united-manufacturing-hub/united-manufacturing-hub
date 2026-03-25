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

package fsmv2

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// WrappedObservedState wraps a developer's TStatus into the full ObservedState
// required by the supervisor. Framework fields and TStatus fields are flattened
// to the same JSON level via custom MarshalJSON/UnmarshalJSON, preserving CSE
// delta sync granularity.
type WrappedObservedState[TStatus any] struct {
	// CollectedAt is when this observation was taken.
	CollectedAt time.Time `json:"collected_at"`
	// ChildrenView provides runtime access to child state (not persisted).
	ChildrenView any `json:"-"`
	// Status is the developer's business data. Flattened to top level via custom MarshalJSON.
	Status TStatus `json:"-"`
	// observedDesiredState is the desired state reference for ObservedState interface.
	// Populated by the collector via SetObservedDesiredState after CollectObservedState returns.
	observedDesiredState DesiredState
	// State is the current FSM state name (set by supervisor via SetState).
	State string `json:"state"`
	// ParentMappedState is the desired state mapped from the parent's current state.
	ParentMappedState string `json:"parent_mapped_state"`
	// LastActionResults contains action history (managed by supervisor).
	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`
	// MetricsEmbedder provides framework and worker metrics (anonymous embed, inline in JSON).
	deps.MetricsEmbedder
	// ChildrenHealthy is the count of healthy children.
	ChildrenHealthy int `json:"children_healthy"`
	// ChildrenUnhealthy is the count of unhealthy children.
	ChildrenUnhealthy int `json:"children_unhealthy"`
	// ShutdownRequested mirrors the desired state's shutdown flag.
	ShutdownRequested bool `json:"ShutdownRequested"` //nolint:tagliatelle // Match existing API field name
}

// wrappedFrameworkFields is the shared alias type used by MarshalJSON and UnmarshalJSON
// to serialize/deserialize framework fields without triggering recursive custom marshal.
type wrappedFrameworkFields struct {
	CollectedAt       time.Time           `json:"collected_at"`
	State             string              `json:"state"`
	ParentMappedState string              `json:"parent_mapped_state"`
	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`
	deps.MetricsEmbedder
	ChildrenHealthy   int  `json:"children_healthy"`
	ChildrenUnhealthy int  `json:"children_unhealthy"`
	ShutdownRequested bool `json:"ShutdownRequested"` //nolint:tagliatelle
}

// MarshalJSON produces flat JSON with framework fields and TStatus fields at the same level.
// This is required for CSE delta sync which does field-level comparison.
func (w WrappedObservedState[TStatus]) MarshalJSON() ([]byte, error) {
	fw := wrappedFrameworkFields{
		CollectedAt:       w.CollectedAt,
		State:             w.State,
		ShutdownRequested: w.ShutdownRequested,
		ParentMappedState: w.ParentMappedState,
		LastActionResults: w.LastActionResults,
		ChildrenHealthy:   w.ChildrenHealthy,
		ChildrenUnhealthy: w.ChildrenUnhealthy,
		MetricsEmbedder:   w.MetricsEmbedder,
	}

	// Step 1: Marshal framework fields to intermediate map.
	fwBytes, err := json.Marshal(fw)
	if err != nil {
		return nil, fmt.Errorf("marshal framework fields: %w", err)
	}

	var merged map[string]json.RawMessage
	if err := json.Unmarshal(fwBytes, &merged); err != nil {
		return nil, fmt.Errorf("unmarshal framework fields to map: %w", err)
	}

	// Step 2: Marshal TStatus fields into the same map.
	statusBytes, err := json.Marshal(w.Status)
	if err != nil {
		return nil, fmt.Errorf("marshal status: %w", err)
	}

	var statusMap map[string]json.RawMessage
	if err := json.Unmarshal(statusBytes, &statusMap); err != nil {
		return nil, fmt.Errorf("WrappedObservedState: TStatus must marshal to a JSON object, got: %s", string(statusBytes))
	}

	// Step 3: Merge status fields, detecting collisions with framework fields.
	for k, v := range statusMap {
		if _, exists := merged[k]; exists {
			return nil, fmt.Errorf("WrappedObservedState: TStatus field %q collides with framework field", k)
		}

		merged[k] = v
	}

	return json.Marshal(merged)
}

// UnmarshalJSON reverses the flat JSON back into framework fields and TStatus.
func (w *WrappedObservedState[TStatus]) UnmarshalJSON(data []byte) error {
	// Step 1: Unmarshal framework fields using the shared alias to avoid recursion.
	var fw wrappedFrameworkFields
	if err := json.Unmarshal(data, &fw); err != nil {
		return fmt.Errorf("unmarshal framework fields: %w", err)
	}

	w.CollectedAt = fw.CollectedAt
	w.State = fw.State
	w.ShutdownRequested = fw.ShutdownRequested
	w.ParentMappedState = fw.ParentMappedState
	w.LastActionResults = fw.LastActionResults
	w.ChildrenHealthy = fw.ChildrenHealthy
	w.ChildrenUnhealthy = fw.ChildrenUnhealthy
	w.MetricsEmbedder = fw.MetricsEmbedder

	// Step 2: Unmarshal the full blob into TStatus.
	// TStatus fields coexist at the same level as framework fields;
	// json.Unmarshal ignores unknown keys for struct targets.
	var status TStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return fmt.Errorf("unmarshal status: %w", err)
	}

	w.Status = status

	return nil
}

// ---------------------------------------------------------------------------
// ObservedState interface methods
// ---------------------------------------------------------------------------

// GetTimestamp returns when this observation was collected.
func (w WrappedObservedState[TStatus]) GetTimestamp() time.Time {
	return w.CollectedAt
}

// GetObservedDesiredState returns the desired state that was active when this
// observation was collected. Injected by the collector via SetObservedDesiredState.
func (w WrappedObservedState[TStatus]) GetObservedDesiredState() DesiredState {
	return w.observedDesiredState
}

// ---------------------------------------------------------------------------
// Collector duck-typed setters (value receivers returning ObservedState)
// ---------------------------------------------------------------------------

// SetState sets the FSM state name. Matches collector pattern:
//
//	interface{ SetState(string) fsmv2.ObservedState }
func (w WrappedObservedState[TStatus]) SetState(s string) ObservedState {
	w.State = s

	return w
}

// SetShutdownRequested sets the shutdown requested flag. Matches collector pattern:
//
//	interface{ SetShutdownRequested(bool) fsmv2.ObservedState }
func (w WrappedObservedState[TStatus]) SetShutdownRequested(v bool) ObservedState {
	w.ShutdownRequested = v

	return w
}

// SetParentMappedState sets the mapped parent state. Matches collector pattern:
//
//	interface{ SetParentMappedState(string) fsmv2.ObservedState }
func (w WrappedObservedState[TStatus]) SetParentMappedState(s string) ObservedState {
	w.ParentMappedState = s

	return w
}

// SetChildrenCounts sets the healthy/unhealthy child counts. Matches collector pattern:
//
//	interface{ SetChildrenCounts(int, int) fsmv2.ObservedState }
func (w WrappedObservedState[TStatus]) SetChildrenCounts(healthy, unhealthy int) ObservedState {
	w.ChildrenHealthy = healthy
	w.ChildrenUnhealthy = unhealthy

	return w
}

// SetChildrenView sets the runtime children view. Matches collector pattern:
//
//	interface{ SetChildrenView(any) fsmv2.ObservedState }
func (w WrappedObservedState[TStatus]) SetChildrenView(v any) ObservedState {
	w.ChildrenView = v

	return w
}

// SetObservedDesiredState injects the desired state reference for GetObservedDesiredState.
// Called by the collector after CollectObservedState returns.
func (w WrappedObservedState[TStatus]) SetObservedDesiredState(d DesiredState) ObservedState {
	w.observedDesiredState = d

	return w
}
