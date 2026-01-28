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

// ConcurrentScenario tests multiple independent workers running concurrently.
//
// This scenario verifies that:
// - Multiple workers can run simultaneously without interference
// - Each worker independently transitions through its state machine
// - No race conditions occur between workers
// - All workers reach their target states
//
// The scenario creates 5 independent helloworld workers that all start
// at the same time and run their state machines concurrently.
var ConcurrentScenario = Scenario{
	Name:        "concurrent",
	Description: "Tests multiple independent workers running concurrently without interference",
	YAMLConfig: `
children:
  - name: "concurrent-worker-1"
    workerType: "helloworld"
    location:
      - enterprise: "test"
      - site: "concurrent"
    userSpec:
      config: |
        state: running
        message: "Hello from worker 1"
  - name: "concurrent-worker-2"
    workerType: "helloworld"
    location:
      - enterprise: "test"
      - site: "concurrent"
    userSpec:
      config: |
        state: running
        message: "Hello from worker 2"
  - name: "concurrent-worker-3"
    workerType: "helloworld"
    location:
      - enterprise: "test"
      - site: "concurrent"
    userSpec:
      config: |
        state: running
        message: "Hello from worker 3"
  - name: "concurrent-worker-4"
    workerType: "helloworld"
    location:
      - enterprise: "test"
      - site: "concurrent"
    userSpec:
      config: |
        state: running
        message: "Hello from worker 4"
  - name: "concurrent-worker-5"
    workerType: "helloworld"
    location:
      - enterprise: "test"
      - site: "concurrent"
    userSpec:
      config: |
        state: running
        message: "Hello from worker 5"
`,
}
