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

// 10MiB
const maxPayloadBytes = 10 << 20

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

	var buf bytes.Buffer
	for _, entry := range entries[startIndex+1 : dataEndIndex] {
		buf.WriteString(strings.TrimSpace(entry.Content))
	}
	raw := buf.Bytes()

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
	hexBuf, epoch, err := extractRaw(entries)
	if err != nil || len(hexBuf) == 0 {
		return err // nil or extractor error
	}

	compressed := make([]byte, hex.DecodedLen(len(hexBuf)))
	if _, err := hex.Decode(compressed, hexBuf); err != nil {
		return fmt.Errorf("hex decode: %w", err)
	}

	/*
		payload, err := decompressLZ4(compressed)
		if err != nil {
			return err
		}

		if len(payload) > maxPayloadBytes {
			return fmt.Errorf("payload %d bytes exceed max %d limit", len(payload), maxPayloadBytes)
		}*/

	svc.ringbuffer.Add(&Buffer{
		Payload:   compressed,
		Timestamp: time.UnixMilli(epoch),
	})
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
