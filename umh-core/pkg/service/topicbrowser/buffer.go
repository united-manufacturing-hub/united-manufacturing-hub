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
// 3. GetSnapshot exposes read-only pointers. The ring buffer keeps its own
//    reference to every item, so a consumer must neither modify nor recycle
//    them.
//
//  ▸ BufferItem is **immutable** after creation.
//  ▸ Items are never returned to bufferItemPool. A snapshot may still be in
//    flight, and nothing tracks when the last consumer is finished.
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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

// BufferItem is an immutable unit stored in the ring buffer.
// Payload is still protobuf‑encoded but hex‑decoded.
type BufferItem struct {
	Timestamp   time.Time // timestamp from within the logs
	Payload     []byte    // hex-decoded data - but not unmarshalled (protobuf)
	SequenceNum uint64    // monotonically increasing sequence number for tracking
}

type Ringbuffer struct {
	buf         []*BufferItem
	writePos    int    // next write index
	count       int    // number of elements
	sequenceNum uint64 // monotonically increasing sequence number
	mu          sync.Mutex
}

// RingBufferSnapshot provides a consistent view of the ring buffer state.
type RingBufferSnapshot struct {
	// Items holds the buffer contents oldest first, the order they were added.
	// Whatever fills this field keeps that order, and a reader that wants the
	// most recent N items takes them from the end.
	Items           []*BufferItem
	LastSequenceNum uint64 // Latest sequence number
}

// NewestN returns up to n entries, the most recently added ones, in the order
// they arrived. Fewer are returned when the snapshot holds fewer than n.
//
// Callers use this instead of slicing Items, so which end is newest is stated
// once here rather than at each call site.
func (s RingBufferSnapshot) NewestN(n int) []*BufferItem {
	if n <= 0 {
		return nil
	}

	if n >= len(s.Items) {
		return s.Items
	}

	return s.Items[len(s.Items)-n:]
}

// AppendNewest adds item as the most recently arrived entry. Producers that
// build a snapshot by hand use this so the call site never states an order.
func (s *RingBufferSnapshot) AppendNewest(item *BufferItem) {
	s.Items = append(s.Items, item)
}

// NewRingbufferWithDefaultCapacity creates a ring buffer with the standard production capacity.
func NewRingbufferWithDefaultCapacity() *Ringbuffer {
	return NewRingbuffer(constants.RingBufferCapacity)
}

func NewRingbuffer(capacity uint64) *Ringbuffer {
	const (
		defaultCap = constants.RingBufferCapacity
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

func (rb *Ringbuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	n := rb.count

	return n
}

// Cap returns the fixed capacity of the ring buffer.
func (rb *Ringbuffer) Cap() int {
	return len(rb.buf)
}

// GetSnapshot returns the ring's items as a flat slice, oldest first, so a
// caller can read them after the lock is released.
func (rb *Ringbuffer) GetSnapshot() RingBufferSnapshot {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	snapshot := RingBufferSnapshot{
		LastSequenceNum: rb.sequenceNum,
		Items:           make([]*BufferItem, 0, rb.count),
	}

	// The oldest live item sits count places behind the next write slot.
	idx := rb.writePos - rb.count
	if idx < 0 {
		idx += len(rb.buf)
	}

	// Go through the ring oldest to newest, adding each item to the snapshot.
	for range rb.count {
		// Only the pointer is copied; the item stays owned by the ring.
		snapshot.AppendNewest(rb.buf[idx])

		// Step forward, wrapping at the end of the array.
		idx++
		if idx == len(rb.buf) {
			idx = 0
		}
	}

	return snapshot
}
