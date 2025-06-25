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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// parseBlock parses the logs, decompresses the payload and sets the timestamp
// You will get a lz4 compressed, hex encoded logline from benthos. This then
// has to get decoded and uncompressed, to write it into the ringbuffer.
// The timestamp is received in ms.
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

	svc.ringbuffer.Add(&Buffer{
		Payload:   payload,
		Timestamp: time.UnixMilli(epoch),
	})

	return nil
}

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
