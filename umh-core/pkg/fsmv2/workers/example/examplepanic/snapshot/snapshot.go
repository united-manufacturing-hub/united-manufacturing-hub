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

package snapshot

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

type ExamplepanicDependencies interface {
	deps.Dependencies
	// IsShouldPanic returns whether the worker should panic during connect action.
	IsShouldPanic() bool
	// SetConnected sets the connection state.
	SetConnected(connected bool)
	// IsConnected returns the current connection state.
	IsConnected() bool
}

type ExamplepanicSnapshot struct {
	Identity deps.Identity
	Desired  ExamplepanicDesiredState
	Observed ExamplepanicObservedState
}

// ExamplepanicDesiredState represents the target configuration for the panic worker.
// NOTE: Dependencies are NOT stored here - they belong in the Worker struct.
// See fsmv2.DesiredState documentation for the architectural invariant.
type ExamplepanicDesiredState struct {

	// ParentMappedState is the desired state derived from parent's ChildStartStates.
	// When parent is in a state listed in ChildStartStates → "running"
	// When parent is in any other state → "stopped"
	// This field is injected by the supervisor via MappedParentStateProvider callback.
	ParentMappedState string `json:"parent_mapped_state"`

	config.BaseDesiredState // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

	ShouldPanic bool `json:"ShouldPanic"`
}

// ShouldBeRunning returns true if the panic worker should be in a running/connected state.
// This is the positive assertion that should be checked before transitioning
// from stopped to starting states.
//
// Children only run when:
// 1. ShutdownRequested is false (not being shut down)
// 2. ParentMappedState is config.DesiredStateRunning (parent wants children to run)
//
// Children wait for parent to reach TryingToStart before connecting.
func (s *ExamplepanicDesiredState) ShouldBeRunning() bool {
	if s.ShutdownRequested {
		return false
	}
	// Only run if parent explicitly wants us running via ChildStartStates
	// Default to not running if ParentMappedState is empty or "stopped"
	return s.ParentMappedState == config.DesiredStateRunning
}

func (s *ExamplepanicDesiredState) IsShouldPanic() bool {
	return s.ShouldPanic
}

type ExamplepanicObservedState struct {
	CollectedAt time.Time `json:"collected_at"`

	LastError error  `json:"last_error,omitempty"`
	ID        string `json:"id"`

	State            string `json:"state"` // Observed lifecycle state (e.g., "running_connected")
	ConnectionHealth string `json:"connection_health"`

	ExamplepanicDesiredState `json:",inline"`

	// LastActionResults contains the action history from the last collection cycle.
	// This is supervisor-managed data: the supervisor auto-records action results
	// via ActionExecutor callback and injects them into deps before CollectObservedState.
	// Workers read deps.GetActionHistory() and assign here in CollectObservedState.
	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	deps.MetricsEmbedder `json:",inline"` // Framework and worker metrics for Prometheus export

	ConnectAttempts int `json:"connect_attempts"`
}

func (o ExamplepanicObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o ExamplepanicObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ExamplepanicDesiredState
}

// SetState sets the FSM state name on this observed state.
// Called by Collector when StateProvider callback is configured.
func (o ExamplepanicObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
// Called by Collector when ShutdownRequestedProvider callback is configured.
func (o ExamplepanicObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// SetParentMappedState sets the parent's mapped state on this observed state.
// Called by Collector when MappedParentStateProvider callback is configured.
// Children can check if parent wants them running via StateMapping.
func (o ExamplepanicObservedState) SetParentMappedState(state string) fsmv2.ObservedState {
	o.ParentMappedState = state

	return o
}

// IsStopRequired reports whether the child needs to stop.
// This is a QUERY on injected data, not a signal emission.
// It combines:
//   - IsShutdownRequested() - explicit system shutdown
//   - !ShouldBeRunning() - parent no longer wants child running
func (o ExamplepanicObservedState) IsStopRequired() bool {
	return o.IsShutdownRequested() || !o.ShouldBeRunning()
}
