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

// GetBuffers returns the current Buffers from newest to oldest with shared references.
//
// PERFORMANCE: This method returns shared references to avoid expensive copying of
// large compressed protobuf payloads. Each Buffer.Payload points to memory allocated
// once in parseBlock() and never modified afterwards.
//
// SAFETY: Buffer objects are immutable after creation. Ring buffer overwrites only
// change pointer slots, never modify existing Buffer objects. This makes reference
// sharing safe.
//
// Do NOT call PutBuffers() on the returned buffers - they are shared references,
// not pooled copies.
func (rb *Ringbuffer) GetBuffers() []*Buffer {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	result := make([]*Buffer, 0, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.writePos - 1 - i + len(rb.buf)) % len(rb.buf)
		if b := rb.buf[idx]; b != nil {
			result = append(result, b) // Share the reference, no copying
		}
	}
	return result
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

// GetBuffersWithCopy returns the current Buffers from newest to oldest with deep copies.
//
// Use this method only when you need to modify the returned buffers. For read-only
// access, use GetBuffers() which is much faster as it avoids copying large payloads.
//
// Call PutBuffers() afterwards to recycle the copied buffers and reduce GC load.
func (rb *Ringbuffer) GetBuffersWithCopy() []*Buffer {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	result := make([]*Buffer, 0, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.writePos - 1 - i + len(rb.buf)) % len(rb.buf)
		if b := rb.buf[idx]; b != nil {
			// clone for new allocated memory
			clone := &Buffer{Timestamp: b.Timestamp}
			if b.Payload != nil {
				clone.Payload = make([]byte, len(b.Payload))
				copy(clone.Payload, b.Payload)
			}
			result = append(result, clone)
		}
	}
	return result
}
