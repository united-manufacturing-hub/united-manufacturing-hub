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

// Package snapshot provides snapshot state types for the application worker.
package snapshot

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// ApplicationObservedState represents the observed state for an application supervisor.
type ApplicationObservedState struct {
	CollectedAt time.Time `json:"collected_at"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`

	State string `json:"state"` // Observed lifecycle state (e.g., "running_connected")

	// DeployedDesiredState is what was last deployed to this application.
	ApplicationDesiredState `json:",inline"`

	deps.MetricsEmbedder `json:",inline"`

	// Infrastructure health from ChildrenView (depth=1, direct children only)
	ChildrenHealthy     int `json:"children_healthy"`
	ChildrenUnhealthy   int `json:"children_unhealthy"`
	ChildrenCircuitOpen int `json:"children_circuit_open"`
	ChildrenStale       int `json:"children_stale"`
}

// GetTimestamp returns the time when this observed state was collected.
func (o ApplicationObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// GetObservedDesiredState returns the desired state that was actually deployed.
func (o ApplicationObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ApplicationDesiredState
}

// SetState sets the FSM state name on this observed state.
// Called by Collector when StateProvider callback is configured.
func (o ApplicationObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
// Called by Collector when ShutdownRequestedProvider callback is configured.
func (o ApplicationObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// SetChildrenView sets the children infrastructure health from the supervisor's ChildrenView.
// Called by Collector when ChildrenViewProvider callback is configured.
func (o ApplicationObservedState) SetChildrenView(view any) fsmv2.ObservedState {
	cv, ok := view.(config.ChildrenView)
	if !ok {
		return o
	}

	o.ChildrenHealthy, o.ChildrenUnhealthy = cv.Counts()
	o.ChildrenCircuitOpen = 0
	o.ChildrenStale = 0

	// Count infrastructure issues from children
	for _, child := range cv.List() {
		if child.IsCircuitOpen {
			o.ChildrenCircuitOpen++
		}

		if child.IsStale {
			o.ChildrenStale++
		}
	}

	return o
}

// HasInfrastructureIssues returns true if any children have circuit breaker open or stale observations.
func (o ApplicationObservedState) HasInfrastructureIssues() bool {
	return o.ChildrenCircuitOpen > 0 || o.ChildrenStale > 0
}

// InfrastructureReason returns a dynamic reason string for infrastructure issues.
func (o ApplicationObservedState) InfrastructureReason() string {
	if !o.HasInfrastructureIssues() {
		return ""
	}

	return fmt.Sprintf("infrastructure degraded: circuit_open=%d, stale=%d",
		o.ChildrenCircuitOpen, o.ChildrenStale)
}

// ApplicationDesiredState represents the desired state for an application supervisor.
type ApplicationDesiredState struct {
	config.BaseDesiredState `json:",inline"`

	// Name is the identifier for this application supervisor.
	Name string `json:"name"`

	// ChildrenSpecs declares the children this application supervisor should manage.
	ChildrenSpecs []config.ChildSpec `json:"childrenSpecs,omitempty"`
}

// GetChildrenSpecs returns the children specifications.
func (d *ApplicationDesiredState) GetChildrenSpecs() []config.ChildSpec {
	return d.ChildrenSpecs
}
