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

package topicbrowser_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	topicbrowserservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"google.golang.org/protobuf/proto"
)

var _ = Describe("Cache Sequence-Based Processing", func() {
	var cache *topicbrowser.Cache

	BeforeEach(func() {
		cache = topicbrowser.NewCache()
		mockSequenceCounter = 0 // Reset sequence counter for each test
	})

	Describe("ProcessIncrementalUpdates", func() {
		It("should process buffers based on sequence number, not timestamp", func() {
			// Create initial buffers with sequence numbers
			buffer1 := createMockBufferWithSequence(map[string]int64{"topic1": 1000}, time.UnixMilli(1000), 1)
			buffer2 := createMockBufferWithSequence(map[string]int64{"topic2": 2000}, time.UnixMilli(2000), 2)
			buffer3 := createMockBufferWithSequence(map[string]int64{"topic3": 3000}, time.UnixMilli(3000), 3)

			// First call - should process all buffers (sequence 1-3)
			obs1 := &topicbrowserfsm.ObservedStateSnapshot{
				ServiceInfo: topicbrowserservice.ServiceInfo{
					Status: topicbrowserservice.Status{
						BufferSnapshot: topicbrowserservice.RingBufferSnapshot{
							Items:           []*topicbrowserservice.BufferItem{buffer3, buffer2, buffer1}, // Newest-to-oldest
							LastSequenceNum: 3,
						},
					},
				},
			}

			result1, err := cache.ProcessIncrementalUpdates(obs1)
			Expect(err).ToNot(HaveOccurred())
			Expect(result1.ProcessedCount).To(Equal(3))
			Expect(result1.DebugInfo).To(ContainSubstring("Processed 3 buffers (sequence 1 to 3)"))

			// Verify cache state
			Expect(cache.GetLastProcessedSequenceNum()).To(Equal(uint64(3)))
			eventMap := cache.GetEventMap()
			Expect(eventMap).To(HaveLen(3))

			// Second call - add new buffer with sequence 4
			buffer4 := createMockBufferWithSequence(map[string]int64{"topic4": 4000}, time.UnixMilli(4000), 4)
			obs2 := &topicbrowserfsm.ObservedStateSnapshot{
				ServiceInfo: topicbrowserservice.ServiceInfo{
					Status: topicbrowserservice.Status{
						BufferSnapshot: topicbrowserservice.RingBufferSnapshot{
							Items:           []*topicbrowserservice.BufferItem{buffer4, buffer3, buffer2, buffer1}, // Newest-to-oldest
							LastSequenceNum: 4,
						},
					},
				},
			}

			result2, err := cache.ProcessIncrementalUpdates(obs2)
			Expect(err).ToNot(HaveOccurred())
			Expect(result2.ProcessedCount).To(Equal(1)) // Only new buffer
			Expect(result2.DebugInfo).To(ContainSubstring("Processed 1 buffers (sequence 4 to 4)"))

			// Verify only new buffer was processed
			Expect(cache.GetLastProcessedSequenceNum()).To(Equal(uint64(4)))
			eventMap = cache.GetEventMap()
			Expect(eventMap).To(HaveLen(4))
		})

		It("should handle data loss detection correctly", func() {
			// Create initial buffers
			buffer1 := createMockBufferWithSequence(map[string]int64{"topic1": 1000}, time.UnixMilli(1000), 1)
			buffer2 := createMockBufferWithSequence(map[string]int64{"topic2": 2000}, time.UnixMilli(2000), 2)
			buffer3 := createMockBufferWithSequence(map[string]int64{"topic3": 3000}, time.UnixMilli(3000), 3)

			// First call - process all buffers
			obs1 := &topicbrowserfsm.ObservedStateSnapshot{
				ServiceInfo: topicbrowserservice.ServiceInfo{
					Status: topicbrowserservice.Status{
						BufferSnapshot: topicbrowserservice.RingBufferSnapshot{
							Items:           []*topicbrowserservice.BufferItem{buffer3, buffer2, buffer1}, // Newest-to-oldest
							LastSequenceNum: 3,
						},
					},
				},
			}

			result1, err := cache.ProcessIncrementalUpdates(obs1)
			Expect(err).ToNot(HaveOccurred())
			Expect(result1.ProcessedCount).To(Equal(3))
			Expect(cache.GetLastProcessedSequenceNum()).To(Equal(uint64(3)))

			// Simulate data loss - large gap in sequence numbers (sequence 10 vs last processed 3)
			// Gap of 7 is larger than buffer capacity of 1, so data loss should be detected
			buffer4 := createMockBufferWithSequence(map[string]int64{"topic4": 4000}, time.UnixMilli(4000), 10)
			obs2 := &topicbrowserfsm.ObservedStateSnapshot{
				ServiceInfo: topicbrowserservice.ServiceInfo{
					Status: topicbrowserservice.Status{
						BufferSnapshot: topicbrowserservice.RingBufferSnapshot{
							Items:           []*topicbrowserservice.BufferItem{buffer4}, // Only one buffer now
							LastSequenceNum: 10,
						},
					},
				},
			}

			result2, err := cache.ProcessIncrementalUpdates(obs2)
			Expect(err).ToNot(HaveOccurred())
			Expect(result2.ProcessedCount).To(Equal(1))
			Expect(result2.DebugInfo).To(ContainSubstring("Data loss detected"))

			// Verify sequence number was updated
			Expect(cache.GetLastProcessedSequenceNum()).To(Equal(uint64(10)))
		})

		It("should provide clear debug information", func() {
			buffer1 := createMockBufferWithSequence(map[string]int64{"topic1": 1000}, time.UnixMilli(1000), 1)
			buffer2 := createMockBufferWithSequence(map[string]int64{"topic2": 2000}, time.UnixMilli(2000), 2)

			obs := &topicbrowserfsm.ObservedStateSnapshot{
				ServiceInfo: topicbrowserservice.ServiceInfo{
					Status: topicbrowserservice.Status{
						BufferSnapshot: topicbrowserservice.RingBufferSnapshot{
							Items:           []*topicbrowserservice.BufferItem{buffer2, buffer1}, // Newest-to-oldest
							LastSequenceNum: 2,
						},
					},
				},
			}

			result, err := cache.ProcessIncrementalUpdates(obs)
			Expect(err).ToNot(HaveOccurred())

			// Check debug information
			Expect(result.ProcessedCount).To(Equal(2))
			Expect(result.SkippedCount).To(Equal(0))
			Expect(result.DebugInfo).To(ContainSubstring("Processed 2 buffers"))
			Expect(result.DebugInfo).To(ContainSubstring("sequence 1 to 2"))
			Expect(result.DebugInfo).To(ContainSubstring("skipped 0"))
		})
	})

	Describe("GetPendingBuffers", func() {
		It("should return buffers newer than threshold timestamp", func() {
			buffer1 := createMockBuffer(map[string]int64{"topic1": 1000}, time.UnixMilli(1000))
			buffer2 := createMockBuffer(map[string]int64{"topic2": 2000}, time.UnixMilli(2000))
			buffer3 := createMockBuffer(map[string]int64{"topic3": 3000}, time.UnixMilli(3000))

			obs := &topicbrowserfsm.ObservedStateSnapshot{
				ServiceInfo: topicbrowserservice.ServiceInfo{
					Status: topicbrowserservice.Status{
						BufferSnapshot: topicbrowserservice.RingBufferSnapshot{
							Items: []*topicbrowserservice.BufferItem{buffer3, buffer2, buffer1}, // Newest-to-oldest
						},
					},
				},
			}

			// Get buffers newer than 1500ms
			threshold := time.UnixMilli(1500)
			pendingBuffers, err := cache.GetPendingBuffers(obs, threshold)

			Expect(err).ToNot(HaveOccurred())
			Expect(pendingBuffers).To(HaveLen(2))        // buffer2 and buffer3
			Expect(pendingBuffers[0]).To(Equal(buffer3)) // Newest first
			Expect(pendingBuffers[1]).To(Equal(buffer2))
		})
	})
})

// Helper function to create mock buffers with sequence numbers.
func createMockBufferWithSequence(events map[string]int64, timestamp time.Time, sequenceNum uint64) *topicbrowserservice.BufferItem {
	bundle := &tbproto.UnsBundle{
		Events: &tbproto.EventTable{
			Entries: make([]*tbproto.EventTableEntry, 0),
		},
		UnsMap: &tbproto.TopicMap{
			Entries: make(map[string]*tbproto.TopicInfo),
		},
	}

	// Add events
	for treeId, ts := range events {
		entry := &tbproto.EventTableEntry{
			UnsTreeId:    treeId,
			ProducedAtMs: uint64(ts),
		}
		bundle.Events.Entries = append(bundle.Events.Entries, entry)

		// Add corresponding topic info
		topicInfo := &tbproto.TopicInfo{
			Name: treeId,
		}
		bundle.UnsMap.Entries[topicbrowser.HashUNSTableEntry(topicInfo)] = topicInfo
	}

	encoded, _ := proto.Marshal(bundle)

	return &topicbrowserservice.BufferItem{
		Payload:     encoded,
		Timestamp:   timestamp,
		SequenceNum: sequenceNum,
	}
}

// Helper function to create mock buffers (backward compatibility).
func createMockBuffer(events map[string]int64, timestamp time.Time) *topicbrowserservice.BufferItem {
	return createMockBufferWithSequence(events, timestamp, 0)
}
