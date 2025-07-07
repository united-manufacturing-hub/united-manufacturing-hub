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
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// Parsing utilities for Benthos‑UMH log blocks.
//
// Memory notes: see package documentation for the full ownership model.
// ─ parseBufferPool supplies ephemeral byte slices.
// ─ Decoded data is stored in BufferItems obtained from bufferItemPool.

// s6 log limits (bytes, before hex decoding)
const (
	// s6LineLimit is the maximum number of characters S6 allows in a single log line
	// This is the default S6 configuration limit before truncation occurs
	s6LineLimit = 8192

	// s6Overhead represents additional characters S6 adds to each log line
	// This accounts for line terminators and other S6-specific formatting
	s6Overhead = 1

	// safetyBuffer provides extra buffer space beyond the calculated minimum
	// This prevents buffer reallocation in edge cases and accounts for
	// minor variations in actual log line lengths
	safetyBuffer = 64

	// parseBufferInitialSize is the starting capacity for hex parsing buffers
	// Calculation: ~8 typical S6-truncated entries × 8,193 bytes per entry = 64KB
	// This handles most common batch sizes without reallocation while avoiding
	// excessive memory usage for small batches
	parseBufferInitialSize = 64 << 10 // 64KB
)

// extractNextBlock scans entries starting at searchStart and returns the raw
// hex payload, its timestamp (epoch ms), and index of the ENDENDEND line.
//
// Expected layout:
//
//	STARTSTARTSTART
//	<hex …>
//	ENDDATAENDDATAENDDATA
//	<epoch‑ms>
//	ENDENDEND
//
// Returned raw slice is **caller‑owned** – the function copies it out of the
// pooled buffer before returning.
func extractNextBlock(entries []s6svc.LogEntry, lastProcessedIndex int) ([]byte, int64, int, error) {
	// Handle the case where lastProcessedIndex is <= 0 (either -1 from proper initialization or 0 from default)
	// Start searching from the beginning in this case
	searchStart := lastProcessedIndex + 1
	if lastProcessedIndex <= 0 {
		searchStart = 0
	}

	if searchStart >= len(entries) {
		return nil, 0, lastProcessedIndex, nil // No new entries
	}

	var (
		blockEndIndex = -1
		dataEndIndex  = -1
		startIndex    = -1
	)

	// Search FORWARD from searchStart for the first complete block
	for i := searchStart; i < len(entries); i++ {
		if strings.Contains(entries[i].Content, constants.BLOCK_START_MARKER) {
			startIndex = i
			break
		}
	}
	if startIndex == -1 {
		return nil, 0, lastProcessedIndex, nil // No new block start found
	}

	// Find DATA_END_MARKER after the start
	for i := startIndex + 1; i < len(entries); i++ {
		if strings.Contains(entries[i].Content, constants.DATA_END_MARKER) {
			dataEndIndex = i
			break
		}
	}
	if dataEndIndex == -1 {
		return nil, 0, lastProcessedIndex, nil // Incomplete block
	}

	// Find BLOCK_END_MARKER after the data end
	for i := dataEndIndex + 1; i < len(entries); i++ {
		if strings.Contains(entries[i].Content, constants.BLOCK_END_MARKER) {
			blockEndIndex = i
			break
		}
	}
	if blockEndIndex == -1 {
		return nil, 0, lastProcessedIndex, nil // Incomplete block
	}

	// Extract hex payload using parseBufferPool (see memory management docs above)
	bufPtr := parseBufferPool.Get().(*[]byte)
	buf := *bufPtr

	// Reset buffer and estimate needed capacity based on S6 log line limits
	buf = buf[:0]
	entryCount := dataEndIndex - startIndex - 1

	// Buffer size estimation based on S6 log line limits:
	// - Benthos protobuf logs get truncated to exactly (s6LineLimit + s6Overhead) hex characters per line
	// - Buffer stores raw hex strings (1 byte per character in Go)
	// - After hex decoding: hex strings reduce to ~50% (8,193 hex chars → ~4,096 bytes binary)
	// - Required buffer size: entryCount * (s6LineLimit + s6Overhead + safetyBuffer) bytes for hex strings
	estimatedSize := entryCount * (s6LineLimit + s6Overhead + safetyBuffer) // ~8.2KB per entry

	if cap(buf) < estimatedSize {
		buf = make([]byte, 0, estimatedSize)
	}

	// Concatenate hex data from all lines between start and data-end markers
	// TrimSpace removes newlines, spaces, and other whitespace from each line
	for _, entry := range entries[startIndex+1 : dataEndIndex] {
		trimmed := strings.TrimSpace(entry.Content)
		buf = append(buf, trimmed...)
	}

	// Copy to owned slice since we're returning it
	raw := make([]byte, len(buf))
	copy(raw, buf)

	// Return buffer to pool
	*bufPtr = buf[:0]
	parseBufferPool.Put(bufPtr)

	// Extract timestamp - use TrimSpace to avoid corrupting hex data
	tsLine := ""
	for _, entry := range entries[dataEndIndex+1 : blockEndIndex] {
		s := strings.TrimSpace(entry.Content)
		if s != "" {
			tsLine = s
			break
		}
	}
	if tsLine == "" {
		return nil, 0, lastProcessedIndex, errors.New("timestamp line is missing between block markers")
	}

	epochMS, err := strconv.ParseInt(tsLine, 10, 64)
	if err != nil {
		return nil, 0, lastProcessedIndex, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return raw, epochMS, blockEndIndex, nil
}

// parseBlock pulls the next unprocessed block, decodes it, and appends the
// resulting BufferItem to the ring buffer.
//
// ─ Concurrency ─
// Serialised by svc.processingMutex; ring‑buffer mutation is thread‑safe.
//
// ─ Memory ─
// ▸ Uses bufferItemPool for the struct.
// ▸ payload is written in‑place, no extra copy.
// ▸ On error the item is returned to the pool immediately.
func (svc *Service) parseBlock(entries []s6svc.LogEntry) error {
	svc.processingMutex.Lock()
	defer svc.processingMutex.Unlock()

	// Try to extract the next unprocessed block
	hexBuf, epoch, newBlockEndIndex, err := extractNextBlock(entries, svc.lastProcessedBlockEndIndex)
	if err != nil {
		// Always advance index even on error to prevent infinite loop on corrupt blocks
		svc.lastProcessedBlockEndIndex = newBlockEndIndex
		return err
	}

	// No new blocks available
	if len(hexBuf) == 0 {
		return nil
	}

	// Get pooled BufferItem (ownership transfer pattern - see docs above)
	item := bufferItemPool.Get().(*BufferItem)

	// Decode hex directly into BufferItem payload
	payloadSize := hex.DecodedLen(len(hexBuf))
	if cap(item.Payload) < payloadSize {
		item.Payload = make([]byte, payloadSize)
	} else {
		item.Payload = item.Payload[:payloadSize]
	}

	if _, err := hex.Decode(item.Payload, hexBuf); err != nil {
		// Always advance index even on decode error to prevent infinite loop
		svc.lastProcessedBlockEndIndex = newBlockEndIndex
		bufferItemPool.Put(item) // Return to pool on error
		return fmt.Errorf("hex decode: %w", err)
	}

	// Set timestamp and transfer ownership to ring buffer
	item.Timestamp = time.UnixMilli(epoch)
	svc.ringbuffer.Add(item)

	// Calculate time since last timestamp for debugging
	var timeSinceLastStr string
	if svc.lastProcessedTimestamp.IsZero() {
		timeSinceLastStr = "first block"
	} else {
		timeSinceLast := item.Timestamp.Sub(svc.lastProcessedTimestamp)
		timeSinceLastStr = fmt.Sprintf("%.2fs since last timestamp", timeSinceLast.Seconds())
	}

	// Debug logging with timestamp, sequence number, and timing information
	if svc.logger != nil {
		svc.logger.Debugf("found block with timestamp %s (%s), giving it sequence number %d, and added to ring buffer",
			item.Timestamp.Format(time.RFC3339),
			timeSinceLastStr,
			item.SequenceNum)
	}

	// Update tracking - only after successful processing
	svc.lastProcessedBlockEndIndex = newBlockEndIndex
	svc.lastProcessedTimestamp = item.Timestamp

	return nil
}

// parseBufferPool reuses buffers for hex data concatenation.
// See memory management documentation above for usage patterns.
var parseBufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, parseBufferInitialSize)
		return &b
	},
}
