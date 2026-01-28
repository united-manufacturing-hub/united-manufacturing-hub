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

// SlowScenario demonstrates long-running action handling and context cancellation.
//
// # Flow
//
//  1. Worker starts in Stopped state
//  2. Worker transitions to TryingToConnect (desired state is "running")
//  3. ConnectAction executes with 2-second delay
//  4. Action sleeps, checking for context cancellation
//  5. After delay completes, action succeeds
//  6. Worker transitions to Connected state
//
// # What to Observe
//
// In logs: "connect_attempting" with delay_seconds: 2, 2-second pause,
// "Connect delay completed successfully", state transition to Connected.
//
// On shutdown during delay: Action returns ctx.Err() immediately,
// worker transitions to TryingToStop instead of Connected.
var SlowScenario = Scenario{
	Name: "slow",

	Description: "Demonstrates long-running action handling and context cancellation",

	YAMLConfig: `
children:
  - name: "slow-worker-1"
    workerType: "exampleslow"
    userSpec:
      config: |
        delaySeconds: 2
`,
}
