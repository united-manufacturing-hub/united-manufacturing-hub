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
	contInst, ok := fsm.FindInstance(snapshot, containerManagerName, coreInstanceName)
	if ok {
		containerData = ContainerFromSnapshot(contInst, s.logger)
	}

	// --- agent + release (only one instance) -------------------------------------------------------------
	var agentData models.Agent
	var agentDataReleaseChannel string
	var agentDataCurrentVersion string
	var agentDataVersions []models.Version
	agInst, ok := fsm.FindInstance(snapshot, agentManagerName, agentInstanceName)
	if ok {
		agentData, agentDataReleaseChannel, agentDataCurrentVersion, agentDataVersions = AgentFromSnapshot(agInst, s.logger)
	}

	// --- redpanda (only one instance) -------------------------------------------------------------
	var redpandaData models.Redpanda
	rpInst, ok := fsm.FindInstance(snapshot, redpandaManagerName, redpandaInstanceName)
	if ok {
		redpandaData = RedpandaFromSnapshot(rpInst, s.logger)
	}

	// --- dfc (multiple instances) ----------------------	---------------------------------------
	var dfcData []models.Dfc
	dfcMgr, ok := fsm.FindManager(snapshot, constants.DataflowcomponentManagerName)
	if ok {
		dfcData = DfcsFromSnapshot(dfcMgr, s.logger)
	}

	// Step 3: Create the status message
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
					Message:       "",
					ObservedState: "running",
					DesiredState:  "running",
					Category:      models.Active,
				},
				Version:  agentDataCurrentVersion,
				Versions: agentDataVersions,
				Channel:  agentDataReleaseChannel,
				SupportedFeatures: []string{
					"custom-dfc",
					"action-deploy-data-flow-component",
					"action-get-data-flow-component",
					"action-delete-data-flow-component",
					"action-edit-data-flow-component",
					"action-get-logs",
					"action-get-data-flow-component-metrics",
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
