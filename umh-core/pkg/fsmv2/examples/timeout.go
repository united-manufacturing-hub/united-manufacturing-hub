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
// Tests three timeout patterns:
//  1. Quick action - completes immediately (happy path)
//  2. Slow action - takes 2 seconds but completes
//  3. Failing action - fails 3 times then succeeds (retry behavior)
//
// # Action Timeout Architecture
//
// ActionExecutor enforces 30s default timeout per action. Actions must check
// ctx.Done() to respect cancellation. On timeout or failure, FSM stays in
// current state and retries on next tick (~100ms). No built-in exponential backoff.
//
// # Implementing Backoff
//
// Three levels: action-internal (recommended), state-based, or restart-based
// (via SignalNeedsRestart after N failures).
//
// # Hierarchy Created
//
//	timeout-quick (exampleslow with 0 delay)
//	timeout-slow (exampleslow with 2s delay)
//	timeout-retry (examplefailing with 3 max failures)
//	timeout-combined (examplefailing with 5 max failures)
var TimeoutScenario = Scenario{
	Name: "timeout",

	Description: "Demonstrates action timeout handling and retry behavior patterns",

	YAMLConfig: `
children:
  - name: "timeout-quick"
    workerType: "exampleslow"
    userSpec:
      config: |
        delaySeconds: 0

  - name: "timeout-slow"
    workerType: "exampleslow"
    userSpec:
      config: |
        delaySeconds: 2

  - name: "timeout-retry"
    workerType: "examplefailing"
    userSpec:
      config: |
        should_fail: true
        max_failures: 3

  - name: "timeout-combined"
    workerType: "examplefailing"
    userSpec:
      config: |
        should_fail: true
        max_failures: 5
        restart_after_failures: 10
`,
}
