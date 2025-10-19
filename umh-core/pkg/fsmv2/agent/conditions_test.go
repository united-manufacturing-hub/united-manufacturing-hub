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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/agent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	agent_monitor_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
)

var _ = Describe("IsFullyHealthy", func() {
	It("should return true when ServiceInfo is not nil and OverallHealth is Active", func() {
		observed := &agent.AgentMonitorObservedState{
			ServiceInfo: &agent_monitor_service.ServiceInfo{
				OverallHealth: models.Active,
			},
		}
		Expect(agent.IsFullyHealthy(observed)).To(BeTrue())
	})

	It("should return false when ServiceInfo is nil", func() {
		observed := &agent.AgentMonitorObservedState{
			ServiceInfo: nil,
		}
		Expect(agent.IsFullyHealthy(observed)).To(BeFalse())
	})

	It("should return false when OverallHealth is Degraded", func() {
		observed := &agent.AgentMonitorObservedState{
			ServiceInfo: &agent_monitor_service.ServiceInfo{
				OverallHealth: models.Degraded,
			},
		}
		Expect(agent.IsFullyHealthy(observed)).To(BeFalse())
	})

	It("should return false when OverallHealth is Neutral", func() {
		observed := &agent.AgentMonitorObservedState{
			ServiceInfo: &agent_monitor_service.ServiceInfo{
				OverallHealth: models.Neutral,
			},
		}
		Expect(agent.IsFullyHealthy(observed)).To(BeFalse())
	})
})

var _ = Describe("BuildDegradedReason", func() {
	It("should return 'No service info available' when ServiceInfo is nil", func() {
		observed := &agent.AgentMonitorObservedState{
			ServiceInfo: nil,
		}
		reason := agent.BuildDegradedReason(observed)
		Expect(reason).To(Equal("No service info available"))
	})

	It("should format health categories correctly", func() {
		observed := &agent.AgentMonitorObservedState{
			ServiceInfo: &agent_monitor_service.ServiceInfo{
				OverallHealth: models.Degraded,
				LatencyHealth: models.Active,
				ReleaseHealth: models.Neutral,
			},
		}
		reason := agent.BuildDegradedReason(observed)
		Expect(reason).To(Equal("Agent health: degraded (Latency: active, Release: neutral)"))
	})

	It("should handle all health categories as Active", func() {
		observed := &agent.AgentMonitorObservedState{
			ServiceInfo: &agent_monitor_service.ServiceInfo{
				OverallHealth: models.Active,
				LatencyHealth: models.Active,
				ReleaseHealth: models.Active,
			},
		}
		reason := agent.BuildDegradedReason(observed)
		Expect(reason).To(Equal("Agent health: active (Latency: active, Release: active)"))
	})
})
