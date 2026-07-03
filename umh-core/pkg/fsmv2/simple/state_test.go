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
	})
})
