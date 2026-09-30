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

// incrementalBufferWithTopic returns a BufferItem whose payload is a marshaled
// UnsBundle carrying a single TopicInfo whose Name is the given topic.
func incrementalBufferWithTopic(seq uint64, topic string, ts time.Time) *topicbrowserservice.BufferItem {
	topicInfo := &tbproto.TopicInfo{
		Name: topic,
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
		SequenceNum: seq,
		Payload:     payload,
		Timestamp:   ts,
	}
}

var _ = Describe("Incremental buffer processing", func() {
	It("ingests the newest buffer (Items[len-1]), not the oldest (Items[0])", func() {
		comm := topicbrowser.NewTopicBrowserCommunicator(zap.NewNop().Sugar())

		// First ProcessRealData establishes the baseline. Because
		// lastProcessedSequence == 0, processAllBuffers runs and stores
		// buffers[len-1].SequenceNum (10) as the last processed sequence.
		baseline := incrementalBufferWithTopic(10, "baseline-topic", time.UnixMilli(1000))
		_, err := comm.ProcessRealData(createMockObservedStateSnapshot([]*topicbrowserservice.BufferItem{baseline}))
		Expect(err).NotTo(HaveOccurred())
		Expect(comm.GetLastProcessedSequence()).To(Equal(uint64(10)))

		// Second snapshot: oldest-to-newest, with more slots than the
		// 3-slot production ring so the newBufferCount=1 window is stable and
		// unambiguous. LastSequenceNum = 11 while the last processed sequence
		// is 10, so newBufferCount = 1: exactly one buffer is new.
		second := []*topicbrowserservice.BufferItem{
			incrementalBufferWithTopic(6, "oldest-topic", time.UnixMilli(1100)), // Items[0]
			incrementalBufferWithTopic(7, "mid-7-topic", time.UnixMilli(1200)),
			incrementalBufferWithTopic(8, "mid-8-topic", time.UnixMilli(1300)),
			incrementalBufferWithTopic(9, "mid-9-topic", time.UnixMilli(1400)),
			incrementalBufferWithTopic(10, "mid-10-topic", time.UnixMilli(1500)),
			incrementalBufferWithTopic(11, "newest-topic", time.UnixMilli(2000)), // Items[len-1]
		}

		_, err = comm.ProcessRealData(createMockObservedStateSnapshot(second))
		Expect(err).NotTo(HaveOccurred())

		// Assert the OBSERVABLE OUTCOME on the ingested topic map, both ways:
		// the newest payload's topic is present and the oldest payload's topic
		// is absent. A test that only checked the newest arrived would falsely
		// pass an implementation that ingests ALL buffers.
		ingestedTopics := make(map[string]bool)
		for _, entry := range comm.GetUnsMap().GetEntries() {
			ingestedTopics[entry.GetName()] = true
		}

		Expect(ingestedTopics).To(HaveKey("newest-topic"), "the newest buffer (Items[len-1]) must be ingested")
		Expect(ingestedTopics).ToNot(HaveKey("oldest-topic"), "the oldest buffer (Items[0]) must NOT be ingested")
	})
})
