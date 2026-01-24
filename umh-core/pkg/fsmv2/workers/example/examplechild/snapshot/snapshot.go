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
type ExamplechildDependencies interface {
	deps.Dependencies
	SetConnected(connected bool)
	IsConnected() bool
}

// ExamplechildSnapshot represents a point-in-time view of the child worker state.
type ExamplechildSnapshot struct {
	Desired  *ExamplechildDesiredState
	Identity deps.Identity
	Observed ExamplechildObservedState
}

// ExamplechildDesiredState represents the target configuration for the child worker.
type ExamplechildDesiredState struct {
	// ParentMappedState derives from parent's ChildStartStates; injected via MappedParentStateProvider.
	ParentMappedState       string `json:"parent_mapped_state"`
	config.BaseDesiredState
}

// ShouldBeRunning returns true if the child should be in a running/connected state.
func (s *ExamplechildDesiredState) ShouldBeRunning() bool {
	if s.ShutdownRequested {
		return false
	}
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

	// LastActionResults contains the action history from the last collection cycle (supervisor-managed).
	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

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
func (o ExamplechildObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
func (o ExamplechildObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// SetParentMappedState sets the parent's mapped state on this observed state.
func (o ExamplechildObservedState) SetParentMappedState(state string) fsmv2.ObservedState {
	o.ParentMappedState = state

	return o
}

// IsStopRequired reports whether the child needs to stop.
func (o ExamplechildObservedState) IsStopRequired() bool {
	return o.IsShutdownRequested() || !o.ShouldBeRunning()
}

