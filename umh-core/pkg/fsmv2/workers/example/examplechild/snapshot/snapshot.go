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

// ExamplechildDependencies interface to avoid import cycles.
// Includes methods for connection state management used by actions.
type ExamplechildDependencies interface {
	deps.Dependencies
	// SetConnected updates the connection state. Called by ConnectAction/DisconnectAction.
	SetConnected(connected bool)
	// IsConnected returns the current connection state. Called by CollectObservedState.
	IsConnected() bool
}

// ExamplechildSnapshot represents a point-in-time view of the child worker state.
// This is the combined snapshot type for type assertions in Next() methods.
type ExamplechildSnapshot struct {
	Desired  *ExamplechildDesiredState
	Identity deps.Identity
	Observed ExamplechildObservedState
}

// ExamplechildDesiredState represents the target configuration for the child worker.
// NOTE: Dependencies are NOT stored here - they belong in the Worker struct.
// See fsmv2.DesiredState documentation for the architectural invariant.
type ExamplechildDesiredState struct {

	// ParentMappedState is the desired state derived from parent's ChildStartStates.
	// When parent is in a state listed in ChildStartStates → "running"
	// When parent is in any other state → "stopped"
	// This field is injected by the supervisor via MappedParentStateProvider callback.
	ParentMappedState       string `json:"parent_mapped_state"`
	config.BaseDesiredState        // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

}

// ShouldBeRunning returns true if the child should be in a running/connected state.
// This is the positive assertion that should be checked before transitioning
// from stopped to starting states.
//
// Children only run when:
// 1. ShutdownRequested is false (not being shut down)
// 2. ParentMappedState is config.DesiredStateRunning (parent wants children to run)
//
// Children wait for parent to reach TryingToStart before connecting.
//
// LIFECYCLE INVARIANT: Do NOT add custom lifecycle fields like ShouldRun.
// Use only ShutdownRequested (from BaseDesiredState) and ParentMappedState (for children).
func (s *ExamplechildDesiredState) ShouldBeRunning() bool {
	if s.ShutdownRequested {
		return false
	}
	// Only run if parent explicitly wants us running via ChildStartStates
	// Default to not running if ParentMappedState is empty or "stopped"
	return s.ParentMappedState == config.DesiredStateRunning
}

// ExamplechildObservedState represents the current state of the child worker.
type ExamplechildObservedState struct {
	CollectedAt time.Time `json:"collected_at"`

	LastError error `json:"last_error,omitempty"`

	ExamplechildDesiredState `json:",inline"`

	ID string `json:"id"`

	State            string `json:"state"` // Observed lifecycle state (e.g., "running_connected")
	ConnectionHealth string `json:"connection_health"`

	// LastActionResults contains the action history from the last collection cycle.
	// This is supervisor-managed data: the supervisor auto-records action results
	// via ActionExecutor callback and injects them into deps before CollectObservedState.
	// Workers read deps.GetActionHistory() and assign here in CollectObservedState.
	// Parents can read this from CSE via StateReader.LoadObservedTyped() to understand
	// WHY children are in their current state.
	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	// Embedded metrics for both framework and worker metrics.
	// Framework metrics provide time-in-state via GetFrameworkMetrics().TimeInCurrentStateMs
	// and state entered time via GetFrameworkMetrics().StateEnteredAtUnix.
	deps.MetricsEmbedder `json:",inline"`

	ConnectAttempts int `json:"connect_attempts"`
}

func (o ExamplechildObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o ExamplechildObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ExamplechildDesiredState
}

// SetState sets the FSM state name on this observed state.
// Called by Collector when StateProvider callback is configured.
func (o ExamplechildObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
// Called by Collector when ShutdownRequestedProvider callback is configured.
func (o ExamplechildObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// SetParentMappedState sets the parent's mapped state on this observed state.
// Called by Collector when MappedParentStateProvider callback is configured.
// Children can check if parent wants them running via StateMapping.
func (o ExamplechildObservedState) SetParentMappedState(state string) fsmv2.ObservedState {
	o.ParentMappedState = state

	return o
}

// IsStopRequired reports whether the child needs to stop.
// This is a QUERY on injected data, not a signal emission.
// It combines:
//   - IsShutdownRequested() - explicit system shutdown
//   - !ShouldBeRunning() - parent no longer wants child running
func (o ExamplechildObservedState) IsStopRequired() bool {
	return o.IsShutdownRequested() || !o.ShouldBeRunning()
}

// NOTE: SetActionHistory was REMOVED - collector should NOT modify ObservedState.
// Workers read deps.GetActionHistory() and assign to LastActionResults in CollectObservedState.
// This follows the same pattern as FrameworkMetrics (supervisor sets on deps, worker reads and assigns).
