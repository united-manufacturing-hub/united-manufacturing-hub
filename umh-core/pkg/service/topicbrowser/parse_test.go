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
	"strconv"
	"time"

	"github.com/pierrec/lz4/v4"
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

	// helper to build a full, well-formed log slice
	buildLogs := func(includeTimestamp bool, dataLine string) []s6svc.LogEntry {
		logs := []s6svc.LogEntry{
			{Content: BLOCK_START_MARKER},
			{Content: dataLine},
			{Content: DATA_END_MARKER},
		}
		if includeTimestamp {
			logs = append(logs, s6svc.LogEntry{Content: strconv.FormatInt(epochMS, 10)})
		}
		logs = append(logs, s6svc.LogEntry{Content: BLOCK_END_MARKER})
		return logs
	}

	BeforeEach(func() {
		payload = []byte("hello world")
		dst := make([]byte, lz4.CompressBlockBound(len(payload)))
		n, err := lz4.CompressBlock(payload, dst, nil)
		Expect(err).NotTo(HaveOccurred())
		compressed = dst[:n]

		epochMS = time.Now().UnixMilli()
		rb = NewRingbuffer(4)
		service = &Service{ringbuffer: rb}
	})

	Context("happy path", func() {
		It("extracts, decompresses and stores the block", func() {
			logs := buildLogs(true, string(compressed))

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
				{Content: BLOCK_START_MARKER},
				{Content: string(compressed)},
			}

			err := service.parseBlock(logs)
			Expect(err).NotTo(HaveOccurred())
			Expect(rb.Len()).To(Equal(0))
		})
	})

	Context("missing timestamp line", func() {
		It("fails with a clear error", func() {
			logs := buildLogs(false, string(compressed)) // no ts line

			err := service.parseBlock(logs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timestamp missing"))
			Expect(rb.Len()).To(Equal(0))
		})
	})

	Context("corrupt compressed data", func() {
		It("propagates the decompression error", func() {
			logs := buildLogs(true, "not-lz4-bytes")

			err := service.parseBlock(logs)
			Expect(err).To(HaveOccurred())
			Expect(rb.Len()).To(Equal(0))
		})
	})
})
