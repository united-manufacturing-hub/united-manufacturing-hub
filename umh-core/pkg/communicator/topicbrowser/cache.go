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
	"fmt"
	"sync"
	"time"

	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	topicbrowserservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// ProcessingSource indicates the source of topic browser data.
type ProcessingSource int

const (
	ProcessingSourceFSM       ProcessingSource = iota // Real data from FSM observed state
	ProcessingSourceSimulator                         // Simulated data for testing/demo
)

// String returns the string representation of the processing source.
func (ps ProcessingSource) String() string {
	switch ps {
	case ProcessingSourceFSM:
		return "FSM"
	case ProcessingSourceSimulator:
		return "simulator"
	default:
		return "unknown"
	}
}

// ProcessingResult contains the result of processing incremental updates.
type ProcessingResult struct {
	LatestTimestamp time.Time // Latest timestamp from processed buffers
	DebugInfo       string    // Debug information about what was processed
	ProcessedCount  int       // Number of buffers successfully processed
	SkippedCount    int       // Number of buffers skipped due to errors
}

// Cache maintains the latest UnsBundle for each topic key
// This provides fast bootstrap for new subscribers and avoids re-decompressing data.
type Cache struct {
	lastSentTimestamp        time.Time                           // Timestamp of last bundle sent to subscribers
	lastCachedTimestamp      time.Time                           // Timestamp of last cached/processed buffer
	eventMap                 map[string]*tbproto.EventTableEntry // key = UnsTreeId from events
	unsMap                   *tbproto.TopicMap
	lastProcessedSequenceNum uint64 // Sequence number of last processed buffer
	mu                       sync.RWMutex
}

// NewCache creates a new topic browser cache.
func NewCache() *Cache {
	return &Cache{
		eventMap: make(map[string]*tbproto.EventTableEntry),
		unsMap: &tbproto.TopicMap{
			Entries: make(map[string]*tbproto.TopicInfo),
		},
		lastProcessedSequenceNum: 0, // Start at 0 to indicate no buffers processed yet
		lastSentTimestamp:        time.Time{},
		lastCachedTimestamp:      time.Time{},
	}
}

// generateDebugInfo creates debug information about the processing result.
func generateDebugInfo(result *ProcessingResult, dataLost bool, minSequenceNum, maxSequenceNum uint64) {
	if result.ProcessedCount == 0 {
		if dataLost {
			result.DebugInfo = fmt.Sprintf("Data loss detected, processed 0 buffers, skipped %d", result.SkippedCount)
		} else {
			result.DebugInfo = fmt.Sprintf("Processed 0 buffers, skipped %d", result.SkippedCount)
		}
	} else {
		if dataLost {
			result.DebugInfo = fmt.Sprintf("Data loss detected, processed %d buffers (sequence %d to %d), skipped %d",
				result.ProcessedCount, minSequenceNum, maxSequenceNum, result.SkippedCount)
		} else {
			result.DebugInfo = fmt.Sprintf("Processed %d buffers (sequence %d to %d), skipped %d",
				result.ProcessedCount, minSequenceNum, maxSequenceNum, result.SkippedCount)
		}
	}
}

// ProcessIncrementalUpdates processes only new buffers based on sequence numbers
// Uses ring buffer metadata (LastSequenceNum, Count) for efficient processing.
func (c *Cache) ProcessIncrementalUpdates(obs *topicbrowserfsm.ObservedStateSnapshot) (*ProcessingResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	log := logger.For(logger.ComponentCommunicator)

	if obs == nil || obs.ServiceInfo.Status.BufferSnapshot.Items == nil {
		return &ProcessingResult{
			DebugInfo: "No observed state or buffer provided",
		}, nil
	}

	snapshot := obs.ServiceInfo.Status.BufferSnapshot
	buffers := snapshot.Items
	lastSequenceNum := snapshot.LastSequenceNum

	result := &ProcessingResult{}

	// Calculate how many new items we have
	if lastSequenceNum <= c.lastProcessedSequenceNum {
		result.DebugInfo = fmt.Sprintf("No new buffers to process (last processed sequence: %d, current: %d)",
			c.lastProcessedSequenceNum, lastSequenceNum)
		log.Infof("Cache update: %s", result.DebugInfo)

		return result, nil
	}

	newItemCount := lastSequenceNum - c.lastProcessedSequenceNum

	// Check for data loss (gap larger than buffer capacity)
	dataLost := false

	var itemsToProcess []*topicbrowserservice.BufferItem

	if newItemCount > uint64(constants.RingBufferCapacity) {
		// Data loss: we can only process what's in the buffer
		dataLost = true
		availableItems := len(buffers)
		itemsToProcess = buffers[:availableItems] // Process all available items
		lostCount := newItemCount - uint64(availableItems)
		log.Warnf("Data loss detected: gap of %d sequences, lost %d buffers, processing all %d available",
			newItemCount, lostCount, availableItems)
	} else {
		// Normal case: process first newItemCount items (they're newest-to-oldest)
		itemsToProcess = buffers[:newItemCount]
	}

	// Process the selected buffers
	var (
		latestTimestamp                time.Time
		minSequenceNum, maxSequenceNum uint64
	)

	for i, buf := range itemsToProcess {
		// Track latest timestamp for cache management
		if buf.Timestamp.After(latestTimestamp) {
			latestTimestamp = buf.Timestamp
		}

		// Track sequence range for debug info
		if minSequenceNum == 0 || buf.SequenceNum < minSequenceNum {
			minSequenceNum = buf.SequenceNum
		}

		if buf.SequenceNum > maxSequenceNum {
			maxSequenceNum = buf.SequenceNum
		}

		// Process this buffer
		err := c.processBuffer(buf, log)
		if err != nil {
			log.Errorf("Failed to process buffer with sequence %d: %v", buf.SequenceNum, err)

			result.SkippedCount++

			// Log details about first few skipped buffers to avoid spam
			if result.SkippedCount <= 3 {
				log.Warnf("Skipped buffer %d: timestamp=%s, size=%d bytes, error=%v",
					i, buf.Timestamp.Format(time.RFC3339), len(buf.Payload), err)
			}

			continue
		}

		result.ProcessedCount++

		// Track latest timestamp from successfully processed buffers
		if buf.Timestamp.After(result.LatestTimestamp) {
			result.LatestTimestamp = buf.Timestamp
		}
	}

	// Log summary if we skipped more than 3 buffers
	if result.SkippedCount > 3 {
		log.Warnf("... and %d more skipped buffers", result.SkippedCount-3)
	}

	// Update tracking
	c.lastProcessedSequenceNum = lastSequenceNum
	if !latestTimestamp.IsZero() {
		c.lastCachedTimestamp = latestTimestamp
	}

	// Generate debug information
	generateDebugInfo(result, dataLost, minSequenceNum, maxSequenceNum)

	log.Infof("Cache update: %s", result.DebugInfo)

	return result, nil
}

// processBuffer processes a single buffer and updates the cache.
func (c *Cache) processBuffer(buf *topicbrowserservice.BufferItem, log *zap.SugaredLogger) error {
	// Unmarshal the protobuf data
	var ub tbproto.UnsBundle
	err := proto.Unmarshal(buf.Payload, &ub)
	if err != nil {
		// Log the unmarshal error with context and report to Sentry
		log.Errorf("Failed to unmarshal protobuf data in topic browser cache: %v", err)

		context := map[string]interface{}{
			"operation":   "unmarshal_protobuf",
			"buffer_size": len(buf.Payload),
			"timezstamp":  buf.Timestamp,
			"component":   "topic_browser_cache",
		}
		sentry.ReportIssueWithContext(err, sentry.IssueTypeError, log, context)

		return err
	}

	// upsert the latest event by UnsTreeId (key)
	// if the event is newer, we overwrite the existing event
	// if the event is older, we skip it
	for _, entry := range ub.GetEvents().GetEntries() {
		existing, exists := c.eventMap[entry.GetUnsTreeId()]
		if !exists || entry.GetProducedAtMs() > existing.GetProducedAtMs() {
			c.eventMap[entry.GetUnsTreeId()] = entry
		}

		log.Infof("Processed event - UnsTreeId: %s, ProducedAtMs: %d, Payload: %s",
			entry.GetUnsTreeId(), entry.GetProducedAtMs(), entry.GetPayload())
	}

	// upsert the uns map
	for _, entry := range ub.GetUnsMap().GetEntries() {
		// generate a hash from the entry by calling HashUNSTableEntry
		hash := HashUNSTableEntry(entry)
		c.unsMap.Entries[hash] = entry
	}

	return nil
}

// GetPendingBuffers returns buffers that haven't been sent to subscribers yet
// This replaces the timestamp-based filtering in getPendingBundlesFromObservedState.
func (c *Cache) GetPendingBuffers(obs *topicbrowserfsm.ObservedStateSnapshot, thresholdTimestamp time.Time) ([]*topicbrowserservice.BufferItem, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if obs == nil || obs.ServiceInfo.Status.BufferSnapshot.Items == nil {
		return nil, nil
	}

	var pendingBuffers []*topicbrowserservice.BufferItem

	for _, buf := range obs.ServiceInfo.Status.BufferSnapshot.Items {
		if buf.Timestamp.After(thresholdTimestamp) {
			pendingBuffers = append(pendingBuffers, buf)
		}
	}

	return pendingBuffers, nil
}

// this function is used to convert the cache into one proto-encoded UnsBundle.
func (c *Cache) ToUnsBundleProto() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// create a new uns bundle
	ub := &tbproto.UnsBundle{
		Events: &tbproto.EventTable{
			Entries: make([]*tbproto.EventTableEntry, 0, len(c.eventMap)),
		},
		UnsMap: &tbproto.TopicMap{
			Entries: make(map[string]*tbproto.TopicInfo),
		},
	}

	// add the latest events to the uns bundle
	for _, entry := range c.eventMap {
		ub.Events.Entries = append(ub.Events.Entries, entry)
	}

	// add the uns map to the uns bundle
	for _, entry := range c.unsMap.GetEntries() {
		ub.UnsMap.Entries[HashUNSTableEntry(entry)] = entry
	}

	// proto encode the uns bundle
	encoded, err := proto.Marshal(ub)
	if err != nil {
		// Log the marshal error with context and report to Sentry
		log := logger.For(logger.ComponentCommunicator)
		log.Errorf("Failed to marshal UnsBundle to protobuf in topic browser cache: %v", err)

		context := map[string]interface{}{
			"operation":    "marshal_protobuf",
			"events_count": len(ub.GetEvents().GetEntries()),
			"unsmap_count": len(ub.GetUnsMap().GetEntries()),
			"component":    "topic_browser_cache",
		}
		sentry.ReportIssueWithContext(err, sentry.IssueTypeError, log, context)

		return nil
	}

	return encoded
}

// GetKeys returns all the topic keys currently in the cache.
func (c *Cache) GetKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.eventMap))
	for key := range c.eventMap {
		keys = append(keys, key)
	}

	return keys
}

// Size returns the number of topics currently cached.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.unsMap.GetEntries())
}

// GetLastProcessedSequenceNum returns the sequence number of the last processed buffer.
func (c *Cache) GetLastProcessedSequenceNum() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lastProcessedSequenceNum
}

// GetLastSentTimestamp returns the timestamp of the last bundle sent to subscribers.
func (c *Cache) GetLastSentTimestamp() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lastSentTimestamp
}

// SetLastSentTimestamp updates the timestamp of the last bundle sent to subscribers.
func (c *Cache) SetLastSentTimestamp(timestamp time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastSentTimestamp = timestamp
}

// GetEventMap returns a copy of the eventMap for testing purposes.
func (c *Cache) GetEventMap() map[string]*tbproto.EventTableEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	eventMapCopy := make(map[string]*tbproto.EventTableEntry)
	for k, v := range c.eventMap {
		eventMapCopy[k] = v
	}

	return eventMapCopy
}

// GetUnsMap returns a copy of the unsMap for testing purposes.
func (c *Cache) GetUnsMap() *tbproto.TopicMap {
	c.mu.RLock()
	defer c.mu.RUnlock()

	unsMapCopy := &tbproto.TopicMap{
		Entries: make(map[string]*tbproto.TopicInfo),
	}
	for k, v := range c.unsMap.GetEntries() {
		unsMapCopy.Entries[k] = v
	}

	return unsMapCopy
}

// GetLastCachedTimestamp returns the timestamp of the last cached/processed buffer.
func (c *Cache) GetLastCachedTimestamp() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lastCachedTimestamp
}
