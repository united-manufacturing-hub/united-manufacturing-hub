// file: s6_instance_test.go
package s6_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internal_fsm "github.com/united-manufacturing-hub/benthos-umh/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/internal/fsmtest"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/constants"
	s6fsm "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/fsm/s6"
	s6service "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/service/s6"
)

var _ = Describe("S6Instance FSM", func() {
	var (
		ctx         context.Context
		testBaseDir string
		tick        uint64
	)

	BeforeEach(func() {
		ctx = context.Background()
		testBaseDir = constants.S6BaseDir
		tick = 0
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
			nextTick, err := fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateToBeCreated,
				internal_fsm.LifecycleStateCreating,
				5, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.CreateCalled).To(BeTrue())

			_, err = fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateCreating,
				s6fsm.OperationalStateStopped,
				5, tick,
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
			_, err := fsmtest.VerifyStableState(ctx, instance, internal_fsm.LifecycleStateToBeCreated, 3, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetError()).ToNot(BeNil()) // Should record the error

			// 4. Fix the error and ensure we eventually get to "stopped"
			mockService.CreateError = nil
			mockService.CreateCalled = false

			// from to_be_created => creating
			nextTick, err := fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateToBeCreated,
				internal_fsm.LifecycleStateCreating,
				10, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.CreateCalled).To(BeTrue())

			// from creating => stopped
			_, err = fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateCreating,
				s6fsm.OperationalStateStopped,
				10, tick,
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
			nextTick, err := fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateStopped,
				5, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Now set desired = running
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateRunning)
			Expect(err).NotTo(HaveOccurred())

			// from stopped => starting => running
			_, err = fsmtest.TestS6StateTransition(ctx, instance,
				s6fsm.OperationalStateStopped,
				s6fsm.OperationalStateRunning,
				10, tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateRunning))
		})

		It("should remain in starting until service is actually up", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "start-slow", s6fsm.OperationalStateStopped)

			// from to_be_created => stopped
			nextTick, err := fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateStopped,
				5, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// desired=running
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateRunning)
			Expect(err).NotTo(HaveOccurred())

			// from stopped => starting
			nextTick, err = fsmtest.TestS6StateTransition(ctx, instance,
				s6fsm.OperationalStateStopped,
				s6fsm.OperationalStateStarting,
				5, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Keep the service in "down" or "restarting" so it doesn't reach running
			mockService.ServiceStates[instance.GetServicePath()] = s6service.ServiceInfo{
				Status: s6service.ServiceRestarting,
			}

			// Verify it remains in "starting"
			_, err = fsmtest.VerifyStableState(ctx, instance, s6fsm.OperationalStateStarting, 3, tick)
			Expect(err).NotTo(HaveOccurred())

			// Finally let the service become up => instance can go to running
			// e.g. mockService.ServiceStates[...] = s6service.ServiceInfo{Status: s6service.ServiceUp}
			_, err = fsmtest.TestS6StateTransition(ctx, instance,
				s6fsm.OperationalStateStarting,
				s6fsm.OperationalStateRunning,
				5, tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateRunning))
		})

		It("should handle start failure and retry after backoff", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "start-fail", s6fsm.OperationalStateStopped)

			// from to_be_created => stopped
			nextTick, err := fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateStopped,
				5, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Configure start error
			mockService.StartError = fmt.Errorf("simulated start failure")

			// desired=running
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateRunning)
			Expect(err).NotTo(HaveOccurred())

			// Verify it remains in stopped due to start error
			nextTick, err = fsmtest.VerifyStableState(ctx, instance, s6fsm.OperationalStateStopped, 3, tick)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetError()).NotTo(BeNil()) // Should record the error
			Expect(mockService.StartCalled).To(BeTrue())

			// Fix the error
			mockService.StartError = nil
			fsmtest.ResetInstanceError(instance)
			mockService.StartCalled = false

			// from stopped => starting
			nextTick, err = fsmtest.TestS6StateTransition(ctx, instance,
				s6fsm.OperationalStateStopped,
				s6fsm.OperationalStateStarting,
				5, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// from starting => running
			_, err = fsmtest.TestS6StateTransition(ctx, instance,
				s6fsm.OperationalStateStarting,
				s6fsm.OperationalStateRunning,
				5, tick,
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
			nextTick, err := fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateRunning,
				8, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// from running => stopped
			_, err = fsmtest.TestS6StateTransition(ctx, instance,
				s6fsm.OperationalStateRunning,
				s6fsm.OperationalStateStopped,
				8, tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StopCalled).To(BeTrue())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})

		It("should remain in stopping until service is actually down", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "stop-slow", s6fsm.OperationalStateRunning)

			// from to_be_created => running
			nextTick, err := fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateRunning,
				8, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// desired=stopped => instance goes from running => stopping
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			nextTick, err = fsmtest.TestS6StateTransition(ctx, instance,
				s6fsm.OperationalStateRunning,
				s6fsm.OperationalStateStopping,
				5, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Keep service in "up" => instance remains stopping
			mockService.ServiceStates[instance.GetServicePath()] = s6service.ServiceInfo{
				Status: s6service.ServiceUp,
			}

			// Verify stable in stopping
			_, err = fsmtest.VerifyStableState(ctx, instance, s6fsm.OperationalStateStopping, 3, tick)
			Expect(err).NotTo(HaveOccurred())

			// Finally let service go down => instance => stopped
			_, err = fsmtest.TestS6StateTransition(ctx, instance,
				s6fsm.OperationalStateStopping,
				s6fsm.OperationalStateStopped,
				5, tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetCurrentFSMState()).To(Equal(s6fsm.OperationalStateStopped))
		})

		It("should handle stop failure and retry after backoff", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "stop-fail", s6fsm.OperationalStateRunning)

			// from to_be_created => running
			nextTick, err := fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateRunning,
				8, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Inject a stop error
			mockService.StopError = fmt.Errorf("simulated stop failure")

			// desired=stopped
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateStopped)
			Expect(err).NotTo(HaveOccurred())

			// Verify it stays in running due to failure
			nextTick, err = fsmtest.VerifyStableState(ctx, instance, s6fsm.OperationalStateRunning, 3, tick)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())
			Expect(instance.GetError()).NotTo(BeNil())
			Expect(mockService.StopCalled).To(BeTrue())

			// Fix the error
			mockService.StopError = nil
			fsmtest.ResetInstanceError(instance)
			mockService.StopCalled = false

			// from running => stopping
			nextTick, err = fsmtest.TestS6StateTransition(ctx, instance,
				s6fsm.OperationalStateRunning,
				s6fsm.OperationalStateStopping,
				5, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// from stopping => stopped
			_, err = fsmtest.TestS6StateTransition(ctx, instance,
				s6fsm.OperationalStateStopping,
				s6fsm.OperationalStateStopped,
				5, tick,
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
			nextTick, err := fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateStopped,
				5, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// cause repeated start failures
			mockService.StartError = fmt.Errorf("simulated start failure")

			// desired=running
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateRunning)
			Expect(err).NotTo(HaveOccurred())

			// Verify it remains in stopped + eventually hits backoff
			nextTick, err = fsmtest.VerifyStableState(ctx, instance, s6fsm.OperationalStateStopped, 100, tick)
			tick = nextTick
			Expect(err).To(HaveOccurred()) // should have hit permanent failure
			Expect(mockService.StartCalled).To(BeTrue())
			Expect(instance.GetError()).NotTo(BeNil())

			// Check backoff is active by verifying no further start calls
			mockService.StartCalled = false
			_, err = fsmtest.VerifyStableState(ctx, instance, s6fsm.OperationalStateStopped, 3, tick)
			Expect(err).To(HaveOccurred()) // should have hit permanent failure
			Expect(mockService.StartCalled).To(BeFalse(), "Should not re-attempt immediately due to backoff")
		})

		It("should reset backoff after a successful operation", func() {
			instance, mockService, _ := fsmtest.SetupS6Instance(testBaseDir, "backoff-reset", s6fsm.OperationalStateStopped)

			// from to_be_created => stopped
			nextTick, err := fsmtest.TestS6StateTransition(ctx, instance,
				internal_fsm.LifecycleStateToBeCreated,
				s6fsm.OperationalStateStopped,
				5, tick,
			)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())

			// Force start error
			mockService.StartError = fmt.Errorf("failing start")
			err = instance.SetDesiredFSMState(s6fsm.OperationalStateRunning)
			Expect(err).NotTo(HaveOccurred())

			// verify stable in stopped due to repeated start errors
			nextTick, err = fsmtest.VerifyStableState(ctx, instance, s6fsm.OperationalStateStopped, 5, tick)
			tick = nextTick
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeTrue())
			Expect(instance.GetError()).NotTo(BeNil())

			// confirm backoff is active
			mockService.StartCalled = false
			_, err = fsmtest.VerifyStableState(ctx, instance, s6fsm.OperationalStateStopped, 2, tick)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeFalse())

			// fix the error
			mockService.StartError = nil
			fsmtest.ResetInstanceError(instance)
			mockService.StartCalled = false

			// now from stopped => running
			_, err = fsmtest.TestS6StateTransition(ctx, instance,
				s6fsm.OperationalStateStopped,
				s6fsm.OperationalStateRunning,
				10, tick,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockService.StartCalled).To(BeTrue())

		})
	})
})
