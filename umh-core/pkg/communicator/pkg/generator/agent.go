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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/agent_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

func agentFromInstanceSnapshot(
	inst *fsm.FSMInstanceSnapshot,
	log *zap.SugaredLogger,
) (models.Agent, string) {

	if inst == nil {
		return buildDefaultAgentData()
	}
	return buildAgentDataFromInstanceSnapshot(*inst, log)
}

// buildAgentDataFromSnapshot creates agent data from a FSM instance snapshot
func buildAgentDataFromInstanceSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.SugaredLogger) (models.Agent, string) {
	agentData := models.Agent{}
	releaseChannel := "n/a"
	// Check if we have actual observedState
	if instance.LastObservedState != nil {
		// Try to cast to the right type
		if snapshot, ok := instance.LastObservedState.(*agent_monitor.AgentObservedStateSnapshot); ok {
			// Ensure all fields are valid before accessing
			if snapshot.ServiceInfoSnapshot.Location == nil {
				log.Warn("Agent location data is nil")
				agentData = models.Agent{
					Location: map[int]string{0: "Unknown location"},
				}
			} else {
				agentData = models.Agent{
					Location: snapshot.ServiceInfoSnapshot.Location,
				}
			}
			// build the health status
			agentData.Health = &models.Health{
				Message:       getHealthMessage(snapshot.ServiceInfoSnapshot.OverallHealth),
				ObservedState: instance.CurrentState,
				DesiredState:  instance.DesiredState,
				Category:      snapshot.ServiceInfoSnapshot.OverallHealth,
			}
			// Check if Release is nil before accessing its properties
			if snapshot.ServiceInfoSnapshot.Release != nil {
				releaseChannel = snapshot.ServiceInfoSnapshot.Release.Channel
			} else {
				log.Warn("Agent release data is nil, defaulting to stable")
				releaseChannel = "n/a"
			}
		} else {
			log.Warn("Agent observed state is not of expected type")
			sentry.ReportIssuef(sentry.IssueTypeError, log, "[buildAgentDataFromSnapshot] Agent observed state is not of expected type")
		}
	} else {
		log.Warn("Agent instance has no observed state")
	}

	return agentData, releaseChannel
}

// buildDefaultAgentData creates default agent data when no real data is available
func buildDefaultAgentData() (models.Agent, string) {
	agentData := models.Agent{
		Location: map[int]string{0: "Unknown location"},
	}
	releaseChannel := "n/a"
	return agentData, releaseChannel
}
