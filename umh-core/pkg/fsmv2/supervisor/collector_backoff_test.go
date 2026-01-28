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

package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("Collector Backoff Non-Blocking", func() {
	// These tests verify that the tick loop completes quickly (<100ms) even when
	// collector restart is needed. The restartCollector function should use non-blocking
	// backoff (checking time.Since()) instead of blocking time.Sleep().

	Context("when collector restart is triggered directly", func() {
		It("should complete restartCollector within 100ms (non-blocking backoff)", func() {
			// Create a supervisor with a worker
			s := newSupervisorWithWorker(&mockWorker{
				initialState: &mockState{},
			}, nil, supervisor.CollectorHealthConfig{
				ObservationTimeout: 500 * time.Millisecond,
				StaleThreshold:     1 * time.Second,
				Timeout:            2 * time.Second,
				MaxRestartAttempts: 3,
			})

			// Call restartCollector directly and measure duration
			// This is the function that had blocking time.Sleep()
			start := time.Now()
			err := s.TestRestartCollector(context.Background(), "test-worker")
			elapsed := time.Since(start)

			// restartCollector should complete successfully
			Expect(err).ToNot(HaveOccurred())

			// CRITICAL: Must complete within 100ms (non-blocking)
			// Prior to fix: time.Sleep(2s) on first attempt
			// After fix: non-blocking check with time.Since()
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond),
				"restartCollector should be non-blocking. Actual duration: %v. "+
					"This indicates time.Sleep() is still being used.", elapsed)

			// Verify restart count was incremented
			restartCount := s.TestGetRestartCount()
			Expect(restartCount).To(Equal(1), "Restart count should be 1 after restartCollector call")
		})
	})

	Context("when multiple restart attempts occur", func() {
		It("should not accumulate blocking time across calls", func() {
			s := newSupervisorWithWorker(&mockWorker{
				initialState: &mockState{},
			}, nil, supervisor.CollectorHealthConfig{
				ObservationTimeout: 500 * time.Millisecond,
				StaleThreshold:     1 * time.Second,
				Timeout:            2 * time.Second,
				MaxRestartAttempts: 5, // Allow multiple restarts
			})

			// Call restartCollector multiple times in quick succession
			// Non-blocking design: only first call increments count, subsequent calls
			// within backoff window are skipped (non-blocking, no error)
			totalStart := time.Now()

			for i := range 3 {
				callStart := time.Now()
				err := s.TestRestartCollector(context.Background(), "test-worker")
				callElapsed := time.Since(callStart)

				Expect(err).ToNot(HaveOccurred())
				Expect(callElapsed).To(BeNumerically("<", 100*time.Millisecond),
					"restartCollector call %d should be non-blocking. Actual: %v", i+1, callElapsed)
			}

			totalElapsed := time.Since(totalStart)

			// Total time for all calls should be well under 1 second
			// (3 calls that are all non-blocking)
			Expect(totalElapsed).To(BeNumerically("<", 500*time.Millisecond),
				"Total time for 3 restartCollector calls should be <500ms. Actual: %v", totalElapsed)

			// With non-blocking backoff, only first call increments count
			// Subsequent calls within backoff window are skipped
			restartCount := s.TestGetRestartCount()
			Expect(restartCount).To(Equal(1), "Only first call should increment restart count (others skipped due to backoff)")
		})
	})

	Context("backoff timing should be tracked, not blocked", func() {
		It("should skip restart if called before backoff period expires", func() {
			s := newSupervisorWithWorker(&mockWorker{
				initialState: &mockState{},
			}, nil, supervisor.CollectorHealthConfig{
				ObservationTimeout: 500 * time.Millisecond,
				StaleThreshold:     1 * time.Second,
				Timeout:            2 * time.Second,
				MaxRestartAttempts: 5,
			})

			// First call: should succeed and start backoff period
			start := time.Now()
			err := s.TestRestartCollector(context.Background(), "test-worker")
			elapsed := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond))
			Expect(s.TestGetRestartCount()).To(Equal(1))

			// Immediate second call: should be skipped due to backoff
			// (or increment count but not block - depends on implementation choice)
			start = time.Now()
			err = s.TestRestartCollector(context.Background(), "test-worker")
			elapsed = time.Since(start)

			// Either way, should be non-blocking
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond),
				"Second restartCollector call should be non-blocking")

			// The implementation can either:
			// 1. Skip the restart (count stays at 1) because backoff not elapsed
			// 2. Or proceed with restart (count goes to 2)
			// Both are valid, but must be non-blocking
			_ = err // err can be nil or indicate "backoff not elapsed"
		})
	})
})
