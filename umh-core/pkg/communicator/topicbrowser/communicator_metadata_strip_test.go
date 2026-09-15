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
	It("strips TopicInfo metadata from both the internal map and the bootstrap bundle", func() {
		comm := topicbrowser.NewTopicBrowserCommunicator(zap.NewNop().Sugar())

		obs := createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{
			bufferWithMetadata(),
		})

		_, err := comm.ProcessRealData(obs)
		Expect(err).NotTo(HaveOccurred())

		// The internal map, before any GetSubscriberData call.
		unsMap := comm.GetUnsMap().GetEntries()
		Expect(unsMap).ToNot(BeEmpty(), "internal topic map must not be empty")
		for hash, entry := range unsMap {
			Expect(entry.GetMetadata()).To(BeEmpty(), "internal map entry %s must be stripped of metadata", hash)
		}

		// The bundle a first-time subscriber receives.
		data, err := comm.GetSubscriberData(false)
		Expect(err).NotTo(HaveOccurred())

		cacheBytes, ok := data.UnsBundles[0]
		Expect(ok).To(BeTrue(), "bootstrap bundle (index 0) must be present")

		var decoded tbproto.UnsBundle
		Expect(proto.Unmarshal(cacheBytes, &decoded)).To(Succeed())

		wiredEntries := decoded.GetUnsMap().GetEntries()
		Expect(wiredEntries).ToNot(BeEmpty(), "bootstrap bundle must carry its topic map")
		for hash, entry := range wiredEntries {
			Expect(entry.GetMetadata()).To(BeEmpty(), "wired topic %s must be stripped of metadata", hash)
		}

		// Stripping removes only Metadata. Everything HashUNSTableEntry writes
		// must survive the round trip, or the strip changes topic-map keys and
		// the console cannot rebuild the topic map.
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

// bufferWithMetadataAndEvent returns a BufferItem whose payload is a marshaled
// UnsBundle carrying one TopicInfo with a populated Metadata map and one event
// under unsTreeID. The event lets a spec prove the payload a subscriber receives
// is the ingested bundle and not an empty one.
func bufferWithMetadataAndEvent(unsTreeID, topicName string) *topicbrowserservice.BufferItem {
	topicInfo := &tbproto.TopicInfo{
		Level0:       "corpB",
		Name:         topicName,
		DataContract: "_historian",
		Metadata:     map[string]string{"unit": "bar", "serial_number": "5678"},
	}
	bundle := &tbproto.UnsBundle{
		Events: &tbproto.EventTable{
			Entries: []*tbproto.EventTableEntry{
				{UnsTreeId: unsTreeID, ProducedAtMs: 1000},
			},
		},
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

var _ = Describe("Topic metadata stripping on incremental bundles", func() {
	It("strips TopicInfo metadata from every bundle an already-bootstrapped subscriber receives", func() {
		comm := topicbrowser.NewTopicBrowserCommunicator(zap.NewNop().Sugar())

		// The first ingest takes the process-all path, the second the
		// incremental path. Both append to pendingToSend, so both must strip.
		first := bufferWithMetadataAndEvent("corpB.pressure.evt", "pressure")
		_, err := comm.ProcessRealData(createMockObservedStateSnapshot(
			[]*topicbrowserservice.BufferItem{first}))
		Expect(err).NotTo(HaveOccurred())

		second := bufferWithMetadataAndEvent("corpB.flow.evt", "flow")
		_, err = comm.ProcessRealData(createMockObservedStateSnapshot(
			[]*topicbrowserservice.BufferItem{second}))
		Expect(err).NotTo(HaveOccurred())

		// isBootstrapped=true: no cache bundle, only the pending incremental
		// buffers. This is the path taken on every poll after the first.
		data, err := comm.GetSubscriberData(true)
		Expect(err).NotTo(HaveOccurred())
		Expect(data.UnsBundles).To(HaveLen(2), "an already-bootstrapped subscriber must receive both pending buffers")

		seenEvents := []string{}

		for _, staged := range []*topicbrowserservice.BufferItem{first, second} {
			var input tbproto.UnsBundle
			Expect(proto.Unmarshal(staged.Payload, &input)).To(Succeed())

			for hash, entry := range input.GetUnsMap().GetEntries() {
				Expect(entry.GetMetadata()).ToNot(BeEmpty(),
					"fixture topic %s must still stage the metadata this spec strips: a fixture that never staged it, or an implementation that stripped the caller's payload in place, would satisfy the wire assertions vacuously", hash)
			}
		}

		for index, encoded := range data.UnsBundles {
			var decoded tbproto.UnsBundle
			Expect(proto.Unmarshal(encoded, &decoded)).To(Succeed())

			entries := decoded.GetUnsMap().GetEntries()

			Expect(entries).ToNot(BeEmpty(),
				"bundle %d must still carry its topic map: an implementation that drops uns_map satisfies the metadata assertion below by having no entries, and topic identity is keyed there (TopicMap in topic_browser_data.proto)", index)

			for hash, entry := range entries {
				Expect(entry.GetMetadata()).To(BeEmpty(), "bundle %d topic %s must be stripped of metadata", index, hash)
				Expect(entry.GetLevel0()).To(Equal("corpB"), "bundle %d topic %s lost a non-metadata field to over-stripping", index, hash)
				Expect(entry.GetDataContract()).To(Equal("_historian"), "bundle %d topic %s lost its data contract to over-stripping", index, hash)
			}

			for _, event := range decoded.GetEvents().GetEntries() {
				seenEvents = append(seenEvents, event.GetUnsTreeId())
			}
		}

		Expect(seenEvents).To(ConsistOf("corpB.pressure.evt", "corpB.flow.evt"),
			"both ingested events must survive to the subscriber: an implementation returning empty or no bundles would satisfy the metadata assertions above")
	})
})
