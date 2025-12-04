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

type ExamplepanicDependencies interface {
	fsmv2.Dependencies
}

type ExamplepanicSnapshot struct {
	Identity fsmv2.Identity
	Observed ExamplepanicObservedState
	Desired  ExamplepanicDesiredState
}

// ExamplepanicDesiredState represents the target configuration for the panic worker.
// NOTE: Dependencies are NOT stored here - they belong in the Worker struct.
// See fsmv2.DesiredState documentation for the architectural invariant.
type ExamplepanicDesiredState struct {
	config.BaseDesiredState // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()
	ShouldPanic             bool `json:"ShouldPanic"`
}

func (s *ExamplepanicDesiredState) ShouldBeRunning() bool {
	return !s.ShutdownRequested
}

func (s *ExamplepanicDesiredState) IsShouldPanic() bool {
	return s.ShouldPanic
}

type ExamplepanicObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`

	ExamplepanicDesiredState `json:",inline"`

	State            string `json:"state"` // Observed lifecycle state (e.g., "running_connected")
	LastError        error  `json:"last_error,omitempty"`
	ConnectAttempts  int    `json:"connect_attempts"`
	ConnectionHealth string `json:"connection_health"`
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
	o.ExamplepanicDesiredState.ShutdownRequested = v

	return o
}
