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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	pkgfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
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
		componentName = "test-topic-browser-fsm"
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
				dataflowcomponent.OperationalStateStopped, 5, tick)
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
				dataflowcomponent.OperationalStateStopped, 5, tick)
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
				dataflowcomponent.OperationalStateStopped, 5, tick)
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

			// Execute transition: Idle -> Degraded
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateDegraded,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// Execute transition: Degraded -> Idle
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateDegraded,
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
				dataflowcomponent.OperationalStateStopped, 5, tick)
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
				dataflowcomponent.OperationalStateStopped, 5, tick)
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

			// Execute transition: Active -> Degraded
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateDegraded,
				5,
				tick,
			)
			Expect(err).NotTo(HaveOccurred())

			// 3. Now set desired state to Stopped
			Expect(instance.SetDesiredFSMState(topicbrowser.OperationalStateStopped)).To(Succeed())

			// Execute transition: Degraded -> Stopping
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx,
				instance,
				mockService,
				mockSvcRegistry,
				componentName,
				topicbrowser.OperationalStateDegraded,
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
				dataflowcomponent.OperationalStateStopped, 5, tick)
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
				dataflowcomponent.OperationalStateStopped, 5, tick)
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

			// Execute transition: Active -> Degraded
			fsmtest.TransitionToTopicBrowserState(mockService, componentName,
				topicbrowser.OperationalStateDegraded)

			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateActive,
				topicbrowser.OperationalStateDegraded,
				5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Can now get Active again

			// Execute transition: Degraded -> Active
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				topicbrowser.OperationalStateDegraded,
				topicbrowser.OperationalStateActive,
				5, tick)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should force-remove component on permanent error when already Stopped", func() {
			// Setup to Active state
			// 1. First get to Stopped state
			tick, err := fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateToBeCreated,
				fsm.LifecycleStateCreating, 5, tick)
			Expect(err).NotTo(HaveOccurred())

			// Creating → Stopped
			tick, err = fsmtest.TestTopicBrowserStateTransition(
				ctx, instance, mockService, mockSvcRegistry, componentName,
				fsm.LifecycleStateCreating,
				dataflowcomponent.OperationalStateStopped, 5, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.AddToManagerCalled).To(BeTrue())

			// inject permanent error
			mockService.StatusError = fmt.Errorf("%s: simulated", backoff.PermanentFailureError)

			tick, recErr, reconciled := fsmtest.ReconcileTopicBrowserUntilError(
				ctx, pkgfsm.SystemSnapshot{Tick: tick},
				instance, mockService, mockSvcRegistry,
				componentName, 5)

			Expect(recErr).To(HaveOccurred())
			Expect(recErr.Error()).To(ContainSubstring(backoff.PermanentFailureError))
			Expect(reconciled).To(BeTrue())
			Expect(mockService.ForceRemoveCalled).To(BeTrue())

			mockService.StatusError = nil // cleanup for other tests
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

})
