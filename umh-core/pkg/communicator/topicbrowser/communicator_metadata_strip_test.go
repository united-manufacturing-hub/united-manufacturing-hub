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

package topicbrowser_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	topicbrowserservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
)

// bufferWithMetadata returns a BufferItem whose payload is a marshaled UnsBundle
// carrying one TopicInfo entry whose Metadata map is populated.
func bufferWithMetadata() *topicbrowserservice.BufferItem {
	topicInfo := &tbproto.TopicInfo{
		Level0:       "corpA",
		Name:         "temperature",
		DataContract: "_historian",
		Metadata:     map[string]string{"unit": "degC", "serial_number": "1234"},
	}
	bundle := &tbproto.UnsBundle{
		UnsMap: &tbproto.TopicMap{
			Entries: map[string]*tbproto.TopicInfo{
				topicbrowser.HashUNSTableEntry(topicInfo): topicInfo,
			},
		},
	}
	payload, err := proto.Marshal(bundle)
	Expect(err).NotTo(HaveOccurred())

	return &topicbrowserservice.BufferItem{
		Payload:   payload,
		Timestamp: time.Now(),
	}
}

var _ = Describe("Topic metadata stripping on ingest", func() {
	It("strips TopicInfo metadata from both the internal map and the bootstrap wire", func() {
		comm := topicbrowser.NewTopicBrowserCommunicator(zap.NewNop().Sugar())

		obs := createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{
			bufferWithMetadata(),
		})

		_, err := comm.ProcessRealData(obs)
		Expect(err).NotTo(HaveOccurred())

		// (b) INTERNAL MAP (before any GetSubscriberData call): every entry is metadata-free.
		unsMap := comm.GetUnsMap().GetEntries()
		Expect(unsMap).ToNot(BeEmpty(), "internal topic map must not be empty")
		for hash, entry := range unsMap {
			Expect(entry.GetMetadata()).To(BeEmpty(), "internal map entry %s must be stripped of metadata", hash)
		}

		// (a) WIRE: the bootstrap bundle (index 0) carries no topic metadata.
		data, err := comm.GetSubscriberData(false)
		Expect(err).NotTo(HaveOccurred())

		cacheBytes, ok := data.UnsBundles[0]
		Expect(ok).To(BeTrue(), "bootstrap bundle (index 0) must be present")

		var decoded tbproto.UnsBundle
		Expect(proto.Unmarshal(cacheBytes, &decoded)).To(Succeed())

		wiredEntries := decoded.GetUnsMap().GetEntries()
		Expect(wiredEntries).ToNot(BeEmpty(), "bootstrap wire must not be empty")
		for hash, entry := range wiredEntries {
			Expect(entry.GetMetadata()).To(BeEmpty(), "wired topic %s must be stripped of metadata", hash)
		}

		// Stripping removes ONLY Metadata: the other TopicInfo fields that
		// HashUNSTableEntry hashes (name, level0, data contract) must survive
		// the ingest -> cache -> wire round trip, or over-stripping silently
		// corrupts topic-map keys/reconstruction.
		preserved := false
		for _, entry := range wiredEntries {
			if entry.GetName() == "temperature" && entry.GetLevel0() == "corpA" && entry.GetDataContract() == "_historian" {
				preserved = true

				break
			}
		}
		Expect(preserved).To(BeTrue(), "non-metadata TopicInfo fields must survive the strip")
	})
})
