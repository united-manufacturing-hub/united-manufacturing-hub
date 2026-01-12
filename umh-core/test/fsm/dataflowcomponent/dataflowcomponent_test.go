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

package dataflowcomponent_test

import (
	"context"
	"fmt"

	pkgfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	dataflowcomponentsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DataFlowComponent FSM", func() {
	var (
		instance        *dataflowcomponent.DataflowComponentInstance
		mockService     *dataflowcomponentsvc.MockDataFlowComponentService
		componentName   string
		ctx             context.Context
		tick            uint64
		mockSvcRegistry *serviceregistry.Registry
	)

	BeforeEach(func() {
		componentName = "test-dataflow-component-fsm"
		ctx = context.Background()
		tick = 0

		instance, mockService, _ = fsmtest.SetupDataflowComponentInstance(componentName, dataflowcomponent.OperationalStateIdle)
		mockSvcRegistry = serviceregistry.NewMockRegistry()
	})

	Context("Basic State Transitions", func() {
		It("should transition from stopped to starting", func() {
			var err error

			// Setup to Stopped state
			// ToBeCreated → Creating
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				dataflowcomponent.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Transition from Stopped to Starting
			fsmtest.TransitionToDataflowComponentState(mockService, componentName, dataflowcomponent.OperationalStateStarting)

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				dataflowcomponent.OperationalStateStopped,
				dataflowcomponent.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartDataFlowComponentCalled).To(BeTrue())
		})

		It("should transition from starting to idle", func() {
			var err error

			// Setup to Stopped state
			// ToBeCreated → Creating
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				dataflowcomponent.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Transition from Stopped to Starting
			fsmtest.TransitionToDataflowComponentState(mockService, componentName, dataflowcomponent.OperationalStateStarting)

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStopped,
				dataflowcomponent.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			fsmtest.TransitionToDataflowComponentState(mockService, componentName, dataflowcomponent.OperationalStateIdle)

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from idle to active", func() {
			var err error

			// Setup to Stopped state
			// ToBeCreated → Creating
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				dataflowcomponent.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				dataflowcomponent.OperationalStateStopped,
				dataflowcomponent.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Idle -> Active
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateIdle,
				dataflowcomponent.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should transition from active to idle", func() {
			var err error

			// Setup to Stopped state
			// ToBeCreated → Creating
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				dataflowcomponent.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				dataflowcomponent.OperationalStateStopped,
				dataflowcomponent.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Idle -> Active
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateIdle,
				dataflowcomponent.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Active -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateActive,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from idle to degraded", func() {
			var err error

			// Setup to Stopped state
			// ToBeCreated → Creating
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				dataflowcomponent.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				dataflowcomponent.OperationalStateStopped,
				dataflowcomponent.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Idle -> Degraded
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateIdle,
				dataflowcomponent.OperationalStateDegraded,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Degraded -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateDegraded,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Transition to Failed State", func() {
		It("should transition from starting to starting failed", func() {
			Skip("TODO: Implement this")
			var err error

			// Setup to Stopped state
			// ToBeCreated → Creating
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				dataflowcomponent.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Transition from Stopped to Starting
			fsmtest.TransitionToDataflowComponentState(mockService, componentName, dataflowcomponent.OperationalStateStarting)
			Expect(err).NotTo(HaveOccurred())

			// set the mock flags for a starting failed state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning: false,
				BenthosFSMState:  benthosfsm.OperationalStateStopping,
			})
			// Add this stopping event to the archive storage
			//archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
			//	Record: storage.Record{
			//		ID:          "test-dataflow-component-fsm",
			//		State:       benthosfsm.OperationalStateStopping,
			//		SourceEvent: benthosfsm.OperationalStateStarting,
			//	},
			//	Time: time.Now(),
			//})

			// Execute transition: Starting -> StartingFailed
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateStartingFailed,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Stopping Flow", func() {
		It("should stop gracefully from Active state", func() {
			var err error

			// Setup to Active state
			// 1. First get to Stopped state
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				dataflowcomponent.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// 2. Transition from Stopped to Active (via Starting and Idle)
			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				dataflowcomponent.OperationalStateStopped,
				dataflowcomponent.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Idle -> Active
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateIdle,
				dataflowcomponent.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 3. Now set desired state to Stopped
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateStopped)).To(Succeed())

			// Execute transition: Active -> Stopping
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateActive,
				dataflowcomponent.OperationalStateStopping,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StopDataFlowComponentCalled).To(BeTrue())

			// Execute transition: Stopping -> Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStopping,
				dataflowcomponent.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should stop gracefully from Degraded state", func() {
			var err error

			// Setup to Degraded state
			// 1. First get to Stopped state
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				dataflowcomponent.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// 2. Transition from Stopped to Starting
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				dataflowcomponent.OperationalStateStopped,
				dataflowcomponent.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Idle -> Degraded
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateIdle,
				dataflowcomponent.OperationalStateDegraded,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 3. Now set desired state to Stopped
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateStopped)).To(Succeed())

			// Execute transition: Degraded -> Stopping
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateDegraded,
				dataflowcomponent.OperationalStateStopping,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StopDataFlowComponentCalled).To(BeTrue())

			// Simulate Benthos stopping
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning: false,
				BenthosFSMState:  benthosfsm.OperationalStateStopped,
			})

			// Execute transition: Stopping -> Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStopping,
				dataflowcomponent.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should stop gracefully from Idle state", func() {
			var err error

			// Setup to Idle state
			// 1. First get to Stopped state
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				dataflowcomponent.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// 2. Set desired state to Active and transition from Stopped to Starting
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				dataflowcomponent.OperationalStateStopped,
				dataflowcomponent.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 3. Now set desired state to Stopped
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateStopped)).To(Succeed())

			// Execute transition: Idle -> Stopping
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateIdle,
				dataflowcomponent.OperationalStateStopping,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StopDataFlowComponentCalled).To(BeTrue())

			// Execute transition: Stopping -> Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStopping,
				dataflowcomponent.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Active to Degraded on metrics failure", func() {
			var err error
			// Setup to Active state
			// 1. First get to Stopped state
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				dataflowcomponent.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// 2. Transition from Stopped to Active (via Starting and Idle)
			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				dataflowcomponent.OperationalStateStopped,
				dataflowcomponent.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Remain Starting
			snapshot := pkgfsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyDataflowComponentStableState(
				ctx, snapshot,
				instance, mockService, mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				3, // number of reconcile cycles to test
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Idle -> Active
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateIdle,
				dataflowcomponent.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Remain Active

			snapshot = pkgfsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyDataflowComponentStableState(
				ctx, snapshot,
				instance, mockService, mockSvcRegistry,
				componentName,
				dataflowcomponent.OperationalStateActive,
				3, // number of reconcile cycles to test
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Active -> Degraded
			fsmtest.TransitionToDataflowComponentState(mockService, componentName,
				dataflowcomponent.OperationalStateDegraded)

			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				dataflowcomponent.OperationalStateActive,
				dataflowcomponent.OperationalStateDegraded,
				5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Can now get Active again

			// Execute transition: Degraded -> Active
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				dataflowcomponent.OperationalStateDegraded,
				dataflowcomponent.OperationalStateActive,
				5, tick)
			Expect(err).NotTo(HaveOccurred())
		})

	})

	Context("Error Handling", func() {
		It("should handle S6 service not exist error correctly", func() {
			var err error

			// Setup to Creating state
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Force the exact error message from the logs with proper error wrapping
			mockService.StatusError = fmt.Errorf("failed to get benthos config: failed to get benthos config file for service benthos-dataflow-%s: %w", componentName, s6.ErrServiceNotExist)

			// Attempt to reconcile - this should not set an FSM error if errors are correctly identified
			snapshot := pkgfsm.SystemSnapshot{Tick: tick}
			err, _ = instance.Reconcile(ctx, snapshot, mockSvcRegistry)
			Expect(err).To(BeNil()) // Should not propagate an error

			// The instance should not have an error set
			Expect(instance.GetLastError()).To(BeNil())

			// Clean up the mock for other tests
			mockService.StatusError = nil
		})
	})

})
