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

package generator

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

const (
	// MaxTopicCount limits the number of topics to prevent memory exhaustion
	MaxTopicCount = 1_000_000

	// MaxBundleSize limits individual bundle size to 10MB
	MaxBundleSize = 10 * 1024 * 1024

	// MaxBufferSize limits total buffer size to 100MB
	MaxBufferSize = 100 * 1024 * 1024

	// MaxBundlesPerRequest limits bundles returned in a single request
	MaxBundlesPerRequest = 5
)

// GenerateTopicBrowser is the main entry point for generating TopicBrowser content.
// It implements the core business logic for topic browser cache management:
//
// The cache (topicbrowser.Cache) maintains exactly one event per topic to provide a snapshot
// of all topics with their latest values. This cache is used to efficiently serve
// new subscribers without overwhelming them with historical data.
//
// Two subscriber types are handled differently:
//
// 1. NEW SUBSCRIBERS (isBootstrapped=false):
//   - Get the complete topicbrowser.Cache (one event per topic) at index 0
//   - PLUS any new bundles that arrived after the cache was last updated
//   - This ensures new subscribers get the full topic landscape immediately
//
// 2. EXISTING SUBSCRIBERS (isBootstrapped=true):
//   - Get only pending bundles from observed state
//   - Uses lastSentTimestamp to determine which bundles haven't been sent yet
//   - This provides incremental updates to maintain real-time synchronization
func GenerateTopicBrowser(cache *topicbrowser.Cache, obs *topicbrowser.ObservedState, isBootstrapped bool, logger *zap.SugaredLogger) *models.TopicBrowser {
	// Validate input parameters
	if cache == nil || obs == nil {
		return &models.TopicBrowser{
			Health: &models.Health{
				Message:       "invalid parameters: cache or observed state is nil",
				ObservedState: "error",
				DesiredState:  "running",
				Category:      models.Degraded,
			},
			TopicCount: 0,
			UnsBundles: make(map[int][]byte),
		}
	}
	// Validate topic count to prevent memory exhaustion
	if err := validateTopicCount(cache); err != nil {
		return &models.TopicBrowser{
			Health: &models.Health{
				Message:       err.Error(),
				ObservedState: "error",
				DesiredState:  "running",
				Category:      models.Degraded,
			},
			TopicCount: 0,
			UnsBundles: make(map[int][]byte),
		}
	}

	// Validate buffer size to prevent excessive memory usage
	if err := validateBufferSize(obs); err != nil {
		return &models.TopicBrowser{
			Health: &models.Health{
				Message:       err.Error(),
				ObservedState: "error",
				DesiredState:  "running",
				Category:      models.Degraded,
			},
			TopicCount: cache.Size(),
			UnsBundles: make(map[int][]byte),
		}
	}

	if isBootstrapped {
		// Existing subscriber: Get only pending bundles from observed state
		return GenerateTbContent(cache, obs, logger)
	} else {
		// New subscriber: Get the full cache PLUS any new bundles since cache was updated
		// First get the content as if for an existing subscriber (new bundles since cache was updated)
		tb := generateTbContentForNewSubscriber(cache, obs, logger)
		return AddCachedBundleToTbContent(cache, tb)
	}
}

// validateTopicCount ensures the topic count doesn't exceed safe limits
func validateTopicCount(cache *topicbrowser.Cache) error {
	topicCount := cache.Size()
	if topicCount > MaxTopicCount {
		return fmt.Errorf("topic count %d exceeds maximum limit of %d", topicCount, MaxTopicCount)
	}
	return nil
}

// validateBufferSize ensures the observed state buffer doesn't exceed safe limits
func validateBufferSize(obs *topicbrowser.ObservedState) error {
	totalSize := int64(0)
	for _, buf := range obs.ServiceInfo.Status.Buffer {
		bundleSize := int64(len(buf.Payload))

		// Check individual bundle size
		if bundleSize > MaxBundleSize {
			return fmt.Errorf("bundle size %d bytes exceeds maximum limit of %d bytes", bundleSize, MaxBundleSize)
		}

		totalSize += bundleSize
	}

	// Check total buffer size
	if totalSize > MaxBufferSize {
		return fmt.Errorf("total buffer size %d bytes exceeds maximum limit of %d bytes", totalSize, MaxBufferSize)
	}

	return nil
}

// validateAndLimitBundles ensures bundle count doesn't exceed safe limits
func validateAndLimitBundles(bundles [][]byte, logger *zap.SugaredLogger) [][]byte {
	if len(bundles) > MaxBundlesPerRequest {
		logger.Warnf("bundle count %d exceeds maximum limit of %d", len(bundles), MaxBundlesPerRequest)
		return bundles[:MaxBundlesPerRequest]
	}
	return bundles
}

// GenerateTbContent generates content for existing subscribers who are already bootstrapped.
//
// BUSINESS LOGIC FOR EXISTING SUBSCRIBERS:
// - Only send bundles that haven't been sent yet (pending bundles)
// - Use lastSentTimestamp from cache to filter observed state buffer
// - Update lastSentTimestamp after processing to track what was sent
//
// This ensures existing subscribers receive only incremental updates,
// preventing duplicate data and maintaining efficient real-time synchronization.
func GenerateTbContent(cache *topicbrowser.Cache, obs *topicbrowser.ObservedState, logger *zap.SugaredLogger) *models.TopicBrowser {
	unsBundles := make(map[int][]byte)
	var latestTimestamp int64

	// Get only pending bundles from observed state
	// These are bundles that arrived after the last sent timestamp
	pendingBundles := getPendingBundlesFromObservedState(cache, obs, logger)
	for i, bundle := range pendingBundles {
		unsBundles[i] = bundle
	}

	// Update the last sent timestamp with the latest from pending bundles
	latestTimestamp = getLatestTimestampFromObservedState(obs, cache.GetLastSentTimestamp())

	// Update the cache with the latest timestamp that was sent
	if latestTimestamp > 0 {
		cache.SetLastSentTimestamp(latestTimestamp)
	}

	return &models.TopicBrowser{
		Health:     &models.Health{},
		TopicCount: cache.Size(),
		UnsBundles: unsBundles,
	}
}

// AddCachedBundleToTbContent adds the complete topicbrowser.Cache to existing subscriber content.
//
// BUSINESS LOGIC FOR NEW SUBSCRIBERS:
// - The cached bundle (index 0) contains exactly one event per topic
// - This provides new subscribers with the complete topic landscape immediately
// - Any additional bundles (index 1+) are incremental updates since cache was updated
//
// This two-part approach ensures new subscribers get both:
// 1. Complete topic coverage (from cache)
// 2. Latest updates (from observed state since cache update)
func AddCachedBundleToTbContent(cache *topicbrowser.Cache, tb *models.TopicBrowser) *models.TopicBrowser {

	// Create new unsBundles map with cache bundle at index 0
	newUnsBundles := make(map[int][]byte)

	// Add the full cache at index 0
	// This contains exactly one event per topic for complete topic coverage
	newUnsBundles[0] = cache.ToUnsBundleProto()

	// Shift existing bundles to start from index 1
	// These are incremental updates since the cache was last updated
	for i, bundle := range tb.UnsBundles {
		newUnsBundles[i+1] = bundle
	}

	tb.UnsBundles = newUnsBundles
	return tb
}

// generateTbContentForNewSubscriber generates incremental content for new subscribers.
//
// This function handles the "incremental updates" part for new subscribers:
// - Gets bundles that arrived after the cache was last updated (lastCachedTimestamp)
// - These bundles will be added to the cache bundle to provide complete coverage
// - Updates lastSentTimestamp to track what was sent to the new subscriber
//
// Note: This is called before AddCachedBundleToTbContent, which adds the full cache.
func generateTbContentForNewSubscriber(cache *topicbrowser.Cache, obs *topicbrowser.ObservedState, logger *zap.SugaredLogger) *models.TopicBrowser {
	unsBundles := make(map[int][]byte)
	var latestTimestamp int64

	// Get new bundles that arrived after the cache was last updated
	// These provide incremental updates on top of the cached snapshot
	newBundles := getNewBundlesSinceLastCached(cache, obs, logger)
	for i, bundle := range newBundles {
		unsBundles[i] = bundle
	}

	// Update the last sent timestamp with the latest from new bundles
	latestTimestamp = getLatestTimestampFromObservedState(obs, cache.GetLastCachedTimestamp())

	// Update the cache with the latest timestamp that was sent
	if latestTimestamp > 0 {
		cache.SetLastSentTimestamp(latestTimestamp)
	}

	return &models.TopicBrowser{
		Health:     &models.Health{},
		TopicCount: cache.Size(),
		UnsBundles: unsBundles,
	}
}

// getPendingBundlesFromObservedState retrieves bundles for existing subscribers.
//
// FILTERING LOGIC:
// - Uses lastSentTimestamp to determine which bundles are still pending
// - Only includes bundles with timestamp > lastSentTimestamp
// - This ensures existing subscribers get only new data they haven't seen yet
//
// This implements the incremental update strategy for existing subscribers.
func getPendingBundlesFromObservedState(cache *topicbrowser.Cache, obs *topicbrowser.ObservedState, logger *zap.SugaredLogger) [][]byte {
	var pendingBundles [][]byte

	// Get the timestamp of the last bundle sent to subscribers
	lastSentTimestamp := cache.GetLastSentTimestamp()

	for _, buf := range obs.ServiceInfo.Status.Buffer {
		// Only include buffers that are newer than the last sent timestamp
		// This ensures we only send pending (unsent) bundles to existing subscribers
		if buf.Timestamp > lastSentTimestamp {
			pendingBundles = append(pendingBundles, buf.Payload)
		}
	}

	// Apply bundle count protection
	return validateAndLimitBundles(pendingBundles, logger)
}

// getNewBundlesSinceLastCached retrieves incremental bundles for new subscribers.
//
// FILTERING LOGIC:
// - Uses lastCachedTimestamp to find bundles newer than the cache
// - These bundles represent updates that occurred after the cache snapshot
// - New subscribers need these in addition to the cache for complete coverage
//
// This ensures new subscribers get both the cache snapshot AND latest updates.
func getNewBundlesSinceLastCached(cache *topicbrowser.Cache, obs *topicbrowser.ObservedState, logger *zap.SugaredLogger) [][]byte {
	var newBundles [][]byte

	// Get the timestamp when cache was last updated
	lastCachedTimestamp := getLastCachedTimestamp(cache)

	// Find buffers in observed state that are newer than the cache timestamp
	// These represent incremental updates since the cache was created
	for _, buf := range obs.ServiceInfo.Status.Buffer {
		if buf.Timestamp > lastCachedTimestamp {
			newBundles = append(newBundles, buf.Payload)
		}
	}

	// Apply bundle count protection
	return validateAndLimitBundles(newBundles, logger)
}

// getLastCachedTimestamp extracts the lastCachedTimestamp from the cache.
//
// The lastCachedTimestamp represents when the TbCache was last updated with
// the "one event per topic" snapshot. This timestamp is used to identify
// which bundles in the observed state are newer than the cached data.
func getLastCachedTimestamp(cache *topicbrowser.Cache) int64 {
	return cache.GetLastCachedTimestamp()
}

// getLatestTimestampFromObservedState finds the most recent timestamp in observed state.
//
// TIMESTAMP TRACKING:
// - Scans observed state buffer for timestamps > thresholdTimestamp
// - Returns the latest timestamp found above the threshold
// - Used to update lastSentTimestamp after processing bundles
//
// This enables proper tracking of what data has been sent to subscribers.
func getLatestTimestampFromObservedState(obs *topicbrowser.ObservedState, thresholdTimestamp int64) int64 {
	var latestTimestamp int64

	for _, buf := range obs.ServiceInfo.Status.Buffer {
		// Find the latest timestamp that's newer than the threshold
		// This helps track the most recent data that was processed
		if buf.Timestamp > thresholdTimestamp && buf.Timestamp > latestTimestamp {
			latestTimestamp = buf.Timestamp
		}
	}

	return latestTimestamp
}
