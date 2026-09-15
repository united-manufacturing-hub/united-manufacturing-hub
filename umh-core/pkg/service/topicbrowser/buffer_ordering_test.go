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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Ringbuffer snapshot ordering", func() {
	sequences := func(items []*BufferItem) []uint64 {
		seqs := make([]uint64, 0, len(items))
		for _, item := range items {
			seqs = append(seqs, item.SequenceNum)
		}

		return seqs
	}

	add := func(rb *Ringbuffer, count int) {
		for i := range count {
			rb.Add(&BufferItem{
				Payload:   []byte{byte(i)},
				Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			})
		}
	}

	It("hands out items in the order they were added", func() {
		rb := NewRingbuffer(3)
		add(rb, 2)

		Expect(sequences(rb.GetSnapshot().Items)).To(Equal([]uint64{1, 2}))
	})

	It("keeps the added order after the ring has wrapped, dropping only the oldest", func() {
		// Five items into three slots, so writePos has wrapped and the oldest
		// live item is no longer at index 0 of the underlying array. Reading
		// backwards from writePos returns 5, 4, 3 and passes the spec above,
		// which is why this case is separate.
		rb := NewRingbuffer(3)
		add(rb, 5)

		Expect(sequences(rb.GetSnapshot().Items)).To(Equal([]uint64{3, 4, 5}))
	})
})

var _ = Describe("RingBufferSnapshot accessors", func() {
	snapshotOf := func(seqs ...uint64) RingBufferSnapshot {
		snapshot := RingBufferSnapshot{}
		for _, seq := range seqs {
			snapshot.AppendNewest(&BufferItem{SequenceNum: seq})
		}

		return snapshot
	}

	It("AppendNewest puts each entry after the ones already there", func() {
		snapshot := snapshotOf(1, 2, 3)

		Expect(snapshot.Items).To(HaveLen(3))
		Expect(snapshot.Items[len(snapshot.Items)-1].SequenceNum).To(Equal(uint64(3)))
	})

	It("NewestN returns the most recent n, still in arrival order", func() {
		snapshot := snapshotOf(1, 2, 3, 4, 5)

		window := snapshot.NewestN(3)

		Expect(window).To(HaveLen(3))
		Expect(window[0].SequenceNum).To(Equal(uint64(3)))
		Expect(window[len(window)-1].SequenceNum).To(Equal(uint64(5)))
	})

	It("NewestN returns everything it holds when n is larger", func() {
		snapshot := snapshotOf(1, 2)

		window := snapshot.NewestN(9)

		Expect(window).To(HaveLen(2))
		Expect(window[0].SequenceNum).To(Equal(uint64(1)))
	})

	It("NewestN returns nothing for a non-positive n", func() {
		snapshot := snapshotOf(1, 2)

		Expect(snapshot.NewestN(0)).To(BeEmpty())
		Expect(snapshot.NewestN(-1)).To(BeEmpty())
	})
})
