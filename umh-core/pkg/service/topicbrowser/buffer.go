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

// This file implements an in-memory, fixed-size ring buffer that
// temporarily stores log payloads coming from the Benthos-UMH pipeline.
//
// The payloads have already been LZ4-decompressed but remain protobuf-encoded;
// de-marshalling is done later by the consumer.  By interposing this buffer
// between Benthos and the communicator, we guarantee a continuous, loss-free
// stream of data even when the reader lags briefly.
//
// Why a ring buffer?
//   • Consistent replay — The consumer can always resume from the exact item it
//     last processed: reads return a snapshot in newest-to-oldest order.
//   • Bounded memory — Capacity is fixed, so memory usage is predictable; when
//     full, the oldest element is overwritten rather than allocating more.
//   • Low overhead — Mutex-protected pointer swaps avoid per-message allocation
//     churn while remaining goroutine-safe.
//

package topicbrowser

import (
	"math"
	"sync"
	"time"
)

type Buffer struct {
	Payload     []byte    // hex-decoded data (protobuf)
	Timestamp   time.Time // timestamp from within the logs
	SequenceNum uint64    // sequence number for tracking order
}

// RingBufferSnapshot represents a snapshot of the ring buffer with sequence tracking
type RingBufferSnapshot struct {
	Items            []*Buffer // buffers from newest to oldest
	LatestSequence   uint64    // highest sequence number in the snapshot
	EarliestSequence uint64    // lowest sequence number in the snapshot
}

type Ringbuffer struct {
	buf        []*Buffer
	writePos   int // next write index
	count      int // number of elements
	mu         sync.Mutex
	nextSeqNum uint64 // sequence number for the next item
}

func NewRingbuffer(capacity uint64) *Ringbuffer {
	const (
		defaultCap = 8
		maxCap     = uint64(math.MaxInt64)
	)

	if capacity == 0 || capacity > maxCap {
		capacity = defaultCap
	}

	return &Ringbuffer{
		buf:        make([]*Buffer, capacity),
		nextSeqNum: 1, // Start from 1
	}
}

// NewRingbufferWithDefaultCapacity creates a ring buffer with a default capacity suitable for topic browser usage
func NewRingbufferWithDefaultCapacity() *Ringbuffer {
	return NewRingbuffer(64) // Reasonable default for topic browser
}

// GetNextSequenceNum returns the next sequence number and increments the counter
func (rb *Ringbuffer) GetNextSequenceNum() uint64 {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	seqNum := rb.nextSeqNum
	rb.nextSeqNum++
	return seqNum
}

// Add a Buffer to the Ringbuffer, overwrite if full.
// It takes a pointer as input for performance optimizations. The data can get
// really big, that is being loaded into the buffer.
func (rb *Ringbuffer) Add(buf *Buffer) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.buf[rb.writePos] = buf
	rb.writePos = (rb.writePos + 1) % len(rb.buf)

	if rb.count < len(rb.buf) {
		rb.count++
	}
}

// Get the current Buffers from newest to oldest
// Allocates new memory before returning the buffer to exclude modified data afterwards.
func (rb *Ringbuffer) Get() []*Buffer {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	out := make([]*Buffer, 0, rb.count)

	for i := range rb.count {
		index := (rb.writePos - 1 - i + len(rb.buf)) % len(rb.buf)

		buf := rb.buf[index]
		if buf != nil {
			// clone for new allocated memory
			clone := &Buffer{
				Timestamp:   buf.Timestamp,
				SequenceNum: buf.SequenceNum,
			}
			if buf.Payload != nil {
				clone.Payload = make([]byte, len(buf.Payload))
				copy(clone.Payload, buf.Payload)
			}
			out = append(out, clone)
		}
	}

	return out
}

// GetSnapshot returns a structured snapshot of the ring buffer with sequence tracking
func (rb *Ringbuffer) GetSnapshot() RingBufferSnapshot {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	items := make([]*Buffer, 0, rb.count)
	var latestSeq, earliestSeq uint64

	for i := range rb.count {
		index := (rb.writePos - 1 - i + len(rb.buf)) % len(rb.buf)

		buf := rb.buf[index]
		if buf != nil {
			// clone for new allocated memory
			clone := &Buffer{
				Timestamp:   buf.Timestamp,
				SequenceNum: buf.SequenceNum,
			}
			if buf.Payload != nil {
				clone.Payload = make([]byte, len(buf.Payload))
				copy(clone.Payload, buf.Payload)
			}
			items = append(items, clone)

			// Track sequence numbers
			if i == 0 { // First item (newest)
				latestSeq = buf.SequenceNum
				earliestSeq = buf.SequenceNum
			} else {
				if buf.SequenceNum > latestSeq {
					latestSeq = buf.SequenceNum
				}
				if buf.SequenceNum < earliestSeq {
					earliestSeq = buf.SequenceNum
				}
			}
		}
	}

	return RingBufferSnapshot{
		Items:            items,
		LatestSequence:   latestSeq,
		EarliestSequence: earliestSeq,
	}
}

// Two pools: one for Buffer structs, one for byte slices large enough to
// hold typical payloads (capacity grows on demand).
var (
	bufPool = sync.Pool{
		New: func() any { return new(Buffer) },
	}
	bytePool = sync.Pool{
		New: func() any {
			b := make([]byte, 0)
			return &b
		},
	}
)

// Get the current Buffers from newest to oldest
// to reduce load on GC call PutBuffers afterwards
func (rb *Ringbuffer) GetBuffers() []*Buffer {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	snap := make([]*Buffer, 0, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.writePos - 1 - i + len(rb.buf)) % len(rb.buf)
		if b := rb.buf[idx]; b != nil {
			snap = append(snap, b)
		}
	}

	clones := make([]*Buffer, len(snap))

	for i, src := range snap {
		dst := bufPool.Get().(*Buffer)
		dst.Timestamp = src.Timestamp
		dst.SequenceNum = src.SequenceNum

		// Ensure capacity without reallocating every time.
		bPtr := bytePool.Get().(*[]byte)
		if cap(*bPtr) < len(src.Payload) {
			*bPtr = make([]byte, len(src.Payload))
		}
		slice := (*bPtr)[:len(src.Payload)]
		copy(slice, src.Payload)

		dst.Payload = slice

		clones[i] = dst
	}
	return clones
}

// PutBuffers allows callers to recycle clones obtained from CloneSlice.
func PutBuffers(bs []*Buffer) {
	for _, b := range bs {
		if b.Payload != nil {
			tmp := b.Payload[:0]
			bytePool.Put(&tmp) // put pointer, not value
		}

		b.Payload = nil
		bufPool.Put(b)
	}
}

func (rb *Ringbuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	n := rb.count
	return n
}
