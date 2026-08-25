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
