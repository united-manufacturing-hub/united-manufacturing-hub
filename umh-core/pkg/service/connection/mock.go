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

package connection

import (
	"context"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// MockConnectionService provides a mock implementation of IConnectionService for testing
type MockConnectionService struct {
	mock               sync.Mutex
	serviceExists      map[string]bool
	serviceInfo        map[string]ServiceInfo
	serviceIsRunning   map[string]bool
	serviceConfig      map[string]connectionconfig.ConnectionServiceConfig
	serviceIsReachable map[string]bool
	serviceIsFlaky     map[string]bool
	reconcileError     error
	reconcileResult    bool
}

// NewMockConnectionService creates a new mock connection service
func NewMockConnectionService() *MockConnectionService {
	return &MockConnectionService{
		serviceExists:      make(map[string]bool),
		serviceInfo:        make(map[string]ServiceInfo),
		serviceIsRunning:   make(map[string]bool),
		serviceConfig:      make(map[string]connectionconfig.ConnectionServiceConfig),
		serviceIsReachable: make(map[string]bool),
		serviceIsFlaky:     make(map[string]bool),
	}
}

// SetServiceExists allows setting whether a service exists for testing
func (m *MockConnectionService) SetServiceExists(connName string, exists bool) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.serviceExists[connName] = exists
}

// SetServiceInfo allows setting a complete ServiceInfo struct for testing
func (m *MockConnectionService) SetServiceInfo(connName string, info ServiceInfo) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.serviceInfo[connName] = info
}

// SetServiceIsRunning allows setting the running state for testing
func (m *MockConnectionService) SetServiceIsRunning(connName string, isRunning bool) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.serviceIsRunning[connName] = isRunning
}

// SetServiceIsReachable allows setting the reachable state for testing
func (m *MockConnectionService) SetServiceIsReachable(connName string, isReachable bool) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.serviceIsReachable[connName] = isReachable
}

// SetServiceIsFlaky allows setting the flaky state for testing
func (m *MockConnectionService) SetServiceIsFlaky(connName string, isFlaky bool) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.serviceIsFlaky[connName] = isFlaky
}

// SetReconcileResults allows setting the reconcile results for testing
func (m *MockConnectionService) SetReconcileResults(err error, result bool) {
	m.mock.Lock()
	defer m.mock.Unlock()
	m.reconcileError = err
	m.reconcileResult = result
}

// Status returns mocked information about the connection health
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
	isRunning := m.serviceIsRunning[connName]
	isReachable := m.serviceIsReachable[connName]
	isFlaky := m.serviceIsFlaky[connName]

	return ServiceInfo{
		IsRunning:     isRunning,
		PortStateOpen: isReachable, // For simplicity, port state is aligned with reachability
		IsReachable:   isReachable,
		IsFlaky:       isFlaky,
		LastChange:    tick,
	}, nil
}

// AddConnection registers a mock connection
func (m *MockConnectionService) AddConnection(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionconfig.ConnectionServiceConfig,
	connName string,
) error {
	m.mock.Lock()
	defer m.mock.Unlock()

	m.serviceExists[connName] = true
	m.serviceConfig[connName] = *cfg

	// Initialize default states
	if _, exists := m.serviceIsRunning[connName]; !exists {
		m.serviceIsRunning[connName] = false
	}
	if _, exists := m.serviceIsReachable[connName]; !exists {
		m.serviceIsReachable[connName] = false
	}
	if _, exists := m.serviceIsFlaky[connName]; !exists {
		m.serviceIsFlaky[connName] = false
	}

	return nil
}

// UpdateConnection updates a mock connection
func (m *MockConnectionService) UpdateConnection(
	ctx context.Context,
	fs filesystem.Service,
	cfg *connectionconfig.ConnectionServiceConfig,
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

// RemoveConnection removes a mock connection
func (m *MockConnectionService) RemoveConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	m.mock.Lock()
	defer m.mock.Unlock()

	delete(m.serviceExists, connName)
	delete(m.serviceConfig, connName)
	delete(m.serviceIsRunning, connName)
	delete(m.serviceIsReachable, connName)
	delete(m.serviceIsFlaky, connName)
	delete(m.serviceInfo, connName)

	return nil
}

// StartConnection starts a mock connection
func (m *MockConnectionService) StartConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	m.mock.Lock()
	defer m.mock.Unlock()

	m.serviceIsRunning[connName] = true
	return nil
}

// StopConnection stops a mock connection
func (m *MockConnectionService) StopConnection(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) error {
	m.mock.Lock()
	defer m.mock.Unlock()

	m.serviceIsRunning[connName] = false
	return nil
}

// ServiceExists checks if a mock connection exists
func (m *MockConnectionService) ServiceExists(
	ctx context.Context,
	fs filesystem.Service,
	connName string,
) bool {
	m.mock.Lock()
	defer m.mock.Unlock()

	return m.serviceExists[connName]
}

// ReconcileManager returns mock reconcile results
func (m *MockConnectionService) ReconcileManager(
	ctx context.Context,
	fs filesystem.Service,
	tick uint64,
) (error, bool) {
	m.mock.Lock()
	defer m.mock.Unlock()

	return m.reconcileError, m.reconcileResult
}
