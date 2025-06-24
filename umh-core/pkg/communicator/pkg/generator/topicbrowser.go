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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

func GenerateTopicBrowser(cache *topicbrowser.Cache, obs *topicbrowser.ObservedState, isBootstrapped bool) *models.TopicBrowser {
	if isBootstrapped {
		// Existing subscriber: Get only pending bundles from observed state
		return GenerateTbContent(cache, obs)
	} else {
		// New subscriber: Get the full cache PLUS any new bundles since cache was updated
		// First get the content as if for an existing subscriber (new bundles since cache was updated)
		tb := generateTbContentForNewSubscriber(cache, obs)
		return AddCachedBundleToTbContent(cache, tb)
	}
}

// GenerateTbContent generates content for existing subscribers
// Gets only pending bundles that arrived after the last sent timestamp
func GenerateTbContent(cache *topicbrowser.Cache, obs *topicbrowser.ObservedState) *models.TopicBrowser {
	unsBundles := make(map[int][]byte)
	var latestTimestamp int64

	// Get only pending bundles from observed state
	// These are bundles that arrived after the last sent timestamp
	pendingBundles := getPendingBundlesFromObservedState(cache, obs)
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

// AddCachedBundleToTbContent takes existing subscriber content and adds the cached bundle at the beginning
// This is used for new subscribers who need the full cache plus any new bundles
func AddCachedBundleToTbContent(cache *topicbrowser.Cache, tb *models.TopicBrowser) *models.TopicBrowser {

	// Create new unsBundles map with cache bundle at index 0
	newUnsBundles := make(map[int][]byte)

	// Add the full cache at index 0
	newUnsBundles[0] = cache.ToUnsBundleProto()

	// Shift existing bundles to start from index 1
	for i, bundle := range tb.UnsBundles {
		newUnsBundles[i+1] = bundle
	}

	tb.UnsBundles = newUnsBundles
	return tb
}

// generateTbContentForNewSubscriber generates content for new subscribers (without the cache bundle)
// Gets new bundles that arrived after the cache was last updated
func generateTbContentForNewSubscriber(cache *topicbrowser.Cache, obs *topicbrowser.ObservedState) *models.TopicBrowser {
	unsBundles := make(map[int][]byte)
	var latestTimestamp int64

	// Get new bundles that arrived after the cache was last updated
	newBundles := getNewBundlesSinceLastCached(cache, obs)
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

// getPendingBundlesFromObservedState retrieves pending bundles from the observed state
// that haven't been sent to existing subscribers yet (based on lastSentTimestamp)
func getPendingBundlesFromObservedState(cache *topicbrowser.Cache, obs *topicbrowser.ObservedState) [][]byte {
	var pendingBundles [][]byte

	// Get the timestamp of the last bundle sent to subscribers
	lastSentTimestamp := cache.GetLastSentTimestamp()

	for _, buf := range obs.ServiceInfo.Status.Buffer {
		// Only include buffers that are newer than the last sent timestamp
		if buf.Timestamp > lastSentTimestamp {
			pendingBundles = append(pendingBundles, buf.Payload)
		}
	}

	return pendingBundles
}

// getNewBundlesSinceLastCached retrieves bundles that arrived after the cache was last updated
// These are additional bundles that a new subscriber should receive on top of the cache
func getNewBundlesSinceLastCached(cache *topicbrowser.Cache, obs *topicbrowser.ObservedState) [][]byte {
	var newBundles [][]byte

	// Get the timestamp when cache was last updated
	lastCachedTimestamp := getLastCachedTimestamp(cache)

	// Find buffers in observed state that are newer than the cache timestamp
	for _, buf := range obs.ServiceInfo.Status.Buffer {
		if buf.Timestamp > lastCachedTimestamp {
			newBundles = append(newBundles, buf.Payload)
		}
	}

	return newBundles
}

// getLastCachedTimestamp extracts the lastCachedTimestamp from the cache
// This is needed to identify which bundles are newer than what's in the cache
func getLastCachedTimestamp(cache *topicbrowser.Cache) int64 {
	return cache.GetLastCachedTimestamp()
}

// getLatestTimestampFromObservedState finds the latest timestamp in the observed state
// that is greater than the given threshold timestamp
func getLatestTimestampFromObservedState(obs *topicbrowser.ObservedState, thresholdTimestamp int64) int64 {
	var latestTimestamp int64

	for _, buf := range obs.ServiceInfo.Status.Buffer {
		if buf.Timestamp > thresholdTimestamp && buf.Timestamp > latestTimestamp {
			latestTimestamp = buf.Timestamp
		}
	}

	return latestTimestamp
}
