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

package redpanda

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/httpclient"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// MockRedpandaService is a mock implementation of the IRedpandaService interface for testing
type MockRedpandaService struct {
	// Tracks calls to methods
	GenerateS6ConfigForRedpandaCalled bool
	GetConfigCalled                   bool
	StatusCalled                      bool
	AddRedpandaToS6ManagerCalled      bool
	UpdateRedpandaInS6ManagerCalled   bool
	RemoveRedpandaFromS6ManagerCalled bool
	StartRedpandaCalled               bool
	StopRedpandaCalled                bool
	ReconcileManagerCalled            bool
	IsLogsFineCalled                  bool
	IsMetricsErrorFreeCalled          bool
	HasProcessingActivityCalled       bool
	ServiceExistsCalled               bool
	ForceRemoveRedpandaCalled         bool

	// Return values for each method
	GenerateS6ConfigForRedpandaResult s6serviceconfig.S6ServiceConfig
	GenerateS6ConfigForRedpandaError  error
	GetConfigResult                   redpandaserviceconfig.RedpandaServiceConfig
	GetConfigError                    error
	StatusResult                      ServiceInfo
	StatusError                       error
	AddRedpandaToS6ManagerError       error
	UpdateRedpandaInS6ManagerError    error
	RemoveRedpandaFromS6ManagerError  error
	StartRedpandaError                error
	StopRedpandaError                 error
	ReconcileManagerError             error
	ReconcileManagerReconciled        bool
	ServiceExistsResult               bool
	ForceRemoveRedpandaError          error

	// For more complex testing scenarios
	ServiceState      *ServiceInfo
	ServiceExistsFlag bool
	S6ServiceConfigs  []config.S6FSMConfig

	// State control for FSM testing
	stateFlags *ServiceStateFlags

	// HTTP client for mocking HTTP requests
	HTTPClient httpclient.HTTPClient

	// S6 service mock
	S6Service s6service.Service
}

// Ensure MockRedpandaService implements IRedpandaService
var _ IRedpandaService = (*MockRedpandaService)(nil)

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

// NewMockRedpandaService creates a new mock Redpanda service
func NewMockRedpandaService() *MockRedpandaService {
	return &MockRedpandaService{
		ServiceState:      nil,
		ServiceExistsFlag: false,
		S6ServiceConfigs:  make([]config.S6FSMConfig, 0),
		stateFlags:        &ServiceStateFlags{},
		HTTPClient:        NewMockHTTPClient(),
		S6Service:         &s6service.MockService{},
	}
}

// SetServiceState sets all state flags at once
func (m *MockRedpandaService) SetServiceState(flags ServiceStateFlags) {
	// Initialize ServiceInfo if not exists
	if m.ServiceState == nil {
		m.ServiceState = &ServiceInfo{
			RedpandaStatus: RedpandaStatus{},
		}
	}

	// Update S6FSMState based on IsS6Running
	if flags.IsS6Running {
		m.ServiceState.S6FSMState = s6_fsm.OperationalStateRunning
	} else {
		m.ServiceState.S6FSMState = s6_fsm.OperationalStateStopped
	}

	// Store the flags
	m.stateFlags = &flags
}

// GetServiceState gets the state flags
func (m *MockRedpandaService) GetServiceState() *ServiceStateFlags {
	return m.stateFlags
}

// GenerateS6ConfigForRedpanda mocks generating S6 config for Redpanda
func (m *MockRedpandaService) GenerateS6ConfigForRedpanda(redpandaConfig *redpandaserviceconfig.RedpandaServiceConfig) (s6serviceconfig.S6ServiceConfig, error) {
	m.GenerateS6ConfigForRedpandaCalled = true
	return m.GenerateS6ConfigForRedpandaResult, m.GenerateS6ConfigForRedpandaError
}

// GetConfig mocks getting the Redpanda configuration
func (m *MockRedpandaService) GetConfig(ctx context.Context) (redpandaserviceconfig.RedpandaServiceConfig, error) {
	m.GetConfigCalled = true

	// If error is set, return it
	if m.GetConfigError != nil {
		return redpandaserviceconfig.RedpandaServiceConfig{}, m.GetConfigError
	}

	// If a result is preset, return it
	if m.GetConfigResult.DefaultTopicRetentionMs != 0 || m.GetConfigResult.DefaultTopicRetentionBytes != 0 {
		return m.GetConfigResult, nil
	}

	// Otherwise return a default config with some test values
	return redpandaserviceconfig.RedpandaServiceConfig{
		DefaultTopicRetentionMs:    1000000,
		DefaultTopicRetentionBytes: 1000000000,
	}, nil
}

// Status mocks getting the status of a Redpanda service
func (m *MockRedpandaService) Status(ctx context.Context, tick uint64) (ServiceInfo, error) {
	m.StatusCalled = true

	// Check if the service exists
	if !m.ServiceExistsFlag {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// If we have a state already stored, return it
	if m.ServiceState != nil {
		return *m.ServiceState, m.StatusError
	}

	// If no state is stored, return the default mock result
	return m.StatusResult, m.StatusError
}

// AddRedpandaToS6Manager mocks adding a Redpanda instance to the S6 manager
func (m *MockRedpandaService) AddRedpandaToS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig) error {
	m.AddRedpandaToS6ManagerCalled = true

	// Check whether the service already exists
	s6ServiceName := constants.RedpandaServiceName
	for _, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			return ErrServiceAlreadyExists
		}
	}

	// Mark service as existing
	m.ServiceExistsFlag = true

	// Create an S6FSMConfig for this service
	s6FSMConfig := config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: s6_fsm.OperationalStateRunning,
		},
		S6ServiceConfig: m.GenerateS6ConfigForRedpandaResult,
	}

	// Add the S6FSMConfig to the list of S6FSMConfigs
	m.S6ServiceConfigs = append(m.S6ServiceConfigs, s6FSMConfig)

	return m.AddRedpandaToS6ManagerError
}

// UpdateRedpandaInS6Manager mocks updating an existing Redpanda instance in the S6 manager
func (m *MockRedpandaService) UpdateRedpandaInS6Manager(ctx context.Context, cfg *redpandaserviceconfig.RedpandaServiceConfig) error {
	m.UpdateRedpandaInS6ManagerCalled = true

	// Check if the service exists
	s6ServiceName := constants.RedpandaServiceName
	found := false
	index := -1
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			found = true
			index = i
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	// Update the config
	m.S6ServiceConfigs[index].S6ServiceConfig = m.GenerateS6ConfigForRedpandaResult

	return m.UpdateRedpandaInS6ManagerError
}

// RemoveRedpandaFromS6Manager mocks removing a Redpanda instance from the S6 manager
func (m *MockRedpandaService) RemoveRedpandaFromS6Manager(ctx context.Context) error {
	m.RemoveRedpandaFromS6ManagerCalled = true

	s6ServiceName := constants.RedpandaServiceName
	found := false

	// Remove the service from the list of S6FSMConfigs
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			m.S6ServiceConfigs = append(m.S6ServiceConfigs[:i], m.S6ServiceConfigs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	// Mark service as not existing
	m.ServiceExistsFlag = false

	return m.RemoveRedpandaFromS6ManagerError
}

// StartRedpanda mocks starting a Redpanda instance
func (m *MockRedpandaService) StartRedpanda(ctx context.Context) error {
	m.StartRedpandaCalled = true

	s6ServiceName := constants.RedpandaServiceName
	found := false

	// Set the desired state to running
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			m.S6ServiceConfigs[i].DesiredFSMState = s6_fsm.OperationalStateRunning
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.StartRedpandaError
}

// StopRedpanda mocks stopping a Redpanda instance
func (m *MockRedpandaService) StopRedpanda(ctx context.Context) error {
	m.StopRedpandaCalled = true

	s6ServiceName := constants.RedpandaServiceName
	found := false

	// Set the desired state to stopped
	for i, s6Config := range m.S6ServiceConfigs {
		if s6Config.Name == s6ServiceName {
			m.S6ServiceConfigs[i].DesiredFSMState = s6_fsm.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.StopRedpandaError
}

// ReconcileManager mocks reconciling the Redpanda manager
func (m *MockRedpandaService) ReconcileManager(ctx context.Context, tick uint64) (error, bool) {
	m.ReconcileManagerCalled = true
	return m.ReconcileManagerError, m.ReconcileManagerReconciled
}

// IsLogsFine mocks checking if the logs are fine
func (m *MockRedpandaService) IsLogsFine(logs []s6service.LogEntry, currentTime time.Time, logWindow time.Duration) bool {
	m.IsLogsFineCalled = true
	// For testing purposes, we'll consider logs fine if they're empty or nil
	return len(logs) == 0
}

// IsMetricsErrorFree mocks checking if metrics are error-free
func (m *MockRedpandaService) IsMetricsErrorFree(metrics Metrics) bool {
	m.IsMetricsErrorFreeCalled = true
	// For testing purposes, we'll consider metrics error-free
	return !metrics.Infrastructure.Storage.FreeSpaceAlert
}

// HasProcessingActivity mocks checking if a Redpanda service has processing activity
func (m *MockRedpandaService) HasProcessingActivity(status RedpandaStatus) bool {
	m.HasProcessingActivityCalled = true
	return status.MetricsState != nil && status.MetricsState.IsActive
}

// ServiceExists mocks checking if a Redpanda service exists
func (m *MockRedpandaService) ServiceExists(ctx context.Context) bool {
	m.ServiceExistsCalled = true
	return m.ServiceExistsResult
}

// ForceRemoveRedpanda mocks forcefully removing a Redpanda instance
func (m *MockRedpandaService) ForceRemoveRedpanda(ctx context.Context) error {
	m.ForceRemoveRedpandaCalled = true
	return m.ForceRemoveRedpandaError
}
