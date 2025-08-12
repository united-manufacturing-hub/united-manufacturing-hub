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

	"github.com/tiendc/go-deepcopy"
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsmmanager "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	dfcservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"go.uber.org/zap"
)

// ErrAlreadyRunning is returned when trying to start a StateMocker that is already running.
var ErrAlreadyRunning = errors.New("state mocker already running")

// ConfigManager interface defines the methods needed by StateMocker to get configuration data.
type ConfigManager interface {
	// GetDataFlowConfig returns the DataFlow component configurations
	GetDataFlowConfig() []config.DataFlowComponentConfig
}

// StateTransition represents a state transition for a component.
type StateTransition struct {
	State  string
	TickAt int
}

// the state mocker is used for unit testing to mock the state of the system
// it runs in a separate goroutine and regularly updates the state of the system
// it is passed a config manager that provides the config being used by the action unit tests
// it then updates the state of the system according to the config just like the real system would do.
type StateMocker struct {
	ConfigManager      ConfigManager
	StateManager       *fsm.SnapshotManager
	PendingTransitions map[string][]StateTransition // key is component ID
	done               chan struct{}                // channel to signal shutdown of goroutine
	mu                 *sync.RWMutex                // mutex to protect the state of the mocker
	IgnoreDfcUntilTick map[string]int               // map of component ID to tick to ignore
	lastConfig         config.FullConfig            // last config is needed to detect config changes (events)
	TickCounter        int
	running            atomic.Bool // flag to track if the mocker is running
	lastConfigSet      bool        // flag to track if the last config has been set
}

// NewStateMocker creates a new StateMocker.
func NewStateMocker(configManager ConfigManager) *StateMocker {
	return &StateMocker{
		ConfigManager:      configManager,
		StateManager:       fsm.NewSnapshotManager(),
		TickCounter:        0,
		PendingTransitions: make(map[string][]StateTransition),
		done:               make(chan struct{}),
		mu:                 &sync.RWMutex{},
		lastConfig:         config.FullConfig{},
		lastConfigSet:      false,
		IgnoreDfcUntilTick: make(map[string]int),
	}
}

func (s *StateMocker) GetStateManager() *fsm.SnapshotManager {
	return s.StateManager
}

// UpdateDfcState updates the state of DataFlow components based on configuration changes.
// It detects config events (additions, removals, edits), schedules appropriate state transitions,
// and updates the system snapshot with the new manager state. This function is the core of the
// state mocking system, simulating how real components would transition between states.
func (s *StateMocker) Tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// only take the dataflowcomponent configs and add them to the state of the dataflowcomponent manager
	s.TickCounter++

	// get the current and the last config and the system snapshot
	curDfcConfig := s.ConfigManager.GetDataFlowConfig()
	lastDfcConfig := s.lastConfig.DataFlow
	systemSnapshot := s.StateManager.GetDeepCopySnapshot()

	// detect config events (add, remove, edit) and add the corresponding state transitions to the pending transitions
	updatedPendingTransitions, updatedIgnoreDfcUntilTick := detectConfigEvents(curDfcConfig, lastDfcConfig, s.lastConfigSet, s.PendingTransitions, s.IgnoreDfcUntilTick, s.TickCounter)
	// Update local state with the changes from the function
	s.PendingTransitions = updatedPendingTransitions
	s.IgnoreDfcUntilTick = updatedIgnoreDfcUntilTick

	// ⚠️ Do NOT assign the slice directly! In Go, `dst = srcSlice` only copies the
	// slice *header* (len, cap, data-ptr). Both variables would then share the same
	// backing array, so any in-place mutation (like changing DesiredFSMState during
	// a test) would update *both* curDfcConfig and lastConfig.DataFlow, and our
	// "desired-state changed" check would never fire. We therefore deep-copy the
	// slice (and its structs) to get an immutable snapshot for this tick.
	s.lastConfig.DataFlow = copyDfcSlice(curDfcConfig) // update the last config
	s.lastConfigSet = true

	// create a new manager snapshot based on the current system snapshot, the config and the pending transitions
	managerSnapshot, updatedPendingTransitions, updatedIgnoreDfcUntilTick := createDfcManagerSnapshot(
		systemSnapshot,
		s.ConfigManager,
		s.IgnoreDfcUntilTick,
		s.TickCounter,
		s.PendingTransitions,
	)

	// Update local state with the changes from the function
	s.PendingTransitions = updatedPendingTransitions
	s.IgnoreDfcUntilTick = updatedIgnoreDfcUntilTick

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

// detect config events (add, remove, edit) and add the corresponding state transitions to the pending transitions.
func detectConfigEvents(curDfcConfig []config.DataFlowComponentConfig, lastDfcConfig []config.DataFlowComponentConfig, lastConfigSet bool, pendingTransitions map[string][]StateTransition, ignoreDfcUntilTick map[string]int, tickCounter int) (map[string][]StateTransition, map[string]int) {
	// in the first run, we dont have a last config, so we dont need to detect any config events
	// after the call of detectConfigEvents, the last config is set
	if !lastConfigSet {
		return pendingTransitions, ignoreDfcUntilTick
	}

	// detect added dfcs
	if len(curDfcConfig) > len(lastDfcConfig) {
		return detectAddedComponents(curDfcConfig, lastDfcConfig, pendingTransitions, ignoreDfcUntilTick, tickCounter)
	}

	// detect removed dfcs
	if len(curDfcConfig) < len(lastDfcConfig) {
		return detectRemovedComponents(curDfcConfig, lastDfcConfig, pendingTransitions, ignoreDfcUntilTick, tickCounter)
	}

	// detect config changes
	if len(curDfcConfig) == len(lastDfcConfig) {
		return detectEditedComponents(curDfcConfig, lastDfcConfig, pendingTransitions, ignoreDfcUntilTick, tickCounter)
	}

	return pendingTransitions, ignoreDfcUntilTick
}

// detectAddedComponents detects added components by comparing current and last config.
func detectAddedComponents(curDfcConfig []config.DataFlowComponentConfig, lastDfcConfig []config.DataFlowComponentConfig, pendingTransitions map[string][]StateTransition, ignoreDfcUntilTick map[string]int, tickCounter int) (map[string][]StateTransition, map[string]int) {
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
			// depending on the desired state, we apply different transitions
			switch dfcConfig.DesiredFSMState {
			case "active":
				pendingTransitions[dfcConfig.Name] = []StateTransition{
					{TickAt: tickCounter, State: internalfsm.LifecycleStateToBeCreated},
					{TickAt: tickCounter + 2, State: internalfsm.LifecycleStateCreating},
					{TickAt: tickCounter + 4, State: dataflowcomponent.OperationalStateStarting},
					{TickAt: tickCounter + 15, State: dataflowcomponent.OperationalStateActive},
				}
			case "stopped":
				pendingTransitions[dfcConfig.Name] = []StateTransition{
					{TickAt: tickCounter, State: internalfsm.LifecycleStateToBeCreated},
					{TickAt: tickCounter + 2, State: internalfsm.LifecycleStateCreating},
					{TickAt: tickCounter + 10, State: dataflowcomponent.OperationalStateStopped},
				}
			}
		}
	}

	return pendingTransitions, ignoreDfcUntilTick
}

// detectRemovedComponents detects removed components by comparing current and last config.
func detectRemovedComponents(curDfcConfig []config.DataFlowComponentConfig, lastDfcConfig []config.DataFlowComponentConfig, pendingTransitions map[string][]StateTransition, ignoreDfcUntilTick map[string]int, tickCounter int) (map[string][]StateTransition, map[string]int) {
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
			ignoreDfcUntilTick[existing.Name] = tickCounter + 10
			pendingTransitions[existing.Name] = []StateTransition{
				{TickAt: tickCounter, State: internalfsm.LifecycleStateRemoving},
				{TickAt: tickCounter + 8, State: internalfsm.LifecycleStateRemoved},
			}
		}
	}

	return pendingTransitions, ignoreDfcUntilTick
}

// detectEditedComponents detects edited components by comparing current and last config.
func detectEditedComponents(curDfcConfig []config.DataFlowComponentConfig, lastDfcConfig []config.DataFlowComponentConfig, pendingTransitions map[string][]StateTransition, ignoreDfcUntilTick map[string]int, tickCounter int) (map[string][]StateTransition, map[string]int) {
	// checking the config for changes (after edit-dataflowcomponent) is not easy because the name of the component can be changed
	// thus, we first check if the name of the component has changed by iterating over the last config and checking if the name is present in the new config
	renamed := false
	oldName := ""
	newName := ""
	desiredState := ""

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
				desiredState = dfcConfig.DesiredFSMState
			}
		}

		zap.S().Info("Detected changed dataflow component; old name: ", zap.String("oldName", oldName), zap.String("newName", newName))
		// we need to update the pending transitions and ignore ticks for the old name
		pendingTransitions[oldName] = []StateTransition{
			{TickAt: tickCounter, State: internalfsm.LifecycleStateRemoving},
			{TickAt: tickCounter + 8, State: internalfsm.LifecycleStateRemoved},
		}
		ignoreDfcUntilTick[oldName] = tickCounter + 10 // give it some time to remove the component from the state
		// the new component should be added after 10 ticks
		switch desiredState { // depending on the desired state, we apply different transitions
		case "active":
			pendingTransitions[newName] = []StateTransition{
				{TickAt: tickCounter + 10, State: internalfsm.LifecycleStateToBeCreated},
				{TickAt: tickCounter + 12, State: internalfsm.LifecycleStateCreating},
				{TickAt: tickCounter + 14, State: dataflowcomponent.OperationalStateStarting},
				{TickAt: tickCounter + 25, State: dataflowcomponent.OperationalStateActive},
			}
		case "stopped":
			pendingTransitions[newName] = []StateTransition{
				{TickAt: tickCounter + 10, State: internalfsm.LifecycleStateToBeCreated},
				{TickAt: tickCounter + 12, State: internalfsm.LifecycleStateCreating},
				{TickAt: tickCounter + 20, State: dataflowcomponent.OperationalStateStopped},
			}
		}

		ignoreDfcUntilTick[newName] = tickCounter + 10 // give it some time to create the component

		return pendingTransitions, ignoreDfcUntilTick
	}
	// if the name is not changed, we need to identify which component has changed
	// we do this by iterating over the last config and checking if the name is present in the new config and then comparing the configs
	for _, dfcConfig := range lastDfcConfig {
		for _, new := range curDfcConfig {
			if new.Name == dfcConfig.Name {
				if !reflect.DeepEqual(new, dfcConfig) {
					zap.S().Info("Detected changed dataflow component", zap.String("name", dfcConfig.Name))
					// we need to update the pending transitions and ignore ticks for the old name
					switch new.DesiredFSMState {
					case "active":
						pendingTransitions[dfcConfig.Name] = []StateTransition{
							{TickAt: tickCounter, State: dataflowcomponent.EventBenthosDegraded},
							{TickAt: tickCounter + 8, State: dataflowcomponent.OperationalStateActive},
						}
					case "stopped":
						pendingTransitions[dfcConfig.Name] = []StateTransition{
							{TickAt: tickCounter, State: dataflowcomponent.EventBenthosDegraded},
							{TickAt: tickCounter + 8, State: dataflowcomponent.OperationalStateStopped},
						}
					}

					ignoreDfcUntilTick[dfcConfig.Name] = tickCounter + 5 // give it some time to apply the new config
				}
			}
		}
	}

	return pendingTransitions, ignoreDfcUntilTick
}

// checkPendingTransitions checks if there are pending transitions for a component and applies them
// It returns the updated state after applying any transitions that are due, along with the updated transitions map.
func checkPendingTransitions(componentName string, currentState string, pendingTransitions map[string][]StateTransition, tickCounter int) (string, map[string][]StateTransition) {
	// Check if there are pending transitions for this component
	if transitions, exists := pendingTransitions[componentName]; exists {
		// Find transitions that should be applied at this tick
		for i, transition := range transitions {
			if transition.TickAt <= tickCounter {
				currentState = transition.State
				zap.S().Info("Transition applied", zap.String("component", componentName), zap.String("state", currentState))
				// Remove applied transitions if they've been processed
				if i < len(transitions)-1 {
					pendingTransitions[componentName] = transitions[i+1:]
				} else {
					// All transitions applied, clear the list
					delete(pendingTransitions, componentName)
				}

				break
			}
		}
	}

	return currentState, pendingTransitions
}

// createDfcManagerSnapshot creates a snapshot of the DataFlowComponent manager state
// it therefore uses the config manager to get the dataflowcomponent configs
// and the pending transitions to update the state of the dataflowcomponent instances based on the tick counter.
func createDfcManagerSnapshot(
	systemSnapshot fsm.SystemSnapshot,
	configManager ConfigManager,
	ignoreDfcUntilTick map[string]int,
	tickCounter int,
	pendingTransitions map[string][]StateTransition,
) (fsm.ManagerSnapshot, map[string][]StateTransition, map[string]int) {
	// start with a basic snapshot
	dfcManagerInstaces := map[string]*fsm.FSMInstanceSnapshot{}

	for _, curDataflowcomponent := range configManager.GetDataFlowConfig() {
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
		var updatedPendingTransitions map[string][]StateTransition

		currentState, updatedPendingTransitions = checkPendingTransitions(curDataflowcomponent.Name, currentState, pendingTransitions, tickCounter)
		pendingTransitions = updatedPendingTransitions

		if ignoreTick, ok := ignoreDfcUntilTick[curDataflowcomponent.Name]; ok && tickCounter < ignoreTick {
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
				ServiceInfo: dfcservice.ServiceInfo{
					BenthosObservedState: benthosfsmmanager.BenthosObservedState{
						ObservedBenthosServiceConfig: benthosserviceconfig.BenthosServiceConfig{
							Input:              curDataflowcomponent.DataFlowComponentServiceConfig.BenthosConfig.Input,
							Pipeline:           curDataflowcomponent.DataFlowComponentServiceConfig.BenthosConfig.Pipeline,
							Output:             curDataflowcomponent.DataFlowComponentServiceConfig.BenthosConfig.Output,
							CacheResources:     curDataflowcomponent.DataFlowComponentServiceConfig.BenthosConfig.CacheResources,
							RateLimitResources: curDataflowcomponent.DataFlowComponentServiceConfig.BenthosConfig.RateLimitResources,
							Buffer:             curDataflowcomponent.DataFlowComponentServiceConfig.BenthosConfig.Buffer,
						},
					},
				},
			},
		}
	}

	// now, we need to check if there is an instance in the system snapshot that is not in the config and should be ignored from removing
	if systemSnapshot.Managers != nil {
		if manager, ok := systemSnapshot.Managers[constants.DataflowcomponentManagerName]; ok {
			for _, instance := range manager.GetInstances() {
				// check if the instance is in the config
				found := false

				for _, dfcConfig := range configManager.GetDataFlowConfig() {
					if dfcConfig.Name == instance.ID {
						found = true

						break
					}
				}

				if !found {
					// if the instance is not in the config, we need to check if it should be ignored from removing
					if ignoreTick, ok := ignoreDfcUntilTick[instance.ID]; ok {
						if tickCounter < ignoreTick {
							dfcManagerInstaces[instance.ID] = instance

							// Apply any pending transitions
							var updatedPendingTransitions map[string][]StateTransition

							newState, updatedPendingTransitions := checkPendingTransitions(instance.ID, instance.CurrentState, pendingTransitions, tickCounter)
							pendingTransitions = updatedPendingTransitions
							dfcManagerInstaces[instance.ID].CurrentState = newState
						}
					}
				}
			}
		}
	}

	return &MockManagerSnapshot{
		Instances: dfcManagerInstaces,
	}, pendingTransitions, ignoreDfcUntilTick
}

// Run starts the state mocker and updates the state of the system periodically
// It returns an error if already running.
func (s *StateMocker) Run() error {
	if s.running.Swap(true) {
		return ErrAlreadyRunning
	}

	ticker := time.NewTicker(constants.DefaultTickerTime)

	// Get a reference to the done channel under mutex protection
	s.mu.RLock()
	done := s.done
	s.mu.RUnlock()

	go func() {
		defer ticker.Stop()
		defer s.running.Store(false)

		for {
			select {
			case <-ticker.C:
				s.Tick()
			case <-done:
				return
			}
		}
	}()

	return nil
}

// Start is a convenience method that runs the state mocker in a new goroutine
// It returns an error if already running.
func (s *StateMocker) Start() error {
	err := s.Run()
	if err != nil {
		return err
	}

	return nil
}

// Stop signals the state mocker to stop updating and waits for it to complete
// It is safe to call Stop even if the mocker isn't running.
func (s *StateMocker) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running.Load() {
		close(s.done)
		// Create a new channel for next run
		s.done = make(chan struct{})
	}
}

// SetTransitionSequence schedules state transitions for a component
// Each transition is defined by (tickOffset, state) where tickOffset is relative to current tick.
func (s *StateMocker) SetTransitionSequence(componentID string, transitions []struct {
	State      string
	TickOffset int
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

// CreateMockSystemSnapshotWithMissingState is a helper function to create a system snapshot with a component that has no observed state.
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

// MockManagerSnapshot is a simple implementation of ManagerSnapshot interface for testing.
type MockManagerSnapshot struct {
	Instances map[string]*fsm.FSMInstanceSnapshot
}

func (m *MockManagerSnapshot) GetName() string {
	return constants.DataflowcomponentManagerName
}

func (m *MockManagerSnapshot) GetInstances() map[string]*fsm.FSMInstanceSnapshot {
	return m.Instances
}

// GetInstance returns an FSM instance by ID.
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

// MockObservedState is a fake implementation of ObservedStateSnapshot for testing.
type MockObservedState struct{}

func (m *MockObservedState) IsObservedStateSnapshot() {}

func copyDfcSlice(src []config.DataFlowComponentConfig) []config.DataFlowComponentConfig {
	if src == nil {
		return nil
	}

	var dst []config.DataFlowComponentConfig

	err := deepcopy.Copy(&dst, &src)
	if err != nil {
		// This should never happen
		panic(err)
	}

	return dst
}
