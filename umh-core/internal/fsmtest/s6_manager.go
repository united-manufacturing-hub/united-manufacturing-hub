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

//go:build test
// +build test

package fsmtest

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	s6service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_orig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6_shared"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
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
		mockService.ServiceStates[servicePath] = s6_shared.ServiceInfo{
			Status: s6_shared.ServiceUp,
			Pid:    12345,
			Uptime: 60,
		}
	case s6.OperationalStateStopped:
		// Configure mock for stopped state
		mockService.ServiceStates[servicePath] = s6_shared.ServiceInfo{
			Status:   s6_shared.ServiceDown,
			ExitCode: 0,
		}
	case s6.OperationalStateStarting:
		// Configure mock for starting/transitioning state
		mockService.ServiceStates[servicePath] = s6_shared.ServiceInfo{
			Status: s6_shared.ServiceRestarting,
		}
	default:
		// Default to unknown state
		mockService.ServiceStates[servicePath] = s6_shared.ServiceInfo{
			Status: s6_shared.ServiceUnknown,
		}
	}
}

// WaitForMockedManagerInstanceState is a helper function similar to WaitForManagerInstanceState
// but optimized for mocked S6Manager instances
func WaitForMockedManagerInstanceState(
	ctx context.Context,
	manager *s6.S6Manager,
	snapshot fsm.SystemSnapshot,
	services serviceregistry.Provider,
	instanceName, desiredState string,
	maxAttempts int,
) (uint64, error) {
	// Simply call the regular function
	return WaitForManagerInstanceState(ctx, manager, snapshot, services, instanceName, desiredState, maxAttempts)
}
