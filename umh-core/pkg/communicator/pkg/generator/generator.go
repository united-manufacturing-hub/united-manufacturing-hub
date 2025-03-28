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
	"math/rand"
	"sync"
	"time"

	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
)

const (
	defaultCacheExpiration = 1 * time.Hour
	defaultCacheCullPeriod = 10 * time.Minute
)

type StatusCollectorType struct {
	latestData *LatestData
	dog        watchdog.Iface
	state      *fsm.SystemSnapshot
	systemMu   *sync.Mutex
	logger     *zap.Logger
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
) *StatusCollectorType {

	logger := logger.New("generator.NewStatusCollector", logger.FormatJSON)

	latestData := &LatestData{}

	collector := &StatusCollectorType{
		latestData: latestData,
		dog:        dog,
		state:      state,
		systemMu:   systemMu,
		logger:     logger,
	}

	return collector
}

func (s *StatusCollectorType) GenerateStatusMessage() *models.StatusMessage {
	s.latestData.mu.RLock()
	defer s.latestData.mu.RUnlock()

	// Lock state for reading
	s.systemMu.Lock()
	var state *fsm.SystemSnapshot
	err := deepcopy.Copy(&state, s.state)
	if err != nil {
		sentry.ReportIssuef(sentry.IssueTypeError, s.logger.Sugar(), "Failed to deepcopy state: %s", err)
		return nil
	}
	s.systemMu.Unlock()

	// Save the state in a map
	managersMap := make(map[string]map[string]fsm.FSMInstanceSnapshot)
	if state != nil {
		for managerName, manager := range state.Managers {
			instancesMap := make(map[string]fsm.FSMInstanceSnapshot)
			instances := manager.GetInstances()

			for instanceName, instance := range instances {
				instancesMap[instanceName] = instance
			}

			managersMap[managerName] = instancesMap
		}
	}

	// Create a mocked status message
	statusMessage := &models.StatusMessage{
		Core: models.Core{
			Agent: models.Agent{
				Health: &models.Health{
					Message:       fmt.Sprintf("Agent is healthy, tick: %d", state.Tick),
					ObservedState: managersMap["BenthosManagerCore"]["hello-world"].CurrentState,
					DesiredState:  managersMap["BenthosManagerCore"]["hello-world"].DesiredState,
					Category:      models.Active,
				},
				Latency: &models.Latency{
					AvgMs: 10.5,
					MaxMs: 25.0,
					MinMs: 5.0,
					P95Ms: float64(rand.Intn(91) + 10),
					P99Ms: 22.8,
				},
				Location: map[int]string{
					0: "Manufacturing Inc.",
					1: "Berlin Factory",
					2: "Assembly Line 3",
				},
			},
			Container: models.Container{
				Health: &models.Health{
					Message:       "Container is operating normally",
					ObservedState: "running",
					DesiredState:  "running",
					Category:      models.Active,
				},
				CPU: &models.CPU{
					Health: &models.Health{
						Message:       "CPU utilization normal",
						ObservedState: "normal",
						DesiredState:  "normal",
						Category:      models.Active,
					},
					TotalUsageMCpu: 350.0,
					CoreCount:      4,
				},
				Disk: &models.Disk{
					Health: &models.Health{
						Message:       "Disk utilization normal",
						ObservedState: "normal",
						DesiredState:  "normal",
						Category:      models.Active,
					},
					DataPartitionUsedBytes:  536870912,   // 512 MB
					DataPartitionTotalBytes: 10737418240, // 10 GB
				},
				Memory: &models.Memory{
					Health: &models.Health{
						Message:       "Memory utilization normal",
						ObservedState: "normal",
						DesiredState:  "normal",
						Category:      models.Active,
					},
					CGroupUsedBytes:  1073741824, // 1 GB
					CGroupTotalBytes: 4294967296, // 4 GB
				},
				Hwid:         "hwid-12345",
				Architecture: models.ArchitectureAmd64,
			},
			Dfcs: []models.Dfc{
				{
					Name: stringPtr("Data Bridge 1"),
					UUID: "dfc-uuid-12345",
					Type: models.DfcTypeDataBridge,
					Health: &models.Health{
						Message:       "DFC is operating normally",
						ObservedState: "running",
						DesiredState:  "running",
						Category:      models.Active,
					},
					Metrics: &models.DfcMetrics{
						AvgInputThroughputPerMinuteInMsgSec: float64(rand.Intn(91) + 10),
					},
					Bridge: &models.DfcBridgeInfo{
						DataContract: "sensor-v1",
						InputType:    "mqtt",
						OutputType:   "kafka",
					},
					Connections: []models.Connection{
						{
							Name: "MQTT Input",
							UUID: "conn-uuid-1",
							Health: &models.Health{
								Message:       "Connection is active",
								ObservedState: "connected",
								DesiredState:  "connected",
								Category:      models.Active,
							},
							URI:           "mqtt://broker:1883",
							LastLatencyMs: 5.2,
						},
						{
							Name: "Kafka Output",
							UUID: "conn-uuid-2",
							Health: &models.Health{
								Message:       "Connection is active",
								ObservedState: "connected",
								DesiredState:  "connected",
								Category:      models.Active,
							},
							URI:           "kafka://kafka:9092",
							LastLatencyMs: 8.1,
						},
					},
				},
			},
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

	// Check if we have state information to use (replace with actual logic based on state)
	if state != nil {
		// Here you would integrate information from the state into the status message
		// For now, we'll just leave the mock data as is
	}

	return statusMessage
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
