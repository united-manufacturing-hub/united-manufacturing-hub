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
	config.BaseDesiredState        // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()
	ShouldFail          bool `json:"ShouldFail"`
}

// ShouldBeRunning returns true if the failing worker should be in a running/connected state.
// This is the positive assertion that should be checked before transitioning
// from stopped to starting states.
func (s *ExamplefailingDesiredState) ShouldBeRunning() bool {
	return !s.ShutdownRequested
}

func (s *ExamplefailingDesiredState) IsShouldFail() bool {
	return s.ShouldFail
}

// ExamplefailingObservedState represents the current state of the failing worker.
type ExamplefailingObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`

	ExamplefailingDesiredState `json:",inline"`

	State            string `json:"state"` // Observed lifecycle state (e.g., "running_connected")
	LastError        error  `json:"last_error,omitempty"`
	ConnectAttempts  int    `json:"connect_attempts"`
	ConnectionHealth string `json:"connection_health"`
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
	o.ExamplefailingDesiredState.ShutdownRequested = v
	return o
}
