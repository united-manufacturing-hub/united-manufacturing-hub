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

func NewRingbuffer(capacity uint32) *Ringbuffer {
	if capacity == 0 {
		capacity = 8
	}
	return &Ringbuffer{
		buf: make([]*Buffer, capacity),
	}
}

// Add a Buffer to the Ringbuffer, overwrite if full
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
