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
	"math"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Ringbuffer", func() {
	const cap uint64 = 3
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

			rb.Add(&Buffer{Payload: []byte{42}, Timestamp: time.Now()})

			// the snapshot must remain unchanged
			Expect(snapshot[0].Payload[0]).To(Equal(byte(7)))
		})
	})

	Context("capacity overflow guard", func() {
		It("falls back to the default when asked for an absurd capacity", func() {
			rb := NewRingbuffer(math.MaxUint64) // far above MaxInt
			Expect(len(rb.buf)).To(BeNumerically(">=", 1))
			Expect(len(rb.buf)).To(BeNumerically("<=", 8)) // defaultCap
		})
	})

	// should be run with ginkgo -race and needs CGO_ENABLED=1
	Context("concurrent Add and Get", func() {
		It("is race‑free with many goroutines", func() {
			const (
				capacity   = 64
				writers    = 16
				readers    = 16
				iterations = 1_000
			)
			rb := NewRingbuffer(capacity)

			// spawn goroutines to perform concurrent write and read and wait for all
			// iterations to be done
			var wg sync.WaitGroup
			for w := range writers {
				wg.Add(1)
				go helperWriter(rb, byte(w), iterations, &wg)
			}
			for range readers {
				wg.Add(1)
				go helperReaderGet(rb, int(capacity), iterations, &wg)
			}
			wg.Wait()
		})
	})

	Context("GetBuffers / PutBuffers", func() {
		It("clones and recycles without leaking", func() {
			rb := NewRingbuffer(4)
			rb.Add(helperBuf(42))

			clones := rb.GetBuffers()
			Expect(clones).To(HaveLen(1))
			clones[0].Payload[0] = 99 // mutate clone

			PutBuffers(clones)
			Expect(clones[0].Payload).To(BeNil()) // zeroed by PutBuffers
		})
	})
})

// helper for buffer format
func helperBuf(id byte) *Buffer {
	return &Buffer{Payload: []byte{id}, Timestamp: time.Now()}
}

// helper to Add to ringbuffer
func helperWriter(rb *Ringbuffer, id byte, n int, wg *sync.WaitGroup) {
	defer GinkgoRecover()
	defer wg.Done()
	for range n {
		rb.Add(helperBuf(id))
	}
}

// helper to Get from ringbuffer
func helperReaderGet(rb *Ringbuffer, capacity int, n int, wg *sync.WaitGroup) {
	defer GinkgoRecover()
	defer wg.Done()
	for range n {
		out := rb.Get()
		Expect(len(out)).To(BeNumerically("<=", capacity))
		for _, b := range out {
			Expect(b).NotTo(BeNil())
			Expect(b.Payload).NotTo(BeNil())
		}
	}
}
