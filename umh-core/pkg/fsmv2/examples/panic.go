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

// PanicScenario demonstrates what happens when an action panics.
//
// # What This Scenario Tests
//
// This scenario showcases the current behavior when an action panics:
//   - Action panics during execution (simulated critical failure)
//   - Process crashes (panics propagate to the main goroutine)
//
// # Flow
//
//  1. Worker starts in Stopped state
//  2. Worker transitions to TryingToConnect (desired state is "running")
//  3. ConnectAction executes and panics (simulated crash)
//  4. Process crashes with panic stack trace
//
// # What to Observe
//
// When running this scenario, you should see:
//   - "Simulating panic in connect action" warning
//   - Panic stack trace and process exit
//
// # Configuration
//
// The worker is configured with:
//   - should_panic: true - causes ConnectAction to panic every time
//
// # Pattern Demonstrated
//
// Action Panic → Process Crash (current behavior)
//
// This scenario demonstrates that panics in actions are NOT recovered.
// In production, actions should use proper error handling instead of panicking.
// Real-world examples that could cause panics:
//   - Nil pointer dereference in protocol handler
//   - Array index out of bounds during message parsing
//   - Unexpected type assertion failure
//
// # Future Consideration
//
// Panic recovery could be added to the ActionExecutor to convert panics
// to errors, allowing the worker to retry. This would improve resilience
// but may mask bugs that should be fixed. For now, panics crash the process
// which makes bugs immediately visible during development.
var PanicScenario = Scenario{
	Name: "panic",

	Description: "Demonstrates what happens when an action panics (process crash)",

	YAMLConfig: `
children:
  - name: "panic-worker-1"
    workerType: "examplepanic"
    userSpec:
      config: |
        should_panic: true
`,
}
