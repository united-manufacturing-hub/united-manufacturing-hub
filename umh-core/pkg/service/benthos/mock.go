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

package benthos

import (
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	s6_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/httpclient"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// MockBenthosService is a mock implementation of the IBenthosService interface for testing
type MockBenthosService struct {
	// Tracks calls to methods
	GenerateS6ConfigForBenthosCalled               bool
	GetConfigCalled                                bool
	StatusCalled                                   bool
	AddBenthosToS6ManagerCalled                    bool
	UpdateBenthosInS6ManagerCalled                 bool
	RemoveBenthosFromS6ManagerCalled               bool
	StartBenthosCalled                             bool
	StopBenthosCalled                              bool
	ReconcileManagerCalled                         bool
	IsBenthosConfigLoadedCalled                    bool
	IsBenthosS6RunningCalled                       bool
	IsBenthosS6StoppedCalled                       bool
	IsBenthosHealthchecksPassedCalled              bool
	IsBenthosRunningForSomeTimeWithoutErrorsCalled bool
	IsBenthosLogsFineCalled                        bool
	IsBenthosMetricsErrorFreeCalled                bool
	IsBenthosDegradedCalled                        bool
	HasProcessingActivityCalled                    bool
	IsLogsFineCalled                               bool
	IsMetricsErrorFreeCalled                       bool
	ServiceExistsCalled                            bool
	ForceRemoveBenthosCalled                       bool
	// Return values for each method
	GenerateS6ConfigForBenthosResult s6serviceconfig.S6ServiceConfig
	GenerateS6ConfigForBenthosError  error
	GetConfigResult                  benthosserviceconfig.BenthosServiceConfig
	GetConfigError                   error
	StatusResult                     ServiceInfo
	StatusError                      error
	AddBenthosToS6ManagerError       error
	UpdateBenthosInS6ManagerError    error
	RemoveBenthosFromS6ManagerError  error
	StartBenthosError                error
	StopBenthosError                 error
	ReconcileManagerError            error
	ReconcileManagerReconciled       bool
	ServiceExistsResult              bool
	ForceRemoveBenthosError          error
	// For more complex testing scenarios
	ServiceStates    map[string]*ServiceInfo
	ExistingServices map[string]bool
	S6ServiceConfigs []config.S6FSMConfig

	// State control for FSM testing
	stateFlags map[string]*ServiceStateFlags

	// HTTP client for mocking HTTP requests
	HTTPClient httpclient.HTTPClient

	// S6 service mock
	S6Service s6service.Service
}

// Ensure MockBenthosService implements IBenthosService
var _ IBenthosService = (*MockBenthosService)(nil)

// ServiceStateFlags contains all the state flags needed for FSM testing
type ServiceStateFlags struct {
	IsS6Running            bool
	IsConfigLoaded         bool
	IsHealthchecksPassed   bool
	IsRunningWithoutErrors bool
	HasProcessingActivity  bool
	IsDegraded             bool
	IsS6Stopped            bool
	S6FSMState             string
}

// NewMockBenthosService creates a new mock Benthos service
func NewMockBenthosService() *MockBenthosService {
	return &MockBenthosService{
		ServiceStates:    make(map[string]*ServiceInfo),
		ExistingServices: make(map[string]bool),
		S6ServiceConfigs: make([]config.S6FSMConfig, 0),
		stateFlags:       make(map[string]*ServiceStateFlags),
		S6Service:        &s6service.MockService{},
	}
}

// SetServiceState sets all state flags for a service at once
func (m *MockBenthosService) SetServiceState(serviceName string, flags ServiceStateFlags) {
	// Ensure ServiceInfo exists for this service
	if _, exists := m.ServiceStates[serviceName]; !exists {
		m.ServiceStates[serviceName] = &ServiceInfo{
			BenthosStatus: BenthosStatus{},
		}
	}

	// Update S6FSMState based on IsS6Running
	if flags.IsS6Running {
		m.ServiceStates[serviceName].S6FSMState = s6_fsm.OperationalStateRunning
	} else {
		m.ServiceStates[serviceName].S6FSMState = s6_fsm.OperationalStateStopped
	}

	// Store the flags
	m.stateFlags[serviceName] = &flags
}

// GetServiceState gets the state flags for a service
func (m *MockBenthosService) GetServiceState(serviceName string) *ServiceStateFlags {
	if flags, exists := m.stateFlags[serviceName]; exists {
		return flags
	}
	// Initialize with default flags if not exists
	flags := &ServiceStateFlags{}
	m.stateFlags[serviceName] = flags
	return flags
}

// GenerateS6ConfigForBenthos mocks generating S6 config for Benthos
func (m *MockBenthosService) GenerateS6ConfigForBenthos(benthosConfig *benthosserviceconfig.BenthosServiceConfig, name string) (s6serviceconfig.S6ServiceConfig, error) {
	m.GenerateS6ConfigForBenthosCalled = true
	return m.GenerateS6ConfigForBenthosResult, m.GenerateS6ConfigForBenthosError
}

// GetConfig mocks getting the Benthos configuration
func (m *MockBenthosService) GetConfig(ctx context.Context, filesystemService filesystem.Service, serviceName string) (benthosserviceconfig.BenthosServiceConfig, error) {
	m.GetConfigCalled = true

	// If error is set, return it
	if m.GetConfigError != nil {
		return benthosserviceconfig.BenthosServiceConfig{}, m.GetConfigError
	}

	// If a result is preset, return it
	if m.GetConfigResult.Input != nil || m.GetConfigResult.Output != nil || m.GetConfigResult.Pipeline != nil {
		return m.GetConfigResult, nil
	}

	// Otherwise return a default config with some test values
	return benthosserviceconfig.BenthosServiceConfig{
		Input:       map[string]interface{}{"type": "http_server"},
		Output:      map[string]interface{}{"type": "http_client"},
		Pipeline:    map[string]interface{}{"processors": []interface{}{}},
		LogLevel:    "INFO",
		MetricsPort: 4195,
	}, nil
}

// Status mocks getting the status of a Benthos service
func (m *MockBenthosService) Status(ctx context.Context, services serviceregistry.Provider, serviceName string, metricsPort uint16, tick uint64, loopStartTime time.Time) (ServiceInfo, error) {
	m.StatusCalled = true

	// Check if the service exists in the ExistingServices map
	if exists, ok := m.ExistingServices[serviceName]; !ok || !exists {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// Return error if metrics port is 0 (missing)
	if metricsPort == 0 {
		return ServiceInfo{}, fmt.Errorf("could not find metrics port for service %s", serviceName)
	}

	// If we have a state already stored, return it
	if state, exists := m.ServiceStates[serviceName]; exists {
		return *state, m.StatusError
	}

	// If no state is stored, return the default mock result
	return m.StatusResult, m.StatusError
}

// Helper methods for state checks
func (m *MockBenthosService) IsBenthosS6Running(serviceName string) bool {
	m.IsBenthosS6RunningCalled = true
	if flags := m.GetServiceState(serviceName); flags != nil {
		return flags.IsS6Running
	}
	return false
}

func (m *MockBenthosService) IsBenthosConfigLoaded(serviceName string) bool {
	m.IsBenthosConfigLoadedCalled = true
	if flags := m.GetServiceState(serviceName); flags != nil {
		return flags.IsConfigLoaded
	}
	return false
}

func (m *MockBenthosService) IsBenthosHealthchecksPassed(serviceName string, currentTime time.Time) bool {
	m.IsBenthosHealthchecksPassedCalled = true
	if flags := m.GetServiceState(serviceName); flags != nil {
		return flags.IsHealthchecksPassed
	}
	return false
}

func (m *MockBenthosService) IsBenthosRunningForSomeTimeWithoutErrors(serviceName string) (bool, s6service.LogEntry) {
	m.IsBenthosRunningForSomeTimeWithoutErrorsCalled = true
	if flags := m.GetServiceState(serviceName); flags != nil {
		return flags.IsRunningWithoutErrors, s6service.LogEntry{}
	}
	return false, s6service.LogEntry{}
}

func (m *MockBenthosService) IsBenthosDegraded(serviceName string) bool {
	m.IsBenthosDegradedCalled = true
	if flags := m.GetServiceState(serviceName); flags != nil {
		return flags.IsDegraded
	}
	return false
}

func (m *MockBenthosService) IsBenthosS6Stopped(serviceName string) bool {
	m.IsBenthosS6StoppedCalled = true
	if flags := m.GetServiceState(serviceName); flags != nil {
		return flags.IsS6Stopped
	}
	return false
}

func (m *MockBenthosService) HasProcessingActivity(status BenthosStatus) (bool, string) {
	m.HasProcessingActivityCalled = true
	if status.BenthosMetrics.MetricsState.IsActive {
		return true, ""
	}
	return false, "benthos metrics state is not active"
}

// AddBenthosToS6Manager mocks adding a Benthos instance to the S6 manager
func (m *MockBenthosService) AddBenthosToS6Manager(ctx context.Context, filesystemService filesystem.Service, cfg *benthosserviceconfig.BenthosServiceConfig, serviceName string) error {
	m.AddBenthosToS6ManagerCalled = true

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
			DesiredFSMState: s6_fsm.OperationalStateRunning,
		},
		S6ServiceConfig: m.GenerateS6ConfigForBenthosResult,
	}

	// Add the S6FSMConfig to the list of S6FSMConfigs
	m.S6ServiceConfigs = append(m.S6ServiceConfigs, s6FSMConfig)

	return m.AddBenthosToS6ManagerError
}

// RemoveBenthosFromS6Manager mocks removing a Benthos instance from the S6 manager
func (m *MockBenthosService) RemoveBenthosFromS6Manager(ctx context.Context, filesystemService filesystem.Service, serviceName string) error {
	m.RemoveBenthosFromS6ManagerCalled = true

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

	return m.RemoveBenthosFromS6ManagerError
}

// StartBenthos mocks starting a Benthos instance
func (m *MockBenthosService) StartBenthos(ctx context.Context, filesystemService filesystem.Service, serviceName string) error {
	m.StartBenthosCalled = true

	found := false

	// Set the desired state to running for the given instance
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == serviceName {
			m.S6ServiceConfigs[i].DesiredFSMState = s6_fsm.OperationalStateRunning
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.StartBenthosError
}

// StopBenthos mocks stopping a Benthos instance
func (m *MockBenthosService) StopBenthos(ctx context.Context, filesystemService filesystem.Service, serviceName string) error {
	m.StopBenthosCalled = true

	found := false

	// Set the desired state to stopped for the given instance
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == serviceName {
			m.S6ServiceConfigs[i].DesiredFSMState = s6_fsm.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.StopBenthosError
}

// ReconcileManager mocks reconciling the Benthos manager
func (m *MockBenthosService) ReconcileManager(ctx context.Context, services serviceregistry.Provider, tick uint64) (error, bool) {
	m.ReconcileManagerCalled = true
	return m.ReconcileManagerError, m.ReconcileManagerReconciled
}

// IsLogsFine mocks checking if the logs are fine
func (m *MockBenthosService) IsLogsFine(logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration) (bool, s6service.LogEntry) {
	m.IsLogsFineCalled = true
	// For testing purposes, always return true
	return true, s6service.LogEntry{}
}

// IsMetricsErrorFree mocks checking if metrics are error-free
func (m *MockBenthosService) IsMetricsErrorFree(metrics benthos_monitor.BenthosMetrics) (bool, string) {
	m.IsMetricsErrorFreeCalled = true
	// For testing purposes, we'll consider metrics error-free
	// This can be enhanced based on testing needs
	return true, ""
}

// UpdateBenthosInS6Manager mocks updating a Benthos service configuration in the S6 manager
func (m *MockBenthosService) UpdateBenthosInS6Manager(ctx context.Context, filesystemService filesystem.Service, cfg *benthosserviceconfig.BenthosServiceConfig, serviceName string) error {
	m.UpdateBenthosInS6ManagerCalled = true

	// Check if the service exists
	found := false
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == serviceName {
			found = true

			// Update the config
			s6Config.S6ServiceConfig = m.GenerateS6ConfigForBenthosResult
			m.S6ServiceConfigs[i] = s6Config
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.UpdateBenthosInS6ManagerError
}

// ServiceExists mocks checking if a Benthos service exists
func (m *MockBenthosService) ServiceExists(ctx context.Context, filesystemService filesystem.Service, serviceName string) bool {
	m.ServiceExistsCalled = true
	return m.ServiceExistsResult
}

// ForceRemoveBenthos mocks removing a Benthos instance from the S6 manager
func (m *MockBenthosService) ForceRemoveBenthos(ctx context.Context, filesystemService filesystem.Service, benthosName string) error {
	m.ForceRemoveBenthosCalled = true
	return m.ForceRemoveBenthosError
}
