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

// CascadeScenario demonstrates how failures propagate through parent-child hierarchies.
//
// # What This Scenario Tests
//
// This scenario demonstrates:
//
//  1. Child-to-Parent Health Propagation (upward cascade)
//     - When a child enters a failing/unhealthy state, the parent detects it
//     - Parent transitions from Running to Degraded
//     - Parent's observed state reflects the unhealthy child count
//
//  2. Recovery Flow (upward cascade resolution)
//     - When the failing child recovers, parent detects healthy state
//     - Parent transitions from Degraded back to Running
//     - System returns to fully operational state
//
//  3. Multi-Child Failure Handling
//     - Multiple children can fail independently
//     - Parent tracks total healthy/unhealthy counts
//     - All children must recover for parent to return to Running
//
// TODO: it is important to note the difference in "what can be implemented with FSM_v2 and what is built into FSM_V2.
// we need to clearly explain it here as the parent goes to degraded is not a standard pattern
// # Architecture: The Supervisor Pattern
//
// In FSM v2, parents don't directly control children. Instead:
//
//	Parent (exampleparent)
//	  └── Declares children in DeriveDesiredState().ChildrenSpecs
//	  └── Supervisor observes child health counts
//	  └── Parent state machine reacts to ChildrenUnhealthy count
//
//	Children (examplefailing)
//	  └── Each child is an independent FSM
//	  └── Child health determined by its observed state
//	  └── Children fail 3 times, then recover
//
// # Hierarchy Created
//
//	cascade-parent (exampleparent)
//	  ├── child-0 (examplefailing - fails 3x then recovers)
//	  └── child-1 (examplefailing - fails 3x then recovers)
//
// # Flow - Cascade Failure + Recovery
//
//	Timeline:
//	  T+0s:   Parent starts in Stopped, desired=running
//	  T+0.1s: Parent → TryingToStart (startup action)
//	  T+0.2s: Parent → Running (startup complete)
//	          Children created: child-0 + child-1 (both examplefailing)
//	  T+0.3s: Children start: Stopped → TryingToConnect
//	  T+0.4s: child-0: TryingToConnect, failure 1/3
//	  T+0.4s: child-1: TryingToConnect, failure 1/3
//	          Parent: ChildrenUnhealthy = 2
//	          Parent → Degraded (detected unhealthy children)
//	  T+0.5s: Children continue failing (failure 2/3)
//	  T+0.6s: Children continue failing (failure 3/3)
//	  T+0.7s: child-0 → Connected (recovery on attempt 4)
//	          Parent: ChildrenUnhealthy = 1 (still degraded)
//	  T+0.8s: child-1 → Connected (recovery on attempt 4)
//	          Parent: ChildrenUnhealthy = 0
//	          Parent → Running (all children healthy)
//
// # What to Observe in Logs
//
// Parent state transitions:
//
//	"state" from="Running:healthy" to="Running:degraded"  // Child failures detected
//	"state" from="Running:degraded" to="Running:healthy"  // All children recovered
//
// Child health counts (in parent's observed state):
//
//	"children_healthy": 0, "children_unhealthy": 2  // Both children failing
//	"children_healthy": 1, "children_unhealthy": 1  // One recovered
//	"children_healthy": 2, "children_unhealthy": 0  // All recovered
//
// Child state transitions:
//
//	"connect_failed_simulated" worker="child-0" attempt=1 remaining=2
//	"connect_failed_simulated" worker="child-1" attempt=1 remaining=2
//	...
//	"connect_succeeded_after_failures" worker="child-0" total_attempts=4
//	"connect_succeeded_after_failures" worker="child-1" total_attempts=4
//
// # Key Insight: Parent Degraded State
//
// The exampleparent worker has these states:
//
//	Stopped         - Not running
//	TryingToStart   - Starting up
//	Running:healthy - All children healthy (normal operation)
//	Running:degraded- Some children unhealthy (degraded operation)
//	TryingToStop    - Shutting down
//
// The Degraded state:
//   - Triggered when ChildrenUnhealthy > 0
//   - Still allows children to run (Running prefix matches ChildStartStates)
//   - Returns to Running:healthy when ChildrenUnhealthy == 0
//
// # ChildStartStates and Cascade
//
// Children are configured with:
//
//	ChildStartStates: []string{"TryingToStart", "Running"}
//
// This means children run when parent state starts with "TryingToStart" or "Running".
// Since Degraded state is "Running:degraded", children continue running.
//
// If you wanted children to STOP when parent degrades, you would use:
//
//	ChildStartStates: []string{"Running:healthy"}  // Only healthy parent
//
// TODO: is this really implemented?
//
// # Cascade Failure vs. Other Patterns
//
//	Transient Failure (failing scenario):
//	  - Single worker fails and recovers
//	  - No parent involvement
//	  - Tests retry logic
//
//	Cascade Failure (this scenario):
//	  - Child failures affect parent state
//	  - Multiple children can fail independently
//	  - Tests health propagation through hierarchy
//
//	Panic (panic scenario):
//	  - Worker crashes entirely
//	  - Process must restart
//	  - Tests crash recovery
//
// # How to Verify This Scenario
//
// When running this scenario, watch for these EXACT event sequences:
//
// Phase 1: Startup
//   - Parent starts: Stopped → TryingToStart → Running:healthy
//   - Children created: child-0, child-1 appear in logs
//
// Phase 2: Failures Begin
//   - Both children log: "connect_failed_simulated" attempt=1 remaining=2
//   - Parent transitions: Running:healthy → Running:degraded
//
// Phase 3: Partial Recovery (if staggered)
//   - First child succeeds: "connect_succeeded_after_failures" total_attempts=4
//   - Parent STAYS in Running:degraded (one child still failing)
//
// Phase 4: Full Recovery
//   - Second child succeeds: "connect_succeeded_after_failures" total_attempts=4
//   - Parent transitions: Running:degraded → Running:healthy
//
// Key Insight: Parent only becomes healthy when ALL children are healthy.
// Degraded state is NOT an error - it's normal resilience behavior.
var CascadeScenario = Scenario{
	Name: "cascade",

	Description: "Demonstrates cascade failure: child failures propagate to parent state, parent recovery when children heal",

	YAMLConfig: `
children:
  # Parent that creates 2 children of type examplefailing
  # Children will fail 3 times each, then recover
  # Parent will go to Degraded state while children are unhealthy
  # Parent will return to Running when all children recover
  - name: "cascade-parent"
    workerType: "exampleparent"
    userSpec:
      config: |
        children_count: 2
        child_worker_type: "examplefailing"
        child_config: |
          should_fail: true
          max_failures: 3
      variables:
        user:
          IP: "127.0.0.1"
          PORT: 8080
`,
}
