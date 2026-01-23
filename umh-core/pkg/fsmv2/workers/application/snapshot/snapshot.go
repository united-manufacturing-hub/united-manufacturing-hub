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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ApplicationObservedState represents the minimal observed state for an application supervisor.
// It embeds the desired state to track what was actually deployed.
type ApplicationObservedState struct {
	CollectedAt time.Time `json:"collected_at"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`

	State string `json:"state"` // Observed lifecycle state (e.g., "running_connected")

	// DeployedDesiredState is what was last deployed to this application.
	ApplicationDesiredState `json:",inline"`

	// Embedded metrics for both framework and worker metrics.
	// Framework metrics provide time-in-state via GetFrameworkMetrics().TimeInCurrentStateMs
	// and state entered time via GetFrameworkMetrics().StateEnteredAtUnix.
	deps.MetricsEmbedder `json:",inline"`
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

// ApplicationDesiredState represents the desired state for an application supervisor.
// It embeds config.BaseDesiredState directly (consistent with ALL other workers)
// and declares ChildrenSpecs as a named field.
type ApplicationDesiredState struct {
	config.BaseDesiredState `json:",inline"`

	// Name is the identifier for this application supervisor.
	Name string `json:"name"`

	// ChildrenSpecs declares the children this application supervisor should manage.
	// Supervisor propagates shutdown to children via database mechanism.
	ChildrenSpecs []config.ChildSpec `json:"childrenSpecs,omitempty"`
}

// GetChildrenSpecs returns the children specifications.
func (d *ApplicationDesiredState) GetChildrenSpecs() []config.ChildSpec {
	return d.ChildrenSpecs
}

// NOTE: IsShutdownRequested() and SetShutdownRequested() are provided by embedded BaseDesiredState.
// No delegation methods needed - this is consistent with all other workers.
