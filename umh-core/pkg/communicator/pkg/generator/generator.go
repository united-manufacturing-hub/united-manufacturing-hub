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
	"crypto/sha256"
	"fmt"
	"strconv"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

type StatusCollectorType struct {
	dog                   watchdog.Iface
	systemSnapshotManager *fsm.SnapshotManager
	logger                *zap.SugaredLogger
	configManager         config.ConfigManager
	topicBrowserCache     *topicbrowser.Cache
	topicBrowserSimulator *topicbrowser.Simulator
}

func NewStatusCollector(
	dog watchdog.Iface,
	systemSnapshotManager *fsm.SnapshotManager,
	configManager config.ConfigManager,
	logger *zap.SugaredLogger,
	topicBrowserCache *topicbrowser.Cache,
	topicBrowserSimulator *topicbrowser.Simulator,
) *StatusCollectorType {

	collector := &StatusCollectorType{
		dog:                   dog,
		systemSnapshotManager: systemSnapshotManager,
		logger:                logger,
		configManager:         configManager,
		topicBrowserCache:     topicBrowserCache,
		topicBrowserSimulator: topicBrowserSimulator,
	}

	return collector
}

func (s *StatusCollectorType) GenerateStatusMessage(isBootstrapped bool) *models.StatusMessage {

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
	contInst, ok := fsm.FindInstance(snapshot, constants.ContainerManagerName, constants.CoreInstanceName)
	if ok {
		containerData = ContainerFromSnapshot(contInst, s.logger)
	}

	// --- agent + release (only one instance) -------------------------------------------------------------
	var agentData models.Agent
	var agentDataReleaseChannel string
	var agentDataCurrentVersion string
	var agentDataVersions []models.Version
	agInst, ok := fsm.FindInstance(snapshot, constants.AgentManagerName, constants.AgentInstanceName)
	if ok {
		agentData, agentDataReleaseChannel, agentDataCurrentVersion, agentDataVersions = AgentFromSnapshot(agInst, s.logger)
	}

	// --- redpanda (only one instance) -------------------------------------------------------------
	var redpandaData models.Redpanda
	rpInst, ok := fsm.FindInstance(snapshot, constants.RedpandaManagerName, constants.RedpandaInstanceName)
	if ok {
		redpandaData = RedpandaFromSnapshot(rpInst, s.logger)
	}

	// --- data models (multiple instances, extracted from the config directly) -------------------------------------------------------------
	dataModels := s.configManager.GetDataModels()
	dataModelData := make([]models.DataModel, len(dataModels))
	for i, dataModel := range dataModels {
		// Extract the latest version from the versions map
		latestVersion := ""
		if len(dataModel.Versions) > 0 {
			// Find the highest version number
			highestVersion := 0
			for versionKey := range dataModel.Versions {
				if len(versionKey) > 1 && versionKey[0] == 'v' {
					if versionNum := parseVersionNumber(versionKey); versionNum > highestVersion {
						highestVersion = versionNum
						latestVersion = versionKey
					}
				}
			}
			// If no versioned keys found, use the first available key
			if latestVersion == "" {
				for versionKey := range dataModel.Versions {
					latestVersion = versionKey
					break
				}
			}
		}

		// Generate a simple hash from the structure (placeholder implementation)
		hash := generateDataModelHash(dataModel)

		dataModelData[i] = models.DataModel{
			Name:          dataModel.Name,
			Description:   dataModel.Description,
			LatestVersion: latestVersion,
			Hash:          hash,
		}
	}

	// --- dfc (multiple instances) ----------------------	---------------------------------------
	var dfcData []models.Dfc
	dfcMgr, ok := fsm.FindManager(snapshot, constants.DataflowcomponentManagerName)
	if ok {
		dfcData = DfcsFromSnapshot(dfcMgr, s.logger)
	}

	// --- protocol converters (multiple instances) as DFCs --------------------------
	protocolConverterMgr, ok := fsm.FindManager(snapshot, constants.ProtocolConverterManagerName)
	if ok {
		protocolConverterDfcs := ProtocolConvertersFromSnapshot(protocolConverterMgr, s.logger)
		dfcData = append(dfcData, protocolConverterDfcs...)
	}

	// --- topic browser -------------------------------------------------------------
	topicBrowserData := &models.TopicBrowser{}

	if s.topicBrowserSimulator.GetSimulatorEnabled() {
		topicBrowserData = GenerateTopicBrowser(s.topicBrowserCache, s.topicBrowserSimulator.GetSimObservedState(), isBootstrapped, s.logger)
	} else {
		inst, ok := fsm.FindInstance(snapshot, constants.TopicBrowserManagerName, constants.TopicBrowserInstanceName)
		if !ok {
			s.logger.Error("Topic browser instance not found")
		} else if inst == nil || inst.LastObservedState == nil {
			s.logger.Error("Topic browser instance has nil observed state or is nil")
		} else {
			obs := inst.LastObservedState.(*topicbrowserfsm.ObservedStateSnapshot)
			topicBrowserData = GenerateTopicBrowser(s.topicBrowserCache, obs, isBootstrapped, s.logger)
		}
	}

	// Step 3: Create the status message
	statusMessage := &models.StatusMessage{
		Core: models.Core{
			Agent: models.Agent{
				Health:   agentData.Health,
				Latency:  &models.Latency{},
				Location: agentData.Location,
			},
			Container:    containerData,
			Dfcs:         dfcData,
			Redpanda:     redpandaData,
			TopicBrowser: *topicBrowserData,
			DataModels:   dataModelData,
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
					"action-deploy-protocol-converter",
					"action-get-logs",
					"action-get-config-file",
					"action-set-config-file",
					"action-get-data-flow-component-metrics",
					"log-logs-suppression", // Prevents logging of GetLogs action results to avoid log flooding when UI auto-refreshes logs (see HandleActionMessage GetLogs suppression for details)
					"core-health",
					"pause-dfc",
					"action-get-metrics",
					"action-delete-protocol-converter",
					"action-edit-protocol-converter",
					"protocol-converter-logs",
				},
			},
		},
	}

	// Derive core health from other healths
	statusMessage.Core.Health = DeriveCoreHealth(
		statusMessage.Core.Agent.Health,
		statusMessage.Core.Container.Health,
		statusMessage.Core.Redpanda.Health,
		statusMessage.Core.Release.Health,
		dfcData,
		s.logger,
	)

	return statusMessage
}

// parseVersionNumber parses a version string (e.g., "v1", "v2") to an integer
func parseVersionNumber(versionStr string) int {
	versionNum, err := strconv.Atoi(versionStr[1:])
	if err != nil {
		return 0
	}
	return versionNum
}

// generateDataModelHash generates a simple hash from the data model structure
func generateDataModelHash(dataModel config.DataModelsConfig) string {
	if len(dataModel.Versions) == 0 {
		return ""
	}

	// Create a hash from the data model name and version count
	h := sha256.New()
	h.Write([]byte(dataModel.Name))
	h.Write([]byte(fmt.Sprintf("%d", len(dataModel.Versions))))

	// Add version keys to the hash for consistency
	for versionKey := range dataModel.Versions {
		h.Write([]byte(versionKey))
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:16] // Return first 16 characters
}
