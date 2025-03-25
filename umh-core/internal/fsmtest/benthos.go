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

package fsmtest

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// CreateBenthosTestConfig creates a standard Benthos config for testing
func CreateBenthosTestConfig(name string, desiredState string) config.BenthosConfig {
	return config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		BenthosServiceConfig: config.BenthosServiceConfig{
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  "root = {\"message\":\"hello world\"}",
					"interval": "1s",
				},
			},
			Output: map[string]interface{}{
				"drop": map[string]interface{}{},
			},
			MetricsPort: 9000,
		},
	}
}

// SetupBenthosServiceState configures the mock service state for Benthos instance tests
func SetupBenthosServiceState(
	mockService *benthossvc.MockBenthosService,
	serviceName string,
	flags benthossvc.ServiceStateFlags,
) {
	// Ensure service exists in mock
	mockService.ExistingServices[serviceName] = true

	// Create service info if it doesn't exist
	if mockService.ServiceStates[serviceName] == nil {
		mockService.ServiceStates[serviceName] = &benthossvc.ServiceInfo{}
	}

	// Set S6 FSM state
	if flags.S6FSMState != "" {
		mockService.ServiceStates[serviceName].S6FSMState = flags.S6FSMState
	}

	// Update S6 observed state
	if flags.IsS6Running {
		mockService.ServiceStates[serviceName].S6ObservedState.ServiceInfo = s6svc.ServiceInfo{
			Status: s6svc.ServiceUp,
			Uptime: 10, // Set uptime to 10s to simulate config loaded
			Pid:    1234,
		}
	} else {
		mockService.ServiceStates[serviceName].S6ObservedState.ServiceInfo = s6svc.ServiceInfo{
			Status: s6svc.ServiceDown,
		}
	}

	// Update health check status
	if flags.IsHealthchecksPassed {
		mockService.ServiceStates[serviceName].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
			IsLive:  true,
			IsReady: true,
		}
	} else {
		mockService.ServiceStates[serviceName].BenthosStatus.HealthCheck = benthossvc.HealthCheck{
			IsLive:  false,
			IsReady: false,
		}
	}

	// Setup metrics state if needed
	if flags.HasProcessingActivity {
		mockService.ServiceStates[serviceName].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
			IsActive: true,
		}
	} else if mockService.ServiceStates[serviceName].BenthosStatus.MetricsState == nil {
		mockService.ServiceStates[serviceName].BenthosStatus.MetricsState = &benthossvc.BenthosMetricsState{
			IsActive: false,
		}
	}

	// Store the service state flags directly
	mockService.SetServiceState(serviceName, flags)
}

// ConfigureBenthosServiceConfig configures the mock service with a default Benthos config
func ConfigureBenthosServiceConfig(mockService *benthossvc.MockBenthosService) {
	mockService.GetConfigResult = config.BenthosServiceConfig{
		Input: map[string]interface{}{
			"generate": map[string]interface{}{
				"mapping":  "root = {\"message\":\"hello world\"}",
				"interval": "1s",
			},
		},
		Output: map[string]interface{}{
			"drop": map[string]interface{}{},
		},
		MetricsPort: 9000,
	}
}

// TransitionToBenthosState is a helper to configure a service for a given high-level state
func TransitionToBenthosState(mockService *benthossvc.MockBenthosService, serviceName string, state string) {
	switch state {
	case benthosfsm.OperationalStateStopped:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          false,
			S6FSMState:           s6fsm.OperationalStateStopped,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	case benthosfsm.OperationalStateStarting:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          false,
			S6FSMState:           s6fsm.OperationalStateStopped,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	case benthosfsm.OperationalStateStartingConfigLoading:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          true,
			S6FSMState:           s6fsm.OperationalStateRunning,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	case benthosfsm.OperationalStateIdle:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:            true,
			S6FSMState:             s6fsm.OperationalStateRunning,
			IsConfigLoaded:         true,
			IsHealthchecksPassed:   true,
			IsRunningWithoutErrors: true,
		})
	case benthosfsm.OperationalStateActive:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:            true,
			S6FSMState:             s6fsm.OperationalStateRunning,
			IsConfigLoaded:         true,
			IsHealthchecksPassed:   true,
			IsRunningWithoutErrors: true,
			HasProcessingActivity:  true,
		})
	case benthosfsm.OperationalStateDegraded:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:            true,
			S6FSMState:             s6fsm.OperationalStateRunning,
			IsConfigLoaded:         true,
			IsHealthchecksPassed:   false,
			IsRunningWithoutErrors: false,
			HasProcessingActivity:  true,
		})
	case benthosfsm.OperationalStateStopping:
		SetupBenthosServiceState(mockService, serviceName, benthossvc.ServiceStateFlags{
			IsS6Running:          false,
			S6FSMState:           s6fsm.OperationalStateStopping,
			IsConfigLoaded:       false,
			IsHealthchecksPassed: false,
		})
	}
}
