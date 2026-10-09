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

package generator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/bridgeadmission"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/agent_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	agentservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
)

var _ = Describe("AgentFromSnapshot", func() {
	agentWith := func(info agentservice.ServiceInfo) models.Agent {
		agent, _, _, _ := AgentFromSnapshot(&fsm.FSMInstanceSnapshot{
			CurrentState:      "degraded",
			DesiredState:      "active",
			LastObservedState: &agent_monitor.AgentObservedStateSnapshot{ServiceInfoSnapshot: info},
		}, zap.NewNop().Sugar())

		return agent
	}

	It("shows the instance as degraded and says that bridge admission is off", func() {
		agent := agentWith(agentservice.ServiceInfo{
			OverallHealth: models.Degraded,
			HealthMessage: bridgeadmission.AdmissionOffReason,
		})

		Expect(agent.Health.Category).To(Equal(models.Degraded))
		Expect(agent.Health.Message).To(Equal(bridgeadmission.AdmissionOffReason))

		core := DeriveCoreHealth(agent.Health, nil, nil, nil, nil, nil, nil, zap.NewNop().Sugar())
		Expect(core.Category).To(Equal(models.Degraded))
		Expect(core.Message).To(ContainSubstring("Agent: " + bridgeadmission.AdmissionOffReason))
	})

	It("keeps the general message when the agent gives no reason", func() {
		Expect(agentWith(agentservice.ServiceInfo{OverallHealth: models.Degraded}).Health.Message).To(Equal("Agent degraded"))
	})
})
