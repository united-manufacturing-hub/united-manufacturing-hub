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

package actions

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
)

// the state mocker is used for unit testing to mock the state of the system
// it runs in a separate goroutine and regularly updates the state of the system
// it is passed a pointer to the config that is being used by the action unit tests
// it then updates the state of the system according to the config just like the real system would do
type StateMocker struct {
	State              *fsm.SystemSnapshot
	Config             *config.FullConfig
	TickCounter        int
	PendingTransitions map[string][]StateTransition // key is component ID
}

// NewStateMocker creates a new StateMocker
func NewStateMocker(config *config.FullConfig) *StateMocker {
	return &StateMocker{
		Config:             config,
		PendingTransitions: make(map[string][]StateTransition),
	}
}

// GetState returns the current state of the system
// it is used by the action unit tests to check the state of the system
// it is called in a separate goroutine and regularly updates the state of the system
// it is passed a pointer to the config that is being used by the action unit tests
// it then updates the state of the system according to the config just like the real system would do

func (s *StateMocker) GetState() *fsm.SystemSnapshot {
	return s.State
}

// Tick advances the state mocker by one tick and updates the system state
func (s *StateMocker) Tick() {
	s.TickCounter++
	s.UpdateState()
}

// here, the actual state update logic is implemented
func (s *StateMocker) UpdateState() {
	// only take the dataflowcomponent configs and add them to the state of the dataflowcomponent manager
	// the other managers are not updated

	//start with a basic snapshot
	dfcManagerInstaces := map[string]*fsm.FSMInstanceSnapshot{}

	for _, curDataflowcomponent := range s.Config.DataFlow {
		// get the default currentState from the last observed state (s.state)
		currentState := curDataflowcomponent.DesiredFSMState
		var instances map[string]*fsm.FSMInstanceSnapshot
		if s.State != nil && s.State.Managers != nil {
			if manager, ok := s.State.Managers[constants.DataflowcomponentManagerName]; ok {
				instances = manager.GetInstances()
			}
		}

		if instance, ok := instances[curDataflowcomponent.Name]; ok {
			currentState = instance.CurrentState
		}

		// Check if there are pending transitions for this component
		if transitions, exists := s.PendingTransitions[curDataflowcomponent.Name]; exists {
			// Find transitions that should be applied at this tick
			for i, transition := range transitions {
				if transition.TickAt <= s.TickCounter {
					currentState = transition.State

					// Remove applied transitions if they've been processed
					if i < len(transitions)-1 {
						s.PendingTransitions[curDataflowcomponent.Name] = transitions[i+1:]
					} else {
						// All transitions applied, clear the list
						delete(s.PendingTransitions, curDataflowcomponent.Name)
					}
					break
				}
			}
		}

		dfcManagerInstaces[curDataflowcomponent.Name] = &fsm.FSMInstanceSnapshot{
			ID:           curDataflowcomponent.Name,
			DesiredState: curDataflowcomponent.DesiredFSMState,
			CurrentState: currentState,
			LastObservedState: &dataflowcomponent.DataflowComponentObservedStateSnapshot{
				Config: curDataflowcomponent.DataFlowComponentServiceConfig,
			},
		}
	}

	managerSnapshot := &MockManagerSnapshot{
		Instances: dfcManagerInstaces,
	}

	snapshot := &fsm.SystemSnapshot{
		Managers: map[string]fsm.ManagerSnapshot{
			constants.DataflowcomponentManagerName: managerSnapshot,
		},
	}

	s.State = snapshot
}

// UpdateState is spawned as a separate goroutine and updates the state of the system
func (s *StateMocker) Start() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.Tick()
	}
}

type StateTransition struct {
	TickAt int
	State  string
}

// SetTransitionSequence schedules state transitions for a component
// Each transition is defined by (tickOffset, state) where tickOffset is relative to current tick
func (s *StateMocker) SetTransitionSequence(componentID string, transitions []struct {
	TickOffset int
	State      string
}) {
	absoluteTransitions := make([]StateTransition, len(transitions))
	for i, t := range transitions {
		absoluteTransitions[i] = StateTransition{
			TickAt: s.TickCounter + t.TickOffset,
			State:  t.State,
		}
	}
	s.PendingTransitions[componentID] = absoluteTransitions
}
