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
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

const (
	// parseBufferInitialSize is the initial size for hex data concatenation buffers
	parseBufferInitialSize = 4096

	// maxPayloadBytes limits individual payload size
	maxPayloadBytes = 10 << 20 // 10MiB
)

// extractRaw searches log entries for a complete START‒END block and returns
// the hex-encoded payload plus its epoch-ms timestamp.
// An unfinished block yields (nil, 0, nil); a finished-but-malformed block
// (e.g., missing or unparsable timestamp) yields (nil, 0, nil) for resilient processing.
//
// Logs will come in this format:
// STARTSTARTSTART
// <hex-encoded data>
// ENDDATAENDDATAENDDATA
// 1750091514783
// ENDENDENDEND
func extractRaw(entries []s6svc.LogEntry, lastProcessedTimestamp time.Time) (compressed []byte, epochMS int64, err error) {
	var (
		blockEndIndex = -1
		dataEndIndex  = -1
		startIndex    = -1
	)

	// Find the first complete block that we haven't processed yet
	for i, entry := range entries {
		if strings.Contains(entry.Content, constants.BLOCK_START_MARKER) {
			startIndex = i
		}
		if strings.Contains(entry.Content, constants.DATA_END_MARKER) && startIndex != -1 {
			dataEndIndex = i
		}
		if strings.Contains(entry.Content, constants.BLOCK_END_MARKER) && dataEndIndex != -1 {
			blockEndIndex = i

			// Check if we should process this block based on timestamp
			tsLine := ""
			for _, tsEntry := range entries[dataEndIndex+1 : blockEndIndex] {
				s := strings.TrimSpace(tsEntry.Content)
				if s != "" {
					tsLine = s
					break
				}
			}

			if tsLine != "" {
				if epochMS, err := strconv.ParseInt(tsLine, 10, 64); err == nil {
					blockTimestamp := time.UnixMilli(epochMS)
					// Only process blocks newer than our last processed timestamp
					if blockTimestamp.After(lastProcessedTimestamp) {
						// This is a new block we should process
						break
					}
				}
			}

			// Reset for next iteration
			blockEndIndex = -1
			dataEndIndex = -1
			startIndex = -1
		}
	}

	if blockEndIndex == -1 || dataEndIndex == -1 || startIndex == -1 {
		return nil, 0, nil // no unprocessed block found
	}

	// Use buffer pool for hex data concatenation
	bufPtr := parseBufferPool.Get().(*[]byte)
	buf := *bufPtr
	buf = buf[:0] // Reset buffer but keep capacity

	defer func() {
		*bufPtr = buf[:0] // Reset for next use
		parseBufferPool.Put(bufPtr)
	}()

	// Concatenate hex data from all lines between markers
	for _, entry := range entries[startIndex+1 : dataEndIndex] {
		content := strings.TrimSpace(entry.Content)
		if content != "" {
			buf = append(buf, content...)
		}
	}

	// Find timestamp
	tsLine := ""
	for _, entry := range entries[dataEndIndex+1 : blockEndIndex] {
		s := strings.TrimSpace(entry.Content)
		if s != "" {
			tsLine = s
			break
		}
	}
	if tsLine == "" {
		return nil, 0, nil // Skip block with missing timestamp
	}

	epochMS, err = strconv.ParseInt(tsLine, 10, 64)
	if err != nil {
		return nil, 0, nil // Skip block with invalid timestamp
	}

	// Make a copy of the hex data
	result := make([]byte, len(buf))
	copy(result, buf)

	return result, epochMS, nil
}

func (svc *Service) parseBlock(entries []s6svc.LogEntry) error {
	svc.processingMutex.RLock()
	lastProcessedTimestamp := svc.lastProcessedTimestamp
	svc.processingMutex.RUnlock()

	hexBuf, epoch, err := extractRaw(entries, lastProcessedTimestamp)
	if err != nil {
		return err // Propagate extraction error
	}
	if len(hexBuf) == 0 {
		return nil // No new data to process
	}

	// Hex decode the payload
	payload := make([]byte, hex.DecodedLen(len(hexBuf)))
	n, err := hex.Decode(payload, hexBuf)
	if err != nil {
		// Skip block with invalid hex data but continue processing
		svc.logger.Warnf("Skipping block with invalid hex data: %v", err)
		svc.processingMutex.Lock()
		svc.lastProcessedTimestamp = time.UnixMilli(epoch)
		svc.processingMutex.Unlock()
		return nil
	}
	payload = payload[:n] // Trim to actual decoded length

	if len(payload) > maxPayloadBytes {
		// Skip oversized payload but continue processing
		svc.logger.Warnf("Skipping oversized payload: %d bytes exceed max %d limit", len(payload), maxPayloadBytes)
		svc.processingMutex.Lock()
		svc.lastProcessedTimestamp = time.UnixMilli(epoch)
		svc.processingMutex.Unlock()
		return nil
	}

	// Create buffer item with sequence number
	item := &Buffer{
		Payload:     payload,
		Timestamp:   time.UnixMilli(epoch),
		SequenceNum: svc.ringbuffer.GetNextSequenceNum(),
	}

	// Add to ring buffer
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
	svc.processingMutex.Lock()
	svc.lastProcessedTimestamp = item.Timestamp
	svc.processingMutex.Unlock()

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
