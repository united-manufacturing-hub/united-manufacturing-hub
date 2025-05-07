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

package actions_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
)

var _ = Describe("StateMocker", func() {
	var (
		stateMocker   *actions.StateMocker
		cfg           *config.FullConfig
		configManager *config.MockConfigManager
		testConfig    dataflowcomponentserviceconfig.DataflowComponentServiceConfig
	)

	BeforeEach(func() {
		cfg = &config.FullConfig{}
		configManager = config.NewMockConfigManager()
		stateMocker = actions.NewStateMocker(configManager)
		testConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
				Input: map[string]interface{}{
					"test": "input",
				},
				Output: map[string]interface{}{
					"test": "output",
				},
			},
		}
	})

	Context("GetState", func() {
		It("should return the current state of the system", func() {
			// Configure the test component with the same name and state that we expect
			componentName := "test-component-build"
			desiredState := "active"

			cfg.DataFlow = []config.DataFlowComponentConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            componentName,
						DesiredFSMState: desiredState,
					},
					DataFlowComponentServiceConfig: testConfig,
				},
			}

			// Update the mock config manager with our test configuration
			configManager.WithConfig(*cfg)

			// Create the expected test instance with matching values
			testInstance := fsm.FSMInstanceSnapshot{
				ID:           componentName,
				DesiredState: desiredState,
				CurrentState: desiredState,
				LastObservedState: &dataflowcomponent.DataflowComponentObservedStateSnapshot{
					Config: testConfig,
				},
			}

			managerSnapshot := &actions.MockManagerSnapshot{
				Instances: map[string]*fsm.FSMInstanceSnapshot{
					componentName: &testInstance,
				},
			}

			stateMocker.Tick()
			state := stateMocker.GetStateManager().GetDeepCopySnapshot()
			testSnapshot := fsm.SystemSnapshot{
				Managers: map[string]fsm.ManagerSnapshot{
					constants.DataflowcomponentManagerName: managerSnapshot,
				},
			}

			Expect(state).To(Equal(testSnapshot))
		})
	})

	Context("StateTransitions", func() {
		It("should simulate a component starting up", func() {
			// Configure the test component
			componentName := "test-component"
			desiredState := "active"
			startingState := "starting"

			cfg.DataFlow = []config.DataFlowComponentConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            componentName,
						DesiredFSMState: desiredState,
					},
					DataFlowComponentServiceConfig: testConfig,
				},
			}

			// Update the mock config manager with our test configuration
			configManager.WithConfig(*cfg)

			// Set up a transition sequence: starting -> active
			stateMocker.SetTransitionSequence(componentName, []struct {
				TickOffset int
				State      string
			}{
				{1, startingState}, // Start in "starting" state
				{3, desiredState},  // After 2 ticks, move to "active" state
			})

			// Initial update
			stateMocker.Tick()

			// First state should be "starting"
			state := stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager := state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component := mockManager.GetInstance(componentName)
			Expect(component.CurrentState).To(Equal(startingState))

			// Advance 1 tick - state should still be "starting"
			stateMocker.Tick()
			state = stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager = state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component = mockManager.GetInstance(componentName)
			Expect(component.CurrentState).To(Equal(startingState))

			// Advance another tick - state should now be "active"
			stateMocker.Tick()
			state = stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager = state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component = mockManager.GetInstance(componentName)
			Expect(component.CurrentState).To(Equal(desiredState))
		})
	})

	Context("ConfigEvents", func() {
		It("should handle adding a new component", func() {
			// Start with no components
			configManager.WithConfig(*cfg)
			stateMocker.Tick()

			// Add a component
			componentName := "new-component"
			cfg.DataFlow = []config.DataFlowComponentConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            componentName,
						DesiredFSMState: "active",
					},
					DataFlowComponentServiceConfig: testConfig,
				},
			}
			configManager.WithConfig(*cfg)

			// Update state and verify component creation process starts
			stateMocker.Tick()
			state := stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager := state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component := mockManager.GetInstance(componentName)
			Expect(component).ToNot(BeNil())
			Expect(component.CurrentState).To(Equal(internalfsm.LifecycleStateToBeCreated))

			// Advance ticks to simulate component creation
			for i := 0; i < 15; i++ {
				stateMocker.Tick()
			}

			// Component should now be active
			state = stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager = state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component = mockManager.GetInstance(componentName)
			Expect(component.CurrentState).To(Equal(dataflowcomponent.OperationalStateActive))
		})

		It("should handle removing a component", func() {
			// Start with one component
			componentName := "existing-component"
			cfg.DataFlow = []config.DataFlowComponentConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            componentName,
						DesiredFSMState: "active",
					},
					DataFlowComponentServiceConfig: testConfig,
				},
			}
			configManager.WithConfig(*cfg)

			// Initialize the state and make component active immediately
			stateMocker.Tick()

			// Verify component exists and is active
			state := stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager := state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component := mockManager.GetInstance(componentName)
			Expect(component).ToNot(BeNil())
			Expect(component.CurrentState).To(Equal(dataflowcomponent.OperationalStateActive))

			// Remove the component from config
			cfg.DataFlow = []config.DataFlowComponentConfig{}
			configManager.WithConfig(*cfg)

			// Update state and verify removal process starts
			stateMocker.Tick()
			state = stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager = state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component = mockManager.GetInstance(componentName)
			Expect(component).ToNot(BeNil())
			Expect(component.CurrentState).To(Equal(internalfsm.LifecycleStateRemoving))

			// Advance ticks to simulate component removal
			for i := 0; i < 8; i++ {
				stateMocker.Tick()
			}

			// Component should be in removed state
			state = stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager = state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component = mockManager.GetInstance(componentName)
			Expect(component.CurrentState).To(Equal(internalfsm.LifecycleStateRemoved))
		})

		It("should handle editing a component's configuration", func() {
			// Start with one component
			componentName := "edit-component"
			cfg.DataFlow = []config.DataFlowComponentConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            componentName,
						DesiredFSMState: "active",
					},
					DataFlowComponentServiceConfig: testConfig,
				},
			}
			configManager.WithConfig(*cfg)

			// Initialize the state and make component active immediately
			stateMocker.Tick()

			// Verify component exists and is active
			state := stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager := state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component := mockManager.GetInstance(componentName)
			Expect(component).ToNot(BeNil())
			Expect(component.CurrentState).To(Equal(dataflowcomponent.OperationalStateActive))

			// Edit the component configuration
			updatedConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
				BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
					Input: map[string]interface{}{
						"test": "updated-input",
					},
					Output: map[string]interface{}{
						"test": "updated-output",
					},
				},
			}

			cfg.DataFlow = []config.DataFlowComponentConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            componentName,
						DesiredFSMState: "active",
					},
					DataFlowComponentServiceConfig: updatedConfig,
				},
			}
			configManager.WithConfig(*cfg)

			// Update state and verify component goes through reconfiguration
			stateMocker.Tick()
			state = stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager = state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component = mockManager.GetInstance(componentName)
			Expect(component).ToNot(BeNil())
			Expect(component.CurrentState).To(Equal(dataflowcomponent.EventBenthosDegraded))

			// Advance ticks to simulate component reconfiguration
			for i := 0; i < 8; i++ {
				stateMocker.Tick()
			}

			// Component should be active again with updated config
			state = stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager = state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component = mockManager.GetInstance(componentName)
			Expect(component.CurrentState).To(Equal(dataflowcomponent.OperationalStateActive))

			// Get the observed state and verify it has the updated config
			observedState, ok := component.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
			Expect(ok).To(BeTrue())
			Expect(observedState.Config).To(Equal(updatedConfig))
		})

		It("should handle editing a component's configuration with changed component name", func() {
			// Start with one component
			oldComponentName := "old-component-name"
			cfg.DataFlow = []config.DataFlowComponentConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            oldComponentName,
						DesiredFSMState: "active",
					},
					DataFlowComponentServiceConfig: testConfig,
				},
			}
			configManager.WithConfig(*cfg)

			// Initialize the state and make component active immediately
			stateMocker.Tick()

			// Verify component exists and is active
			state := stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager := state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			component := mockManager.GetInstance(oldComponentName)
			Expect(component).ToNot(BeNil())
			Expect(component.CurrentState).To(Equal(dataflowcomponent.OperationalStateActive))

			// Edit the component with a new name
			newComponentName := "new-component-name"
			cfg.DataFlow = []config.DataFlowComponentConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            newComponentName,
						DesiredFSMState: "active",
					},
					DataFlowComponentServiceConfig: testConfig,
				},
			}
			configManager.WithConfig(*cfg)

			// Update state and verify old component starts removal process
			stateMocker.Tick()
			state = stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager = state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			oldComponent := mockManager.GetInstance(oldComponentName)
			Expect(oldComponent).ToNot(BeNil())
			Expect(oldComponent.CurrentState).To(Equal(internalfsm.LifecycleStateRemoving))

			// Advance ticks to let old component reach removed state
			for i := 0; i < 8; i++ {
				stateMocker.Tick()
			}

			// Verify old component is now in removed state
			state = stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager = state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			oldComponent = mockManager.GetInstance(oldComponentName)
			Expect(oldComponent).ToNot(BeNil())
			Expect(oldComponent.CurrentState).To(Equal(internalfsm.LifecycleStateRemoved))

			// Advance ticks to let new component appear and initialize
			for i := 0; i < 5; i++ {
				stateMocker.Tick()
			}

			// Verify new component has appeared and is in creating state
			state = stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager = state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			newComponent := mockManager.GetInstance(newComponentName)
			Expect(newComponent).ToNot(BeNil())
			Expect(newComponent.CurrentState).To(Equal(internalfsm.LifecycleStateCreating))

			// Advance ticks to let new component reach active state
			for i := 0; i < 15; i++ {
				stateMocker.Tick()
			}

			// Verify old component has disappeared
			state = stateMocker.GetStateManager().GetDeepCopySnapshot()
			mockManager = state.Managers[constants.DataflowcomponentManagerName].(*actions.MockManagerSnapshot)
			oldComponent = mockManager.GetInstance(oldComponentName)
			Expect(oldComponent).To(BeNil())

			// Verify new component is active with expected config
			newComponent = mockManager.GetInstance(newComponentName)
			Expect(newComponent).ToNot(BeNil())
			Expect(newComponent.CurrentState).To(Equal(dataflowcomponent.OperationalStateActive))

			observedState, ok := newComponent.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
			Expect(ok).To(BeTrue())
			Expect(observedState.Config).To(Equal(testConfig))
		})
	})
})
