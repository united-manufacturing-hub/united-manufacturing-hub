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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
)

// MockNmapService is a mock implementation of the INmapService interface for testing
type MockNmapService struct {
	// Configs keeps track of registered services
	Configs map[string]*config.NmapServiceConfig

	// StateMap keeps track of desired states
	StateMap map[string]string

	// Status response to return
	StatusResult ServiceInfo

	// GetConfig response to return
	GetConfigResult config.NmapServiceConfig

	// Error to return for various operations
	GenerateConfigError error
	GetConfigError      error
	StatusError         error
	AddServiceError     error
	UpdateServiceError  error
	RemoveServiceError  error
	StartServiceError   error
	StopServiceError    error
	ReconcileError      error
	ExistsError         bool // if true, ServiceExists returns false

	// For more complex testing scenarios
	ServiceStates    map[string]*ServiceInfo
	ExistingServices map[string]bool
	S6ServiceConfigs []config.S6FSMConfig
}

// Ensure MockNmapService implements INmapService
var _ INmapService = (*MockNmapService)(nil)

// NewMockNmapService creates a new mock nmap service
func NewMockNmapService() *MockNmapService {
	return &MockNmapService{
		Configs:          make(map[string]*config.NmapServiceConfig),
		StateMap:         make(map[string]string),
		ServiceStates:    make(map[string]*ServiceInfo),
		ExistingServices: make(map[string]bool),
	}
}

// GenerateS6ConfigForNmap generates a mock S6 config
func (m *MockNmapService) GenerateS6ConfigForNmap(nmapConfig *config.NmapServiceConfig, s6ServiceName string) (s6serviceconfig.S6ServiceConfig, error) {
	if m.GenerateConfigError != nil {
		return s6serviceconfig.S6ServiceConfig{}, m.GenerateConfigError
	}

	return s6serviceconfig.S6ServiceConfig{
		Command: []string{"/bin/sh", "/path/to/script.sh"},
		ConfigFiles: map[string]string{
			"run_nmap.sh": "mock script content",
		},
	}, nil
}

// GetConfig returns the mock config
func (m *MockNmapService) GetConfig(ctx context.Context, nmapName string) (config.NmapServiceConfig, error) {
	if ctx.Err() != nil {
		return config.NmapServiceConfig{}, ctx.Err()
	}

	if m.GetConfigError != nil {
		return config.NmapServiceConfig{}, m.GetConfigError
	}

	if cfg, exists := m.Configs[nmapName]; exists {
		return *cfg, nil
	}

	return m.GetConfigResult, nil
}

// Status returns the mock status
func (m *MockNmapService) Status(ctx context.Context, nmapName string, tick uint64) (ServiceInfo, error) {
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	if m.StatusError != nil {
		return ServiceInfo{}, m.StatusError
	}

	if _, exists := m.Configs[nmapName]; !exists {
		return ServiceInfo{}, ErrServiceNotExist
	}

	return m.StatusResult, nil
}

// AddNmapToS6Manager mocks adding a service
func (m *MockNmapService) AddNmapToS6Manager(ctx context.Context, cfg *config.NmapServiceConfig, nmapName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if m.AddServiceError != nil {
		return m.AddServiceError
	}

	if _, exists := m.Configs[nmapName]; exists {
		return ErrServiceAlreadyExists
	}

	m.Configs[nmapName] = cfg
	m.StateMap[nmapName] = s6fsm.OperationalStateRunning // default state is running

	// Add to legacy fields for compatibility
	s6ServiceName := "nmap-" + nmapName
	m.ExistingServices[s6ServiceName] = true

	// Create an S6FSMConfig for this service
	s6FSMConfig := config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: s6fsm.OperationalStateRunning,
		},
	}

	// Add to S6ServiceConfigs
	m.S6ServiceConfigs = append(m.S6ServiceConfigs, s6FSMConfig)

	return nil
}

// UpdateNmapInS6Manager mocks updating a service
func (m *MockNmapService) UpdateNmapInS6Manager(ctx context.Context, cfg *config.NmapServiceConfig, nmapName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if m.UpdateServiceError != nil {
		return m.UpdateServiceError
	}

	if _, exists := m.Configs[nmapName]; !exists {
		return ErrServiceNotExist
	}

	m.Configs[nmapName] = cfg

	// Update legacy fields for compatibility
	s6ServiceName := "nmap-" + nmapName
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			m.S6ServiceConfigs[i].S6ServiceConfig = s6serviceconfig.S6ServiceConfig{
				Command: []string{"/bin/sh", "/path/to/script.sh"},
				ConfigFiles: map[string]string{
					"run_nmap.sh": "updated mock script content",
				},
			}
			break
		}
	}

	return nil
}

// RemoveNmapFromS6Manager mocks removing a service
func (m *MockNmapService) RemoveNmapFromS6Manager(ctx context.Context, nmapName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if m.RemoveServiceError != nil {
		return m.RemoveServiceError
	}

	if _, exists := m.Configs[nmapName]; !exists {
		return ErrServiceNotExist
	}

	delete(m.Configs, nmapName)
	delete(m.StateMap, nmapName)

	// Update legacy fields for compatibility
	s6ServiceName := "nmap-" + nmapName
	delete(m.ExistingServices, s6ServiceName)
	delete(m.ServiceStates, s6ServiceName)

	// Remove from S6ServiceConfigs
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			m.S6ServiceConfigs = append(m.S6ServiceConfigs[:i], m.S6ServiceConfigs[i+1:]...)
			break
		}
	}

	return nil
}

// StartNmap mocks starting a service
func (m *MockNmapService) StartNmap(ctx context.Context, nmapName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if m.StartServiceError != nil {
		return m.StartServiceError
	}

	if _, exists := m.Configs[nmapName]; !exists {
		return ErrServiceNotExist
	}

	m.StateMap[nmapName] = s6fsm.OperationalStateRunning

	// Update legacy fields for compatibility
	s6ServiceName := "nmap-" + nmapName
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			m.S6ServiceConfigs[i].DesiredFSMState = s6fsm.OperationalStateRunning
			break
		}
	}

	return nil
}

// StopNmap mocks stopping a service
func (m *MockNmapService) StopNmap(ctx context.Context, nmapName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if m.StopServiceError != nil {
		return m.StopServiceError
	}

	if _, exists := m.Configs[nmapName]; !exists {
		return ErrServiceNotExist
	}

	m.StateMap[nmapName] = s6fsm.OperationalStateStopped

	// Update legacy fields for compatibility
	s6ServiceName := "nmap-" + nmapName
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			m.S6ServiceConfigs[i].DesiredFSMState = s6fsm.OperationalStateStopped
			break
		}
	}

	return nil
}

// ReconcileManager mocks reconciling the manager
func (m *MockNmapService) ReconcileManager(ctx context.Context, tick uint64) (error, bool) {
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	if m.ReconcileError != nil {
		return m.ReconcileError, false
	}

	return nil, true
}

// ServiceExists mocks checking if a service exists
func (m *MockNmapService) ServiceExists(ctx context.Context, nmapName string) bool {
	if m.ExistsError {
		return false
	}

	_, exists := m.Configs[nmapName]
	return exists
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
