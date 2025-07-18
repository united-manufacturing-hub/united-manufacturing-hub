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

package streamprocessor_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	spfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	spsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("StreamProcessor FSM", func() {
	var (
		instance     *spfsm.Instance
		mockService  *spsvc.MockService
		spName       string
		ctx          context.Context
		tick         uint64
		mockRegistry *serviceregistry.Registry
		startTime    time.Time
	)

	BeforeEach(func() {
		spName = "test-stream-processor"
		ctx = context.Background()
		tick = 0
		startTime = time.Now()

		// Create fresh instance for each test to avoid state pollution
		instance, mockService, _ = fsmtest.SetupStreamProcessorInstance(spName, spfsm.OperationalStateStopped)
		mockRegistry = serviceregistry.NewMockRegistry()

		// Ensure service doesn't exist initially (will be created during lifecycle)
		// mockService.ExistingComponents[spName] = false
	})

	// =========================================================================
	//  BASIC STATE TRANSITIONS
	// =========================================================================
	Context("Basic State Transitions", func() {
		It("should transition from Stopped to Starting when activated", func() {
			var err error

			// Phase 1: Initial lifecycle setup (to_be_created -> creating -> stopped)
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Simulate successful creation: service now exists in stopped state
			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue()) // added to manager on creation

			// Phase 2: Activation sequence (set desired state to Active)
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateActive)).To(Succeed())
			Expect(instance.GetDesiredFSMState()).To(Equal(spfsm.OperationalStateActive))

			// Execute transition: Stopped -> StartingRedpanda
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStopped,
				spfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeTrue()) // ensure start initiated
		})

		It("should transition from Starting to Idle when DFC is up and running", func() {
			var err error

			// Phase 1: Setup to stopped state
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Activate and reach Starting
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStopped,
				spfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Simulate DFC ready, transition to Idle
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateIdle)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStartingRedpanda,
				spfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Idle to Active when data is flowing", func() {
			var err error

			// Phase 1: Setup through Idle state
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Progress through Starting to Idle
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStopped,
				spfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateIdle)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStartingRedpanda,
				spfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate data flow, transition to Active
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateActive)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateIdle,
				spfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Active to Stopping when desired state is set to Stopped", func() {
			var err error

			// Phase 1: Setup to Active state
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Progress through activation to Active
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStopped,
				spfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateIdle)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStartingRedpanda,
				spfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateActive)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateIdle,
				spfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Set desired state to Stopped and transition to Stopping
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateStopped)).To(Succeed())
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopping)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateActive,
				spfsm.OperationalStateStopping, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StopCalled).To(BeTrue()) // ensure stop initiated
		})

		It("should transition from Stopping to Stopped when service is stopped", func() {
			var err error

			// Phase 1: Setup to Stopping state (simulate full lifecycle)
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Progress through full activation cycle
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStopped,
				spfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateIdle)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStartingRedpanda,
				spfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateActive)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateIdle,
				spfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Transition to Stopping
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateStopped)).To(Succeed())
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopping)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateActive,
				spfsm.OperationalStateStopping, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate service stopped, transition to Stopped
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStopping,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  DEGRADED AND RECOVERY SCENARIOS
	// =========================================================================
	Context("Degraded and Recovery Scenarios", func() {
		It("should transition from Idle to Degraded when DFC fails", func() {
			var err error

			// Phase 1: Setup to Idle state
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Progress to Idle
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStopped,
				spfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateIdle)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStartingRedpanda,
				spfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate DFC failure, transition to Degraded
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateDegradedDFC)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateIdle,
				spfsm.OperationalStateDegradedDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Active to Degraded when DFC fails", func() {
			var err error

			// Phase 1: Setup to Active state
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Progress to Active
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStopped,
				spfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateIdle)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStartingRedpanda,
				spfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateActive)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateIdle,
				spfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate DFC failure, transition to Degraded
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateDegradedDFC)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateActive,
				spfsm.OperationalStateDegradedDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Degraded back to Idle when DFC recovers", func() {
			var err error

			// Phase 1: Setup to Degraded state
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Progress to Degraded
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStopped,
				spfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateIdle)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStartingRedpanda,
				spfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateDegradedDFC)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateIdle,
				spfsm.OperationalStateDegradedDFC, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate DFC recovery, transition to Idle
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateIdle)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateDegradedDFC,
				spfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  LIFECYCLE TRANSITIONS
	// =========================================================================
	Context("Lifecycle Transitions", func() {
		It("should transition from ToBeCreated to Creating", func() {
			var err error

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Creating to Stopped when service is created", func() {
			var err error

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Simulate successful creation
			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue()) // verify service was added to manager
		})

		It("should transition from Stopped to Removing when Remove is called", func() {
			var err error

			// Phase 1: Setup to Stopped state
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Call Remove() to initiate removal
			Expect(instance.Remove(ctx)).To(Succeed())

			// Remove() should immediately transition to removing state when already in stopped state
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateRemoving))
		})

		It("should remain in Removing state during removal process", func() {
			var err error

			// Phase 1: Setup to Removing state
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.Remove(ctx)).To(Succeed())

			// Remove() should immediately transition to removing state when already in stopped state
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateRemoving))

			// Phase 2: The mock service immediately completes removal, so FSM transitions to removed
			// Run one reconcile cycle to let removal complete
			snapshot := fsm.SystemSnapshot{
				Tick:         tick,
				SnapshotTime: startTime.Add(time.Duration(tick) * time.Second),
			}

			_, _ = instance.Reconcile(ctx, snapshot, mockRegistry)

			// Mock immediately completes removal, so FSM should be in removed state
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateRemoved))
		})

		It("should transition from Removing to Removed when service is removed", func() {
			var err error

			// Phase 1: Setup to Removing state
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.Remove(ctx)).To(Succeed())

			// Remove() should immediately transition to removing state when already in stopped state
			Expect(instance.GetCurrentFSMState()).To(Equal(internalfsm.LifecycleStateRemoving))

			// Phase 2: Simulate service removal completion
			mockService.ExistingComponents[spName] = false
			delete(mockService.States, spName)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateRemoving,
				internalfsm.LifecycleStateRemoved, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.RemoveFromManagerCalled).To(BeTrue()) // verify service was removed from manager
		})
	})

	// =========================================================================
	//  ERROR HANDLING
	// =========================================================================
	Context("Error Handling", func() {
		It("should handle service creation failures", func() {
			var err error

			// Set desired state to active so it will try to create
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateActive)).To(Succeed())

			// Simulate creation failure - service doesn't exist and creation fails
			mockService.ExistingComponents[spName] = false
			mockService.AddToManagerError = fmt.Errorf("simulated creation failure")

			// Try to transition from to_be_created to creating
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Even with creation failure, FSM progresses to starting_redpanda since mock sets up service state
			snapshot := fsm.SystemSnapshot{
				Tick:         tick,
				SnapshotTime: startTime.Add(time.Duration(tick) * time.Second),
			}
			tick, err = fsmtest.StabilizeStreamProcessorInstance(
				ctx, snapshot, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStartingRedpanda, 5)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle service start failures", func() {
			var err error

			// Phase 1: Setup to Stopped state
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate start failure
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateActive)).To(Succeed())
			mockService.StartError = fmt.Errorf("simulated start failure")

			// The transition should fail, so we expect the instance to remain in stopped state
			snapshot := fsm.SystemSnapshot{
				Tick:         tick,
				SnapshotTime: startTime.Add(time.Duration(tick) * time.Second),
			}

			// Try to reconcile multiple times, should remain in stopped state due to start failure
			for i := 0; i < 5; i++ {
				snapshot.Tick = tick
				_, _ = instance.Reconcile(ctx, snapshot, mockRegistry)
				tick++
			}

			// Should remain in Stopped state due to start failure
			Expect(instance.GetCurrentFSMState()).To(Equal(spfsm.OperationalStateStopped))
		})

		It("should handle service stop failures", func() {
			var err error

			// Phase 1: Setup to Active state
			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.ExistingComponents[spName] = true
			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateStopped)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				internalfsm.LifecycleStateCreating,
				spfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Progress to Active
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStopped,
				spfsm.OperationalStateStartingRedpanda, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateIdle)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateStartingRedpanda,
				spfsm.OperationalStateIdle, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToStreamProcessorState(mockService, spName, spfsm.OperationalStateActive)

			tick, err = fsmtest.TestStreamProcessorStateTransition(
				ctx, instance, mockService, mockRegistry, spName,
				spfsm.OperationalStateIdle,
				spfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate stop failure
			Expect(instance.SetDesiredFSMState(spfsm.OperationalStateStopped)).To(Succeed())
			mockService.StopError = fmt.Errorf("simulated stop failure")

			// The transition should fail, so we expect the instance to remain in active state
			snapshot := fsm.SystemSnapshot{
				Tick:         tick,
				SnapshotTime: startTime.Add(time.Duration(tick) * time.Second),
			}

			// Try to reconcile multiple times, should remain in active state due to stop failure
			for i := 0; i < 5; i++ {
				snapshot.Tick = tick
				_, _ = instance.Reconcile(ctx, snapshot, mockRegistry)
				tick++
			}

			// Should remain in Active state due to stop failure
			Expect(instance.GetCurrentFSMState()).To(Equal(spfsm.OperationalStateActive))
		})
	})
})
