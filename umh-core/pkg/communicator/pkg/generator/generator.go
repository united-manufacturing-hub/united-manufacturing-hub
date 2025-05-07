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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

const (
	// Manager name constants
	// TODO: clean up these constants
	containerManagerName         = logger.ComponentContainerManager + "_" + constants.DefaultManagerName
	benthosManagerName           = logger.ComponentBenthosManager + "_" + constants.DefaultManagerName
	agentManagerName             = logger.ComponentAgentManager + "_" + constants.DefaultManagerName
	redpandaManagerName          = logger.ComponentRedpandaManager + constants.DefaultManagerName
	dataflowcomponentManagerName = constants.DataflowcomponentManagerName
	// Instance name constants
	coreInstanceName     = "Core"
	agentInstanceName    = "agent"
	redpandaInstanceName = "redpanda"
)

type StatusCollectorType struct {
	dog                   watchdog.Iface
	systemSnapshotManager *fsm.SnapshotManager
	logger                *zap.SugaredLogger
	configManager         config.ConfigManager
}

func NewStatusCollector(
	dog watchdog.Iface,
	systemSnapshotManager *fsm.SnapshotManager,
	configManager config.ConfigManager,
	logger *zap.SugaredLogger,
) *StatusCollectorType {

	collector := &StatusCollectorType{
		dog:                   dog,
		systemSnapshotManager: systemSnapshotManager,
		logger:                logger,
		configManager:         configManager,
	}

	return collector
}

func (s *StatusCollectorType) getDataFlowComponentData() ([]models.Dfc, error) {
	var dfcData []models.Dfc

	snapshot := s.systemSnapshotManager.GetDeepCopySnapshot()
	if dataflowcomponentManager, exists := snapshot.Managers[constants.DataflowcomponentManagerName]; exists {
		instances := dataflowcomponentManager.GetInstances()

		for _, instance := range instances {
			dfc, err := buildDataFlowComponentDataFromSnapshot(*instance, s.logger)
			if err != nil {
				s.logger.Error("Error building dataflowcomponent data", zap.Error(err))
				continue
			}
			dfcData = append(dfcData, dfc)
		}
	} else {
		s.logger.Warn("Dataflowcomponent manager not found in system snapshot",
			zap.String("managerName", constants.DataflowcomponentManagerName),
			zap.Any("allManagers", snapshot.Managers))
		return nil, fmt.Errorf("dataflowcomponent manager not found in system snapshot")
	}

	return dfcData, nil
}

// findInstance finds an instance in the system snapshot
// this is useful if we want to fetch the data from a manager, that always has one instance (e.g., core, agent, container, redpanda)
// returns nil if the instance is not found
func findInstance(
	snap fsm.SystemSnapshot,
	managerName, instanceName string,
) (*fsm.FSMInstanceSnapshot, bool) {

	mgr, ok := snap.Managers[managerName]
	if !ok {
		return nil, false
	}
	inst, ok := mgr.GetInstances()[instanceName]
	return inst, ok
}

// findManager finds a manager in the system snapshot
// returns nil if the manager is not found
func findManager(
	snap fsm.SystemSnapshot,
	managerName string,
) (fsm.ManagerSnapshot, bool) {
	mgr, ok := snap.Managers[managerName]
	return mgr, ok
}

func (s *StatusCollectorType) GenerateStatusMessage() *models.StatusMessage {

	// Step 1: Get the snapshot
	snapshot := s.systemSnapshotManager.GetDeepCopySnapshot()
	if len(snapshot.Managers) == 0 {
		sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "[GenerateStatusMessage] State is nil, using empty state")
		s.logger.Error("State is nil, using empty state")
		return nil
	}

	// Step 2: Build the status message

	// --- container (only one instance) ---------------------------------------------------------
	var containerData models.Container
	contInst, ok := findInstance(snapshot, containerManagerName, coreInstanceName)
	if ok {
		containerData = buildContainerDataFromInstanceSnapshot(*contInst, s.logger)
	}

	// --- agent (only one instance) -------------------------------------------------------------
	var agentData models.Agent
	var agentDataReleaseChannel string
	agInst, ok := findInstance(snapshot, agentManagerName, agentInstanceName)
	if ok {
		agentData, agentDataReleaseChannel = agentFromInstanceSnapshot(agInst, s.logger)
	}

	// --- redpanda (only one instance) -------------------------------------------------------------
	var redpandaData models.Redpanda
	rpInst, ok := findInstance(snapshot, redpandaManagerName, redpandaInstanceName)
	if ok {
		redpandaData = redpandaFromInstanceSnapshot(rpInst, s.logger)
	}

	// --- dfc (multiple instances) ----------------------	---------------------------------------
	var dfcData []models.Dfc
	dfcMgr, ok := findManager(snapshot, constants.DataflowcomponentManagerName)
	if ok {
		dfcData = dfcsFromManagerSnapshot(dfcMgr, s.logger)
	}

	// Create the status message
	statusMessage := &models.StatusMessage{
		Core: models.Core{
			Agent: models.Agent{
				Health:   agentData.Health,
				Latency:  &models.Latency{},
				Location: agentData.Location,
			},
			Container:        containerData,
			Dfcs:             dfcData,
			Redpanda:         redpandaData,
			UnifiedNamespace: models.UnifiedNamespace{},
			Release: models.Release{
				Health: &models.Health{
					Message:       "release monitoring is not implemented yet",
					ObservedState: "n/a",
					DesiredState:  "running",
					Category:      models.Neutral,
				},
				Version: "n/a",
				Channel: agentDataReleaseChannel,
				SupportedFeatures: []string{
					"custom-dfc",
					"action-deploy-data-flow-component",
					"action-get-data-flow-component",
					"action-delete-data-flow-component",
					"action-edit-data-flow-component",
				},
			},
		},
	}

	// Derive and set core health from other healths
	//TODO: set core health from other healths
	statusMessage.Core.Health = &models.Health{
		Message:       "core monitoring is not implemented yet",
		ObservedState: "running",
		DesiredState:  "running",
		Category:      models.Active,
	}

	return statusMessage
}

// getHealthMessage returns an appropriate message based on health category
func getHealthMessage(health models.HealthCategory) string {
	switch health {
	case models.Active:
		return "Component is operating normally"
	case models.Degraded:
		return "Component stopped working"
	case models.Neutral:
		return "Component status is neutral"
	default:
		return "Component status unknown"
	}
}
