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
// Two FSM reliability patterns:
//  1. Transient Failure + Recovery: fails N times, then succeeds
//  2. Permanent Failure: never succeeds, stays stuck indefinitely
//
// Retry happens via reconciliation loop at tick interval (~100ms), not exponential backoff.
var FailingScenario = Scenario{
	Name: "failing",

	Description: "Demonstrates action failure handling with recovery vs permanent failure patterns",

	YAMLConfig: `
children:
  # Worker that recovers after 3 failures
  - name: "failing-worker-recovery"
    workerType: "examplefailing"
    userSpec:
      config: |
        should_fail: true
        max_failures: 3

  # Worker that never succeeds (stuck worker)
  - name: "failing-worker-permanent"
    workerType: "examplefailing"
    userSpec:
      config: |
        should_fail: true
        max_failures: 999999

  # Worker that triggers full restart after 5 consecutive failures via SignalNeedsRestart
  - name: "failing-worker-restart"
    workerType: "examplefailing"
    userSpec:
      config: |
        should_fail: true
        max_failures: 999999
        restart_after_failures: 5
`,
}
