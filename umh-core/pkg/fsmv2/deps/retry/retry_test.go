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

package retry_test

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps/retry"
)

var _ = Describe("Retry Tracker", func() {
	var tracker retry.Tracker

	BeforeEach(func() {
		tracker = retry.New()
	})

	Describe("Attempt()", func() {
		It("should increment consecutive errors without setting degraded timestamp", func() {
			// This is the key behavior that fixes the ResetTransportAction bug:
			// Attempt() advances the counter to break modulo-N triggers
			// but does NOT mark the system as degraded (no timestamp set)
			tracker.Attempt()

			Expect(tracker.ConsecutiveErrors()).To(Equal(1))

			_, isDegraded := tracker.DegradedSince()
			Expect(isDegraded).To(BeFalse(), "Attempt() should NOT set degraded timestamp")
		})

		It("should increment counter on multiple calls", func() {
			tracker.Attempt()
			tracker.Attempt()
			tracker.Attempt()

			Expect(tracker.ConsecutiveErrors()).To(Equal(3))
		})
	})

	Describe("RecordError()", func() {
		It("should set degraded timestamp on first error only", func() {
			beforeFirst := time.Now()
			tracker.RecordError()
			afterFirst := time.Now()

			degradedTime, isDegraded := tracker.DegradedSince()
			Expect(isDegraded).To(BeTrue())
			Expect(degradedTime).To(BeTemporally(">=", beforeFirst))
			Expect(degradedTime).To(BeTemporally("<=", afterFirst))
			Expect(tracker.ConsecutiveErrors()).To(Equal(1))

			// Second error should NOT change the degraded timestamp
			time.Sleep(10 * time.Millisecond)
			tracker.RecordError()

			degradedTime2, isDegraded2 := tracker.DegradedSince()
			Expect(isDegraded2).To(BeTrue())
			Expect(degradedTime2).To(Equal(degradedTime), "degraded timestamp should not change on subsequent errors")
			Expect(tracker.ConsecutiveErrors()).To(Equal(2))
		})

		It("should store error class and retry-after with options", func() {
			tracker.RecordError(
				retry.WithClass("network"),
				retry.WithRetryAfter(30*time.Second),
			)

			lastError := tracker.LastError()
			Expect(lastError.Class).To(Equal("network"))
			Expect(lastError.RetryAfter).To(Equal(30 * time.Second))
			Expect(lastError.OccurredAt).NotTo(BeZero())
		})

		It("should update error info on each error", func() {
			tracker.RecordError(retry.WithClass("network"))
			tracker.RecordError(retry.WithClass("rate_limit"))

			lastError := tracker.LastError()
			Expect(lastError.Class).To(Equal("rate_limit"))
		})
	})

	Describe("RecordSuccess()", func() {
		It("should reset all error state", func() {
			// Set up some error state
			tracker.RecordError(
				retry.WithClass("network"),
				retry.WithRetryAfter(30*time.Second),
			)
			tracker.RecordError()
			Expect(tracker.ConsecutiveErrors()).To(Equal(2))

			// Record success
			tracker.RecordSuccess()

			// Verify all state is reset
			Expect(tracker.ConsecutiveErrors()).To(Equal(0))

			_, isDegraded := tracker.DegradedSince()
			Expect(isDegraded).To(BeFalse())

			lastError := tracker.LastError()
			Expect(lastError.Class).To(BeEmpty())
			Expect(lastError.RetryAfter).To(Equal(time.Duration(0)))
			Expect(lastError.OccurredAt).To(BeZero())
		})
	})

	Describe("ShouldReset()", func() {
		It("should return true at modulo boundaries", func() {
			// This tests the transport reset trigger logic:
			// At 5, 10, 15, etc. errors, ShouldReset(5) returns true

			// 0 errors - should not reset
			Expect(tracker.ShouldReset(5)).To(BeFalse())

			// 1-4 errors - should not reset
			for i := 1; i <= 4; i++ {
				tracker.RecordError()
				Expect(tracker.ShouldReset(5)).To(BeFalse(), "should not reset at %d errors", i)
			}

			// 5 errors - should reset
			tracker.RecordError()
			Expect(tracker.ShouldReset(5)).To(BeTrue(), "should reset at 5 errors")

			// 6 errors - should NOT reset (this is what Attempt() achieves)
			tracker.Attempt()
			Expect(tracker.ShouldReset(5)).To(BeFalse(), "should NOT reset at 6 errors")

			// Continue to 10
			for i := 7; i <= 9; i++ {
				tracker.RecordError()
				Expect(tracker.ShouldReset(5)).To(BeFalse(), "should not reset at %d errors", i)
			}

			// 10 errors - should reset again
			tracker.RecordError()
			Expect(tracker.ShouldReset(5)).To(BeTrue(), "should reset at 10 errors")
		})
	})

	Describe("BackoffElapsed()", func() {
		It("should return true when not degraded", func() {
			// Fresh tracker - no errors yet
			Expect(tracker.BackoffElapsed(1 * time.Hour)).To(BeTrue())
		})

		It("should return false during backoff period", func() {
			tracker.RecordError()

			// Immediately after error, backoff should not have elapsed
			Expect(tracker.BackoffElapsed(1 * time.Hour)).To(BeFalse())
		})

		It("should return true after backoff period", func() {
			tracker.RecordError()

			// Very short backoff - should elapse quickly
			Eventually(func() bool {
				return tracker.BackoffElapsed(10 * time.Millisecond)
			}).Should(BeTrue())
		})
	})

	Describe("Thread Safety", func() {
		It("should be safe for concurrent access", func() {
			const goroutines = 10
			const iterations = 100

			var wg sync.WaitGroup
			wg.Add(goroutines * 3) // 3 types of operations

			// Concurrent RecordError
			for range goroutines {
				go func() {
					defer wg.Done()
					for range iterations {
						tracker.RecordError(retry.WithClass("test"))
					}
				}()
			}

			// Concurrent Attempt
			for range goroutines {
				go func() {
					defer wg.Done()
					for range iterations {
						tracker.Attempt()
					}
				}()
			}

			// Concurrent reads
			for range goroutines {
				go func() {
					defer wg.Done()
					for range iterations {
						_ = tracker.ConsecutiveErrors()
						_, _ = tracker.DegradedSince()
						_ = tracker.LastError()
						_ = tracker.ShouldReset(5)
						_ = tracker.BackoffElapsed(time.Second)
					}
				}()
			}

			// Should complete without race conditions
			wg.Wait()

			// Total increments should be goroutines * iterations * 2 (RecordError + Attempt)
			expectedMin := goroutines * iterations * 2
			Expect(tracker.ConsecutiveErrors()).To(BeNumerically(">=", expectedMin))
		})
	})

	Describe("ResetTransportAction Bug Fix", func() {
		// This test documents the exact bug that was found and how it's fixed
		It("should demonstrate the fix for infinite reset_transport loop", func() {
			// Simulate 5 sync failures (the threshold)
			for range 5 {
				tracker.RecordError(retry.WithClass("network"))
			}

			// At 5 errors, ShouldReset returns true - this triggers ResetTransportAction
			Expect(tracker.ShouldReset(5)).To(BeTrue(), "should trigger reset at 5 errors")

			// BUG (before fix): ResetTransportAction didn't call Attempt()
			// So counter stayed at 5, and ShouldReset(5) kept returning true
			// causing an infinite loop

			// FIX: ResetTransportAction now calls Attempt() to advance counter
			tracker.Attempt()

			// After Attempt(), counter is 6, and ShouldReset(5) returns false
			// This breaks the infinite loop
			Expect(tracker.ConsecutiveErrors()).To(Equal(6))
			Expect(tracker.ShouldReset(5)).To(BeFalse(), "should NOT trigger reset at 6 errors")

			// Now SyncAction can run instead of another ResetTransport
			// If sync fails, counter goes to 7, 8, 9, 10...
			// At 10, another reset is triggered (which is correct behavior)
			tracker.RecordError(retry.WithClass("network")) // 7
			tracker.RecordError(retry.WithClass("network")) // 8
			tracker.RecordError(retry.WithClass("network")) // 9
			Expect(tracker.ShouldReset(5)).To(BeFalse())

			tracker.RecordError(retry.WithClass("network")) // 10
			Expect(tracker.ShouldReset(5)).To(BeTrue(), "should trigger reset at 10 errors")
		})
	})
})
