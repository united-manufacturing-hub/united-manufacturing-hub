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

package fsmtest

import (
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/nmapserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	connectionservicefsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	dataflowcomponentfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	nmapfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	protocolconverterfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

var (
	goodDataflowComponentReadConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
			Input: map[string]interface{}{
				"generate": map[string]interface{}{
					"mapping":  "root = {\"message\":\"hello world\"}",
					"interval": "1s",
				},
			},
			Output: map[string]interface{}{ // will be overwritten by the protocol converter service
				"drop": map[string]interface{}{},
			},
		},
	}

	goodDataflowComponentWriteConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
			Input: map[string]interface{}{},
			Output: map[string]interface{}{ // will be overwritten by the protocol converter service
				"stdout": map[string]interface{}{},
			},
		},
	}

	goodConnectionServiceConfig = connectionserviceconfig.ConnectionServiceConfigTemplate{
		NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
			Target: "localhost",
			Port:   "443",
		},
	}

	goodConnectionServiceConfigRuntime = connectionserviceconfig.ConnectionServiceConfigRuntime{
		NmapServiceConfig: nmapserviceconfig.NmapServiceConfig{
			Target: "localhost",
			Port:   443,
		},
	}

	missingDataflowComponentConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}

	missingConnectionServiceConfig = connectionserviceconfig.ConnectionServiceConfig{}
)

// CreateProtocolConverterTestConfig creates a standard ProtocolConverter config for testing
// it will contain a read and write DFC and a connection
func CreateProtocolConverterTestConfig(name string, desiredState string) config.ProtocolConverterConfig {
	return config.ProtocolConverterConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
			Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
				DataflowComponentReadServiceConfig: goodDataflowComponentReadConfig,
				// ignoring write DFC for now as I get otherwise the error message of
				// failed to build runtime config: template: pc:5:36: executing "pc" at <.internal.umh_topic>: map has no entry for key "umh_topic"
				// DataflowComponentWriteServiceConfig: goodDataflowComponentWriteConfig,
				ConnectionServiceConfig: goodConnectionServiceConfig,
			},
		},
	}
}

// CreateProtocolConverterTestConfigWithMissingDfc creates a standard ProtocolConverter config for testing
// it will contain a read and write DFC and a connection
func CreateProtocolConverterTestConfigWithMissingDfc(name string, desiredState string) config.ProtocolConverterConfig {
	return config.ProtocolConverterConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
			Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
				DataflowComponentReadServiceConfig: missingDataflowComponentConfig,
				// ignoring write DFC for now as I get otherwise the error message of
				// failed to build runtime config: template: pc:5:36: executing "pc" at <.internal.umh_topic>: map has no entry for key "umh_topic"
				// DataflowComponentWriteServiceConfig: goodDataflowComponentWriteConfig,
				ConnectionServiceConfig: goodConnectionServiceConfig,
			},
		},
	}
}

// CreateProtocolConverterTestConfigWithInvalidPort creates a ProtocolConverter config with an invalid port for testing error handling
// The invalid port will cause conversion from template to runtime to fail
func CreateProtocolConverterTestConfigWithInvalidPort(name string, desiredState string, invalidPort string) config.ProtocolConverterConfig {
	invalidConnectionServiceConfig := connectionserviceconfig.ConnectionServiceConfigTemplate{
		NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
			Target: "localhost",
			Port:   invalidPort, // This will cause parsing to fail
		},
	}

	return config.ProtocolConverterConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
			Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
				DataflowComponentReadServiceConfig: goodDataflowComponentReadConfig,
				// ignoring write DFC for now as I get otherwise the error message of
				// failed to build runtime config: template: pc:5:36: executing "pc" at <.internal.umh_topic>: map has no entry for key "umh_topic"
				// DataflowComponentWriteServiceConfig: goodDataflowComponentWriteConfig,
				ConnectionServiceConfig: invalidConnectionServiceConfig,
			},
		},
	}
}

// SetupProtocolConverterServiceState configures the mock service state for ProtocolConverter instance tests
func SetupProtocolConverterServiceState(
	mockService *protocolconvertersvc.MockProtocolConverterService,
	serviceName string,
	flags protocolconvertersvc.ConverterStateFlags,
) {
	// Use the new delegation approach - this will:
	// 1. Forward to DFC mock with converted flags
	// 2. Forward to Connection mock with converted flags
	// 3. Build aggregated ServiceInfo for Status() calls
	// 4. Store flags for backward compatibility
	mockService.SetConverterState(serviceName, flags)
}

// ConfigureProtocolConverterServiceConfig configures the mock service with a default ProtocolConverter config
func ConfigureProtocolConverterServiceConfig(mockService *protocolconvertersvc.MockProtocolConverterService) {
	mockService.GetConfigResult = protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{
		DataflowComponentReadServiceConfig: goodDataflowComponentReadConfig,
		// TODO: add write DFC config
		// DataflowComponentWriteServiceConfig: goodDataflowComponentWriteConfig,
		ConnectionServiceConfig: goodConnectionServiceConfigRuntime,
	}
}

func ConfigureProtocolConverterServiceConfigWithMissingDfc(mockService *protocolconvertersvc.MockProtocolConverterService) {
	mockService.GetConfigResult = protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{
		DataflowComponentReadServiceConfig: missingDataflowComponentConfig,
		// TODO: add write DFC config
		// DataflowComponentWriteServiceConfig: missingDataflowComponentConfig,
		ConnectionServiceConfig: goodConnectionServiceConfigRuntime,
	}
}

// TransitionToProtocolConverterState is a helper to configure a service for a given high-level state
// TODO: add write DFC state
func TransitionToProtocolConverterState(mockService *protocolconvertersvc.MockProtocolConverterService, serviceName string, state string) {
	switch state {
	case protocolconverterfsm.OperationalStateStopped:
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       false,
			IsConnectionUp:     false,
			IsRedpandaRunning:  false,
			DfcFSMReadState:    dataflowcomponentfsm.OperationalStateStopped,
			ConnectionFSMState: connectionservicefsm.OperationalStateStopped,
			RedpandaFSMState:   redpandafsm.OperationalStateStopped,
			PortState:          nmapfsm.PortStateClosed,
		})
		ConfigureProtocolConverterServiceConfig(mockService)
	case protocolconverterfsm.OperationalStateStartingConnection:
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       false,
			IsConnectionUp:     false,
			IsRedpandaRunning:  false,
			DfcFSMReadState:    dataflowcomponentfsm.OperationalStateStopped,
			ConnectionFSMState: connectionservicefsm.OperationalStateStarting,
			RedpandaFSMState:   redpandafsm.OperationalStateStopped,
			PortState:          nmapfsm.PortStateClosed,
		})
		ConfigureProtocolConverterServiceConfig(mockService)
	case protocolconverterfsm.OperationalStateStartingRedpanda:
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       false,
			IsConnectionUp:     true,
			IsRedpandaRunning:  false,
			DfcFSMReadState:    dataflowcomponentfsm.OperationalStateStopped,
			ConnectionFSMState: connectionservicefsm.OperationalStateUp,
			RedpandaFSMState:   redpandafsm.OperationalStateStarting,
			PortState:          nmapfsm.PortStateOpen,
		})
		ConfigureProtocolConverterServiceConfig(mockService)
	case protocolconverterfsm.OperationalStateStartingDFC:
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       false,
			IsConnectionUp:     true,
			IsRedpandaRunning:  true,
			DfcFSMReadState:    dataflowcomponentfsm.OperationalStateStarting,
			ConnectionFSMState: connectionservicefsm.OperationalStateUp,
			RedpandaFSMState:   redpandafsm.OperationalStateIdle,
			PortState:          nmapfsm.PortStateOpen,
		})
		ConfigureProtocolConverterServiceConfig(mockService)
	case protocolconverterfsm.OperationalStateStartingFailedDFC:
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       false,
			IsConnectionUp:     true,
			IsRedpandaRunning:  true,
			DfcFSMReadState:    dataflowcomponentfsm.OperationalStateStartingFailed,
			ConnectionFSMState: connectionservicefsm.OperationalStateUp,
			RedpandaFSMState:   redpandafsm.OperationalStateIdle,
			PortState:          nmapfsm.PortStateOpen,
		})
		ConfigureProtocolConverterServiceConfig(mockService)
	case protocolconverterfsm.OperationalStateStartingFailedDFCMissing:
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       false,
			IsConnectionUp:     true,
			IsRedpandaRunning:  true,
			DfcFSMReadState:    "",
			ConnectionFSMState: connectionservicefsm.OperationalStateUp,
			RedpandaFSMState:   redpandafsm.OperationalStateIdle,
			PortState:          nmapfsm.PortStateOpen,
		})
		ConfigureProtocolConverterServiceConfigWithMissingDfc(mockService) // missing DFC
	case protocolconverterfsm.OperationalStateIdle:
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       true,
			IsConnectionUp:     true,
			IsRedpandaRunning:  true,
			DfcFSMReadState:    dataflowcomponentfsm.OperationalStateIdle,
			ConnectionFSMState: connectionservicefsm.OperationalStateUp,
			RedpandaFSMState:   redpandafsm.OperationalStateIdle,
			PortState:          nmapfsm.PortStateOpen,
		})
		ConfigureProtocolConverterServiceConfig(mockService)
	case protocolconverterfsm.OperationalStateActive:
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       true,
			IsConnectionUp:     true,
			IsRedpandaRunning:  true,
			DfcFSMReadState:    dataflowcomponentfsm.OperationalStateActive,
			ConnectionFSMState: connectionservicefsm.OperationalStateUp,
			RedpandaFSMState:   redpandafsm.OperationalStateActive,
			PortState:          nmapfsm.PortStateOpen,
		})
		ConfigureProtocolConverterServiceConfig(mockService)
	case protocolconverterfsm.OperationalStateDegradedConnection:
		// Now to prevent case 3 of IsOtherDegraded, we need to set the DFC to not active
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       true,
			IsConnectionUp:     false,
			IsRedpandaRunning:  true,
			DfcFSMReadState:    dataflowcomponentfsm.OperationalStateDegraded, // to prevent case 3 of IsOtherDegraded, which is catching weird edeg cases whjere thje DFC is in good state but connection has issues. So to get degraded connetion, the DFC must also be degraded.
			ConnectionFSMState: connectionservicefsm.OperationalStateDegraded,
			RedpandaFSMState:   redpandafsm.OperationalStateActive,
			PortState:          nmapfsm.PortStateOpen,
		})
		ConfigureProtocolConverterServiceConfig(mockService)
	case protocolconverterfsm.OperationalStateDegradedRedpanda:
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       true,
			IsConnectionUp:     true,
			IsRedpandaRunning:  false,
			DfcFSMReadState:    dataflowcomponentfsm.OperationalStateActive,
			ConnectionFSMState: connectionservicefsm.OperationalStateUp,
			RedpandaFSMState:   redpandafsm.OperationalStateDegraded,
			PortState:          nmapfsm.PortStateOpen,
		})
		ConfigureProtocolConverterServiceConfig(mockService)
	case protocolconverterfsm.OperationalStateDegradedDFC:
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       false,
			IsConnectionUp:     true,
			IsRedpandaRunning:  true,
			DfcFSMReadState:    dataflowcomponentfsm.OperationalStateDegraded,
			ConnectionFSMState: connectionservicefsm.OperationalStateUp,
			RedpandaFSMState:   redpandafsm.OperationalStateActive,
			PortState:          nmapfsm.PortStateOpen,
		})
		ConfigureProtocolConverterServiceConfig(mockService)
	case protocolconverterfsm.OperationalStateDegradedOther:
		SetupProtocolConverterServiceState(mockService, serviceName, protocolconvertersvc.ConverterStateFlags{
			IsDFCRunning:       true,
			IsConnectionUp:     false,
			IsRedpandaRunning:  true,
			DfcFSMReadState:    dataflowcomponentfsm.OperationalStateActive,
			ConnectionFSMState: connectionservicefsm.OperationalStateDegraded,
			RedpandaFSMState:   redpandafsm.OperationalStateActive,
			PortState:          nmapfsm.PortStateOpen,
		})
		ConfigureProtocolConverterServiceConfig(mockService)
	}
}

// SetupProtocolConverterInstance creates and configures a ProtocolConverter instance for testing.
// Returns the instance, the mock service, and the config used to create it.
func SetupProtocolConverterInstance(serviceName string, desiredState string) (*protocolconverterfsm.ProtocolConverterInstance, *protocolconvertersvc.MockProtocolConverterService, config.ProtocolConverterConfig) {
	// Create test config
	cfg := CreateProtocolConverterTestConfig(serviceName, desiredState)

	// Create mock service
	mockService := protocolconvertersvc.NewMockProtocolConverterService()

	// Set up initial service states - the delegation approach will handle ConverterStates automatically
	mockService.ExistingComponents = make(map[string]bool)
	// Mark the service as existing so Status() calls don't fail
	mockService.ExistingComponents[serviceName] = true

	// Configure service with default config
	ConfigureProtocolConverterServiceConfig(mockService)

	// Add mock service registry
	mockSvcRegistry := serviceregistry.NewMockRegistry()

	// Create new instance
	instance := setUpMockProtocolConverterInstance(cfg, mockService, mockSvcRegistry)

	return instance, mockService, cfg
}

// SetupProtocolConverterInstanceWithMissingDfc creates and configures a ProtocolConverter instance for testing.
// Same as SetupProtocolConverterInstance, but with a missing DFC config
// Returns the instance, the mock service, and the config used to create it.
func SetupProtocolConverterInstanceWithMissingDfc(serviceName string, desiredState string) (*protocolconverterfsm.ProtocolConverterInstance, *protocolconvertersvc.MockProtocolConverterService, config.ProtocolConverterConfig) {
	// Create test config
	cfg := CreateProtocolConverterTestConfigWithMissingDfc(serviceName, desiredState)

	// Create mock service
	mockService := protocolconvertersvc.NewMockProtocolConverterService()

	// Set up initial service states - the delegation approach will handle ConverterStates automatically
	mockService.ExistingComponents = make(map[string]bool)
	// Mark the service as existing so Status() calls don't fail
	mockService.ExistingComponents[serviceName] = true

	// Configure service with default config
	ConfigureProtocolConverterServiceConfig(mockService)
	mockSvcRegistry := serviceregistry.NewMockRegistry()

	// Create new instance
	instance := setUpMockProtocolConverterInstance(cfg, mockService, mockSvcRegistry)

	return instance, mockService, cfg
}

// SetupProtocolConverterInstanceWithInvalidPort creates a ProtocolConverter instance with invalid port configuration for testing error handling
func SetupProtocolConverterInstanceWithInvalidPort(serviceName string, desiredState string, invalidPort string) (*protocolconverterfsm.ProtocolConverterInstance, *protocolconvertersvc.MockProtocolConverterService, config.ProtocolConverterConfig) {
	cfg := CreateProtocolConverterTestConfigWithInvalidPort(serviceName, desiredState, invalidPort)
	mockService := protocolconvertersvc.NewMockProtocolConverterService()
	mockSvcRegistry := serviceregistry.NewMockRegistry()

	instance := setUpMockProtocolConverterInstance(cfg, mockService, mockSvcRegistry)

	return instance, mockService, cfg
}

// setUpMockProtocolConverterInstance creates a ProtocolConverterInstance with a mock service
// This is an internal helper function used by SetupProtocolConverterInstance
func setUpMockProtocolConverterInstance(
	cfg config.ProtocolConverterConfig,
	mockService *protocolconvertersvc.MockProtocolConverterService,
	mockSvcRegistry *serviceregistry.Registry,
) *protocolconverterfsm.ProtocolConverterInstance {
	// Create the instance
	instance := protocolconverterfsm.NewProtocolConverterInstance("", cfg)

	// Set the mock service
	instance.SetService(mockService)
	return instance
}

// TestProtocolConverterStateTransition tests a transition from one state to another.
//
// Parameters:
//   - ctx: Context for cancellation
//   - instance: The ProtocolConverterInstance to reconcile
//   - mockService: The mock service to use
//   - services: The service registry provider
//   - serviceName: The name of the service instance
//   - fromState: Starting state to verify before transition
//   - toState: Target state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: The starting tick value for reconciliation
//   - startTimestamp: The starting timestamp for reconciliation
//
// Returns:
//   - uint64: The final tick value after transition
//   - error: Any error that occurred during transition
func TestProtocolConverterStateTransition(
	ctx context.Context,
	instance *protocolconverterfsm.ProtocolConverterInstance,
	mockService *protocolconvertersvc.MockProtocolConverterService,
	services serviceregistry.Provider,
	serviceName string,
	fromState string,
	toState string,
	maxAttempts int,
	startTick uint64,
	startTimestamp time.Time,
) (uint64, error) {
	// 1. Verify we are in the correct starting state
	if instance.GetCurrentFSMState() != fromState {
		return startTick, fmt.Errorf("instance not in expected state; want '%s', got '%s'", fromState, instance.GetCurrentFSMState())
	}

	// 2. Set up the mock service for the target state
	TransitionToProtocolConverterState(mockService, serviceName, toState)

	// 3. Execute reconciliation in a loop until we reach the target state
	tick := startTick
	for i := 0; i < maxAttempts; i++ {
		currentState := instance.GetCurrentFSMState()
		if currentState == toState {
			return tick, nil // Success!
		}

		// Call reconcile directly on the instance
		snapshot := fsm.SystemSnapshot{
			Tick:         tick,
			SnapshotTime: startTimestamp.Add(time.Duration(tick) * constants.DefaultTickerTime),
			CurrentConfig: config.FullConfig{
				Agent: config.AgentConfig{
					Location: map[int]string{
						0: "test-location",
					},
				},
			},
		}
		err, _ := instance.Reconcile(ctx, snapshot, services)
		if err != nil {
			return tick, err
		}
		tick++
	}

	return startTick + uint64(maxAttempts), fmt.Errorf("failed to reach target state '%s' after %d attempts; current state: '%s'", toState, maxAttempts, instance.GetCurrentFSMState())
}

// VerifyProtocolConverterStableState ensures that an instance remains in the same state
// over multiple reconciliation cycles.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The ProtocolConverterInstance to reconcile
//   - mockService: The mock service to use
//   - services: The service registry provider
//   - serviceName: The name of the service instance
//   - expectedState: The state the instance should remain in
//   - numCycles: Number of reconcile cycles to perform
//
// Returns:
//   - uint64: The final tick value after verification
//   - error: Any error that occurred during verification
func VerifyProtocolConverterStableState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *protocolconverterfsm.ProtocolConverterInstance,
	mockService *protocolconvertersvc.MockProtocolConverterService,
	services serviceregistry.Provider,
	serviceName string,
	expectedState string,
	numCycles int,
) (uint64, error) {
	// Initial state check
	if instance.GetCurrentFSMState() != expectedState {
		return snapshot.Tick, fmt.Errorf("instance is not in expected state %s; actual: %s",
			expectedState, instance.GetCurrentFSMState())
	}

	// Ensure the mock service stays configured for the expected state
	TransitionToProtocolConverterState(mockService, serviceName, expectedState)

	// Execute reconcile cycles and check state stability
	tick := snapshot.Tick
	for i := 0; i < numCycles; i++ {
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick
		_, _ = instance.Reconcile(ctx, currentSnapshot, services)
		tick++

		if instance.GetCurrentFSMState() != expectedState {
			return tick, fmt.Errorf(
				"state changed from %s to %s during cycle %d/%d",
				expectedState, instance.GetCurrentFSMState(), i+1, numCycles)
		}
	}

	return tick, nil
}

// StabilizeProtocolConverterInstance ensures the ProtocolConverter instance reaches and remains in a stable state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The ProtocolConverterInstance to stabilize
//   - mockService: The mock service to use
//   - services: The service registry provider
//   - serviceName: The name of the service
//   - targetState: The desired state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after stabilization
//   - error: Any error that occurred during stabilization
func StabilizeProtocolConverterInstance(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *protocolconverterfsm.ProtocolConverterInstance,
	mockService *protocolconvertersvc.MockProtocolConverterService,
	services serviceregistry.Provider,
	serviceName string,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	// Configure the mock service for the target state
	TransitionToProtocolConverterState(mockService, serviceName, targetState)

	// First wait for the instance to reach the target state
	tick := snapshot.Tick
	for i := 0; i < maxAttempts; i++ {
		currentState := instance.GetCurrentFSMState()
		if currentState == targetState {
			// Now verify it remains stable
			return VerifyProtocolConverterStableState(ctx, snapshot, instance, mockService, services, serviceName, targetState, 3)
		}

		currentSnapshot := snapshot
		currentSnapshot.Tick = tick
		_, _ = instance.Reconcile(ctx, currentSnapshot, services)
		tick++
	}

	return tick, fmt.Errorf(
		"failed to reach state %s after %d attempts; current state: %s",
		targetState, maxAttempts, instance.GetCurrentFSMState())
}

// ResetProtocolConverterInstanceError resets the error and backoff state of a ProtocolConverterInstance.
// This is useful in tests to clear error conditions.
//
// Parameters:
//   - mockService: The mock service to reset
func ResetProtocolConverterInstanceError(mockService *protocolconvertersvc.MockProtocolConverterService) {
	// Clear any error conditions in the mock
	mockService.AddToManagerError = nil
	mockService.UpdateInManagerError = nil
	mockService.RemoveFromManagerError = nil
	mockService.StartConnectionError = nil
	mockService.StartDFCError = nil
	mockService.StopError = nil
	mockService.ForceRemoveError = nil
	mockService.ReconcileManagerError = nil
}

// CreateMockProtocolConverterInstance creates a ProtocolConverter instance for testing.
// It sets up a new instance with a mock service.
func CreateMockProtocolConverterInstance(
	serviceName string,
	mockService protocolconvertersvc.IProtocolConverterService,
	desiredState string,
	services serviceregistry.Provider,
) *protocolconverterfsm.ProtocolConverterInstance {
	cfg := CreateProtocolConverterTestConfig(serviceName, desiredState)
	instance := protocolconverterfsm.NewProtocolConverterInstance("", cfg)
	instance.SetService(mockService)
	return instance
}

// WaitForProtocolConverterDesiredState waits for an instance's desired state to reach a target value.
// This is useful for testing error handling scenarios where the instance changes its own desired state.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The ProtocolConverterInstance to monitor
//   - services: The service registry provider
//   - targetState: The desired state to wait for
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after waiting
//   - error: Any error that occurred during waiting
func WaitForProtocolConverterDesiredState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *protocolconverterfsm.ProtocolConverterInstance,
	services serviceregistry.Provider,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		// Check if we've reached the target desired state
		if instance.GetDesiredFSMState() == targetState {
			return tick, nil
		}

		// Run a reconcile cycle
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick
		err, _ := instance.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}

		tick++
	}

	return tick, fmt.Errorf(
		"failed to reach desired state %s after %d attempts; current desired state: %s",
		targetState, maxAttempts, instance.GetDesiredFSMState())
}

// ReconcileProtocolConverterUntilError performs reconciliation until an error occurs or maximum attempts are reached.
// This is useful for testing error handling scenarios where we expect an error to occur during reconciliation.
//
// Parameters:
//   - ctx: Context for cancellation
//   - snapshot: The system snapshot to use
//   - instance: The ProtocolConverterInstance to reconcile
//   - mockService: The mock service that may produce an error
//   - services: The service registry provider
//   - serviceName: The name of the service
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//
// Returns:
//   - uint64: The final tick value after reconciliation
//   - error: The error encountered during reconciliation (nil if no error occurred after maxAttempts)
//   - bool: Whether reconciliation was successful (false if an error was encountered)
func ReconcileProtocolConverterUntilError(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *protocolconverterfsm.ProtocolConverterInstance,
	mockService *protocolconvertersvc.MockProtocolConverterService,
	services serviceregistry.Provider,
	serviceName string,
	maxAttempts int,
) (uint64, error, bool) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		// Perform a reconcile cycle and capture the error and reconciled status
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick
		err, reconciled := instance.Reconcile(ctx, currentSnapshot, services)
		tick++

		if err != nil {
			// Error found, return it along with the current tick
			return tick, err, reconciled
		}
	}

	// No error found after maxAttempts
	return tick, nil, true
}
