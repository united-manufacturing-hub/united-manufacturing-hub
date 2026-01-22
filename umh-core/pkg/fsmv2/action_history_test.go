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

package fsmv2_test

import (
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// Tests run as part of FSMv2 Suite (see dependencies_test.go)

var _ = Describe("ActionHistoryRecorder", func() {
	Describe("InMemoryActionHistoryRecorder", func() {
		var recorder *fsmv2.InMemoryActionHistoryRecorder

		BeforeEach(func() {
			recorder = fsmv2.NewInMemoryActionHistoryRecorder()
		})

		Describe("Record and Drain", func() {
			It("should record action results", func() {
				recorder.Record(fsmv2.ActionResult{
					ActionType: "TestAction",
					Success:    true,
					Timestamp:  time.Now(),
				})

				results := recorder.Drain()
				Expect(results).To(HaveLen(1))
				Expect(results[0].ActionType).To(Equal("TestAction"))
				Expect(results[0].Success).To(BeTrue())
			})

			It("should clear buffer after drain", func() {
				recorder.Record(fsmv2.ActionResult{ActionType: "Test1"})
				firstDrain := recorder.Drain()
				Expect(firstDrain).To(HaveLen(1))

				secondDrain := recorder.Drain()
				Expect(secondDrain).To(BeEmpty())
			})

			It("should preserve all fields in recorded results", func() {
				now := time.Now()
				latency := 500 * time.Millisecond

				recorder.Record(fsmv2.ActionResult{
					Timestamp:  now,
					ActionType: "SyncAction",
					ErrorMsg:   "connection refused",
					Latency:    latency,
					Success:    false,
				})

				results := recorder.Drain()
				Expect(results).To(HaveLen(1))
				Expect(results[0].Timestamp).To(Equal(now))
				Expect(results[0].ActionType).To(Equal("SyncAction"))
				Expect(results[0].ErrorMsg).To(Equal("connection refused"))
				Expect(results[0].Latency).To(Equal(latency))
				Expect(results[0].Success).To(BeFalse())
			})

			It("should record multiple results in order", func() {
				recorder.Record(fsmv2.ActionResult{ActionType: "First"})
				recorder.Record(fsmv2.ActionResult{ActionType: "Second"})
				recorder.Record(fsmv2.ActionResult{ActionType: "Third"})

				results := recorder.Drain()
				Expect(results).To(HaveLen(3))
				Expect(results[0].ActionType).To(Equal("First"))
				Expect(results[1].ActionType).To(Equal("Second"))
				Expect(results[2].ActionType).To(Equal("Third"))
			})

			It("should return empty slice when no results recorded", func() {
				results := recorder.Drain()
				Expect(results).NotTo(BeNil())
				Expect(results).To(BeEmpty())
			})
		})

		Describe("Thread Safety", func() {
			It("should be thread-safe for concurrent recording", func() {
				var wg sync.WaitGroup
				numGoroutines := 100

				for i := 0; i < numGoroutines; i++ {
					wg.Add(1)
					go func(n int) {
						defer wg.Done()
						recorder.Record(fsmv2.ActionResult{
							ActionType: fmt.Sprintf("Action%d", n),
							Success:    true,
						})
					}(i)
				}

				wg.Wait()

				results := recorder.Drain()
				Expect(results).To(HaveLen(numGoroutines))
			})

			It("should be thread-safe for concurrent record and drain", func() {
				var wg sync.WaitGroup
				numRecords := 50

				// Start recording goroutines
				for i := 0; i < numRecords; i++ {
					wg.Add(1)
					go func(n int) {
						defer wg.Done()
						recorder.Record(fsmv2.ActionResult{
							ActionType: fmt.Sprintf("Action%d", n),
						})
					}(i)
				}

				// Periodically drain while recording
				drainedCount := 0
				for i := 0; i < 5; i++ {
					results := recorder.Drain()
					drainedCount += len(results)
					time.Sleep(time.Millisecond)
				}

				wg.Wait()

				// Final drain to get any remaining
				finalResults := recorder.Drain()
				drainedCount += len(finalResults)

				// All records should be accounted for
				Expect(drainedCount).To(Equal(numRecords))
			})
		})

		Describe("Interface compliance", func() {
			It("should implement ActionHistoryRecorder interface", func() {
				var _ fsmv2.ActionHistoryRecorder = recorder
			})
		})
	})
})
