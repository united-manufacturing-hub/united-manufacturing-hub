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

package topicbrowser_test

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	pkgfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	topicbrowser "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TopicBrowser FSM", func() {
	var (
		instance        *topicbrowser.TopicBrowserInstance
		mockService     *topicbrowsersvc.MockService
		componentName   string
		ctx             context.Context
		tick            uint64
		mockSvcRegistry *serviceregistry.Registry
	)

	BeforeEach(func() {
		componentName = constants.TopicBrowserServiceName
		ctx = context.Background()
		tick = 0

		instance, mockService, _ = fsmtest.SetupTopicBrowserInstance(componentName, topicbrowser.OperationalStateStopped)
		mockSvcRegistry = serviceregistry.NewMockRegistry()
	})

	Context("Basic State Transitions", func() {
		It("should transition from stopped to starting", func() {
			var err error

			// Setup to Stopped state
			// ToBeCreated → Creating
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue())

			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Transition from Stopped to Starting
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStarting)

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeTrue())
		})

		It("should transition from starting to active", func() {
			var err error

			// Setup to Stopped state
			// ToBeCreated → Creating
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue())

			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Transition from Stopped to Starting
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStarting)

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Set the mock flags for an idle state
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateActive)

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from active to degraded", func() {
			var err error

			// Setup to Stopped state
			// ToBeCreated → Creating
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue())

			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Active -> DegradedBenthos
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateDegradedBenthos,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: DegradedBenthos -> Active
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateDegradedBenthos,
				topicbrowser.OperationalStateActive,
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
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue())

			// 2. Transition from Stopped to Active (via Starting and Idle)
			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Active
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 3. Now set desired state to Stopped
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateStopped)).To(Succeed())

			// Execute transition: Active -> Stopping
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateStopping,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StopCalled).To(BeTrue())

			// Execute transition: Stopping -> Stopped
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateStopping,
				topicbrowser.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should stop gracefully from Degraded state", func() {
			var err error

			// Setup to Degraded state
			// 1. First get to Stopped state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue())

			// 2. Transition from Stopped to Starting
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Active
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Active -> DegradedBenthos
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateDegradedBenthos,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 3. Now set desired state to Stopped (while still in DegradedBenthos)
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateStopped)).To(Succeed())

			// Execute transition: DegradedBenthos -> Stopping
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateDegradedBenthos,
				topicbrowser.OperationalStateStopping,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StopCalled).To(BeTrue())

			// Simulate Benthos stopping
			mockService.SetState(componentName, topicbrowsersvc.StateFlags{
				BenthosFSMState: benthosfsm.OperationalStateStopped,
			})

			// Execute transition: Stopping -> Stopped
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateStopping,
				topicbrowser.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should stop gracefully from Active state", func() {
			var err error

			// Setup to Active state
			// 1. First get to Stopped state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue())

			// 2. Set desired state to Active and transition from Stopped to Starting
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Idle
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 3. Now set desired state to Stopped
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateStopped)).To(Succeed())

			// Execute transition: Active -> Stopping
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateStopping,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StopCalled).To(BeTrue())

			// Execute transition: Stopping -> Stopped
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateStopping,
				topicbrowser.OperationalStateStopped,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition from Active to Degraded on metrics failure", func() {
			var err error
			// Setup to Active state
			// 1. First get to Stopped state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue())

			// 2. Transition from Stopped to Active (via Starting and Idle)
			// Set desired state to Active
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Execute transition: Stopped → Starting
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Remain Starting
			snapshot := pkgfsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyTopicBrowserStableState(
				ctx, snapshot,
				instance, mockService, mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateStarting,
				3, // number of reconcile cycles to test
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Starting -> Active
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateActive,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Remain Active
			snapshot = pkgfsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyTopicBrowserStableState(
				ctx, snapshot,
				instance, mockService, mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateActive,
				3, // number of reconcile cycles to test
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Active -> DegradedBenthos
			fsmtest.TransitionToTopicBrowserState(mockService, componentName,
				topicbrowser.OperationalStateDegradedBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateDegradedBenthos,
				5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Can now get Active again

			// Execute transition: DegradedBenthos -> Active
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateDegradedBenthos,
				topicbrowser.OperationalStateActive,
				5, tick)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Error Handling", func() {
		It("should handle S6 service not exist error correctly", func() {
			var err error

			// Setup to Creating state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
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

	// =========================================================================
	//  STARTUP FAILURE RECOVERY SCENARIOS
	// =========================================================================
	Context("Startup Failure Recovery Scenarios", func() {
		It("should restart startup when benthos fails during StartingBenthos state", func() {
			var err error

			// Phase 1: Get to StartingBenthos state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[componentName] = true
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStopped)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Progress to StartingBenthos
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateStartingBenthos, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate benthos failure during StartingBenthos state
			// This should restart the startup sequence (go back to Starting)
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStarting)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingBenthos,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Continue normal startup after recovery
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateIdle)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateIdle, 10, tick)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should restart startup when benthos fails during StartingRedpanda state", func() {
			var err error

			// Phase 1: Get to StartingRedpanda state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[componentName] = true
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStopped)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Progress to StartingRedpanda
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateStartingBenthos, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingBenthos,
				topicbrowser.OperationalStateStartingRedpanda, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate benthos failure during StartingRedpanda state
			// This should restart the startup sequence (go back to Starting)
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStarting)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingRedpanda,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Continue normal startup after recovery
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateIdle)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateIdle, 10, tick)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should restart startup when redpanda fails during StartingRedpanda state", func() {
			var err error

			// Phase 1: Get to StartingRedpanda state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[componentName] = true
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStopped)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Progress to StartingRedpanda
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateStartingBenthos, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingBenthos,
				topicbrowser.OperationalStateStartingRedpanda, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate redpanda failure during StartingRedpanda state
			// This should restart the startup sequence (go back to Starting)
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStarting)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingRedpanda,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Continue normal startup after recovery
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateIdle)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateIdle, 10, tick)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  DEGRADED AND RECOVERY SCENARIOS
	// =========================================================================
	Context("Degraded and Recovery Scenarios", func() {
		It("should transition to DegradedBenthos when benthos becomes unavailable and recover when restored", func() {
			var err error

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[componentName] = true
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStopped)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateStartingBenthos, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingBenthos,
				topicbrowser.OperationalStateStartingRedpanda, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateIdle)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingRedpanda,
				topicbrowser.OperationalStateIdle, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate benthos failure and recovery
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateDegradedBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateIdle,
				topicbrowser.OperationalStateDegradedBenthos, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Restore benthos and verify recovery
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateIdle)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateDegradedBenthos,
				topicbrowser.OperationalStateIdle, 5, tick)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should transition to DegradedRedpanda when Redpanda becomes unavailable and recover when restored", func() {
			var err error

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[componentName] = true
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStopped)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateStartingBenthos, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingBenthos,
				topicbrowser.OperationalStateStartingRedpanda, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateIdle)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingRedpanda,
				topicbrowser.OperationalStateIdle, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate Redpanda failure and recovery
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateDegradedRedpanda)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateIdle,
				topicbrowser.OperationalStateDegradedRedpanda, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Restore Redpanda and verify recovery
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateIdle)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateDegradedRedpanda,
				topicbrowser.OperationalStateIdle, 5, tick)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle multiple degraded states and recover properly", func() {
			var err error

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[componentName] = true
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStopped)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Progress through startup to Idle
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateStartingBenthos, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingBenthos,
				topicbrowser.OperationalStateStartingRedpanda, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateIdle)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingRedpanda,
				topicbrowser.OperationalStateIdle, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateActive)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateIdle,
				topicbrowser.OperationalStateActive, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Test degraded benthos -> degraded redpanda transitions
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateDegradedBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateDegradedBenthos, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Now also degrade redpanda (benthos still degraded)
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateDegradedRedpanda)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateDegradedBenthos,
				topicbrowser.OperationalStateDegradedRedpanda, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 3: Recover from all degraded states
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateIdle)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateDegradedRedpanda,
				topicbrowser.OperationalStateIdle, 5, tick)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  STARTUP FAILURE SCENARIOS
	// =========================================================================
	Context("Startup Failure Scenarios", func() {
		It("should handle benthos startup failures gracefully", func() {
			var err error

			// Phase 1: Progress to StartingBenthos state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[componentName] = true
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStopped)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence to StartingBenthos
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateStartingBenthos, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate benthos startup failure - it should transition to degraded
			// Configure benthos to fail startup
			fsmtest.SetupTopicBrowserServiceState(mockService, componentName, topicbrowsersvc.StateFlags{
				BenthosFSMState:       benthosfsm.OperationalStateStopped, // Benthos failed to start
				RedpandaFSMState:      redpandafsm.OperationalStateActive, // Redpanda is OK
				HasProcessingActivity: false,
				HasBenthosOutput:      false,
			})

			// It should stay in StartingBenthos since benthos isn't ready
			tick, err = fsmtest.VerifyTopicBrowserStableState(
				ctx, pkgfsm.SystemSnapshot{Tick: tick}, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingBenthos, 3)
			Expect(err).NotTo(HaveOccurred())

			// Current state should remain starting since benthos is not ready
			Expect(instance.GetCurrentFSMState()).To(Equal(topicbrowser.OperationalStateStartingBenthos))
		})

		It("should restart startup when redpanda fails during startup", func() {
			var err error

			// Phase 1: Progress to StartingRedpanda state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			mockService.Existing[componentName] = true
			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStopped)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				topicbrowser.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Progress through startup sequence to StartingRedpanda
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStopped,
				topicbrowser.OperationalStateStarting, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStarting,
				topicbrowser.OperationalStateStartingBenthos, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateStartingRedpanda)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateStartingBenthos,
				topicbrowser.OperationalStateStartingRedpanda, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Simulate redpanda startup failure
			// Configure redpanda to fail startup
			fsmtest.SetupTopicBrowserServiceState(mockService, componentName, topicbrowsersvc.StateFlags{
				BenthosFSMState:       benthosfsm.OperationalStateActive,   // Benthos is OK
				RedpandaFSMState:      redpandafsm.OperationalStateStopped, // Redpanda failed to start
				HasProcessingActivity: false,
				HasBenthosOutput:      false,
			})

			// Verify we're in the right starting state
			initialState := instance.GetCurrentFSMState()
			Expect(initialState).To(Equal(topicbrowser.OperationalStateStartingRedpanda))

			// Execute reconcile - should transition back to Starting due to failure
			_, _ = instance.Reconcile(ctx, pkgfsm.SystemSnapshot{Tick: tick}, mockSvcRegistry)
			tick++

			// Should now be back in Starting state to restart the sequence
			currentState := instance.GetCurrentFSMState()
			Expect(currentState).To(Equal(topicbrowser.OperationalStateStarting),
				fmt.Sprintf("Expected transition to %s but got %s",
					topicbrowser.OperationalStateStarting, currentState))
		})
	})

	// =========================================================================
	//  STATE STABILITY
	// =========================================================================
	Context("State Stability", func() {
		It("should remain stable in Idle state when no activity occurs", func() {
			var err error

			// Set to active from stopped state
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Phase 1: Establish Idle state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				topicbrowser.OperationalStateIdle, 15, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Verify stability over multiple reconcile cycles
			tick, err = fsmtest.VerifyTopicBrowserStableState(
				ctx, pkgfsm.SystemSnapshot{Tick: tick}, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateIdle, 5)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remain stable in Active state when processing continues", func() {
			var err error

			// Set to active from stopped state
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Phase 1: Establish Active state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				topicbrowser.OperationalStateIdle, 15, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateActive)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateIdle,
				topicbrowser.OperationalStateActive, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Verify stability over multiple reconcile cycles
			tick, err = fsmtest.VerifyTopicBrowserStableState(
				ctx, pkgfsm.SystemSnapshot{Tick: tick}, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateActive, 5)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remain stable in degraded states when issues persist", func() {
			var err error

			// Set to active from stopped state
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateActive)).To(Succeed())

			// Phase 1: Establish DegradedBenthos state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				topicbrowser.OperationalStateActive, 15, tick)
			Expect(err).NotTo(HaveOccurred())

			fsmtest.TransitionToTopicBrowserState(mockService, componentName, topicbrowser.OperationalStateDegradedBenthos)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateDegradedBenthos, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Phase 2: Verify stability in degraded state over multiple reconcile cycles
			tick, err = fsmtest.VerifyTopicBrowserStableState(
				ctx, pkgfsm.SystemSnapshot{Tick: tick}, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateDegradedBenthos, 5)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// =========================================================================
	//  ERROR HANDLING
	// =========================================================================
	Context("Error Handling", func() {
		It("should handle S6 service not exist error correctly", func() {
			var err error

			// Setup to Creating state
			tick, err = fsmtest.TestTopicBrowserStateTransition(
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
