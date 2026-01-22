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

// ExamplefailingDependencies interface to avoid import cycles.
// Includes all methods needed by actions.
type ExamplefailingDependencies interface {
	fsmv2.Dependencies
	// GetShouldFail returns whether the worker should simulate failures.
	GetShouldFail() bool
	// IncrementAttempts increments and returns the current attempt count.
	IncrementAttempts() int
	// GetAttempts returns the current attempt count without incrementing.
	GetAttempts() int
	// ResetAttempts resets the attempt counter to zero.
	ResetAttempts()
	// GetMaxFailures returns the configured maximum number of failures before success.
	GetMaxFailures() int
	// SetConnected marks the worker as connected.
	SetConnected(connected bool)
	// IsConnected returns whether the worker is currently connected.
	IsConnected() bool
	// GetRestartAfterFailures returns the restart threshold (0 = no restart).
	GetRestartAfterFailures() int
	// GetFailureCycles returns the configured number of failure cycles.
	GetFailureCycles() int
	// GetCurrentCycle returns the current failure cycle (0-indexed).
	GetCurrentCycle() int
	// AllCyclesComplete returns true if all failure cycles have been completed.
	AllCyclesComplete() bool
	// AdvanceCycle advances to the next failure cycle and resets attempts.
	AdvanceCycle() int
	// IncrementTicksInConnected increments and returns ticks in Connected state.
	IncrementTicksInConnected() int
	// GetTicksInConnected returns the number of ticks in Connected state.
	GetTicksInConnected() int
	// ResetTicksInConnected resets the ticks counter to zero.
	ResetTicksInConnected()
}

// ExamplefailingSnapshot represents a point-in-time view of the failing worker state.
type ExamplefailingSnapshot struct {
	Identity fsmv2.Identity
	Observed ExamplefailingObservedState
	Desired  ExamplefailingDesiredState
}

// ExamplefailingDesiredState represents the target configuration for the failing worker.
// NOTE: Dependencies are NOT stored here - they belong in the Worker struct.
// See fsmv2.DesiredState documentation for the architectural invariant.
type ExamplefailingDesiredState struct {
	config.BaseDesiredState // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

	// ParentMappedState is the desired state derived from parent's ChildStartStates.
	// When parent is in a state listed in ChildStartStates → "running"
	// When parent is in any other state → "stopped"
	// This field is injected by the supervisor via MappedParentStateProvider callback.
	ParentMappedState string `json:"parent_mapped_state"`

	ShouldFail bool `json:"ShouldFail"`
}

// ShouldBeRunning returns true if the failing worker should be in a running/connected state.
// This is the positive assertion that should be checked before transitioning
// from stopped to starting states.
//
// Children only run when:
// 1. ShutdownRequested is false (not being shut down)
// 2. ParentMappedState is config.DesiredStateRunning (parent wants children to run)
//
// Children wait for parent to reach TryingToStart before connecting.
func (s *ExamplefailingDesiredState) ShouldBeRunning() bool {
	if s.ShutdownRequested {
		return false
	}
	// Only run if parent explicitly wants us running via ChildStartStates
	// Default to not running if ParentMappedState is empty or "stopped"
	return s.ParentMappedState == config.DesiredStateRunning
}

func (s *ExamplefailingDesiredState) IsShouldFail() bool {
	return s.ShouldFail
}

// ExamplefailingObservedState represents the current state of the failing worker.
type ExamplefailingObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`

	ExamplefailingDesiredState `json:",inline"`

	State                 string `json:"state"` // Observed lifecycle state (e.g., "running_connected")
	LastError             error  `json:"last_error,omitempty"`
	ConnectAttempts       int    `json:"connect_attempts"`
	ConnectionHealth      string `json:"connection_health"`
	RestartAfterFailures  int    `json:"restart_after_failures"` // Restart threshold from config
	AllCyclesComplete     bool   `json:"all_cycles_complete"`    // True when all failure cycles done
	TicksInConnectedState int    `json:"ticks_in_connected"`     // Ticks spent in Connected state
	CurrentCycle          int    `json:"current_cycle"`          // Current failure cycle (0-indexed)
	TotalCycles           int    `json:"total_cycles"`           // Total failure cycles configured

	// LastActionResults contains the action history from the last collection cycle.
	// This is supervisor-managed data: the supervisor auto-records action results
	// via ActionExecutor callback and injects them into deps before CollectObservedState.
	// Workers read deps.GetActionHistory() and assign here in CollectObservedState.
	LastActionResults []fsmv2.ActionResult `json:"last_action_results,omitempty"`

	fsmv2.MetricsEmbedder `json:",inline"` // Framework and worker metrics for Prometheus export
}

func (o ExamplefailingObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o ExamplefailingObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ExamplefailingDesiredState
}

// SetState sets the FSM state name on this observed state.
// Called by Collector when StateProvider callback is configured.
func (o ExamplefailingObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
// Called by Collector when ShutdownRequestedProvider callback is configured.
func (o ExamplefailingObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// SetParentMappedState sets the parent's mapped state on this observed state.
// Called by Collector when MappedParentStateProvider callback is configured.
// Children can check if parent wants them running via StateMapping.
func (o ExamplefailingObservedState) SetParentMappedState(state string) fsmv2.ObservedState {
	o.ParentMappedState = state

	return o
}

// IsStopRequired reports whether the child needs to stop.
// This is a QUERY on injected data, not a signal emission.
// It combines:
//   - IsShutdownRequested() - explicit system shutdown
//   - !ShouldBeRunning() - parent no longer wants child running
func (o ExamplefailingObservedState) IsStopRequired() bool {
	return o.IsShutdownRequested() || !o.ShouldBeRunning()
}
