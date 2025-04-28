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

// Package connection provides network monitoring and connectivity management.
// This file contains a mock implementation for testing.
package connection

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

// MockConnectionService provides a mock implementation of IConnectionService for testing.
// It allows for configuring specific test scenarios by setting predefined responses
// and tracking method calls.
//
// Usage examples:
//
//	// Create and configure the mock
//	mockService := NewMockConnectionService()
//	mockService.SetServiceExists("test-conn", true)
//	mockService.SetServiceIsReachable("test-conn", false)
//
//	// Test your code that uses IConnectionService
//	status, err := myComponent.DoSomethingWithConnection(mockService, "test-conn")
type MockConnectionService struct {
	// Tracks calls to methods
	GenerateNmapConfigForConnectionCalled bool
	GetConfigCalled                       bool
	StatusCalled                          bool
	AddConnectionToNmapManagerCalled      bool
	UpdateConnectionInNmapManagerCalled   bool
	RemoveConnectionFromNmapManagerCalled bool
	StartConnectionCalled                 bool
	StopConnectionCalled                  bool
	ForceRemoveConnectionCalled           bool
	ServiceExistsCalled                   bool
	ReconcileManagerCalled                bool

	// Return values for each method
	GenerateNmapConfigForConnectionResult nmapserviceconfig.NmapServiceConfig
	GenerateNmapConfigConnectionError     error
	GetConfigResult                       connectionserviceconfig.ConnectionServiceConfig
	GetConfigError                        error
	StatusResult                          ServiceInfo
	StatusError                           error
	AddConnectionToNmapManagerError       error
	UpdateConnectionInNmapManagerError    error
	RemoveConnectionFromNmapManagerError  error
	StartConnectionError                  error
	StopConnectionError                   error
	ForceRemoveConnectionError            error
	ServiceExistsResult                   bool
	ReconcileManagerError                 error
	ReconcileManagerReconciled            bool

	// For more complex testing scenarios
	ConnectionStates    map[string]*ServiceInfo
	ExistingConnections map[string]bool
	NmapConfigs         []config.NmapConfig
	RecentNmapStates    map[string][]string

	// State control for FSM testing
	stateFlags map[string]*ConnectionStateFlags

	// Nmap service mock
	NmapService nmap.INmapService
}

var _ IConnectionService = (*MockConnectionService)(nil)

// ConnectionStateFlags contains all the state flags needed for FSM testing
type ConnectionStateFlags struct {
	IsNmapRunning bool
	NmapFSMState  string
	IsFlaky       bool
}

// NewMockConnectionService creates a new mock connection service
// with initialized internal maps.
func NewMockConnectionService() *MockConnectionService {
	return &MockConnectionService{
		ConnectionStates:    make(map[string]*ServiceInfo),
		ExistingConnections: make(map[string]bool),
		NmapConfigs:         make([]config.NmapConfig, 0),
		stateFlags:          make(map[string]*ConnectionStateFlags),
		NmapService:         nmap.NewMockNmapService(),
	}
}

// SetComponentState sets all state flags for a component at once
func (m *MockConnectionService) SetConnectionState(connectionState string, flags ConnectionStateFlags) {
	// Ensure ServiceInfo exists for this component
	if _, exists := m.ConnectionStates[connectionState]; !exists {
		m.ConnectionStates[connectionState] = &ServiceInfo{
			NmapFSMState: flags.NmapFSMState,
		}
	}

	// Store the flags
	m.stateFlags[connectionState] = &flags
}

// GetComponentState gets the state flags for a component
func (m *MockConnectionService) GetConnectionState(connectionName string) *ConnectionStateFlags {
	if flags, exists := m.stateFlags[connectionName]; exists {
		return flags
	}
	// Initialize with default flags if not exists
	flags := &ConnectionStateFlags{}
	m.stateFlags[connectionName] = flags
	return flags
}

// GenerateNmapConfigForConnection mocks generating Nmap config for a Connection
func (m *MockConnectionService) GenerateNmapConfigForConnection(connectionConfig *connectionserviceconfig.ConnectionServiceConfig, connectionName string) (nmapserviceconfig.NmapServiceConfig, error) {
	m.GenerateNmapConfigForConnectionCalled = true
	return m.GenerateNmapConfigForConnectionResult, m.GenerateNmapConfigConnectionError
}

// GetConfig mocks getting the Connection configuration
func (m *MockConnectionService) GetConfig(ctx context.Context, filesystemService filesystem.Service, connectionName string) (connectionserviceconfig.ConnectionServiceConfig, error) {
	m.GetConfigCalled = true

	// If error is set, return it
	if m.GetConfigError != nil {
		return connectionserviceconfig.ConnectionServiceConfig{}, m.GetConfigError
	}

	// If a result is preset, return it
	return m.GetConfigResult, nil
}

// Status mocks getting the status of a Connection
func (m *MockConnectionService) Status(ctx context.Context, filesystemService filesystem.Service, connectionName string, tick uint64) (ServiceInfo, error) {
	m.StatusCalled = true

	// Check if the connection exists in the ExistingConnections map
	if exists, ok := m.ExistingConnections[connectionName]; !ok || !exists {
		return ServiceInfo{}, ErrServiceNotExist
	}

	// If we have a state already stored, return it
	if state, exists := m.ConnectionStates[connectionName]; exists {
		state.IsFlaky = m.isConnectionFlaky(connectionName)
		return *state, m.StatusError
	}

	// If no state is stored, return the default mock result
	return m.StatusResult, m.StatusError
}

// AddConnectionToNmapManager mocks adding a Connection to the Nmap manager
func (m *MockConnectionService) AddConnectionToNmapManager(ctx context.Context, filesystemService filesystem.Service, cfg *connectionserviceconfig.ConnectionServiceConfig, connectionName string) error {
	m.AddConnectionToNmapManagerCalled = true

	nmapName := fmt.Sprintf("connection-%s", connectionName)

	// Check whether the component already exists
	for _, nmapConfig := range m.NmapConfigs {
		if nmapConfig.Name == nmapName {
			return ErrServiceAlreadyExists
		}
	}

	// Add the component to the list of existing components
	m.ExistingConnections[connectionName] = true

	// Create a NmapConfig for this component
	nmapConfig := config.NmapConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            nmapName,
			DesiredFSMState: nmapfsm.OperationalStateOpen,
		},
		NmapServiceConfig: m.GenerateNmapConfigForConnectionResult,
	}

	// Add the NmapConfig to the list of NmapConfigs
	m.NmapConfigs = append(m.NmapConfigs, nmapConfig)

	return m.AddConnectionToNmapManagerError
}

// UpdateConnectionInNmapManager mocks updating a Connection in the Nmap manager
func (m *MockConnectionService) UpdateConnectionInNmapManager(ctx context.Context, filesystemService filesystem.Service, cfg *connectionserviceconfig.ConnectionServiceConfig, connectionName string) error {
	m.UpdateConnectionInNmapManagerCalled = true

	nmapName := fmt.Sprintf("connection-%s", connectionName)

	// Check if the component exists
	found := false
	index := -1
	for i, nmapConfig := range m.NmapConfigs {
		if nmapConfig.Name == nmapName {
			found = true
			index = i
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	// Update the BenthosConfig
	currentDesiredState := m.NmapConfigs[index].DesiredFSMState
	m.NmapConfigs[index] = config.NmapConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            nmapName,
			DesiredFSMState: currentDesiredState,
		},
		NmapServiceConfig: m.GenerateNmapConfigForConnectionResult,
	}

	return m.UpdateConnectionInNmapManagerError
}

// RemoveConnectionFromNmapManager mocks removing a Connection from the Nmap manager
func (m *MockConnectionService) RemoveConnectionFromNmapManager(ctx context.Context, filesystemService filesystem.Service, connectionName string) error {
	m.RemoveConnectionFromNmapManagerCalled = true

	nmapName := fmt.Sprintf("connection-%s", connectionName)

	found := false

	// Remove the NmapConfig from the list of NmapConfigs
	for i, nmapConfig := range m.NmapConfigs {
		if nmapConfig.Name == nmapName {
			m.NmapConfigs = append(m.NmapConfigs[:i], m.NmapConfigs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	// Remove the connection from the list of existing connections
	delete(m.ExistingConnections, connectionName)
	delete(m.ConnectionStates, connectionName)

	return m.RemoveConnectionFromNmapManagerError
}

// StartDataFlowComponent mocks starting a Connection
func (m *MockConnectionService) StartConnection(ctx context.Context, filesystemService filesystem.Service, connectionName string) error {
	m.StartConnectionCalled = true

	nmapName := fmt.Sprintf("connection-%s", connectionName)

	found := false

	// Set the desired state to active for the given component
	for i, nmapConfig := range m.NmapConfigs {
		if nmapConfig.Name == nmapName {
			m.NmapConfigs[i].DesiredFSMState = nmapfsm.OperationalStateOpen
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.StartConnectionError
}

// StopConnection mocks stopping a Connection
func (m *MockConnectionService) StopConnection(ctx context.Context, filesystemService filesystem.Service, connectionName string) error {
	m.StopConnectionCalled = true

	nmapName := fmt.Sprintf("connection-%s", connectionName)

	found := false

	// Set the desired state to stopped for the given component
	for i, nmapConfig := range m.NmapConfigs {
		if nmapConfig.Name == nmapName {
			m.NmapConfigs[i].DesiredFSMState = nmapfsm.OperationalStateStopped
			found = true
			break
		}
	}

	if !found {
		return ErrServiceNotExist
	}

	return m.StopConnectionError
}

// ForceRemoveConnection mocks force removing a Connection
func (m *MockConnectionService) ForceRemoveConnection(ctx context.Context, filesystemService filesystem.Service, connectionName string) error {
	m.ForceRemoveConnectionCalled = true
	return m.ForceRemoveConnectionError
}

// ServiceExists mocks checking if a DataFlowComponent exists
func (m *MockConnectionService) ServiceExists(ctx context.Context, filesystemService filesystem.Service, connectionName string) bool {
	m.ServiceExistsCalled = true
	return m.ServiceExistsResult
}

// ReconcileManager mocks reconciling the DataFlowComponent manager
func (m *MockConnectionService) ReconcileManager(ctx context.Context, filesystemService filesystem.Service, tick uint64) (error, bool) {
	m.ReconcileManagerCalled = true
	return m.ReconcileManagerError, m.ReconcileManagerReconciled
}

func (c *MockConnectionService) isConnectionFlaky(connName string) bool {
	scans, exists := c.RecentNmapStates[connName]

	if !exists || len(scans) < 3 {
		// Need at least 3 samples to determine flakiness
		return false
	}

	firstState := scans[0]
	secondState := scans[1]

	// if those states are equal everything is right
	if firstState == secondState {
		return false
	}

	// if not we check if theres a second difference, which we then consider flaky
	for _, state := range scans[2:] {
		if state != secondState {
			return true
		}
	}
	return false
}
