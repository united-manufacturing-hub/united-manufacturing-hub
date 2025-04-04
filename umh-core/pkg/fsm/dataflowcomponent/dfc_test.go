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

//go:build test
// +build test

package dataflowcomponent_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
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
		tick               uint64
		maxAttempts        int
	)

	BeforeEach(func() {
		ctx = context.Background()
		tick = 1
		maxAttempts = 10

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
		// Using the fsmtest helper to wait for the stopped state
		nextTick, waitErr := fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateStopped, maxAttempts, tick)
		Expect(waitErr).NotTo(HaveOccurred())
		Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStopped))
		tick = nextTick
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

			// Use WaitForInstanceState to handle the transition to Starting
			var nextTick uint64
			nextTick, err = fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateStarting, maxAttempts, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStarting))
			tick = nextTick

			// Then wait for transition to Active
			nextTick, err = fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateActive, maxAttempts, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateActive))
			tick = nextTick
		})

		It("should transition to Stopped when the desired state is set to Stopped", func() {
			// Set the desired state to Active and wait for Active state
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			var nextTick uint64
			nextTick, err = fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateActive, maxAttempts, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateActive))
			tick = nextTick

			// Now set the desired state to Stopped
			err = testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			// Wait for transition to Stopping
			nextTick, err = fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateStopping, maxAttempts, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStopping))
			tick = nextTick

			// Then wait for transition to Stopped
			nextTick, err = fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateStopped, maxAttempts, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStopped))
			tick = nextTick
		})
	})

	Describe("Benthos Config Management", func() {
		It("should add the component to the benthos config when starting", func() {
			// Set the desired state to Active
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			// Wait for the starting state which triggers config modification
			var nextTick uint64
			nextTick, err = fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateStarting, maxAttempts, tick)
			Expect(err).NotTo(HaveOccurred())
			tick = nextTick

			// Verify component exists in benthos config
			exists, err := mockAdapter.ComponentExistsInBenthosConfig(ctx, "test-component")
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("should remove the component from the benthos config when stopping", func() {
			// Set the desired state to Active and wait for Active state
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			var nextTick uint64
			nextTick, err = fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateActive, maxAttempts, tick)
			Expect(err).NotTo(HaveOccurred())
			tick = nextTick

			// Now set the desired state to Stopped
			err = testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			// Wait for transition to Stopping which triggers config removal
			nextTick, err = fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateStopping, maxAttempts, tick)
			Expect(err).NotTo(HaveOccurred())
			tick = nextTick

			// Verify component was removed
			exists, err := mockAdapter.ComponentExistsInBenthosConfig(ctx, "test-component")
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("should update the component in the benthos config when already active", func() {
			// Set the desired state to Active and wait for Active state
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			var nextTick uint64
			nextTick, err = fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateActive, maxAttempts, tick)
			Expect(err).NotTo(HaveOccurred())
			tick = nextTick

			// Make a change to the component config
			testComponent.Config.ServiceConfig.Input = map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  "root = \"updated hello world!\"",
					"interval": "2s",
					"count":    0,
				},
			}

			// Reconcile to trigger config update
			err, _ = testComponent.Reconcile(ctx, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++

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
			// Set the initial state to active and wait for Active state
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			var nextTick uint64
			nextTick, err = fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateActive, maxAttempts, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(testComponent.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateActive))
			tick = nextTick

			// Simulate Benthos being in a healthy state with mockAdapter
			mockAdapter.SetComponentState("test-component", benthosfsm.OperationalStateActive, true)

			// Reconcile to update observed state
			err, _ = testComponent.Reconcile(ctx, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++

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
			err, _ = testComponent.Reconcile(ctx, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++

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

			// Set the initial state to active and wait for Active state
			err := testComponent.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)
			Expect(err).NotTo(HaveOccurred())

			var nextTick uint64
			nextTick, err = fsmtest.WaitForInstanceState(ctx, testComponent, dataflowcomponent.OperationalStateActive, maxAttempts, tick)
			Expect(err).NotTo(HaveOccurred())
			tick = nextTick

			// Try to reconcile with GetComponentBenthosObservedState failure
			err, _ = testComponent.Reconcile(ctx, tick)
			// The reconcile should succeed despite the failure to get the observed state
			Expect(err).NotTo(HaveOccurred())
			tick++

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
			err, _ = testComponent.Reconcile(ctx, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++

			// Verify observed state is back
			observedState = testComponent.GetLastObservedState().(dataflowcomponent.DFCObservedState)
			Expect(observedState.BenthosStateMap).To(HaveKey("test-component"))

			// And verify the state is what we set it to
			benthosState := observedState.BenthosStateMap["test-component"]
			Expect(benthosState.ServiceInfo.S6FSMState).To(Equal(benthosfsm.OperationalStateActive))
		})
	})
})
