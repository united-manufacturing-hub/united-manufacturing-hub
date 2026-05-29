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
	exampleparent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
)

// Pins the Enabled-only child-gating contract for parent Degraded:
// exampleparent children stay running while the parent is Degraded. They are
// emitted with Enabled=true; the supervisor's disable-mapping pass turns that
// into the child's IsDisabled bit, and the child's ShouldStop() reads it. An
// enabled child therefore does not stop while the parent is Degraded.
var _ = Describe("DegradedState child gating (Enabled-only)", func() {
	var stateObj *state.DegradedState

	BeforeEach(func() {
		stateObj = &state.DegradedState{}
	})

	It("keeps children running through parent Degraded (Enabled=true)", func() {
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

		// Pin the gating CONSEQUENCE through live fields, not field shape: the
		// disable-mapping pass sets IsDisabled = !Enabled, and ShouldStop() reads
		// it. An enabled child resolves to ShouldStop()==false during Degraded.
		runningChild := fsmv2.WorkerSnapshot[any, any]{IsDisabled: !child.Enabled}
		Expect(runningChild.ShouldStop()).To(BeFalse(),
			"converged gating: an enabled child does not stop during parent Degraded")

		// Counter-direction: a disabled child (Enabled=false → IsDisabled=true)
		// stops. This is the behavior the old parent-state mapping expressed.
		disabledChild := fsmv2.WorkerSnapshot[any, any]{IsDisabled: true}
		Expect(disabledChild.ShouldStop()).To(BeTrue(),
			"a disabled child stops regardless of parent state")
	})
})
