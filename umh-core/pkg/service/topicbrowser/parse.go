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
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/pierrec/lz4/v4"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// parseBlock parses the logs, decompresses the payload and sets the timestamp
func (svc *Service) parseBlock(entries []s6svc.LogEntry) error {
	hexBuf, epoch, err := extractRaw(entries)
	if err != nil || len(hexBuf) == 0 {
		return err
	}

	compressed := make([]byte, hex.DecodedLen(len(hexBuf)))
	if _, err = hex.Decode(compressed, hexBuf); err != nil {
		return err
	}

	r := lz4.NewReader(bytes.NewReader(compressed))
	payload, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	// LZ4 block-decompression
	//	dst := make([]byte, len(raw)*4)
	//	n, err := lz4.UncompressBlock(raw, dst)
	//	if err == lz4.ErrInvalidSourceShortBuffer {
	//		dst = make([]byte, len(raw)*8)
	//		n, err = lz4.UncompressBlock(raw, dst)
	//	}
	//	if err != nil {
	//		return err
	//	}

	svc.ringbuffer.Add(&Buffer{
		Payload:   payload,
		Timestamp: time.UnixMilli(epoch),
	})

	return nil
}

// extracts the payload (still lz4-compressed) plus the epoch-ms timestamp.
func extractRaw(entries []s6svc.LogEntry) (compressed []byte, epochMS int64, err error) {
	var (
		blockEndIndex = -1
		dataEndIndex  = -1
		startIndex    = -1
	)

	for i := len(entries) - 1; i >= 0; i-- {
		if strings.Contains(entries[i].Content, BLOCK_END_MARKER) {
			blockEndIndex = i
			break
		}
	}
	if blockEndIndex == -1 {
		return nil, 0, nil // no finished block yet
	}

	for i := blockEndIndex - 1; i >= 0; i-- {
		if strings.Contains(entries[i].Content, DATA_END_MARKER) {
			dataEndIndex = i
			break
		}
	}
	if dataEndIndex == -1 {
		return nil, 0, nil // tail not written yet
	}

	for i := dataEndIndex - 1; i >= 0; i-- {
		if strings.Contains(entries[i].Content, BLOCK_START_MARKER) {
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
		return nil, 0, errors.New("timestamp missing in block")
	}

	epochMS, err = strconv.ParseInt(tsLine, 10, 64)
	if err != nil {
		return nil, 0, err
	}

	return raw, epochMS, nil
}
