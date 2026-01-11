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
// # What This Scenario Tests
//
// This scenario showcases the FSM's handling of actions that take significant time:
//   - Action simulates a slow operation (e.g., connection establishment)
//   - FSM waits for action to complete without blocking other workers
//   - Context cancellation properly interrupts the action on shutdown
//
// # Flow (Single Pattern - Easy to Trace)
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
// In logs, you should see:
//   - "connect_attempting" with delay_seconds: 2
//   - 2-second pause during action execution
//   - "Connect delay completed successfully"
//   - "Connect action completed"
//   - State transition: TryingToConnect → Connected
//
// # Shutdown Behavior
//
// If shutdown is requested during the delay:
//   - "Connect action cancelled during delay" is logged
//   - Action returns ctx.Err() immediately
//   - Worker transitions to TryingToStop instead of Connected
//
// # Configuration
//
// The worker is configured with:
//   - delaySeconds: 2 - simulates 2-second connection establishment
//
// # Pattern Demonstrated
//
// Long-Running Action + Context-Aware Cancellation
//
// This is essential for graceful shutdown. Real-world examples:
//   - TCP connection establishment (may take several seconds)
//   - TLS handshake with certificate validation
//   - OPC UA session creation with discovery
//   - Database connection pool initialization
//
// The FSM ensures actions respect context cancellation, enabling
// clean shutdown without waiting for slow operations to complete.
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
