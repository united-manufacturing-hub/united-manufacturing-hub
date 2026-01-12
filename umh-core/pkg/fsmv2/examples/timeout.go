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

package examples

// TimeoutScenario demonstrates action timeout handling and retry behavior.
//
// # What This Scenario Tests
//
// This scenario demonstrates three levels of timeout behavior:
//
//  1. Quick Action Success
//     - Action completes well within timeout
//     - Normal happy path
//
//  2. Slow Action Success
//     - Action takes significant time but completes
//     - Demonstrates patience in action execution
//
//  3. Combined with Failure Pattern
//     - Action that fails several times before succeeding
//     - Shows retry + delay combined
//
// # Understanding Timeouts in FSM v2
//
// ## Action Timeout Architecture
//
// The ActionExecutor enforces timeouts on individual actions:
//
//	ActionExecutor (30s default timeout)
//	  └── Creates context with deadline for each action
//	  └── Action must check ctx.Done() to respect cancellation
//	  └── If timeout exceeded, context is cancelled
//	  └── Action returns ctx.Err() (context.DeadlineExceeded)
//
// ## Timeout vs Failure
//
//	Timeout:
//	  - Context cancelled by executor
//	  - Action was taking too long
//	  - Error: context.DeadlineExceeded
//	  - May indicate resource contention or network issues
//
//	Failure:
//	  - Action returns error by design
//	  - Could be business logic failure
//	  - Error: application-specific
//	  - May indicate misconfiguration or permanent issues
//
// ## FSM Retry Behavior
//
// Whether timeout or failure, FSM handles them the same way:
//
//  1. Action returns error
//  2. FSM stays in current state
//  3. Next tick: State.Next() returns same action
//  4. Action executes again
//
// The retry interval is determined by tick interval (~100ms default).
// There is NO built-in exponential backoff between retries.
//
// # Implementing Backoff
//
// For production systems, backoff can be implemented at different levels:
//
// ## Level 1: Action-Internal Backoff (Recommended)
//
//	type RetryableAction struct {
//	    attemptCount int
//	    baseDelay    time.Duration
//	    maxDelay     time.Duration
//	}
//
//	func (a *RetryableAction) Execute(ctx context.Context, deps any) error {
//	    delay := a.calculateBackoff()
//	    select {
//	    case <-time.After(delay):
//	    case <-ctx.Done():
//	        return ctx.Err()
//	    }
//
//	    a.attemptCount++
//	    return a.doActualWork(ctx, deps)
//	}
//
//	func (a *RetryableAction) calculateBackoff() time.Duration {
//	    delay := a.baseDelay * time.Duration(1<<a.attemptCount)
//	    if delay > a.maxDelay {
//	        delay = a.maxDelay
//	    }
//	    return delay
//	}
//
// ## Level 2: State-Based Backoff
//
// State can track attempts and delay transitions:
//
//	func (s *TryingToConnectState) Next(snap any) (State, Signal, Action) {
//	    // Check if enough time has passed since last attempt
//	    if snap.Observed.TimeSinceLastAttempt < s.calculateBackoff() {
//	        return s, SignalNone, nil // Wait, don't retry yet
//	    }
//	    return s, SignalNone, &ConnectAction{}
//	}
//
// ## Level 3: Restart-Based Recovery
//
// After N failures, trigger full worker restart:
//
//	restart_after_failures: 5
//
// This is implemented in examplefailing worker via SignalNeedsRestart.
//
// # What to Observe in Logs
//
// Quick worker (completes immediately):
//
//	"connect_attempting" delay_seconds=0
//	"connect_succeeded"
//	State: Stopped → TryingToConnect → Connected
//
// Slow worker (2-second delay):
//
//	"connect_attempting" delay_seconds=2
//	[2 second pause]
//	"Connect delay completed successfully"
//	"Connect action completed"
//	State: Stopped → TryingToConnect → Connected
//
// Combined slow + failing worker:
//
//	"connect_attempting" delay_seconds=1
//	"connect_failed_simulated" attempt=1 remaining=2
//	[1 second pause due to action delay]
//	"connect_failed_simulated" attempt=2 remaining=1
//	[1 second pause]
//	"connect_failed_simulated" attempt=3 remaining=0
//	[1 second pause]
//	"connect_succeeded_after_failures" total_attempts=4
//
// # Real-World Parallels
//
// Quick Connection:
//   - Local Redis instance
//   - In-memory database
//   - Fast network resources
//
// Slow Connection:
//   - Remote database with high latency
//   - TLS handshake with certificate chain validation
//   - OPC UA discovery and session establishment
//
// Failure + Timeout:
//   - Database under heavy load (slow + may reject connections)
//   - Network flapping (sometimes fast, sometimes slow, sometimes fails)
//   - Rate-limited API (fails with 429, needs backoff)
//
// # Hierarchy Created
//
//	timeout-quick (exampleslow with 0 delay)
//	timeout-slow (exampleslow with 2s delay)
//	timeout-retry (examplefailing with 3 max failures)
//
// Note: These are siblings at root level, not parent-child.
// Each demonstrates a different timeout/retry pattern independently.
var TimeoutScenario = Scenario{
	Name: "timeout",

	Description: "Demonstrates action timeout handling and retry behavior patterns",

	YAMLConfig: `
children:
  # Quick action - completes immediately
  # Demonstrates: Normal fast path, no delay
  - name: "timeout-quick"
    workerType: "exampleslow"
    userSpec:
      config: |
        delaySeconds: 0

  # Slow action - takes 2 seconds
  # Demonstrates: Long-running action that succeeds
  - name: "timeout-slow"
    workerType: "exampleslow"
    userSpec:
      config: |
        delaySeconds: 2

  # Failing action - fails 3 times then succeeds
  # Demonstrates: Retry behavior without backoff delay
  - name: "timeout-retry"
    workerType: "examplefailing"
    userSpec:
      config: |
        should_fail: true
        max_failures: 3

  # Combined pattern - slow + failing
  # This uses failing worker which has its own timing due to retry loop
  # Demonstrates: Multiple failure types in combination
  - name: "timeout-combined"
    workerType: "examplefailing"
    userSpec:
      config: |
        should_fail: true
        max_failures: 5
        restart_after_failures: 10
`,
}
