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

package state_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	exampleparent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
)

// Pins the Enabled-only child-gating contract for parent Degraded:
// exampleparent children stay running while the parent is Degraded. They are
// emitted with Enabled=true and an empty ChildStartStates, so the supervisor's
// computeMappedState maps them to "running" for every alive parent state
// (computeMappedState returns DesiredStateRunning when ChildStartStates is empty).
// Before convergence, ChildStartStates listed only TryingToStart and Running,
// which mapped children to "stopped" during Degraded.
var _ = Describe("DegradedState child gating (Enabled-only)", func() {
	var stateObj *state.DegradedState

	BeforeEach(func() {
		stateObj = &state.DegradedState{}
	})

	It("keeps children running through parent Degraded (Enabled=true, no ChildStartStates)", func() {
		snap := fsmv2.Snapshot{
			Observed: fsmv2.Observation[exampleparent.ExampleparentStatus]{
				ChildrenHealthy:   0,
				ChildrenUnhealthy: 1,
			},
			Desired: &fsmv2.WrappedDesiredState[exampleparent.ExampleparentConfig]{
				Config: exampleparent.ExampleparentConfig{ChildrenCount: 1},
			},
		}

		result := stateObj.Next(snap)

		Expect(result.State).To(BeAssignableToTypeOf(&state.DegradedState{}),
			"parent stays Degraded while a child is unhealthy")
		Expect(result.Children).To(HaveLen(1),
			"parent must keep emitting its child during Degraded (not despawn)")

		child := result.Children[0]
		Expect(child.Enabled).To(BeTrue(),
			"child must carry run intent (Enabled=true) during parent Degraded")
		Expect(child.ChildStartStates).To(BeEmpty(),
			"empty ChildStartStates makes the child run for every alive parent state, "+
				"including Degraded — the converged Enabled-only contract")

		// Pin the gating CONSEQUENCE, not just the field shape: the supervisor's
		// mapping (GetMappedChildState mirrors supervisor.computeMappedState) must
		// resolve this child to "running" while the parent is Degraded.
		Expect(child.GetMappedChildState("Degraded")).To(Equal(config.DesiredStateRunning),
			"converged gating: child maps to running during parent Degraded")

		// Counter-direction: document the pre-convergence behavior this change
		// removes — the old ChildStartStates list mapped children to "stopped"
		// during Degraded (Degraded was not in the list).
		oldGating := config.ChildSpec{ChildStartStates: []string{"TryingToStart", "Running"}}
		Expect(oldGating.GetMappedChildState("Degraded")).To(Equal(config.DesiredStateStopped),
			"pre-convergence: the old list stopped children during Degraded")
	})
})
