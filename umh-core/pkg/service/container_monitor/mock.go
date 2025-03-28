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
func (m *MockService) GetStatus(ctx context.Context) (*models.Container, error) {
	args := m.Called(ctx)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*models.Container), args.Error(1)
}

// GetHealth is a mock implementation of Service.GetHealth
func (m *MockService) GetHealth(ctx context.Context) (*models.Health, error) {
	args := m.Called(ctx)

	var health *models.Health

	if args.Get(0) != nil {
		health = args.Get(0).(*models.Health)
	}

	return health, args.Error(1)
}

// CreateDefaultMockContainer returns a default container with metrics for testing
func CreateDefaultMockContainer() *models.Container {
	return &models.Container{
		Health: &models.Health{
			Message:       "Container is operating normally",
			ObservedState: constants.ContainerStateRunning,
			DesiredState:  constants.ContainerStateRunning,
			Category:      models.Active,
		},
		CPU: &models.CPU{
			Health: &models.Health{
				Message:       "CPU utilization normal",
				ObservedState: constants.ContainerStateNormal,
				DesiredState:  constants.ContainerStateNormal,
				Category:      models.Active,
			},
			TotalUsageMCpu: 350.0,
			CoreCount:      4,
		},
		Memory: &models.Memory{
			Health: &models.Health{
				Message:       "Memory utilization normal",
				ObservedState: constants.ContainerStateNormal,
				DesiredState:  constants.ContainerStateNormal,
				Category:      models.Active,
			},
			CGroupUsedBytes:  1073741824, // 1 GB
			CGroupTotalBytes: 4294967296, // 4 GB
		},
		Disk: &models.Disk{
			Health: &models.Health{
				Message:       "Disk utilization normal",
				ObservedState: constants.ContainerStateNormal,
				DesiredState:  constants.ContainerStateNormal,
				Category:      models.Active,
			},
			DataPartitionUsedBytes:  536870912,   // 512 MB
			DataPartitionTotalBytes: 10737418240, // 10 GB
		},
		Hwid:         "hwid-test-12345",
		Architecture: models.ArchitectureAmd64,
	}
}

// CreateCriticalMockContainer creates a container with critical health status
func CreateCriticalMockContainer() *models.Container {
	return &models.Container{
		Health: &models.Health{
			Message:       "Container CPU utilization critical",
			ObservedState: constants.ContainerStateRunning,
			DesiredState:  constants.ContainerStateRunning,
			Category:      models.Degraded,
		},
		CPU: &models.CPU{
			Health: &models.Health{
				Message:       "CPU utilization critical",
				ObservedState: constants.ContainerStateCritical,
				DesiredState:  constants.ContainerStateNormal,
				Category:      models.Degraded,
			},
			TotalUsageMCpu: 950.0,
			CoreCount:      4,
		},
		Memory: &models.Memory{
			Health: &models.Health{
				Message:       "Memory utilization critical",
				ObservedState: constants.ContainerStateCritical,
				DesiredState:  constants.ContainerStateNormal,
				Category:      models.Degraded,
			},
			CGroupUsedBytes:  4000000000, // Almost full memory
			CGroupTotalBytes: 4294967296, // 4 GB
		},
		Disk: &models.Disk{
			Health: &models.Health{
				Message:       "Disk utilization critical",
				ObservedState: constants.ContainerStateCritical,
				DesiredState:  constants.ContainerStateNormal,
				Category:      models.Degraded,
			},
			DataPartitionUsedBytes:  9800000000,  // Almost full disk
			DataPartitionTotalBytes: 10737418240, // 10 GB
		},
		Hwid:         "hwid-test-12345",
		Architecture: models.ArchitectureAmd64,
	}
}

// SetupMockForHealthyState configures the mock to return healthy container status
func (m *MockService) SetupMockForHealthyState() {
	container := CreateDefaultMockContainer()

	m.On("GetStatus", mock.Anything).Return(container, nil)
	m.On("GetHealth", mock.Anything).Return(container.Health, nil)
}

// SetupMockForCriticalState configures the mock to return critical container status
func (m *MockService) SetupMockForCriticalState() {
	container := CreateCriticalMockContainer()

	m.On("GetStatus", mock.Anything).Return(container, nil)
	m.On("GetHealth", mock.Anything).Return(container.Health, nil)
}

// SetupMockForError configures the mock to return errors
func (m *MockService) SetupMockForError(err error) {
	m.On("GetStatus", mock.Anything).Return(nil, err)
	m.On("GetHealth", mock.Anything).Return(nil, err)
}
