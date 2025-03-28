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
func (m *MockService) GetStatus(ctx context.Context) (*ContainerStatus, error) {
	args := m.Called(ctx)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).(*ContainerStatus), args.Error(1)
}

// CreateDefaultContainerStatus returns a default container status with healthy metrics for testing
func CreateDefaultContainerStatus() *ContainerStatus {
	return &ContainerStatus{
		CPU: &models.CPU{
			TotalUsageMCpu: 350.0,
			CoreCount:      4,
		},
		Memory: &models.Memory{
			CGroupUsedBytes:  1073741824, // 1 GB
			CGroupTotalBytes: 4294967296, // 4 GB
		},
		Disk: &models.Disk{
			DataPartitionUsedBytes:  536870912,   // 512 MB
			DataPartitionTotalBytes: 10737418240, // 10 GB
		},
		OverallHealth: models.Active,
		CPUHealth:     models.Active,
		MemoryHealth:  models.Active,
		DiskHealth:    models.Active,
		Hwid:          "hwid-test-12345",
		Architecture:  models.ArchitectureAmd64,
	}
}

// CreateDegradedContainerStatus creates a container status with degraded health
func CreateDegradedContainerStatus() *ContainerStatus {
	return &ContainerStatus{
		CPU: &models.CPU{
			TotalUsageMCpu: 950.0,
			CoreCount:      4,
		},
		Memory: &models.Memory{
			CGroupUsedBytes:  4000000000, // Almost full memory
			CGroupTotalBytes: 4294967296, // 4 GB
		},
		Disk: &models.Disk{
			DataPartitionUsedBytes:  9800000000,  // Almost full disk
			DataPartitionTotalBytes: 10737418240, // 10 GB
		},
		OverallHealth: models.Degraded,
		CPUHealth:     models.Degraded,
		MemoryHealth:  models.Active,
		DiskHealth:    models.Active,
		Hwid:          "hwid-test-12345",
		Architecture:  models.ArchitectureAmd64,
	}
}

// SetupMockForHealthyState configures the mock to return healthy container status
func (m *MockService) SetupMockForHealthyState() {
	status := CreateDefaultContainerStatus()
	m.On("GetStatus", mock.Anything).Return(status, nil)
}

// SetupMockForDegradedState configures the mock to return degraded container status
func (m *MockService) SetupMockForDegradedState() {
	status := CreateDegradedContainerStatus()
	m.On("GetStatus", mock.Anything).Return(status, nil)
}

// SetupMockForCriticalState configures the mock to return degraded container status (for backward compatibility)
func (m *MockService) SetupMockForCriticalState() {
	status := CreateDegradedContainerStatus()
	m.On("GetStatus", mock.Anything).Return(status, nil)
}

// SetupMockForError configures the mock to return errors
func (m *MockService) SetupMockForError(err error) {
	m.On("GetStatus", mock.Anything).Return(nil, err)
}
