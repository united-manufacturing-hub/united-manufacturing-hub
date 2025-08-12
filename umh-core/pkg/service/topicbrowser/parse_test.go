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
	"strconv"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// benthos log from samples for topic_browser plugin.
const rawLog = `STARTSTARTSTART
0ac2260aea030a106561363132363366626232303663306512d5030a13656e74657270726973652d6f662d6b696e67731a0a5f686973746f7269616e2a096d795f646174615f3432400a0d6b61666b615f6d73675f6b6579122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3432380a05746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3432390a0a627269646765645f6279122b70726f746f636f6c2d636f6e7665727465722d756e696d706c656d656e7465642d67656e65726174652d31321b0a0d646174615f636f6e7472616374120a5f686973746f7269616e32240a0d6c6f636174696f6e5f706174681213656e74657270726973652d6f662d6b696e677332150a087461675f6e616d6512096d795f646174615f3432230a126b61666b615f74696d657374616d705f6d73120d3137353138383839343138393932160a105f696e697469616c4d6574616461746112027b7d321b0a0b6b61666b615f746f706963120c756d682e6d65737361676573323c0a09756d685f746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f340aea030a103764616531383835383865363162356212d5030a13656e74657270726973652d6f662d6b696e67731a0a5f686973746f7269616e2a096d795f646174615f3632380a05746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3632230a126b61666b615f74696d657374616d705f6d73120d31373531383838393433373233321b0a0b6b61666b615f746f706963120c756d682e6d6573736167657332150a087461675f6e616d6512096d795f646174615f3632240a0d6c6f636174696f6e5f706174681213656e74657270726973652d6f662d6b696e677332160a105f696e697469616c4d6574616461746112027b7d323c0a09756d685f746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3632400a0d6b61666b615f6d73675f6b6579122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f36321b0a0d646174615f636f6e7472616374120a5f686973746f7269616e32390a0a627269646765645f6279122b70726f746f636f6c2d636f6e7665727465722d756e696d706c656d656e7465642d67656e65726174652d310aea030a103739653737646562623130376431393912d5030a13656e74657270726973652d6f662d6b696e67731a0a5f686973746f7269616e2a096d795f646174615f3732160a105f696e697469616c4d6574616461746112027b7d32380a05746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3732240a0d6c6f636174696f6e5f706174681213656e74657270726973652d6f662d6b696e6773323c0a09756d685f746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f37321b0a0d646174615f636f6e7472616374120a5f686973746f7269616e32150a087461675f6e616d6512096d795f646174615f37321b0a0b6b61666b615f746f706963120c756d682e6d6573736167657332390a0a627269646765645f6279122b70726f746f636f6c2d636f6e7665727465722d756e696d706c656d656e7465642d67656e65726174652d3132400a0d6b61666b615f6d73675f6b6579122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3732230a126b61666b615f74696d657374616d705f6d73120d313735313838383934343637380aea030a106562643431383765643836313936656312d5030a13656e74657270726973652d6f662d6b696e67731a0a5f686973746f7269616e2a096d795f646174615f39323c0a09756d685f746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3932160a105f696e697469616c4d6574616461746112027b7d321b0a0d646174615f636f6e7472616374120a5f686973746f7269616e32230a126b61666b615f74696d657374616d705f6d73120d3137353138383839343637333532400a0d6b61666b615f6d73675f6b6579122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3932380a05746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3932150a087461675f6e616d6512096d795f646174615f3932390a0a627269646765645f6279122b70726f746f636f6c2d636f6e7665727465722d756e696d706c656d656e7465642d67656e65726174652d31321b0a0b6b61666b615f746f706963120c756d682e6d6573736167657332240a0d6c6f636174696f6e5f706174681213656e74657270726973652d6f662d6b696e67730aea030a103663616533373937633863386230623512d5030a13656e74657270726973652d6f662d6b696e67731a0a5f686973746f7269616e2a096d795f646174615f30321b0a0b6b61666b615f746f706963120c756d682e6d6573736167657332400a0d6b61666b615f6d73675f6b6579122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3032380a05746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3032390a0a627269646765645f6279122b70726f746f636f6c2d636f6e7665727465722d756e696d706c656d656e7465642d67656e65726174652d3132230a126b61666b615f74696d657374616d705f6d73120d3137353138383839343738313232150a087461675f6e616d6512096d795f646174615f3032160a105f696e697469616c4d6574616461746112027b7d32240a0d6c6f636174696f6e5f706174681213656e74657270726973652d6f662d6b696e6773321b0a0d646174615f636f6e7472616374120a5f686973746f7269616e323c0a09756d685f746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f300aea030a106361303037336238636436643634383112d5030a13656e74657270726973652d6f662d6b696e67731a0a5f686973746f7269616e2a096d795f646174615f3232240a0d6c6f636174696f6e5f706174681213656e74657270726973652d6f662d6b696e677332390a0a627269646765645f6279122b70726f746f636f6c2d636f6e7665727465722d756e696d706c656d656e7465642d67656e65726174652d31321b0a0d646174615f636f6e7472616374120a5f686973746f7269616e32380a05746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3232150a087461675f6e616d6512096d795f646174615f3232400a0d6b61666b615f6d73675f6b6579122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f32323c0a09756d685f746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3232160a105f696e697469616c4d6574616461746112027b7d321b0a0b6b61666b615f746f706963120c756d682e6d6573736167657332230a126b61666b615f74696d657374616d705f6d73120d313735313838383933393638320aea030a103230653838363263343265303635393712d5030a13656e74657270726973652d6f662d6b696e67731a0a5f686973746f7269616e2a096d795f646174615f33321b0a0b6b61666b615f746f706963120c756d682e6d65737361676573321b0a0d646174615f636f6e7472616374120a5f686973746f7269616e32160a105f696e697469616c4d6574616461746112027b7d32150a087461675f6e616d6512096d795f646174615f3332390a0a627269646765645f6279122b70726f746f636f6c2d636f6e7665727465722d756e696d706c656d656e7465642d67656e65726174652d31323c0a09756d685f746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3332400a0d6b61666b615f6d73675f6b6579122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3332380a05746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3332230a126b61666b615f74696d657374616d705f6d73120d3137353138383839343037303232240a0d6c6f636174696f6e5f706174681213656e74657270726973652d6f662d6b696e67730aea030a103635626165343039373630666164653612d5030a13656e74657270726973652d6f662d6b696e67731a0a5f686973746f7269616e2a096d795f646174615f38323c0a09756d685f746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f38321b0a0d646174615f636f6e7472616374120a5f686973746f7269616e321b0a0b6b61666b615f746f706963120c756d682e6d6573736167657332150a087461675f6e616d6512096d795f646174615f3832400a0d6b61666b615f6d73675f6b6579122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3832380a05746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3832390a0a627269646765645f6279122b70726f746f636f6c2d636f6e7665727465722d756e696d706c656d656e7465642d67656e65726174652d3132230a126b61666b615f74696d657374616d705f6d73120d3137353138383839343537313032160a105f696e697469616c4d6574616461746112027b7d32240a0d6c6f636174696f6e5f706174681213656e74657270726973652d6f662d6b696e67730aea030a106632666563363535346535363763353512d5030a13656e74657270726973652d6f662d6b696e67731a0a5f686973746f7269616e2a096d795f646174615f35323c0a09756d685f746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f35321b0a0b6b61666b615f746f706963120c756d682e6d6573736167657332240a0d6c6f636174696f6e5f706174681213656e74657270726973652d6f662d6b696e677332390a0a627269646765645f6279122b70726f746f636f6c2d636f6e7665727465722d756e696d706c656d656e7465642d67656e65726174652d3132400a0d6b61666b615f6d73675f6b6579122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3532160a105f696e697469616c4d6574616461746112027b7d32380a05746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3532230a126b61666b615f74696d657374616d705f6d73120d31373531383838393432373130321b0a0d646174615f636f6e7472616374120a5f686973746f7269616e32150a087461675f6e616d6512096d795f646174615f350aea030a103162653037636534396663356437613712d5030a13656e74657270726973652d6f662d6b696e67731a0a5f686973746f7269616e2a096d795f646174615f3132390a0a627269646765645f6279122b70726f746f636f6c2d636f6e7665727465722d756e696d706c656d656e7465642d67656e65726174652d3132380a05746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3132400a0d6b61666b615f6d73675f6b6579122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3132240a0d6c6f636174696f6e5f706174681213656e74657270726973652d6f662d6b696e677332150a087461675f6e616d6512096d795f646174615f3132160a105f696e697469616c4d6574616461746112027b7d323c0a09756d685f746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f3132230a126b61666b615f74696d657374616d705f6d73120d31373531383838393438373933321b0a0d646174615f636f6e7472616374120a5f686973746f7269616e321b0a0b6b61666b615f746f706963120c756d682e6d65737361676573128b040a88040a103162653037636534396663356437613710012ad4030a400a0d6b61666b615f6d73675f6b6579122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f310a1b0a0d646174615f636f6e7472616374120a5f686973746f7269616e0a150a087461675f6e616d6512096d795f646174615f310a380a05746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f310a3c0a09756d685f746f706963122f756d682e76312e656e74657270726973652d6f662d6b696e67732e5f686973746f7269616e2e6d795f646174615f310a160a105f696e697469616c4d6574616461746112027b7d0a1b0a0b6b61666b615f746f706963120c756d682e6d657373616765730a230a126b61666b615f74696d657374616d705f6d73120d313735313838383934383739330a240a0d6c6f636174696f6e5f706174681213656e74657270726973652d6f662d6b696e67730a390a0a627269646765645f6279122b70726f746f636f6c2d636f6e7665727465722d756e696d706c656d656e7465642d67656e65726174652d3112297b2274696d657374616d705f6d73223a313735313838383934383638362c2276616c7565223a32317d38b9dcdfa5fe325214080128cedbdfa5fe321209090000000000003540
ENDDATAENDDATAENDDATA
1751888949547
ENDENDENDEND`

// --------------------------------------------------------------------------

var _ = Describe("extractRaw / parseBlock", func() {
	var (
		compressed []byte // hex line from rawLog
		payload    []byte // expected decoded payload
		epochMS    int64  // timestamp parsed from rawLog
		rb         *Ringbuffer
		service    *Service
	)

	BeforeEach(func() {
		lines := strings.Split(rawLog, "\n")
		hexLine := strings.TrimSpace(lines[1])           // second line is the hex payload
		tsLine := strings.TrimSpace(lines[len(lines)-2]) // epoch
		var err error
		epochMS, err = strconv.ParseInt(tsLine, 10, 64)
		Expect(err).NotTo(HaveOccurred())

		// decode hex to get the expected payload
		payload, err = hex.DecodeString(hexLine)
		Expect(err).NotTo(HaveOccurred())

		compressed = []byte(hexLine)

		rb = NewRingbuffer(4)
		service = &Service{ringbuffer: rb}
	})

	Context("extraction and hex-decoding", func() {
		It("extracts, hex-decodes and stores the block", func() {
			logs := buildLogs(true, string(compressed), epochMS)

			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1))

			got := rb.GetSnapshot().Items[0]
			Expect(got.Payload).To(Equal(payload))
			Expect(got.Timestamp).To(Equal(time.UnixMilli(epochMS)))
		})
	})

	Context("incomplete block", func() {
		It("returns without error and without writing", func() {
			logs := []s6_shared.LogEntry{
				{Content: constants.BLOCK_START_MARKER},
				{Content: string(compressed)},
			}

			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(0))
		})
	})

	Context("missing timestamp line", func() {
		It("skips block with missing timestamp and continues processing", func() {
			logs := buildLogs(false, string(compressed), epochMS)

			// Should succeed by skipping the bad block
			err := service.parseBlock(logs)
			Expect(err).ToNot(HaveOccurred())
			Expect(rb.Len()).To(Equal(0)) // Block was skipped, nothing added
		})
	})

	Context("corrupt hex data", func() {
		It("skips block with corrupt hex and continues processing", func() {
			logs := buildLogs(true, "not-hex-bytes", epochMS)

			// Should succeed by skipping the bad block
			err := service.parseBlock(logs)
			Expect(err).ToNot(HaveOccurred())
			Expect(rb.Len()).To(Equal(0)) // Block was skipped, nothing added
		})
	})

	Context("resilient processing", func() {
		It("continues processing good blocks after skipping bad ones", func() {
			baseTime := time.UnixMilli(1735732800000) // 2025-01-01T12:00:00.000Z

			// Create mixed logs: good block, bad block, good block
			goodLogs1 := buildLogsWithTimestamps(true, string(compressed), baseTime.UnixMilli())
			badLogs := buildLogsWithTimestamps(true, "invalid-hex", baseTime.Add(5*time.Minute).UnixMilli())
			goodLogs2 := buildLogsWithTimestamps(true, string(compressed), baseTime.Add(10*time.Minute).UnixMilli())

			allLogs := append(append(goodLogs1, badLogs...), goodLogs2...)

			service.ResetBlockProcessing()

			// Process first good block
			Expect(service.parseBlock(allLogs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1)) // First block processed
			Expect(service.lastProcessedTimestamp).To(Equal(baseTime))

			// Process bad block - should be skipped
			Expect(service.parseBlock(allLogs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1))                                                   // Still only one block, bad block skipped
			Expect(service.lastProcessedTimestamp).To(Equal(baseTime.Add(5 * time.Minute))) // Timestamp advanced

			// Process second good block
			Expect(service.parseBlock(allLogs)).To(Succeed())
			Expect(rb.Len()).To(Equal(2)) // Second good block processed
			Expect(service.lastProcessedTimestamp).To(Equal(baseTime.Add(10 * time.Minute)))
		})
	})

	Context("origin benthos block ", func() {
		It("processes the full benthos sample", func() {
			var logs []s6_shared.LogEntry
			for _, l := range strings.Split(rawLog, "\n") {
				l = strings.TrimSpace(l)
				if l != "" {
					logs = append(logs, s6_shared.LogEntry{Content: l})
				}
			}

			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1))

			got := rb.GetSnapshot().Items[0]

			Expect(got.Timestamp).To(Equal(time.UnixMilli(epochMS)))
			Expect(got.Payload).To(Equal(payload))
			// check if e.g. the "umh_topic" exists in the payload
			Expect(string(got.Payload)).To(ContainSubstring("umh_topic"))
		})
	})

	Context("duplicate block processing prevention", func() {
		It("does not process the same block multiple times", func() {
			logs := buildLogs(true, string(compressed), epochMS)

			// First call should process the block
			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1))

			// Second call with same logs should not add another block
			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1)) // Still only 1 block

			// Third call should also not add another block
			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1)) // Still only 1 block
		})

		It("processes new blocks when they appear", func() {
			logs1 := buildLogsWithTimestamps(true, string(compressed), epochMS)
			logs2 := append(logs1, buildLogsWithTimestamps(true, string(compressed), epochMS+1000)...)

			// Process first block
			Expect(service.parseBlock(logs1)).To(Succeed())
			Expect(rb.Len()).To(Equal(1))

			// Process with both blocks - should add the second block
			Expect(service.parseBlock(logs2)).To(Succeed())
			Expect(rb.Len()).To(Equal(2))
		})
	})

	Context("timestamp-based tracking", func() {
		It("processes from beginning when lastProcessedTimestamp is zero", func() {
			// Ensure service starts with zero timestamp
			service.ResetBlockProcessing()
			Expect(service.lastProcessedTimestamp.IsZero()).To(BeTrue())

			logs := buildLogsWithTimestamps(true, string(compressed), epochMS)

			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1))

			// Verify the timestamp was updated
			Expect(service.lastProcessedTimestamp).To(Equal(time.UnixMilli(epochMS)))
		})

		It("only processes blocks with timestamps after lastProcessedTimestamp", func() {
			baseTime := time.UnixMilli(1735732800000) // 2025-01-01T12:00:00.000Z

			// Create logs with different timestamps
			logs1 := buildLogsWithTimestamps(true, string(compressed), baseTime.UnixMilli())
			logs2 := buildLogsWithTimestamps(true, string(compressed), baseTime.Add(5*time.Minute).UnixMilli())
			logs3 := buildLogsWithTimestamps(true, string(compressed), baseTime.Add(10*time.Minute).UnixMilli())

			allLogs := append(append(logs1, logs2...), logs3...)

			// Process first block
			service.ResetBlockProcessing()
			Expect(service.parseBlock(logs1)).To(Succeed())
			Expect(rb.Len()).To(Equal(1))
			Expect(service.lastProcessedTimestamp).To(Equal(baseTime))

			// Process all logs - should find and process the next unprocessed block (logs2)
			Expect(service.parseBlock(allLogs)).To(Succeed())
			Expect(rb.Len()).To(Equal(2)) // 1 original + 1 new
			Expect(service.lastProcessedTimestamp).To(Equal(baseTime.Add(5 * time.Minute)))

			// Process again to get the third block (logs3)
			Expect(service.parseBlock(allLogs)).To(Succeed())
			Expect(rb.Len()).To(Equal(3)) // 1 original + 2 new
			Expect(service.lastProcessedTimestamp).To(Equal(baseTime.Add(10 * time.Minute)))
		})

		It("handles ring buffer wrap scenario correctly", func() {
			baseTime := time.UnixMilli(1735732800000) // 2025-01-01T12:00:00.000Z

			// Simulate processing many blocks to reach ring buffer capacity
			service.ResetBlockProcessing()

			// Process first batch of logs
			logs1 := buildLogsWithTimestamps(true, string(compressed), baseTime.UnixMilli())
			logs2 := buildLogsWithTimestamps(true, string(compressed), baseTime.Add(5*time.Minute).UnixMilli())
			firstBatch := append(logs1, logs2...)

			Expect(service.parseBlock(firstBatch)).To(Succeed())
			Expect(rb.Len()).To(Equal(1)) // Only processes one block per call
			Expect(service.lastProcessedTimestamp).To(Equal(baseTime))

			// Process again to get the second block
			Expect(service.parseBlock(firstBatch)).To(Succeed())
			Expect(rb.Len()).To(Equal(2))
			Expect(service.lastProcessedTimestamp).To(Equal(baseTime.Add(5 * time.Minute)))

			// Now simulate ring buffer wrap by creating new logs that would appear
			// in a wrapped ring buffer but with newer timestamps
			logs3 := buildLogsWithTimestamps(true, string(compressed), baseTime.Add(15*time.Minute).UnixMilli())
			logs4 := buildLogsWithTimestamps(true, string(compressed), baseTime.Add(20*time.Minute).UnixMilli())

			// Create a "wrapped" scenario where the log entries are in a different order
			// but the timestamp-based search should still find the correct entries
			wrappedLogs := append(append(logs3, logs4...), firstBatch...)

			Expect(service.parseBlock(wrappedLogs)).To(Succeed())
			Expect(rb.Len()).To(Equal(3)) // First new block processed
			Expect(service.lastProcessedTimestamp).To(Equal(baseTime.Add(15 * time.Minute)))

			// Process again to get the fourth block
			Expect(service.parseBlock(wrappedLogs)).To(Succeed())
			Expect(rb.Len()).To(Equal(4)) // Should have processed both new blocks
			Expect(service.lastProcessedTimestamp).To(Equal(baseTime.Add(20 * time.Minute)))
		})

		It("handles duplicate timestamps correctly", func() {
			baseTime := time.UnixMilli(1735732800000) // 2025-01-01T12:00:00.000Z

			// Create two blocks with the same timestamp but different content
			logs1 := buildLogsWithTimestamps(true, string(compressed), baseTime.UnixMilli())
			logs2 := buildLogsWithTimestamps(true, "deadbeef", baseTime.UnixMilli())

			allLogs := append(logs1, logs2...)

			service.ResetBlockProcessing()
			Expect(service.parseBlock(allLogs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1)) // First block processed

			// Process again - since second block has same timestamp, it won't be processed
			Expect(service.parseBlock(allLogs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1)) // Still only one block (timestamp-based tracking prevents duplicate timestamps)
		})

		It("skips processing when no newer entries exist", func() {
			baseTime := time.UnixMilli(1735732800000) // 2025-01-01T12:00:00.000Z

			// Process initial block
			logs := buildLogsWithTimestamps(true, string(compressed), baseTime.UnixMilli())
			service.ResetBlockProcessing()
			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1))

			// Process same logs again - should not add anything
			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1)) // No new blocks added

			// Add an older block - should not be processed
			olderLogs := buildLogsWithTimestamps(true, string(compressed), baseTime.Add(-5*time.Minute).UnixMilli())
			allLogsWithOlder := append(olderLogs, logs...)

			Expect(service.parseBlock(allLogsWithOlder)).To(Succeed())
			Expect(rb.Len()).To(Equal(1)) // Still no new blocks added
		})

		It("handles S6 ring buffer wrap at 10k entries correctly", func() {
			// This test simulates the exact scenario that was failing:
			// S6 ring buffer reaches 10,000 entries and wraps, but topic browser
			// can still find new blocks using timestamp-based tracking

			baseTime := time.UnixMilli(1735732800000) // 2025-01-01T12:00:00.000Z
			service.ResetBlockProcessing()

			// Simulate having processed many entries before reaching the wrap point
			// Set lastProcessedTimestamp to a specific time
			service.lastProcessedTimestamp = baseTime.Add(60 * time.Minute)

			// Create a log slice that simulates a wrapped S6 ring buffer
			// In a real scenario, this would be 10,000 entries, but we'll use fewer for testing
			var wrappedLogs []s6_shared.LogEntry

			// Add some "old" entries that were from earlier processing (before wrap)
			for i := range 5 {
				oldTime := baseTime.Add(time.Duration(i*10) * time.Minute)
				wrappedLogs = append(wrappedLogs, buildLogsWithTimestamps(true, string(compressed), oldTime.UnixMilli())...)
			}

			// Add some entries that were processed right before the wrap
			alreadyProcessedTime := baseTime.Add(55 * time.Minute)
			wrappedLogs = append(wrappedLogs, buildLogsWithTimestamps(true, string(compressed), alreadyProcessedTime.UnixMilli())...)

			// Add NEW entries that appeared after the wrap and should be processed
			newTime1 := baseTime.Add(65 * time.Minute) // 5 minutes after lastProcessedTimestamp
			newTime2 := baseTime.Add(70 * time.Minute) // 10 minutes after lastProcessedTimestamp
			wrappedLogs = append(wrappedLogs, buildLogsWithTimestamps(true, string(compressed), newTime1.UnixMilli())...)
			wrappedLogs = append(wrappedLogs, buildLogsWithTimestamps(true, string(compressed), newTime2.UnixMilli())...)

			// Process the wrapped log buffer - should find first new block
			Expect(service.parseBlock(wrappedLogs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1)) // First new block processed

			// Process again to get the second new block
			Expect(service.parseBlock(wrappedLogs)).To(Succeed())
			Expect(rb.Len()).To(Equal(2)) // Both new blocks processed

			// Verify that the service tracked the latest timestamp
			Expect(service.lastProcessedTimestamp).To(Equal(newTime2))

			// Verify the blocks in the ring buffer have the correct timestamps
			snapshot := rb.GetSnapshot()
			Expect(snapshot.Items).To(HaveLen(2))
			// The actual order depends on which block was found first by the search algorithm
			timestamps := []time.Time{snapshot.Items[0].Timestamp, snapshot.Items[1].Timestamp}
			Expect(timestamps).To(ContainElements(newTime1, newTime2))
		})
	})
})

// buildLogs creates a synthetic Benthos log block for tests.  It wraps the
// supplied hex-encoded data line between BLOCK_START / DATA_END / BLOCK_END
// markers and, if includeTimestamp is true, inserts the given epochMS as the
// timestamp line.  The returned slice is ready to be fed into parseBlock.
func buildLogs(includeTimestamp bool, dataLine string, epochMS int64) []s6_shared.LogEntry {
	logs := []s6_shared.LogEntry{
		{Content: constants.BLOCK_START_MARKER},
		{Content: dataLine},
		{Content: constants.DATA_END_MARKER},
	}
	if includeTimestamp {
		logs = append(logs, s6_shared.LogEntry{Content: strconv.FormatInt(epochMS, 10)})
	}

	logs = append(logs, s6_shared.LogEntry{Content: constants.BLOCK_END_MARKER})

	return logs
}

// buildLogsWithTimestamps creates a synthetic Benthos log block for tests with explicit timestamps.
// It wraps the supplied hex-encoded data line between BLOCK_START / DATA_END / BLOCK_END
// markers and sets the given timestamp on all log entries. This is useful for testing
// timestamp-based processing logic.
func buildLogsWithTimestamps(includeTimestamp bool, dataLine string, epochMS int64) []s6_shared.LogEntry {
	timestamp := time.UnixMilli(epochMS)

	logs := []s6_shared.LogEntry{
		{Content: constants.BLOCK_START_MARKER, Timestamp: timestamp},
		{Content: dataLine, Timestamp: timestamp},
		{Content: constants.DATA_END_MARKER, Timestamp: timestamp},
	}
	if includeTimestamp {
		logs = append(logs, s6_shared.LogEntry{Content: strconv.FormatInt(epochMS, 10), Timestamp: timestamp})
	}

	logs = append(logs, s6_shared.LogEntry{Content: constants.BLOCK_END_MARKER, Timestamp: timestamp})

	return logs
}
