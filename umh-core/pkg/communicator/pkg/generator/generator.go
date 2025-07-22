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
	"context"
	"time"

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
	dog                      watchdog.Iface
	systemSnapshotManager    *fsm.SnapshotManager
	logger                   *zap.SugaredLogger
	configManager            config.ConfigManager
	topicBrowserCommunicator *topicbrowser.TopicBrowserCommunicator
}

func NewStatusCollector(
	dog watchdog.Iface,
	systemSnapshotManager *fsm.SnapshotManager,
	configManager config.ConfigManager,
	logger *zap.SugaredLogger,
	topicBrowserCommunicator *topicbrowser.TopicBrowserCommunicator,
) *StatusCollectorType {

	collector := &StatusCollectorType{
		dog:                      dog,
		systemSnapshotManager:    systemSnapshotManager,
		logger:                   logger,
		configManager:            configManager,
		topicBrowserCommunicator: topicBrowserCommunicator,
	}

	return collector
}

// UpdateTopicBrowserCache processes new topic browser data using the communicator
// This automatically handles both real FSM data and simulated data based on the communicator mode
func (s *StatusCollectorType) UpdateTopicBrowserCache() error {
	s.logger.Debug("Updating topic browser cache")

	if s.topicBrowserCommunicator.IsSimulatorEnabled() {
		return s.updateTopicBrowserCacheFromSimulator()
	} else {
		return s.updateTopicBrowserCacheFromFSM()
	}
}

// updateTopicBrowserCacheFromSimulator updates the cache using simulated data
// This generates fake topic browser data for testing/demo purposes
func (s *StatusCollectorType) updateTopicBrowserCacheFromSimulator() error {
	s.logger.Debug("Updating topic browser cache from simulator")

	// Process simulated data
	result, err := s.topicBrowserCommunicator.ProcessSimulatedData()
	if err != nil {
		s.logger.Errorf("Failed to update topic browser cache from simulator: %v", err)
		return err
	}

	// Log what was processed for debugging
	s.logger.Infof("Topic browser cache updated from simulator: %s", result.DebugInfo)

	return nil
}

// updateTopicBrowserCacheFromFSM updates the cache using real FSM observed state
// This processes actual topic browser data from the running system
func (s *StatusCollectorType) updateTopicBrowserCacheFromFSM() error {
	s.logger.Debug("Updating topic browser cache from FSM")

	// Get the current system snapshot
	snapshot := s.systemSnapshotManager.GetDeepCopySnapshot()

	// Find the topic browser instance in the snapshot
	tbInstance, ok := fsm.FindInstance(snapshot, constants.TopicBrowserManagerName, constants.TopicBrowserInstanceName)
	if !ok || tbInstance == nil {
		s.logger.Debug("Topic browser instance not found in snapshot - system not ready yet")
		return nil // Not an error, just not ready yet
	}

	// Extract the observed state from the instance
	tbObservedState, ok := tbInstance.LastObservedState.(*topicbrowserfsm.ObservedStateSnapshot)
	if !ok || tbObservedState == nil {
		s.logger.Debug("Topic browser observed state not available - system not ready yet")
		return nil // Not an error, just not ready yet
	}

	// Process the FSM data using the communicator
	result, err := s.topicBrowserCommunicator.ProcessRealData(tbObservedState)
	if err != nil {
		s.logger.Errorf("Failed to update topic browser cache from FSM: %v", err)
		return err
	}

	// Log detailed debug information about what was processed
	s.logger.Debugf("Topic browser cache updated from FSM: %s", result.DebugInfo)

	if result.ProcessedCount > 0 {
		s.logger.Debugf("FSM processing details - new buffers: %d, latest timestamp: %s",
			result.ProcessedCount, result.LatestTimestamp.Format(time.RFC3339))
	}

	if result.SkippedCount > 0 {
		s.logger.Warnf("FSM processing warnings - skipped %d buffers due to processing errors",
			result.SkippedCount)
	}

	return nil
}

func (s *StatusCollectorType) GenerateStatusMessage(ctx context.Context, isBootstrapped bool) *models.StatusMessage {

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
	dataModelData, err := DataModelsFromConfig(ctx, s.configManager, s.logger)
	if err != nil {
		s.logger.Warnf("Failed to get data models from config: %v", err)
		return &models.StatusMessage{} // Return empty status message on error
	}

	// --- data contracts (multiple instances, extracted from the config directly) -------------------------------------------------------------
	dataContractData, err := DataContractsFromConfig(ctx, s.configManager, s.logger)
	if err != nil {
		s.logger.Warnf("Failed to get data contracts from config: %v", err)
		return &models.StatusMessage{} // Return empty status message on error
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

	// --- stream processors (multiple instances) as DFCs --------------------------
	streamProcessorMgr, ok := fsm.FindManager(snapshot, constants.StreamProcessorManagerName)
	if ok {
		streamProcessorDfcs := StreamProcessorsFromSnapshot(streamProcessorMgr, s.logger)
		dfcData = append(dfcData, streamProcessorDfcs...)
	}

	// --- topic browser -------------------------------------------------------------
	topicBrowserData := &models.TopicBrowser{}

	if s.topicBrowserCommunicator.IsSimulatorEnabled() {
		topicBrowserData = GenerateTopicBrowserFromCommunicator(s.topicBrowserCommunicator, isBootstrapped, s.logger, nil)
	} else {
		inst, ok := fsm.FindInstance(snapshot, constants.TopicBrowserManagerName, constants.TopicBrowserInstanceName)
		if !ok {
			s.logger.Error("Topic browser instance not found")
		} else if inst == nil || inst.LastObservedState == nil {
			s.logger.Error("Topic browser instance has nil observed state or is nil")
		} else {
			topicBrowserData = GenerateTopicBrowserFromCommunicator(s.topicBrowserCommunicator, isBootstrapped, s.logger, inst)
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
			Container:     containerData,
			Dfcs:          dfcData,
			Redpanda:      redpandaData,
			TopicBrowser:  *topicBrowserData,
			DataModels:    dataModelData,
			DataContracts: dataContractData,
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
		statusMessage.Core.TopicBrowser.Health,
		statusMessage.Core.Release.Health,
		dfcData,
		s.logger,
	)

	return statusMessage
}
