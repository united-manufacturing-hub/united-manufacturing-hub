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
	// MaxTopicCount limits the number of topics to prevent memory exhaustion.
	MaxTopicCount = 1_000_000

	// MaxBundleSize limits individual bundle size to 50MB.
	MaxBundleSize = 50 * 1024 * 1024

	// MaxBufferSize limits total buffer size to 200MB (3x 50MB bundles with safety margin).
	MaxBufferSize = 200 * 1024 * 1024
)

// validateBufferSizeFromSnapshot ensures the buffer snapshot doesn't exceed safe limits.
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

// SubscriberData contains data prepared for topic browser subscribers (UI clients).
type SubscriberData struct {
	LatestTimestamp time.Time // Latest timestamp in this subscriber batch

	// UnsBundles holds ready-to-send payloads indexed by position: 0 is the
	// whole cache, 1 and up are queued bundles.
	UnsBundles map[int][]byte

	Summary    string // Human-readable summary for debugging
	TopicCount int    // Current number of topics in cache
}

// queuedBundle is a bundle this package has encoded and is holding for
// delivery; pendingToSend is a slice of these.
//
// It is deliberately not a topicbrowserservice.BufferItem. This package owns a
// queuedBundle's bytes, while a ring-buffer item's belong to the ring buffer,
// which keeps its own reference after handing out a snapshot. Keeping the types
// apart means a ring-buffer item cannot reach pendingToSend. See the Data &
// Ownership Flow section of the pkg/service/topicbrowser package doc.
//
// The per-topic metadata map is dropped on the way in, so the payload differs
// from the ring-buffer item it came from.
type queuedBundle struct {
	// Emission time parsed from the benthos log block, copied from
	// topicbrowserservice.BufferItem.Timestamp. It trails wall clock by the
	// pipeline lag, so a wall-clock value must never be compared against it.
	Timestamp time.Time
	Payload   []byte // re-encoded UnsBundle, allocated here
}

// TopicBrowserCommunicator manages topic browser data flow from ring buffer to UI subscribers
// It handles both the internal cache state and the communication/subscriber management.
type TopicBrowserCommunicator struct {
	// sentWatermark is the position below which bundles are never sent again.
	// MarkDataAsSent advances it, once per notify tick.
	sentWatermark time.Time

	// pendingWatermark is the highest bundle timestamp GetSubscriberData has put
	// into a subscriber message. MarkDataAsSent promotes it to sentWatermark.
	//
	// Both are bundle emission times, never wall clock. See queuedBundle.Timestamp.
	pendingWatermark time.Time

	// 📊 INTERNAL CACHE STATE: The actual topic browser data storage
	eventMap map[string]*tbproto.EventTableEntry // Latest event per topic (key = UnsTreeId)
	unsMap   *tbproto.TopicMap                   // Topic metadata

	// 🎭 SIMULATOR (Optional): For testing/demo purposes
	simulator *Simulator
	logger    *zap.SugaredLogger // Component-specific logging

	// 📡 COMMUNICATION STATE: Subscriber and delivery management
	pendingToSend         []queuedBundle // Bundles not yet sent to subscribers
	lastProcessedSequence uint64         // Last processed buffer sequence number

	// 🔧 CONFIGURATION
	maxPendingBuffers int // Cleanup threshold for old pending buffers
	mu                sync.RWMutex

	simulatorEnabled bool
}

// NewTopicBrowserCommunicator creates a communicator for real FSM data processing.
func NewTopicBrowserCommunicator(logger *zap.SugaredLogger) *TopicBrowserCommunicator {
	return &TopicBrowserCommunicator{
		eventMap:              make(map[string]*tbproto.EventTableEntry),
		unsMap:                &tbproto.TopicMap{Entries: make(map[string]*tbproto.TopicInfo)},
		lastProcessedSequence: 0, // Start from beginning
		pendingToSend:         make([]queuedBundle, 0),
		sentWatermark:         time.Time{},
		simulator:             nil,
		simulatorEnabled:      false,
		maxPendingBuffers:     100, // Default cleanup threshold
		logger:                logger,
	}
}

// NewTopicBrowserCommunicatorWithSimulator creates a communicator with simulator enabled.
func NewTopicBrowserCommunicatorWithSimulator(logger *zap.SugaredLogger) *TopicBrowserCommunicator {
	tbc := NewTopicBrowserCommunicator(logger)
	tbc.simulator = NewSimulator()
	tbc.simulator.InitializeSimulator()
	tbc.simulatorEnabled = true

	return tbc
}

// IsSimulatorEnabled returns whether this communicator is in simulator mode.
func (tbc *TopicBrowserCommunicator) IsSimulatorEnabled() bool {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()

	return tbc.simulatorEnabled
}

// ProcessRealData processes new buffers from FSM observed state
// This handles actual topic browser data from the running system.
func (tbc *TopicBrowserCommunicator) ProcessRealData(obs *topicbrowserfsm.ObservedStateSnapshot) (*ProcessingResult, error) {
	if tbc.simulatorEnabled {
		return nil, errors.New("communicator is in simulator mode, cannot process real data")
	}

	return tbc.processNewBuffers(obs, ProcessingSourceFSM)
}

// ProcessSimulatedData generates and processes simulated topic browser data
// This handles fake data for testing/demo purposes.
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
// It reads BufferItems from obs.ServiceInfo.Status.BufferSnapshot.Items, updates
// the internal cache from each, and queues a queuedBundle for delivery. The
// ring buffer's own items are not referenced after this returns.
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
		result, err = tbc.processIncrementalBuffers(snapshot.NewestN(int(newBufferCount)), source)
	}

	if err != nil {
		return nil, err
	}

	// Update the last processed sequence number
	tbc.lastProcessedSequence = snapshot.LastSequenceNum

	return result, nil
}

// processAllBuffers processes all buffers in the snapshot (used after overwrite detection).
func (tbc *TopicBrowserCommunicator) processAllBuffers(buffers []*topicbrowserservice.BufferItem, source ProcessingSource) (*ProcessingResult, error) {
	result := &ProcessingResult{}

	// buffers arrive oldest first, so the last one ingested is the newest.
	var newestSeq uint64

	for _, buf := range buffers {
		bundle, err := tbc.ingestBuffer(buf)
		if err != nil {
			tbc.logger.Errorf("Failed to process buffer seq=%d: %v", buf.SequenceNum, err)

			result.SkippedCount++

			continue
		}

		result.ProcessedCount++
		newestSeq = buf.SequenceNum

		tbc.pendingToSend = append(tbc.pendingToSend, bundle)

		// Track latest timestamp
		if buf.Timestamp.After(result.LatestTimestamp) {
			result.LatestTimestamp = buf.Timestamp
		}
	}

	result.DebugInfo = fmt.Sprintf("Processed ALL %d buffers from %s after overwrite (seq up to %d), %d errors",
		result.ProcessedCount, source.String(), newestSeq, result.SkippedCount)

	tbc.logger.Debugf("TopicBrowserCommunicator DebugInfo: %s", result.DebugInfo)

	return result, nil
}

// processIncrementalBuffers processes only new buffers since last processing.
func (tbc *TopicBrowserCommunicator) processIncrementalBuffers(buffers []*topicbrowserservice.BufferItem, source ProcessingSource) (*ProcessingResult, error) {
	result := &ProcessingResult{}

	// buffers arrive oldest first, so the range reported below is the first and
	// last entry actually ingested, skipped ones excluded.
	var minSeq, maxSeq uint64

	for _, buf := range buffers {
		bundle, err := tbc.ingestBuffer(buf)
		if err != nil {
			tbc.logger.Errorf("Failed to process buffer seq=%d: %v", buf.SequenceNum, err)

			result.SkippedCount++

			continue
		}

		result.ProcessedCount++
		if result.ProcessedCount == 1 {
			minSeq = buf.SequenceNum
		}

		maxSeq = buf.SequenceNum

		tbc.pendingToSend = append(tbc.pendingToSend, bundle)

		// Track latest timestamp
		if buf.Timestamp.After(result.LatestTimestamp) {
			result.LatestTimestamp = buf.Timestamp
		}
	}

	// Validate topic count after processing
	if len(tbc.eventMap) > MaxTopicCount {
		tbc.logger.Errorf("Topic count %d exceeds maximum limit of %d", len(tbc.eventMap), MaxTopicCount)
		// Don't fail processing, just warn - the data is already processed
	}

	// Generate debug info showing only the sequence range of processed buffers
	var debugInfo string

	if result.ProcessedCount > 0 {
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
	tbc.logger.Debugf("TopicBrowserCommunicator: %s", result.DebugInfo)

	return result, nil
}

// ingestBuffer updates the internal cache maps from one buffer, and returns that
// buffer's bundle re-encoded without the metadata map.
func (tbc *TopicBrowserCommunicator) ingestBuffer(buf *topicbrowserservice.BufferItem) (queuedBundle, error) {
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

		return queuedBundle{}, fmt.Errorf("failed to unmarshal protobuf: %w", err)
	}

	// Update event map: keep only the latest event per topic
	for _, entry := range ub.GetEvents().GetEntries() {
		existing, exists := tbc.eventMap[entry.GetUnsTreeId()]
		if !exists || entry.GetProducedAtMs() > existing.GetProducedAtMs() {
			tbc.eventMap[entry.GetUnsTreeId()] = entry
		}
	}

	// Nothing built from this bundle carries metadata afterwards: not the
	// internal cache below, and not the payload encoded from it.
	removeMetadata(&ub)

	for _, entry := range ub.GetUnsMap().GetEntries() {
		tbc.unsMap.Entries[HashUNSTableEntry(entry)] = entry
	}

	// Encoding on ingest costs one marshal per buffer, so the cost is
	// independent of how many subscribers later read the bundle.
	stripped, err := proto.Marshal(&ub)
	if err != nil {
		context := map[string]interface{}{
			"operation":   "marshal_stripped_bundle",
			"buffer_size": len(buf.Payload),
			"timestamp":   buf.Timestamp,
			"component":   "topic_browser_communicator",
		}
		sentry.ReportIssueWithContext(err, sentry.IssueTypeError, tbc.logger, context)

		return queuedBundle{}, fmt.Errorf("failed to marshal stripped protobuf: %w", err)
	}

	return queuedBundle{
		Timestamp: buf.Timestamp,
		Payload:   stripped,
	}, nil
}

// GetSubscriberData prepares topic browser data for UI subscribers
// For new subscribers (isBootstrapped=false): includes complete cache + incremental data
// For existing subscribers (isBootstrapped=true): includes only incremental data.
func (tbc *TopicBrowserCommunicator) GetSubscriberData(isBootstrapped bool) (*SubscriberData, error) {
	// Write lock: this advances pendingWatermark below.
	tbc.mu.Lock()
	defer tbc.mu.Unlock()

	data := &SubscriberData{
		UnsBundles: make(map[int][]byte),
		TopicCount: len(tbc.unsMap.GetEntries()),
	}

	index := 0

	// Bundles reach a subscriber from two sources. A subscriber that has just
	// connected also gets bundle 0, the whole cache re-encoded by getCacheBundle.
	// Every subscriber gets whatever getUnsentBundles returns: the bundles
	// queued since its last send.
	if !isBootstrapped {
		cacheBundle := tbc.getCacheBundle()
		if cacheBundle != nil {
			data.UnsBundles[0] = cacheBundle
			index = 1
		}

		data.Summary = "Prepared cache bundle + "
	}

	unsent := tbc.getUnsentBundles()

	for _, bundle := range unsent {
		data.UnsBundles[index] = bundle.Payload
		index++

		if bundle.Timestamp.After(data.LatestTimestamp) {
			data.LatestTimestamp = bundle.Timestamp
		}
	}

	if data.LatestTimestamp.After(tbc.pendingWatermark) {
		tbc.pendingWatermark = data.LatestTimestamp
	}

	data.Summary += fmt.Sprintf("%d incremental buffers", len(unsent))
	tbc.logger.Debugf("TopicBrowserCommunicator: %s", data.Summary)

	return data, nil
}

// MarkDataAsSent promotes pendingWatermark, so every bundle GetSubscriberData
// has put in a subscriber message stops being selected.
//
// It takes no timestamp because no caller holds one. GetSubscriberData returns
// the value in SubscriberData.LatestTimestamp, and the status collector drops
// it when building models.TopicBrowser, so the notify loop that calls this has
// no access to it.
func (tbc *TopicBrowserCommunicator) MarkDataAsSent() {
	tbc.mu.Lock()
	defer tbc.mu.Unlock()

	if tbc.pendingWatermark.After(tbc.sentWatermark) {
		tbc.sentWatermark = tbc.pendingWatermark
		tbc.logger.Debugf("Marked data as sent up to timestamp: %s", tbc.sentWatermark.Format(time.RFC3339))
	}

	// Cleanup old pending buffers to prevent memory growth
	tbc.cleanupOldPendingBuffers()
}

// cleanupOldPendingBuffers drops bundles that are no longer pending.
func (tbc *TopicBrowserCommunicator) cleanupOldPendingBuffers() {
	if len(tbc.pendingToSend) <= tbc.maxPendingBuffers {
		return
	}

	filtered := make([]queuedBundle, 0, len(tbc.pendingToSend))

	for _, bundle := range tbc.pendingToSend {
		if bundle.Timestamp.After(tbc.sentWatermark) {
			filtered = append(filtered, bundle)
		}
	}

	dropped := len(tbc.pendingToSend) - len(filtered)

	tbc.pendingToSend = filtered

	tbc.logger.Debugf("Cleaned up %d old pending buffers", dropped)
}

// removeMetadata clears the per-topic metadata map on every topic in the bundle.
// The console reads none of it, and it is the bulk of what a topic costs to
// send.
//
// The entries belong to a bundle the caller has just unmarshaled, so nothing
// else holds them yet and clearing in place is safe.
func removeMetadata(ub *tbproto.UnsBundle) {
	for _, entry := range ub.GetUnsMap().GetEntries() {
		entry.Metadata = nil
	}
}

// getUnsentBundles returns the queued bundles above sentWatermark, in ingest
// order. Caller must hold tbc.mu.
func (tbc *TopicBrowserCommunicator) getUnsentBundles() []queuedBundle {
	unsent := make([]queuedBundle, 0, len(tbc.pendingToSend))

	for _, bundle := range tbc.pendingToSend {
		if bundle.Timestamp.After(tbc.sentWatermark) {
			unsent = append(unsent, bundle)
		}
	}

	return unsent
}

// getCacheBundle returns the complete cache as a protobuf-encoded UnsBundle.
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
	for hash, entry := range tbc.unsMap.GetEntries() {
		ub.UnsMap.Entries[hash] = entry
	}

	// Encode to protobuf
	encoded, err := proto.Marshal(ub)
	if err != nil {
		tbc.logger.Errorf("Failed to marshal cache bundle: %v", err)

		context := map[string]interface{}{
			"operation":    "marshal_cache_bundle",
			"events_count": len(ub.GetEvents().GetEntries()),
			"unsmap_count": len(ub.GetUnsMap().GetEntries()),
			"component":    "topic_browser_communicator",
		}
		sentry.ReportIssueWithContext(err, sentry.IssueTypeError, tbc.logger, context)

		return nil
	}

	return encoded
}

// GetTopicCount returns the current number of topics in the cache.
func (tbc *TopicBrowserCommunicator) GetTopicCount() int {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()

	return len(tbc.unsMap.GetEntries())
}

// GetLastProcessedSequence returns the last processed buffer sequence number (for testing/debugging).
func (tbc *TopicBrowserCommunicator) GetLastProcessedSequence() uint64 {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()

	return tbc.lastProcessedSequence
}

// GetEventMap returns a copy of the internal event map (for testing).
func (tbc *TopicBrowserCommunicator) GetEventMap() map[string]*tbproto.EventTableEntry {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()

	eventMapCopy := make(map[string]*tbproto.EventTableEntry)
	for k, v := range tbc.eventMap {
		eventMapCopy[k] = v
	}

	return eventMapCopy
}

// GetUnsMap returns a copy of the internal topic map (for testing).
func (tbc *TopicBrowserCommunicator) GetUnsMap() *tbproto.TopicMap {
	tbc.mu.RLock()
	defer tbc.mu.RUnlock()

	unsMapCopy := &tbproto.TopicMap{
		Entries: make(map[string]*tbproto.TopicInfo),
	}
	for k, v := range tbc.unsMap.GetEntries() {
		unsMapCopy.Entries[k] = v
	}

	return unsMapCopy
}
