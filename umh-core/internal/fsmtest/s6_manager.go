package fsmtest

import (
	"context"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/fsm/s6"
	s6service "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/service/s6"
)

// ConfigureS6MockServiceState configures the mock service for a specific instance
// This is useful when you need to simulate specific service states in tests
func ConfigureS6MockServiceState(s6Instance *s6.S6Instance, state string) {
	if s6Instance == nil {
		return
	}

	// Get the mock service
	mockService, ok := s6Instance.GetService().(*s6service.MockService)
	if !ok {
		return // Not a mock service
	}

	servicePath := s6Instance.GetServicePath()
	mockService.ExistingServices[servicePath] = true

	// Ensure we don't trigger config mismatch removals
	mockService.GetConfigResult = s6Instance.GetConfig().S6ServiceConfig

	switch state {
	case s6.OperationalStateRunning:
		// Configure mock for running state
		mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
			Status: s6service.ServiceUp,
			Pid:    12345,
			Uptime: 60,
		}
	case s6.OperationalStateStopped:
		// Configure mock for stopped state
		mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
			Status:   s6service.ServiceDown,
			ExitCode: 0,
		}
	case s6.OperationalStateStarting:
		// Configure mock for starting/transitioning state
		mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
			Status: s6service.ServiceRestarting,
		}
	default:
		// Default to unknown state
		mockService.ServiceStates[servicePath] = s6service.ServiceInfo{
			Status: s6service.ServiceUnknown,
		}
	}
}

// WaitForMockedManagerInstanceState is a helper function similar to WaitForManagerInstanceState
// but optimized for mocked S6Manager instances
func WaitForMockedManagerInstanceState(
	ctx context.Context,
	manager *s6.S6Manager,
	fullConfig config.FullConfig,
	instanceName, desiredState string,
	maxAttempts int,
	tick uint64,
) (uint64, error) {
	// Simply call the regular function
	return WaitForManagerInstanceState(ctx, manager, fullConfig, instanceName, desiredState, maxAttempts, tick)
}
