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

package deps

import (
	"sync"
	"time"
)

// ActionResult captures the outcome of a single action execution.
// Parent workers can use this to understand WHY children are in their current state.
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
type InMemoryActionHistoryRecorder struct {
	results []ActionResult
	mu      sync.Mutex
}

// NewInMemoryActionHistoryRecorder creates a new thread-safe ActionHistoryRecorder.
func NewInMemoryActionHistoryRecorder() *InMemoryActionHistoryRecorder {
	return &InMemoryActionHistoryRecorder{
		results: make([]ActionResult, 0, 10),
	}
}

// Record adds an action result to the buffer.
func (r *InMemoryActionHistoryRecorder) Record(result ActionResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.results = append(r.results, result)
}

// Drain returns all buffered results and clears the buffer.
// Never returns nil; returns empty slice if no results buffered.
func (r *InMemoryActionHistoryRecorder) Drain() []ActionResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	results := r.results
	r.results = make([]ActionResult, 0, 10)

	return results
}

// Compile-time check that InMemoryActionHistoryRecorder implements ActionHistoryRecorder.
var _ ActionHistoryRecorder = (*InMemoryActionHistoryRecorder)(nil)
