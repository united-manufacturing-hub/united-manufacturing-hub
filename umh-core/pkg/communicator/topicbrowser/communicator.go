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

package topicbrowser

import (
	"errors"
	"fmt"
	"sync"
	"time"

	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	topicbrowserservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	// MaxTopicCount limits the number of topics to prevent memory exhaustion
	MaxTopicCount = 1_000_000

	// MaxBundleSize limits individual bundle size to 10MB
	MaxBundleSize = 10 * 1024 * 1024

	// MaxBufferSize limits total buffer size to 100MB
	MaxBufferSize = 100 * 1024 * 1024
)

// validateBufferSizeFromSnapshot ensures the buffer snapshot doesn't exceed safe limits
func validateBufferSizeFromSnapshot(snapshot topicbrowserservice.RingBufferSnapshot) error {
	totalSize := int64(0)
	for _, buf := range snapshot.Items {
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

// SubscriberData contains data prepared for topic browser subscribers (UI clients)
type SubscriberData struct {
	UnsBundles      map[int][]byte // Ready-to-send data indexed by position (0=cache, 1+=incremental)
	TopicCount      int            // Current number of topics in cache
	LatestTimestamp time.Time      // Latest timestamp in this subscriber batch
	Summary         string         // Human-readable summary for debugging
}

// TopicBrowserCommunicator manages topic browser data flow from ring buffer to UI subscribers
// It handles both the internal cache state and the communication/subscriber management
type TopicBrowserCommunicator struct {
	mu sync.RWMutex

	// 📊 INTERNAL CACHE STATE: The actual topic browser data storage
	eventMap              map[string]*tbproto.EventTableEntry // Latest event per topic (key = UnsTreeId)
	unsMap                *tbproto.TopicMap                   // Topic metadata
	lastProcessedSequence uint64                              // Last processed buffer sequence number

	// 📡 COMMUNICATION STATE: Subscriber and delivery management
	pendingToSend     []*topicbrowserservice.BufferItem // Processed buffers not yet sent to subscribers
	lastSentTimestamp time.Time                         // Last timestamp sent to subscribers

	// 🎭 SIMULATOR (Optional): For testing/demo purposes
	simulator        *Simulator
	simulatorEnabled bool

	// 🔧 CONFIGURATION
	maxPendingBuffers int                // Cleanup threshold for old pending buffers
	logger            *zap.SugaredLogger // Component-specific logging
}

// NewTopicBrowserCommunicator creates a communicator for real FSM data processing
func NewTopicBrowserCommunicator(logger *zap.SugaredLogger) *TopicBrowserCommunicator {
	return &TopicBrowserCommunicator{
		eventMap:              make(map[string]*tbproto.EventTableEntry),
		unsMap:                &tbproto.TopicMap{Entries: make(map[string]*tbproto.TopicInfo)},
		lastProcessedSequence: 0, // Start from beginning
		pendingToSend:         make([]*topicbrowserservice.BufferItem, 0),
		lastSentTimestamp:     time.Time{},
		simulator:             nil,
		simulatorEnabled:      false,
		maxPendingBuffers:     100, // Default cleanup threshold
		logger:                logger,
	}
}

// NewTopicBrowserCommunicatorWithSimulator creates a communicator with simulator enabled
func NewTopicBrowserCommunicatorWithSimulator(logger *zap.SugaredLogger) *TopicBrowserCommunicator {
	tbc := NewTopicBrowserCommunicator(logger)
	tbc.simulator = NewSimulator()
	tbc.simulator.InitializeSimulator()
	tbc.simulatorEnabled = true
	return tbc
}

// IsSimulatorEnabled returns whether this communicator is in simulator mode
func (tbc *TopicBrowserCommunicator) IsSimulatorEnabled() bool {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()
	return tbc.simulatorEnabled
}

// ProcessRealData processes new buffers from FSM observed state
// This handles actual topic browser data from the running system
func (tbc *TopicBrowserCommunicator) ProcessRealData(obs *topicbrowserfsm.ObservedStateSnapshot) (*ProcessingResult, error) {
	if tbc.simulatorEnabled {
		return nil, errors.New("communicator is in simulator mode, cannot process real data")
	}

	return tbc.processNewBuffers(obs, ProcessingSourceFSM)
}

// ProcessSimulatedData generates and processes simulated topic browser data
// This handles fake data for testing/demo purposes
func (tbc *TopicBrowserCommunicator) ProcessSimulatedData() (*ProcessingResult, error) {
	if !tbc.simulatorEnabled {
		return nil, errors.New("simulator not enabled on this communicator")
	}

	// Generate new simulated data
	tbc.simulator.Tick()
	obs := tbc.simulator.GetSimObservedState()

	return tbc.processNewBuffers(obs, ProcessingSourceSimulator)
}

// processNewBuffers handles the core buffer processing logic for both real and simulated data
//
// ## MEMORY MANAGEMENT
// This function implements the consumer responsibility pattern for topicbrowser BufferItems:
// - Gets BufferItems from obs.ServiceInfo.Status.BufferSnapshot.Items
// - Processes the BufferItems for internal cache updates
// - Returns BufferItems to pool immediately after processing since data is stored in internal cache
// - Follows the documented ownership model from topicbrowser package
func (tbc *TopicBrowserCommunicator) processNewBuffers(obs *topicbrowserfsm.ObservedStateSnapshot, source ProcessingSource) (*ProcessingResult, error) {
	tbc.mu.Lock()
	defer tbc.mu.Unlock()

	if obs == nil {
		return &ProcessingResult{
			DebugInfo: "No observed state provided",
		}, nil
	}

	// Use the new structured buffer snapshot
	snapshot := obs.ServiceInfo.Status.BufferSnapshot

	if snapshot.LastSequenceNum == 0 || len(snapshot.Items) == 0 {
		return &ProcessingResult{
			DebugInfo: "No buffers in snapshot",
		}, nil
	}

	// Validate buffer sizes before processing
	if err := validateBufferSizeFromSnapshot(snapshot); err != nil {
		sentry.ReportIssue(err, sentry.IssueTypeError, tbc.logger)
		return nil, fmt.Errorf("buffer size validation failed: %w", err)
	}

	var result *ProcessingResult

	// Process the buffers and update internal cache
	var err error
	if tbc.lastProcessedSequence == 0 {
		// First time processing or reset: process all buffers
		result, err = tbc.processAllBuffers(snapshot.Items, source)
	} else {
		// Incremental processing: only process new buffers
		newBufferCount := snapshot.LastSequenceNum - tbc.lastProcessedSequence
		result, err = tbc.processIncrementalBuffers(snapshot.Items, newBufferCount, source)
	}

	if err != nil {
		return nil, err
	}

	// Update the last processed sequence number
	tbc.lastProcessedSequence = snapshot.LastSequenceNum

	// NOTE: BufferItems are stored in pendingToSend and will be returned to pool
	// after delivery to subscribers (handled by MarkDataAsSent/cleanupOldPendingBuffers)

	return result, nil
}

// processAllBuffers processes all buffers in the snapshot (used after overwrite detection)
func (tbc *TopicBrowserCommunicator) processAllBuffers(buffers []*topicbrowserservice.BufferItem, source ProcessingSource) (*ProcessingResult, error) {
	result := &ProcessingResult{}

	for _, buf := range buffers {
		if err := tbc.updateInternalCache(buf); err != nil {
			tbc.logger.Errorf("Failed to process buffer seq=%d: %v", buf.SequenceNum, err)
			result.SkippedCount++
			continue
		}

		result.ProcessedCount++
		tbc.pendingToSend = append(tbc.pendingToSend, buf)

		// Track latest timestamp
		if buf.Timestamp.After(result.LatestTimestamp) {
			result.LatestTimestamp = buf.Timestamp
		}
	}

	// Update sequence tracking to latest
	if len(buffers) > 0 {
		tbc.lastProcessedSequence = buffers[0].SequenceNum // Newest buffer (first in slice)
	}

	result.DebugInfo = fmt.Sprintf("Processed ALL %d buffers from %s after overwrite (seq up to %d), %d errors",
		result.ProcessedCount, source.String(), tbc.lastProcessedSequence, result.SkippedCount)

	tbc.logger.Debugf("TopicBrowserCommunicator DebugInfo: %s", result.DebugInfo)

	return result, nil
}

// processIncrementalBuffers processes only new buffers since last processing
func (tbc *TopicBrowserCommunicator) processIncrementalBuffers(buffers []*topicbrowserservice.BufferItem, newBufferCount uint64, source ProcessingSource) (*ProcessingResult, error) {
	result := &ProcessingResult{}

	// Buffers are newest-to-oldest, so we need the last N items
	startIndex := len(buffers) - int(newBufferCount)
	if startIndex < 0 {
		startIndex = 0
	}

	for i := startIndex; i < len(buffers); i++ {
		buf := buffers[i]

		if err := tbc.updateInternalCache(buf); err != nil {
			tbc.logger.Errorf("Failed to process buffer seq=%d: %v", buf.SequenceNum, err)
			result.SkippedCount++
			continue
		}

		result.ProcessedCount++
		tbc.pendingToSend = append(tbc.pendingToSend, buf)

		// Track latest timestamp
		if buf.Timestamp.After(result.LatestTimestamp) {
			result.LatestTimestamp = buf.Timestamp
		}
	}

	// Update sequence tracking to latest
	if len(buffers) > 0 {
		tbc.lastProcessedSequence = buffers[0].SequenceNum // Newest buffer (first in slice)
	}

	// Validate topic count after processing
	if len(tbc.eventMap) > MaxTopicCount {
		tbc.logger.Errorf("Topic count %d exceeds maximum limit of %d", len(tbc.eventMap), MaxTopicCount)
		// Don't fail processing, just warn - the data is already processed
	}

	// Generate debug info showing only the sequence range of processed buffers
	var debugInfo string
	if result.ProcessedCount > 0 {
		minSeq := buffers[len(buffers)-1].SequenceNum // Oldest processed buffer
		maxSeq := buffers[startIndex].SequenceNum     // Newest processed buffer
		if minSeq == maxSeq {
			debugInfo = fmt.Sprintf("Processed %d incremental buffers from %s (seq %d), %d errors",
				result.ProcessedCount, source.String(), minSeq, result.SkippedCount)
		} else {
			debugInfo = fmt.Sprintf("Processed %d incremental buffers from %s (seq %d-%d), %d errors",
				result.ProcessedCount, source.String(), minSeq, maxSeq, result.SkippedCount)
		}
	} else {
		debugInfo = fmt.Sprintf("Processed %d incremental buffers from %s, %d errors",
			result.ProcessedCount, source.String(), result.SkippedCount)
	}

	result.DebugInfo = debugInfo
	tbc.logger.Infof("TopicBrowserCommunicator: %s", result.DebugInfo)

	return result, nil
}

// updateInternalCache processes a single buffer and updates the internal cache maps
func (tbc *TopicBrowserCommunicator) updateInternalCache(buf *topicbrowserservice.BufferItem) error {
	// Unmarshal the protobuf data
	var ub tbproto.UnsBundle
	if err := proto.Unmarshal(buf.Payload, &ub); err != nil {
		context := map[string]interface{}{
			"operation":   "unmarshal_protobuf",
			"buffer_size": len(buf.Payload),
			"timestamp":   buf.Timestamp,
			"component":   "topic_browser_communicator",
		}
		sentry.ReportIssueWithContext(err, sentry.IssueTypeError, tbc.logger, context)
		return fmt.Errorf("failed to unmarshal protobuf: %w", err)
	}

	// Update event map: keep only the latest event per topic
	for _, entry := range ub.Events.Entries {
		existing, exists := tbc.eventMap[entry.UnsTreeId]
		if !exists || entry.ProducedAtMs > existing.ProducedAtMs {
			tbc.eventMap[entry.UnsTreeId] = entry
		}
	}

	// Update topic map
	for _, entry := range ub.UnsMap.Entries {
		hash := HashUNSTableEntry(entry)
		tbc.unsMap.Entries[hash] = entry
	}

	return nil
}

// GetSubscriberData prepares topic browser data for UI subscribers
// For new subscribers (isBootstrapped=false): includes complete cache + incremental data
// For existing subscribers (isBootstrapped=true): includes only incremental data
func (tbc *TopicBrowserCommunicator) GetSubscriberData(isBootstrapped bool) (*SubscriberData, error) {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()

	data := &SubscriberData{
		UnsBundles: make(map[int][]byte),
		TopicCount: len(tbc.unsMap.Entries),
	}

	index := 0

	// New subscribers get the complete cache as bundle 0
	if !isBootstrapped {
		cacheBundle := tbc.getCacheBundle()
		if cacheBundle != nil {
			data.UnsBundles[0] = cacheBundle
			index = 1
		}
		data.Summary = "Prepared cache bundle + "
	}

	// Add incremental buffers that haven't been sent yet
	incrementalCount := 0
	for _, buf := range tbc.pendingToSend {
		if buf.Timestamp.After(tbc.lastSentTimestamp) {
			data.UnsBundles[index] = buf.Payload
			index++
			incrementalCount++

			if buf.Timestamp.After(data.LatestTimestamp) {
				data.LatestTimestamp = buf.Timestamp
			}
		}
	}

	data.Summary += fmt.Sprintf("%d incremental buffers", incrementalCount)
	tbc.logger.Infof("TopicBrowserCommunicator: %s", data.Summary)

	return data, nil
}

// MarkDataAsSent marks data as delivered to subscribers and updates the last sent timestamp
// This is used for tracking what data has been successfully delivered
func (tbc *TopicBrowserCommunicator) MarkDataAsSent(timestamp time.Time) {
	tbc.mu.Lock()
	defer tbc.mu.Unlock()

	// Update the last sent timestamp for tracking purposes
	if timestamp.After(tbc.lastSentTimestamp) {
		tbc.lastSentTimestamp = timestamp
		tbc.logger.Debugf("Marked data as sent up to timestamp: %s", timestamp.Format(time.RFC3339))
	}

	// Cleanup old pending buffers to prevent memory growth
	tbc.cleanupOldPendingBuffers()
}

// cleanupOldPendingBuffers removes old buffers from the pending queue and returns them to pool
func (tbc *TopicBrowserCommunicator) cleanupOldPendingBuffers() {
	if len(tbc.pendingToSend) <= tbc.maxPendingBuffers {
		return
	}

	// Separate buffers: keep newer ones, collect older ones for pool return
	var buffersToReturn []*topicbrowserservice.BufferItem
	filtered := make([]*topicbrowserservice.BufferItem, 0, len(tbc.pendingToSend))

	for _, buf := range tbc.pendingToSend {
		if buf.Timestamp.After(tbc.lastSentTimestamp) {
			filtered = append(filtered, buf)
		} else {
			buffersToReturn = append(buffersToReturn, buf)
		}
	}

	tbc.pendingToSend = filtered

	// Return old buffers to pool
	if len(buffersToReturn) > 0 {
		topicbrowserservice.PutBufferItems(buffersToReturn)
		tbc.logger.Debugf("Cleaned up %d old pending buffers and returned them to pool", len(buffersToReturn))
	}
}

// getCacheBundle returns the complete cache as a protobuf-encoded UnsBundle
func (tbc *TopicBrowserCommunicator) getCacheBundle() []byte {
	// Create bundle with all current cache data
	ub := &tbproto.UnsBundle{
		Events: &tbproto.EventTable{
			Entries: make([]*tbproto.EventTableEntry, 0, len(tbc.eventMap)),
		},
		UnsMap: &tbproto.TopicMap{
			Entries: make(map[string]*tbproto.TopicInfo),
		},
	}

	// Add all events from cache
	for _, entry := range tbc.eventMap {
		ub.Events.Entries = append(ub.Events.Entries, entry)
	}

	// Add all topic info from cache
	for hash, entry := range tbc.unsMap.Entries {
		ub.UnsMap.Entries[hash] = entry
	}

	// Encode to protobuf
	encoded, err := proto.Marshal(ub)
	if err != nil {
		tbc.logger.Errorf("Failed to marshal cache bundle: %v", err)
		context := map[string]interface{}{
			"operation":    "marshal_cache_bundle",
			"events_count": len(ub.Events.Entries),
			"unsmap_count": len(ub.UnsMap.Entries),
			"component":    "topic_browser_communicator",
		}
		sentry.ReportIssueWithContext(err, sentry.IssueTypeError, tbc.logger, context)
		return nil
	}

	return encoded
}

// GetTopicCount returns the current number of topics in the cache
func (tbc *TopicBrowserCommunicator) GetTopicCount() int {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()
	return len(tbc.unsMap.Entries)
}

// GetLastProcessedSequence returns the last processed buffer sequence number (for testing/debugging)
func (tbc *TopicBrowserCommunicator) GetLastProcessedSequence() uint64 {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()
	return tbc.lastProcessedSequence
}

// GetEventMap returns a copy of the internal event map (for testing)
func (tbc *TopicBrowserCommunicator) GetEventMap() map[string]*tbproto.EventTableEntry {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()

	eventMapCopy := make(map[string]*tbproto.EventTableEntry)
	for k, v := range tbc.eventMap {
		eventMapCopy[k] = v
	}
	return eventMapCopy
}

// GetUnsMap returns a copy of the internal topic map (for testing)
func (tbc *TopicBrowserCommunicator) GetUnsMap() *tbproto.TopicMap {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()

	unsMapCopy := &tbproto.TopicMap{
		Entries: make(map[string]*tbproto.TopicInfo),
	}
	for k, v := range tbc.unsMap.Entries {
		unsMapCopy.Entries[k] = v
	}
	return unsMapCopy
}
