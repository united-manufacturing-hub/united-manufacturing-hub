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
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
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
	mock               sync.Mutex                                                 // Protects concurrent access to mock state
	serviceExists      map[string]bool                                            // Tracks which services exist
	serviceInfo        map[string]ServiceInfo                                     // Predefined ServiceInfo responses
	serviceIsHealthy   map[string]bool                                            // Individual status flags
	serviceConfig      map[string]connectionserviceconfig.ConnectionServiceConfig // Configs for services
	serviceIsReachable map[string]bool                                            // Reachability flags
	serviceIsFlaky     map[string]bool                                            // Flakiness flags
	reconcileError     error                                                      // Error to return from ReconcileManager
	reconcileResult    bool                                                       // Result to return from ReconcileManager
}

// NewMockConnectionService creates a new mock connection service
// with initialized internal maps.
func NewMockConnectionService() *MockConnectionService {
	return &MockConnectionService{
		serviceExists:      make(map[string]bool),
		serviceInfo:        make(map[string]ServiceInfo),
		serviceIsHealthy:   make(map[string]bool),
		serviceConfig:      make(map[string]connectionserviceconfig.ConnectionServiceConfig),
		serviceIsReachable: make(map[string]bool),
		serviceIsFlaky:     make(map[string]bool),
	}
}

// SetServiceExists allows setting whether a service exists for testing.
// This affects the response of the ServiceExists method.
func (m *MockConnectionService) SetServiceExists(connName string, exists bool) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.serviceExists[connName] = exists
}

// SetServiceInfo allows setting a complete ServiceInfo struct for testing.
// This will be returned directly by the Status method.
func (m *MockConnectionService) SetServiceInfo(connName string, info ServiceInfo) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.serviceInfo[connName] = info
}

// SetServiceIsRunning allows setting the running state for testing.
// This affects the IsRunning field in the ServiceInfo returned by Status
// when no complete ServiceInfo has been predefined.
func (m *MockConnectionService) SetServiceIsHealthy(connName string, isHealthy bool) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.serviceIsHealthy[connName] = isHealthy
}

// SetServiceIsReachable allows setting the reachable state for testing.
// This affects the IsReachable field in the ServiceInfo returned by Status
// when no complete ServiceInfo has been predefined.
func (m *MockConnectionService) SetServiceIsReachable(connName string, isReachable bool) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.serviceIsReachable[connName] = isReachable
}

// SetServiceIsFlaky allows setting the flaky state for testing.
// This affects the IsFlaky field in the ServiceInfo returned by Status
// when no complete ServiceInfo has been predefined.
func (m *MockConnectionService) SetServiceIsFlaky(connName string, isFlaky bool) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.serviceIsFlaky[connName] = isFlaky
}

// SetReconcileResults allows setting the reconcile results for testing.
// These values will be returned by the ReconcileManager method.
func (m *MockConnectionService) SetReconcileResults(err error, result bool) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.reconcileError = err
	m.reconcileResult = result
}

// Status returns mocked information about the connection health.
// If a complete ServiceInfo has been set via SetServiceInfo, it returns that.
// Otherwise, it constructs one from the individual flags.
func (m *MockConnectionService) Status(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
	tick uint64,
) (ServiceInfo, error) {
	m.mock.Lock()
	defer m.mock.Unlock()

	// If we have a preconfigured service info, return that
	if info, exists := m.serviceInfo[connName]; exists {
		return info, nil
	}

	// Otherwise construct one from the individual flags
	isHealthy := m.serviceIsHealthy[connName]
	isReachable := m.serviceIsReachable[connName]
	isFlaky := m.serviceIsFlaky[connName]

	return ServiceInfo{
		IsHealthy:     isHealthy,
		PortStateOpen: isReachable, // For simplicity, port state is aligned with reachability
		IsReachable:   isReachable,
		IsFlaky:       isFlaky,
		LastChange:    tick,
	}, nil
}

// AddConnection registers a mock connection.
// It marks the service as existing and stores the configuration.
func (m *MockConnectionService) AddConnection(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionserviceconfig.ConnectionServiceConfig,
	connName string,
) error {
	m.mock.Lock()
	defer m.mock.Unlock()

	m.serviceExists[connName] = true
	m.serviceConfig[connName] = *cfg

	// Initialize default states
	if _, exists := m.serviceIsHealthy[connName]; !exists {
		m.serviceIsHealthy[connName] = false
	}
	if _, exists := m.serviceIsReachable[connName]; !exists {
		m.serviceIsReachable[connName] = false
	}
	if _, exists := m.serviceIsFlaky[connName]; !exists {
		m.serviceIsFlaky[connName] = false
	}

	return nil
}

// UpdateConnection updates a mock connection.
// It creates the service if it doesn't exist and updates its configuration.
func (m *MockConnectionService) UpdateConnection(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionserviceconfig.ConnectionServiceConfig,
	connName string,
) error {
	m.mock.Lock()
	defer m.mock.Unlock()

	if _, exists := m.serviceExists[connName]; !exists {
		m.serviceExists[connName] = true
	}

	m.serviceConfig[connName] = *cfg
	return nil
}

// RemoveConnection removes a mock connection.
// It deletes all stored data for the connection.
func (m *MockConnectionService) RemoveConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	m.mock.Lock()
	defer m.mock.Unlock()

	delete(m.serviceExists, connName)
	delete(m.serviceConfig, connName)
	delete(m.serviceIsHealthy, connName)
	delete(m.serviceIsReachable, connName)
	delete(m.serviceIsFlaky, connName)
	delete(m.serviceInfo, connName)

	return nil
}

// StartConnection starts a mock connection.
// It sets the running state to true.
func (m *MockConnectionService) StartConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	m.mock.Lock()
	defer m.mock.Unlock()

	m.serviceIsHealthy[connName] = true
	return nil
}

// StopConnection stops a mock connection.
// It sets the running state to false.
func (m *MockConnectionService) StopConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	m.mock.Lock()
	defer m.mock.Unlock()

	m.serviceIsHealthy[connName] = false
	return nil
}

// ServiceExists checks if a mock connection exists.
// Returns the value previously set with SetServiceExists.
func (m *MockConnectionService) ServiceExists(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) bool {
	m.mock.Lock()
	defer m.mock.Unlock()

	return m.serviceExists[connName]
}

// ReconcileManager mocks the reconciliation of all connections.
// Returns the error and boolean set by SetReconcileResults.
func (m *MockConnectionService) ReconcileManager(
	ctx context.Context,
	fs filesystem.Service,
	tick uint64,
) (error, bool) {
	m.mock.Lock()
	defer m.mock.Unlock()

	return m.reconcileError, m.reconcileResult
}
