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

var _ = Describe("Ringbuffer", func() {
	const cap uint32 = 3
	var rb *Ringbuffer

	newBuf := func(id byte) *Buffer {
		// tiny helper to create distinct payloads & timestamps
		return &Buffer{
			Payload:   []byte{id},
			Timestamp: time.Now().Add(time.Duration(id) * time.Second),
		}
	}

	BeforeEach(func() {
		rb = NewRingbuffer(cap)
	})

	Context("Add / Get basics", func() {
		It("stores elements and returns them newest-first", func() {
			rb.Add(newBuf(0))
			rb.Add(newBuf(1))
			Expect(rb.Len()).To(Equal(2))

			out := rb.Get()
			Expect(out).To(HaveLen(2))
			// expect newest first (id 1, then id 0)
			Expect(out[0].Payload[0]).To(Equal(byte(1)))
			Expect(out[1].Payload[0]).To(Equal(byte(0)))
		})
	})

	Context("Overwrite when full", func() {
		It("drops the oldest element once capacity is exceeded", func() {
			rb.Add(newBuf(0))
			rb.Add(newBuf(1))
			rb.Add(newBuf(2))
			// Overwrite
			rb.Add(newBuf(3))

			out := rb.Get()
			Expect(out).To(HaveLen(int(cap))) // still full capacity
			ids := []byte{out[0].Payload[0], out[1].Payload[0], out[2].Payload[0]}
			// oldest (0) must be gone, newest (3) present
			Expect(ids).To(Equal([]byte{3, 2, 1}))
		})
	})

	Context("Copy semantics", func() {
		It("returns deep copies that don’t alias the internal storage", func() {
			orig := newBuf(7)
			rb.Add(orig)

			snapshot := rb.Get()
			Expect(snapshot).To(HaveLen(1))

			// mutate the original payload
			orig.Payload[0] = 99

			rb.mu.Lock() // safe access inside the test
			rb.buf[0].Payload[0] = 42
			rb.mu.Unlock()

			// the snapshot must remain unchanged
			Expect(snapshot[0].Payload[0]).To(Equal(byte(7)))
		})
	})
})
