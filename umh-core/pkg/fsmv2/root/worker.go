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

package root

import (
	"context"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// PassthroughWorker is a generic root worker that parses YAML configuration
// to dynamically discover and create child workers. It doesn't hardcode child
// types - any registered worker type can be instantiated as a child.
//
// This implements the "passthrough pattern" where the root supervisor simply
// passes through ChildrenSpecs from the YAML config without knowing about
// specific child implementations.
type PassthroughWorker struct {
	id   string
	name string
}

// NewPassthroughWorker creates a new passthrough root worker.
func NewPassthroughWorker(id, name string) *PassthroughWorker {
	return &PassthroughWorker{
		id:   id,
		name: name,
	}
}

// CollectObservedState returns the current observed state of the root supervisor.
// Since the root supervisor has minimal internal state, this mainly tracks
// the deployed desired state for comparison.
func (w *PassthroughWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	// Check context cancellation first.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return &PassthroughObservedState{
		ID:          w.id,
		CollectedAt: time.Now(),
		Name:        w.name,
		DeployedDesiredState: PassthroughDesiredState{
			DesiredState: config.DesiredState{
				State: "running",
			},
			Name: w.name,
		},
	}, nil
}

// childrenConfig is the structure for parsing children from YAML.
type childrenConfig struct {
	Children []config.ChildSpec `yaml:"children"`
}

// DeriveDesiredState parses the YAML configuration to extract children specifications.
// This is the core of the passthrough pattern - the root doesn't need to know about
// child types, it just passes through the ChildSpec array from the config.
//
// Example YAML config:
//
//	children:
//	  - name: "child-1"
//	    workerType: "example-child"
//	    userSpec:
//	      config: |
//	        value: 10
//	  - name: "child-2"
//	    workerType: "example-child"
//	    userSpec:
//	      config: |
//	        value: 20
func (w *PassthroughWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	// Handle nil spec (used during initialization in AddWorker).
	if spec == nil {
		return config.DesiredState{
			State:         "running",
			ChildrenSpecs: nil,
		}, nil
	}

	// Get UserSpec from the spec interface.
	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return config.DesiredState{}, fmt.Errorf("invalid spec type: expected config.UserSpec, got %T", spec)
	}

	// Parse children from YAML config.
	var childrenCfg childrenConfig
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &childrenCfg); err != nil {
			return config.DesiredState{}, fmt.Errorf("failed to parse children config: %w", err)
		}
	}

	return config.DesiredState{
		State:         "running",
		ChildrenSpecs: childrenCfg.Children,
	}, nil
}

// GetInitialState returns the starting state for this root worker.
// Currently returns nil as the state machine is not yet implemented.
// TODO: Implement state machine for root supervisor.
func (w *PassthroughWorker) GetInitialState() fsmv2.State[any, any] {
	// Return nil for now - state machine will be implemented later.
	// The supervisor can handle nil initial state gracefully.
	return nil
}
