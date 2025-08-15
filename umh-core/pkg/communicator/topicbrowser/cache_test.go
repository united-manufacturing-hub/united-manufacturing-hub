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
	"google.golang.org/protobuf/proto"

	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	topicbrowserservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
)

var _ = Describe("Cache", func() {
	var cache *topicbrowser.Cache

	BeforeEach(func() {
		cache = topicbrowser.NewCache()
		mockSequenceCounter = 0 // Reset sequence counter for each test
	})

	Describe("Cache Update and Data Management", func() {
		It("should gracefully upsert new data into the cache", func() {
			// Create first UnsBundle with initial data
			bundle1 := createMockUnsBundle(map[string]int64{
				"topic1": 1000,
				"topic2": 1100,
			}, map[string]string{
				"uns.topic1": "Topic 1 Info",
				"uns.topic2": "Topic 2 Info",
			})

			// Create observed state with first bundle
			obs1 := createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{
				{Payload: bundle1, Timestamp: time.UnixMilli(1000)},
			})

			// Update cache with first bundle
			_, err := cache.ProcessIncrementalUpdates(obs1)
			Expect(err).ToNot(HaveOccurred())

			// Verify first update
			eventMap := cache.GetEventMap()
			Expect(eventMap).To(HaveLen(2))
			Expect(eventMap["topic1"].GetProducedAtMs()).To(Equal(uint64(1000)))
			Expect(eventMap["topic2"].GetProducedAtMs()).To(Equal(uint64(1100)))

			unsMap := cache.GetUnsMap()
			Expect(unsMap.GetEntries()).To(HaveLen(2))
			for _, entry := range unsMap.GetEntries() {
				Expect(entry.GetName()).To(Or(Equal("uns.topic1"), Equal("uns.topic2")))
			}

			// Create second UnsBundle with updated and new data
			bundle2 := createMockUnsBundle(map[string]int64{
				"topic1": 2000, // Updated timestamp
				"topic3": 1500, // New topic
			}, map[string]string{
				"uns.topic1": "Updated Topic 1 Info",
				"uns.topic3": "Topic 3 Info",
			})

			// Create observed state with second bundle
			obs2 := createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{
				{Payload: bundle2, Timestamp: time.UnixMilli(2000)},
			})

			// Update cache with second bundle
			_, err = cache.ProcessIncrementalUpdates(obs2)
			Expect(err).ToNot(HaveOccurred())

			// Verify graceful upsert
			eventMap = cache.GetEventMap()
			Expect(eventMap).To(HaveLen(3))
			Expect(eventMap["topic1"].GetProducedAtMs()).To(Equal(uint64(2000))) // Updated
			Expect(eventMap["topic2"].GetProducedAtMs()).To(Equal(uint64(1100))) // Unchanged
			Expect(eventMap["topic3"].GetProducedAtMs()).To(Equal(uint64(1500))) // New

			unsMap = cache.GetUnsMap()
			Expect(unsMap.GetEntries()).To(HaveLen(3))
		})

		It("should only use new bundles for cache update", func() {
			// Create initial bundle
			bundle1 := createMockUnsBundle(map[string]int64{
				"topic1": 1000,
			}, map[string]string{
				"uns.topic1": "Initial Topic 1",
			})

			// Create observed state with initial bundle
			obs1 := createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{
				{Payload: bundle1, Timestamp: time.UnixMilli(1000)},
			})

			// Update cache
			_, err := cache.ProcessIncrementalUpdates(obs1)
			Expect(err).ToNot(HaveOccurred())
			Expect(cache.GetLastCachedTimestamp()).To(Equal(time.UnixMilli(1000)))

			// Verify initial state
			eventMap := cache.GetEventMap()
			Expect(eventMap["topic1"].GetProducedAtMs()).To(Equal(uint64(1000)))

			// Create a modified version of the old bundle (simulating modification after processing)
			modifiedOldBundle := createMockUnsBundle(map[string]int64{
				"topic1": 999, // Different timestamp but older than lastCacheTimestamp
			}, map[string]string{
				"uns.topic1": "Modified Old Topic 1",
			})

			// Create new bundle with newer timestamp
			newBundle := createMockUnsBundle(map[string]int64{
				"topic2": 2000,
			}, map[string]string{
				"uns.topic2": "New Topic 2",
			})

			// Create observed state with both old (modified) and new bundles
			obs2 := createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{
				{Payload: modifiedOldBundle, Timestamp: time.UnixMilli(500)}, // Old timestamp - should be ignored
				{Payload: newBundle, Timestamp: time.UnixMilli(2000)},        // New timestamp - should be processed
			})

			// Update cache
			_, err = cache.ProcessIncrementalUpdates(obs2)
			Expect(err).ToNot(HaveOccurred())

			// Verify that only new bundle was processed
			eventMap = cache.GetEventMap()
			Expect(eventMap).To(HaveLen(2))
			Expect(eventMap["topic1"].GetProducedAtMs()).To(Equal(uint64(1000))) // Unchanged from original
			Expect(eventMap["topic2"].GetProducedAtMs()).To(Equal(uint64(2000))) // New topic added

			// Verify timestamp was updated correctly
			Expect(cache.GetLastCachedTimestamp()).To(Equal(time.UnixMilli(2000)))
		})

		It("should generate proper proto-encoded UnsBundle with the data", func() {
			// Setup cache with test data
			bundle1 := createMockUnsBundle(map[string]int64{
				"topic1": 1000,
				"topic2": 1100,
			}, map[string]string{
				"uns.topic1": "Topic 1 Info",
				"uns.topic2": "Topic 2 Info",
			})

			obs := createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{
				{Payload: bundle1, Timestamp: time.UnixMilli(1000)},
			})

			_, err := cache.ProcessIncrementalUpdates(obs)
			Expect(err).ToNot(HaveOccurred())

			// Generate proto-encoded bundle
			encodedBundle := cache.ToUnsBundleProto()
			Expect(encodedBundle).ToNot(BeNil())

			// Decode and verify the generated bundle
			var decodedBundle tbproto.UnsBundle
			err = proto.Unmarshal(encodedBundle, &decodedBundle)
			Expect(err).ToNot(HaveOccurred())

			// Verify events are correctly encoded
			Expect(decodedBundle.GetEvents().GetEntries()).To(HaveLen(2))

			// Create a map for easier verification
			eventsByTreeId := make(map[string]*tbproto.EventTableEntry)
			for _, entry := range decodedBundle.GetEvents().GetEntries() {
				eventsByTreeId[entry.GetUnsTreeId()] = entry
			}

			Expect(eventsByTreeId["topic1"].GetProducedAtMs()).To(Equal(uint64(1000)))
			Expect(eventsByTreeId["topic2"].GetProducedAtMs()).To(Equal(uint64(1100)))

			// Verify UnsMap is correctly encoded
			Expect(decodedBundle.GetUnsMap().GetEntries()).To(HaveLen(2))
			for _, entry := range decodedBundle.GetUnsMap().GetEntries() {
				Expect(entry.GetName()).To(Or(Equal("uns.topic1"), Equal("uns.topic2")))
			}
		})

		It("should handle multiple updates with complex data scenarios", func() {
			// First update: Initial data
			bundle1 := createMockUnsBundle(map[string]int64{
				"topic1": 1000,
				"topic2": 1100,
				"topic3": 1200,
			}, map[string]string{
				"uns.topic1": "Topic 1",
				"uns.topic2": "Topic 2",
				"uns.topic3": "Topic 3",
			})

			obs1 := createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{
				{Payload: bundle1, Timestamp: time.UnixMilli(1200)},
			})

			_, err := cache.ProcessIncrementalUpdates(obs1)
			Expect(err).ToNot(HaveOccurred())

			// Second update: Mix of older and newer data
			bundle2 := createMockUnsBundle(map[string]int64{
				"topic1": 900,  // Older - should be ignored
				"topic2": 1500, // Newer - should update
				"topic4": 1300, // New topic
			}, map[string]string{
				"uns.topic1": "Old Topic 1", // Should not update
				"uns.topic2": "Updated Topic 2",
				"uns.topic4": "New Topic 4",
			})

			obs2 := createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{
				{Payload: bundle2, Timestamp: time.UnixMilli(1500)},
			})

			_, err = cache.ProcessIncrementalUpdates(obs2)
			Expect(err).ToNot(HaveOccurred())

			// Third update: All new data
			bundle3 := createMockUnsBundle(map[string]int64{
				"topic1": 2000, // Much newer - should update
				"topic5": 1800, // New topic
			}, map[string]string{
				"uns.topic1": "Latest Topic 1",
				"uns.topic5": "Topic 5",
			})

			obs3 := createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{
				{Payload: bundle3, Timestamp: time.UnixMilli(2000)},
			})

			_, err = cache.ProcessIncrementalUpdates(obs3)
			Expect(err).ToNot(HaveOccurred())

			// Verify final state
			eventMap := cache.GetEventMap()
			Expect(eventMap).To(HaveLen(5))
			Expect(eventMap["topic1"].GetProducedAtMs()).To(Equal(uint64(2000))) // Updated to latest
			Expect(eventMap["topic2"].GetProducedAtMs()).To(Equal(uint64(1500))) // Updated in second batch
			Expect(eventMap["topic3"].GetProducedAtMs()).To(Equal(uint64(1200))) // Original value
			Expect(eventMap["topic4"].GetProducedAtMs()).To(Equal(uint64(1300))) // Added in second batch
			Expect(eventMap["topic5"].GetProducedAtMs()).To(Equal(uint64(1800))) // Added in third batch

			unsMap := cache.GetUnsMap()
			Expect(unsMap.GetEntries()).To(HaveLen(5))

			// Verify cache timestamp tracking
			Expect(cache.GetLastCachedTimestamp()).To(Equal(time.UnixMilli(2000)))
		})

		It("should handle invalid protobuf data gracefully", func() {
			// Create observed state with invalid protobuf data
			obs := createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{
				{Payload: []byte("invalid protobuf data"), Timestamp: time.UnixMilli(1000)},
				{Payload: []byte{0x00, 0xFF, 0xAA}, Timestamp: time.UnixMilli(1100)}, // Invalid binary data
			})

			// Update should not fail even with invalid data
			_, err := cache.ProcessIncrementalUpdates(obs)
			Expect(err).ToNot(HaveOccurred())

			// Cache should remain empty since no valid data was processed
			eventMap := cache.GetEventMap()
			Expect(eventMap).To(HaveLen(0))

			unsMap := cache.GetUnsMap()
			Expect(unsMap.GetEntries()).To(HaveLen(0))

			// But timestamp should be updated to latest processed timestamp
			Expect(cache.GetLastCachedTimestamp()).To(Equal(time.UnixMilli(1100)))
		})

	})
})

// Helper functions for creating mock data

// Global sequence counter for test mock buffers.
var mockSequenceCounter uint64

func createMockUnsBundle(events map[string]int64, unsTopics map[string]string) []byte {
	bundle := &tbproto.UnsBundle{
		Events: &tbproto.EventTable{
			Entries: make([]*tbproto.EventTableEntry, 0),
		},
		UnsMap: &tbproto.TopicMap{
			Entries: make(map[string]*tbproto.TopicInfo),
		},
	}

	// Add events
	for treeId, timestamp := range events {
		entry := &tbproto.EventTableEntry{
			UnsTreeId:    treeId,
			ProducedAtMs: uint64(timestamp),
			// Add other required fields as needed
		}
		bundle.Events.Entries = append(bundle.Events.Entries, entry)
	}

	// Add UNS map entries
	for name := range unsTopics {
		topicInfo := &tbproto.TopicInfo{
			Name: name,
			// Add other required fields as needed
		}
		bundle.UnsMap.Entries[topicbrowser.HashUNSTableEntry(topicInfo)] = topicInfo
	}

	encoded, _ := proto.Marshal(bundle)

	return encoded
}

func createMockObservedStateSnapshot(buffers []*topicbrowserservice.BufferItem) *topicbrowserfsm.ObservedStateSnapshot {
	// Assign sequence numbers to buffers that don't have them
	var maxSeq uint64

	for _, buf := range buffers {
		if buf.SequenceNum == 0 {
			mockSequenceCounter++
			buf.SequenceNum = mockSequenceCounter
		}

		if buf.SequenceNum > maxSeq {
			maxSeq = buf.SequenceNum
		}
	}

	return &topicbrowserfsm.ObservedStateSnapshot{
		ServiceInfo: topicbrowserservice.ServiceInfo{
			Status: topicbrowserservice.Status{
				BufferSnapshot: topicbrowserservice.RingBufferSnapshot{
					Items: buffers,

					LastSequenceNum: maxSeq,
				},
				Logs: []s6svc.LogEntry{}, // Initialize with empty slice
			},
			// Leave other fields as zero values
		},
	}
}
