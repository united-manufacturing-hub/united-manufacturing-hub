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

package s6_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var _ = Describe("S6Instance FSM", func() {
	var (
		ctx             context.Context
		testBaseDir     string
		tick            uint64
		mockSvcRegistry *serviceregistry.Registry
	)

	BeforeEach(func() {
		ctx = context.Background()
		testBaseDir = constants.S6BaseDir
		tick = 0
		mockSvcRegistry = serviceregistry.NewMockRegistry()
	})

	// -------------------------------------------------------------------------
	//  CREATION FLOW
	// -------------------------------------------------------------------------
	Context("Creation Flow", func() {

		It("should transition from to_be_created to stopped (normal creation)", func() {
			// 1. Create a new S6Instance in the "to_be_created" phase
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "creation-success", s6fsm.OperationalStateStopped)

			// 2. Check it initially
			Expect(instance.GetCurrentFSMState()).To(Equal(internal_fsm.LifecycleStateToBeCreated))

			// 3. Transition from to_be_created => creating => stopped
			snapshot := fsm.SystemSnapshot{Tick: tick}
			tick, err := fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateToBeCreated,
				internal_fsm.LifecycleStateCreating,
				5,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.CreateCalled).To(BeTrue())

			snapshot = fsm.SystemSnapshot{Tick: tick}
			_, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateCreating,
				s6fsm.OperationalStateStopped,
				5,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})

		It("should handle creation failure and retry", func() {
			// 1. Set up an instance that wants to end in stopped
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "creation-failure", s6fsm.OperationalStateStopped)

			// 2. Force the creation to fail initially
			mockService.CreateError = fmt.Errorf("simulated create failure")

			// 3. Verify it remains in to_be_created despite multiple reconciles
			snapshot := fsm.SystemSnapshot{Tick: tick}
			tick, err := fsmtest.VerifyStableState(ctx, snapshot, instance, mockSvcRegistry, internal_fsm.LifecycleStateToBeCreated, 3)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetError()).ToNot(BeNil()) // Should record the error

			// 4. Fix the error and ensure we eventually get to "stopped"
			mockService.CreateError = nil
			mockService.CreateCalled = false

			// from to_be_created => creating
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateToBeCreated,
				internal_fsm.LifecycleStateCreating,
				10,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.CreateCalled).To(BeTrue())

			// from creating => stopped
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateCreating,
				s6fsm.OperationalStateStopped,
				10,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})

		It("should handle permanent creation failure (self-removal or remain to_be_created)", func() {
			// Implementation depends on your FSM design. Possibly:
			// - If the FSM sees a "PermanentFailureError" during creation, it
			//   transitions to a special removed state or remains in to_be_created
			//   but sets a permanent error, etc.
			// Insert your exact logic here.
			// e.g.:
			// instance, mockService, _ := fsmtest.SetupS6Instance(...)

			// mockService.CreateError = fmt.Errorf("%s: unrecoverable error", backoff.PermanentFailureError)
			// ...
			// Expect final result. Possibly the instance removes itself or remains in error.
		})
	})

	// -------------------------------------------------------------------------
	//  STARTING FLOW
	// -------------------------------------------------------------------------
	Context("Starting Flow", func() {

		It("should transition from stopped to running", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "start-normal", s6fsm.OperationalStateStopped)

			// from to_be_created => stopped
			snapshot := fsm.SystemSnapshot{Tick: tick}
			tick, err := fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateStopped,
				5,
			)
			Expect(err).NotTo(HaveOccurred())

			// Now set desired = running
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateRunning)
			Expect(err).NotTo(HaveOccurred())

			// from stopped => starting => running
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				s6fsm.OperationalStateStopped,
				s6fsm.OperationalStateRunning,
				10,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateRunning))
		})

		It("should remain in starting until service is actually up", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "start-slow", s6fsm.OperationalStateStopped)

			// from to_be_created => stopped
			snapshot := fsm.SystemSnapshot{Tick: tick}
			tick, err := fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateStopped,
				5,
			)
			Expect(err).NotTo(HaveOccurred())

			// desired=running
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateRunning)
			Expect(err).NotTo(HaveOccurred())

			// from stopped => starting
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				s6fsm.OperationalStateStopped,
				s6fsm.OperationalStateStarting,
				5,
			)
			Expect(err).NotTo(HaveOccurred())

			// Keep the service in "down" or "restarting" so it doesn't reach running
			mockService.ServiceStates[instance.GetServicePath()] = s6service.ServiceInfo{
				Status: s6service.ServiceRestarting,
			}

			// Verify it remains in "starting"
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyStableState(ctx, snapshot, instance, mockSvcRegistry, s6fsm.OperationalStateStarting, 3)
			Expect(err).NotTo(HaveOccurred())

			// Finally let the service become up => instance can go to running
			// e.g. mockService.ServiceStates[...] = s6service.ServiceInfo{Status: s6service.ServiceUp}
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				s6fsm.OperationalStateStarting,
				s6fsm.OperationalStateRunning,
				5,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateRunning))
		})

		It("should handle start failure and retry after backoff", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "start-fail", s6fsm.OperationalStateStopped)

			// from to_be_created => stopped
			snapshot := fsm.SystemSnapshot{Tick: tick}
			tick, err := fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateStopped,
				5,
			)
			Expect(err).NotTo(HaveOccurred())

			// Configure start error
			mockService.StartError = fmt.Errorf("simulated start failure")

			// desired=running
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateRunning)
			Expect(err).NotTo(HaveOccurred())

			// Verify it remains in stopped due to start error
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyStableState(ctx, snapshot, instance, mockSvcRegistry, s6fsm.OperationalStateStopped, 3)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetError()).NotTo(BeNil()) // Should record the error
			Expect(mockService.StartCalled).To(BeTrue())

			// Fix the error
			mockService.StartError = nil
			snapshot = fsm.SystemSnapshot{Tick: tick}
			fsmtest.ResetInstanceError(instance, snapshot, mockSvcRegistry)
			mockService.StartCalled = false

			// from stopped => starting
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				s6fsm.OperationalStateStopped,
				s6fsm.OperationalStateStarting,
				5,
			)
			Expect(err).NotTo(HaveOccurred())

			// from starting => running
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				s6fsm.OperationalStateStarting,
				s6fsm.OperationalStateRunning,
				5,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateRunning))
		})
	})

	// -------------------------------------------------------------------------
	//  STOPPING FLOW
	// -------------------------------------------------------------------------
	Context("Stopping Flow", func() {

		It("should transition from running to stopped", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "stop-normal", s6fsm.OperationalStateRunning)

			// from to_be_created => running
			snapshot := fsm.SystemSnapshot{Tick: tick}
			tick, err := fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateRunning,
				8,
			)
			Expect(err).NotTo(HaveOccurred())

			// from running => stopped
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				s6fsm.OperationalStateRunning,
				s6fsm.OperationalStateStopped,
				8,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StopCalled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})

		It("should remain in stopping until service is actually down", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "stop-slow", s6fsm.OperationalStateRunning)

			// from to_be_created => running
			snapshot := fsm.SystemSnapshot{Tick: tick}
			tick, err := fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateRunning,
				8,
			)
			Expect(err).NotTo(HaveOccurred())

			// desired=stopped => instance goes from running => stopping
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				s6fsm.OperationalStateRunning,
				s6fsm.OperationalStateStopping,
				5,
			)
			Expect(err).NotTo(HaveOccurred())

			// Keep service in "up" => instance remains stopping
			mockService.ServiceStates[instance.GetServicePath()] = s6service.ServiceInfo{
				Status: s6service.ServiceUp,
			}

			// Verify stable in stopping
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyStableState(ctx, snapshot, instance, mockSvcRegistry, s6fsm.OperationalStateStopping, 3)
			Expect(err).NotTo(HaveOccurred())

			// Finally let service go down => instance => stopped
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				s6fsm.OperationalStateStopping,
				s6fsm.OperationalStateStopped,
				5,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})

		It("should handle stop failure and retry after backoff", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "stop-fail", s6fsm.OperationalStateRunning)

			// from to_be_created => running
			snapshot := fsm.SystemSnapshot{Tick: tick}
			tick, err := fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateRunning,
				8,
			)
			Expect(err).NotTo(HaveOccurred())

			// Inject a stop error
			mockService.StopError = fmt.Errorf("simulated stop failure")

			// desired=stopped
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			// Verify it stays in running due to failure
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyStableState(ctx, snapshot, instance, mockSvcRegistry, s6fsm.OperationalStateRunning, 3)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetError()).NotTo(BeNil())
			Expect(mockService.StopCalled).To(BeTrue())

			// Fix the error
			mockService.StopError = nil
			snapshot = fsm.SystemSnapshot{Tick: tick}
			fsmtest.ResetInstanceError(instance, snapshot, mockSvcRegistry)
			mockService.StopCalled = false

			// from running => stopping
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				s6fsm.OperationalStateRunning,
				s6fsm.OperationalStateStopping,
				5,
			)
			Expect(err).NotTo(HaveOccurred())

			// from stopping => stopped
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				s6fsm.OperationalStateStopping,
				s6fsm.OperationalStateStopped,
				5,
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -------------------------------------------------------------------------
	//  ERROR & BACKOFF FLOW
	// -------------------------------------------------------------------------
	Context("Error & Backoff Handling", func() {

		It("should apply backoff when operations fail repeatedly", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "backoff-fail", s6fsm.OperationalStateStopped)

			// from to_be_created => stopped
			snapshot := fsm.SystemSnapshot{Tick: tick}
			tick, err := fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateStopped,
				5,
			)
			Expect(err).NotTo(HaveOccurred())

			// cause repeated start failures
			mockService.StartError = fmt.Errorf("simulated start failure")

			// desired=running
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateRunning)
			Expect(err).NotTo(HaveOccurred())

			// Verify it remains in stopped + eventually hits backoff
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyStableState(ctx, snapshot, instance, mockSvcRegistry, s6fsm.OperationalStateStopped, 100)
			Expect(err).To(HaveOccurred()) // should have hit permanent failure
			Expect(mockService.StartCalled).To(BeTrue())
			Expect(instance.GetError()).NotTo(BeNil())

			// Check backoff is active by verifying no further start calls
			mockService.StartCalled = false
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyStableState(ctx, snapshot, instance, mockSvcRegistry, s6fsm.OperationalStateStopped, 3)
			Expect(err).To(HaveOccurred()) // should have hit permanent failure
			Expect(mockService.StartCalled).To(BeFalse(), "Should not re-attempt immediately due to backoff")
		})

		It("should reset backoff after a successful operation", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "backoff-reset", s6fsm.OperationalStateStopped)

			// from to_be_created => stopped
			snapshot := fsm.SystemSnapshot{Tick: tick}
			tick, err := fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateStopped,
				5,
			)
			Expect(err).NotTo(HaveOccurred())

			// Force start error
			mockService.StartError = fmt.Errorf("failing start")
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateRunning)
			Expect(err).NotTo(HaveOccurred())

			// verify stable in stopped due to repeated start errors
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyStableState(ctx, snapshot, instance, mockSvcRegistry, s6fsm.OperationalStateStopped, 5)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeTrue())
			Expect(instance.GetError()).NotTo(BeNil())

			// confirm backoff is active
			mockService.StartCalled = false
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.VerifyStableState(ctx, snapshot, instance, mockSvcRegistry, s6fsm.OperationalStateStopped, 2)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeFalse())

			// fix the error
			mockService.StartError = nil
			snapshot = fsm.SystemSnapshot{Tick: tick}
			fsmtest.ResetInstanceError(instance, snapshot, mockSvcRegistry)
			mockService.StartCalled = false

			// now from stopped => running
			snapshot = fsm.SystemSnapshot{Tick: tick}
			tick, err = fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				s6fsm.OperationalStateStopped,
				s6fsm.OperationalStateRunning,
				10,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeTrue())

		})

		It("should call forceRemoval when not in a terminal state", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "backoff-notTerminal", s6fsm.OperationalStateStopped)

			// from to_be_created => stopped
			snapshot := fsm.SystemSnapshot{Tick: tick}
			tick, err := fsmtest.TestS6StateTransition(ctx, snapshot, instance, mockSvcRegistry,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateStopped,
				5,
			)
			Expect(err).NotTo(HaveOccurred())

			// Set desired state to running to trigger start operations
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateRunning)
			Expect(err).NotTo(HaveOccurred())

			// Create a permanent error that will be encountered during state transition (StartError)
			// Note: StatusError is no longer used here because the S6 FSM continues reconciling
			// even when observed state updates fail (to prevent deadlocks in re-creation)
			mockService.StartError = fmt.Errorf("%s: test permanent error", backoff.PermanentFailureError)

			recErr, reconciled := fsmtest.ReconcileS6UntilError(ctx, fsm.SystemSnapshot{Tick: tick}, instance, mockSvcRegistry, 100)
			Expect(recErr).To(HaveOccurred())
			Expect(reconciled).To(BeTrue())

			Expect(mockService.ForceRemoveCalled).To(BeTrue())
		})
	})
})
