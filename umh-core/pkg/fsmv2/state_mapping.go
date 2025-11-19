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

package fsmv2

// StateMapping defines how parent state maps to child desired states.
// This enables parent FSMs to declaratively specify what state their
// children should be in when the parent is in a particular state.
type StateMapping struct {
	// ParentState is the state that triggers child updates
	ParentState string

	// ChildDesired maps child IDs to their desired states
	// Example: {"child-1": "running", "child-2": "running"}
	ChildDesired map[string]string

	// Condition is optional function to check before applying mapping.
	// If nil, the mapping is always applied.
	// If provided, mapping is only applied when condition returns true.
	Condition func(parentSnapshot Snapshot) bool
}

// StateMappingRegistry holds mappings for hierarchical FSMs.
// It enables parent FSMs to control child FSM desired states
// through a discoverable and easy-to-use API.
type StateMappingRegistry struct {
	mappings []StateMapping
}

// NewStateMappingRegistry creates a new empty registry.
func NewStateMappingRegistry() *StateMappingRegistry {
	return &StateMappingRegistry{
		mappings: make([]StateMapping, 0),
	}
}

// Register adds a new state mapping to the registry.
func (r *StateMappingRegistry) Register(mapping StateMapping) {
	r.mappings = append(r.mappings, mapping)
}

// DeriveChildDesiredStates returns the desired states for all children
// based on the current parent state. It evaluates all registered mappings
// for the given parent state and merges the results.
//
// Later mappings can override earlier ones for the same child ID.
// Mappings with conditions are only included if the condition returns true.
func (r *StateMappingRegistry) DeriveChildDesiredStates(
	parentState string,
	parentSnapshot Snapshot,
) map[string]string {
	result := make(map[string]string)

	for _, mapping := range r.mappings {
		if mapping.ParentState == parentState {
			if mapping.Condition == nil || mapping.Condition(parentSnapshot) {
				for childID, desiredState := range mapping.ChildDesired {
					result[childID] = desiredState
				}
			}
		}
	}

	return result
}

// GetChildDesiredState returns the desired state for a specific child
// based on the current parent state. Returns the state and true if found,
// or empty string and false if not found.
func (r *StateMappingRegistry) GetChildDesiredState(
	parentState string,
	childID string,
	parentSnapshot Snapshot,
) (string, bool) {
	states := r.DeriveChildDesiredStates(parentState, parentSnapshot)
	state, ok := states[childID]
	return state, ok
}
