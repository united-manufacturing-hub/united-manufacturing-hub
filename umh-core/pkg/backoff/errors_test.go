package backoff_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/backoff"
)

var _ = Describe("Error Helpers", func() {
	Context("when checking error types", func() {
		It("should correctly identify temporary backoff errors", func() {
			// Direct error with constant
			tempErr := errors.New(backoff.TemporaryBackoffError)
			Expect(backoff.IsTemporaryBackoffError(tempErr)).To(BeTrue())
			Expect(backoff.IsPermanentFailureError(tempErr)).To(BeFalse())
			Expect(backoff.IsBackoffError(tempErr)).To(BeTrue())

			// Error with additional text
			tempWithMsg := errors.New(backoff.TemporaryBackoffError + ": system busy")
			Expect(backoff.IsTemporaryBackoffError(tempWithMsg)).To(BeTrue())
			Expect(backoff.IsPermanentFailureError(tempWithMsg)).To(BeFalse())
			Expect(backoff.IsBackoffError(tempWithMsg)).To(BeTrue())

			// Wrapped error
			origErr := errors.New("connection refused")
			wrappedErr := fmt.Errorf("%s: %w", backoff.TemporaryBackoffError, origErr)
			Expect(backoff.IsTemporaryBackoffError(wrappedErr)).To(BeTrue())
			Expect(backoff.IsPermanentFailureError(wrappedErr)).To(BeFalse())
			Expect(backoff.IsBackoffError(wrappedErr)).To(BeTrue())
		})

		It("should correctly identify permanent failure errors", func() {
			// Direct error with constant
			permErr := errors.New(backoff.PermanentFailureError)
			Expect(backoff.IsTemporaryBackoffError(permErr)).To(BeFalse())
			Expect(backoff.IsPermanentFailureError(permErr)).To(BeTrue())
			Expect(backoff.IsBackoffError(permErr)).To(BeTrue())

			// Error with additional text
			permWithMsg := errors.New(backoff.PermanentFailureError + ": max retries exceeded")
			Expect(backoff.IsTemporaryBackoffError(permWithMsg)).To(BeFalse())
			Expect(backoff.IsPermanentFailureError(permWithMsg)).To(BeTrue())
			Expect(backoff.IsBackoffError(permWithMsg)).To(BeTrue())

			// Wrapped error
			origErr := errors.New("invalid configuration")
			wrappedErr := fmt.Errorf("%s: %w", backoff.PermanentFailureError, origErr)
			Expect(backoff.IsTemporaryBackoffError(wrappedErr)).To(BeFalse())
			Expect(backoff.IsPermanentFailureError(wrappedErr)).To(BeTrue())
			Expect(backoff.IsBackoffError(wrappedErr)).To(BeTrue())
		})

		It("should handle mixed and special cases", func() {
			// Nil error
			Expect(backoff.IsTemporaryBackoffError(nil)).To(BeFalse())
			Expect(backoff.IsPermanentFailureError(nil)).To(BeFalse())
			Expect(backoff.IsBackoffError(nil)).To(BeFalse())

			// Regular error
			regularErr := errors.New("just a normal error")
			Expect(backoff.IsTemporaryBackoffError(regularErr)).To(BeFalse())
			Expect(backoff.IsPermanentFailureError(regularErr)).To(BeFalse())
			Expect(backoff.IsBackoffError(regularErr)).To(BeFalse())

			// Error that contains both strings (edge case, should match first pattern)
			mixedErr := errors.New(backoff.TemporaryBackoffError + " and also " + backoff.PermanentFailureError)
			Expect(backoff.IsTemporaryBackoffError(mixedErr)).To(BeTrue())
			Expect(backoff.IsPermanentFailureError(mixedErr)).To(BeTrue())
			Expect(backoff.IsBackoffError(mixedErr)).To(BeTrue())
		})
	})

	Context("when extracting original errors", func() {
		It("should extract the original error from wrapped errors", func() {
			// Create an original error
			origErr := errors.New("original error")

			// Wrap it with fmt.Errorf
			wrappedErr := fmt.Errorf("%s: %w", backoff.TemporaryBackoffError, origErr)

			// Extract should return the original
			extracted := backoff.ExtractOriginalError(wrappedErr)
			Expect(extracted).To(Equal(origErr))
		})

		It("should handle unwrapped errors", func() {
			// Simple error without wrapping
			simpleErr := errors.New("simple error")
			extracted := backoff.ExtractOriginalError(simpleErr)
			Expect(extracted).To(Equal(simpleErr))
		})

		It("should handle nil errors", func() {
			extracted := backoff.ExtractOriginalError(nil)
			Expect(extracted).To(BeNil())
		})

		It("should handle deeply nested errors", func() {
			// Create a chain of wrapped errors
			level1 := errors.New("level 1 error")
			level2 := fmt.Errorf("level 2: %w", level1)
			level3 := fmt.Errorf("%s: %w", backoff.TemporaryBackoffError, level2)

			// Extract should find the root cause
			extracted := backoff.ExtractOriginalError(level3)
			Expect(extracted).To(Equal(level1))
		})
	})
})
