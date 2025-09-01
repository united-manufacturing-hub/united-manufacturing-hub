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

package fsm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/looplab/fsm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
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

		logger := logger.Sugar()
		backoffConfig := backoff.DefaultConfig(config.ID, logger)
		fsmInstance = NewBaseFSMInstance(config, backoffConfig, logger)

		tick = 0
	})

	Context("when using backoff functionality", func() {
		It("should track errors and reset correctly", func() {
			// Initially no error
			Expect(fsmInstance.GetError()).To(Succeed())
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
			Expect(fsmInstance.GetError()).To(Succeed())
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

			logger := logger.Sugar()
			backoffConfig := backoff.NewBackoffConfig(config.ID, 1, 600, 2, logger)
			specialInstance := NewBaseFSMInstance(config, backoffConfig, logger)
			// Replace the default backoffManager with one that has fewer retries for testing
			//	backoffConfig := backoff.DefaultConfig(specialInstance.cfg.ID, logger.Sugar())
			//	backoffConfig.MaxRetries = 2 // Only allow 2 retries
			//	specialInstance.backoffManager = backoff.NewBackoffManager(backoffConfig)

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
			Expect(fsmInstance.GetError()).To(Succeed())
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

	Context("when using SendEvent with different context states", func() {
		It("should reject events when context is already cancelled", func() {
			// Create a cancelled context
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			// Attempt to send an event
			err := fsmInstance.SendEvent(ctx, "start")

			// Should return the context's error
			Expect(err).To(MatchError(context.Canceled))
		})

		It("should reject events when deadline is too close", func() {
			// Create a context with a very short deadline (1ms)
			shortDeadline := time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), shortDeadline)
			defer cancel()

			// Wait to ensure we're very close to the deadline
			time.Sleep(shortDeadline / 2)

			// Attempt to send an event
			err := fsmInstance.SendEvent(ctx, "start")

			// Should return context deadline exceeded
			Expect(err).To(MatchError("context deadline exceeded"))
		})

		It("should accept events with sufficient deadline time remaining", func() {
			// Create a context with plenty of time
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Set state to a valid source state for our transition
			fsmInstance.SetCurrentFSMState("stopped")

			// Send the event - should succeed because we have a valid transition and sufficient context time
			err := fsmInstance.SendEvent(ctx, "start")

			// There should be no error since we have a valid transition and plenty of time
			Expect(err).ToNot(HaveOccurred())

			// Verify transition occurred
			Expect(fsmInstance.GetCurrentFSMState()).To(Equal("running"))
		})
	})

	Context("when using IsDeadlineExceededAndHandle", func() {
		It("should handle context deadline exceeded errors", func() {
			// Test with a context deadline exceeded error
			deadlineErr := context.DeadlineExceeded

			// Should return true for deadline exceeded (handled)
			handled := fsmInstance.IsDeadlineExceededAndHandle(deadlineErr, tick, "test operation")

			Expect(handled).To(BeTrue())

			// Should have set the error on the FSM
			Expect(fsmInstance.GetError()).To(Equal(deadlineErr))
		})

		It("should pass through non-deadline exceeded errors", func() {
			// Test with a regular error
			regularErr := errors.New("regular error")

			// Should return false for non-deadline errors (not handled)
			handled := fsmInstance.IsDeadlineExceededAndHandle(regularErr, tick, "test operation")

			Expect(handled).To(BeFalse())

			// Should not have set the error on the FSM
			Expect(fsmInstance.GetError()).To(Succeed())
		})
	})
})
