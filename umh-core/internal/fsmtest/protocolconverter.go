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

	goodConnectionServiceConfig = connectionserviceconfig.ConnectionServiceConfig{
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
			Template: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
				DataflowComponentReadServiceConfig:  goodDataflowComponentReadConfig,
				DataflowComponentWriteServiceConfig: goodDataflowComponentWriteConfig,
				ConnectionServiceConfig:             goodConnectionServiceConfig,
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
		DataflowComponentReadServiceConfig:  goodDataflowComponentReadConfig,
		DataflowComponentWriteServiceConfig: goodDataflowComponentWriteConfig,
		ConnectionServiceConfig:             goodConnectionServiceConfig,
	}
}

func ConfigureProtocolConverterServiceConfigWithMissingDfc(mockService *protocolconvertersvc.MockProtocolConverterService) {
	mockService.GetConfigResult = protocolconverterserviceconfig.ProtocolConverterServiceConfigRuntime{
		DataflowComponentReadServiceConfig:  missingDataflowComponentConfig,
		DataflowComponentWriteServiceConfig: missingDataflowComponentConfig,
		ConnectionServiceConfig:             goodConnectionServiceConfig,
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

	// Add mock service registry
	mockSvcRegistry := serviceregistry.NewMockRegistry()

	// Create new instance
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
//   - filesystemService: The filesystem service to use
//   - serviceName: The name of the service instance
//   - fromState: Starting state to verify before transition
//   - toState: Target state to reach
//   - maxAttempts: Maximum number of reconcile cycles to attempt
//   - startTick: The starting tick value for reconciliation
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
	// Verify we are in the correct starting state
	if instance.GetCurrentFSMState() != fromState {
		return startTick, fmt.Errorf("instance not in expected state; want '%s', got '%s'", fromState, instance.GetCurrentFSMState())
	}

	// Execute reconciliation in a loop until we reach the target state
	tick := startTick
	for i := 0; i < maxAttempts; i++ {
		// Call reconcile directly on the instance
		snapshot := fsm.SystemSnapshot{
			Tick:         tick,
			SnapshotTime: startTimestamp.Add(time.Duration(tick) * constants.DefaultTickerTime),
		}
		err, _ := instance.Reconcile(ctx, snapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		// Check if we've reached the target state
		if instance.GetCurrentFSMState() == toState {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("failed to reach target state '%s' after %d attempts; current state: '%s'", toState, maxAttempts, instance.GetCurrentFSMState())
}

// WaitForProtocolConverterManagerStable waits for the manager to reach a stable state with all instances
func WaitForProtocolConverterManagerStable(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *protocolconverterfsm.ProtocolConverterManager,
	services serviceregistry.Provider,
) (uint64, error) {
	tick := snapshot.Tick
	maxAttempts := 10

	for i := 0; i < maxAttempts; i++ {
		// Create a copy of the snapshot with updated tick
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick

		// Reconcile the manager
		err, _ := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}
		tick++
	}

	return tick, nil
}

// WaitForProtocolConverterManagerInstanceState waits for an instance to reach a specific state
func WaitForProtocolConverterManagerInstanceState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *protocolconverterfsm.ProtocolConverterManager,
	services serviceregistry.Provider,
	instanceName string,
	expectedState string,
	maxAttempts int,
) (uint64, error) {
	// Same pattern as in other similar functions
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		// Create a new snapshot copy with updated tick
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick

		// Reconcile the manager
		err, _ := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		// Get the instance and check its state
		instance, found := manager.GetInstance(fmt.Sprintf("protconv-%s", instanceName))
		if found && instance.GetCurrentFSMState() == expectedState {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("instance %s didn't reach expected state: %s", instanceName, expectedState)
}

// WaitForProtocolConverterManagerInstanceRemoval waits for an instance to be removed
func WaitForProtocolConverterManagerInstanceRemoval(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	manager *protocolconverterfsm.ProtocolConverterManager,
	services serviceregistry.Provider,
	instanceName string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick

	for i := 0; i < maxAttempts; i++ {
		// Create a new snapshot copy with updated tick
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick

		// Reconcile the manager
		err, _ := manager.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err
		}
		tick++

		// Check if the instance is gone
		instances := manager.GetInstances()
		if _, exists := instances[instanceName]; !exists {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("instance %s was not removed after %d attempts", instanceName, maxAttempts)
}
