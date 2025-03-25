package fsm

import (
	"errors"
	"testing"

	"github.com/looplab/fsm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/backoff"
)

func TestBaseFSM(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BaseFSM Suite")
}

var _ = Describe("BaseFSMInstance", func() {
	var (
		fsmInstance *BaseFSMInstance
		logger      *zap.Logger
		tick        uint64
	)

	BeforeEach(func() {
		logger = zaptest.NewLogger(GinkgoT())

		// Create a basic FSM instance configuration
		config := BaseFSMInstanceConfig{
			ID:                           "test-fsm",
			DesiredFSMState:              "running",
			OperationalStateAfterCreate:  "running",
			OperationalStateBeforeRemove: "stopped",
			OperationalTransitions: []fsm.EventDesc{
				{Name: "start", Src: []string{"stopped"}, Dst: "running"},
				{Name: "stop", Src: []string{"running"}, Dst: "stopped"},
			},
		}

		fsmInstance = NewBaseFSMInstance(config, logger.Sugar())

		tick = 0
	})

	Context("when using backoff functionality", func() {
		It("should track errors and reset correctly", func() {
			// Initially no error
			Expect(fsmInstance.GetError()).To(BeNil())
			Expect(fsmInstance.IsPermanentlyFailed()).To(BeFalse())

			// Set a temporary error
			testErr := errors.New("test error")
			isPermanent := fsmInstance.SetError(testErr, tick)
			Expect(isPermanent).To(BeFalse(), "First error should not be permanent")
			Expect(fsmInstance.GetError()).To(Equal(testErr))

			// Should skip operations in backoff state
			Expect(fsmInstance.ShouldSkipReconcileBecauseOfError(tick)).To(BeTrue())

			// Reset the state
			fsmInstance.ResetState()
			Expect(fsmInstance.GetError()).To(BeNil())
			Expect(fsmInstance.ShouldSkipReconcileBecauseOfError(tick)).To(BeFalse())
		})

		It("should detect permanent failure after max retries", func() {
			// Need to create a special instance with a low max retry count for testing
			config := BaseFSMInstanceConfig{
				ID:                           "test-fsm-retry",
				DesiredFSMState:              "running",
				OperationalStateAfterCreate:  "running",
				OperationalStateBeforeRemove: "stopped",
			}

			specialInstance := NewBaseFSMInstance(config, logger.Sugar())

			// Replace the default backoffManager with one that has fewer retries for testing
			backoffConfig := backoff.DefaultConfig(specialInstance.cfg.ID, logger.Sugar())
			backoffConfig.MaxRetries = 2 // Only allow 2 retries
			specialInstance.backoffManager = backoff.NewBackoffManager(backoffConfig)

			testErr := errors.New("test error")

			// First error - should not be permanent
			isPermanent := specialInstance.SetError(testErr, tick)
			tick++
			Expect(isPermanent).To(BeFalse(), "First error should not be permanent")
			Expect(specialInstance.IsPermanentlyFailed()).To(BeFalse())

			// Second error - still not permanent
			isPermanent = specialInstance.SetError(testErr, tick)
			tick++
			Expect(isPermanent).To(BeFalse(), "Second error should not be permanent")
			Expect(specialInstance.IsPermanentlyFailed()).To(BeFalse())

			// Third error - now should be permanent (exceeding MaxRetries of 2)
			isPermanent = specialInstance.SetError(testErr, tick)
			tick++
			Expect(isPermanent).To(BeTrue(), "Third error should be permanent")
			Expect(specialInstance.IsPermanentlyFailed()).To(BeTrue())

			// Check error type
			backoffErr := specialInstance.GetBackoffError(tick)
			Expect(backoff.IsPermanentFailureError(backoffErr)).To(BeTrue())
		})

		It("should generate appropriate backoff errors", func() {
			testErr := errors.New("reconcile failed")
			fsmInstance.SetError(testErr, tick)

			// Get the backoff error and verify its properties
			backoffErr := fsmInstance.GetBackoffError(tick)
			Expect(backoff.IsTemporaryBackoffError(backoffErr)).To(BeTrue())

			// Extract the original error
			originalErr := backoff.ExtractOriginalError(backoffErr)
			Expect(originalErr).To(Equal(testErr))
		})

		It("should clear errors with ResetState", func() {
			testErr := errors.New("test error")
			fsmInstance.SetError(testErr, tick)

			// Verify error is set
			Expect(fsmInstance.GetError()).To(Equal(testErr))
			Expect(fsmInstance.ShouldSkipReconcileBecauseOfError(tick)).To(BeTrue())

			// Reset the state
			fsmInstance.ResetState()

			// Verify error is cleared
			Expect(fsmInstance.GetError()).To(BeNil())
			Expect(fsmInstance.ShouldSkipReconcileBecauseOfError(tick)).To(BeFalse())
		})
	})

	Context("when working with the FSM", func() {
		It("should maintain desired and current states", func() {
			// Check initial state
			Expect(fsmInstance.GetCurrentFSMState()).To(Equal(LifecycleStateToBeCreated))
			Expect(fsmInstance.GetDesiredFSMState()).To(Equal("running"))

			// Set a new desired state
			fsmInstance.SetDesiredFSMState("stopped")
			Expect(fsmInstance.GetDesiredFSMState()).To(Equal("stopped"))
		})
	})
})
