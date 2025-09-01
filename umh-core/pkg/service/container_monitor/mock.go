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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// MockService is a mock implementation of the container monitor Service interface.
type MockService struct {

	// Return values for each method
	GetStatusError error
	GetHealthError error

	// Results for each method
	GetStatusResult *ServiceInfo
	GetHealthResult *models.Health

	// For more complex testing scenarios
	healthState models.HealthCategory
	// Tracks calls to methods
	GetStatusCalled bool
	GetHealthCalled bool
}

// NewMockService creates a new mock service instance.
func NewMockService() *MockService {
	return &MockService{
		// Default to Active health
		healthState: models.Active,
		// Initialize with default healthy status
		GetStatusResult: CreateDefaultContainerStatus(),
		GetHealthResult: &models.Health{
			Message:       "Container is operating normally",
			ObservedState: models.Active.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Active,
		},
	}
}

// GetStatus is a mock implementation of Service.GetStatus.
func (m *MockService) GetStatus(ctx context.Context) (*ServiceInfo, error) {
	m.GetStatusCalled = true

	if m.GetStatusError != nil {
		return nil, m.GetStatusError
	}

	// Return the configured result
	return m.GetStatusResult, nil
}

// GetHealth is a mock implementation of Service.GetHealth.
func (m *MockService) GetHealth(ctx context.Context) (*models.Health, error) {
	m.GetHealthCalled = true

	if m.GetHealthError != nil {
		return nil, m.GetHealthError
	}

	// Return the configured result
	return m.GetHealthResult, nil
}

// CreateDefaultContainerStatus returns a default container status with healthy metrics for testing.
func CreateDefaultContainerStatus() *ServiceInfo {
	return &ServiceInfo{
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

// CreateDegradedContainerStatus creates a container status with degraded health.
func CreateDegradedContainerStatus() *ServiceInfo {
	return &ServiceInfo{
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

// SetupMockForHealthyState configures the mock to return healthy container status.
func (m *MockService) SetupMockForHealthyState() {
	m.GetStatusResult = CreateDefaultContainerStatus()
	m.healthState = models.Active

	m.GetHealthResult = &models.Health{
		Message:       "Container is operating normally",
		ObservedState: models.Active.String(),
		DesiredState:  models.Active.String(),
		Category:      models.Active,
	}
}

// SetupMockForDegradedState configures the mock to return degraded container status.
func (m *MockService) SetupMockForDegradedState() {
	m.GetStatusResult = CreateDegradedContainerStatus()
	m.healthState = models.Degraded

	m.GetHealthResult = &models.Health{
		Message:       "CPU metrics degraded",
		ObservedState: models.Degraded.String(),
		DesiredState:  models.Active.String(),
		Category:      models.Degraded,
	}
}

// SetupMockForCriticalState is deprecated, use SetupMockForDegradedState instead
// This is kept for backward compatibility with existing tests.
func (m *MockService) SetupMockForCriticalState() {
	m.SetupMockForDegradedState()
}

// SetupMockForError configures the mock to return errors.
func (m *MockService) SetupMockForError(err error) {
	m.GetStatusError = err
	m.GetHealthError = err
}
