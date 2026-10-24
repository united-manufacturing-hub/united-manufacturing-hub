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

// Package execution provides the ActionExecutor for FSMv2 action execution.
//
// # Overview
//
// The ActionExecutor handles asynchronous execution of FSM actions with:
//   - Non-blocking enqueue (supervisor tick loop is never blocked)
//   - Per-action timeout with context cancellation
//   - Worker pool for concurrent action execution
//   - Metrics for queue size, utilization, and execution duration
//
// # Asynchronous execution
//
// Actions execute asynchronously to keep the supervisor's tick loop fast
// (typically <100ms). Long-running actions like network calls or process starts
// run in the worker pool instead of blocking the tick loop.
//
// Each action has a timeout. If an action hangs, the context is cancelled and
// the supervisor retries with backoff. Action failures are isolated - a crashing
// action does not affect other workers.
//
// # Worker pool
//
// The ActionExecutor uses a fixed worker pool (default: 10 workers) with a
// buffered channel (2x worker count). This bounds concurrency to prevent
// resource exhaustion and provides backpressure - if the queue is full,
// enqueue returns an error instead of blocking.
//
// # Per-action timeouts
//
// Actions have configurable timeouts (default: 30s). A slow action times out
// and releases its worker slot, allowing other actions to execute. Without
// timeouts, a hung action could hold a worker slot forever.
//
// # Idempotency requirement
//
// All actions MUST be idempotent. The supervisor retries actions after timeouts,
// failures, and network partitions. If an action times out but partially
// completed, the retry must be safe.
//
// Example idempotent action:
//
//	func (a *StartProcessAction) Execute(ctx context.Context, deps any) error {
//	    // Check if already done (idempotency)
//	    if processIsRunning(a.ProcessPath) {
//	        return nil  // Already started, safe to call again
//	    }
//	    return startProcess(ctx, a.ProcessPath)
//	}
//
// # Exponential backoff
//
// Failed actions retry with exponential backoff. This handles transient failures
// (network blips, resource contention) by giving the system time to recover.
// Backoff also prevents thundering herd retries and ensures failing actions
// do not starve healthy actions of worker slots.
//
// # Usage
//
//	executor := execution.NewActionExecutor(10, "my-supervisor", logger)
//	executor.Start(ctx)
//
//	// Enqueue action (non-blocking)
//	err := executor.EnqueueAction("action-1", myAction, deps)
//	if err != nil {
//	    // Queue full or action already in progress
//	}
//
//	// Check if action is running
//	if executor.HasActionInProgress("action-1") {
//	    // Don't enqueue duplicate
//	}
//
//	// Shutdown
//	executor.Shutdown()
//
// # Testing idempotency
//
// Use VerifyActionIdempotency to test action idempotency:
//
//	execution.VerifyActionIdempotency(action, 3, func() {
//	    // Verify expected state after 3 executions
//	    Expect(fileExists("test.txt")).To(BeTrue())
//	})
//
// # Metrics
//
// The ActionExecutor exposes Prometheus metrics:
//   - fsmv2_action_queue_size: Current queue depth
//   - fsmv2_action_execution_duration_seconds: Histogram of execution times
//   - fsmv2_action_timeout_total: Count of timed-out actions
//   - fsmv2_worker_pool_utilization: Percentage of workers in use
//
// # Thread safety
//
// The ActionExecutor is thread-safe:
//   - EnqueueAction() can be called from any goroutine
//   - HasActionInProgress() uses read locks for concurrent access
//   - GetActiveActionCount() uses read locks for concurrent access
//   - Start() and Shutdown() should be called once from the main goroutine
package execution
