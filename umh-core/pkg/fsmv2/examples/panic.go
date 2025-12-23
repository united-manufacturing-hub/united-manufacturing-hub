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

// PanicScenario demonstrates panic recovery in action handlers.
//
// # What This Scenario Tests
//
// This scenario showcases how the ActionExecutor handles panics gracefully:
//   - Action panics during execution (simulated critical failure)
//   - Panic is caught, logged, and converted to an error
//   - Worker pool remains functional and retries the action
//
// # Flow
//
//  1. Worker starts in Stopped state
//  2. Worker transitions to TryingToConnect (desired state is "running")
//  3. ConnectAction executes and panics (simulated crash)
//  4. Panic is recovered and logged as "action_panic" with stack trace
//  5. Worker remains in TryingToConnect and retries on next tick
//
// # What to Observe
//
// When running this scenario, you should see:
//   - "Simulating panic in connect action" warning
//   - "action_panic" log entries with stack traces
//   - Worker staying in TryingToConnect (never reaching Connected)
//
// # Configuration
//
// The worker is configured with:
//   - should_panic: true - causes ConnectAction to panic every time
//
// # Pattern Demonstrated
//
// Action Panic → Error (Recovered by ActionExecutor)
//
// This scenario demonstrates that panics in actions ARE recovered by the
// ActionExecutor. The defer/recover wrapper in executeWorkWithRecovery():
//   - Catches the panic before it propagates
//   - Logs the panic with full stack trace
//   - Clears the action from in-progress map
//   - Allows the worker to retry on the next tick
//
// Real-world examples that could cause panics:
//   - Nil pointer dereference in protocol handler
//   - Array index out of bounds during message parsing
//   - Unexpected type assertion failure
//
// # Resilience vs Visibility Trade-off
//
// Panic recovery improves system resilience (workers don't crash permanently)
// but may mask bugs. The "action_panic" log with stack trace ensures bugs
// remain visible for debugging while the system continues operating.
var PanicScenario = Scenario{
	Name: "panic",

	Description: "Demonstrates panic recovery in action handlers",

	YAMLConfig: `
children:
  - name: "panic-worker-1"
    workerType: "examplepanic"
    userSpec:
      config: |
        should_panic: true
`,
}
