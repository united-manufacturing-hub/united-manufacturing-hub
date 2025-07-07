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
// The payloads have already been hex-decoded but remain protobuf-encoded;
// de-marshalling is done later by the consumer.  By interposing this buffer
// between Benthos and the communicator, we guarantee a continuous, loss-free
// stream of data even when the reader lags briefly.
// Package topicbrowser provides a high‑throughput parser plus an in‑process,
// fixed‑size ring buffer for Benthos‑UMH logs.
//
// ─── Data & Ownership Flow ──────────────────────────────────────────────
//
//   s6 log  →  parseBlock()  →  Ringbuffer.Add()  →  GetSnapshot()
//
// 1. parseBlock concatenates hex lines into a scratch slice taken from
//    parseBufferPool, decodes the hex directly into a *BufferItem* obtained
//    from bufferItemPool, then hands that BufferItem to the ring buffer.
// 2. The ring buffer owns the BufferItem until it is overwritten.
// 3. GetSnapshot exposes *read‑only* pointers; consumers must call
//    PutBufferItems once they are finished so the structs can be reused.
//
//  ▸ BufferItem is **immutable** after creation.
//  ▸ Overwritten items are **not** returned automatically to the pool to
//    avoid use‑after‑free while snapshots might still be in flight.
//  ▸ parseBufferPool buffers are always returned immediately.
//
// Concurrency guarantees: Add and GetSnapshot are mutex‑protected and safe to
// call from multiple goroutines; BufferItems obtained from snapshots are
// safe for concurrent *read* access only.
//

package topicbrowser

import (
	"math"
	"sync"
	"time"
)

// BufferItem is an immutable unit stored in the ring buffer.
// Payload is still protobuf‑encoded but hex‑decoded.
type BufferItem struct {
	Payload     []byte    // hex-decoded data - but not unmarshalled (protobuf)
	Timestamp   time.Time // timestamp from within the logs
	SequenceNum uint64    // monotonically increasing sequence number for tracking
}

type Ringbuffer struct {
	buf         []*BufferItem
	writePos    int    // next write index
	count       int    // number of elements
	sequenceNum uint64 // monotonically increasing sequence number
	mu          sync.Mutex
}

// RingBufferSnapshot provides a consistent view of the ring buffer state
type RingBufferSnapshot struct {
	WritePos        int           // Current write position
	Count           int           // Number of elements in buffer
	LastSequenceNum uint64        // Latest sequence number
	Items           []*BufferItem // Current buffer contents, newest-to-oldest
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
		buf: make([]*BufferItem, capacity),
	}
}

// Add writes buf at the current position (overwriting oldest if full).
// Overwritten items are NOT returned to bufferItemPool; the consumer that
// still holds a snapshot may be reading them.
func (rb *Ringbuffer) Add(buf *BufferItem) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Assign sequence number to buffer
	rb.sequenceNum++
	buf.SequenceNum = rb.sequenceNum

	rb.buf[rb.writePos] = buf
	rb.writePos = (rb.writePos + 1) % len(rb.buf)

	if rb.count < len(rb.buf) {
		rb.count++
	}
}

// bufferItemPool recycles BufferItem shells.  See package docs for limits.
var bufferItemPool = sync.Pool{
	New: func() any {
		return &BufferItem{}
	},
}

// PutBufferItems must be called by every consumer of GetSnapshot() when
// finished. It zeroes the structs and returns them to bufferItemPool.
func PutBufferItems(items []*BufferItem) {
	for _, item := range items {
		// Clear the item and return to pool
		item.Payload = nil
		item.Timestamp = time.Time{}
		item.SequenceNum = 0
		bufferItemPool.Put(item)
	}
}

func (rb *Ringbuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	n := rb.count
	return n
}

// GetSnapshot returns a consistent snapshot of the ring buffer state
// This is the primary interface for consumers to get ring buffer data
func (rb *Ringbuffer) GetSnapshot() RingBufferSnapshot {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	return RingBufferSnapshot{
		WritePos:        rb.writePos,
		Count:           rb.count,
		LastSequenceNum: rb.sequenceNum,
		Items:           rb.getBuffersInternal(),
	}
}

// getBuffersInternal returns shared references to buffer items (newest to oldest)
// This is used internally by GetSnapshot and assumes mutex is already held
func (rb *Ringbuffer) getBuffersInternal() []*BufferItem {
	result := make([]*BufferItem, 0, rb.count)
	for i := 0; i < rb.count; i++ {
		idx := (rb.writePos - 1 - i + len(rb.buf)) % len(rb.buf)
		if b := rb.buf[idx]; b != nil {
			result = append(result, b) // Share the reference, no copying
		}
	}
	return result
}
