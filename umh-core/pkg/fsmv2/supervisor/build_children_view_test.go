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

package supervisor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// fakeChildSupervisor is a minimal SupervisorInterface stub used to drive
// buildChildrenView in tests. Only the accessor methods that buildChildInfo
// reads return meaningful values; the rest panic to flag accidental use.
type fakeChildSupervisor struct {
	supervisor.SupervisorInterface

	stateName     string
	stateReason   string
	workerType    string
	hierarchyPath string
	phase         config.LifecyclePhase
	stale         bool
	circuitOpen   bool
}

func (f *fakeChildSupervisor) GetCurrentStateNameAndReason() (string, string) {
	return f.stateName, f.stateReason
}
func (f *fakeChildSupervisor) GetLifecyclePhase() config.LifecyclePhase { return f.phase }
func (f *fakeChildSupervisor) GetWorkerType() string                    { return f.workerType }
func (f *fakeChildSupervisor) GetHierarchyPath() string                 { return f.hierarchyPath }
func (f *fakeChildSupervisor) IsObservationStale() bool                 { return f.stale }
func (f *fakeChildSupervisor) IsCircuitOpen() bool                      { return f.circuitOpen }

// Other SupervisorInterface methods (Start, AddWorker, GetDebugInfo, etc.)
// fall through to the embedded nil interface; buildChildrenView never invokes
// them, so there is no need for explicit stubs. When buildChildInfo grows a
// new accessor, add the matching stub above — calling an unimplemented method
// on the nil embed would panic with a nil-pointer dereference at runtime.

var _ = Describe("buildChildrenView", func() {
	It("emits children in deterministic name-sorted order regardless of map insertion order", func() {
		// makeChildren rebuilds a fresh map per iteration on purpose: Go
		// randomises map iteration order at construction time, so hoisting
		// the allocation out of the loop would freeze a single random order
		// and silently break the determinism check.
		makeChildren := func() map[string]supervisor.SupervisorInterface {
			return map[string]supervisor.SupervisorInterface{
				"gamma": &fakeChildSupervisor{
					stateName: "Connected", phase: config.PhaseRunningHealthy, workerType: "test",
				},
				"alpha": &fakeChildSupervisor{
					stateName: "Connected", phase: config.PhaseRunningHealthy, workerType: "test",
				},
				"beta": &fakeChildSupervisor{
					stateName: "Reconnecting", phase: config.PhaseRunningDegraded, workerType: "test",
				},
				"delta": &fakeChildSupervisor{
					stateName: "Stopped", phase: config.PhaseStopped, workerType: "test",
				},
			}
		}

		// Iterate enough times to exercise Go's randomised map iteration order.
		for i := 0; i < 25; i++ {
			view := supervisor.TestBuildChildrenView(makeChildren())
			Expect(view.Children).To(HaveLen(4))
			Expect(view.Children[0].Name).To(Equal("alpha"))
			Expect(view.Children[1].Name).To(Equal("beta"))
			Expect(view.Children[2].Name).To(Equal("delta"))
			Expect(view.Children[3].Name).To(Equal("gamma"))
		}
	})

	It("propagates each child's lifecycle phase from the supervisor into ChildInfo.Phase", func() {
		view := supervisor.TestBuildChildrenView(map[string]supervisor.SupervisorInterface{
			"healthy": &fakeChildSupervisor{
				stateName: "Connected", phase: config.PhaseRunningHealthy, workerType: "test",
			},
			"degraded": &fakeChildSupervisor{
				stateName: "Reconnecting", phase: config.PhaseRunningDegraded, workerType: "test",
			},
			"stopped": &fakeChildSupervisor{
				stateName: "Stopped", phase: config.PhaseStopped, workerType: "test",
			},
		})

		byName := map[string]config.ChildInfo{}
		for _, c := range view.Children {
			byName[c.Name] = c
		}

		Expect(byName["healthy"].Phase).To(Equal(config.PhaseRunningHealthy))
		Expect(byName["healthy"].IsHealthy).To(BeTrue())
		Expect(byName["degraded"].Phase).To(Equal(config.PhaseRunningDegraded))
		Expect(byName["degraded"].IsHealthy).To(BeFalse())
		Expect(byName["stopped"].Phase).To(Equal(config.PhaseStopped))

		// Aggregate predicates use the Phase field.
		Expect(view.HealthyCount).To(Equal(1))
		Expect(view.UnhealthyCount).To(Equal(1)) // degraded
		Expect(view.AllHealthy).To(BeFalse())
		Expect(view.AllOperational).To(BeFalse()) // stopped is not operational
		Expect(view.AllStopped).To(BeFalse())
	})

	It("returns the same empty-children predicates for empty and nil maps", func() {
		empty := supervisor.TestBuildChildrenView(map[string]supervisor.SupervisorInterface{})
		nilMap := supervisor.TestBuildChildrenView(nil)

		for _, v := range []config.ChildrenView{empty, nilMap} {
			Expect(v.Children).To(BeEmpty())
			Expect(v.AllHealthy).To(BeTrue())
			Expect(v.AllOperational).To(BeTrue())
			Expect(v.AllStopped).To(BeTrue())
			Expect(v.HealthyCount).To(BeZero())
			Expect(v.UnhealthyCount).To(BeZero())
		}
	})
})
