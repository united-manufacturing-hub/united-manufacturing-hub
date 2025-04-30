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
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
)

// ErrAlreadyRunning is returned when trying to start a StateMocker that is already running
var ErrAlreadyRunning = errors.New("state mocker already running")

// ConfigManager interface defines the methods needed by StateMocker to get configuration data
type ConfigManager interface {
	// GetDataFlowConfig returns the DataFlow component configurations
	GetDataFlowConfig() []config.DataFlowComponentConfig
}

// StateTransition represents a state transition for a component
type StateTransition struct {
	TickAt int
	State  string
}

// the state mocker is used for unit testing to mock the state of the system
// it runs in a separate goroutine and regularly updates the state of the system
// it is passed a config manager that provides the config being used by the action unit tests
// it then updates the state of the system according to the config just like the real system would do
type StateMocker struct {
	State              *fsm.SystemSnapshot
	ConfigManager      ConfigManager
	TickCounter        int
	PendingTransitions map[string][]StateTransition // key is component ID
	done               chan struct{}                // channel to signal shutdown of goroutine
	running            atomic.Bool                  // flag to track if the mocker is running
	mu                 *sync.RWMutex                // mutex to protect the state of the mocker
}

// NewStateMocker creates a new StateMocker
func NewStateMocker(configManager ConfigManager) *StateMocker {
	return &StateMocker{
		ConfigManager: configManager,
		State: &fsm.SystemSnapshot{
			Managers: make(map[string]fsm.ManagerSnapshot),
		},
		TickCounter:        0,
		PendingTransitions: make(map[string][]StateTransition),
		done:               make(chan struct{}),
		mu:                 &sync.RWMutex{},
	}
}

// GetSystemState returns the current state of the system
// it is used by the action unit tests to check the state of the system
// it is called in a separate goroutine and regularly updates the state of the system
// it then updates the state of the system according to the config just like the real system would do
func (s *StateMocker) GetSystemState() *fsm.SystemSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

func (s *StateMocker) GetMutex() *sync.RWMutex {
	return s.mu
}

// GetConfigManager returns the config manager that is being used by the state mocker
func (s *StateMocker) GetConfigManager() ConfigManager {
	return s.ConfigManager
}

// Tick advances the state mocker by one tick and updates the system state
func (s *StateMocker) Tick() {
	s.TickCounter++
	s.UpdateDfcState()
}

// here, the actual state update logic is implemented
func (s *StateMocker) UpdateDfcState() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// only take the dataflowcomponent configs and add them to the state of the dataflowcomponent manager

	managerSnapshot := s.createDfcManagerSnapshot()

	// update the state of the system with the new manager snapshot
	if s.State.Managers == nil {
		s.State.Managers = make(map[string]fsm.ManagerSnapshot)
	}
	s.State.Managers[constants.DataflowcomponentManagerName] = managerSnapshot
}

// createDfcManagerSnapshot creates a snapshot of the DataFlowComponent manager state
// it therefore uses the config manager to get the dataflowcomponent configs
// and the pending transitions to update the state of the dataflowcomponent instances based on the tick counter
func (s *StateMocker) createDfcManagerSnapshot() fsm.ManagerSnapshot {
	//start with a basic snapshot
	dfcManagerInstaces := map[string]*fsm.FSMInstanceSnapshot{}

	for _, curDataflowcomponent := range s.ConfigManager.GetDataFlowConfig() {
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

	return &MockManagerSnapshot{
		Instances: dfcManagerInstaces,
	}
}

// Run starts the state mocker and updates the state of the system periodically
// It returns an error if already running
func (s *StateMocker) Run() error {
	if s.running.Swap(true) {
		return ErrAlreadyRunning
	}

	ticker := time.NewTicker(constants.DefaultTickerTime)

	go func() {
		defer ticker.Stop()
		defer s.running.Store(false)

		for {
			select {
			case <-ticker.C:
				s.Tick()
			case <-s.done:
				return
			}
		}
	}()

	return nil
}

// Start is a convenience method that runs the state mocker in a new goroutine
// It returns an error if already running
func (s *StateMocker) Start() error {
	if err := s.Run(); err != nil {
		return err
	}
	return nil
}

// Stop signals the state mocker to stop updating and waits for it to complete
// It is safe to call Stop even if the mocker isn't running
func (s *StateMocker) Stop() {
	if s.running.Load() {
		close(s.done)
		// Create a new channel for next run
		s.done = make(chan struct{})
	}
}

// SetTransitionSequence schedules state transitions for a component
// Each transition is defined by (tickOffset, state) where tickOffset is relative to current tick
func (s *StateMocker) SetTransitionSequence(componentID string, transitions []struct {
	TickOffset int
	State      string
}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	absoluteTransitions := make([]StateTransition, len(transitions))
	for i, t := range transitions {
		absoluteTransitions[i] = StateTransition{
			TickAt: s.TickCounter + t.TickOffset,
			State:  t.State,
		}
	}
	s.PendingTransitions[componentID] = absoluteTransitions
}

// CreateMockSystemSnapshotWithMissingState is a helper function to create a system snapshot with a component that has no observed state
func CreateMockSystemSnapshotWithMissingState() *fsm.SystemSnapshot {
	// Create a dataflowcomponent manager with an instance that has no observed state
	instanceSlice := []fsm.FSMInstanceSnapshot{
		{
			ID:                "test-component-missing-state",
			DesiredState:      "active",
			CurrentState:      "active",
			LastObservedState: nil, // No observed state
		},
	}

	// Convert slice to map
	instances := make(map[string]*fsm.FSMInstanceSnapshot)
	for i := range instanceSlice {
		instance := instanceSlice[i]
		instances[instance.ID] = &instance
	}

	managerSnapshot := &MockManagerSnapshot{
		Instances: instances,
	}

	// Create and return system snapshot
	return &fsm.SystemSnapshot{
		Managers: map[string]fsm.ManagerSnapshot{
			constants.DataflowcomponentManagerName: managerSnapshot,
		},
	}
}

// MockManagerSnapshot is a simple implementation of ManagerSnapshot interface for testing
type MockManagerSnapshot struct {
	Instances map[string]*fsm.FSMInstanceSnapshot
}

func (m *MockManagerSnapshot) GetName() string {
	return constants.DataflowcomponentManagerName
}

func (m *MockManagerSnapshot) GetInstances() map[string]*fsm.FSMInstanceSnapshot {
	return m.Instances
}

// GetInstance returns an FSM instance by ID
func (m *MockManagerSnapshot) GetInstance(id string) *fsm.FSMInstanceSnapshot {
	if instance, exists := m.Instances[id]; exists {
		return instance
	}
	return nil
}

func (m *MockManagerSnapshot) GetSnapshotTime() time.Time {
	return time.Now()
}

func (m *MockManagerSnapshot) GetManagerTick() uint64 {
	return 0
}

// MockObservedState is a fake implementation of ObservedStateSnapshot for testing
type MockObservedState struct{}

func (m *MockObservedState) IsObservedStateSnapshot() {}
