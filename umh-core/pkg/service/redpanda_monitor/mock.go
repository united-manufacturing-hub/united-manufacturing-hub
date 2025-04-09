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

package redpanda_monitor

import (
	"context"
	"errors"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// MockRedpandaMonitorService is a mock implementation of the IRedpandaMonitorService interface for testing
type MockRedpandaMonitorService struct {
	// Tracks calls to methods
	GenerateS6ConfigForRedpandaMonitorCalled bool
	StatusCalled                             bool
	AddRedpandaToS6ManagerCalled             bool
	RemoveRedpandaFromS6ManagerCalled        bool
	StartRedpandaCalled                      bool
	StopRedpandaCalled                       bool
	ReconcileManagerCalled                   bool
	ServiceExistsCalled                      bool

	// Return values for each method
	GenerateS6ConfigForRedpandaMonitorResult s6serviceconfig.S6ServiceConfig
	GenerateS6ConfigForRedpandaMonitorError  error
	StatusResult                             ServiceInfo
	StatusError                              error
	AddRedpandaToS6ManagerError              error
	RemoveRedpandaFromS6ManagerError         error
	StartRedpandaError                       error
	StopRedpandaError                        error
	ReconcileManagerError                    error
	ReconcileManagerReconciled               bool
	ServiceExistsResult                      bool

	// For more complex testing scenarios
	ServiceState      *ServiceInfo
	ServiceExistsFlag bool
	S6ServiceConfig   *config.S6FSMConfig

	// State control for FSM testing
	stateFlags *ServiceStateFlags

	// Mock metrics state
	metricsState *RedpandaMetricsState
}

// Ensure MockRedpandaMonitorService implements IRedpandaMonitorService
var _ IRedpandaMonitorService = (*MockRedpandaMonitorService)(nil)

// ServiceStateFlags contains all the state flags needed for FSM testing
type ServiceStateFlags struct {
	IsRunning       bool
	IsConfigLoaded  bool
	IsMetricsActive bool
	S6FSMState      string
}

// NewMockRedpandaMonitorService creates a new mock Redpanda monitor service
func NewMockRedpandaMonitorService() *MockRedpandaMonitorService {
	return &MockRedpandaMonitorService{
		ServiceState:      nil,
		ServiceExistsFlag: false,
		S6ServiceConfig:   nil,
		stateFlags:        &ServiceStateFlags{},
		metricsState:      NewRedpandaMetricsState(),
	}
}

// SetServiceState sets all state flags at once
func (m *MockRedpandaMonitorService) SetServiceState(flags ServiceStateFlags) {
	// Initialize ServiceInfo if not exists
	if m.ServiceState == nil {
		m.ServiceState = &ServiceInfo{
			RedpandaStatus: RedpandaMonitorStatus{
				LastScan: &RedpandaMetricsAndClusterConfig{
					Metrics: &RedpandaMetrics{
						MetricsState: m.metricsState,
					},
				},
			},
		}
	}

	// Update S6FSMState based on IsRunning
	if flags.IsRunning {
		m.ServiceState.S6FSMState = s6fsm.OperationalStateRunning
	} else {
		m.ServiceState.S6FSMState = s6fsm.OperationalStateStopped
	}

	// Update the metrics state based on IsMetricsActive
	if flags.IsMetricsActive {
		m.metricsState.IsActive = true
	} else {
		m.metricsState.IsActive = false
	}

	// Store the flags
	m.stateFlags = &flags
}

// GetServiceState gets the state flags
func (m *MockRedpandaMonitorService) GetServiceState() *ServiceStateFlags {
	return m.stateFlags
}

// GenerateS6ConfigForRedpandaMonitor mocks generating S6 config for Redpanda monitor
func (m *MockRedpandaMonitorService) GenerateS6ConfigForRedpandaMonitor() (s6serviceconfig.S6ServiceConfig, error) {
	m.GenerateS6ConfigForRedpandaMonitorCalled = true

	// If error is set, return it
	if m.GenerateS6ConfigForRedpandaMonitorError != nil {
		return s6serviceconfig.S6ServiceConfig{}, m.GenerateS6ConfigForRedpandaMonitorError
	}

	// If a result is preset, return it
	if len(m.GenerateS6ConfigForRedpandaMonitorResult.Command) > 0 {
		return m.GenerateS6ConfigForRedpandaMonitorResult, nil
	}

	// Return a default config
	s6Config := s6serviceconfig.S6ServiceConfig{
		Command: []string{
			"/bin/sh",
			fmt.Sprintf("%s/%s/config/run_redpanda_monitor.sh", constants.S6BaseDir, "redpanda-monitor-mock"),
		},
		Env: map[string]string{},
		ConfigFiles: map[string]string{
			"run_redpanda_monitor.sh": "mocked script content",
		},
	}

	return s6Config, nil
}

// Status mocks getting the status of a Redpanda Monitor service
func (m *MockRedpandaMonitorService) Status(ctx context.Context, filesystemService filesystem.Service, tick uint64) (ServiceInfo, error) {
	m.StatusCalled = true

	// Check for context cancellation
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	// Check if the service exists
	if !m.ServiceExistsFlag {
		return ServiceInfo{}, errors.New("service 'redpanda-monitor' not found")
	}

	// If we have a state already stored, return it
	if m.ServiceState != nil {
		return *m.ServiceState, m.StatusError
	}

	// If no state is stored, return the default mock result
	return m.StatusResult, m.StatusError
}

// AddRedpandaMonitorToS6Manager mocks adding a Redpanda Monitor instance to the S6 manager
func (m *MockRedpandaMonitorService) AddRedpandaMonitorToS6Manager(ctx context.Context) error {
	m.AddRedpandaToS6ManagerCalled = true

	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check whether the service already exists
	if m.S6ServiceConfig != nil {
		return ErrServiceAlreadyExists
	}

	// Mark service as existing
	m.ServiceExistsFlag = true

	// Create an S6FSMConfig for this service
	s6ServiceName := "redpanda-monitor"
	s6Config, _ := m.GenerateS6ConfigForRedpandaMonitor()
	s6FSMConfig := config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: s6fsm.OperationalStateRunning,
		},
		S6ServiceConfig: s6Config,
	}

	// Store the S6FSMConfig
	m.S6ServiceConfig = &s6FSMConfig

	return m.AddRedpandaToS6ManagerError
}

// RemoveRedpandaMonitorFromS6Manager mocks removing a Redpanda Monitor instance from the S6 manager
func (m *MockRedpandaMonitorService) RemoveRedpandaMonitorFromS6Manager(ctx context.Context) error {
	m.RemoveRedpandaFromS6ManagerCalled = true

	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check whether the service exists
	if m.S6ServiceConfig == nil {
		return ErrServiceNotExist
	}

	// Mark service as not existing
	m.ServiceExistsFlag = false
	m.S6ServiceConfig = nil

	return m.RemoveRedpandaFromS6ManagerError
}

// StartRedpandaMonitor mocks starting a Redpanda Monitor instance
func (m *MockRedpandaMonitorService) StartRedpandaMonitor(ctx context.Context) error {
	m.StartRedpandaCalled = true

	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check if the service exists
	if m.S6ServiceConfig == nil {
		return ErrServiceNotExist
	}

	// Set the desired state to running
	m.S6ServiceConfig.DesiredFSMState = s6fsm.OperationalStateRunning

	return m.StartRedpandaError
}

// StopRedpandaMonitor mocks stopping a Redpanda Monitor instance
func (m *MockRedpandaMonitorService) StopRedpandaMonitor(ctx context.Context) error {
	m.StopRedpandaCalled = true

	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check if the service exists
	if m.S6ServiceConfig == nil {
		return ErrServiceNotExist
	}

	// Set the desired state to stopped
	m.S6ServiceConfig.DesiredFSMState = s6fsm.OperationalStateStopped

	return m.StopRedpandaError
}

// ReconcileManager mocks reconciling the Redpanda Monitor manager
func (m *MockRedpandaMonitorService) ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (error, bool) {
	m.ReconcileManagerCalled = true

	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// Check if the service exists
	if m.S6ServiceConfig == nil {
		return ErrServiceNotExist, false
	}

	// After successful reconciliation, mark the service as existing
	m.ServiceExistsResult = true
	m.ServiceExistsFlag = true

	return m.ReconcileManagerError, m.ReconcileManagerReconciled
}

// ServiceExists mocks checking if a Redpanda Monitor service exists
func (m *MockRedpandaMonitorService) ServiceExists(ctx context.Context, filesystemService filesystem.Service) bool {
	m.ServiceExistsCalled = true

	// Check for context cancellation
	if ctx.Err() != nil {
		return false
	}

	return m.ServiceExistsResult
}

// SetMetricsState allows tests to directly set the metrics state
func (m *MockRedpandaMonitorService) SetMetricsState(isActive bool, freeBytes int64, totalBytes int64, freeSpaceAlert bool) {
	if m.metricsState == nil {
		m.metricsState = NewRedpandaMetricsState()
	}

	m.metricsState.IsActive = isActive

	// Create metrics to be used in service state
	metrics := Metrics{
		Infrastructure: InfrastructureMetrics{
			Storage: StorageMetrics{
				FreeBytes:      freeBytes,
				TotalBytes:     totalBytes,
				FreeSpaceAlert: freeSpaceAlert,
			},
		},
	}

	// Update the service state if it exists
	if m.ServiceState != nil && m.ServiceState.RedpandaStatus.LastScan != nil {
		m.ServiceState.RedpandaStatus.LastScan.Metrics = &RedpandaMetrics{
			MetricsState: m.metricsState,
			Metrics:      metrics,
		}
	}
}

// SetMockLogs allows tests to set the mock logs for the service
func (m *MockRedpandaMonitorService) SetMockLogs(logs []s6service.LogEntry) {
	if m.ServiceState == nil {
		m.ServiceState = &ServiceInfo{
			RedpandaStatus: RedpandaMonitorStatus{
				LastScan: &RedpandaMetricsAndClusterConfig{
					Metrics: &RedpandaMetrics{
						MetricsState: m.metricsState,
					},
				},
			},
		}
	}

	m.ServiceState.RedpandaStatus.Logs = logs
}
