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

package agent_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/agent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	agentmonitorservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
)

var _ = Describe("ActiveState", func() {
	var (
		state    *agent.ActiveState
		snapshot fsmv2.Snapshot
		desired  *agent.AgentMonitorDesiredState
		observed *agent.AgentMonitorObservedState
	)

	BeforeEach(func() {
		state = &agent.ActiveState{}
		desired = &agent.AgentMonitorDesiredState{}
		observed = &agent.AgentMonitorObservedState{
			ServiceInfo: healthyServiceInfo(),
			CollectedAt: time.Now(),
		}
	})

	Describe("Next", func() {
		Context("when shutdown is requested", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(true)
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should transition to StoppingState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&agent.StoppingState{}))
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

		Context("when ServiceInfo is nil", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.ServiceInfo = nil
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should transition to DegradedState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&agent.DegradedState{}))
			})

			It("should include reason for degraded state", func() {
				nextState, _, _ := state.Next(snapshot)
				degradedState := nextState.(*agent.DegradedState)
				Expect(degradedState.Reason()).To(ContainSubstring("No service info available"))
			})
		})

		Context("when ServiceInfo is unhealthy", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.ServiceInfo = unhealthyServiceInfo()
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should transition to DegradedState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&agent.DegradedState{}))
			})

			It("should include reason for degraded state", func() {
				nextState, _, _ := state.Next(snapshot)
				degradedState := nextState.(*agent.DegradedState)
				Expect(degradedState.Reason()).To(ContainSubstring("Agent health"))
			})
		})

		Context("when all conditions are normal", func() {
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				observed.ServiceInfo = healthyServiceInfo()
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: observed,
				}
			})

			It("should stay in ActiveState", func() {
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
			BeforeEach(func() {
				desired.SetShutdownRequested(false)
				snapshot = fsmv2.Snapshot{
					Desired:  desired,
					Observed: &invalidObservedState{},
				}
			})

			It("should transition to DegradedState", func() {
				nextState, _, _ := state.Next(snapshot)
				Expect(nextState).To(BeAssignableToTypeOf(&agent.DegradedState{}))
			})

			It("should include diagnostic reason", func() {
				nextState, _, _ := state.Next(snapshot)
				degradedState := nextState.(*agent.DegradedState)
				Expect(degradedState.Reason()).To(ContainSubstring("Invalid observed state type"))
			})
		})
	})

	Describe("String", func() {
		It("should return state name", func() {
			Expect(state.String()).To(Equal("Active"))
		})
	})

	Describe("Reason", func() {
		It("should return descriptive reason", func() {
			Expect(state.Reason()).To(Equal("Monitoring active with healthy metrics"))
		})
	})
})

func healthyServiceInfo() *agentmonitorservice.ServiceInfo {
	return &agentmonitorservice.ServiceInfo{
		OverallHealth: models.Active,
		LatencyHealth: models.Active,
		ReleaseHealth: models.Active,
	}
}

func unhealthyServiceInfo() *agentmonitorservice.ServiceInfo {
	return &agentmonitorservice.ServiceInfo{
		OverallHealth: models.Degraded,
		LatencyHealth: models.Degraded,
		ReleaseHealth: models.Active,
	}
}

type invalidObservedState struct{}

func (i *invalidObservedState) GetTimestamp() time.Time {
	return time.Now()
}

func (i *invalidObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &agent.AgentMonitorDesiredState{}
}
