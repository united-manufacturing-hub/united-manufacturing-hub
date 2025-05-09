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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/agent_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

// AgentFromSnapshot converts an optional FSMInstanceSnapshot into a
// models.Agent and the corresponding release-channel string.
//
// If inst is nil or unusable we fall back to defaults *and* return
// "n/a" for the channel.
func AgentFromSnapshot(
	inst *fsm.FSMInstanceSnapshot,
	log *zap.SugaredLogger,
) (models.Agent, string, string, []models.Version) {

	if inst == nil {
		return defaultAgent(), "n/a", "n/a", []models.Version{}
	}

	agent, channel, currentVersion, versions, err := buildAgent(*inst, log)
	if err != nil {
		log.Error("unable to build agent data", zap.Error(err))
		return defaultAgent(), "n/a", "n/a", []models.Version{}
	}
	return agent, channel, currentVersion, versions
}

// buildAgent maps a **non-nil** instance snapshot to models.Agent.
// It returns “n/a” and an error if the observed state cannot be cast.
func buildAgent(
	instance fsm.FSMInstanceSnapshot,
	log *zap.SugaredLogger,
) (models.Agent, string, string, []models.Version, error) {

	snap, ok := instance.LastObservedState.(*agent_monitor.AgentObservedStateSnapshot)
	if !ok || snap == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, log,
			"[buildAgent] unexpected observed state %T", instance.LastObservedState)
		return defaultAgent(), "n/a", "n/a", []models.Version{}, fmt.Errorf("invalid observed-state")
	}

	agent := models.Agent{
		Location: snap.ServiceInfoSnapshot.Location,
		Health: &models.Health{
			Message:       getAgentHealthMessage(snap.ServiceInfoSnapshot.OverallHealth),
			ObservedState: instance.CurrentState,
			DesiredState:  instance.DesiredState,
			Category:      snap.ServiceInfoSnapshot.OverallHealth,
		},
	}

	channel := "n/a"
	if snap.ServiceInfoSnapshot.Release != nil {
		channel = snap.ServiceInfoSnapshot.Release.Channel
	}

	currentVersion := "n/a"
	if snap.ServiceInfoSnapshot.Release != nil {
		currentVersion = snap.ServiceInfoSnapshot.Release.Version
	}

	versions := []models.Version{}
	if snap.ServiceInfoSnapshot.Release != nil {
		versions = snap.ServiceInfoSnapshot.Release.Versions
	}

	return agent, channel, currentVersion, versions, nil
}

// defaultAgent returns hard-coded values when no snapshot information
// is available.
func defaultAgent() models.Agent {
	return models.Agent{
		Location: map[int]string{0: "Unknown location"},
		Health: &models.Health{
			Message:       "Agent status unknown",
			ObservedState: "unknown",
			DesiredState:  "running",
			Category:      models.Neutral,
		},
	}
}

// getHealthMessage is agent-specific. Extend as needed.
func getAgentHealthMessage(cat models.HealthCategory) string {
	switch cat {
	case models.Active:
		return "Agent operating normally"
	case models.Degraded:
		return "Agent degraded"
	default:
		return "Agent status unknown"
	}
}
