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
// The cache (topicbrowser.Cache) maintains exactly one event per topic to provide
// the last known value for each topic. This cache is used to guarantee that new subscribers
// get the last known value for each topic.
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

	// Generate incremental content based on subscriber type
	var tb *models.TopicBrowser
	if isBootstrapped {
		// Existing subscriber: Get bundles newer than lastSentTimestamp
		tb = GenerateTbContent(cache, obs, cache.GetLastSentTimestamp(), logger)
	} else {
		// New subscriber: Get bundles newer than lastCachedTimestamp
		tb = GenerateTbContent(cache, obs, cache.GetLastCachedTimestamp(), logger)
	}

	// For new subscribers, prepend the cached bundle (index 0) with complete topic coverage
	// This ensures new subscribers get both:
	// 1. Complete topic landscape (from cache at index 0)
	// 2. Latest incremental updates (from observed state at index 1+)
	if !isBootstrapped {
		tb = AddCachedBundleToTbContent(cache, tb)
	}

	return tb
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

// GenerateTbContent generates incremental content for subscribers.
//
// BUSINESS LOGIC FOR SUBSCRIBERS:
// - Only send bundles that are newer than the thresholdTimestamp
// - For existing subscribers: thresholdTimestamp = lastSentTimestamp
// - For new subscribers: thresholdTimestamp = lastCachedTimestamp
// - Update lastSentTimestamp after processing to track what was sent
//
// This ensures subscribers receive only incremental updates based on their type,
// preventing duplicate data and maintaining efficient real-time synchronization.
func GenerateTbContent(cache *topicbrowser.Cache, obs *topicbrowser.ObservedState, thresholdTimestamp int64, logger *zap.SugaredLogger) *models.TopicBrowser {
	unsBundles := make(map[int][]byte)
	var latestTimestamp int64

	// Get only pending bundles from observed state
	// These are bundles that arrived after the last sent timestamp
	pendingBundles := getPendingBundlesFromObservedState(obs, thresholdTimestamp, logger)
	for i, bundle := range pendingBundles {
		unsBundles[i] = bundle
	}

	// Update the last sent timestamp with the latest from pending bundles
	latestTimestamp = getLatestTimestampFromObservedState(obs, thresholdTimestamp)

	// Update the cache with the latest timestamp that was sent
	if latestTimestamp > 0 {
		cache.SetLastSentTimestamp(latestTimestamp)
	}

	// Generate health from observed state
	health := generateTopicBrowserHealth(obs)

	return &models.TopicBrowser{
		Health:     health,
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

// getPendingBundlesFromObservedState retrieves bundles for existing subscribers.
//
// FILTERING LOGIC:
// - Uses thresholdTimestamp to determine which bundles are still pending
// - Only includes bundles with timestamp > thresholdTimestamp
// - This ensures subscribers get only new data they haven't seen yet
//
// This implements the incremental update strategy for both subscriber types.
func getPendingBundlesFromObservedState(obs *topicbrowser.ObservedState, thresholdTimestamp int64, logger *zap.SugaredLogger) [][]byte {
	var pendingBundles [][]byte

	for _, buf := range obs.ServiceInfo.Status.Buffer {
		// Only include buffers that are newer than the threshold timestamp
		// This ensures we only send pending (unsent) bundles to subscribers
		if buf.Timestamp > thresholdTimestamp {
			pendingBundles = append(pendingBundles, buf.Payload)
		}
	}

	// Apply bundle count protection
	return validateAndLimitBundles(pendingBundles, logger)
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

// generateTopicBrowserHealth creates a health object from the topic browser observed state.
// This follows the same pattern as other generators (redpanda, dfc, etc.) by extracting
// health information from the observed state's status reason and processing flags.
func generateTopicBrowserHealth(obs *topicbrowser.ObservedState) *models.Health {
	serviceInfo := obs.ServiceInfo

	// Determine health category based on processing state and metrics validity
	healthCat := getTopicBrowserHealthCategory(serviceInfo)

	// Generate detailed status message
	message := getTopicBrowserStatusMessage(serviceInfo)

	return &models.Health{
		Message:       message,
		ObservedState: "running", // to be implemented (get from the instance)
		DesiredState:  "running", // to be implemented (get from the instance)
		Category:      healthCat,
	}
}

// getTopicBrowserHealthCategory determines the health category based on service state
func getTopicBrowserHealthCategory(serviceInfo topicbrowser.ServiceInfo) models.HealthCategory {
	// Check for invalid metrics first (this indicates a problem)
	if serviceInfo.InvalidMetrics {
		return models.Degraded
	}

	// Check processing state
	benthosActive := serviceInfo.BenthosProcessing
	redpandaActive := serviceInfo.RedpandaProcessing

	if benthosActive && redpandaActive {
		return models.Active
	} else if benthosActive || redpandaActive {
		return models.Degraded
	} else {
		return models.Degraded
	}
}

// getTopicBrowserStatusMessage generates a detailed status message for the topic browser
func getTopicBrowserStatusMessage(serviceInfo topicbrowser.ServiceInfo) string {
	// Use status reason if available and meaningful
	if serviceInfo.StatusReason != "" {
		return serviceInfo.StatusReason
	}

	// Generate message based on processing state
	benthosActive := serviceInfo.BenthosProcessing
	redpandaActive := serviceInfo.RedpandaProcessing

	if serviceInfo.InvalidMetrics {
		return "Topic browser has invalid metrics - data flow inconsistency detected"
	} else if benthosActive && redpandaActive {
		return "Topic browser is actively processing data from both Benthos and Redpanda"
	} else if benthosActive {
		return "Topic browser is processing data (Benthos active, Redpanda idle)"
	} else if redpandaActive {
		return "Topic browser is processing data (Redpanda active, Benthos idle)"
	} else {
		return "Topic browser is idle - no active data processing"
	}
}
