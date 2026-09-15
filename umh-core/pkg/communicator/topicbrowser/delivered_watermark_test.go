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

// bufferWithTopicAt builds a buffer carrying one identifiable topic. The sequence
// number is set explicitly because createMockObservedStateSnapshot's fallback
// counter is a package global shared with every other spec in this suite.
func bufferWithTopicAt(seq uint64, topic string, ts time.Time) *topicbrowserservice.BufferItem {
	topicInfo := &tbproto.TopicInfo{Name: topic, Level0: "corpA"}
	bundle := &tbproto.UnsBundle{
		UnsMap: &tbproto.TopicMap{
			Entries: map[string]*tbproto.TopicInfo{
				topicbrowser.HashUNSTableEntry(topicInfo): topicInfo,
			},
		},
	}
	payload, err := proto.Marshal(bundle)
	Expect(err).NotTo(HaveOccurred())

	return &topicbrowserservice.BufferItem{SequenceNum: seq, Payload: payload, Timestamp: ts}
}

// deliveredTopics decodes every bundle handed to a subscriber and returns the
// topic names inside, which is how a spec tells one buffer's payload from another.
func deliveredTopics(data *topicbrowser.SubscriberData) []string {
	var names []string

	for _, raw := range data.UnsBundles {
		var decoded tbproto.UnsBundle
		Expect(proto.Unmarshal(raw, &decoded)).To(Succeed())

		for _, entry := range decoded.GetUnsMap().GetEntries() {
			names = append(names, entry.GetName())
		}
	}

	return names
}

var _ = Describe("Delivery watermark", func() {
	It("still delivers a buffer that arrived while a send was in flight, and does not deliver it twice", func() {
		comm := topicbrowser.NewTopicBrowserCommunicator(zap.NewNop().Sugar())

		// These must be in the past: the gap between a buffer's emission time and
		// wall clock is what the defect turns on. See queuedBundle.Timestamp.
		emittedFirst := time.Now().Add(-10 * time.Second)
		emittedSecond := time.Now().Add(-5 * time.Second)

		first := bufferWithTopicAt(1, "topic-first", emittedFirst)
		second := bufferWithTopicAt(2, "topic-second", emittedSecond)

		// Tick 1: the first buffer is ingested and handed to a subscriber.
		_, err := comm.ProcessRealData(createMockObservedStateSnapshot(
			[]*topicbrowserservice.BufferItem{first}))
		Expect(err).NotTo(HaveOccurred())

		sent, err := comm.GetSubscriberData(true)
		Expect(err).NotTo(HaveOccurred())
		Expect(deliveredTopics(sent)).To(ConsistOf("topic-first"),
			"the first tick must deliver the first buffer")

		// The second buffer arrives before the first is marked sent.
		// Items are oldest-first, so the new buffer is last.
		_, err = comm.ProcessRealData(createMockObservedStateSnapshot(
			[]*topicbrowserservice.BufferItem{first, second}))
		Expect(err).NotTo(HaveOccurred())

		// Mark exactly the way notify() does.
		comm.MarkDataAsSent()

		// Tick 2 must deliver the buffer that arrived in between...
		sent, err = comm.GetSubscriberData(true)
		Expect(err).NotTo(HaveOccurred())
		Expect(deliveredTopics(sent)).To(ContainElement("topic-second"),
			"a buffer that arrived before the mark must still be delivered")

		// ...and must not re-deliver what tick 1 already sent.
		Expect(deliveredTopics(sent)).ToNot(ContainElement("topic-first"),
			"an already-delivered buffer must not be delivered twice")
	})
})
