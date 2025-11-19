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

// Package root provides a generic passthrough root supervisor for FSMv2.
// The root supervisor dynamically creates children based on YAML configuration,
// allowing any registered worker type to be instantiated as a child.
package root

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// PassthroughObservedState represents the minimal observed state for a root supervisor.
// It embeds the desired state to track what was actually deployed.
type PassthroughObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collected_at"`
	Name        string    `json:"name"`

	// DeployedDesiredState is what was last deployed to this root.
	DeployedDesiredState PassthroughDesiredState `json:"deployed_desired_state"`
}

// GetTimestamp returns the time when this observed state was collected.
func (o PassthroughObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// GetObservedDesiredState returns the desired state that was actually deployed.
func (o PassthroughObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.DeployedDesiredState
}

// PassthroughDesiredState represents the desired state for a root supervisor.
// It embeds config.DesiredState to get ChildrenSpecs and IsShutdownRequested().
type PassthroughDesiredState struct {
	config.DesiredState

	// Name is the identifier for this root supervisor.
	Name string `json:"name"`
}

// IsShutdownRequested returns true if shutdown has been requested.
// This delegates to the embedded config.DesiredState.
func (d *PassthroughDesiredState) IsShutdownRequested() bool {
	return d.DesiredState.IsShutdownRequested()
}

// GetChildrenSpecs returns the children specifications from the embedded DesiredState.
func (d *PassthroughDesiredState) GetChildrenSpecs() []config.ChildSpec {
	return d.ChildrenSpecs
}

// Config holds the configuration for creating a root supervisor.
type Config struct {
	// Name is the identifier for this root supervisor instance.
	Name string

	// YAMLConfig is the raw YAML configuration containing children specifications.
	// The root supervisor parses this to extract the children array.
	//
	// Example:
	//   children:
	//     - name: "child-1"
	//       workerType: "example-child"
	//       userSpec:
	//         config: |
	//           value: 10
	YAMLConfig string
}
