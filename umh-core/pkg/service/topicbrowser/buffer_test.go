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
	const capacity uint64 = 3
	var rb *Ringbuffer

	newBuf := func(id byte) *BufferItem {
		// tiny helper to create distinct payloads & timestamps
		return &BufferItem{
			Payload:   []byte{id},
			Timestamp: time.Now().Add(time.Duration(id) * time.Second),
		}
	}

	BeforeEach(func() {
		rb = NewRingbuffer(capacity)
	})

	Context("Add / Get basics", func() {
		It("stores elements and returns them newest-first", func() {
			rb.Add(newBuf(0))
			rb.Add(newBuf(1))
			Expect(rb.Len()).To(Equal(2))

			snapshot := rb.GetSnapshot()
			Expect(snapshot.Items).To(HaveLen(2))
			// expect newest first (id 1, then id 0)
			Expect(snapshot.Items[0].Payload[0]).To(Equal(byte(1)))
			Expect(snapshot.Items[1].Payload[0]).To(Equal(byte(0)))
		})
	})

	Context("Overwrite when full", func() {
		It("drops the oldest element once capacity is exceeded", func() {
			rb.Add(newBuf(0))
			rb.Add(newBuf(1))
			rb.Add(newBuf(2))
			// Overwrite
			rb.Add(newBuf(3))

			snapshot := rb.GetSnapshot()
			Expect(snapshot.Items).To(HaveLen(int(capacity))) // still full capacity
			ids := []byte{snapshot.Items[0].Payload[0], snapshot.Items[1].Payload[0], snapshot.Items[2].Payload[0]}
			// oldest (0) must be gone, newest (3) present
			Expect(ids).To(Equal([]byte{3, 2, 1}))
		})
	})

	Context("capacity overflow guard", func() {
		It("falls back to the default when asked for an absurd capacity", func() {
			rb := NewRingbuffer(math.MaxUint64) // far above MaxInt
			Expect(rb.buf).ToNot(BeEmpty())
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
			var waitGroup sync.WaitGroup
			for w := range writers {
				waitGroup.Add(1)
				go helperWriter(rb, byte(w), iterations, &waitGroup)
			}
			for range readers {
				waitGroup.Add(1)
				go helperReaderGet(rb, int(capacity), iterations, &waitGroup)
			}
			waitGroup.Wait()
		})
	})

	Context("GetSnapshot / memory management", func() {
		It("provides immutable snapshot access", func() {
			rb := NewRingbuffer(4)
			rb.Add(helperBuf(42))

			snapshot := rb.GetSnapshot()
			Expect(snapshot.Items).To(HaveLen(1))

			// Verify immutable access - snapshot won't change if we add more items
			rb.Add(helperBuf(99))
			Expect(snapshot.Items).To(HaveLen(1))                    // Original snapshot unchanged
			Expect(snapshot.Items[0].Payload[0]).To(Equal(byte(42))) // Original data preserved

			// New snapshot reflects updates
			newSnapshot := rb.GetSnapshot()
			Expect(newSnapshot.Items).To(HaveLen(2))
			Expect(newSnapshot.Items[0].Payload[0]).To(Equal(byte(99))) // Newest first
		})

		It("demonstrates proper consumer memory management", func() {
			rb := NewRingbuffer(4)
			rb.Add(helperBuf(42))
			rb.Add(helperBuf(43))

			// Consumer pattern: get snapshot and always return items to pool
			snapshot := rb.GetSnapshot()
			defer PutBufferItems(snapshot.Items) // CRITICAL: Always return to pool when done

			// Process the items
			Expect(snapshot.Items).To(HaveLen(2))
			for _, item := range snapshot.Items {
				// Verify we can read the data
				Expect(item.Payload).ToNot(BeNil())
				Expect(item.Payload[0]).To(BeNumerically(">=", 42))
			}

			// At this point, defer will call PutBufferItems to return items to pool
			// After that, snapshot.Items should not be used
		})

		It("shows that PutBufferItems clears the items", func() {
			rb := NewRingbuffer(2)
			rb.Add(helperBuf(100))

			snapshot := rb.GetSnapshot()
			Expect(snapshot.Items).To(HaveLen(1))
			Expect(snapshot.Items[0].Payload[0]).To(Equal(byte(100)))

			// Return to pool - this clears the items
			PutBufferItems(snapshot.Items)

			// Items are now cleared (should not use after PutBufferItems!)
			Expect(snapshot.Items[0].Payload).To(BeNil())
			Expect(snapshot.Items[0].Timestamp).To(Equal(time.Time{}))
			Expect(snapshot.Items[0].SequenceNum).To(Equal(uint64(0)))
		})
	})

	Context("Sequence number tracking", func() {
		It("assigns monotonically increasing sequence numbers", func() {
			rb := NewRingbuffer(3)

			buf1 := newBuf(1)
			buf2 := newBuf(2)
			buf3 := newBuf(3)

			rb.Add(buf1)
			rb.Add(buf2)
			rb.Add(buf3)

			snapshot := rb.GetSnapshot()
			Expect(snapshot.LastSequenceNum).To(Equal(uint64(3)))
			Expect(snapshot.Items).To(HaveLen(3))

			// Verify sequence numbers are assigned correctly
			Expect(buf1.SequenceNum).To(Equal(uint64(1)))
			Expect(buf2.SequenceNum).To(Equal(uint64(2)))
			Expect(buf3.SequenceNum).To(Equal(uint64(3)))
		})

		It("handles sequence numbers with ring buffer overflow", func() {
			rb := NewRingbuffer(2) // Small buffer to test overflow

			buf1 := newBuf(1)
			buf2 := newBuf(2)
			buf3 := newBuf(3) // This will overwrite buf1

			rb.Add(buf1)
			rb.Add(buf2)
			rb.Add(buf3)

			snapshot := rb.GetSnapshot()
			Expect(snapshot.LastSequenceNum).To(Equal(uint64(3)))
			Expect(snapshot.Items).To(HaveLen(2)) // Buffer capacity

			// Only buf2 and buf3 should remain
			Expect(snapshot.Items).To(HaveLen(2))
			Expect(snapshot.Items[0].SequenceNum).To(Equal(uint64(3))) // Newest first
			Expect(snapshot.Items[1].SequenceNum).To(Equal(uint64(2)))
		})
	})

	Context("GetSnapshot method", func() {
		It("returns consistent snapshot data", func() {
			rb := NewRingbuffer(3)

			rb.Add(newBuf(10))
			rb.Add(newBuf(20))

			snapshot := rb.GetSnapshot()

			Expect(snapshot.LastSequenceNum).To(Equal(uint64(2)))
			Expect(snapshot.Items).To(HaveLen(2))

			// Items should be newest-to-oldest
			Expect(snapshot.Items[0].Payload[0]).To(Equal(byte(20))) // Newest
			Expect(snapshot.Items[1].Payload[0]).To(Equal(byte(10))) // Older
		})
	})
})

// helper for buffer format.
func helperBuf(id byte) *BufferItem {
	return &BufferItem{Payload: []byte{id}, Timestamp: time.Now()}
}

// helper to Add to ringbuffer.
func helperWriter(rb *Ringbuffer, id byte, n int, wg *sync.WaitGroup) {
	defer GinkgoRecover()
	defer wg.Done()

	for range n {
		rb.Add(helperBuf(id))
	}
}

// helper to Get from ringbuffer using GetSnapshot.
func helperReaderGet(rb *Ringbuffer, capacity int, n int, wg *sync.WaitGroup) {
	defer GinkgoRecover()
	defer wg.Done()

	for range n {
		snapshot := rb.GetSnapshot()
		Expect(len(snapshot.Items)).To(BeNumerically("<=", capacity))

		for _, b := range snapshot.Items {
			Expect(b).NotTo(BeNil())
			Expect(b.Payload).NotTo(BeNil())
		}
	}
}
