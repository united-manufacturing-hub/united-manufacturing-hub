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

// ExampleparentSnapshot represents a point-in-time view of the parent worker state.
type ExampleparentSnapshot struct {
	Identity fsmv2.Identity
	Observed ExampleparentObservedState
	Desired  *ExampleparentDesiredState
}

// ExampleparentDesiredState represents the target configuration for the parent worker.
type ExampleparentDesiredState struct {
	config.BaseDesiredState        // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()
	ChildCount          int `json:"ChildCount"`
}

// ShouldBeRunning returns true if the parent should be in a running state.
// This is the positive assertion that should be checked before transitioning
// from stopped to starting states.
func (s *ExampleparentDesiredState) ShouldBeRunning() bool {
	return !s.ShutdownRequested
}

// ExampleparentObservedState represents the current state of the parent worker.
type ExampleparentObservedState struct {
	ID             string        `json:"id"`
	CollectedAt    time.Time     `json:"collected_at"`
	StateEnteredAt time.Time     `json:"state_entered_at"` // When current state was entered (from StateTracker)
	Elapsed        time.Duration `json:"elapsed"`          // Time since state was entered (pre-computed, mockable via Clock)

	ExampleparentDesiredState `json:",inline"`

	State             string `json:"state"` // Observed lifecycle state (e.g., "running_connected")
	ChildrenHealthy   int    `json:"children_healthy"`
	ChildrenUnhealthy int    `json:"children_unhealthy"`
}

func (o ExampleparentObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o ExampleparentObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ExampleparentDesiredState
}

// SetState sets the FSM state name on this observed state.
// Called by Collector when StateProvider callback is configured.
func (o ExampleparentObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
// Called by Collector when ShutdownRequestedProvider callback is configured.
func (o ExampleparentObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// SetChildrenCounts sets the children health counts on this observed state.
// Called by Collector when ChildrenCountsProvider callback is configured.
func (o ExampleparentObservedState) SetChildrenCounts(healthy, unhealthy int) fsmv2.ObservedState {
	o.ChildrenHealthy = healthy
	o.ChildrenUnhealthy = unhealthy

	return o
}
