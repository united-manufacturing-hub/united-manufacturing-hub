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

// FailingDependencies interface to avoid import cycles.
type FailingDependencies interface {
	fsmv2.Dependencies
	GetShouldFail() bool
}

// FailingSnapshot represents a point-in-time view of the failing worker state.
type FailingSnapshot struct {
	Identity fsmv2.Identity
	Observed FailingObservedState
	Desired  FailingDesiredState
}

// FailingDesiredState represents the target configuration for the failing worker.
// NOTE: Dependencies are NOT stored here - they belong in the Worker struct.
// See fsmv2.DesiredState documentation for the architectural invariant.
type FailingDesiredState struct {
	config.BaseDesiredState        // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()
	ShouldFail          bool `json:"ShouldFail"`
}

// ShouldBeRunning returns true if the failing worker should be in a running/connected state.
// This is the positive assertion that should be checked before transitioning
// from stopped to starting states.
func (s *FailingDesiredState) ShouldBeRunning() bool {
	return !s.ShutdownRequested
}

func (s *FailingDesiredState) IsShouldFail() bool {
	return s.ShouldFail
}

// FailingObservedState represents the current state of the failing worker.
type FailingObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`

	FailingDesiredState `json:",inline"`

	State            string `json:"state"` // Observed lifecycle state (e.g., "running_connected")
	LastError        error  `json:"last_error,omitempty"`
	ConnectAttempts  int    `json:"connect_attempts"`
	ConnectionHealth string `json:"connection_health"`
}

func (o FailingObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o FailingObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.FailingDesiredState
}
