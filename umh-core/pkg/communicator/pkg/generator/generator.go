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
	"reflect"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
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
	containerManagerName         = logger.ComponentContainerManager + "_" + constants.DefaultManagerName
	benthosManagerName           = logger.ComponentBenthosManager + "_" + constants.DefaultManagerName
	dataFlowComponentManagerName = logger.ComponentDataFlowComponentManager + constants.DefaultManagerName

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

	// Start with a base mocked status message
	statusMessage := createMockedStatusMessage(s.state.Tick)

	// Update each component with real data if available
	s.updateContainerData(statusMessage)
	s.updateLocationData(statusMessage)
	s.updateDfcData(statusMessage)
	s.updateUnifiedNamespaceData(statusMessage)

	// Return the populated status message
	return statusMessage
}

// updateContainerData updates the status message with real container data if available
func (s *StatusCollectorType) updateContainerData(statusMessage *models.StatusMessage) {
	if containerManager, exists := s.state.Managers[containerManagerName]; exists {
		instances := containerManager.GetInstances()

		if instance, ok := instances[coreInstanceName]; ok {
			statusMessage.Core.Container = buildContainerDataFromSnapshot(instance, s.logger)
			return
		} else {
			s.logger.Warn("Core instance not found in container manager",
				zap.String("instanceName", coreInstanceName))
		}
	} else {
		s.logger.Warn("Container manager not found in system snapshot",
			zap.String("managerName", containerManagerName))
	}

	// Return empty container data if real data isn't available
	statusMessage.Core.Container = models.Container{}
}

// updateLocationData updates the status message with real location data if available
func (s *StatusCollectorType) updateLocationData(statusMessage *models.StatusMessage) {
	if s.configManager != nil {
		// Create a context with a short timeout for getting the config
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		// Get the current config
		cfg, err := s.configManager.GetConfig(ctx, 0)
		if err == nil {
			statusMessage.Core.Agent.Location = cfg.Agent.Location
			return
		} else {
			s.logger.Warn("Failed to get location from config manager", zap.Error(err))
		}
	}

	// Return empty location data if real data isn't available
	statusMessage.Core.Agent.Location = map[int]string{}
}

// updateDfcData updates the status message with real DFC data if available
func (s *StatusCollectorType) updateDfcData(statusMessage *models.StatusMessage) {
	s.logger.Info("Updating DFC data")
	//log all existing managers
	for managerName, manager := range s.state.Managers {
		s.logger.Info("Manager", zap.String("name", managerName), zap.Any("instances", manager.GetInstances()))
	}
	if dfcManager, exists := s.state.Managers[dataFlowComponentManagerName]; exists {
		instances := dfcManager.GetInstances()

		// If we have DFC instances, use them
		if len(instances) > 0 {
			statusMessage.Core.Dfcs = buildDfcsFromInstances(instances, s.logger)
			return
		} else {
			s.logger.Debug("No DFC instances found in DFC manager")
		}
	} else {
		s.logger.Warn("DataFlowComponent manager not found in system snapshot",
			zap.String("managerName", dataFlowComponentManagerName))
	}
	s.logger.Info("No DFC instances found in DFC manager")
	// Return empty array if real data isn't available
	statusMessage.Core.Dfcs = []models.Dfc{}
}

// updateUnifiedNamespaceData updates the status message with real unified namespace data if available
func (s *StatusCollectorType) updateUnifiedNamespaceData(statusMessage *models.StatusMessage) {
	if s.latestData != nil {
		// Add real EventsTable data if available
		if !reflect.DeepEqual(s.latestData.EventTable, models.EventsTable{}) {
			if statusMessage.Core.UnifiedNamespace.EventsTable == nil {
				statusMessage.Core.UnifiedNamespace.EventsTable = make(map[string]models.EventsTable)
			}
			statusMessage.Core.UnifiedNamespace.EventsTable["event1"] = s.latestData.EventTable
		}

		// Add real UnsTable data if available
		if !reflect.DeepEqual(s.latestData.UnsTable, models.UnsTable{}) {
			if statusMessage.Core.UnifiedNamespace.UnsTable == nil {
				statusMessage.Core.UnifiedNamespace.UnsTable = make(map[string]models.UnsTable)
			}
			statusMessage.Core.UnifiedNamespace.UnsTable["uns1"] = s.latestData.UnsTable
		}
	}
}

// createMockedStatusMessage creates a base status message with mock data only for unimplemented components
func createMockedStatusMessage(tick uint64) *models.StatusMessage {
	return &models.StatusMessage{
		Core: models.Core{
			Agent: models.Agent{ // No handler for this yet, so we keep mock data
				Health: &models.Health{
					Message:       fmt.Sprintf("Agent is healthy, tick: %d", tick),
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
				Location: map[int]string{}, // Will be filled by updateLocationData
			},
			Container: models.Container{}, // Will be filled by updateContainerData
			Dfcs:      []models.Dfc{},     // Will be filled by updateDfcData
			Redpanda: models.Redpanda{ // No handler for this yet, so we keep mock data
				Health: &models.Health{
					Message:       "Redpanda is operating normally",
					ObservedState: "running",
					DesiredState:  "running",
					Category:      models.Active,
				},
				AvgIncomingThroughputPerMinuteInMsgSec: 150.5,
				AvgOutgoingThroughputPerMinuteInMsgSec: 120.3,
			},
			UnifiedNamespace: models.UnifiedNamespace{ // Will be filled by updateUnifiedNamespaceData
				EventsTable: map[string]models.EventsTable{},
				UnsTable:    map[string]models.UnsTable{},
			},
			Release: models.Release{ // No handler for this yet, so we keep mock data
				Version: "1.0.0",
				Channel: "stable",
				SupportedFeatures: []string{
					"data-bridge",
					"protocol-converter",
					"custom-dfc",
				},
			},
		},
		Plugins: map[string]interface{}{ // No handler for this yet, so we keep mock data
			"examplePlugin": map[string]interface{}{
				"status":  "active",
				"version": "0.1.0",
			},
		},
	}
}

// buildContainerDataFromSnapshot creates container data from a FSM instance snapshot
func buildContainerDataFromSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.Logger) models.Container {
	// Try to get observed state from instance
	containerData := buildDefaultContainerData()

	// Check if we have actual observedState
	if instance.LastObservedState != nil {
		// Try to cast to the right type
		if snapshot, ok := instance.LastObservedState.(*container.ContainerObservedStateSnapshot); ok {
			status := snapshot.ContainerStatusSnapshot

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

// DFCObservedState is a local definition to avoid importing the dataflowcomponent package
type DFCObservedState interface {
	IsObservedState()
	// Adding the required fields to match the actual DFC observed state structure
	GetConfigExists() bool
	GetLastConfigUpdateSuccessful() bool
	GetLastError() string
}

// buildDfcsFromInstances creates Dfc models from FSM instances
func buildDfcsFromInstances(instances map[string]fsm.FSMInstanceSnapshot, log *zap.Logger) []models.Dfc {
	dfcs := make([]models.Dfc, 0, len(instances))

	for name, instance := range instances {
		log.Debug("Building DFC data for instance", zap.String("name", name))

		// Default DFC structure with basic data from instance
		dfc := models.Dfc{
			Name: stringPtr(name),
			// Use name as UUID as a fallback if we can't extract real UUID
			UUID: name,
			Type: models.DfcTypeCustom, // Default to custom type
			Health: &models.Health{
				Message:       "DFC status available",
				ObservedState: instance.CurrentState,
				DesiredState:  instance.DesiredState,
				Category:      models.Active, // Default to active
			},
			Metrics: &models.DfcMetrics{
				// Default metric with random value
				AvgInputThroughputPerMinuteInMsgSec: float64(rand.Intn(91) + 10),
			},
		}

		// If we have observed state, extract more information
		if instance.LastObservedState != nil {
			// Try to cast to DataFlowComponent's observed state
			if dfcObservedState, ok := instance.LastObservedState.(DFCObservedState); ok {
				// Set health message based on observed state
				if !dfcObservedState.GetLastConfigUpdateSuccessful() {
					dfc.Health.Message = "DFC configuration update failed: " + dfcObservedState.GetLastError()
					dfc.Health.Category = models.Degraded
				}
			}

			// Try to access any additional metadata that might be present in the instance
			// This is a simplified approach since FSMInstanceSnapshot doesn't have a Meta field
			// Try to extract information from the observed state directly or use defaults

			// For a real implementation, you might need to access this data differently
			// or store it in the observed state

			// Default to data bridge type for now since we can't access detailed metadata
			dfc.Type = models.DfcTypeDataBridge

			// Create simple bridge info with default values
			dfc.Bridge = &models.DfcBridgeInfo{
				DataContract: "unknown",
				InputType:    "unknown",
				OutputType:   "unknown",
			}

			// Add standard input/output connections for a bridge
			dfc.Connections = []models.Connection{
				{
					Name: "Input Connection",
					UUID: name + "-input",
					Health: &models.Health{
						Message:       "Connection status derived from DFC",
						ObservedState: instance.CurrentState,
						DesiredState:  instance.DesiredState,
						Category:      dfc.Health.Category,
					},
					URI:           "unknown://input",
					LastLatencyMs: 5.0,
				},
				{
					Name: "Output Connection",
					UUID: name + "-output",
					Health: &models.Health{
						Message:       "Connection status derived from DFC",
						ObservedState: instance.CurrentState,
						DesiredState:  instance.DesiredState,
						Category:      dfc.Health.Category,
					},
					URI:           "unknown://output",
					LastLatencyMs: 5.0,
				},
			}
		}

		dfcs = append(dfcs, dfc)
	}

	return dfcs
}

// getStringValueOrDefault gets a string value from a map or returns the default
func getStringValueOrDefault(config map[string]interface{}, key, defaultValue string) string {
	if val, ok := config[key].(string); ok && val != "" {
		return val
	}
	return defaultValue
}
