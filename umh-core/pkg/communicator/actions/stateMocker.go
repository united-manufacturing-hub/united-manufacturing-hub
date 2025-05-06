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
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"go.uber.org/zap"
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
	StateManager       *fsm.SnapshotManager
	ConfigManager      ConfigManager
	TickCounter        int
	PendingTransitions map[string][]StateTransition // key is component ID
	done               chan struct{}                // channel to signal shutdown of goroutine
	running            atomic.Bool                  // flag to track if the mocker is running
	mu                 *sync.RWMutex                // mutex to protect the state of the mocker
	LastConfig         config.FullConfig            // last config is needed to detect config changes (events)
	LastConfigSet      bool                         // flag to track if the last config has been set
	IgnoreDfcUntilTick map[string]int               // map of component ID to tick to ignore
}

// NewStateMocker creates a new StateMocker
func NewStateMocker(configManager ConfigManager) *StateMocker {
	return &StateMocker{
		ConfigManager:      configManager,
		StateManager:       fsm.NewSnapshotManager(),
		TickCounter:        0,
		PendingTransitions: make(map[string][]StateTransition),
		done:               make(chan struct{}),
		mu:                 &sync.RWMutex{},
		LastConfig:         config.FullConfig{},
		LastConfigSet:      false,
		IgnoreDfcUntilTick: make(map[string]int),
	}
}

func (s *StateMocker) GetStateManager() *fsm.SnapshotManager {
	return s.StateManager
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

// UpdateDfcState updates the state of DataFlow components based on configuration changes.
// It detects config events (additions, removals, edits), schedules appropriate state transitions,
// and updates the system snapshot with the new manager state. This function is the core of the
// state mocking system, simulating how real components would transition between states.
func (s *StateMocker) UpdateDfcState() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// only take the dataflowcomponent configs and add them to the state of the dataflowcomponent manager

	//get the current and the last config
	curDfcConfig := s.ConfigManager.GetDataFlowConfig()
	lastDfcConfig := s.LastConfig.DataFlow

	// get the old system snapshot
	systemSnapshot := s.StateManager.GetDeepCopySnapshot()

	// detect config events (add, remove, edit) and add the corresponding state transitions to the pending transitions
	s.detectConfigEvents(curDfcConfig, lastDfcConfig)

	// update the last config
	s.LastConfig.DataFlow = curDfcConfig
	s.LastConfigSet = true

	// create a new manager snapshot based on the current system snapshot, the config and the pending transitions
	managerSnapshot := s.createDfcManagerSnapshot(systemSnapshot)

	// update the state of the system with the new manager snapshot
	if systemSnapshot.Managers == nil {
		managers := make(map[string]fsm.ManagerSnapshot)
		s.StateManager.UpdateSnapshot(&fsm.SystemSnapshot{
			Managers: managers,
		})
	}
	s.StateManager.UpdateSnapshot(&fsm.SystemSnapshot{
		Managers: map[string]fsm.ManagerSnapshot{
			constants.DataflowcomponentManagerName: managerSnapshot,
		},
	})
}

// detect config events (add, remove, edit) and add the corresponding state transitions to the pending transitions
func (s *StateMocker) detectConfigEvents(curDfcConfig []config.DataFlowComponentConfig, lastDfcConfig []config.DataFlowComponentConfig) {

	if !s.LastConfigSet {
		return
	}

	// detect added dfcs
	if len(curDfcConfig) > len(lastDfcConfig) {
		for _, dfcConfig := range curDfcConfig {
			found := false
			for _, existing := range lastDfcConfig {
				if existing.Name == dfcConfig.Name {
					found = true
					break
				}
			}
			if !found {
				zap.S().Info("Detected new dataflow component", zap.String("name", dfcConfig.Name))
				// add the default add transitions
				s.PendingTransitions[dfcConfig.Name] = []StateTransition{
					{TickAt: s.TickCounter, State: internalfsm.LifecycleStateToBeCreated},
					{TickAt: s.TickCounter + 2, State: internalfsm.LifecycleStateCreating},
					{TickAt: s.TickCounter + 4, State: dataflowcomponent.OperationalStateStarting},
					{TickAt: s.TickCounter + 15, State: dataflowcomponent.OperationalStateActive},
				}
			}
		}
		return
	}

	// detect removed dfcs
	if len(curDfcConfig) < len(lastDfcConfig) {
		for _, existing := range lastDfcConfig {
			found := false
			for _, new := range curDfcConfig {
				if new.Name == existing.Name {
					found = true
					break
				}
			}
			if !found {
				// we wait for 10 ticks before we remove the component from the state
				// in the meantime, the state will be updated
				zap.S().Info("Detected removed dataflow component", zap.String("name", existing.Name))
				s.IgnoreDfcUntilTick[existing.Name] = s.TickCounter + 10
				s.PendingTransitions[existing.Name] = []StateTransition{
					{TickAt: s.TickCounter, State: internalfsm.LifecycleStateRemoving},
					{TickAt: s.TickCounter + 8, State: internalfsm.LifecycleStateRemoved},
				}
			}
		}
		return
	}

	// detect config changes
	if len(curDfcConfig) == len(lastDfcConfig) {
		// checking the config for changes (after edit-dataflowcomponent) is not easy because the name of the component can be changed
		// thus, we first check if the name of the component has changed by iterating over the last config and checking if the name is present in the new config
		renamed := false
		oldName := ""
		newName := ""
		for _, dfcConfig := range lastDfcConfig {
			found := false
			for _, new := range curDfcConfig {
				if new.Name == dfcConfig.Name {
					found = true
					break
				}
			}
			if !found {
				renamed = true
				oldName = dfcConfig.Name
			}
		}
		if renamed {
			// iterate over the new config and check if a name is not present in the last config
			for _, dfcConfig := range curDfcConfig {
				found := false
				for _, existing := range lastDfcConfig {
					if existing.Name == dfcConfig.Name {
						found = true
						break
					}
				}
				if !found {
					newName = dfcConfig.Name
				}
			}
			zap.S().Info("Detected changed dataflow component; old name: ", zap.String("oldName", oldName), zap.String("newName", newName))
			// we need to update the pending transitions and ignore ticks for the old name
			s.PendingTransitions[oldName] = []StateTransition{
				{TickAt: s.TickCounter, State: internalfsm.LifecycleStateRemoving},
				{TickAt: s.TickCounter + 8, State: internalfsm.LifecycleStateRemoved},
			}
			s.IgnoreDfcUntilTick[oldName] = s.TickCounter + 10 // give it some time to remove the component from the state
			// the new component should be added after 10 ticks
			s.PendingTransitions[newName] = []StateTransition{
				{TickAt: s.TickCounter + 10, State: internalfsm.LifecycleStateToBeCreated},
				{TickAt: s.TickCounter + 12, State: internalfsm.LifecycleStateCreating},
				{TickAt: s.TickCounter + 14, State: dataflowcomponent.OperationalStateStarting},
				{TickAt: s.TickCounter + 25, State: dataflowcomponent.OperationalStateActive},
			}
			s.IgnoreDfcUntilTick[newName] = s.TickCounter + 10 // give it some time to create the component
			return
		}
		// if the name is not changed, we need to identify which component has changed
		// we do this by iterating over the last config and checking if the name is present in the new config and then comparing the configs
		for _, dfcConfig := range lastDfcConfig {
			for _, new := range curDfcConfig {
				if new.Name == dfcConfig.Name {
					if !reflect.DeepEqual(new, dfcConfig) {
						zap.S().Info("Detected changed dataflow component", zap.String("name", dfcConfig.Name))
						// we need to update the pending transitions and ignore ticks for the old name
						s.PendingTransitions[dfcConfig.Name] = []StateTransition{
							{TickAt: s.TickCounter, State: dataflowcomponent.EventBenthosDegraded},
							{TickAt: s.TickCounter + 8, State: dataflowcomponent.OperationalStateActive},
						}
						s.IgnoreDfcUntilTick[dfcConfig.Name] = s.TickCounter + 5 // give it some time to apply the new config
					}
				}
			}
		}
	}

}

// checkPendingTransitions checks if there are pending transitions for a component and applies them
// It returns the updated state after applying any transitions that are due
func (s *StateMocker) checkPendingTransitions(componentName string, currentState string) string {
	// Check if there are pending transitions for this component
	if transitions, exists := s.PendingTransitions[componentName]; exists {
		// Find transitions that should be applied at this tick
		for i, transition := range transitions {
			if transition.TickAt <= s.TickCounter {
				currentState = transition.State
				zap.S().Info("Transition applied", zap.String("component", componentName), zap.String("state", currentState))
				// Remove applied transitions if they've been processed
				if i < len(transitions)-1 {
					s.PendingTransitions[componentName] = transitions[i+1:]
				} else {
					// All transitions applied, clear the list
					delete(s.PendingTransitions, componentName)
				}
				break
			}
		}
	}
	return currentState
}

// createDfcManagerSnapshot creates a snapshot of the DataFlowComponent manager state
// it therefore uses the config manager to get the dataflowcomponent configs
// and the pending transitions to update the state of the dataflowcomponent instances based on the tick counter
func (s *StateMocker) createDfcManagerSnapshot(systemSnapshot fsm.SystemSnapshot) fsm.ManagerSnapshot {
	//start with a basic snapshot
	dfcManagerInstaces := map[string]*fsm.FSMInstanceSnapshot{}

	for _, curDataflowcomponent := range s.ConfigManager.GetDataFlowConfig() {
		instanceExistsInCurrentSnapshot := false

		// get the default currentState from the last observed state (s.state)
		currentState := curDataflowcomponent.DesiredFSMState
		var instances map[string]*fsm.FSMInstanceSnapshot

		if systemSnapshot.Managers != nil {
			if manager, ok := systemSnapshot.Managers[constants.DataflowcomponentManagerName]; ok {
				instances = manager.GetInstances()
			}
		}

		if instance, ok := instances[curDataflowcomponent.Name]; ok {
			currentState = instance.CurrentState
			instanceExistsInCurrentSnapshot = true
		}

		// Apply any pending transitions
		currentState = s.checkPendingTransitions(curDataflowcomponent.Name, currentState)

		if ignoreTick, ok := s.IgnoreDfcUntilTick[curDataflowcomponent.Name]; ok && s.TickCounter < ignoreTick {
			// we dont update the config of this component until the tick counter is greater than the ignore tick
			// if exists, we use the instance from the last system snapshot, else we do nothing
			if instanceExistsInCurrentSnapshot && instances[curDataflowcomponent.Name] != nil {
				dfcManagerInstaces[curDataflowcomponent.Name] = instances[curDataflowcomponent.Name]
				dfcManagerInstaces[curDataflowcomponent.Name].CurrentState = currentState
			}
			continue
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

	// now, we need to check if there is an instance in the system snapshot that is not in the config and should be ignored from removing
	if systemSnapshot.Managers != nil {
		if manager, ok := systemSnapshot.Managers[constants.DataflowcomponentManagerName]; ok {
			for _, instance := range manager.GetInstances() {
				// check if the instance is in the config
				found := false
				for _, dfcConfig := range s.ConfigManager.GetDataFlowConfig() {
					if dfcConfig.Name == instance.ID {
						found = true
						break
					}
				}
				if !found {
					// if the instance is not in the config, we need to check if it should be ignored from removing
					if ignoreTick, ok := s.IgnoreDfcUntilTick[instance.ID]; ok {
						if s.TickCounter < ignoreTick {
							dfcManagerInstaces[instance.ID] = instance
							dfcManagerInstaces[instance.ID].CurrentState = s.checkPendingTransitions(instance.ID, instance.CurrentState)
						}
					}
				}
			}
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
