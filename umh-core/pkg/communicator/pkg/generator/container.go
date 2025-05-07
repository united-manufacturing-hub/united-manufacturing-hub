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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

func containerFromInstanceSnapshot(
	inst *fsm.FSMInstanceSnapshot,
	log *zap.SugaredLogger,
) models.Container {

	if inst == nil {
		return buildDefaultContainerData()
	}
	return buildContainerDataFromInstanceSnapshot(*inst, log)
}

// buildContainerDataFromSnapshot creates container data from a FSM instance snapshot
func buildContainerDataFromInstanceSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.SugaredLogger) models.Container {
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
