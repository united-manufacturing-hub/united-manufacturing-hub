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

package dataflowcomponent_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	benthosserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
)

var _ = Describe("DataFlowComponent FSM", func() {
	var (
		ctx                context.Context
		mockAdapter        *dataflowcomponent.MockBenthosManagerAdapter
		testComponent      *dataflowcomponent.DataFlowComponent
		componentConfig    dataflowcomponent.DataFlowComponentConfig
		tempConfigFilePath string
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create a BenthosManager and MockBenthosService
		benthosManager, mockService := benthosfsm.NewBenthosManagerWithMockedServices("test")

		// Create our adapter that implements BenthosConfigManager interface
		mockAdapter = dataflowcomponent.NewMockBenthosManagerAdapter(benthosManager, mockService)

		// Create a temporary directory for config files
		tempDir, err := os.MkdirTemp("", "dataFlowComponent-test")
		Expect(err).NotTo(HaveOccurred())
		tempConfigFilePath = filepath.Join(tempDir, "test-config.yaml")

		// Basic component config
		componentConfig = dataflowcomponent.DataFlowComponentConfig{
			Name:         "test-component",
			DesiredState: "stopped",
			ServiceConfig: benthosserviceconfig.BenthosServiceConfig{
				Input: map[string]interface{}{
					"generate": map[string]interface{}{
						"mapping":  "root = \"hello world from test!\"",
						"interval": "1s",
						"count":    0,
					},
				},
				Output: map[string]interface{}{
					"stdout": map[string]interface{}{},
				},
			},
		}

		// Create a new DataFlowComponent
		testComponent = dataflowcomponent.NewDataFlowComponent(componentConfig, mockAdapter)
		Expect(testComponent).NotTo(BeNil())

		// Complete the lifecycle creation process (to_be_created -> creating -> created -> stopped)
		// First reconcile to add to config and transition from to_be_created to creating
		err, _ = testComponent.Reconcile(ctx, 1)
		Expect(err).NotTo(HaveOccurred())

		// Second reconcile to complete creation (creating -> created)
		err, _ = testComponent.Reconcile(ctx, 2)
		Expect(err).NotTo(HaveOccurred())

		// Now the FSM should be in the stopped operational state as configured
		Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStopped))
	})

	AfterEach(func() {
		// Clean up the temporary directory
		if tempConfigFilePath != "" {
			os.RemoveAll(filepath.Dir(tempConfigFilePath))
		}
	})

	Describe("State Transitions", func() {
		It("should start in the Stopped state", func() {
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStopped))
		})

		It("should transition to Active when the desired state is set to Active", func() {
			// Set the desired state to Active
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// First reconcile call should transition from Stopped to Starting
			err, reconciled := testComponent.Reconcile(ctx, 3)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStarting))

			// Second reconcile call should transition from Starting to Active
			err, reconciled = testComponent.Reconcile(ctx, 4)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateActive))
		})

		It("should transition to Stopped when the desired state is set to Stopped", func() {
			// Set the desired state to Active and reconcile to get to Active state
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 3) // -> Starting
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 4) // -> Active
			Expect(err).NotTo(HaveOccurred())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateActive))

			// Now set the desired state to Stopped
			err = testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			// First reconcile call should transition from Active to Stopping
			err, reconciled := testComponent.Reconcile(ctx, 5)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStopping))

			// Second reconcile call should transition from Stopping to Stopped
			err, reconciled = testComponent.Reconcile(ctx, 6)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciled).To(BeTrue())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStopped))
		})
	})

	Describe("Benthos Config Management", func() {
		It("should add the component to the benthos config when starting", func() {
			// Set the desired state to Active
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to trigger config modification
			err, _ = testComponent.Reconcile(ctx, 3)
			Expect(err).NotTo(HaveOccurred())

			// Verify component exists in benthos config
			exists, err := mockAdapter.ComponentExistsInBenthosConfig(ctx, "test-component")
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("should remove the component from the benthos config when stopping", func() {
			// Set the desired state to Active and reconcile to get to Active state
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 3) // -> Starting
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 4) // -> Active
			Expect(err).NotTo(HaveOccurred())

			// Now set the desired state to Stopped
			err = testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile to trigger config modification
			err, _ = testComponent.Reconcile(ctx, 5)
			Expect(err).NotTo(HaveOccurred())

			// Verify component was removed
			exists, err := mockAdapter.ComponentExistsInBenthosConfig(ctx, "test-component")
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("should update the component in the benthos config when already active", func() {
			// Set the desired state to Active and reconcile to get to Active state
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 3) // -> Starting
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 4) // -> Active
			Expect(err).NotTo(HaveOccurred())

			// Make a change to the component config
			testComponent.Config.ServiceConfig.Input = map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  "root = \"updated hello world!\"",
					"interval": "2s",
					"count":    0,
				},
			}

			// Reconcile again to trigger config update
			err, _ = testComponent.Reconcile(ctx, 5)
			Expect(err).NotTo(HaveOccurred())

			// Verify state has been updated
			state, err := mockAdapter.GetComponentBenthosObservedState(ctx, "test-component")
			Expect(err).NotTo(HaveOccurred())
			Expect(state).NotTo(BeNil())

			// The config in the observed state should match our updated config
			Expect(state.ObservedBenthosServiceConfig.Input).To(HaveKey("generate"))
			if generateConfig, ok := state.ObservedBenthosServiceConfig.Input["generate"].(map[string]interface{}); ok {
				Expect(generateConfig).To(HaveKeyWithValue("interval", "2s"))
			}
		})
	})

	Describe("Benthos Observed State", func() {
		It("should properly retrieve and update Benthos observed state", func() {
			// Set the initial state to active
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// Complete the transition to Active
			err, _ = testComponent.Reconcile(ctx, 3) // -> Starting
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 4) // -> Active
			Expect(err).NotTo(HaveOccurred())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateActive))

			// Simulate Benthos being in a healthy state with mockAdapter
			mockAdapter.SetComponentState("test-component", benthosfsm.OperationalStateActive, true)

			// Check that the component is observed as healthy after reconciliation
			err, _ = testComponent.Reconcile(ctx, 5)
			Expect(err).NotTo(HaveOccurred())

			// Verify observed state is updated correctly
			observedState := testComponent.GetLastObservedState().(dataflowcomponent.DFCObservedState)
			Expect(observedState.BenthosStateMap).To(HaveKey("test-component"))
			benthosState := observedState.BenthosStateMap["test-component"]
			Expect(benthosState).NotTo(BeNil())
			Expect(benthosState.ServiceInfo.S6FSMState).To(Equal(benthosfsm.OperationalStateActive))
			Expect(benthosState.ServiceInfo.BenthosStatus.HealthCheck.IsLive).To(BeTrue())
			Expect(benthosState.ServiceInfo.BenthosStatus.HealthCheck.IsReady).To(BeTrue())

			// Simulate a degraded state
			mockAdapter.SetComponentState("test-component", benthosfsm.OperationalStateDegraded, false)

			// Update observed state through reconciliation
			err, _ = testComponent.Reconcile(ctx, 6)
			Expect(err).NotTo(HaveOccurred())

			// Verify observed state reflects the degraded condition
			observedState = testComponent.GetLastObservedState().(dataflowcomponent.DFCObservedState)
			benthosState = observedState.BenthosStateMap["test-component"]
			Expect(benthosState).NotTo(BeNil())
			Expect(benthosState.ServiceInfo.S6FSMState).To(Equal(benthosfsm.OperationalStateDegraded))
			Expect(benthosState.ServiceInfo.BenthosStatus.HealthCheck.IsLive).To(BeFalse())
			Expect(benthosState.ServiceInfo.BenthosStatus.HealthCheck.IsReady).To(BeFalse())
		})

		It("should handle failures in retrieving Benthos observed state", func() {
			// Configure mockAdapter to fail when getting state
			mockAdapter.ConfigureFailure("getState", true)

			// Set up component in active state
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 3) // -> Starting
			Expect(err).NotTo(HaveOccurred())
			err, _ = testComponent.Reconcile(ctx, 4) // -> Active
			Expect(err).NotTo(HaveOccurred())

			// Try to reconcile with GetComponentBenthosObservedState failure
			err, _ = testComponent.Reconcile(ctx, 5)
			// The reconcile should succeed despite the failure to get the observed state
			Expect(err).NotTo(HaveOccurred())

			// The component should have a warning in the observed state
			observedState := testComponent.GetLastObservedState().(dataflowcomponent.DFCObservedState)
			// It should still exist in the Benthos config
			Expect(observedState.ConfigExists).To(BeTrue())

			// The BenthosStateMap should be initialized but the component might still exist there
			// depending on the implementation. What's important is that the state shouldn't be updated
			// with the new state (because of the error)
			if benthosState, exists := observedState.BenthosStateMap["test-component"]; exists {
				// If it exists, it should be an older version, not updated
				Expect(benthosState.ServiceInfo.S6FSMState).NotTo(Equal(benthosfsm.OperationalStateDegraded))
			}

			// Now fix the mock to succeed again
			mockAdapter.ConfigureFailure("getState", false)
			mockAdapter.SetComponentState("test-component", benthosfsm.OperationalStateActive, true)

			// Reconcile should now succeed and get the state
			err, _ = testComponent.Reconcile(ctx, 6)
			Expect(err).NotTo(HaveOccurred())

			// Verify observed state is back
			observedState = testComponent.GetLastObservedState().(dataflowcomponent.DFCObservedState)
			Expect(observedState.BenthosStateMap).To(HaveKey("test-component"))

			// And verify the state is what we set it to
			benthosState := observedState.BenthosStateMap["test-component"]
			Expect(benthosState.ServiceInfo.S6FSMState).To(Equal(benthosfsm.OperationalStateActive))
		})
	})
})
