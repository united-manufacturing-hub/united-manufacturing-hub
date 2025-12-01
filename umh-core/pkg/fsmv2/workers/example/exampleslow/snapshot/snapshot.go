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

type ExampleslowDependencies interface {
	fsmv2.Dependencies
}

type ExampleslowSnapshot struct {
	Identity fsmv2.Identity
	Observed ExampleslowObservedState
	Desired  ExampleslowDesiredState
}

type ExampleslowDesiredState struct {
	config.BaseDesiredState // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()
	ShouldRun    bool
	DelaySeconds int
	// Dependencies removed: Actions receive deps via Execute() parameter, not DesiredState
}

func (s *ExampleslowDesiredState) ShouldBeRunning() bool {
	return !s.ShutdownRequested && s.ShouldRun
}

type ExampleslowObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`

	ExampleslowDesiredState `json:",inline"`

	State            string `json:"state"` // Observed lifecycle state (e.g., "running_connected")
	LastError        error  `json:"last_error,omitempty"`
	ConnectAttempts  int    `json:"connect_attempts"`
	ConnectionHealth string `json:"connection_health"`
}

func (o ExampleslowObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o ExampleslowObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ExampleslowDesiredState
}

// SetState sets the FSM state name on this observed state.
// Called by Collector when StateProvider callback is configured.
func (o ExampleslowObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s
	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
// Called by Collector when ShutdownRequestedProvider callback is configured.
func (o ExampleslowObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ExampleslowDesiredState.ShutdownRequested = v
	return o
}
