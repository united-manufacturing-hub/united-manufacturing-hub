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
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pierrec/lz4/v4"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

var lz4FrameMagic = []byte{0x04, 0x22, 0x4d, 0x18}

// / extractRaw searches log entries for a complete START‒END block and returns
// the hex-encoded LZ4 payload plus its epoch-ms timestamp.
// An unfinished block yields (nil, 0, nil); a finished-but-malformed block
// (e.g., missing or unparsable timestamp) propagates a descriptive error./
//
// Logs will come in this format:
// STARTSTARTSTART
// 042290f3a1b060f0708340f5602050fc2036c0f6f02000f1803040f6a02080f90600d2f9fe86a02190f2a01220f39024b0f2601000f1702340f5703530f0a064f0f5b030c0ffb0e1a0f6a02080f24180d2f87f06a02310f9501530f1601050f5a02340f92010a0f2f034b0f4b05050f8702340f5202720f3f0d030f6a02050f57024e0ff61f000f6a02071f3324180d2fa0e56a02310f0d01050eab000fab2d0b0f95024b0f10030a0fe801050f8302530f36024f0f9203330f6a02080fd8900d2f88ed6a02190f27020a0f9401530f3201330f123f500fbb02050f4e034b0ff403050f66030c0f550d150f6a02081f323e070c2ff1f46a02190fa001510ff500030f6102340f6505020fba11080f3e03530f7d024d0f96071b0f4f0
// ENDDATAENDDATENDDATA
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
		return nil, 0, err
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

	payload, err := decompressLZ4(compressed)
	if err != nil {
		return err
	}

	svc.ringbuffer.Add(&Buffer{
		Payload:   payload,
		Timestamp: time.UnixMilli(epoch),
	})
	return nil
}

// decompressionBufferPool reuses decompression buffers.
var decompressionBufferPool = sync.Pool{
	New: func() any {
		// Start with 64KB buffer, will grow as needed
		b := make([]byte, 0, 64*1024)
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

	need := len(src) * 4
	if cap(buf) < need {
		buf = make([]byte, need)
	}
	buf = buf[:cap(buf)]

	n, err := lz4.UncompressBlock(src, buf)
	if err == lz4.ErrInvalidSourceShortBuffer {
		buf = make([]byte, len(src)*8)
		n, err = lz4.UncompressBlock(src, buf)
	}
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
//   - LZ4 *frame*
//   - Raw block with 4-byte length prefix
//   - Raw block without prefix (legacy)   ← handled via decompressBlock
func decompressLZ4(compressed []byte) ([]byte, error) {
	// ── frame ────────────────────────────────────────────────────────────
	if bytes.HasPrefix(compressed, lz4FrameMagic) {
		r := lz4.NewReader(bytes.NewReader(compressed))
		return io.ReadAll(r)
	}

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
