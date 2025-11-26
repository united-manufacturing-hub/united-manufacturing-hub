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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
)

// ChildDependencies interface to avoid import cycles.
// Includes methods for connection state management used by actions.
type ChildDependencies interface {
	fsmv2.Dependencies
	// SetConnected updates the connection state. Called by ConnectAction/DisconnectAction.
	SetConnected(connected bool)
	// IsConnected returns the current connection state. Called by CollectObservedState.
	IsConnected() bool
}

// ChildSnapshot represents a point-in-time view of the child worker state.
// This is the combined snapshot type for type assertions in Next() methods.
type ChildSnapshot struct {
	Identity fsmv2.Identity
	Observed ChildObservedState
	Desired  *ChildDesiredState
}

// ChildDesiredState represents the target configuration for the child worker.
type ChildDesiredState struct {
	helpers.BaseDesiredState          // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()
	Dependencies             ChildDependencies
}

// ShouldBeRunning returns true if the child should be in a running/connected state.
// This is the positive assertion that should be checked before transitioning
// from stopped to starting states.
func (s *ChildDesiredState) ShouldBeRunning() bool {
	return !s.ShutdownRequested
}

// ChildObservedState represents the current state of the child worker.
type ChildObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`

	ChildDesiredState

	ConnectionStatus string `json:"connection_status"`
	LastError        error  `json:"last_error,omitempty"`
	ConnectAttempts  int    `json:"connect_attempts"`
	ConnectionHealth string `json:"connection_health"`
}

func (o ChildObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o ChildObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ChildDesiredState
}
