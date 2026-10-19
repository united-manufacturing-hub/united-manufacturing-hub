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

package simple

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

var probedTarget = probeConfig{Target: "plc:502"}

func snapWith(status Status[probeStatus]) fsmv2.Snapshot {
	return fsmv2.Snapshot{
		Observed: fsmv2.Observation[Status[probeStatus]]{Status: status},
		Desired:  &fsmv2.WrappedDesiredState[probeConfig]{},
	}
}

func probedSnapWith(workerType string, status Status[probeStatus]) fsmv2.Snapshot {
	return fsmv2.Snapshot{
		Observed: fsmv2.Observation[Status[probeStatus]]{Status: status},
		Desired:  &fsmv2.WrappedDesiredState[probeConfig]{Config: probedTarget},
		Identity: deps.Identity{ID: "probe", WorkerType: workerType},
	}
}

func registerNoopProbe(workerType string) {
	registerProbe(workerType, &typedProbe[probeConfig, struct{}]{
		workerType: workerType,
		probe:      func(context.Context, struct{}, probeConfig) {},
	})
}

func stoppingSnap() fsmv2.Snapshot {
	desired := &fsmv2.WrappedDesiredState[probeConfig]{}
	desired.SetShutdownRequested(true)

	return fsmv2.Snapshot{
		Observed: fsmv2.Observation[Status[probeStatus]]{Status: Status[probeStatus]{}},
		Desired:  desired,
	}
}

func disabledSnap() fsmv2.Snapshot {
	desired := &fsmv2.WrappedDesiredState[probeConfig]{}
	desired.SetDisabled(true)

	return fsmv2.Snapshot{
		Observed: fsmv2.Observation[Status[probeStatus]]{Status: Status[probeStatus]{}},
		Desired:  desired,
	}
}

var _ = Describe("state machine", func() {
	Describe("runningState", func() {
		It("stays running and emits the reason when the verdict is healthy", func() {
			s := &runningState[probeConfig, probeStatus]{}

			res := s.Next(snapWith(Status[probeStatus]{Degraded: false, Reason: "running (no health check)"}))

			Expect(res.State).To(BeIdenticalTo(s))
			Expect(res.Reason).To(Equal("running (no health check)"))
		})

		It("flips to degraded, carrying the reason, when the verdict is degraded", func() {
			s := &runningState[probeConfig, probeStatus]{}

			res := s.Next(snapWith(Status[probeStatus]{Degraded: true, Reason: "poll error: dial timeout"}))

			Expect(res.State).To(BeAssignableToTypeOf(&degradedState[probeConfig, probeStatus]{}))
			Expect(res.Reason).To(Equal("poll error: dial timeout"))
		})

		It("stops on shutdown before considering the verdict", func() {
			s := &runningState[probeConfig, probeStatus]{}

			res := s.Next(stoppingSnap())

			Expect(res.State).To(BeAssignableToTypeOf(&stoppedState[probeConfig, probeStatus]{}))
		})
	})

	Describe("degradedState", func() {
		It("recovers to running when the verdict clears", func() {
			s := &degradedState[probeConfig, probeStatus]{}

			res := s.Next(snapWith(Status[probeStatus]{Degraded: false, Reason: "reachable"}))

			Expect(res.State).To(BeAssignableToTypeOf(&runningState[probeConfig, probeStatus]{}))
			Expect(res.Reason).To(Equal("reachable"))
		})

		It("stays degraded while the verdict persists", func() {
			s := &degradedState[probeConfig, probeStatus]{}

			res := s.Next(snapWith(Status[probeStatus]{Degraded: true, Reason: "still unreachable"}))

			Expect(res.State).To(BeIdenticalTo(s))
			Expect(res.Reason).To(Equal("still unreachable"))
		})

		It("stops on shutdown even while degraded", func() {
			s := &degradedState[probeConfig, probeStatus]{}

			res := s.Next(stoppingSnap())

			Expect(res.State).To(BeAssignableToTypeOf(&stoppedState[probeConfig, probeStatus]{}))
		})
	})

	Describe("stoppedState", func() {
		It("signals removal on a terminal shutdown", func() {
			s := &stoppedState[probeConfig, probeStatus]{}

			res := s.Next(stoppingSnap())

			Expect(res.State).To(BeIdenticalTo(s))
			Expect(res.Signal).To(Equal(fsmv2.SignalNeedsRemoval))
		})

		It("stays parked without signalling removal when only disabled", func() {
			s := &stoppedState[probeConfig, probeStatus]{}

			res := s.Next(disabledSnap())

			Expect(res.State).To(BeIdenticalTo(s))
			Expect(res.Signal).To(Equal(fsmv2.SignalNone))
		})

		It("resumes running once the stop clears", func() {
			s := &stoppedState[probeConfig, probeStatus]{}

			res := s.Next(snapWith(Status[probeStatus]{Degraded: false, Reason: "reachable"}))

			Expect(res.State).To(BeAssignableToTypeOf(&runningState[probeConfig, probeStatus]{}))
		})
	})

	Describe("probe emission", func() {
		It("emits a probe for the snapshot config when running stays running", func() {
			registerNoopProbe("simplestate_running_probe")

			state := &runningState[probeConfig, probeStatus]{}

			result := state.Next(probedSnapWith("simplestate_running_probe", Status[probeStatus]{Reason: "reachable"}))

			Expect(result.State).To(BeIdenticalTo(state))
			Expect(result.Action).To(Equal(&probeAction[probeConfig]{workerType: "simplestate_running_probe", cfg: probedTarget}))
		})

		It("emits a probe when degraded stays degraded, so a degraded worker keeps probing", func() {
			registerNoopProbe("simplestate_degraded_probe")

			state := &degradedState[probeConfig, probeStatus]{}

			result := state.Next(probedSnapWith("simplestate_degraded_probe", Status[probeStatus]{Degraded: true, Reason: "no result yet"}))

			Expect(result.State).To(BeIdenticalTo(state))
			Expect(result.Action).To(Equal(&probeAction[probeConfig]{workerType: "simplestate_degraded_probe", cfg: probedTarget}))
		})

		It("emits nothing for a worker type without a probe", func() {
			state := &runningState[probeConfig, probeStatus]{}

			result := state.Next(probedSnapWith("simplestate_no_probe", Status[probeStatus]{Reason: "reachable"}))

			Expect(result.Action).To(BeNil())
		})

		It("emits nothing in the step that switches running to degraded", func() {
			registerNoopProbe("simplestate_flip_down")

			state := &runningState[probeConfig, probeStatus]{}

			result := state.Next(probedSnapWith("simplestate_flip_down", Status[probeStatus]{Degraded: true, Reason: "stale"}))

			Expect(result.State).To(BeAssignableToTypeOf(&degradedState[probeConfig, probeStatus]{}))
			Expect(result.Action).To(BeNil())
		})

		It("emits nothing in the step that switches degraded to running", func() {
			registerNoopProbe("simplestate_flip_up")

			state := &degradedState[probeConfig, probeStatus]{}

			result := state.Next(probedSnapWith("simplestate_flip_up", Status[probeStatus]{Reason: "reachable"}))

			Expect(result.State).To(BeAssignableToTypeOf(&runningState[probeConfig, probeStatus]{}))
			Expect(result.Action).To(BeNil())
		})

		It("emits nothing while stopped", func() {
			registerNoopProbe("simplestate_stopped")

			desired := &fsmv2.WrappedDesiredState[probeConfig]{Config: probedTarget}
			desired.SetDisabled(true)

			state := &stoppedState[probeConfig, probeStatus]{}

			result := state.Next(fsmv2.Snapshot{
				Observed: fsmv2.Observation[Status[probeStatus]]{Status: Status[probeStatus]{}},
				Desired:  desired,
				Identity: deps.Identity{ID: "probe", WorkerType: "simplestate_stopped"},
			})

			Expect(result.Action).To(BeNil())
		})
	})

	Describe("state names", func() {
		// These names reach operators as log fields and as Prometheus label
		// values on state_transitions_total. The states are generic, so their
		// reflect names carry the full import path of every type argument
		// unless DeriveStateName drops them.
		It("names the states without their type arguments", func() {
			Expect((&runningState[probeConfig, probeStatus]{}).String()).To(Equal("running"))
			Expect((&degradedState[probeConfig, probeStatus]{}).String()).To(Equal("degraded"))
			Expect((&stoppedState[probeConfig, probeStatus]{}).String()).To(Equal("stopped"))
		})
	})
})
