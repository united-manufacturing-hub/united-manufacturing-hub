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

package topicbrowser_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("TopicBrowser FSM", func() {
	var (
		instance     *topicbrowserfsm.Instance
		mockService  *topicbrowsersvc.MockService
		serviceName  string
		ctx          context.Context
		tick         uint64
		mockRegistry *serviceregistry.Registry
		startTime    time.Time
	)

	BeforeEach(func() {
		serviceName = "test-topicbrowser"
		ctx = context.Background()
		tick = 0
		startTime = time.Now()

		// Initialize a TopicBrowser instance with default desired state (Stopped to begin with)
		instance, mockService, _ = fsmtest.SetupTopicbrowserInstance(serviceName, topicbrowserfsm.OperationalStateStopped)
		mockRegistry = serviceregistry.NewMockRegistry()
	})

	// =========================================================================
	//  BASIC STATE TRANSITIONS
	// =========================================================================
	Context("Basic State Transitions", func() {
		It("should transition from Stopped to Starting when activated", func() {
			var err error

			// Phase 1: Initial lifecycle setup (to_be_created -> creating -> stopped)
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Simulate successful creation: service now exists in stopped state
			mockService.Existing[serviceName] = true
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				topicbrowserfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue()) // added to manager on creation

			// Phase 2: Activation sequence (set desired state to Active)
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())
			Expect(instance.GetDesiredFSMState()).To(Equal(topicbrowserfsm.OperationalStateActive))

			// Execute transition: Stopped -> Starting
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopped,
				topicbrowserfsm.OperationalStateStarting, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeTrue()) // ensure start initiated
		})

		It("should transition from Starting to Active when services are ready", func() {
			var err error

			// Phase 1: Setup to stopped state
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[serviceName] = true
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				topicbrowserfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Activate and reach Starting
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopped,
				topicbrowserfsm.OperationalStateStarting, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Simulate services ready, transition to Active
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStarting,
				topicbrowserfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Active to Stopping when deactivated", func() {
			var err error

			// Phase 1: Reach Active state through full startup sequence
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[serviceName] = true
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				topicbrowserfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Active
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopped,
				topicbrowserfsm.OperationalStateStarting, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStarting,
				topicbrowserfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Request stop (Active -> Stopping)
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateStopped)).To(Succeed())

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateActive,
				topicbrowserfsm.OperationalStateStopping, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StopCalled).To(BeTrue())
		})

		It("should transition from Stopping to Stopped when services are stopped", func() {
			var err error

			// Phase 1: Reach Active state
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[serviceName] = true
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				topicbrowserfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopped,
				topicbrowserfsm.OperationalStateStarting, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStarting,
				topicbrowserfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Stop and reach Stopping
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateStopped)).To(Succeed())

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateActive,
				topicbrowserfsm.OperationalStateStopping, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Complete shutdown (Stopping -> Stopped)
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopping,
				topicbrowserfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  DEGRADED AND RECOVERY SCENARIOS
	// =========================================================================
	Context("Degraded and Recovery Scenarios", func() {
		It("should transition to Degraded when services fail and recover when restored", func() {
			var err error

			// Phase 1: Establish Active state
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[serviceName] = true
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				topicbrowserfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())

			// Progress through startup to Active
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopped,
				topicbrowserfsm.OperationalStateStarting, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStarting,
				topicbrowserfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate service degradation
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateDegraded)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateActive,
				topicbrowserfsm.OperationalStateDegraded, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Restore services and verify recovery
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateDegraded,
				topicbrowserfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle multiple degradation and recovery cycles", func() {
			var err error

			// Phase 1: Establish Active state
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[serviceName] = true
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				topicbrowserfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopped,
				topicbrowserfsm.OperationalStateStarting, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStarting,
				topicbrowserfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: First degradation cycle
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateDegraded)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateActive,
				topicbrowserfsm.OperationalStateDegraded, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Recovery
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateDegraded,
				topicbrowserfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Second degradation cycle
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateDegraded)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateActive,
				topicbrowserfsm.OperationalStateDegraded, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Final recovery
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateDegraded,
				topicbrowserfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  STATE STABILITY
	// =========================================================================
	Context("State Stability", func() {
		It("should remain stable in Active state when services are healthy", func() {
			var err error

			// Set to active from stopped state
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())

			// Phase 1: Establish Active state
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				topicbrowserfsm.OperationalStateActive, 15, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Verify stability over multiple reconcile cycles
			tick, err = fsmtest.VerifyTopicbrowserStableState(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateActive, 5)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remain stable in Stopped state when desired state is stopped", func() {
			var err error

			// Phase 1: Establish Stopped state (this is the default)
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				topicbrowserfsm.OperationalStateStopped, 10, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Verify stability over multiple reconcile cycles
			tick, err = fsmtest.VerifyTopicbrowserStableState(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopped, 5)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remain stable in Degraded state when services remain unhealthy", func() {
			var err error

			// Phase 1: Establish Active state first
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				topicbrowserfsm.OperationalStateActive, 15, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Transition to degraded
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateDegraded)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateActive,
				topicbrowserfsm.OperationalStateDegraded, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Verify stability in degraded state
			tick, err = fsmtest.VerifyTopicbrowserStableState(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateDegraded, 5)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  ERROR HANDLING
	// =========================================================================
	Context("Error Handling", func() {
		It("should handle service creation failures gracefully", func() {
			var err error

			// Configure mock to return error on service creation
			mockService.AddToManagerError = fmt.Errorf("mock service creation error")

			// Attempt reconciliation several times
			finalTick, err, _ := fsmtest.ReconcileTopicbrowserUntilError(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockRegistry, serviceName, 5)

			// Should get an error because service creation failed
			Expect(err).To(HaveOccurred())
			Expect(finalTick).To(BeNumerically(">", tick))

			// The instance should be stuck in creating state with error
			currentState := instance.GetCurrentFSMState()
			Expect(currentState).To(Equal(internalfsm.LifecycleStateCreating))

			// Reset error and verify recovery
			fsmtest.ResetTopicbrowserInstanceError(mockService)
			mockService.Existing[serviceName] = true
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				topicbrowserfsm.OperationalStateStopped, 5, finalTick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle service start failures gracefully", func() {
			var err error

			// Phase 1: Establish stopped state
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[serviceName] = true
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				topicbrowserfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Configure start error and attempt activation
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())
			mockService.StartError = fmt.Errorf("mock service start error")

			// Should get stuck trying to start
			finalTick, err, _ := fsmtest.ReconcileTopicbrowserUntilError(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockRegistry, serviceName, 5)

			Expect(err).To(HaveOccurred())
			Expect(finalTick).To(BeNumerically(">", tick))

			// Reset error and verify recovery
			fsmtest.ResetTopicbrowserInstanceError(mockService)
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopped,
				topicbrowserfsm.OperationalStateActive, 10, finalTick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  LIFECYCLE MANAGEMENT
	// =========================================================================
	Context("Lifecycle Management", func() {
		It("should handle complete lifecycle from creation to removal", func() {
			var err error

			// Phase 1: Creation lifecycle
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[serviceName] = true
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				topicbrowserfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Operational usage
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopped,
				topicbrowserfsm.OperationalStateStarting, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStarting,
				topicbrowserfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Shutdown before removal
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateStopped)).To(Succeed())

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateActive,
				topicbrowserfsm.OperationalStateStopping, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopping,
				topicbrowserfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 4: Removal lifecycle
			Expect(instance.Remove(ctx)).To(Succeed())

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopped,
				internalfsm.LifecycleStateRemoving, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.RemoveFromManagerCalled).To(BeTrue())

			// Simulate successful removal
			delete(mockService.Existing, serviceName)
			delete(mockService.States, serviceName)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateRemoving,
				internalfsm.LifecycleStateRemoved, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle desired state changes during lifecycle transitions", func() {
			var err error

			// Start with creation
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Change desired state while still creating (should be ignored until creation completes)
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())

			// Complete creation - should go to stopped first, then start transitioning to active
			mockService.Existing[serviceName] = true
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateStopped)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateCreating,
				topicbrowserfsm.OperationalStateStopped, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Now should start transitioning to active since that's the desired state
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStopped,
				topicbrowserfsm.OperationalStateStarting, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Complete transition to active
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateStarting,
				topicbrowserfsm.OperationalStateActive, 5, tick, startTime)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  CONFIGURATION SCENARIOS
	// =========================================================================
	Context("Configuration Scenarios", func() {
		It("should handle empty service name gracefully", func() {
			// Create instance with empty service name
			emptyInstance, emptyMockService, _ := fsmtest.SetupTopicbrowserInstance("", topicbrowserfsm.OperationalStateStopped)

			tick := uint64(0)

			// Should handle gracefully - might stay in to_be_created or transition to creating with error
			finalTick, _, _ := fsmtest.ReconcileTopicbrowserUntilError(
				ctx, fsm.SystemSnapshot{Tick: tick}, emptyInstance, emptyMockService, mockRegistry, "", 5)

			// The behavior depends on implementation - it might error or handle gracefully
			// Either way, it shouldn't crash
			Expect(finalTick).To(BeNumerically(">=", tick))

			// Verify instance state is reasonable
			currentState := emptyInstance.GetCurrentFSMState()
			Expect(currentState).To(BeElementOf([]string{
				internalfsm.LifecycleStateToBeCreated,
				internalfsm.LifecycleStateCreating,
			}))
		})

		It("should handle rapid desired state changes", func() {
			var err error

			// Phase 1: Get to stopped state
			tick, err = fsmtest.TestTopicbrowserStateTransition(
				ctx, instance, mockService, mockRegistry, serviceName,
				internalfsm.LifecycleStateToBeCreated,
				topicbrowserfsm.OperationalStateStopped, 10, tick, startTime)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Rapid state changes
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateStopped)).To(Succeed())
			Expect(instance.SetDesiredFSMState(topicbrowserfsm.OperationalStateActive)).To(Succeed())

			// Should eventually settle on the final desired state (Active)
			fsmtest.TransitionToTopicbrowserState(mockService, serviceName, topicbrowserfsm.OperationalStateActive)

			tick, err = fsmtest.StabilizeTopicbrowserInstance(
				ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockService, mockRegistry, serviceName,
				topicbrowserfsm.OperationalStateActive, 15)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
