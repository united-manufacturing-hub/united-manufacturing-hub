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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// ExamplefailingDependencies interface to avoid import cycles.
type ExamplefailingDependencies interface {
	deps.Dependencies
	GetShouldFail() bool
	IncrementAttempts() int
	GetAttempts() int
	ResetAttempts()
	GetMaxFailures() int
	SetConnected(connected bool)
	IsConnected() bool
	GetRestartAfterFailures() int
	GetFailureCycles() int
	GetCurrentCycle() int
	AllCyclesComplete() bool
	AdvanceCycle() int
	IncrementTicksInConnected() int
	GetTicksInConnected() int
	ResetTicksInConnected()
	// Recovery delay support (time-based - kept for backward compatibility)
	SetLastFailureTime(t time.Time)
	GetLastFailureTime() time.Time
	ShouldDelayRecovery() bool
	GetRecoveryDelayMs() int
	// Recovery delay support (observation-based - preferred)
	SetRecoveryDelayObservations(n int)
	GetRecoveryDelayObservations() int
	IncrementObservationsSinceFailure() int
	GetObservationsSinceFailure() int
	ResetObservationsSinceFailure()
}

// ExamplefailingSnapshot represents a point-in-time view of the failing worker state.
type ExamplefailingSnapshot struct {
	Identity deps.Identity
	Desired  ExamplefailingDesiredState
	Observed ExamplefailingObservedState
}

// ExamplefailingDesiredState represents the target configuration for the failing worker.
// Dependencies are NOT stored here - they belong in the Worker struct.
type ExamplefailingDesiredState struct {
	// Injected by supervisor via MappedParentStateProvider callback.
	ParentMappedState string `json:"parent_mapped_state"`

	config.BaseDesiredState // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

	ShouldFail bool `json:"ShouldFail"`
}

// ShouldBeRunning returns true if ShutdownRequested is false and parent wants children to run.
func (s *ExamplefailingDesiredState) ShouldBeRunning() bool {
	if s.ShutdownRequested {
		return false
	}

	return s.ParentMappedState == config.DesiredStateRunning
}

func (s *ExamplefailingDesiredState) IsShouldFail() bool {
	return s.ShouldFail
}

type ExamplefailingObservedState struct {
	CollectedAt time.Time `json:"collected_at"`

	LastError error  `json:"last_error,omitempty"`
	ID        string `json:"id"`

	State            string `json:"state"`
	ConnectionHealth string `json:"connection_health"`

	ExamplefailingDesiredState `json:",inline"`

	// Supervisor-managed: auto-recorded via ActionExecutor and injected into deps.
	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	deps.MetricsEmbedder `json:",inline"`

	ConnectAttempts       int `json:"connect_attempts"`
	RestartAfterFailures  int `json:"restart_after_failures"`
	TicksInConnectedState int `json:"ticks_in_connected"`
	CurrentCycle          int `json:"current_cycle"`
	TotalCycles           int `json:"total_cycles"`

	AllCyclesComplete bool `json:"all_cycles_complete"`

	// RecoveryDelayActive is true when waiting after a failure before retrying.
	// This keeps the worker in the unhealthy state long enough for parents to observe.
	RecoveryDelayActive bool `json:"recovery_delay_active"`
	// ObservationsSinceFailure tracks how many observation cycles have passed since the last failure.
	ObservationsSinceFailure int `json:"observations_since_failure"`
}

func (o ExamplefailingObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o ExamplefailingObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ExamplefailingDesiredState
}

func (o ExamplefailingObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

func (o ExamplefailingObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

func (o ExamplefailingObservedState) SetParentMappedState(state string) fsmv2.ObservedState {
	o.ParentMappedState = state

	return o
}

// IsStopRequired reports whether shutdown is requested or parent wants child stopped.
func (o ExamplefailingObservedState) IsStopRequired() bool {
	return o.IsShutdownRequested() || !o.ShouldBeRunning()
}
