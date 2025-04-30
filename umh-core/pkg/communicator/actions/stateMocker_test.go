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

			stateMocker.UpdateDfcState()
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
				{0, startingState}, // Start in "starting" state
				{2, desiredState},  // After 2 ticks, move to "active" state
			})

			// Initial update
			stateMocker.UpdateDfcState()

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
})
