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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// Adjust these imports to match your project structure

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	benthosserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
)

var _ = Describe("DataFlowComponent FSM", func() {
	var (
		instance      *dataflowcomponent.DataFlowComponent
		mockAdapter   *dataflowcomponent.MockBenthosManagerAdapter
		componentName string
		ctx           context.Context
		tick          uint64
	)

	BeforeEach(func() {
		ctx = context.Background()
		tick = 0

		// Setup a default instance with a desired state of "stopped" initially
		componentName = "test-dfc"

		// Create a BenthosManager and MockBenthosService
		benthosManager, mockService := benthosfsm.NewBenthosManagerWithMockedServices("test")

		// Create our adapter that implements BenthosConfigManager interface
		mockAdapter = dataflowcomponent.NewMockBenthosManagerAdapter(benthosManager, mockService)

		// Basic component config
		componentConfig := dataflowcomponent.DataFlowComponentConfig{
			Name:         componentName,
			DesiredState: dataflowcomponent.OperationalStateStopped,
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
		instance = dataflowcomponent.NewDataFlowComponent(componentConfig, mockAdapter)
	})

	// -------------------------------------------------------------------------
	//  BASIC STATE TRANSITIONS
	// -------------------------------------------------------------------------
	Context("Basic State Transitions", func() {
		It("should transition from Stopped to Starting when activated", func() {
			var err error

			// 1. First, we need to get from to_be_created to stopped
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStopped,
				5, // attempts
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 2. Now set desired state = Active => from "stopped" => "starting"
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())
			Expect(instance.GetDesiredFSMState()).To(Equal(dataflowcomponent.OperationalStateActive))

			// 3. Check transition to "starting"
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 4. Verify AddComponentToBenthosConfig was called
			exists, err := mockAdapter.ComponentExistsInBenthosConfig(ctx, componentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("should transition from Starting to Active", func() {
			var err error

			// 1. First, we need to get from to_be_created to stopped
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStopped,
				5, // attempts
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 2. Set desired state to Active
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// 3. Wait for transition to "starting"
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 4. Set up benthos state to be active
			mockAdapter.SetComponentState(componentName, benthosfsm.OperationalStateActive, true)

			// 5. Wait for transition to "active"
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateActive))
		})
	})

	// -------------------------------------------------------------------------
	//  DEGRADED STATE
	// -------------------------------------------------------------------------
	Context("Degraded State", func() {
		It("should transition to Degraded when issues occur", func() {
			var err error

			// 1. First, get to active state
			// Get to stopped
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set desired state to active
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Get to starting
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set healthy state for transition to active
			mockAdapter.SetComponentState(componentName, benthosfsm.OperationalStateActive, true)

			// Get to active
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 2. Now simulate degradation - set unhealthy state
			mockAdapter.SetComponentState(componentName, benthosfsm.OperationalStateDegraded, false)

			// Call reconcile directly to simulate the FSM observing the degraded state
			err, _ = instance.Reconcile(ctx, tick)
			Expect(err).NotTo(HaveOccurred())
			tick++

			// Send degraded event
			// Note: since we don't have direct access to the FSM's SendEvent, we'll need
			// to use multiple reconcile calls
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateDegraded,
				10, // More attempts needed for this transition
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateDegraded))
		})

		It("should recover from Degraded to Active when issues are resolved", func() {
			var err error

			// 1. First, get to degraded state
			// Steps: to_be_created -> stopped -> starting -> active -> degraded
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			mockAdapter.SetComponentState(componentName, benthosfsm.OperationalStateActive, true)

			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Degrade the instance
			mockAdapter.SetComponentState(componentName, benthosfsm.OperationalStateDegraded, false)

			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateDegraded,
				10,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 2. Now recover - set healthy state
			mockAdapter.SetComponentState(componentName, benthosfsm.OperationalStateActive, true)

			// Wait for transition back to active
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateActive,
				10,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateActive))
		})
	})

	// -------------------------------------------------------------------------
	//  STOPPING FLOW
	// -------------------------------------------------------------------------
	Context("Stopping Flow", func() {
		It("should stop gracefully from Active state", func() {
			var err error

			// 1. Set up active state
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			mockAdapter.SetComponentState(componentName, benthosfsm.OperationalStateActive, true)

			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 2. Now request to stop
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateStopped)).To(Succeed())

			// 3. Check transition to stopping
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStopping,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 4. Then confirm transition to stopped
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 5. Verify component was removed from benthos config
			exists, err := mockAdapter.ComponentExistsInBenthosConfig(ctx, componentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("should stop gracefully from Degraded state", func() {
			var err error

			// 1. First, get to degraded state
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			mockAdapter.SetComponentState(componentName, benthosfsm.OperationalStateActive, true)

			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Degrade
			mockAdapter.SetComponentState(componentName, benthosfsm.OperationalStateDegraded, false)

			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateDegraded,
				10,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 2. Now request to stop
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateStopped)).To(Succeed())

			// 3. Check transition to stopping
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStopping,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 4. Then confirm transition to stopped
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  ERROR HANDLING
	// -------------------------------------------------------------------------
	Context("Error Handling", func() {
		It("should handle errors during component configuration", func() {
			var err error

			// 1. First, wait for component to reach stopped state
			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 2. Set up mock to return error AFTER component has reached stopped state
			mockAdapter.ConfigureFailure("add", true)

			// 3. Try to activate
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// 4. This would normally transition to Starting, but should fail
			// We expect it to stay in the stopped state
			for i := 0; i < 5; i++ {
				err, _ = instance.Reconcile(ctx, tick)
				tick++
			}

			// Verify error was set in the component
			Expect(instance.GetError()).To(HaveOccurred())

			// Should stay in stopped state due to error
			Expect(instance.GetCurrentFSMState()).To(Equal(dataflowcomponent.OperationalStateStopped))

			// 5. Clear error and try again
			mockAdapter.ConfigureFailure("add", false)

			tick, err = fsmtest.WaitForInstanceState(
				ctx, instance, dataflowcomponent.OperationalStateStarting,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
