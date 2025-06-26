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
	Payload   []byte    // lz4-decompressed data - but not unmarshalled (protobuf)
	Timestamp time.Time // timestamp from within the logs
}

type Ringbuffer struct {
	buf      []*Buffer
	writePos int // next write index
	count    int // number of elements
	mu       sync.Mutex
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
		buf: make([]*Buffer, capacity),
	}
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

// Two pools: one for Buffer structs, one for byte slices large enough to
// hold typical payloads (capacity grows on demand).
var (
	bufPool = sync.Pool{
		New: func() any { return new(Buffer) },
	}
	bytePool = sync.Pool{
		New: func() any { return make([]byte, 0) },
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

		// Ensure capacity without reallocating every time.
		b := bytePool.Get().([]byte)
		if cap(b) < len(src.Payload) {
			b = make([]byte, len(src.Payload))
		}
		b = b[:len(src.Payload)]
		copy(b, src.Payload)

		dst.Payload = b
		clones[i] = dst
	}
	return clones
}

// PutBuffers allows callers to recycle clones obtained from CloneSlice.
func PutBuffers(bs []*Buffer) {
	for _, b := range bs {
		bytePool.Put(b.Payload[:0]) // keep underlying array for reuse
		b.Payload = nil
		bufPool.Put(b)
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
			clone := &Buffer{Timestamp: buf.Timestamp}
			if buf.Payload != nil {
				clone.Payload = make([]byte, len(buf.Payload))
				copy(clone.Payload, buf.Payload)
			}
			out = append(out, clone)
		}
	}

	return out
}

func (rb *Ringbuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	n := rb.count
	return n
}
