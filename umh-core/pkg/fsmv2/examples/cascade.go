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

// CascadeScenario shows parent-child health propagation.
//
// What happens:
//  1. Parent starts with 2 children (examplefailing workers)
//  2. Children fail 3 times, parent goes to Degraded state
//  3. Children recover, parent returns to Running state
//
// Key concept: FSMv2 automatically injects ChildrenHealthy/ChildrenUnhealthy
// counts into parent's ObservedState. The parent's state machine reads these
// and decides when to transition to/from Degraded.
//
// See exampleparent/state/ for the transition logic.
var CascadeScenario = Scenario{
	Name: "cascade",

	Description: "Shows cascade failure: child failures propagate to parent state, parent recovery when children heal",

	YAMLConfig: `
children:
  # Parent that creates 2 children of type examplefailing
  # Children will fail 3 times per cycle, for 2 cycles total:
  # Cycle 1 (startup): Children fail 3x, then connect -> Parent reaches Running
  # Cycle 2 (runtime): Children disconnect, fail 3x again -> Parent goes Degraded
  # After cycle 2: Children connect permanently -> Parent returns to Running
  # This tests the complete cascade flow including Degraded state transitions.
  - name: "cascade-parent"
    workerType: "exampleparent"
    userSpec:
      config: |
        children_count: 2
        child_worker_type: "examplefailing"
        child_config: |
          should_fail: true
          max_failures: 3
          failure_cycles: 2
          recovery_delay_observations: 3
      variables:
        user:
          IP: "127.0.0.1"
          PORT: 8080
`,
}
