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
	"strconv"
	"time"

	"github.com/pierrec/lz4/v4"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("extractRaw / parseBlock", func() {
	var (
		payload    []byte      // original message
		compressed []byte      // LZ4-compressed block
		epochMS    int64       // timestamp used in the test logs
		rb         *Ringbuffer // tiny buffer for the service
		service    *Service    // minimal service with just the ring-buffer
	)

	BeforeEach(func() {
		// create a ringbuffer with length=4
		// then add 1 entry with hex encoded and lz4 compressed example data
		payload = []byte("hello world")

		var buf bytes.Buffer
		w := lz4.NewWriter(&buf)
		_, err := w.Write(payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(w.Close()).To(Succeed())

		compressed = []byte(hex.EncodeToString(buf.Bytes()))

		epochMS = time.Now().UnixMilli()
		rb = NewRingbuffer(4)
		service = &Service{ringbuffer: rb}
	})

	Context("extraction and decompression", func() {
		It("extracts, decompresses and stores the block", func() {
			// confirms that parseBlock can recognise a complete log block, hex-decode
			// and LZ4-decompress the payload, then write exactly one entry to the
			// ring buffer whose bytes and timestamp match the originals.
			logs := buildLogs(true, string(compressed), epochMS)

			err := service.parseBlock(logs)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Len()).To(Equal(1))

			got := rb.Get()[0]
			Expect(string(got.Payload)).To(Equal(string(payload)))
			Expect(got.Timestamp).To(Equal(time.UnixMilli(epochMS)))
		})
	})

	Context("incomplete block", func() {
		It("returns without error and without writing", func() {
			// missing DATA_END + tail markers
			logs := []s6svc.LogEntry{
				{Content: constants.BLOCK_START_MARKER},
				{Content: string(compressed)},
			}

			err := service.parseBlock(logs)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Len()).To(Equal(0))
		})
	})

	Context("missing timestamp line", func() {
		It("fails with a clear error", func() {
			logs := buildLogs(false, string(compressed), epochMS) // no ts line

			err := service.parseBlock(logs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timestamp line is missing between block markers"))
			Expect(rb.Len()).To(Equal(0))
		})
	})

	Context("corrupt compressed data", func() {
		It("propagates the decompression error", func() {
			logs := buildLogs(true, "not-lz4-bytes", epochMS)

			err := service.parseBlock(logs)
			Expect(err).To(HaveOccurred())
			Expect(rb.Len()).To(Equal(0))
		})
	})
})

// buildLogs creates a synthetic Benthos log block for tests.  It wraps the
// supplied hex-encoded data line between BLOCK_START / DATA_END / BLOCK_END
// markers and, if includeTimestamp is true, inserts the given epochMS as the
// timestamp line.  The returned slice is ready to be fed into parseBlock.
func buildLogs(includeTimestamp bool, dataLine string, epochMS int64) []s6svc.LogEntry {
	logs := []s6svc.LogEntry{
		{Content: constants.BLOCK_START_MARKER},
		{Content: dataLine},
		{Content: constants.DATA_END_MARKER},
	}
	if includeTimestamp {
		logs = append(logs, s6svc.LogEntry{Content: strconv.FormatInt(epochMS, 10)})
	}
	logs = append(logs, s6svc.LogEntry{Content: constants.BLOCK_END_MARKER})
	return logs
}
