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
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/agent_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

const (
	// Manager name constants
	containerManagerName = logger.ComponentContainerManager + "_" + constants.DefaultManagerName
	benthosManagerName   = logger.ComponentBenthosManager + "_" + constants.DefaultManagerName
	agentManagerName     = logger.ComponentAgentManager + "_" + constants.DefaultManagerName

	// Instance name constants
	coreInstanceName  = "Core"
	agentInstanceName = "agent"
)

type StatusCollectorType struct {
	latestData    *LatestData
	dog           watchdog.Iface
	state         *fsm.SystemSnapshot
	systemMu      *sync.Mutex
	logger        *zap.SugaredLogger
	configManager config.ConfigManager
}

type LatestData struct {
	mu sync.RWMutex // A mutex to synchronize access to the fields

	UnsTable   models.UnsTable
	EventTable models.EventsTable
}

func NewStatusCollector(
	dog watchdog.Iface,
	state *fsm.SystemSnapshot,
	systemMu *sync.Mutex,
	configManager config.ConfigManager,
	logger *zap.SugaredLogger,
) *StatusCollectorType {

	latestData := &LatestData{}

	collector := &StatusCollectorType{
		latestData:    latestData,
		dog:           dog,
		state:         state,
		systemMu:      systemMu,
		logger:        logger,
		configManager: configManager,
	}

	return collector
}

func (s *StatusCollectorType) getDataFlowComponentData() ([]models.Dfc, error) {
	var dfcData []models.Dfc

	if dataflowcomponentManager, exists := s.state.Managers[constants.DataflowcomponentManagerName]; exists {
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
			zap.Any("allManagers", s.state.Managers))
		return nil, fmt.Errorf("dataflowcomponent manager not found in system snapshot")
	}

	return dfcData, nil
}

func (s *StatusCollectorType) GenerateStatusMessage() *models.StatusMessage {
	s.latestData.mu.RLock()
	defer s.latestData.mu.RUnlock()

	// Lock state for reading and hold it until we're done accessing state data
	s.systemMu.Lock()
	defer s.systemMu.Unlock()

	if s.state == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "[GenerateStatusMessage] State is nil, using empty state")
		s.logger.Error("State is nil, using empty state")
		return nil
	}

	// Create container data from the container manager if available
	var containerData models.Container

	if containerManager, exists := s.state.Managers[containerManagerName]; exists {
		instances := containerManager.GetInstances()

		if instance, ok := instances[coreInstanceName]; ok {
			containerData = buildContainerDataFromSnapshot(*instance, s.logger)
		} else {
			s.logger.Warn("Core instance not found in container manager",
				zap.String("instanceName", coreInstanceName))
			containerData = buildDefaultContainerData()
		}
	} else {
		s.logger.Warn("Container manager not found in system snapshot",
			zap.String("managerName", containerManagerName))

		containerData = buildDefaultContainerData()
	}

	dfcData, err := s.getDataFlowComponentData()
	if err != nil {
		s.logger.Error("Error getting dataflowcomponent data", zap.Error(err))
	}

	// extract agent data from the agent manager if available
	var agentData models.Agent
	var releaseChannel string

	if agentManager, exists := s.state.Managers[agentManagerName]; exists {
		instances := agentManager.GetInstances()

		s.logger.Debug("Agent manager instances",
			zap.Any("instances", instances))

		if instance, ok := instances[agentInstanceName]; ok {
			agentData, releaseChannel = buildAgentDataFromSnapshot(*instance, s.logger)
		} else {
			s.logger.Warn("Agent instance not found in agent manager",
				zap.String("instanceName", agentInstanceName),
				zap.Any("instances", instances))
			sentry.ReportIssuef(sentry.IssueTypeError, s.logger, "[GenerateStatusMessage] Agent instance not found in agent manager")
			agentData = models.Agent{}
			releaseChannel = "n/a"
		}
	}

	// Create the status message
	statusMessage := &models.StatusMessage{
		Core: models.Core{
			Agent: models.Agent{
				Health:   agentData.Health,
				Latency:  &models.Latency{},
				Location: agentData.Location,
			},
			Container: containerData,
			Dfcs:      dfcData,
			Redpanda: models.Redpanda{
				Health: &models.Health{
					Message:       "redpanda monitoring is not implemented yet",
					ObservedState: "n/a",
					DesiredState:  "running",
					Category:      models.Neutral,
				},
				AvgIncomingThroughputPerMinuteInMsgSec: 0,
				AvgOutgoingThroughputPerMinuteInMsgSec: 0,
			},
			UnifiedNamespace: models.UnifiedNamespace{},
			Release: models.Release{
				Health: &models.Health{
					Message:       "release monitoring is not implemented yet",
					ObservedState: "n/a",
					DesiredState:  "running",
					Category:      models.Neutral,
				},
				Version: "n/a",
				Channel: releaseChannel,
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

// buildContainerDataFromSnapshot creates container data from a FSM instance snapshot
func buildContainerDataFromSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.SugaredLogger) models.Container {
	// Try to get observed state from instance
	containerData := buildDefaultContainerData()

	// Check if we have actual observedState
	if instance.LastObservedState != nil {
		// Try to cast to the right type
		if snapshot, ok := instance.LastObservedState.(*container.ContainerObservedStateSnapshot); ok {
			status := snapshot.ServiceInfoSnapshot

			// Create health status
			containerData.Health = &models.Health{
				Message:       getHealthMessage(status.OverallHealth),
				ObservedState: instance.CurrentState,
				DesiredState:  instance.DesiredState,
				Category:      status.OverallHealth,
			}

			// Fill in CPU metrics
			if status.CPU != nil {
				containerData.CPU = status.CPU
				// Ensure health is set
				if containerData.CPU.Health == nil {
					containerData.CPU.Health = &models.Health{
						Message:       getHealthMessage(status.CPUHealth),
						ObservedState: status.CPUHealth.String(),
						DesiredState:  models.Active.String(),
						Category:      status.CPUHealth,
					}
				}
			}

			// Fill in Memory metrics
			if status.Memory != nil {
				containerData.Memory = status.Memory
				// Ensure health is set
				if containerData.Memory.Health == nil {
					containerData.Memory.Health = &models.Health{
						Message:       getHealthMessage(status.MemoryHealth),
						ObservedState: status.MemoryHealth.String(),
						DesiredState:  models.Active.String(),
						Category:      status.MemoryHealth,
					}
				}
			}

			// Fill in Disk metrics
			if status.Disk != nil {
				containerData.Disk = status.Disk
				// Ensure health is set
				if containerData.Disk.Health == nil {
					containerData.Disk.Health = &models.Health{
						Message:       getHealthMessage(status.DiskHealth),
						ObservedState: status.DiskHealth.String(),
						DesiredState:  models.Active.String(),
						Category:      status.DiskHealth,
					}
				}
			}

			// Set hardware info
			containerData.Hwid = status.Hwid
			containerData.Architecture = status.Architecture
		} else {
			log.Warn("Container observed state is not of expected type")
		}
	} else {
		log.Warn("Container instance has no observed state")
	}

	return containerData
}

func buildDataFlowComponentDataFromSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.SugaredLogger) (models.Dfc, error) {
	dfcData := models.Dfc{}

	// Check if we have actual observedState
	if instance.LastObservedState != nil {
		// Try to cast to the right type

		// Create health status based on instance.CurrentState
		extractHealthStatus := func(state string) models.HealthCategory {
			switch state {
			case dataflowcomponent.OperationalStateActive:
				return models.Active
			case dataflowcomponent.OperationalStateDegraded:
				return models.Degraded
			default:
				return models.Neutral
			}
		}

		// get the metrics from the instance
		observed, ok := instance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
		if !ok {
			err := fmt.Errorf("observed state %T does not match DataflowComponentObservedStateSnapshot", instance.LastObservedState)
			log.Error(err)
			return models.Dfc{}, err
		}
		serviceInfo := observed.ServiceInfo
		inputThroughput := int64(0)
		if serviceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState != nil && serviceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState.Input.LastCount > 0 {
			inputThroughput = int64(serviceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosMetrics.MetricsState.Input.MessagesPerTick / constants.DefaultTickerTime.Seconds())
		}

		dfcData.Health = &models.Health{
			Message:       getHealthMessage(extractHealthStatus(instance.CurrentState)),
			ObservedState: instance.CurrentState,
			DesiredState:  instance.DesiredState,
			Category:      extractHealthStatus(instance.CurrentState),
		}

		dfcData.Type = "custom" // this is a custom DFC; protocol converters will have a separate fsm
		dfcData.UUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID).String()
		dfcData.Metrics = &models.DfcMetrics{
			AvgInputThroughputPerMinuteInMsgSec: float64(inputThroughput),
		}
		dfcData.Name = &instance.ID
	} else {
		log.Warn("no observed state found for dataflowcomponent", zap.String("instanceID", instance.ID))
		return models.Dfc{}, fmt.Errorf("no observed state found for dataflowcomponent")
	}

	return dfcData, nil
}

// getHealthMessage returns an appropriate message based on health category
func getHealthMessage(health models.HealthCategory) string {
	switch health {
	case models.Active:
		return "Component is operating normally"
	case models.Degraded:
		return "Component is operating at reduced capacity"
	case models.Neutral:
		return "Component status is neutral"
	default:
		return "Component status unknown"
	}
}

// buildDefaultContainerData creates default container data when no real data is available
func buildDefaultContainerData() models.Container {
	return models.Container{
		Health: &models.Health{
			Message:       "Container status unknown",
			ObservedState: "unknown",
			DesiredState:  "running",
			Category:      models.Neutral,
		},
		CPU: &models.CPU{
			Health: &models.Health{
				Message:       "CPU status unknown",
				ObservedState: "unknown",
				DesiredState:  "normal",
				Category:      models.Neutral,
			},
			TotalUsageMCpu: 0,
			CoreCount:      0,
		},
		Disk: &models.Disk{
			Health: &models.Health{
				Message:       "Disk status unknown",
				ObservedState: "unknown",
				DesiredState:  "normal",
				Category:      models.Neutral,
			},
			DataPartitionUsedBytes:  0,
			DataPartitionTotalBytes: 0,
		},
		Memory: &models.Memory{
			Health: &models.Health{
				Message:       "Memory status unknown",
				ObservedState: "unknown",
				DesiredState:  "normal",
				Category:      models.Neutral,
			},
			CGroupUsedBytes:  0,
			CGroupTotalBytes: 0,
		},
		Hwid:         "unknown",
		Architecture: models.ArchitectureAmd64,
	}
}

// buildAgentDataFromSnapshot creates agent data from a FSM instance snapshot
func buildAgentDataFromSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.SugaredLogger) (models.Agent, string) {
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
			releaseChannel = snapshot.ServiceInfoSnapshot.Release.Channel
		} else {
			log.Warn("Agent observed state is not of expected type")
			sentry.ReportIssuef(sentry.IssueTypeError, log, "[buildAgentDataFromSnapshot] Agent observed state is not of expected type")
		}
	} else {
		log.Warn("Agent instance has no observed state")
	}

	return agentData, releaseChannel
}
