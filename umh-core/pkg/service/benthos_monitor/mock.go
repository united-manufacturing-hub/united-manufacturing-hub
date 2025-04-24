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

package benthos_monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// MockBenthosMonitorService is a mock implementation of the IBenthosMonitorService interface for testing
type MockBenthosMonitorService struct {
	// Tracks calls to methods
	GenerateS6ConfigForBenthosMonitorCalled bool
	StatusCalled                            bool
	AddBenthosToS6ManagerCalled             bool
	UpdateBenthosMonitorInS6ManagerCalled   bool
	RemoveBenthosFromS6ManagerCalled        bool
	StartBenthosCalled                      bool
	StopBenthosCalled                       bool
	ReconcileManagerCalled                  bool
	ServiceExistsCalled                     bool

	// Return values for each method
	GenerateS6ConfigForBenthosMonitorResult s6serviceconfig.S6ServiceConfig
	GenerateS6ConfigForBenthosMonitorError  error
	StatusResult                            ServiceInfo
	StatusError                             error
	AddBenthosToS6ManagerError              error
	UpdateBenthosMonitorInS6ManagerError    error
	RemoveBenthosFromS6ManagerError         error
	StartBenthosError                       error
	StopBenthosError                        error
	ReconcileManagerError                   error
	ReconcileManagerReconciled              bool
	ServiceExistsResult                     bool

	// For more complex testing scenarios
	ServiceState      *ServiceInfo
	ServiceExistsFlag bool
	S6ServiceConfig   *config.S6FSMConfig

	// State control for FSM testing
	stateFlags *ServiceStateFlags

	// Mock metrics state
	metricsState *BenthosMetricsState
}

// Ensure MockBenthosMonitorService implements IBenthosMonitorService
var _ IBenthosMonitorService = (*MockBenthosMonitorService)(nil)

// ServiceStateFlags contains all the state flags needed for FSM testing
type ServiceStateFlags struct {
	IsRunning       bool
	IsMetricsActive bool
	S6FSMState      string
}

// NewMockRedpandaMonitorService creates a new mock Redpanda monitor service
func NewMockBenthosMonitorService() *MockBenthosMonitorService {
	return &MockBenthosMonitorService{
		ServiceState:      nil,
		ServiceExistsFlag: false,
		S6ServiceConfig:   nil,
		stateFlags:        &ServiceStateFlags{},
		metricsState:      NewBenthosMetricsState(),
	}
}

// SetServiceState sets all state flags at once
func (m *MockBenthosMonitorService) SetServiceState(flags ServiceStateFlags) {
	// Initialize ServiceInfo if not exists
	if m.ServiceState == nil {
		m.ServiceState = &ServiceInfo{
			BenthosStatus: BenthosMonitorStatus{
				LastScan: &BenthosMetricsScan{
					MetricsState: m.metricsState,
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
func (m *MockBenthosMonitorService) GetServiceState() *ServiceStateFlags {
	return m.stateFlags
}

// SetReadyStatus sets the ready status of the Benthos Monitor service
func (m *MockBenthosMonitorService) SetReadyStatus(inputConnected bool, outputConnected bool, errorMsg string) {
	if m.ServiceState == nil {
		m.ServiceState = &ServiceInfo{
			BenthosStatus: BenthosMonitorStatus{
				LastScan: &BenthosMetricsScan{},
			},
		}
	}
	m.ServiceState.BenthosStatus.LastScan.HealthCheck.IsReady = inputConnected && outputConnected && errorMsg == ""
	m.ServiceState.BenthosStatus.LastScan.HealthCheck.ReadyError = errorMsg
	m.ServiceState.BenthosStatus.LastScan.HealthCheck.ConnectionStatuses = []connStatus{
		{
			Label:     "",
			Path:      "input",
			Connected: inputConnected,
			Error:     errorMsg,
		},
		{
			Label:     "",
			Path:      "output",
			Connected: outputConnected,
			Error:     errorMsg,
		},
	}
}

// SetLiveStatus sets the live status of the Benthos Monitor service
func (m *MockBenthosMonitorService) SetLiveStatus(isLive bool) {
	if m.ServiceState == nil {
		m.ServiceState = &ServiceInfo{
			BenthosStatus: BenthosMonitorStatus{
				LastScan: &BenthosMetricsScan{},
			},
		}
	}
	m.ServiceState.BenthosStatus.LastScan.HealthCheck.IsLive = isLive
}

// SetMetricsResponse sets the metrics response of the Benthos Monitor service
func (m *MockBenthosMonitorService) SetMetricsResponse(metrics Metrics) {
	m.ServiceState.BenthosStatus.LastScan.BenthosMetrics = &BenthosMetrics{
		Metrics: metrics,
	}
}

// GenerateS6ConfigForBenthosMonitor mocks generating S6 config for Benthos monitor
func (m *MockBenthosMonitorService) GenerateS6ConfigForBenthosMonitor(s6ServiceName string, port uint16) (s6serviceconfig.S6ServiceConfig, error) {
	m.GenerateS6ConfigForBenthosMonitorCalled = true

	// If error is set, return it
	if m.GenerateS6ConfigForBenthosMonitorError != nil {
		return s6serviceconfig.S6ServiceConfig{}, m.GenerateS6ConfigForBenthosMonitorError
	}

	// If a result is preset, return it
	if len(m.GenerateS6ConfigForBenthosMonitorResult.Command) > 0 {
		return m.GenerateS6ConfigForBenthosMonitorResult, nil
	}

	// Return a default config
	s6Config := s6serviceconfig.S6ServiceConfig{
		Command: []string{
			"/bin/sh",
			fmt.Sprintf("%s/%s/config/run_benthos_monitor.sh", constants.S6BaseDir, s6ServiceName),
		},
		Env: map[string]string{},
		ConfigFiles: map[string]string{
			"run_benthos_monitor.sh": "mocked script content",
		},
	}

	return s6Config, nil
}

// Status mocks getting the status of a Benthos Monitor service
func (m *MockBenthosMonitorService) Status(ctx context.Context, filesystemService filesystem.Service, tick uint64) (ServiceInfo, error) {
	m.StatusCalled = true

	// Check for context cancellation
	if ctx.Err() != nil {
		return ServiceInfo{}, ctx.Err()
	}

	// Check if the service exists
	if !m.ServiceExistsFlag {
		return ServiceInfo{}, errors.New("service 'benthos-monitor' not found")
	}

	// If we have a state already stored, return it
	if m.ServiceState != nil {
		now := time.Now()
		m.ServiceState.BenthosStatus.LastScan = &BenthosMetricsScan{
			MetricsState:   m.metricsState,
			HealthCheck:    m.ServiceState.BenthosStatus.LastScan.HealthCheck,
			BenthosMetrics: m.ServiceState.BenthosStatus.LastScan.BenthosMetrics,
			LastUpdatedAt:  now,
		}
		return *m.ServiceState, m.StatusError
	}

	// If no state is stored, return the default mock result
	return m.StatusResult, m.StatusError
}

// AddBenthosMonitorToS6Manager mocks adding a Benthos Monitor instance to the S6 manager
func (m *MockBenthosMonitorService) AddBenthosMonitorToS6Manager(ctx context.Context, port uint16) error {
	m.AddBenthosToS6ManagerCalled = true

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
	// keep the accessor in sync
	m.ServiceExistsResult = true

	// Create an S6FSMConfig for this service
	s6ServiceName := "benthos-monitor"
	s6Config, _ := m.GenerateS6ConfigForBenthosMonitor(s6ServiceName, port)
	s6FSMConfig := config.S6FSMConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            s6ServiceName,
			DesiredFSMState: s6fsm.OperationalStateRunning,
		},
		S6ServiceConfig: s6Config,
	}

	// Store the S6FSMConfig
	m.S6ServiceConfig = &s6FSMConfig

	return m.AddBenthosToS6ManagerError
}

// UpdateBenthosMonitorInS6Manager mocks updating a Benthos Monitor instance in the S6 manager
func (m *MockBenthosMonitorService) UpdateBenthosMonitorInS6Manager(ctx context.Context, port uint16) error {
	m.UpdateBenthosMonitorInS6ManagerCalled = true

	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Check whether the service exists
	if m.S6ServiceConfig == nil {
		return ErrServiceNotExist
	}

	// Remove the old instance
	if err := m.RemoveBenthosMonitorFromS6Manager(ctx); err != nil {
		return fmt.Errorf("failed to remove old benthos monitor: %w", err)
	}

	// Add the new instance
	return m.AddBenthosMonitorToS6Manager(ctx, port)
}

// RemoveBenthosMonitorFromS6Manager mocks removing a Benthos Monitor instance from the S6 manager
func (m *MockBenthosMonitorService) RemoveBenthosMonitorFromS6Manager(ctx context.Context) error {
	m.RemoveBenthosFromS6ManagerCalled = true

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
	// keep the accessor in sync
	m.ServiceExistsResult = false
	m.S6ServiceConfig = nil

	return m.RemoveBenthosFromS6ManagerError
}

// StartBenthosMonitor mocks starting a Benthos Monitor instance
func (m *MockBenthosMonitorService) StartBenthosMonitor(ctx context.Context) error {
	m.StartBenthosCalled = true

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

	return m.StartBenthosError
}

// StopBenthosMonitor mocks stopping a Benthos Monitor instance
func (m *MockBenthosMonitorService) StopBenthosMonitor(ctx context.Context) error {
	m.StopBenthosCalled = true

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

	return m.StopBenthosError
}

// ReconcileManager mocks reconciling the Benthos Monitor manager
func (m *MockBenthosMonitorService) ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (error, bool) {
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

// ServiceExists mocks checking if a Benthos Monitor service exists
func (m *MockBenthosMonitorService) ServiceExists(ctx context.Context, filesystemService filesystem.Service) bool {
	m.ServiceExistsCalled = true

	// Check for context cancellation
	if ctx.Err() != nil {
		return false
	}

	return m.ServiceExistsResult
}

// SetMetricsState allows tests to directly set the metrics state
func (m *MockBenthosMonitorService) SetMetricsState(isActive bool) {
	if m.metricsState == nil {
		m.metricsState = NewBenthosMetricsState()
	}

	m.metricsState.IsActive = isActive

	// Update the service state if it exists
	if m.ServiceState != nil && m.ServiceState.BenthosStatus.LastScan != nil {
		m.ServiceState.BenthosStatus.LastScan.MetricsState = m.metricsState
	}
}

// SetMockLogs allows tests to set the mock logs for the service
func (m *MockBenthosMonitorService) SetMockLogs(logs []s6service.LogEntry) {
	if m.ServiceState == nil {
		m.ServiceState = &ServiceInfo{
			BenthosStatus: BenthosMonitorStatus{
				LastScan: &BenthosMetricsScan{
					MetricsState: m.metricsState,
				},
			},
		}
	}

	m.ServiceState.BenthosStatus.Logs = logs
}
