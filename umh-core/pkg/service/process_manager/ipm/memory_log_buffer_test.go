//go:build internal_process_manager
// +build internal_process_manager

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

package ipm_test

import (
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
)

var _ = Describe("MemoryLogBuffer", func() {
	var buffer *ipm.MemoryLogBuffer

	Context("when creating a new buffer", func() {
		It("should create with default size when given zero or negative size", func() {
			buffer = ipm.NewMemoryLogBuffer(0)
			Expect(buffer.MaxSize()).To(Equal(ipm.DefaultLogBufferSize))

			buffer = ipm.NewMemoryLogBuffer(-100)
			Expect(buffer.MaxSize()).To(Equal(ipm.DefaultLogBufferSize))
		})

		It("should create with specified size when given positive size", func() {
			buffer = ipm.NewMemoryLogBuffer(5000)
			Expect(buffer.MaxSize()).To(Equal(5000))
		})

		It("should start empty", func() {
			buffer = ipm.NewMemoryLogBuffer(1000)
			Expect(buffer.Size()).To(Equal(0))
			Expect(buffer.GetEntries()).To(BeEmpty())
		})
	})

	Context("when adding entries", func() {
		BeforeEach(func() {
			buffer = ipm.NewMemoryLogBuffer(5)
		})

		It("should add entries and maintain size", func() {
			entry1 := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "First entry",
			}
			entry2 := process_shared.LogEntry{
				Timestamp: time.Now().Add(time.Second),
				Content:   "Second entry",
			}

			buffer.AddEntry(entry1)
			Expect(buffer.Size()).To(Equal(1))

			buffer.AddEntry(entry2)
			Expect(buffer.Size()).To(Equal(2))

			entries := buffer.GetEntries()
			Expect(entries).To(HaveLen(2))
			Expect(entries[0].Content).To(Equal("First entry"))
			Expect(entries[1].Content).To(Equal("Second entry"))
		})

		It("should maintain chronological order", func() {
			baseTime := time.Now()

			entry1 := process_shared.LogEntry{
				Timestamp: baseTime,
				Content:   "Entry 1",
			}
			entry2 := process_shared.LogEntry{
				Timestamp: baseTime.Add(time.Second),
				Content:   "Entry 2",
			}
			entry3 := process_shared.LogEntry{
				Timestamp: baseTime.Add(2 * time.Second),
				Content:   "Entry 3",
			}

			buffer.AddEntry(entry1)
			buffer.AddEntry(entry2)
			buffer.AddEntry(entry3)

			entries := buffer.GetEntries()
			Expect(entries).To(HaveLen(3))
			Expect(entries[0].Content).To(Equal("Entry 1"))
			Expect(entries[1].Content).To(Equal("Entry 2"))
			Expect(entries[2].Content).To(Equal("Entry 3"))
		})
	})

	Context("when buffer reaches capacity", func() {
		BeforeEach(func() {
			buffer = ipm.NewMemoryLogBuffer(3) // Small buffer for testing
		})

		It("should overwrite oldest entries when full", func() {
			// Fill the buffer
			for i := 1; i <= 3; i++ {
				entry := process_shared.LogEntry{
					Timestamp: time.Now().Add(time.Duration(i) * time.Second),
					Content:   fmt.Sprintf("Entry %d", i),
				}
				buffer.AddEntry(entry)
			}

			entries := buffer.GetEntries()
			Expect(entries).To(HaveLen(3))
			Expect(buffer.Size()).To(Equal(3))

			// Add one more entry - should overwrite the first
			entry4 := process_shared.LogEntry{
				Timestamp: time.Now().Add(4 * time.Second),
				Content:   "Entry 4",
			}
			buffer.AddEntry(entry4)

			entries = buffer.GetEntries()
			Expect(entries).To(HaveLen(3))
			Expect(buffer.Size()).To(Equal(3))

			// Should contain entries 2, 3, 4 in chronological order
			Expect(entries[0].Content).To(Equal("Entry 2"))
			Expect(entries[1].Content).To(Equal("Entry 3"))
			Expect(entries[2].Content).To(Equal("Entry 4"))
		})

		It("should handle multiple wraparounds correctly", func() {
			// Add 10 entries to a buffer of size 3
			for i := 1; i <= 10; i++ {
				entry := process_shared.LogEntry{
					Timestamp: time.Now().Add(time.Duration(i) * time.Second),
					Content:   fmt.Sprintf("Entry %d", i),
				}
				buffer.AddEntry(entry)
			}

			entries := buffer.GetEntries()
			Expect(entries).To(HaveLen(3))

			// Should contain the last 3 entries (8, 9, 10)
			Expect(entries[0].Content).To(Equal("Entry 8"))
			Expect(entries[1].Content).To(Equal("Entry 9"))
			Expect(entries[2].Content).To(Equal("Entry 10"))
		})
	})

	Context("when clearing the buffer", func() {
		BeforeEach(func() {
			buffer = ipm.NewMemoryLogBuffer(1000)

			// Add some entries
			for i := 1; i <= 5; i++ {
				entry := process_shared.LogEntry{
					Timestamp: time.Now().Add(time.Duration(i) * time.Second),
					Content:   fmt.Sprintf("Entry %d", i),
				}
				buffer.AddEntry(entry)
			}
		})

		It("should clear all entries and reset size", func() {
			Expect(buffer.Size()).To(Equal(5))
			Expect(buffer.GetEntries()).To(HaveLen(5))

			buffer.Clear()

			Expect(buffer.Size()).To(Equal(0))
			Expect(buffer.GetEntries()).To(BeEmpty())
		})

		It("should allow adding entries after clearing", func() {
			buffer.Clear()

			entry := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "New entry after clear",
			}
			buffer.AddEntry(entry)

			Expect(buffer.Size()).To(Equal(1))
			entries := buffer.GetEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal("New entry after clear"))
		})
	})

	Context("when accessed concurrently", func() {
		BeforeEach(func() {
			buffer = ipm.NewMemoryLogBuffer(1000)
		})

		It("should handle concurrent writes and reads safely", func() {
			const numGoroutines = 10
			const entriesPerGoroutine = 100

			var wg sync.WaitGroup

			// Start multiple goroutines adding entries
			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(routineID int) {
					defer wg.Done()
					for j := 0; j < entriesPerGoroutine; j++ {
						entry := process_shared.LogEntry{
							Timestamp: time.Now(),
							Content:   fmt.Sprintf("Routine %d Entry %d", routineID, j),
						}
						buffer.AddEntry(entry)
					}
				}(i)
			}

			// Start goroutines reading entries
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 50; j++ {
						entries := buffer.GetEntries()
						// Just ensure we can read without panicking
						_ = len(entries)
						time.Sleep(time.Microsecond)
					}
				}()
			}

			// Start goroutines checking size
			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 100; j++ {
						size := buffer.Size()
						maxSize := buffer.MaxSize()
						// Ensure size is never greater than maxSize
						Expect(size).To(BeNumerically("<=", maxSize))
						time.Sleep(time.Microsecond)
					}
				}()
			}

			wg.Wait()

			// Final validation
			finalEntries := buffer.GetEntries()
			Expect(len(finalEntries)).To(BeNumerically("<=", buffer.MaxSize()))
			Expect(buffer.Size()).To(Equal(len(finalEntries)))
		})

		It("should handle concurrent clear operations safely", func() {
			// Add some initial entries
			for i := 0; i < 100; i++ {
				entry := process_shared.LogEntry{
					Timestamp: time.Now(),
					Content:   fmt.Sprintf("Entry %d", i),
				}
				buffer.AddEntry(entry)
			}

			var wg sync.WaitGroup

			// Start goroutines that clear the buffer
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 10; j++ {
						buffer.Clear()
						time.Sleep(time.Microsecond)
					}
				}()
			}

			// Start goroutines that add entries
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func(routineID int) {
					defer wg.Done()
					for j := 0; j < 50; j++ {
						entry := process_shared.LogEntry{
							Timestamp: time.Now(),
							Content:   fmt.Sprintf("Concurrent entry %d-%d", routineID, j),
						}
						buffer.AddEntry(entry)
						time.Sleep(time.Microsecond)
					}
				}(i)
			}

			wg.Wait()

			// Should complete without panicking
			finalSize := buffer.Size()
			Expect(finalSize).To(BeNumerically(">=", 0))
			Expect(finalSize).To(BeNumerically("<=", buffer.MaxSize()))
		})
	})

	Context("edge cases", func() {
		It("should handle buffer of size 1", func() {
			buffer = ipm.NewMemoryLogBuffer(1)

			entry1 := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "First",
			}
			entry2 := process_shared.LogEntry{
				Timestamp: time.Now().Add(time.Second),
				Content:   "Second",
			}

			buffer.AddEntry(entry1)
			entries := buffer.GetEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal("First"))

			buffer.AddEntry(entry2)
			entries = buffer.GetEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal("Second"))
		})

		It("should handle empty entries correctly", func() {
			buffer = ipm.NewMemoryLogBuffer(100)

			entry := process_shared.LogEntry{
				Timestamp: time.Now(),
				Content:   "", // Empty content
			}

			buffer.AddEntry(entry)
			entries := buffer.GetEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Content).To(Equal(""))
		})

		It("should preserve exact timestamps", func() {
			buffer = ipm.NewMemoryLogBuffer(100)

			timestamp := time.Date(2025, 1, 15, 12, 30, 45, 123456789, time.UTC)
			entry := process_shared.LogEntry{
				Timestamp: timestamp,
				Content:   "Timestamp test",
			}

			buffer.AddEntry(entry)
			entries := buffer.GetEntries()
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Timestamp).To(Equal(timestamp))
		})
	})
})
