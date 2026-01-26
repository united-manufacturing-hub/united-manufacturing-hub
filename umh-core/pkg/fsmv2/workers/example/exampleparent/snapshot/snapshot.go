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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// ExampleparentSnapshot represents a point-in-time view of the parent worker state.
type ExampleparentSnapshot struct {
	Desired  *ExampleparentDesiredState
	Identity deps.Identity
	Observed ExampleparentObservedState
}

// ExampleparentDesiredState represents the target configuration for the parent worker.
type ExampleparentDesiredState struct {
	config.BaseDesiredState
	ChildCount int `json:"ChildCount"`
}

// ShouldBeRunning returns true if the parent should be in a running state.
func (s *ExampleparentDesiredState) ShouldBeRunning() bool {
	return !s.ShutdownRequested
}

// ExampleparentObservedState represents the current state of the parent worker.
type ExampleparentObservedState struct {
	CollectedAt time.Time `json:"collected_at"`

	ID string `json:"id"`

	State string `json:"state"`

	// Supervisor injects action history into deps; workers read via GetActionHistory().
	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	ExampleparentDesiredState `json:",inline"`

	deps.MetricsEmbedder `json:",inline"`

	ChildrenHealthy   int `json:"children_healthy"`
	ChildrenUnhealthy int `json:"children_unhealthy"`
}

func (o ExampleparentObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o ExampleparentObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ExampleparentDesiredState
}

// SetState sets the FSM state name on this observed state.
func (o ExampleparentObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
func (o ExampleparentObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// SetChildrenCounts sets the children health counts on this observed state.
func (o ExampleparentObservedState) SetChildrenCounts(healthy, unhealthy int) fsmv2.ObservedState {
	o.ChildrenHealthy = healthy
	o.ChildrenUnhealthy = unhealthy

	return o
}
