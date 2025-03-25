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

package nmap

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
)

// MockNmapService is a mock implementation of the INmapService interface for testing
type MockNmapService struct {
	// Tracks calls to methods
	GenerateS6ConfigForNmapCalled bool
	GetConfigCalled               bool
	StatusCalled                  bool
	AddNmapToS6ManagerCalled      bool
	UpdateNmapInS6ManagerCalled   bool
	RemoveNmapFromS6ManagerCalled bool
	StartNmapCalled               bool
	StopNmapCalled                bool
	ServiceExistsCalled           bool
	ReconcileManagerCalled        bool

	// Return values for each method
	GenerateS6ConfigForNmapResult config.S6ServiceConfig
	GenerateS6ConfigForNmapError  error
	GetConfigResult               config.NmapServiceConfig
	GetConfigError                error
	StatusResult                  ServiceInfo
	StatusError                   error
	AddNmapToS6ManagerError       error
	UpdateNmapInS6ManagerError    error
	RemoveNmapFromS6ManagerError  error
	StartNmapError                error
	StopNmapError                 error
	ServiceExistsResult           bool
	ReconcileManagerError         error
	ReconcileManagerReconciled    bool

	// For more complex testing scenarios
	ServiceStates    map[string]*ServiceInfo
	ExistingServices map[string]bool
	S6ServiceConfigs []config.S6FSMConfig
}

// Ensure MockNmapService implements INmapService
var _ INmapService = (*MockNmapService)(nil)

// NewMockNmapService creates a new mock Nmap service
func NewMockNmapService() *MockNmapService {
	return &MockNmapService{
		ServiceStates:    make(map[string]*ServiceInfo),
		ExistingServices: make(map[string]bool),
		S6ServiceConfigs: make([]config.S6FSMConfig, 0),
	}
}

// GenerateS6ConfigForNmap mocks generating S6 config for Nmap
func (m *MockNmapService) GenerateS6ConfigForNmap(nmapConfig *config.NmapServiceConfig, s6ServiceName string) (config.S6ServiceConfig, error) {
	m.GenerateS6ConfigForNmapCalled = true
	return m.GenerateS6ConfigForNmapResult, m.GenerateS6ConfigForNmapError
}

// GetConfig mocks getting the Nmap configuration
func (m *MockNmapService) GetConfig(ctx context.Context, serviceName string) (config.NmapServiceConfig, error) {
	m.GetConfigCalled = true

	// If error is set, return it
	if m.GetConfigError != nil {
		return config.NmapServiceConfig{}, m.GetConfigError
	}

	// If a result is preset, return it
	if m.GetConfigResult.Target != "" || m.GetConfigResult.Port != 0 {
		return m.GetConfigResult, nil
	}

	// Otherwise return a default config with some test values
	return config.NmapServiceConfig{
		Target: "example.com",
		Port:   443,
	}, nil
}

// Status mocks getting the status of a Nmap service
func (m *MockNmapService) Status(ctx context.Context, serviceName string, tick uint64) (ServiceInfo, error) {
	m.StatusCalled = true

	// Check if the service exists in the ExistingServices map
	if exists, ok := m.ExistingServices[serviceName]; !ok || !exists {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// If we have a state already stored, return it
	if state, exists := m.ServiceStates[serviceName]; exists {
		return *state, m.StatusError
	}

	// If no state is stored, return the default mock result
	return m.StatusResult, m.StatusError
}

// AddNmapToS6Manager mocks adding a Nmap instance to the S6 manager
func (m *MockNmapService) AddNmapToS6Manager(ctx context.Context, cfg *config.NmapServiceConfig, serviceName string) error {
	m.AddNmapToS6ManagerCalled = true

	// Check whether the service already exists
	for _, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == serviceName {
			return ErrServiceAlreadyExists
		}
	}

	// Add the service to the list of existing services
	m.ExistingServices[serviceName] = true

	// Create an S6FSMConfig for this service
	s6FSMConfig := config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            serviceName,
			DesiredFSMState: s6fsm.OperationalStateRunning,
		},
		S6ServiceConfig: m.GenerateS6ConfigForNmapResult,
	}

	// Add the S6FSMConfig to the list of S6FSMConfigs
	m.S6ServiceConfigs = append(m.S6ServiceConfigs, s6FSMConfig)

	return m.AddNmapToS6ManagerError
}

// RemoveNmapFromS6Manager mocks removing a Nmap instance from the S6 manager
func (m *MockNmapService) RemoveNmapFromS6Manager(ctx context.Context, serviceName string) error {
	m.RemoveNmapFromS6ManagerCalled = true

	found := false

	// Remove the service from the list of S6FSMConfigs
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == serviceName {
			m.S6ServiceConfigs = append(m.S6ServiceConfigs[:i], m.S6ServiceConfigs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	// Remove the service from the list of existing services
	delete(m.ExistingServices, serviceName)
	delete(m.ServiceStates, serviceName)

	return m.RemoveNmapFromS6ManagerError
}

// StartNmap mocks starting a Nmap instance
func (m *MockNmapService) StartNmap(ctx context.Context, serviceName string) error {
	m.StartNmapCalled = true

	found := false

	// Set the desired state to running for the given instance
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == serviceName {
			m.S6ServiceConfigs[i].DesiredFSMState = s6fsm.OperationalStateRunning
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.StartNmapError
}

// StopNmap mocks stopping a Nmap instance
func (m *MockNmapService) StopNmap(ctx context.Context, serviceName string) error {
	m.StopNmapCalled = true

	found := false

	// Set the desired state to stopped for the given instance
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == serviceName {
			m.S6ServiceConfigs[i].DesiredFSMState = s6fsm.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.StopNmapError
}

// ReconcileManager mocks reconciling the Nmap manager
func (m *MockNmapService) ReconcileManager(ctx context.Context, tick uint64) (error, bool) {
	m.ReconcileManagerCalled = true
	return m.ReconcileManagerError, m.ReconcileManagerReconciled
}

// ServiceExists mocks checking if a Nmap service exists
func (m *MockNmapService) ServiceExists(ctx context.Context, serviceName string) bool {
	m.ServiceExistsCalled = true
	return m.ServiceExistsResult
}

// UpdateNmapInS6Manager mocks updating a Nmap service configuration in the S6 manager
func (m *MockNmapService) UpdateNmapInS6Manager(ctx context.Context, cfg *config.NmapServiceConfig, serviceName string) error {
	m.UpdateNmapInS6ManagerCalled = true

	// Check if the service exists
	found := false
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == serviceName {
			found = true

			// Update the config
			s6Config.S6ServiceConfig = m.GenerateS6ConfigForNmapResult
			m.S6ServiceConfigs[i] = s6Config
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.UpdateNmapInS6ManagerError
}

// SetStatusInfo sets a mock status for a given service
func (m *MockNmapService) SetStatusInfo(serviceName string, status ServiceInfo) {
	m.ServiceStates[serviceName] = &status
	m.ExistingServices[serviceName] = true
}

// SetServicePortState sets a specific port state for a service's scan result
func (m *MockNmapService) SetServicePortState(serviceName string, state string, latencyMs float64) {
	// Initialize if not exists
	if _, exists := m.ServiceStates[serviceName]; !exists {
		now := time.Now()
		m.ServiceStates[serviceName] = &ServiceInfo{
			S6FSMState: s6fsm.OperationalStateRunning,
			NmapStatus: NmapStatus{
				IsRunning: true,
				LastScan: &NmapScanResult{
					Timestamp: now,
					PortResult: PortResult{
						Port: 443,
					},
					Metrics: ScanMetrics{
						ScanDuration: 0.5,
					},
				},
			},
		}
	}

	// Set the port state and latency
	info := m.ServiceStates[serviceName]
	if info.NmapStatus.LastScan != nil {
		info.NmapStatus.LastScan.PortResult.State = state
		info.NmapStatus.LastScan.PortResult.LatencyMs = latencyMs
	}
}
