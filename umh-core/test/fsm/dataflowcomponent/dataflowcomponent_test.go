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
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm" // for LifecycleStateToBeCreated, etc.

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	dataflowcomponentsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/storage"
)

var _ = Describe("DataFlowComponent FSM", func() {
	var (
		instance       *dataflowcomponent.DataflowComponentInstance
		mockService    *dataflowcomponentsvc.MockDataFlowComponentService
		componentName  string
		ctx            context.Context
		tick           uint64
		mockFS         *filesystem.MockFileSystem
		archiveStorage storage.ArchiveStorer
	)

	BeforeEach(func() {
		componentName = "test-dataflow-component-fsm"
		ctx = context.Background()
		tick = 0

		archiveStorage = storage.NewArchiveEventStorage(100)
		instance, mockService, _ = fsmtest.SetupDataflowComponentInstance(componentName, dataflowcomponent.OperationalStateIdle, archiveStorage)
		mockFS = filesystem.NewMockFileSystem()
	})

	Context("Basic State Transitions", func() {
		It("should transition from stopped to starting", func() {
			var err error

			// Setup to Stopped state
			tick, err = setupToStoppedState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Transition from Stopped to Starting
			tick, err = transitionToStartingState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartDataFlowComponentCalled).To(BeTrue())
		})

		It("should transition from starting to idle", func() {
			var err error

			// Setup to Stopped state
			tick, err = setupToStoppedState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Transition from Stopped to Starting
			tick, err = transitionToStartingState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning: true,
				BenthosFSMState:  benthosfsm.OperationalStateIdle,
			})

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
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
			tick, err = setupToStoppedState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Transition from Stopped to Starting
			tick, err = transitionToStartingState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateIdle,
				IsBenthosProcessingMetricsActive: false,
			})

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an active state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateActive,
				IsBenthosProcessingMetricsActive: true,
			})
			// Execute transition: Idle -> Active
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
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
			tick, err = setupToStoppedState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Transition from Stopped to Starting
			tick, err = transitionToStartingState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateIdle,
				IsBenthosProcessingMetricsActive: false,
			})

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an active state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateActive,
				IsBenthosProcessingMetricsActive: true,
			})
			// Execute transition: Idle -> Active
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateIdle,
				dataflowcomponent.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateIdle,
				IsBenthosProcessingMetricsActive: false,
			})

			// Execute transition: Active -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
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
			tick, err = setupToStoppedState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Transition from Stopped to Starting
			tick, err = transitionToStartingState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateIdle,
				IsBenthosProcessingMetricsActive: false,
			})

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an degraded state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateDegraded,
				IsBenthosProcessingMetricsActive: false,
			})
			// Execute transition: Idle -> Degraded
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateIdle,
				dataflowcomponent.OperationalStateDegraded,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from degraded to idle", func() {
			var err error

			// Setup to Stopped state
			tick, err = setupToStoppedState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Transition from Stopped to Starting
			tick, err = transitionToStartingState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateIdle,
				IsBenthosProcessingMetricsActive: false,
			})

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an degraded state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateDegraded,
				IsBenthosProcessingMetricsActive: false,
			})
			// Execute transition: Idle -> Degraded
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateIdle,
				dataflowcomponent.OperationalStateDegraded,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateIdle,
				IsBenthosProcessingMetricsActive: true,
			})
			// Execute transition: Degraded -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
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

			var err error

			// Setup to Stopped state
			tick, err = setupToStoppedState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// Transition from Stopped to Starting
			tick, err = transitionToStartingState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())

			// set the mock flags for a starting failed state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning: false,
				BenthosFSMState:  benthosfsm.OperationalStateStopping,
			})
			// Add this stopping event to the archive storage
			archiveStorage.StoreDataPoint(ctx, storage.DataPoint{
				Record: storage.Record{
					ID:          "test-dataflow-component-fsm",
					State:       benthosfsm.OperationalStateStopping,
					SourceEvent: benthosfsm.OperationalStateStarting,
				},
				Time: time.Now(),
			})

			// Execute transition: Starting -> StartingFailed
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
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
			tick, err = setupToStoppedState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// 2. Transition from Stopped to Active (via Starting and Idle)
			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())

			// Transition from Stopped to Starting
			tick, err = transitionToStartingState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateIdle,
				IsBenthosProcessingMetricsActive: false,
			})

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an active state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateActive,
				IsBenthosProcessingMetricsActive: true,
			})

			// Execute transition: Idle -> Active
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
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
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateActive,
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
				mockFS,
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
			tick, err = setupToStoppedState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// 2. Transition from Stopped to Starting
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())
			tick, err = transitionToStartingState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateIdle,
				IsBenthosProcessingMetricsActive: false,
			})

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateStarting,
				dataflowcomponent.OperationalStateIdle,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for degraded state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateDegraded,
				IsBenthosProcessingMetricsActive: false,
			})

			// Execute transition: Idle -> Degraded
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
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
				mockFS,
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
				mockFS,
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
			tick, err = setupToStoppedState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddDataFlowComponentToBenthosManagerCalled).To(BeTrue())

			// 2. Set desired state to Active and transition from Stopped to Starting
			Expect(instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive)).To(Succeed())
			tick, err = transitionToStartingState(ctx, instance, mockService, mockFS, componentName, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
				IsBenthosRunning:                 true,
				BenthosFSMState:                  benthosfsm.OperationalStateIdle,
				IsBenthosProcessingMetricsActive: false,
			})

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestDataflowComponentStateTransition(
				ctx,
				instance,
				mockService,
				mockFS,
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
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateIdle,
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
				mockFS,
				componentName,
				dataflowcomponent.OperationalStateStopping,
				dataflowcomponent.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

})

// Helper function to bring the DataflowComponent to Stopped state
func setupToStoppedState(
	ctx context.Context,
	instance *dataflowcomponent.DataflowComponentInstance,
	mockService *dataflowcomponentsvc.MockDataFlowComponentService,
	mockFS *filesystem.MockFileSystem,
	componentName string,
	tick uint64,
) (uint64, error) {
	var err error

	// Step 1: ToBeCreated -> Creating
	tick, err = fsmtest.TestDataflowComponentStateTransition(
		ctx,
		instance,
		mockService,
		mockFS,
		componentName,
		internalfsm.LifecycleStateToBeCreated,
		internalfsm.LifecycleStateCreating,
		5,
		tick,
	)
	if err != nil {
		return tick, err
	}

	// Setup mock service state
	mockService.ComponentStates[componentName] = &dataflowcomponentsvc.ServiceInfo{
		BenthosFSMState: benthosfsm.OperationalStateStopped,
		BenthosObservedState: benthosfsm.BenthosObservedState{
			ServiceInfo: benthossvc.ServiceInfo{
				S6ObservedState: s6fsm.S6ObservedState{
					ServiceInfo: s6svc.ServiceInfo{
						Status: s6svc.ServiceDown,
						Uptime: 60,
					},
				},
			},
		},
	}
	mockService.ExistingComponents[componentName] = true

	// Step 2: Creating -> Stopped
	tick, err = fsmtest.TestDataflowComponentStateTransition(
		ctx,
		instance,
		mockService,
		mockFS,
		componentName,
		internalfsm.LifecycleStateCreating,
		dataflowcomponent.OperationalStateStopped,
		5,
		tick,
	)

	return tick, err
}

// Helper function to prepare for and execute transition to Starting state
func transitionToStartingState(
	ctx context.Context,
	instance *dataflowcomponent.DataflowComponentInstance,
	mockService *dataflowcomponentsvc.MockDataFlowComponentService,
	mockFS *filesystem.MockFileSystem,
	componentName string,
	tick uint64,
) (uint64, error) {
	// Set desired state to Active
	if err := instance.SetDesiredFSMState(dataflowcomponent.OperationalStateActive); err != nil {
		return tick, err
	}

	// Set up initial component state
	mockService.SetComponentState(componentName, dataflowcomponentsvc.ComponentStateFlags{
		IsBenthosRunning: false,
	})

	// Execute transition: Stopped -> Starting
	return fsmtest.TestDataflowComponentStateTransition(
		ctx,
		instance,
		mockService,
		mockFS,
		componentName,
		dataflowcomponent.OperationalStateStopped,
		dataflowcomponent.OperationalStateStarting,
		5,
		tick,
	)
}
