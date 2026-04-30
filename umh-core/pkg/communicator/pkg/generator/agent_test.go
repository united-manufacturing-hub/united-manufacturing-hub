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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	agentfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/agent_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	agentsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
	"go.uber.org/zap"
)

var _ = Describe("buildAgent (P13)", func() {
	var log *zap.SugaredLogger

	BeforeEach(func() {
		log = zap.NewNop().Sugar()
	})

	It("uses ServiceInfoSnapshot.HealthMessage when non-empty", func() {
		snap := &agentfsm.AgentObservedStateSnapshot{
			ServiceInfoSnapshot: agentsvc.ServiceInfo{
				OverallHealth: models.Degraded,
				HealthMessage: `Invalid agent.releaseChannel value "nigtly". Allowed: nightly, stable, enterprise.`,
				Release:       &models.Release{Channel: "stable", Version: "1.0.0"},
			},
		}
		instance := fsm.FSMInstanceSnapshot{
			ID:                "Core",
			CurrentState:      "active",
			DesiredState:      "active",
			LastObservedState: snap,
		}

		agent, _, _, _, err := buildAgent(instance, log)
		Expect(err).NotTo(HaveOccurred())
		Expect(agent.Health).NotTo(BeNil())
		Expect(agent.Health.Message).To(ContainSubstring("nigtly"))
		Expect(agent.Health.Message).To(ContainSubstring("agent.releaseChannel"))
		Expect(agent.Health.Category).To(Equal(models.Degraded))
	})

	It("falls back to canned message when HealthMessage is empty (Active)", func() {
		snap := &agentfsm.AgentObservedStateSnapshot{
			ServiceInfoSnapshot: agentsvc.ServiceInfo{
				OverallHealth: models.Active,
				Release:       &models.Release{Channel: "stable", Version: "1.0.0"},
			},
		}
		instance := fsm.FSMInstanceSnapshot{
			ID:                "Core",
			CurrentState:      "active",
			DesiredState:      "active",
			LastObservedState: snap,
		}

		agent, _, _, _, err := buildAgent(instance, log)
		Expect(err).NotTo(HaveOccurred())
		Expect(agent.Health).NotTo(BeNil())
		Expect(agent.Health.Message).To(Equal("Agent operating normally"))
	})

	It("ignores HealthMessage when category is Active (guards against stale message)", func() {
		snap := &agentfsm.AgentObservedStateSnapshot{
			ServiceInfoSnapshot: agentsvc.ServiceInfo{
				OverallHealth: models.Active,
				HealthMessage: "ignore me — stale Degraded message",
				Release:       &models.Release{Channel: "stable", Version: "1.0.0"},
			},
		}
		instance := fsm.FSMInstanceSnapshot{
			ID:                "Core",
			CurrentState:      "active",
			DesiredState:      "active",
			LastObservedState: snap,
		}

		agent, _, _, _, err := buildAgent(instance, log)
		Expect(err).NotTo(HaveOccurred())
		Expect(agent.Health).NotTo(BeNil())
		Expect(agent.Health.Message).To(Equal("Agent operating normally"))
	})

	It("falls back to canned message when HealthMessage is empty (Degraded)", func() {
		snap := &agentfsm.AgentObservedStateSnapshot{
			ServiceInfoSnapshot: agentsvc.ServiceInfo{
				OverallHealth: models.Degraded,
				Release:       &models.Release{Channel: "stable", Version: "1.0.0"},
			},
		}
		instance := fsm.FSMInstanceSnapshot{
			ID:                "Core",
			CurrentState:      "degraded",
			DesiredState:      "active",
			LastObservedState: snap,
		}

		agent, _, _, _, err := buildAgent(instance, log)
		Expect(err).NotTo(HaveOccurred())
		Expect(agent.Health).NotTo(BeNil())
		Expect(agent.Health.Message).To(Equal("Agent degraded"))
	})
})
