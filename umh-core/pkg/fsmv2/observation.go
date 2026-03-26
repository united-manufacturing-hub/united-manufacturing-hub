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
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// DetectFieldCollisions checks at init time whether T's JSON field names
// collide with reserved framework field names used by Observation.
// Called by register.Worker to fail-fast on namespace conflicts.
func DetectFieldCollisions[T any]() error {
	reserved := collectJSONFieldNames(reflect.TypeOf(observationFrameworkFields{}))

	var zero T
	tType := reflect.TypeOf(zero)
	if tType == nil {
		return fmt.Errorf("DetectFieldCollisions: T must be a concrete type, got nil (interface type)")
	}
	if tType.Kind() == reflect.Ptr {
		tType = tType.Elem()
	}

	if tType.Kind() != reflect.Struct {
		return fmt.Errorf("DetectFieldCollisions: T must be a struct, got %s", tType.Kind())
	}

	statusNames := collectJSONFieldNames(tType)

	var collisions []string
	for name := range statusNames {
		if reserved[name] {
			collisions = append(collisions, name)
		}
	}

	if len(collisions) > 0 {
		sort.Strings(collisions)

		return fmt.Errorf("DetectFieldCollisions: TStatus JSON field(s) %v collide with framework reserved field(s)", collisions)
	}

	return nil
}

// collectJSONFieldNames returns the set of JSON field names for a struct type,
// recursing into anonymous (embedded) structs that have no JSON tag.
func collectJSONFieldNames(t reflect.Type) map[string]bool {
	names := make(map[string]bool)

	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("json")

		if tag == "-" {
			continue
		}

		if tag == "" {
			if field.Anonymous {
				ft := field.Type
				if ft.Kind() == reflect.Ptr {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct {
					for k := range collectJSONFieldNames(ft) {
						names[k] = true
					}
				}
			}

			continue
		}

		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = field.Name
		}
		if name != "" {
			names[name] = true
		}
	}

	return names
}

// NewObservation wraps a developer's TStatus into an Observation
// ready for collector post-processing. The collector sets CollectedAt,
// framework metrics, action history, and accumulated worker metrics
// after CollectObservedState returns.
//
// Usage in CollectObservedState:
//
//	return fsmv2.NewObservation(MyStatus{
//	    Reachable: w.deps.CheckReachable(ctx, cfg),
//	}), nil
func NewObservation[TStatus any](status TStatus) ObservedState {
	return Observation[TStatus]{Status: status}
}

// Observation wraps a developer's TStatus into the full ObservedState
// required by the supervisor. Framework fields and TStatus fields are flattened
// to the same JSON level via custom MarshalJSON/UnmarshalJSON, preserving CSE
// delta sync granularity.
type Observation[TStatus any] struct {
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

// observationFrameworkFields is the shared alias type used by MarshalJSON and UnmarshalJSON
// to serialize/deserialize framework fields without triggering recursive custom marshal.
type observationFrameworkFields struct {
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
func (o Observation[TStatus]) MarshalJSON() ([]byte, error) {
	fw := observationFrameworkFields{
		CollectedAt:       o.CollectedAt,
		State:             o.State,
		ShutdownRequested: o.ShutdownRequested,
		ParentMappedState: o.ParentMappedState,
		LastActionResults: o.LastActionResults,
		ChildrenHealthy:   o.ChildrenHealthy,
		ChildrenUnhealthy: o.ChildrenUnhealthy,
		MetricsEmbedder:   o.MetricsEmbedder,
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
	statusBytes, err := json.Marshal(o.Status)
	if err != nil {
		return nil, fmt.Errorf("marshal status: %w", err)
	}

	var statusMap map[string]json.RawMessage
	if err := json.Unmarshal(statusBytes, &statusMap); err != nil {
		return nil, fmt.Errorf("Observation: TStatus must marshal to a JSON object, got: %s", string(statusBytes))
	}

	// Step 3: Merge status fields, detecting collisions with framework fields.
	for k, v := range statusMap {
		if _, exists := merged[k]; exists {
			return nil, fmt.Errorf("Observation: TStatus field %q collides with framework field", k)
		}

		merged[k] = v
	}

	return json.Marshal(merged)
}

// UnmarshalJSON reverses the flat JSON back into framework fields and TStatus.
func (o *Observation[TStatus]) UnmarshalJSON(data []byte) error {
	// Step 1: Unmarshal framework fields using the shared alias to avoid recursion.
	var fw observationFrameworkFields
	if err := json.Unmarshal(data, &fw); err != nil {
		return fmt.Errorf("unmarshal framework fields: %w", err)
	}

	o.CollectedAt = fw.CollectedAt
	o.State = fw.State
	o.ShutdownRequested = fw.ShutdownRequested
	o.ParentMappedState = fw.ParentMappedState
	o.LastActionResults = fw.LastActionResults
	o.ChildrenHealthy = fw.ChildrenHealthy
	o.ChildrenUnhealthy = fw.ChildrenUnhealthy
	o.MetricsEmbedder = fw.MetricsEmbedder

	// Step 2: Unmarshal the full blob into TStatus.
	// TStatus fields coexist at the same level as framework fields;
	// json.Unmarshal ignores unknown keys for struct targets.
	var status TStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return fmt.Errorf("unmarshal status: %w", err)
	}

	o.Status = status

	return nil
}

// ---------------------------------------------------------------------------
// ObservedState interface methods
// ---------------------------------------------------------------------------

// GetTimestamp returns when this observation was collected.
func (o Observation[TStatus]) GetTimestamp() time.Time {
	return o.CollectedAt
}

// GetObservedDesiredState returns the desired state that was active when this
// observation was collected. Injected by the collector via SetObservedDesiredState.
func (o Observation[TStatus]) GetObservedDesiredState() DesiredState {
	return o.observedDesiredState
}

// ---------------------------------------------------------------------------
// Collector duck-typed setters (value receivers returning ObservedState)
// ---------------------------------------------------------------------------

// SetState sets the FSM state name. Matches collector pattern:
//
//	interface{ SetState(string) fsmv2.ObservedState }
func (o Observation[TStatus]) SetState(s string) ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested flag. Matches collector pattern:
//
//	interface{ SetShutdownRequested(bool) fsmv2.ObservedState }
func (o Observation[TStatus]) SetShutdownRequested(v bool) ObservedState {
	o.ShutdownRequested = v

	return o
}

// SetParentMappedState sets the mapped parent state. Matches collector pattern:
//
//	interface{ SetParentMappedState(string) fsmv2.ObservedState }
func (o Observation[TStatus]) SetParentMappedState(s string) ObservedState {
	o.ParentMappedState = s

	return o
}

// SetChildrenCounts sets the healthy/unhealthy child counts. Matches collector pattern:
//
//	interface{ SetChildrenCounts(int, int) fsmv2.ObservedState }
func (o Observation[TStatus]) SetChildrenCounts(healthy, unhealthy int) ObservedState {
	o.ChildrenHealthy = healthy
	o.ChildrenUnhealthy = unhealthy

	return o
}

// SetChildrenView sets the runtime children view. Matches collector pattern:
//
//	interface{ SetChildrenView(any) fsmv2.ObservedState }
func (o Observation[TStatus]) SetChildrenView(v any) ObservedState {
	o.ChildrenView = v

	return o
}

// SetObservedDesiredState injects the desired state reference for GetObservedDesiredState.
// Called by the collector after CollectObservedState returns.
func (o Observation[TStatus]) SetObservedDesiredState(d DesiredState) ObservedState {
	o.observedDesiredState = d

	return o
}

// SetCollectedAt sets the observation timestamp. Matches collector pattern:
//
//	interface{ SetCollectedAt(time.Time) fsmv2.ObservedState }
func (o Observation[TStatus]) SetCollectedAt(t time.Time) ObservedState {
	o.CollectedAt = t

	return o
}

// SetWorkerMetrics sets the worker-level metrics. Matches collector pattern:
//
//	interface{ SetWorkerMetrics(deps.Metrics) fsmv2.ObservedState }
func (o Observation[TStatus]) SetWorkerMetrics(m deps.Metrics) ObservedState {
	o.Metrics.Worker = m

	return o
}

// SetFrameworkMetrics sets the framework-level metrics. Matches collector pattern:
//
//	interface{ SetFrameworkMetrics(deps.FrameworkMetrics) fsmv2.ObservedState }
func (o Observation[TStatus]) SetFrameworkMetrics(fm deps.FrameworkMetrics) ObservedState {
	o.Metrics.Framework = fm

	return o
}

// SetActionHistory sets the action result history. Matches collector pattern:
//
//	interface{ SetActionHistory([]deps.ActionResult) fsmv2.ObservedState }
func (o Observation[TStatus]) SetActionHistory(h []deps.ActionResult) ObservedState {
	o.LastActionResults = h

	return o
}
