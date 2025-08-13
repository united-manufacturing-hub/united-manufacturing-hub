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
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dataflowcomponentfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	redpandafsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	spfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	spsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// CreateStreamProcessorTestConfig creates a standard StreamProcessor config for testing.
func CreateStreamProcessorTestConfig(name string, desiredState string) config.StreamProcessorConfig {
	return config.StreamProcessorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		StreamProcessorServiceConfig: streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
			Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
				Model: streamprocessorserviceconfig.ModelRef{
					Name:    "test-model",
					Version: "v1",
				},
				Sources: streamprocessorserviceconfig.SourceMapping{
					"test-source": "test-topic",
				},
				Mapping: map[string]interface{}{
					"root": "this",
				},
			},
		},
	}
}

// CreateStreamProcessorTestConfigWithMissingDfc creates a StreamProcessor config with missing DFC for testing.
func CreateStreamProcessorTestConfigWithMissingDfc(name string, desiredState string) config.StreamProcessorConfig {
	return config.StreamProcessorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: desiredState,
		},
		StreamProcessorServiceConfig: streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
			Config: streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{},
		},
	}
}

// SetupStreamProcessorServiceState configures the mock service state for StreamProcessor instance tests.
func SetupStreamProcessorServiceState(
	mockService *spsvc.MockService,
	serviceName string,
	flags spsvc.StateFlags,
) {
	mockService.SetProcessorState(serviceName, flags)
}

// ConfigureStreamProcessorServiceConfig configures the mock service with a default StreamProcessor config.
func ConfigureStreamProcessorServiceConfig(mockService *spsvc.MockService) {
	mockService.GetConfigResult = streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{
		Model: streamprocessorserviceconfig.ModelRef{
			Name:    "test-model",
			Version: "v1",
		},
		Sources: streamprocessorserviceconfig.SourceMapping{
			"test-source": "test-topic",
		},
		Mapping: map[string]interface{}{
			"root": "this",
		},
	}
}

func ConfigureStreamProcessorServiceConfigWithMissingDfc(mockService *spsvc.MockService) {
	mockService.GetConfigResult = streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{}
}

// TransitionToStreamProcessorState is a helper to configure a service for a given high-level state.
func TransitionToStreamProcessorState(mockService *spsvc.MockService, serviceName string, state string) {
	switch state {
	case spfsm.OperationalStateStopped:
		SetupStreamProcessorServiceState(mockService, serviceName, spsvc.StateFlags{
			IsDFCRunning:      false,
			IsRedpandaRunning: false,
			DfcFSMReadState:   dataflowcomponentfsm.OperationalStateStopped,
			RedpandaFSMState:  redpandafsm.OperationalStateStopped,
		})
		ConfigureStreamProcessorServiceConfig(mockService)
	case spfsm.OperationalStateStartingRedpanda:
		SetupStreamProcessorServiceState(mockService, serviceName, spsvc.StateFlags{
			IsDFCRunning:      false,
			IsRedpandaRunning: false,
			DfcFSMReadState:   dataflowcomponentfsm.OperationalStateStopped,
			RedpandaFSMState:  redpandafsm.OperationalStateStarting,
		})
		ConfigureStreamProcessorServiceConfig(mockService)
	case spfsm.OperationalStateStartingDFC:
		SetupStreamProcessorServiceState(mockService, serviceName, spsvc.StateFlags{
			IsDFCRunning:      false,
			IsRedpandaRunning: true,
			DfcFSMReadState:   dataflowcomponentfsm.OperationalStateStarting,
			RedpandaFSMState:  redpandafsm.OperationalStateIdle,
		})
		ConfigureStreamProcessorServiceConfig(mockService)
	case spfsm.OperationalStateStartingFailedDFC:
		SetupStreamProcessorServiceState(mockService, serviceName, spsvc.StateFlags{
			IsDFCRunning:      false,
			IsRedpandaRunning: true,
			DfcFSMReadState:   dataflowcomponentfsm.OperationalStateStartingFailed,
			RedpandaFSMState:  redpandafsm.OperationalStateIdle,
		})
		ConfigureStreamProcessorServiceConfigWithMissingDfc(mockService)
	case spfsm.OperationalStateIdle:
		SetupStreamProcessorServiceState(mockService, serviceName, spsvc.StateFlags{
			IsDFCRunning:      true,
			IsRedpandaRunning: true,
			DfcFSMReadState:   dataflowcomponentfsm.OperationalStateIdle,
			RedpandaFSMState:  redpandafsm.OperationalStateIdle,
		})
		ConfigureStreamProcessorServiceConfig(mockService)
	case spfsm.OperationalStateActive:
		SetupStreamProcessorServiceState(mockService, serviceName, spsvc.StateFlags{
			IsDFCRunning:      true,
			IsRedpandaRunning: true,
			DfcFSMReadState:   dataflowcomponentfsm.OperationalStateActive,
			RedpandaFSMState:  redpandafsm.OperationalStateActive,
		})
		ConfigureStreamProcessorServiceConfig(mockService)
	case spfsm.OperationalStateDegradedRedpanda:
		SetupStreamProcessorServiceState(mockService, serviceName, spsvc.StateFlags{
			IsDFCRunning:      true,
			IsRedpandaRunning: false,
			DfcFSMReadState:   dataflowcomponentfsm.OperationalStateActive,
			RedpandaFSMState:  redpandafsm.OperationalStateDegraded,
		})
		ConfigureStreamProcessorServiceConfig(mockService)
	case spfsm.OperationalStateDegradedDFC:
		SetupStreamProcessorServiceState(mockService, serviceName, spsvc.StateFlags{
			IsDFCRunning:      false,
			IsRedpandaRunning: true,
			DfcFSMReadState:   dataflowcomponentfsm.OperationalStateDegraded,
			RedpandaFSMState:  redpandafsm.OperationalStateActive,
		})
		ConfigureStreamProcessorServiceConfig(mockService)
	case spfsm.OperationalStateDegradedOther:
		SetupStreamProcessorServiceState(mockService, serviceName, spsvc.StateFlags{
			IsDFCRunning:      false,
			IsRedpandaRunning: false,
			DfcFSMReadState:   dataflowcomponentfsm.OperationalStateDegraded,
			RedpandaFSMState:  redpandafsm.OperationalStateDegraded,
		})
		ConfigureStreamProcessorServiceConfig(mockService)
	}
}

// SetupStreamProcessorInstance creates and configures a StreamProcessor instance for testing.
func SetupStreamProcessorInstance(serviceName string, desiredState string) (*spfsm.Instance, *spsvc.MockService, config.StreamProcessorConfig) {
	cfg := CreateStreamProcessorTestConfig(serviceName, desiredState)
	mockService := spsvc.NewMockService()
	mockSvcRegistry := serviceregistry.NewMockRegistry()

	return setUpMockStreamProcessorInstance(cfg, mockService, mockSvcRegistry), mockService, cfg
}

// SetupStreamProcessorInstanceWithMissingDfc creates and configures a StreamProcessor instance with missing DFC for testing.
func SetupStreamProcessorInstanceWithMissingDfc(serviceName string, desiredState string) (*spfsm.Instance, *spsvc.MockService, config.StreamProcessorConfig) {
	cfg := CreateStreamProcessorTestConfigWithMissingDfc(serviceName, desiredState)
	mockService := spsvc.NewMockService()
	mockSvcRegistry := serviceregistry.NewMockRegistry()

	return setUpMockStreamProcessorInstance(cfg, mockService, mockSvcRegistry), mockService, cfg
}

func setUpMockStreamProcessorInstance(
	cfg config.StreamProcessorConfig,
	mockService *spsvc.MockService,
	mockSvcRegistry *serviceregistry.Registry,
) *spfsm.Instance {
	// Create and configure the mock service
	mockService.ExistingComponents = make(map[string]bool)
	mockService.States = make(map[string]*spsvc.ServiceInfo)

	// Create the StreamProcessor instance
	instance := spfsm.NewInstance("", cfg)

	// Set the mock service on the instance
	instance.SetService(mockService)

	// Initialize the service status for the instance
	serviceName := cfg.Name
	mockService.ExistingComponents[serviceName] = false // Service doesn't exist initially (will be created during lifecycle)
	mockService.States[serviceName] = &spsvc.ServiceInfo{
		DFCFSMState:      dataflowcomponentfsm.OperationalStateStopped,
		RedpandaFSMState: redpandafsm.OperationalStateStopped,
	}

	return instance
}

// TestStreamProcessorStateTransition tests a state transition for a StreamProcessor instance.
func TestStreamProcessorStateTransition(
	ctx context.Context,
	instance *spfsm.Instance,
	mockService *spsvc.MockService,
	services serviceregistry.Provider,
	serviceName string,
	fromState string,
	toState string,
	maxAttempts int,
	startTick uint64,
	startTimestamp time.Time,
) (uint64, error) {
	tick := startTick

	// Validate initial state
	if instance.GetCurrentFSMState() != fromState {
		return tick, fmt.Errorf("instance is not in expected initial state %s, current state: %s", fromState, instance.GetCurrentFSMState())
	}

	// Configure the service for the target state
	TransitionToStreamProcessorState(mockService, serviceName, toState)

	// Wait for the state transition
	for range maxAttempts {
		snapshot := fsm.SystemSnapshot{
			Tick:         tick,
			SnapshotTime: startTimestamp.Add(time.Duration(tick) * time.Second), //nolint:gosec // G115: Safe conversion, test tick count is reasonable duration
		}

		err, _ := instance.Reconcile(ctx, snapshot, services)
		if err != nil {
			return tick, fmt.Errorf("reconcile failed: %w", err)
		}

		tick++

		if instance.GetCurrentFSMState() == toState {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("state transition from %s to %s failed after %d attempts, current state: %s", fromState, toState, maxAttempts, instance.GetCurrentFSMState())
}

// VerifyStreamProcessorStableState verifies that a StreamProcessor instance remains in a stable state.
func VerifyStreamProcessorStableState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *spfsm.Instance,
	mockService *spsvc.MockService,
	services serviceregistry.Provider,
	serviceName string,
	expectedState string,
	numCycles int,
) (uint64, error) {
	tick := snapshot.Tick

	if instance.GetCurrentFSMState() != expectedState {
		return tick, fmt.Errorf("instance is not in expected state %s, current state: %s", expectedState, instance.GetCurrentFSMState())
	}

	for range numCycles {
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick

		err, _ := instance.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, fmt.Errorf("reconcile failed during stability check: %w", err)
		}

		tick++

		if instance.GetCurrentFSMState() != expectedState {
			return tick, fmt.Errorf("instance state changed from %s to %s during stability check", expectedState, instance.GetCurrentFSMState())
		}
	}

	return tick, nil
}

// StabilizeStreamProcessorInstance stabilizes a StreamProcessor instance to a target state.
func StabilizeStreamProcessorInstance(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *spfsm.Instance,
	mockService *spsvc.MockService,
	services serviceregistry.Provider,
	serviceName string,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick

	// Configure the service for the target state
	TransitionToStreamProcessorState(mockService, serviceName, targetState)

	for range maxAttempts {
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick

		err, _ := instance.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, fmt.Errorf("reconcile failed during stabilization: %w", err)
		}

		tick++

		if instance.GetCurrentFSMState() == targetState {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("failed to stabilize instance to state %s after %d attempts, current state: %s", targetState, maxAttempts, instance.GetCurrentFSMState())
}

// ResetStreamProcessorInstanceError resets any error states in the mock service.
func ResetStreamProcessorInstanceError(mockService *spsvc.MockService) {
	mockService.StatusError = nil
	mockService.GetConfigError = nil
	mockService.AddToManagerError = nil
	mockService.UpdateInManagerError = nil
	mockService.RemoveFromManagerError = nil
	mockService.StartError = nil
	mockService.StopError = nil
	mockService.ForceRemoveError = nil
}

// CreateMockStreamProcessorInstance creates a mock StreamProcessor instance for testing.
func CreateMockStreamProcessorInstance(
	serviceName string,
	mockService spsvc.IStreamProcessorService,
	desiredState string,
	services serviceregistry.Provider,
) *spfsm.Instance {
	cfg := CreateStreamProcessorTestConfig(serviceName, desiredState)
	instance := spfsm.NewInstance("", cfg)
	instance.SetService(mockService)

	return instance
}

// WaitForStreamProcessorDesiredState waits for a StreamProcessor instance to reach a desired state.
func WaitForStreamProcessorDesiredState(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *spfsm.Instance,
	services serviceregistry.Provider,
	targetState string,
	maxAttempts int,
) (uint64, error) {
	tick := snapshot.Tick

	for range maxAttempts {
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick

		err, _ := instance.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, fmt.Errorf("reconcile failed while waiting for desired state: %w", err)
		}

		tick++

		if instance.GetCurrentFSMState() == targetState {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("instance did not reach desired state %s after %d attempts, current state: %s", targetState, maxAttempts, instance.GetCurrentFSMState())
}

// ReconcileStreamProcessorUntilError reconciles a StreamProcessor instance until an error occurs.
func ReconcileStreamProcessorUntilError(
	ctx context.Context,
	snapshot fsm.SystemSnapshot,
	instance *spfsm.Instance,
	mockService *spsvc.MockService,
	services serviceregistry.Provider,
	serviceName string,
	maxAttempts int,
) (uint64, error, bool) {
	tick := snapshot.Tick

	for range maxAttempts {
		currentSnapshot := snapshot
		currentSnapshot.Tick = tick

		err, _ := instance.Reconcile(ctx, currentSnapshot, services)
		if err != nil {
			return tick, err, true
		}

		tick++
	}

	return tick, nil, false
}
