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

package logging

import (
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
)

// MemoryLogBuffer is a thread-safe circular buffer for storing log entries in memory.
// It maintains a fixed-size buffer of log entries with FIFO behavior - when the buffer
// is full, new entries overwrite the oldest entries. This prevents unbounded memory
// growth while keeping the most recent log entries available for fast retrieval.
type MemoryLogBuffer struct {
	mu      sync.RWMutex
	entries []process_shared.LogEntry
	maxSize int
	current int  // Current write position in circular buffer
	full    bool // Whether buffer has wrapped around
}

// NewMemoryLogBuffer creates a new memory log buffer with the specified maximum size.
// The maxSize parameter determines how many log entries are kept in memory before
// older entries are overwritten. A typical value is 10,000 entries which provides
// a good balance between memory usage and log retention.
func NewMemoryLogBuffer(maxSize int) *MemoryLogBuffer {
	if maxSize <= 0 {
		maxSize = constants.DefaultLogBufferSize
	}

	return &MemoryLogBuffer{
		entries: make([]process_shared.LogEntry, maxSize),
		maxSize: maxSize,
		current: 0,
		full:    false,
	}
}

// AddEntry adds a new log entry to the buffer. If the buffer is full, the oldest
// entry is overwritten. This operation is thread-safe and can be called concurrently
// from multiple goroutines. The circular buffer ensures memory usage remains bounded
// while maintaining the most recent log entries for retrieval.
func (mlb *MemoryLogBuffer) AddEntry(entry process_shared.LogEntry) {
	mlb.mu.Lock()
	defer mlb.mu.Unlock()

	// Add entry at current position
	mlb.entries[mlb.current] = entry

	// Move to next position
	mlb.current++
	if mlb.current >= mlb.maxSize {
		mlb.current = 0
		mlb.full = true
	}
}

// GetEntries returns a copy of all log entries in chronological order (oldest first).
// This operation is thread-safe and returns a snapshot of the current buffer state.
// The returned slice is independent of the internal buffer, so it can be safely
// modified without affecting the buffer. For large buffers, this operation involves
// copying all entries, so it should be used judiciously in high-frequency scenarios.
func (mlb *MemoryLogBuffer) GetEntries() []process_shared.LogEntry {
	mlb.mu.RLock()
	defer mlb.mu.RUnlock()

	if !mlb.full && mlb.current == 0 {
		// Buffer is empty
		return []process_shared.LogEntry{}
	}

	var result []process_shared.LogEntry

	if mlb.full {
		// Buffer has wrapped around - we need to read from current position to end,
		// then from beginning to current position to maintain chronological order
		totalEntries := mlb.maxSize
		result = make([]process_shared.LogEntry, 0, totalEntries)

		// Copy from current position to end (oldest entries)
		result = append(result, mlb.entries[mlb.current:]...)
		// Copy from beginning to current position (newest entries)
		result = append(result, mlb.entries[:mlb.current]...)
	} else {
		// Buffer hasn't wrapped - just copy from beginning to current position
		result = make([]process_shared.LogEntry, mlb.current)
		copy(result, mlb.entries[:mlb.current])
	}

	return result
}

// Clear removes all entries from the buffer and resets it to empty state.
// This operation is thread-safe and can be used to free memory or reset
// the buffer state. After calling Clear, GetEntries will return an empty slice
// until new entries are added via AddEntry.
func (mlb *MemoryLogBuffer) Clear() {
	mlb.mu.Lock()
	defer mlb.mu.Unlock()

	mlb.current = 0
	mlb.full = false
	// Note: We don't need to zero out the entries slice as they will be
	// overwritten when new entries are added. This saves on unnecessary work.
}

// Size returns the current number of entries in the buffer. This operation
// is thread-safe and provides the actual count of log entries currently stored.
// The returned value will be between 0 and maxSize (inclusive).
func (mlb *MemoryLogBuffer) Size() int {
	mlb.mu.RLock()
	defer mlb.mu.RUnlock()

	if mlb.full {
		return mlb.maxSize
	}
	return mlb.current
}

// MaxSize returns the maximum capacity of the buffer. This value is set during
// buffer creation and remains constant throughout the buffer's lifetime.
func (mlb *MemoryLogBuffer) MaxSize() int {
	return mlb.maxSize
}
