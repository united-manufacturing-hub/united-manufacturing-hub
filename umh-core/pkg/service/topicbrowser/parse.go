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
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pierrec/lz4/v4"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// extractNextBlock searches for the next complete block after lastProcessedIndex
// Returns: hexPayload, epochMS, newBlockEndIndex, error
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

	// Extract hex payload using the same logic as before
	bufPtr := parseBufferPool.Get().(*[]byte)
	buf := *bufPtr

	// Reset buffer and estimate needed capacity based on S6 log line limits
	buf = buf[:0]
	entryCount := dataEndIndex - startIndex - 1

	// S6 Log Line Limit Analysis:
	// - S6 default max line length: 8,192 bytes + 1 overhead = 8,193 characters
	// - Benthos protobuf logs get truncated to exactly 8,193 hex characters per line
	// - Buffer stores raw hex strings (1 byte per character in Go)
	// - After hex decoding: hex strings reduce to ~50% (8,193 hex chars → ~4,096 bytes binary)
	// - Required buffer size: entryCount * 8,193 bytes for hex strings
	//
	// Previous estimation used 3MB per entry (740x overallocation!)
	// Actual needs: ~8KB per entry for S6-truncated hex strings
	const s6LineLimit = 8192
	const s6Overhead = 1
	const safetyBuffer = 64
	estimatedSize := entryCount * (s6LineLimit + s6Overhead + safetyBuffer) // ~8.2KB per entry

	if cap(buf) < estimatedSize {
		buf = make([]byte, 0, estimatedSize)
	}

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

	// Extract timestamp
	tsLine := ""
	for _, entry := range entries[dataEndIndex+1 : blockEndIndex] {
		s := strings.ToLower(entry.Content)
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

// / extractRaw searches log entries for a complete START‒END block and returns
// the hex-encoded LZ4 payload plus its epoch-ms timestamp.
// An unfinished block yields (nil, 0, nil); a finished-but-malformed block
// (e.g., missing or unparsable timestamp) propagates a descriptive error./
//
// Logs will come in this format:
// STARTSTARTSTART
// <hex-encoded LZ4>
// ENDDATAENDDATAENDDATA
// 1750091514783
// ENDENDENDEND
func extractRaw(entries []s6svc.LogEntry) (compressed []byte, epochMS int64, err error) {
	var (
		blockEndIndex = -1
		dataEndIndex  = -1
		startIndex    = -1
	)

	for i := len(entries) - 1; i >= 0; i-- {
		if strings.Contains(entries[i].Content, constants.BLOCK_END_MARKER) {
			blockEndIndex = i
			break
		}
	}
	if blockEndIndex == -1 {
		return nil, 0, nil // no finished block yet
	}

	for i := blockEndIndex - 1; i >= 0; i-- {
		if strings.Contains(entries[i].Content, constants.DATA_END_MARKER) {
			dataEndIndex = i
			break
		}
	}
	if dataEndIndex == -1 {
		return nil, 0, nil // tail not written yet
	}

	for i := dataEndIndex - 1; i >= 0; i-- {
		if strings.Contains(entries[i].Content, constants.BLOCK_START_MARKER) {
			startIndex = i
			break
		}
	}
	if startIndex == -1 {
		return nil, 0, nil // head not written yet
	}

	// Use pooled buffer with pre-allocation to avoid repeated growth
	bufPtr := parseBufferPool.Get().(*[]byte)
	buf := *bufPtr

	// Reset buffer and estimate needed capacity based on S6 log line limits
	buf = buf[:0]
	entryCount := dataEndIndex - startIndex - 1

	// S6 Log Line Limit Analysis:
	// - S6 default max line length: 8,192 bytes + 1 overhead = 8,193 characters
	// - Benthos protobuf logs get truncated to exactly 8,193 hex characters per line
	// - Buffer stores raw hex strings (1 byte per character in Go)
	// - After hex decoding: hex strings reduce to ~50% (8,193 hex chars → ~4,096 bytes binary)
	// - Required buffer size: entryCount * 8,193 bytes for hex strings
	//
	// Previous estimation used 3MB per entry (740x overallocation!)
	// Actual needs: ~8KB per entry for S6-truncated hex strings
	const s6LineLimit = 8192
	const s6Overhead = 1
	const safetyBuffer = 64
	estimatedSize := entryCount * (s6LineLimit + s6Overhead + safetyBuffer) // ~8.2KB per entry

	if cap(buf) < estimatedSize {
		buf = make([]byte, 0, estimatedSize)
	}

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

	tsLine := ""
	for _, entry := range entries[dataEndIndex+1 : blockEndIndex] {
		s := strings.ToLower(entry.Content)
		if s != "" {
			tsLine = s
			break
		}
	}
	if tsLine == "" {
		return nil, 0, errors.New("timestamp line is missing between block markers")
	}

	epochMS, err = strconv.ParseInt(tsLine, 10, 64)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return raw, epochMS, nil
}

func (svc *Service) parseBlock(entries []s6svc.LogEntry) error {
	svc.processingMutex.Lock()
	defer svc.processingMutex.Unlock()

	// Try to extract the next unprocessed block
	hexBuf, epoch, newBlockEndIndex, err := extractNextBlock(entries, svc.lastProcessedBlockEndIndex)
	if err != nil {
		return err // Return errors for broken blocks to maintain backward compatibility
	}

	// No new blocks available
	if len(hexBuf) == 0 {
		return nil
	}

	// Process the block
	compressed := make([]byte, hex.DecodedLen(len(hexBuf)))
	if _, err := hex.Decode(compressed, hexBuf); err != nil {
		return fmt.Errorf("hex decode: %w", err)
	}

	// Decompress the data before storing in ring buffer
	payload, err := decompressLZ4(compressed)
	if err != nil {
		return err
	}

	// Add decompressed data to ring buffer
	svc.ringbuffer.Add(&Buffer{
		Payload:   payload, // Store decompressed data
		Timestamp: time.UnixMilli(epoch),
	})

	// Update tracking - only after successful processing
	svc.lastProcessedBlockEndIndex = newBlockEndIndex

	return nil
}

// decompressionBufferPool reuses decompression buffers.
var decompressionBufferPool = sync.Pool{
	New: func() any {
		// Start with 64KB buffer, will grow as needed
		b := make([]byte, 0, 64<<10)
		return &b
	},
}

// parseBufferPool reuses buffers for hex data parsing to avoid bytes.Buffer growth
// Based on S6 line limit analysis: typical entries are ~8KB each (8,193 hex chars)
// Start with reasonable size for common cases, will grow as needed
var parseBufferPool = sync.Pool{
	New: func() any {
		// Start with 64KB buffer for hex parsing - handles ~8 typical S6-truncated entries
		// Will grow automatically for larger batches
		b := make([]byte, 0, 64<<10)
		return &b
	},
}

// decompressBlock returns the raw, uncompressed bytes.
//
// It expects a *raw LZ4 block* (no header) and uses the same pool /
// grow-once strategy you already tuned for protobuf bundles.
func decompressBlock(src []byte) ([]byte, error) {
	bufPtr := decompressionBufferPool.Get().(*[]byte)
	buf := *bufPtr

	// LZ4 guarantees that the decompressed size will be less than 255x the compressed size. (https://stackoverflow.com/questions/25740471/lz4-library-decompressed-data-upper-bound-size-estimation)
	need := len(src) * 255
	if cap(buf) < need {
		buf = make([]byte, need)
	}
	buf = buf[:cap(buf)]

	n, err := lz4.UncompressBlock(src, buf)
	if err != nil {
		// put the buffer back before returning the error
		*bufPtr = buf[:0]
		decompressionBufferPool.Put(bufPtr)
		return nil, err
	}

	// copy the useful bytes into a fresh slice we own
	out := make([]byte, n)
	copy(out, buf[:n])

	// zero-len the pooled buffer and return it
	*bufPtr = buf[:0]
	decompressionBufferPool.Put(bufPtr)

	return out, nil
}

// decompressLZ4 recognises:
//   - Raw block with 4-byte length prefix
//   - Raw block without prefix (legacy)   ← handled via decompressBlock
func decompressLZ4(compressed []byte) ([]byte, error) {
	if len(compressed) >= 4 {
		orig := int(binary.LittleEndian.Uint32(compressed[:4]))
		if 0 < orig && orig <= 64<<20 {
			dst := make([]byte, orig)
			if n, err := lz4.UncompressBlock(compressed[4:], dst); err == nil && n == orig {
				return dst, nil
			}
		}
	}

	return decompressBlock(compressed)
}
