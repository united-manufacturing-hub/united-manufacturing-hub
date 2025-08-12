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

package backoff

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestBackoff(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Backoff Suite")
}

var _ = Describe("BackoffManager", func() {
	var (
		manager *BackoffManager
		config  Config
		logger  *zap.SugaredLogger
		tick    uint64
	)

	BeforeEach(func() {
		// Setup a test logger that writes to the test log
		zapLogger := zaptest.NewLogger(GinkgoT())
		logger = zapLogger.Sugar()

		// Create a config with very short but predictable intervals for testing
		// Using fixed values to make tests more reliable
		config = DefaultConfig("test-backoff-manager", logger)

		manager = NewBackoffManager(config)
		tick = 0
	})

	Context("when initializing", func() {
		It("should create a manager with default config", func() {
			defaultConfig := DefaultConfig("DefaultComponent", logger)
			defaultManager := NewBackoffManager(defaultConfig)
			Expect(defaultManager).NotTo(BeNil())
		})

		It("should create a manager with custom config", func() {
			Expect(manager).NotTo(BeNil())
		})
	})

	Context("when handling errors", func() {
		It("should track errors and reset correctly", func() {
			// Initially no error
			Expect(manager.GetLastError()).To(Succeed())
			Expect(manager.IsPermanentlyFailed()).To(BeFalse())

			// Set a temporary error
			testErr := errors.New("test error")
			isPermanent := manager.SetError(testErr, tick)
			tick++
			Expect(isPermanent).To(BeFalse()) // Not permanent on first attempt
			Expect(manager.GetLastError()).To(Equal(testErr))
			Expect(manager.IsPermanentlyFailed()).To(BeFalse())

			// Reset should clear the error state
			manager.Reset()
			Expect(manager.GetLastError()).To(Succeed())
			Expect(manager.ShouldSkipOperation(tick)).To(BeFalse())
		})

		It("should detect permanent failure after max retries", func() {
			testErr := errors.New("test error")

			// Need to access the impl directly to set exact retries
			// Create a new manager with a shorter MaxRetries for this test
			specialConfig := DefaultConfig("test-backoff-manager", logger)
			specialConfig.MaxRetries = 2
			specialManager := NewBackoffManager(specialConfig)

			// First error - not permanent
			isPermanent := specialManager.SetError(testErr, tick)
			tick++
			Expect(isPermanent).To(BeFalse(), "First error should not be permanent")
			Expect(specialManager.IsPermanentlyFailed()).To(BeFalse())

			// Second error - not permanent yet (since we allow 2 retries)
			isPermanent = specialManager.SetError(testErr, tick)
			tick++
			Expect(isPermanent).To(BeFalse(), "Second error should not be permanent")
			Expect(specialManager.IsPermanentlyFailed()).To(BeFalse())

			// Third error - now should be permanent (exceeded 2 retries)
			isPermanent = specialManager.SetError(testErr, tick)
			tick++
			Expect(isPermanent).To(BeTrue(), "Third error should be permanent")
			Expect(specialManager.IsPermanentlyFailed()).To(BeTrue())

			// Check that we get a permanent failure error
			backoffErr := specialManager.GetBackoffError(tick)
			Expect(IsPermanentFailureError(backoffErr)).To(BeTrue(), "Error should indicate permanent failure")
			Expect(IsTemporaryBackoffError(backoffErr)).To(BeFalse(), "Error should not indicate temporary failure")
		})
	})

	Context("when checking operation skip", func() {
		It("should not skip operations initially", func() {
			Expect(manager.ShouldSkipOperation(tick)).To(BeFalse())
		})

		It("should skip operations in backoff state", func() {
			// Set an error to enter backoff state
			testErr := errors.New("test error")
			manager.SetError(testErr, tick)

			// Now operations should be skipped
			Expect(manager.ShouldSkipOperation(tick)).To(BeTrue())
			tick++
		})

		It("should not skip operations after reset", func() {
			// Set an error to enter backoff state
			testErr := errors.New("test error")
			manager.SetError(testErr, tick)
			Expect(manager.ShouldSkipOperation(tick)).To(BeTrue())

			// Reset and check again
			manager.Reset()
			Expect(manager.ShouldSkipOperation(tick)).To(BeFalse())
			tick++
		})
	})

	Context("when working with backoff errors", func() {
		It("should generate appropriate temporary backoff errors", func() {
			// Set an error but not permanently failed
			testErr := errors.New("test error")
			manager.SetError(testErr, tick)

			backoffErr := manager.GetBackoffError(tick)
			tick++
			Expect(IsTemporaryBackoffError(backoffErr)).To(BeTrue())
			Expect(IsPermanentFailureError(backoffErr)).To(BeFalse())
		})

		It("should generate appropriate permanent backoff errors", func() {
			// Need to use a manager with a specific number of retries for reliable testing
			specialConfig := DefaultConfig("test-backoff-manager", logger)
			specialConfig.MaxRetries = 2
			specialManager := NewBackoffManager(specialConfig)

			testErr := errors.New("test error")

			// Set errors until we reach permanent failure
			// First error
			specialManager.SetError(testErr, tick)
			tick++
			// Second error
			specialManager.SetError(testErr, tick)
			tick++
			// Third error should cause permanent failure
			isPermanent := specialManager.SetError(testErr, tick)
			tick++
			// Verify permanent failure
			Expect(isPermanent).To(BeTrue(), "Should be permanent after exceeding max retries")
			Expect(specialManager.IsPermanentlyFailed()).To(BeTrue())

			// Get error and verify it's a permanent failure error
			backoffErr := specialManager.GetBackoffError(tick)
			tick++
			Expect(IsPermanentFailureError(backoffErr)).To(BeTrue(), "Error should be permanent failure type")
			Expect(IsTemporaryBackoffError(backoffErr)).To(BeFalse(), "Error should not be temporary")
		})

		It("should preserve original error", func() {
			testErr := errors.New("original test error")
			manager.SetError(testErr, tick)
			backoffErr := manager.GetBackoffError(tick)
			extractedErr := ExtractOriginalError(backoffErr)
			Expect(extractedErr).To(Equal(testErr))
		})
	})

	Context("when using error helpers", func() {
		It("should correctly identify temporary backoff errors", func() {
			tempErr := errors.New(TemporaryBackoffError + ": test")
			Expect(IsTemporaryBackoffError(tempErr)).To(BeTrue())
			Expect(IsPermanentFailureError(tempErr)).To(BeFalse())
			Expect(IsBackoffError(tempErr)).To(BeTrue())
		})

		It("should correctly identify permanent failure errors", func() {
			permErr := errors.New(PermanentFailureError + ": test")
			Expect(IsTemporaryBackoffError(permErr)).To(BeFalse())
			Expect(IsPermanentFailureError(permErr)).To(BeTrue())
			Expect(IsBackoffError(permErr)).To(BeTrue())
		})

		It("should not identify regular errors as backoff errors", func() {
			regularErr := errors.New("regular error")
			Expect(IsTemporaryBackoffError(regularErr)).To(BeFalse())
			Expect(IsPermanentFailureError(regularErr)).To(BeFalse())
			Expect(IsBackoffError(regularErr)).To(BeFalse())
		})
	})

	Context("integration with time-based backoff", func() {
		It("should respect backoff delay", func() {
			// We need a simpler test that doesn't depend on complex timing
			// Create a fresh manager for this test
			testManager := NewBackoffManager(config)

			// Set an error to trigger backoff
			testErr := errors.New("test error")
			testManager.SetError(testErr, tick)
			// Should be in backoff immediately after setting error
			Expect(testManager.ShouldSkipOperation(tick)).To(BeTrue())
			tick++

			// Reset the manager to clear backoff state
			testManager.Reset()

			// Should not be in backoff after reset
			Expect(testManager.ShouldSkipOperation(tick)).To(BeFalse())
		})
	})

	Context("when using backoff manager in FSM", func() {
		It("should calculate reasonable backoff intervals", func() {
			// First error
			manager.SetError(errors.New("error1"), 0)
			Expect(manager.suspendedUntilTick).To(Equal(uint64(1)))

			// Second error
			manager.SetError(errors.New("error2"), 1)
			Expect(manager.suspendedUntilTick).To(BeNumerically("~", 2, 3))
		})
	})
})
