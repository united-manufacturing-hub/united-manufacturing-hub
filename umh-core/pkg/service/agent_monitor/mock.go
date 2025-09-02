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

package agent_monitor

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6/s6_default"
)

// MockService is a mock implementation of the agent monitor Service interface.
type MockService struct {

	// Return values for each method
	GetStatusError error

	fs filesystem.Service

	// Results for each method
	GetStatusResult *ServiceInfo

	// For more complex testing scenarios
	healthState models.HealthCategory
	// Tracks calls to methods
	GetStatusCalled bool
}

// NewMockService creates a new mock service instance.
func NewMockService(fs filesystem.Service) *MockService {
	return &MockService{
		// Default to Active health
		healthState: models.Active,
		// Initialize with default healthy status
		GetStatusResult: CreateDefaultAgentStatus(),
		fs:              fs,
	}
}

// Status is a mock implementation of Service.Status.
func (m *MockService) Status(ctx context.Context, snapshot fsm.SystemSnapshot) (*ServiceInfo, error) {
	m.GetStatusCalled = true

	if m.GetStatusError != nil {
		return nil, m.GetStatusError
	}

	// Return the configured result
	return m.GetStatusResult, nil
}

// CreateDefaultAgentStatus returns a default agent status with healthy metrics for testing.
func CreateDefaultAgentStatus() *ServiceInfo {
	return &ServiceInfo{
		Location: map[int]string{
			1: "Plant",
			2: "Area",
			3: "Line",
		},
		Latency: &models.Latency{},
		AgentLogs: []s6_shared.LogEntry{
			{
				Timestamp: time.Now().Add(-10 * time.Minute),
				Content:   "INFO: Test log entry 1",
			},
			{
				Timestamp: time.Now().Add(-5 * time.Minute),
				Content:   "WARN: Test log entry 2",
			},
		},
		AgentMetrics: map[string]interface{}{
			"metric1": 100,
			"metric2": "value",
		},
		Release: &models.Release{
			Channel:  "stable",
			Version:  "1.0.0",
			Versions: []models.Version{{Name: "component1", Version: "1.0.0"}},
		},
		OverallHealth: models.Active,
		LatencyHealth: models.Active,
		ReleaseHealth: models.Active,
	}
}

// CreateDegradedAgentStatus creates an agent status with degraded health.
func CreateDegradedAgentStatus() *ServiceInfo {
	status := CreateDefaultAgentStatus()
	status.OverallHealth = models.Degraded
	status.LatencyHealth = models.Degraded

	return status
}

// SetupMockForHealthyState configures the mock to return healthy agent status.
func (m *MockService) SetupMockForHealthyState() {
	m.GetStatusResult = CreateDefaultAgentStatus()
	m.healthState = models.Active
}

// SetupMockForDegradedState configures the mock to return degraded agent status.
func (m *MockService) SetupMockForDegradedState() {
	m.GetStatusResult = CreateDegradedAgentStatus()
	m.healthState = models.Degraded
}

// SetupMockForError configures the mock to return errors.
func (m *MockService) SetupMockForError(err error) {
	m.GetStatusError = err
}

func (m *MockService) GetFilesystemService() filesystem.Service {
	return m.fs
}
