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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ExamplechildDependencies interface to avoid import cycles.
// Includes methods for connection state management used by actions.
type ExamplechildDependencies interface {
	fsmv2.Dependencies
	// SetConnected updates the connection state. Called by ConnectAction/DisconnectAction.
	SetConnected(connected bool)
	// IsConnected returns the current connection state. Called by CollectObservedState.
	IsConnected() bool
}

// ExamplechildSnapshot represents a point-in-time view of the child worker state.
// This is the combined snapshot type for type assertions in Next() methods.
type ExamplechildSnapshot struct {
	Identity fsmv2.Identity
	Observed ExamplechildObservedState
	Desired  *ExamplechildDesiredState
}

// ExamplechildDesiredState represents the target configuration for the child worker.
// NOTE: Dependencies are NOT stored here - they belong in the Worker struct.
// See fsmv2.DesiredState documentation for the architectural invariant.
type ExamplechildDesiredState struct {
	config.BaseDesiredState // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

	// ParentMappedState is the desired state derived from parent's ChildStartStates.
	// When parent is in a state listed in ChildStartStates → "running"
	// When parent is in any other state → "stopped"
	// This field is injected by the supervisor via MappedParentStateProvider callback.
	ParentMappedState string `json:"parent_mapped_state"`
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
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`

	ExamplechildDesiredState `json:",inline"`

	State            string `json:"state"` // Observed lifecycle state (e.g., "running_connected")
	LastError        error  `json:"last_error,omitempty"`
	ConnectAttempts  int    `json:"connect_attempts"`
	ConnectionHealth string `json:"connection_health"`
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
