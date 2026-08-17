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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	topicbrowserservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
)

var _ = Describe("ring-buffer ownership on acknowledge", func() {
	It("keeps the payloads of items the ring buffer still owns after the consumer acknowledges delivery", func() {
		// More pending buffers than maxPendingBuffers (100) so cleanupOldPendingBuffers
		// takes the cleanup branch instead of short-circuiting.
		const (
			capacity  = 120
			itemCount = capacity // fill the ring; no ring overwrite yet
		)

		rb := topicbrowserservice.NewRingbuffer(capacity)
		comm := topicbrowser.NewTopicBrowserCommunicator(zap.NewNop().Sugar())

		// Every payload is a valid UnsBundle so each item flows through
		// updateInternalCache into pendingToSend (a skipped/unparseable buffer
		// would never be cleaned, and would pass the payload assertion vacuously).
		base := time.Now().Add(-time.Hour)
		original := make([][]byte, itemCount) // payload written for seq i+1, for byte-for-byte comparison
		for i := range itemCount {
			topicInfo := &tbproto.TopicInfo{Name: fmt.Sprintf("topic-%d", i), Level0: "corpA"}
			bundle := &tbproto.UnsBundle{
				UnsMap: &tbproto.TopicMap{
					Entries: map[string]*tbproto.TopicInfo{
						topicbrowser.HashUNSTableEntry(topicInfo): topicInfo,
					},
				},
			}
			payload, err := proto.Marshal(bundle)
			Expect(err).NotTo(HaveOccurred())
			original[i] = payload
			rb.Add(&topicbrowserservice.BufferItem{
				Payload:   payload,
				Timestamp: base.Add(time.Duration(i) * time.Second),
			})
		}

		snapshot := rb.GetSnapshot()
		Expect(snapshot.Items).To(HaveLen(itemCount), "ring must hold the items before processing")

		result, err := comm.ProcessRealData(createMockObservedStateSnapshot(snapshot.Items))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ProcessedCount).To(Equal(itemCount),
			"every ring item must flow into pendingToSend so acknowledge/cleanup touches them")

		// Acknowledge delivery at a time newer than every buffer, making all of
		// them eligible for cleanup.
		comm.MarkDataAsSent(time.Now())

		// Re-read the ring fresh. The ring still owns these items, so each
		// payload must survive the consumer's acknowledge/cleanup. Both this
		// snapshot and the one passed to ProcessRealData alias the same structs,
		// but asserting on the ring re-read is what exposes the ownership bug.
		fresh := rb.GetSnapshot()
		Expect(fresh.Items).To(HaveLen(itemCount), "ring must still hold every item after acknowledgment")
		for _, item := range fresh.Items {
			Expect(item.Payload).To(Equal(original[item.SequenceNum-1]),
				"ring-owned payload must survive consumer acknowledge (seq %d)", item.SequenceNum)
		}
	})
})
