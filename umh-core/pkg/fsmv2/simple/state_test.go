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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

func snapWith(status Status[probeStatus]) fsmv2.Snapshot {
	return fsmv2.Snapshot{
		Observed: fsmv2.Observation[Status[probeStatus]]{Status: status},
		Desired:  &fsmv2.WrappedDesiredState[probeConfig]{},
	}
}

func stoppingSnap() fsmv2.Snapshot {
	desired := &fsmv2.WrappedDesiredState[probeConfig]{}
	desired.SetShutdownRequested(true)

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
		It("stays stopped while shutdown is requested", func() {
			s := &stoppedState[probeConfig, probeStatus]{}

			res := s.Next(stoppingSnap())

			Expect(res.State).To(BeIdenticalTo(s))
		})

		It("resumes running once shutdown clears", func() {
			s := &stoppedState[probeConfig, probeStatus]{}

			res := s.Next(snapWith(Status[probeStatus]{Degraded: false, Reason: "reachable"}))

			Expect(res.State).To(BeAssignableToTypeOf(&runningState[probeConfig, probeStatus]{}))
		})
	})
})
