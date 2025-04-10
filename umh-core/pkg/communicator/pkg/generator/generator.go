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
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

const (
	// Manager name constants
	containerManagerName = logger.ComponentContainerManager + "_" + constants.DefaultManagerName
	benthosManagerName   = logger.ComponentBenthosManager + "_" + constants.DefaultManagerName

	// Instance name constants
	coreInstanceName = "Core"
)

type StatusCollectorType struct {
	latestData    *LatestData
	dog           watchdog.Iface
	state         *fsm.SystemSnapshot
	systemMu      *sync.Mutex
	logger        *zap.Logger
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
) *StatusCollectorType {

	logger := logger.New("generator.NewStatusCollector", logger.FormatJSON)

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

func (s *StatusCollectorType) GenerateStatusMessage() *models.StatusMessage {
	s.latestData.mu.RLock()
	defer s.latestData.mu.RUnlock()

	// Lock state for reading and hold it until we're done accessing state data
	s.systemMu.Lock()
	defer s.systemMu.Unlock()

	if s.state == nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.logger.Sugar(), "[GenerateStatusMessage] State is nil, using empty state")
		s.logger.Error("State is nil, using empty state")
		return nil
	}

	// Create container data from the container manager if available
	var containerData models.Container

	if containerManager, exists := s.state.Managers[containerManagerName]; exists {
		instances := containerManager.GetInstances()

		if instance, ok := instances[coreInstanceName]; ok {
			containerData = buildContainerDataFromSnapshot(instance, s.logger)
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

	// Get the actual location from the system configuration
	location := map[int]string{}

	// Try to get the location from the config manager first
	if s.configManager != nil {
		// Create a context with a short timeout for getting the config
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		// Get the current config
		cfg, err := s.configManager.GetConfig(ctx, 0)
		if err == nil {
			location = cfg.Agent.Location
		} else {
			s.logger.Warn("Failed to get location from config manager", zap.Error(err))
		}
	}

	// Create a status message with real data
	statusMessage := &models.StatusMessage{
		Core: models.Core{
			Agent: models.Agent{
				Health: &models.Health{
					Message:       fmt.Sprintf("Agent is healthy, tick: %d", s.state.Tick),
					ObservedState: "running",
					DesiredState:  "running",
					Category:      models.Active,
				},
				Latency: &models.Latency{
					AvgMs: 10.5,
					MaxMs: 25.0,
					MinMs: 5.0,
					P95Ms: float64(rand.Intn(91) + 10),
					P99Ms: 22.8,
				},
				Location: location, // Use the actual location from config
			},
			Container: containerData,
			Dfcs:      getDfcsFromConfig(s.configManager, s.logger),
			Redpanda: models.Redpanda{
				Health: &models.Health{
					Message:       "Redpanda is operating normally",
					ObservedState: "running",
					DesiredState:  "running",
					Category:      models.Active,
				},
				AvgIncomingThroughputPerMinuteInMsgSec: 150.5,
				AvgOutgoingThroughputPerMinuteInMsgSec: 120.3,
			},
			UnifiedNamespace: models.UnifiedNamespace{
				EventsTable: map[string]models.EventsTable{
					"event1": s.latestData.EventTable,
				},
				UnsTable: map[string]models.UnsTable{
					"uns1": s.latestData.UnsTable,
				},
			},
			Release: models.Release{
				Version: "1.0.0",
				Channel: "stable",
				SupportedFeatures: []string{
					"data-bridge",
					"protocol-converter",
					"custom-dfc",
					"action-deploy-data-flow-component",
					"action-get-data-flow-component",
				},
			},
		},
		Plugins: map[string]interface{}{
			"examplePlugin": map[string]interface{}{
				"status":  "active",
				"version": "0.1.0",
			},
		},
	}

	return statusMessage
}

// buildContainerDataFromSnapshot creates container data from a FSM instance snapshot
func buildContainerDataFromSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.Logger) models.Container {
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

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// getDfcsFromConfig retrieves the Data Flow Components from the configuration
func getDfcsFromConfig(configManager config.ConfigManager, logger *zap.Logger) []models.Dfc {
	if configManager == nil {
		logger.Warn("Config manager is nil, cannot retrieve DFCs")
		return []models.Dfc{}
	}

	// Create a context with a short timeout for getting the config
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Get the current config
	cfg, err := configManager.GetConfig(ctx, 0)
	if err != nil {
		logger.Warn("Failed to get config for DFCs", zap.Error(err))
		return []models.Dfc{}
	}

	dfcs := make([]models.Dfc, 0, len(cfg.DataFlow))

	// Convert each DataFlowComponentConfig to a Dfc model
	for _, component := range cfg.DataFlow {
		// Generate UUID from name
		dfcUUID := dataflowcomponentconfig.GenerateUUIDFromName(component.FSMInstanceConfig.Name).String()

		dfc := models.Dfc{
			Name: stringPtr(component.FSMInstanceConfig.Name),
			UUID: dfcUUID,
			Health: &models.Health{
				Message:       "DFC is operating normally", // Mocked as requested
				ObservedState: "running",                   // Mocked as requested
				DesiredState:  component.FSMInstanceConfig.DesiredFSMState,
				Category:      models.Active,
			},
			Metrics: &models.DfcMetrics{
				AvgInputThroughputPerMinuteInMsgSec: float64(rand.Intn(91) + 10), // Keeping some randomized metrics
			},
		}

		// Process the DataFlowComponentConfig to determine type
		// Default to custom type
		dfc.Type = models.DfcTypeCustom

		dfcs = append(dfcs, dfc)
	}

	return dfcs
}
