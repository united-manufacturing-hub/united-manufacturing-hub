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

type ExampleslowDependencies interface {
	deps.Dependencies
	SetConnected(connected bool)
	IsConnected() bool
	SetDelaySeconds(delaySeconds int)
	GetDelaySeconds() int
}

type ExampleslowSnapshot struct {
	Identity deps.Identity
	Desired  ExampleslowDesiredState
	Observed ExampleslowObservedState
}

type ExampleslowDesiredState struct {
	// ParentMappedState is the desired state derived from parent's ChildStartStates via state mapping.
	ParentMappedState string `json:"parent_mapped_state"`

	config.BaseDesiredState

	DelaySeconds int
}

// ShouldBeRunning returns true if not shutting down and parent wants children to run.
func (s *ExampleslowDesiredState) ShouldBeRunning() bool {
	if s.ShutdownRequested {
		return false
	}
	return s.ParentMappedState == config.DesiredStateRunning
}

type ExampleslowObservedState struct {
	CollectedAt time.Time `json:"collected_at"`

	LastError error  `json:"last_error,omitempty"`
	ID        string `json:"id"`

	State            string `json:"state"` // Observed lifecycle state (e.g., "running_connected")
	ConnectionHealth string `json:"connection_health"`

	ExampleslowDesiredState `json:",inline"`

	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	deps.MetricsEmbedder `json:",inline"` // Framework and worker metrics for Prometheus export

	ConnectAttempts int `json:"connect_attempts"`
}

func (o ExampleslowObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o ExampleslowObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ExampleslowDesiredState
}

func (o ExampleslowObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

func (o ExampleslowObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

func (o ExampleslowObservedState) SetParentMappedState(state string) fsmv2.ObservedState {
	o.ParentMappedState = state

	return o
}

// IsStopRequired returns true if shutdown is requested or parent no longer wants child running.
func (o ExampleslowObservedState) IsStopRequired() bool {
	return o.IsShutdownRequested() || !o.ShouldBeRunning()
}
