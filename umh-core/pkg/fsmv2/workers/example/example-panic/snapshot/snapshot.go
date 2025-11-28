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

type PanicDependencies interface {
	fsmv2.Dependencies
}

type PanicSnapshot struct {
	Identity fsmv2.Identity
	Observed PanicObservedState
	Desired  PanicDesiredState
}

type PanicDesiredState struct {
	config.BaseDesiredState          // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()
	ShouldPanic      bool `json:"ShouldPanic"`
	Dependencies     PanicDependencies
}

func (s *PanicDesiredState) ShouldBeRunning() bool {
	return !s.ShutdownRequested
}

func (s *PanicDesiredState) IsShouldPanic() bool {
	return s.ShouldPanic
}

// InjectDependencies implements fsmv2.DependencyInjector.
func (s *PanicDesiredState) InjectDependencies(deps any) {
	if typedDeps, ok := deps.(PanicDependencies); ok {
		s.Dependencies = typedDeps
	}
}

type PanicObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`

	PanicDesiredState `json:",inline"`

	State            string `json:"state"` // Observed lifecycle state (e.g., "running_connected")
	LastError        error  `json:"last_error,omitempty"`
	ConnectAttempts  int    `json:"connect_attempts"`
	ConnectionHealth string `json:"connection_health"`
}

func (o PanicObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o PanicObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.PanicDesiredState
}
