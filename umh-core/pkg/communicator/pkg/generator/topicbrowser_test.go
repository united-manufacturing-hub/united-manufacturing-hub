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

package generator_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/generator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	topicbrowserservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
)

var _ = Describe("TopicBrowser Generator", func() {
	var (
		cache  *topicbrowser.Cache
		obs    *topicbrowserfsm.ObservedStateSnapshot
		logger *zap.SugaredLogger
	)

	BeforeEach(func() {
		cache = topicbrowser.NewCache()
		obs = createMockObservedState([]*topicbrowserservice.Buffer{})
		logger = zap.NewNop().Sugar()
	})

	Describe("GenerateTopicBrowser", func() {
		Context("when isBootstrapped is true (existing subscribers)", func() {
			It("should generate content with only pending bundles", func() {
				// Setup: Create a cache with some data
				setupCacheWithMockData(cache, map[string]int64{
					"topic1": 1000,
					"topic2": 1100,
				}, time.UnixMilli(1000))

				// Create observed state with buffers that are newer than lastSentTimestamp
				obs = createMockObservedState([]*topicbrowserservice.Buffer{
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic3": 2000}), Timestamp: time.UnixMilli(2000)},
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic4": 2100}), Timestamp: time.UnixMilli(2100)},
				})

				// Set lastSentTimestamp to filter buffers
				cache.SetLastSentTimestamp(time.UnixMilli(1500).UTC())

				// Act: Generate content for existing subscriber
				result := generator.GenerateTopicBrowser(cache, obs, true, logger)

				// Assert: Should contain only pending bundles, no cache bundle
				Expect(result).ToNot(BeNil())
				Expect(result.Health).ToNot(BeNil())
				Expect(result.TopicCount).To(Equal(2))   // Cache size
				Expect(result.UnsBundles).To(HaveLen(2)) // Only pending bundles

				// Verify bundle ordering (should start from index 0)
				Expect(result.UnsBundles).To(HaveKey(0))
				Expect(result.UnsBundles).To(HaveKey(1))

				// Verify lastSentTimestamp was updated
				Expect(cache.GetLastSentTimestamp()).To(Equal(time.UnixMilli(2100).UTC()))
			})

			It("should return empty bundles when no pending data exists", func() {
				// Setup: Create cache with data
				setupCacheWithMockData(cache, map[string]int64{
					"topic1": 1000,
				}, time.UnixMilli(1000))

				// Create observed state with buffers that are older than lastSentTimestamp
				obs = createMockObservedState([]*topicbrowserservice.Buffer{
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic1": 500}), Timestamp: time.UnixMilli(500)},
				})

				// Set lastSentTimestamp to a recent time
				cache.SetLastSentTimestamp(time.UnixMilli(2000).UTC())

				// Act: Generate content for existing subscriber
				result := generator.GenerateTopicBrowser(cache, obs, true, logger)

				// Assert: Should return empty bundles
				Expect(result).ToNot(BeNil())
				Expect(result.UnsBundles).To(HaveLen(0))
				Expect(result.TopicCount).To(Equal(1)) // Cache size unchanged

				// lastSentTimestamp should remain unchanged since no new data
				Expect(cache.GetLastSentTimestamp()).To(Equal(time.UnixMilli(2000).UTC()))
			})
		})

		Context("when isBootstrapped is false (new subscribers)", func() {
			It("should generate content with cache bundle at index 0 plus new bundles", func() {
				// Setup: Create cache with initial data
				setupCacheWithMockData(cache, map[string]int64{
					"topic1": 1000,
					"topic2": 1100,
				}, time.UnixMilli(1000))

				// Create observed state with buffers newer than lastCachedTimestamp
				obs = createMockObservedState([]*topicbrowserservice.Buffer{
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic3": 2000}), Timestamp: time.UnixMilli(2000)},
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic4": 2100, "topic5": 2200}), Timestamp: time.UnixMilli(2100)}, // add two new topics in one bundle
				})

				// Act: Generate content for new subscriber
				result := generator.GenerateTopicBrowser(cache, obs, false, logger)

				// Assert: Should contain cache bundle at index 0 + new bundles at indices 1,2
				Expect(result).ToNot(BeNil())
				Expect(result.Health).ToNot(BeNil())
				Expect(result.TopicCount).To(Equal(2))   // Cache size
				Expect(result.UnsBundles).To(HaveLen(3)) // Cache + 2 new bundles

				// Verify that the cache bundle contains the correct data (order-agnostic)
				firstBundle := result.UnsBundles[0]
				var firstBundleData tbproto.UnsBundle
				err := proto.Unmarshal(firstBundle, &firstBundleData)
				Expect(err).ToNot(HaveOccurred())
				Expect(firstBundleData.Events.Entries).To(HaveLen(2))

				// Create a map for order-agnostic verification
				eventsByTreeId := make(map[string]*tbproto.EventTableEntry)
				for _, entry := range firstBundleData.Events.Entries {
					eventsByTreeId[entry.UnsTreeId] = entry
				}

				// Verify both topics are present with correct timestamps
				Expect(eventsByTreeId).To(HaveKey("topic1"))
				Expect(eventsByTreeId).To(HaveKey("topic2"))
				Expect(eventsByTreeId["topic1"].ProducedAtMs).To(Equal(uint64(1000)))
				Expect(eventsByTreeId["topic2"].ProducedAtMs).To(Equal(uint64(1100)))
				Expect(firstBundleData.UnsMap.Entries).To(HaveLen(2))

				// Verify that the new bundles contain the correct data
				secondBundle := result.UnsBundles[1]
				var secondBundleData tbproto.UnsBundle
				err = proto.Unmarshal(secondBundle, &secondBundleData)
				Expect(err).ToNot(HaveOccurred())
				Expect(secondBundleData.Events.Entries).To(HaveLen(1))
				Expect(secondBundleData.Events.Entries[0].UnsTreeId).To(Equal("topic3"))
				Expect(secondBundleData.Events.Entries[0].ProducedAtMs).To(Equal(uint64(2000)))
				Expect(secondBundleData.UnsMap.Entries).To(HaveLen(1))

				thirdBundle := result.UnsBundles[2]
				var thirdBundleData tbproto.UnsBundle
				err = proto.Unmarshal(thirdBundle, &thirdBundleData)
				Expect(err).ToNot(HaveOccurred())
				Expect(thirdBundleData.Events.Entries).To(HaveLen(2))

				// Create map for order-agnostic verification of third bundle
				thirdEventsByTreeId := make(map[string]*tbproto.EventTableEntry)
				for _, entry := range thirdBundleData.Events.Entries {
					thirdEventsByTreeId[entry.UnsTreeId] = entry
				}

				Expect(thirdEventsByTreeId).To(HaveKey("topic4"))
				Expect(thirdEventsByTreeId).To(HaveKey("topic5"))
				Expect(thirdEventsByTreeId["topic4"].ProducedAtMs).To(Equal(uint64(2100)))
				Expect(thirdEventsByTreeId["topic5"].ProducedAtMs).To(Equal(uint64(2200)))
				Expect(thirdBundleData.UnsMap.Entries).To(HaveLen(2))

				// Verify bundle ordering
				Expect(result.UnsBundles).To(HaveKey(0)) // Cache bundle
				Expect(result.UnsBundles).To(HaveKey(1)) // First new bundle
				Expect(result.UnsBundles).To(HaveKey(2)) // Second new bundle

				// Verify cache bundle at index 0 contains the cached data
				cacheBundleData := result.UnsBundles[0]
				Expect(cacheBundleData).ToNot(BeNil())

				// Decode and verify cache bundle contains cached events
				var cacheBundle tbproto.UnsBundle
				err = proto.Unmarshal(cacheBundleData, &cacheBundle)
				Expect(err).ToNot(HaveOccurred())
				Expect(cacheBundle.Events.Entries).To(HaveLen(2))

				// Verify lastSentTimestamp was updated
				Expect(cache.GetLastSentTimestamp()).To(Equal(time.UnixMilli(2100).UTC()))
			})

			It("should work correctly when no new bundles exist since cache", func() {
				// Setup: Create cache with data
				setupCacheWithMockData(cache, map[string]int64{
					"topic1": 1000,
					"topic2": 1100,
				}, time.UnixMilli(2000)) // Recent cache timestamp

				// Create observed state with only old buffers
				obs = createMockObservedState([]*topicbrowserservice.Buffer{
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic1": 1500}), Timestamp: time.UnixMilli(1500)},
				})

				// Act: Generate content for new subscriber
				result := generator.GenerateTopicBrowser(cache, obs, false, logger)

				// Assert: Should contain only the cache bundle at index 0
				Expect(result).ToNot(BeNil())
				Expect(result.UnsBundles).To(HaveLen(1)) // Only cache bundle
				Expect(result.UnsBundles).To(HaveKey(0)) // Cache bundle at index 0

				// Verify cache bundle contains the cached data
				cacheBundleData := result.UnsBundles[0]
				var cacheBundle tbproto.UnsBundle
				err := proto.Unmarshal(cacheBundleData, &cacheBundle)
				Expect(err).ToNot(HaveOccurred())
				Expect(cacheBundle.Events.Entries).To(HaveLen(2))
			})

			It("should handle empty cache correctly", func() {
				// Use empty cache (no setup)
				// Create observed state with some bundles
				obs = createMockObservedState([]*topicbrowserservice.Buffer{
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic1": 1000}), Timestamp: time.UnixMilli(1000)},
				})

				// Act: Generate content for new subscriber
				result := generator.GenerateTopicBrowser(cache, obs, false, logger)

				// Assert: Should contain cache bundle (empty) + new bundles
				Expect(result).ToNot(BeNil())
				Expect(result.UnsBundles).To(HaveLen(2)) // Empty cache + 1 new bundle
				Expect(result.UnsBundles).To(HaveKey(0)) // Cache bundle
				Expect(result.UnsBundles).To(HaveKey(1)) // New bundle

				// Verify empty cache bundle
				cacheBundleData := result.UnsBundles[0]
				var cacheBundle tbproto.UnsBundle
				err := proto.Unmarshal(cacheBundleData, &cacheBundle)
				Expect(err).ToNot(HaveOccurred())
				Expect(cacheBundle.Events.Entries).To(HaveLen(0)) // Empty cache
			})
		})

		Context("timestamp tracking behavior", func() {
			It("should correctly track lastSentTimestamp for existing subscribers", func() {
				// Setup cache
				setupCacheWithMockData(cache, map[string]int64{"topic1": 1000}, time.UnixMilli(1000))
				cache.SetLastSentTimestamp(time.UnixMilli(1500).UTC())

				// Create observed state with multiple timestamps
				obs = createMockObservedState([]*topicbrowserservice.Buffer{
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic2": 2000}), Timestamp: time.UnixMilli(2000)},
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic3": 1800}), Timestamp: time.UnixMilli(1800)},
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic4": 2200}), Timestamp: time.UnixMilli(2200)},
				})

				// Act: Generate for existing subscriber
				result := generator.GenerateTopicBrowser(cache, obs, true, logger)

				// Assert: Should update to latest timestamp
				Expect(result.UnsBundles).To(HaveLen(3))                                   // All bundles are newer than lastSentTimestamp
				Expect(cache.GetLastSentTimestamp()).To(Equal(time.UnixMilli(2200).UTC())) // Latest timestamp
			})

			It("should correctly track lastSentTimestamp for new subscribers", func() {
				// Setup cache with lastCachedTimestamp
				setupCacheWithMockData(cache, map[string]int64{"topic1": 1000}, time.UnixMilli(1500))

				// Create observed state with newer bundles
				obs = createMockObservedState([]*topicbrowserservice.Buffer{
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic2": 2000}), Timestamp: time.UnixMilli(2000)},
					{Payload: createMockUnsBundleBytes(map[string]int64{"topic3": 2100}), Timestamp: time.UnixMilli(2100)},
				})

				// Act: Generate for new subscriber
				result := generator.GenerateTopicBrowser(cache, obs, false, logger)

				// Assert: Should include cache + new bundles and update timestamp
				Expect(result.UnsBundles).To(HaveLen(3))                                   // Cache + 2 new
				Expect(cache.GetLastSentTimestamp()).To(Equal(time.UnixMilli(2100).UTC())) // Latest from new bundles
			})
		})

		Context("edge cases", func() {
			It("should handle empty observed state", func() {
				// Setup cache with data
				setupCacheWithMockData(cache, map[string]int64{"topic1": 1000}, time.UnixMilli(1000))

				// Use empty observed state
				obs = createMockObservedState([]*topicbrowserservice.Buffer{})

				// Test both scenarios
				existingResult := generator.GenerateTopicBrowser(cache, obs, true, logger)
				Expect(existingResult.UnsBundles).To(HaveLen(0))

				newResult := generator.GenerateTopicBrowser(cache, obs, false, logger)
				Expect(newResult.UnsBundles).To(HaveLen(1)) // Only cache bundle
			})

			It("should handle nil cache", func() {
				obs = createMockObservedState([]*topicbrowserservice.Buffer{})

				Expect(func() {
					generator.GenerateTopicBrowser(nil, obs, true, logger)
				}).NotTo(Panic())

				Expect(func() {
					generator.GenerateTopicBrowser(nil, obs, false, logger)
				}).NotTo(Panic())
			})
		})
	})
})

// Helper functions for creating mock data

func setupCacheWithMockData(cache *topicbrowser.Cache, events map[string]int64, lastCachedTimestamp time.Time) {
	// Create mock UnsBundle with the given events
	bundleData := createMockUnsBundleBytes(events)

	// Create observed state with this bundle
	obs := createMockObservedState([]*topicbrowserservice.Buffer{
		{Payload: bundleData, Timestamp: lastCachedTimestamp},
	})

	// Update the cache
	err := cache.Update(obs)
	Expect(err).ToNot(HaveOccurred())
}

func createMockUnsBundleBytes(events map[string]int64) []byte {
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
			UnsTreeId:     treeId,
			ProducedAtMs:  uint64(timestamp),
			PayloadFormat: tbproto.PayloadFormat_TIMESERIES,
			RawKafkaMsg: &tbproto.EventKafka{
				Headers: map[string]string{"source": "test"},
				Payload: []byte("mock payload"),
			},
			BridgedBy: []string{"test-bridge"},
		}
		bundle.Events.Entries = append(bundle.Events.Entries, entry)

		// Add corresponding topic info
		topicInfo := &tbproto.TopicInfo{
			Level0:            "test-enterprise",
			LocationSublevels: []string{"test-site", "test-area"},
			DataContract:      "_historian",
			Name:              treeId,
			Metadata:          map[string]string{"unit": "celsius"},
		}
		bundle.UnsMap.Entries[treeId] = topicInfo
	}

	encoded, err := proto.Marshal(bundle)
	Expect(err).ToNot(HaveOccurred())
	return encoded
}

func createMockObservedState(buffers []*topicbrowserservice.Buffer) *topicbrowserfsm.ObservedStateSnapshot {
	return &topicbrowserfsm.ObservedStateSnapshot{
		ServiceInfo: topicbrowserservice.ServiceInfo{
			Status: topicbrowserservice.Status{
				Buffer: buffers,
			},
		},
	}
}
