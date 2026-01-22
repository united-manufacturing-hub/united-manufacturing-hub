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

package fsmv2

import (
	"sync"
	"time"
)

// ActionResult captures the outcome of a single action execution.
// Parent workers can use this to understand WHY children are in their current state.
//
// Field ordering is by decreasing size for optimal memory alignment:
//   - Timestamp: 24 bytes (time.Time)
//   - ActionType: 16 bytes (string header)
//   - ErrorMsg: 16 bytes (string header)
//   - Latency: 8 bytes (time.Duration)
//   - Success: 1 byte (bool)
type ActionResult struct {
	Timestamp  time.Time     `json:"timestamp"`            // When the action completed
	ActionType string        `json:"action_type"`          // e.g., "AuthenticateAction", "SyncAction"
	ErrorMsg   string        `json:"error_msg,omitempty"`  // Error message if Success=false
	Latency    time.Duration `json:"latency_ns,omitempty"` // Time taken to execute
	Success    bool          `json:"success"`              // True if action completed without error
}

// ActionHistoryRecorder buffers action results during execution.
// This is used INTERNALLY by the supervisor - workers should NOT use this directly.
//
// The supervisor owns the ActionHistoryRecorder and auto-records action results
// via ActionExecutor callback. Workers access action history read-only via
// deps.GetActionHistory() which returns []ActionResult.
//
// Data flow:
//  1. ActionExecutor.executeWorkWithRecovery() → workerCtx.actionHistory.Record()
//  2. ActionHistoryProvider callback drains → ActionHistorySetter injects into deps
//  3. CollectObservedState() → deps.GetActionHistory() → ObservedState.LastActionResults
//
// Thread-safe: Can be called concurrently from multiple goroutines.
type ActionHistoryRecorder interface {
	// Record adds an action result to the buffer.
	// Called by supervisor's ActionExecutor after Execute() completes.
	Record(result ActionResult)

	// Drain returns all buffered results and clears the buffer.
	// Called by ActionHistoryProvider callback before CollectObservedState().
	// Returns an empty slice if no results are buffered.
	Drain() []ActionResult
}

// InMemoryActionHistoryRecorder is a thread-safe implementation of ActionHistoryRecorder.
// It uses a mutex to protect concurrent access to the results buffer.
type InMemoryActionHistoryRecorder struct {
	mu      sync.Mutex
	results []ActionResult
}

// NewInMemoryActionHistoryRecorder creates a new thread-safe ActionHistoryRecorder.
// The initial capacity is set to 10 to avoid frequent reallocations for typical workloads.
func NewInMemoryActionHistoryRecorder() *InMemoryActionHistoryRecorder {
	return &InMemoryActionHistoryRecorder{
		results: make([]ActionResult, 0, 10),
	}
}

// Record adds an action result to the buffer.
// Thread-safe: Can be called concurrently from multiple goroutines.
func (r *InMemoryActionHistoryRecorder) Record(result ActionResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.results = append(r.results, result)
}

// Drain returns all buffered results and clears the buffer.
// Thread-safe: Can be called concurrently, but typically called by collector.
// Returns an empty slice if no results are buffered (never returns nil).
func (r *InMemoryActionHistoryRecorder) Drain() []ActionResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	results := r.results
	r.results = make([]ActionResult, 0, 10)

	return results
}

// Compile-time check that InMemoryActionHistoryRecorder implements ActionHistoryRecorder.
var _ ActionHistoryRecorder = (*InMemoryActionHistoryRecorder)(nil)
