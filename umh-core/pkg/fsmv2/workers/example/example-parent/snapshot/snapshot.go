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

// ParentSnapshot represents a point-in-time view of the parent worker state.
type ParentSnapshot struct {
	Identity fsmv2.Identity
	Observed ParentObservedState
	Desired  *ParentDesiredState
}

// ParentDesiredState represents the target configuration for the parent worker.
type ParentDesiredState struct {
	helpers.BaseDesiredState        // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()
	ChildCount               int `json:"ChildCount"`
}

// ShouldBeRunning returns true if the parent should be in a running state.
// This is the positive assertion that should be checked before transitioning
// from stopped to starting states.
func (s *ParentDesiredState) ShouldBeRunning() bool {
	return !s.ShutdownRequested
}

// ParentObservedState represents the current state of the parent worker.
type ParentObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`

	ParentDesiredState

	ChildrenHealthy   int `json:"children_healthy"`
	ChildrenUnhealthy int `json:"children_unhealthy"`
}

func (o ParentObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o ParentObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ParentDesiredState
}
