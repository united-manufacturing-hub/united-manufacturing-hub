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

type SlowDependencies interface {
	fsmv2.Dependencies
}

type SlowSnapshot struct {
	Identity fsmv2.Identity
	Observed SlowObservedState
	Desired  SlowDesiredState
}

type SlowDesiredState struct {
	helpers.BaseDesiredState          // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()
	ShouldRun            bool
	DelaySeconds         int
	Dependencies         SlowDependencies
}

func (s *SlowDesiredState) ShouldBeRunning() bool {
	return !s.ShutdownRequested && s.ShouldRun
}

type SlowObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`

	SlowDesiredState

	ConnectionStatus string `json:"connection_status"`
	LastError        error  `json:"last_error,omitempty"`
	ConnectAttempts  int    `json:"connect_attempts"`
	ConnectionHealth string `json:"connection_health"`
}

func (o SlowObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o SlowObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.SlowDesiredState
}
