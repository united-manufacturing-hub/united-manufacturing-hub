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

// FailingScenario demonstrates action failure handling with retry and recovery patterns.
//
// # What This Scenario Tests
//
// This scenario showcases two contrasting FSM reliability patterns:
//
//  1. Transient Failure + Recovery (failing-worker-recovery)
//     - Action fails a fixed number of times, then succeeds
//     - Demonstrates the common "retry until success" pattern
//
//  2. Permanent Failure (failing-worker-permanent)
//     - Action never succeeds (simulates misconfigured or unreachable service)
//     - Demonstrates what happens when a worker stays stuck indefinitely
//     - The FSM has NO built-in max retry limit - it retries forever
//
// # Flow - Recovery Worker
//
//  1. Worker starts in Stopped state
//  2. Worker transitions to TryingToConnect (desired state is "running")
//  3. ConnectAction executes, returns error (failure 1 of 3)
//  4. FSM stays in TryingToConnect, Next() returns same action
//  5. ConnectAction fails again (failure 2 of 3)
//  6. ConnectAction fails again (failure 3 of 3)
//  7. ConnectAction succeeds (attempt 4)
//  8. Worker transitions to Connected state
//
// # Flow - Permanent Failure Worker
//
//  1. Worker starts in Stopped state
//  2. Worker transitions to TryingToConnect
//  3. ConnectAction executes, returns error (failure 1 of 999999)
//  4. FSM stays in TryingToConnect, retries forever
//  5. Worker NEVER reaches Connected state
//  6. Operator must intervene (change config or restart)
//
// # What to Observe
//
// In logs, you should see:
//
// Recovery worker (healthy pattern):
//   - "connect_attempting" with attempt=1,2,3 and max_failures=3
//   - "connect_failed_simulated" with remaining=2,1,0
//   - "connect_succeeded_after_failures" with total_attempts=4
//   - State: TryingToConnect → Connected
//
// Permanent failure worker (stuck pattern):
//   - "connect_attempting" with attempt=1,2,3,4,5... (incrementing forever)
//   - "connect_failed_simulated" with remaining=999998,999997... (huge numbers)
//   - NO "connect_succeeded" message ever
//   - State: Stays in TryingToConnect indefinitely
//
// # Key Insight: FSM Retry Behavior
//
// The FSM does NOT implement automatic exponential backoff between action retries.
// Instead, retry happens via the reconciliation loop:
//   - Action fails → FSM stays in current state
//   - Next tick: State.Next() called again → returns same action
//   - Action executes again
//
// The delay between retries is determined by the tick interval (~100ms by default),
// NOT by an exponential backoff algorithm. For real exponential backoff, the action
// itself must implement delays, or the state must use a timer-based approach.
//
// # When to Use Each Pattern
//
// Recovery pattern (max_failures: N):
//   - Testing transient network issues
//   - Testing database reconnection
//   - Testing external API rate limiting
//
// Permanent failure pattern (max_failures: huge):
//   - Testing stuck worker detection
//   - Testing operator alerting
//   - Testing manual intervention workflows
//
// # Real-World Parallels
//
// Recovery: Network drops for 30 seconds, then reconnects
// Permanent: Wrong IP address configured, will never connect
var FailingScenario = Scenario{
	Name: "failing",

	Description: "Demonstrates action failure handling with recovery vs permanent failure patterns",

	YAMLConfig: `
children:
  # Worker that recovers after 3 failures - demonstrates transient failure handling
  - name: "failing-worker-recovery"
    workerType: "examplefailing"
    userSpec:
      config: |
        should_fail: true
        max_failures: 3

  # Worker that never succeeds - demonstrates permanent failure (stuck worker)
  - name: "failing-worker-permanent"
    workerType: "examplefailing"
    userSpec:
      config: |
        should_fail: true
        max_failures: 999999

  # Worker that triggers full restart after 5 failures - demonstrates SignalNeedsRestart
  # After 5 consecutive failures, the worker emits SignalNeedsRestart which triggers:
  # 1. Graceful shutdown (worker goes through cleanup states)
  # 2. Worker state reset to initial
  # 3. Worker restarts fresh and tries again
  - name: "failing-worker-restart"
    workerType: "examplefailing"
    userSpec:
      config: |
        should_fail: true
        max_failures: 999999
        restart_after_failures: 5
`,
}
