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
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
)

// Parsing utilities for Benthos‑UMH log blocks.
//
// Memory notes: see package documentation for the full ownership model.
// ─ parseBufferPool supplies ephemeral byte slices.
// ─ Decoded data is stored in BufferItems obtained from bufferItemPool.

// s6 log limits (bytes, before hex decoding).
const (
	// s6LineLimit is the maximum number of characters S6 allows in a single log line
	// This is the default S6 configuration limit before truncation occurs.
	s6LineLimit = 8192

	// s6Overhead represents additional characters S6 adds to each log line
	// This accounts for line terminators and other S6-specific formatting.
	s6Overhead = 1

	// safetyBuffer provides extra buffer space beyond the calculated minimum
	// This prevents buffer reallocation in edge cases and accounts for
	// minor variations in actual log line lengths.
	safetyBuffer = 64

	// parseBufferInitialSize is the starting capacity for hex parsing buffers
	// Calculation: ~8 typical S6-truncated entries × 8,193 bytes per entry = 64KB
	// This handles most common batch sizes without reallocation while avoiding
	// excessive memory usage for small batches.
	parseBufferInitialSize = 64 << 10 // 64KB
)

// Pre-compiled byte markers for efficient searching.
var (
	blockStartMarkerBytes = []byte(constants.BLOCK_START_MARKER)
	dataEndMarkerBytes    = []byte(constants.DATA_END_MARKER)
	blockEndMarkerBytes   = []byte(constants.BLOCK_END_MARKER)
)

// maxMarkerLineLength is the maximum expected length of lines containing markers
// Hex payload lines are ~8KB, marker lines are tiny (~50 bytes max).
const maxMarkerLineLength = 100

// containsMarker efficiently checks if content contains the given marker using byte operations.
func containsMarker(content string, marker []byte) bool {
	// Fast path: if line is too long, it can't contain a marker
	if len(content) > maxMarkerLineLength {
		return false
	}

	// Use unsafe conversion to avoid string->[]byte allocation
	// This is safe because we're only reading from the bytes, not modifying them
	contentBytes := unsafe.Slice(unsafe.StringData(content), len(content))

	return bytes.Contains(contentBytes, marker)
}

// extractNextBlock scans entries starting after lastProcessedTimestamp and returns the raw
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
func extractNextBlock(entries []s6_shared.LogEntry, lastProcessedTimestamp time.Time) ([]byte, int64, int, error) {
	// Find the first complete block with timestamp > lastProcessedTimestamp
	// If lastProcessedTimestamp is zero (initial state), start from beginning
	searchStart := 0

	if searchStart >= len(entries) {
		return nil, 0, len(entries) - 1, nil // No new entries
	}

	// Search for complete blocks and check their timestamps
	for searchStart < len(entries) {
		var (
			blockEndIndex = -1
			dataEndIndex  = -1
			startIndex    = -1
		)

		// Search FORWARD from searchStart for the first complete block
		for i := searchStart; i < len(entries); i++ {
			if containsMarker(entries[i].Content, blockStartMarkerBytes) {
				startIndex = i

				break
			}
		}

		if startIndex == -1 {
			return nil, 0, len(entries) - 1, nil // No new block start found
		}

		// Find DATA_END_MARKER after the start
		for i := startIndex + 1; i < len(entries); i++ {
			if containsMarker(entries[i].Content, dataEndMarkerBytes) {
				dataEndIndex = i

				break
			}
		}

		if dataEndIndex == -1 {
			return nil, 0, len(entries) - 1, nil // Incomplete block
		}

		// Find BLOCK_END_MARKER after the data end
		for i := dataEndIndex + 1; i < len(entries); i++ {
			if containsMarker(entries[i].Content, blockEndMarkerBytes) {
				blockEndIndex = i

				break
			}
		}

		if blockEndIndex == -1 {
			return nil, 0, len(entries) - 1, nil // Incomplete block
		}

		// Extract timestamp from this block
		tsLine := ""

		for _, entry := range entries[dataEndIndex+1 : blockEndIndex] {
			s := strings.TrimSpace(entry.Content)
			if s != "" {
				tsLine = s

				break
			}
		}

		if tsLine == "" {
			// Skip this malformed block and continue searching
			searchStart = blockEndIndex + 1

			continue
		}

		epochMS, err := strconv.ParseInt(tsLine, 10, 64)
		if err != nil {
			// Move past this malformed block and continue searching
			searchStart = blockEndIndex + 1

			continue
		}

		blockTimestamp := time.UnixMilli(epochMS)

		// Check if this block should be processed
		if lastProcessedTimestamp.IsZero() || blockTimestamp.After(lastProcessedTimestamp) {
			// Found a block that should be processed - extract it
			return extractBlockData(entries, startIndex, dataEndIndex, blockEndIndex, epochMS)
		}

		// This block is too old, continue searching
		searchStart = blockEndIndex + 1
	}

	// No unprocessed blocks found
	return nil, 0, len(entries) - 1, nil
}

// extractBlockData extracts the hex payload from a complete block.
func extractBlockData(entries []s6_shared.LogEntry, startIndex, dataEndIndex, blockEndIndex int, epochMS int64) ([]byte, int64, int, error) {
	// Extract hex payload using parseBufferPool (see memory management docs above)
	bufPtrRaw := parseBufferPool.Get()

	bufPtr, ok := bufPtrRaw.(*[]byte)
	if !ok {
		return nil, 0, 0, errors.New("parseBufferPool returned unexpected type")
	}

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
func (svc *Service) parseBlock(entries []s6_shared.LogEntry) error {
	svc.processingMutex.Lock()
	defer svc.processingMutex.Unlock()

	// Try to extract the next unprocessed block
	hexBuf, epoch, _, err := extractNextBlock(entries, svc.lastProcessedTimestamp)
	if err != nil {
		// Error occurred, but we don't need to advance index anymore with timestamp-based tracking
		return err
	}

	// No new blocks available
	if len(hexBuf) == 0 {
		return nil
	}

	// Get pooled BufferItem (ownership transfer pattern - see docs above)
	itemRaw := bufferItemPool.Get()

	item, ok := itemRaw.(*BufferItem)
	if !ok {
		return nil
	}

	// Decode hex directly into BufferItem payload
	payloadSize := hex.DecodedLen(len(hexBuf))
	if cap(item.Payload) < payloadSize {
		item.Payload = make([]byte, payloadSize)
	} else {
		item.Payload = item.Payload[:payloadSize]
	}

	// now := time.Now()
	if _, err := hex.Decode(item.Payload, hexBuf); err != nil {
		// Return to pool on error
		bufferItemPool.Put(item)

		// Skip this malformed block and continue processing
		// Update timestamp so we don't get stuck on this bad block
		blockTimestamp := time.UnixMilli(epoch)
		svc.lastProcessedTimestamp = blockTimestamp

		// Log the error for debugging but don't fail
		if svc.logger != nil {
			svc.logger.Warnf("skipping malformed block with timestamp %s due to hex decode error: %v",
				blockTimestamp.Format(time.RFC3339), err)
		}

		return nil // Continue processing next block
	}
	/*
		decodedIn := time.Since(now)
		zap.S().Infof("decoded block in %s, block size (pre-hex): %d, block size (post-hex): %d", decodedIn, len(hexBuf), len(item.Payload))
	*/
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
