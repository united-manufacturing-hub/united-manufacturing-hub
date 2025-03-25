package s6

import (
	"context"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
)

// MockService is a mock implementation of the S6 Service interface for testing
type MockService struct {
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
	GetConfigResult               config.S6ServiceConfig
	CleanS6ServiceDirectoryResult error
	GetS6ConfigFileResult         []byte
	ForceRemoveResult             error
	GetLogsResult                 []LogEntry
	// For more complex testing scenarios
	ServiceStates    map[string]ServiceInfo
	ExistingServices map[string]bool
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
func (m *MockService) Create(ctx context.Context, servicePath string, config config.S6ServiceConfig) error {
	m.CreateCalled = true
	m.ExistingServices[servicePath] = true
	return m.CreateError
}

// Remove mocks removing an S6 service
func (m *MockService) Remove(ctx context.Context, servicePath string) error {
	m.RemoveCalled = true
	delete(m.ExistingServices, servicePath)
	delete(m.ServiceStates, servicePath)
	return m.RemoveError
}

// Start mocks starting an S6 service
func (m *MockService) Start(ctx context.Context, servicePath string) error {
	m.StartCalled = true

	if !m.ExistingServices[servicePath] {
		return ErrServiceNotExist
	}

	info := m.ServiceStates[servicePath]
	info.Status = ServiceUp
	m.ServiceStates[servicePath] = info

	return m.StartError
}

// Stop mocks stopping an S6 service
func (m *MockService) Stop(ctx context.Context, servicePath string) error {
	m.StopCalled = true

	if !m.ExistingServices[servicePath] {
		return ErrServiceNotExist
	}

	info := m.ServiceStates[servicePath]
	info.Status = ServiceDown
	m.ServiceStates[servicePath] = info

	return m.StopError
}

// Restart mocks restarting an S6 service
func (m *MockService) Restart(ctx context.Context, servicePath string) error {
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
func (m *MockService) Status(ctx context.Context, servicePath string) (ServiceInfo, error) {
	m.StatusCalled = true

	if state, exists := m.ServiceStates[servicePath]; exists {
		return state, m.StatusError
	}

	return m.StatusResult, m.StatusError
}

// ServiceExists mocks checking if an S6 service exists
func (m *MockService) ServiceExists(ctx context.Context, servicePath string) (bool, error) {
	m.ServiceExistsCalled = true
	if exists := m.ExistingServices[servicePath]; exists {
		return true, m.ServiceExistsError
	}
	return false, m.ServiceExistsError
}

// GetConfig mocks getting the config of an S6 service
func (m *MockService) GetConfig(ctx context.Context, servicePath string) (config.S6ServiceConfig, error) {
	m.GetConfigCalled = true
	return m.GetConfigResult, m.GetConfigError
}

func (m *MockService) ExitHistory(ctx context.Context, servicePath string) ([]ExitEvent, error) {
	m.ExitHistoryCalled = true
	return m.ExitHistoryResult, m.ExitHistoryError
}

// CleanS6ServiceDirectory implements the Service interface
func (m *MockService) CleanS6ServiceDirectory(ctx context.Context, path string) error {
	m.CleanS6ServiceDirectoryCalled = true
	return m.CleanS6ServiceDirectoryResult
}

// GetS6ConfigFile is a mock method
func (m *MockService) GetS6ConfigFile(ctx context.Context, servicePath string, configFileName string) ([]byte, error) {
	m.GetS6ConfigFileCalled = true
	return m.GetS6ConfigFileResult, m.GetS6ConfigFileError
}

// ForceRemove is a mock method
func (m *MockService) ForceRemove(ctx context.Context, servicePath string) error {
	m.ForceRemoveCalled = true
	return m.ForceRemoveError
}

func (m *MockService) GetLogs(ctx context.Context, servicePath string) ([]LogEntry, error) {
	m.GetLogsCalled = true

	return m.GetLogsResult, m.GetLogsError
}
