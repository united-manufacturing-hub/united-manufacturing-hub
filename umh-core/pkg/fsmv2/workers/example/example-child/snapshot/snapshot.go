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
)

// ChildDependencies interface to avoid import cycles
type ChildDependencies interface {
	fsmv2.Dependencies
}

// ChildSnapshot represents a point-in-time view of the child worker state
type ChildSnapshot struct {
	Identity fsmv2.Identity
	Observed ChildObservedState
	Desired  ChildDesiredState
}

// ChildDesiredState represents the target configuration for the child worker
type ChildDesiredState struct {
	shutdownRequested bool
	Dependencies      ChildDependencies
}

func (s *ChildDesiredState) IsShutdownRequested() bool {
	return s.shutdownRequested
}

// ChildObservedState represents the current state of the child worker
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
