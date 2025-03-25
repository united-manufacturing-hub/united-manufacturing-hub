package s6

import (
	"fmt"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	public_fsm "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/metrics"
	s6service "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/service/s6"
)

// NewS6ManagerWithMockedServices creates an S6Manager with fully mocked instances
// that never touch the filesystem. Use this for testing manager logic without
// real S6 interactions.
func NewS6ManagerWithMockedServices(name string) *S6Manager {
	managerName := fmt.Sprintf("%s%s", logger.ComponentS6Manager, name)

	baseManager := public_fsm.NewBaseFSMManager[config.S6FSMConfig](
		managerName,
		"/dev/null", // Prevent any real filesystem writes
		// Extract S6 configs from full config - same as original
		func(fullConfig config.FullConfig) ([]config.S6FSMConfig, error) {
			return fullConfig.Services, nil
		},
		// Get name from S6 config - same as original
		func(cfg config.S6FSMConfig) (string, error) {
			return cfg.Name, nil
		},
		// Get desired state from S6 config - same as original
		func(cfg config.S6FSMConfig) (string, error) {
			return cfg.DesiredFSMState, nil
		},
		// Create S6 instance from config - with mock service
		func(cfg config.S6FSMConfig) (public_fsm.FSMInstance, error) {
			// Create instance with mock service
			instance, err := NewS6Instance("/dev/null", cfg)
			if err != nil {
				return nil, err
			}

			// Import the mockService package
			mockService := s6service.NewMockService()

			// Pre-configure mock responses for stopped state
			servicePath := instance.GetServicePath()
			mockService.ExistingServices[servicePath] = true
			mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
				Status: s6service.ServiceDown,
			}

			// Setup mock to return the config we're setting
			mockService.GetConfigResult = cfg.S6ServiceConfig

			// Replace the real service with our mock
			instance.SetService(mockService)

			return instance, nil
		},
		// Compare S6 configs - this is critical - always return true to avoid removal
		func(instance public_fsm.FSMInstance, cfg config.S6FSMConfig) (bool, error) {
			s6Instance, ok := instance.(*S6Instance)
			if !ok {
				return false, fmt.Errorf("instance is not an S6Instance")
			}

			// Get the mock service
			mockService, ok := s6Instance.GetService().(*s6service.MockService)
			if ok {
				// For mocks, update the stored config
				mockService.GetConfigResult = cfg.S6ServiceConfig

				// Return true to avoid triggering unnecessary recreation
				return true, nil
			}

			// Fall back to normal comparison for non-mocks
			return s6Instance.config.S6ServiceConfig.Equal(cfg.S6ServiceConfig), nil
		},
		// Set S6 config - same as original but update mock too
		func(instance public_fsm.FSMInstance, cfg config.S6FSMConfig) error {
			s6Instance, ok := instance.(*S6Instance)
			if !ok {
				return fmt.Errorf("instance is not an S6Instance")
			}

			// Update the instance config
			s6Instance.config.S6ServiceConfig = cfg.S6ServiceConfig

			// Get the mock service and update its config too
			if mockService, ok := s6Instance.GetService().(*s6service.MockService); ok {
				mockService.GetConfigResult = cfg.S6ServiceConfig
			}

			return nil
		},
	)

	metrics.InitErrorCounter(metrics.ComponentS6Manager, name)

	return &S6Manager{
		BaseFSMManager: baseManager,
	}
}
