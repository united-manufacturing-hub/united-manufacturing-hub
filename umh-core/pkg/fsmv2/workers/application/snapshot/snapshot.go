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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ApplicationObservedState represents the minimal observed state for an application supervisor.
// It embeds the desired state to track what was actually deployed.
type ApplicationObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`
	Name        string    `json:"name"`

	// DeployedDesiredState is what was last deployed to this application.
	DeployedDesiredState ApplicationDesiredState `json:"deployed_desired_state"`
}

// GetTimestamp returns the time when this observed state was collected.
func (o ApplicationObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// GetObservedDesiredState returns the desired state that was actually deployed.
func (o ApplicationObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.DeployedDesiredState
}

// ApplicationDesiredState represents the desired state for an application supervisor.
// It embeds config.DesiredState to get ChildrenSpecs and IsShutdownRequested().
type ApplicationDesiredState struct {
	config.DesiredState

	// Name is the identifier for this application supervisor.
	Name string `json:"name"`
}

// IsShutdownRequested returns true if shutdown has been requested.
// This delegates to the embedded config.DesiredState.
func (d *ApplicationDesiredState) IsShutdownRequested() bool {
	return d.DesiredState.IsShutdownRequested()
}

// GetChildrenSpecs returns the children specifications from the embedded DesiredState.
func (d *ApplicationDesiredState) GetChildrenSpecs() []config.ChildSpec {
	return d.ChildrenSpecs
}
