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

package agent_monitor_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/agent_monitor"
)

var _ = Describe("DegradedState", func() {
	var (
		snapshot fsmv2.Snapshot
		desired  *agent_monitor.AgentMonitorDesiredState
		observed *agent_monitor.AgentMonitorObservedState
	)

	BeforeEach(func() {
		desired = &agent_monitor.AgentMonitorDesiredState{}
		observed = &agent_monitor.AgentMonitorObservedState{
			ServiceInfo: unhealthyServiceInfo(),
			CollectedAt: time.Now(),
		}
	})

	Describe("Next", func() {
		Context("when shutdown is requested", func() {
			var state *agent_monitor.DegradedState

			BeforeEach(func() {
				state = &agent_monitor.DegradedState{}
				desired.SetShutdownRequested(true)
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should transition to StoppingState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&agent_monitor.StoppingState{}))
			})

			It("should not signal anything", func() {
				_, signal, _ := state.Next(snapshot)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeNil())
			})
		})

		Context("when health recovered", func() {
			var state *agent_monitor.DegradedState

			BeforeEach(func() {
				state = &agent_monitor.DegradedState{}
				desired.SetShutdownRequested(false)
				observed.ServiceInfo = healthyServiceInfo()
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should transition to ActiveState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&agent_monitor.ActiveState{}))
			})

			It("should not signal anything", func() {
				_, signal, _ := state.Next(snapshot)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeNil())
			})
		})

		Context("when health still unhealthy", func() {
			var state *agent_monitor.DegradedState

			BeforeEach(func() {
				state = &agent_monitor.DegradedState{}
				desired.SetShutdownRequested(false)
				observed.ServiceInfo = unhealthyServiceInfo()
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should stay in DegradedState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(Equal(state))
			})

			It("should not signal anything", func() {
				_, signal, _ := state.Next(snapshot)
				Expect(signal).To(Equal(fsmv2.SignalNone))
			})

			It("should not return an action", func() {
				_, _, action := state.Next(snapshot)
				Expect(action).To(BeNil())
			})
		})

		Context("when invalid observed state type", func() {
			var state *agent_monitor.DegradedState

			BeforeEach(func() {
				state = &agent_monitor.DegradedState{}
				desired.SetShutdownRequested(false)
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: &invalidObservedState{},
				}
			})

			It("should stay in DegradedState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&agent_monitor.DegradedState{}))
			})

			It("should update reason", func() {
				nextState, _, _ := state.Next(snapshot)
				degradedState := nextState.(*agent_monitor.DegradedState)
				Expect(degradedState.Reason()).To(ContainSubstring("Invalid observed state type"))
			})
		})
	})

	Describe("String", func() {
		It("should return state name", func() {
			state := &agent_monitor.DegradedState{}
			Expect(state.String()).To(Equal("Degraded"))
		})
	})

	Describe("Reason", func() {
		Context("when reason is not set", func() {
			It("should return generic reason", func() {
				state := &agent_monitor.DegradedState{}
				Expect(state.Reason()).To(Equal("Monitoring degraded"))
			})
		})
	})
})
