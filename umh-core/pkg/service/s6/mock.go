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

package s6

import (
	"context"
	"fmt"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// MockService is a mock implementation of the S6 Service interface for testing
type MockService struct {
	// Mutex to protect concurrent access to shared maps
	mu sync.RWMutex

	// Tracks calls to methods
	CreateCalled                  bool
	RemoveCalled                  bool
	StartCalled                   bool
	StopCalled                    bool
	RestartCalled                 bool
	StatusCalled                  bool
	ExitHistoryCalled             bool
	ServiceExistsCalled           bool
	GetConfigCalled               bool
	CleanS6ServiceDirectoryCalled bool
	GetS6ConfigFileCalled         bool
	ForceRemoveCalled             bool
	GetLogsCalled                 bool
	GetStructuredLogsCalled       bool

	// Return values for each method
	CreateError                  error
	RemoveError                  error
	StartError                   error
	StopError                    error
	RestartError                 error
	StatusError                  error
	ExitHistoryError             error
	ServiceExistsError           error
	GetConfigError               error
	CleanS6ServiceDirectoryError error
	GetS6ConfigFileError         error
	ForceRemoveError             error
	GetLogsError                 error

	// Results for each method
	CreateResult                  error
	RemoveResult                  error
	StartResult                   error
	StopResult                    error
	RestartResult                 error
	StatusResult                  ServiceInfo
	ExitHistoryResult             []ExitEvent
	ServiceExistsResult           bool
	GetConfigResult               s6serviceconfig.S6ServiceConfig
	CleanS6ServiceDirectoryResult error
	GetS6ConfigFileResult         []byte
	ForceRemoveResult             error
	GetLogsResult                 []LogEntry

	// Used parameters for each method (only if needed for certain tests)
	ForceRemovePath string

	// For more complex testing scenarios
	ServiceStates    map[string]ServiceInfo
	ExistingServices map[string]bool
	// New fields for EnsureSupervision
	MockExists bool
	ErrorMode  bool
}

// NewMockService creates a new mock S6 service
func NewMockService() *MockService {
	return &MockService{
		ServiceStates:    make(map[string]ServiceInfo),
		ExistingServices: make(map[string]bool),
		StatusResult: ServiceInfo{
			Status: ServiceUnknown,
		},
	}
}

// Create mocks creating an S6 service
func (m *MockService) Create(ctx context.Context, servicePath string, config s6serviceconfig.S6ServiceConfig, services serviceregistry.Provider) error {
	m.CreateCalled = true

	m.mu.Lock()
	defer m.mu.Unlock()

	m.ExistingServices[servicePath] = true
	return m.CreateError
}

// Remove mocks removing an S6 service
func (m *MockService) Remove(ctx context.Context, servicePath string, services serviceregistry.Provider) error {
	m.RemoveCalled = true

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.ExistingServices, servicePath)
	delete(m.ServiceStates, servicePath)
	return m.RemoveError
}

// Start mocks starting an S6 service
func (m *MockService) Start(ctx context.Context, servicePath string, services serviceregistry.Provider) error {
	m.StartCalled = true

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.ExistingServices[servicePath] {
		return ErrServiceNotExist
	}

	info := m.ServiceStates[servicePath]
	info.Status = ServiceUp
	m.ServiceStates[servicePath] = info

	return m.StartError
}

// Stop mocks stopping an S6 service
func (m *MockService) Stop(ctx context.Context, servicePath string, services serviceregistry.Provider) error {
	m.StopCalled = true

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.ExistingServices[servicePath] {
		return ErrServiceNotExist
	}

	info := m.ServiceStates[servicePath]
	info.Status = ServiceDown
	m.ServiceStates[servicePath] = info

	return m.StopError
}

// Restart mocks restarting an S6 service
func (m *MockService) Restart(ctx context.Context, servicePath string, services serviceregistry.Provider) error {
	m.RestartCalled = true

	if !m.ExistingServices[servicePath] {
		return ErrServiceNotExist
	}

	info := m.ServiceStates[servicePath]
	info.Status = ServiceRestarting
	m.ServiceStates[servicePath] = info

	// Simulate a successful restart
	info.Status = ServiceUp
	m.ServiceStates[servicePath] = info

	return m.RestartError
}

// Status mocks getting the status of an S6 service
func (m *MockService) Status(ctx context.Context, servicePath string, services serviceregistry.Provider) (ServiceInfo, error) {
	m.StatusCalled = true

	if state, exists := m.ServiceStates[servicePath]; exists {
		return state, m.StatusError
	}

	return m.StatusResult, m.StatusError
}

// ServiceExists mocks checking if an S6 service exists
func (m *MockService) ServiceExists(ctx context.Context, servicePath string, services serviceregistry.Provider) (bool, error) {
	m.ServiceExistsCalled = true
	if exists := m.ExistingServices[servicePath]; exists {
		return true, m.ServiceExistsError
	}
	return false, m.ServiceExistsError
}

// GetConfig mocks getting the config of an S6 service
func (m *MockService) GetConfig(ctx context.Context, servicePath string, services serviceregistry.Provider) (s6serviceconfig.S6ServiceConfig, error) {
	m.GetConfigCalled = true
	return m.GetConfigResult, m.GetConfigError
}

func (m *MockService) ExitHistory(ctx context.Context, servicePath string, services serviceregistry.Provider) ([]ExitEvent, error) {
	m.ExitHistoryCalled = true
	return m.ExitHistoryResult, m.ExitHistoryError
}

// CleanS6ServiceDirectory implements the Service interface
func (m *MockService) CleanS6ServiceDirectory(ctx context.Context, path string, services serviceregistry.Provider) error {
	m.CleanS6ServiceDirectoryCalled = true
	return m.CleanS6ServiceDirectoryResult
}

// GetS6ConfigFile is a mock method
func (m *MockService) GetS6ConfigFile(ctx context.Context, servicePath string, configFileName string, services serviceregistry.Provider) ([]byte, error) {
	m.GetS6ConfigFileCalled = true
	return m.GetS6ConfigFileResult, m.GetS6ConfigFileError
}

// ForceRemove is a mock method
func (m *MockService) ForceRemove(ctx context.Context, servicePath string, services serviceregistry.Provider) error {
	m.ForceRemoveCalled = true
	m.ForceRemovePath = servicePath
	return m.ForceRemoveError
}

func (m *MockService) GetLogs(ctx context.Context, servicePath string, services serviceregistry.Provider) ([]LogEntry, error) {
	m.GetLogsCalled = true

	return m.GetLogsResult, m.GetLogsError
}

// EnsureSupervision is a mock implementation that checks if supervise dir exists
func (m *MockService) EnsureSupervision(ctx context.Context, servicePath string, services serviceregistry.Provider) (bool, error) {
	// Check if we should return an error
	if m.ErrorMode {
		return false, fmt.Errorf("mock error")
	}

	// Mock successful supervision
	if m.MockExists {
		return true, nil
	}

	// If first call, return false (supervision not ready yet),
	// On subsequent calls, return true (supervision ready)
	if !m.MockExists {
		m.MockExists = true // Set to true for next call
		return false, nil
	}

	return true, nil
}
