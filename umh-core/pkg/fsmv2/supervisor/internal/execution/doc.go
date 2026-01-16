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
// # Why Asynchronous Execution?
//
// Actions are executed asynchronously for several key reasons:
//
// 1. Non-blocking Tick Loop: The supervisor's tick loop must complete quickly
// (typically <100ms) to maintain responsiveness. Long-running actions (network
// calls, process starts) would stall all workers if executed synchronously.
//
// 2. Timeout Protection: Actions have per-operation timeouts. If an action hangs
// (e.g., network partition), the context is cancelled and the action fails cleanly.
// The supervisor can then retry with backoff.
//
// 3. Isolation: Action failures are isolated to the action itself. A crashing
// action doesn't bring down the supervisor or affect other workers.
//
// # Why Worker Pool?
//
// The ActionExecutor uses a fixed worker pool (default: 10 workers) because:
//
// 1. Bounded Concurrency: Prevents resource exhaustion from too many concurrent
// actions. In production, thousands of actions could be enqueued simultaneously.
//
// 2. Queue Backpressure: The buffered channel (2x worker count) provides natural
// backpressure. If the queue is full, enqueue returns an error rather than
// blocking indefinitely.
//
// 3. Predictable Resource Usage: A fixed pool means predictable memory and CPU
// usage regardless of action volume.
//
// # Why Per-Action Timeouts?
//
// Actions have configurable timeouts (default: 30s) because:
//
// 1. Graceful Degradation: A slow action doesn't block other actions. It times
// out, logs an error, and the supervisor can retry with backoff.
//
// 2. Resource Reclamation: Timed-out actions release their worker slot, allowing
// other actions to execute.
//
// 3. Deadlock Prevention: Without timeouts, a hung action could hold a worker
// slot forever, eventually exhausting the pool.
//
// # Why Idempotency Requirement?
//
// All actions MUST be idempotent (safe to call multiple times) because:
//
// 1. Retry After Timeout: If an action times out but partially completed, the
// supervisor will retry it. Idempotency ensures the retry is safe.
//
// 2. Network Partitions: Actions may be executed but their completion ack lost
// due to network issues. The supervisor will retry, so idempotency is essential.
//
// 3. Exponential Backoff: The supervisor retries failed actions with exponential
// backoff. Each retry must be safe.
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
// # Why Exponential Backoff?
//
// Failed actions are retried with exponential backoff because:
//
// 1. Transient Failures: Many failures are transient (network blips, resource
// contention). Exponential backoff gives the system time to recover.
//
// 2. Thundering Herd Prevention: Without backoff, all failed actions would
// retry simultaneously, potentially overwhelming the system.
//
// 3. Fair Scheduling: Backoff ensures that repeatedly failing actions don't
// starve healthy actions of worker slots.
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
// # Testing Idempotency
//
// Use the VerifyActionIdempotency helper to test action idempotency:
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
// # Thread Safety
//
// The ActionExecutor is fully thread-safe:
//   - EnqueueAction() can be called from any goroutine
//   - HasActionInProgress() uses read locks for concurrent access
//   - GetActiveActionCount() uses read locks for concurrent access
//   - Start() and Shutdown() should be called once from the main goroutine
package execution
