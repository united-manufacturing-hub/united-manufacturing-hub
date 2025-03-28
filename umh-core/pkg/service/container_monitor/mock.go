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

package container_monitor

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// MockService is a mock implementation of the container monitor Service interface
type MockService struct {
	mock.Mock
}

// NewMockService creates a new mock service instance
func NewMockService() *MockService {
	return &MockService{}
}

// GetStatus is a mock implementation of Service.GetStatus
func (m *MockService) GetStatus(ctx context.Context) (*ContainerMetrics, error) {
	args := m.Called(ctx)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*ContainerMetrics), args.Error(1)
}

// GetHealth is a mock implementation of Service.GetHealth
func (m *MockService) GetHealth(ctx context.Context) (*models.Health, *models.CPU, *models.Disk, *models.Memory, error) {
	args := m.Called(ctx)

	var health *models.Health
	var cpu *models.CPU
	var disk *models.Disk
	var memory *models.Memory

	if args.Get(0) != nil {
		health = args.Get(0).(*models.Health)
	}
	if args.Get(1) != nil {
		cpu = args.Get(1).(*models.CPU)
	}
	if args.Get(2) != nil {
		disk = args.Get(2).(*models.Disk)
	}
	if args.Get(3) != nil {
		memory = args.Get(3).(*models.Memory)
	}

	return health, cpu, disk, memory, args.Error(4)
}

// CreateDefaultMockMetrics returns a default set of container metrics for testing
func CreateDefaultMockMetrics() *ContainerMetrics {
	return &ContainerMetrics{
		CPU: &CPUMetrics{
			TotalUsageMCpu: 350.0,
			CoreCount:      4,
			LoadPercent:    50.0,
		},
		Memory: &MemoryMetrics{
			CGroupUsedBytes:  1073741824, // 1 GB
			CGroupTotalBytes: 4294967296, // 4 GB
		},
		Disk: &DiskMetrics{
			DataPartitionUsedBytes:  536870912,   // 512 MB
			DataPartitionTotalBytes: 10737418240, // 10 GB
		},
		HWID:         "hwid-test-12345",
		Architecture: models.ArchitectureAmd64,
		CollectedAt:  time.Now(),
	}
}

// CreateCriticalMockMetrics creates metrics that will trigger critical health status
func CreateCriticalMockMetrics() *ContainerMetrics {
	return &ContainerMetrics{
		CPU: &CPUMetrics{
			TotalUsageMCpu: 950.0,
			CoreCount:      4,
			LoadPercent:    95.0, // Critical CPU load
		},
		Memory: &MemoryMetrics{
			CGroupUsedBytes:  4000000000, // Almost full memory
			CGroupTotalBytes: 4294967296, // 4 GB
		},
		Disk: &DiskMetrics{
			DataPartitionUsedBytes:  9800000000,  // Almost full disk
			DataPartitionTotalBytes: 10737418240, // 10 GB
		},
		HWID:         "hwid-test-12345",
		Architecture: models.ArchitectureAmd64,
		CollectedAt:  time.Now(),
	}
}

// CreateDefaultMockHealth creates default health objects for testing
func CreateDefaultMockHealth() (*models.Health, *models.CPU, *models.Disk, *models.Memory) {
	containerHealth := &models.Health{
		Message:       "Container is operating normally",
		ObservedState: constants.ContainerStateRunning,
		DesiredState:  constants.ContainerStateRunning,
		Category:      models.Active,
	}

	cpuHealth := &models.Health{
		Message:       "CPU utilization normal",
		ObservedState: constants.ContainerStateNormal,
		DesiredState:  constants.ContainerStateNormal,
		Category:      models.Active,
	}

	diskHealth := &models.Health{
		Message:       "Disk utilization normal",
		ObservedState: constants.ContainerStateNormal,
		DesiredState:  constants.ContainerStateNormal,
		Category:      models.Active,
	}

	memoryHealth := &models.Health{
		Message:       "Memory utilization normal",
		ObservedState: constants.ContainerStateNormal,
		DesiredState:  constants.ContainerStateNormal,
		Category:      models.Active,
	}

	// Create component models
	cpu := &models.CPU{
		Health:         cpuHealth,
		TotalUsageMCpu: 350.0,
		CoreCount:      4,
	}

	disk := &models.Disk{
		Health:                  diskHealth,
		DataPartitionUsedBytes:  536870912,
		DataPartitionTotalBytes: 10737418240,
	}

	memory := &models.Memory{
		Health:           memoryHealth,
		CGroupUsedBytes:  1073741824,
		CGroupTotalBytes: 4294967296,
	}

	return containerHealth, cpu, disk, memory
}

// SetupMockForHealthyState configures the mock to return healthy metrics and status
func (m *MockService) SetupMockForHealthyState() {
	metrics := CreateDefaultMockMetrics()
	containerHealth, cpu, disk, memory := CreateDefaultMockHealth()

	m.On("GetStatus", mock.Anything).Return(metrics, nil)
	m.On("GetHealth", mock.Anything).Return(containerHealth, cpu, disk, memory, nil)
}

// SetupMockForCriticalState configures the mock to return critical metrics and status
func (m *MockService) SetupMockForCriticalState() {
	metrics := CreateCriticalMockMetrics()

	// Create critical health
	containerHealth := &models.Health{
		Message:       "Container CPU utilization critical",
		ObservedState: constants.ContainerStateRunning,
		DesiredState:  constants.ContainerStateRunning,
		Category:      models.Degraded,
	}

	cpuHealth := &models.Health{
		Message:       "CPU utilization critical",
		ObservedState: constants.ContainerStateCritical,
		DesiredState:  constants.ContainerStateNormal,
		Category:      models.Degraded,
	}

	diskHealth := &models.Health{
		Message:       "Disk utilization critical",
		ObservedState: constants.ContainerStateCritical,
		DesiredState:  constants.ContainerStateNormal,
		Category:      models.Degraded,
	}

	memoryHealth := &models.Health{
		Message:       "Memory utilization critical",
		ObservedState: constants.ContainerStateCritical,
		DesiredState:  constants.ContainerStateNormal,
		Category:      models.Degraded,
	}

	// Create component models
	cpu := &models.CPU{
		Health:         cpuHealth,
		TotalUsageMCpu: metrics.CPU.TotalUsageMCpu,
		CoreCount:      metrics.CPU.CoreCount,
	}

	disk := &models.Disk{
		Health:                  diskHealth,
		DataPartitionUsedBytes:  metrics.Disk.DataPartitionUsedBytes,
		DataPartitionTotalBytes: metrics.Disk.DataPartitionTotalBytes,
	}

	memory := &models.Memory{
		Health:           memoryHealth,
		CGroupUsedBytes:  metrics.Memory.CGroupUsedBytes,
		CGroupTotalBytes: metrics.Memory.CGroupTotalBytes,
	}

	m.On("GetStatus", mock.Anything).Return(metrics, nil)
	m.On("GetHealth", mock.Anything).Return(containerHealth, cpu, disk, memory, nil)
}

// SetupMockForError configures the mock to return errors
func (m *MockService) SetupMockForError(err error) {
	m.On("GetStatus", mock.Anything).Return(nil, err)
	m.On("GetHealth", mock.Anything).Return(nil, nil, nil, nil, err)
}
