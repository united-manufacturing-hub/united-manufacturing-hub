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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// benthos log from samples for topic_browser plugin
const rawLog = `STARTSTARTSTART
f68c0afa0f0a92050a106532373539323530663831323635613912fd040a0a656e74657270726973651206706c616e7431120d6d616368696e696e67417265611208636e632d6c696e651204636e63351206706c633132331a0a5f686973746f7269616e2206617869732e792a0576616c756532160a0c7669727475616c5f706174681206617869732e79325c0a05746f7069631253756d682e76312e7e00122e7d00192e7c00142e7b00622e636e63352e7900162e7800122e7700142e7600ff170a627269646765645f62791208756d685f636f726532640a0d6b61666b615f6d73675f6b65797e0043d2110a087461675f6e616d6512059100ff03450a0d6c6f636174696f6e5f706174681234f00021f20d32160a105f696e697469616c4d6574616461746112027b7d32230a12d800f31074696d657374616d705f6d73120d31373530373034323230353738321b0a0b2500f30a6f706963120c756d682e6d6573736167657332600a09756d681b000f160142f6031b0a0d646174615f636f6e7472616374120abe01ff080a82050a103537303737353432323137656333646212ed950221098d02c30a617869732e717569636b2a8a01185de9001f50e9002206b80002760253717569636b7a020fe600094232610a0da701047f020f80003f145958030f5b003f1c3ec4021f2dc4021a0f1703011b1a1e0407b0010fec02060fdc03050f04030e3f39343404030aff090add050a103864366434303137623665333433396212c8058502200f1a050202dd00f80a77726f6e672a106f6e5f707572706f73655f77726f6e67326f98021f62bd01220f1505046d77726f6e672e71000f30010c7c3139393738327331050f9a0051146bc4020f6d00510f9a05340f5902090fe7020402f8010fe70205171c45060e2e020f0503050f7f04095f12b8490a896805006f10012ad1040a3a00091f0afb032c1f0a1e01091f0ab304471f0a7901036e717569636b0a8f041f0aa0054f1f0a76030c6f32303934340ade01041f0abc064b1f0a8b02044812347b22580826223a5808f7213834322c2276616c7565223a2268656c6c6f20776f726c64227d3890d6e9f0f9325218080228aad5e9f0f9321a0d0a0b23000f8c02080fb701090fa500040fa302300f9f08080f9101050fce01120fe002480f11024c0f2303630f2d04090f8c020a2f37338c02101fc98c021c0f0401000f0e02050f3f04750f0b032f0f290c070f2504050f6b050a0fa7024c0f6103480f1604090f8c025f0fd9012d0f88014c0f2d01090f20020a0f5103680fec020a0f5902480f5704000f4d06120f7f03040f8c020a2f39381805101fe21805310f9e10020fb7084d0f5a02090fc201480fb502050fc701120f2b06500fda020a0f4a042d0feb03090f8c025f0f8e00090f5f0c490fd9024c0fa5012d0f9003050f3103090f9902500f3903050f1305000fc4020a0f6903110f8c020b1f391805101fe318051c0f5d01090ffa00050fa901050f60024c0f7101000fff01500fb701110f31100b0f18032d0f1204480f8a04090f8c02092f3930bc0c111fe78c021c0f8e00090fc90b060f5e012d0fe502050f1903090f8a02630f65020a0fa702120f4a02480ffc034b0f8c020a2f31341805101ff28c02390f4801480fd204750f1703050ffb02000f2a03090fb603450f0c030a0f8c02682f32318c02101ff98c021c0fd0004b0f8b142e0f1302000fc602500fa102050f1a020a0f2605120fb402050f9f060a0f0503090f2f04470f18050a1f32300a112f81d66014012e85065b1c6f10022aa4050af81b5d0f5d18060feb1b611f0aeb1b591f0aeb1b330f9c0a0b0f11030c6f31383935370adc1b0a0f9909060f2f03209c77726f6e67123f7b22411c0bae191f2ce11904ff0131383930327d38cdc6e9f0f9325a410a4a002d0f0803080f39010d4f39393738c201350f0303050f62010a0fab025a0f0402070f9b17080f57020b0f5105035f77726f6e67fc03630ffa045d0f0803298f393930317d38cace08032f7f393930317d0a958527006f10012add040a19274a280a6074010fe725410f1e052b4f393537370005060ffd03340f0428520f6607060fdc09000f2329040f92210a0fa808075f3139353136340b0525b9cb941f2ffccaa808010f9802070fb900180fc101050f1b020a0fde01340f3501090ffb01540fc203ad0fbb02050fa2070c5f32303537389802074f32303531e4120625a2d3980220e1d29802002c22b068656c6c6f20776f726c64
ENDDATAENDDATAENDDATA
1750704221069
ENDENDENDEND`

// --------------------------------------------------------------------------

var _ = Describe("extractRaw / parseBlock", func() {
	var (
		payload    []byte // uncompressed bytes
		compressed []byte // hex line from rawLog
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

		// decode and decompress so we know the expected payload
		compressedBytes, err := hex.DecodeString(hexLine)
		Expect(err).NotTo(HaveOccurred())

		payload, err = decompressLZ4(compressedBytes)
		Expect(err).NotTo(HaveOccurred())

		compressed = []byte(hexLine)

		rb = NewRingbuffer(4)
		service = &Service{ringbuffer: rb}
	})

	Context("extraction and decompression", func() {
		It("extracts, decompresses and stores the block", func() {
			logs := buildLogs(true, string(compressed), epochMS)

			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1))

			got := rb.Get()[0]
			Expect(got.Payload).To(Equal(payload))
			Expect(got.Timestamp).To(Equal(time.UnixMilli(epochMS)))
		})
	})

	Context("incomplete block", func() {
		It("returns without error and without writing", func() {
			logs := []process_shared.LogEntry{
				{Content: constants.BLOCK_START_MARKER},
				{Content: string(compressed)},
			}

			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(0))
		})
	})

	Context("missing timestamp line", func() {
		It("fails with a clear error", func() {
			logs := buildLogs(false, string(compressed), epochMS)

			err := service.parseBlock(logs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timestamp line is missing"))
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

	Context("origin benthos block ", func() {
		It("processes the full benthos sample", func() {
			var logs []process_shared.LogEntry
			for _, l := range strings.Split(rawLog, "\n") {
				l = strings.TrimSpace(l)
				if l != "" {
					logs = append(logs, process_shared.LogEntry{Content: l})
				}
			}

			Expect(service.parseBlock(logs)).To(Succeed())
			Expect(rb.Len()).To(Equal(1))

			got := rb.Get()[0]

			// NOTE: uncomment for showing decompressed data
			// GinkgoWriter.Printf("\n[DEBUG] uncompressed (%d bytes):\n%s\n\n",
			//	len(got.Payload), string(got.Payload))

			Expect(got.Timestamp).To(Equal(time.UnixMilli(epochMS)))
			Expect(got.Payload).To(Equal(payload))
			// check if e.g. the "umh_topic" exists in the payload
			Expect(string(got.Payload)).To(ContainSubstring("umh_topic"))
		})
	})
})

// buildLogs creates a synthetic Benthos log block for tests.  It wraps the
// supplied hex-encoded data line between BLOCK_START / DATA_END / BLOCK_END
// markers and, if includeTimestamp is true, inserts the given epochMS as the
// timestamp line.  The returned slice is ready to be fed into parseBlock.
func buildLogs(includeTimestamp bool, dataLine string, epochMS int64) []process_shared.LogEntry {
	logs := []process_shared.LogEntry{
		{Content: constants.BLOCK_START_MARKER},
		{Content: dataLine},
		{Content: constants.DATA_END_MARKER},
	}
	if includeTimestamp {
		logs = append(logs, process_shared.LogEntry{Content: strconv.FormatInt(epochMS, 10)})
	}
	logs = append(logs, process_shared.LogEntry{Content: constants.BLOCK_END_MARKER})
	return logs
}
