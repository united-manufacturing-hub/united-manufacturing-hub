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

package streamprocessor

import (
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	spsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor"
)

// Operational state constants (using internal_fsm compatible naming)
const (
	// OperationalStateStopping is the state when the service is in the process of stopping
	OperationalStateStopping = "stopping"
	// OperationalStateStopped is the initial state and also the state when the service is stopped
	OperationalStateStopped = "stopped"

	// Starting phase states
	// OperationalStateStartingRedpanda is the state when the stream processor waits for redpanda to be either idle or active
	OperationalStateStartingRedpanda = "starting_redpanda"
	// OperationalStateStartingDFC is the state when the stream processor waits for the DFC to be either idle or active
	OperationalStateStartingDFC = "starting_dfc"
	// OperationalStateStartingFailedDFC is the state when the DFC failed to start and landed up in starting_failed state
	OperationalStateStartingFailedDFC = "starting_failed_dfc"

	// Running phase states
	// OperationalStateIdle is the state when the service is running but not actively processing data
	OperationalStateIdle = "idle"
	// OperationalStateActive is the state when the service is running and actively processing data
	OperationalStateActive = "active"
	// OperationalStateDegradedRedpanda is the state when the stream processor successfully started up once, but then the redpanda goes down or degraded
	OperationalStateDegradedRedpanda = "degraded_redpanda"
	// OperationalStateDegradedDFC is the state when the stream processor successfully started up once, but then the DFC goes down or degraded
	OperationalStateDegradedDFC = "degraded_dfc"

	// OperationalStateDegradedOther is the state when the stream processor successfully started up once, but then we detected some state that cannot happen
	// State 1: redpanda is idle or active, but the DFC benthos has no output active (or vice versa) --> they depend on each other
	// State 2: DFC is idle, but redpanda is active (or vice versa) --> they depend on each other
	OperationalStateDegradedOther = "degraded_other"
)

// Operational event constants
const (
	// Basic lifecycle events
	EventStart           = "start"
	EventStartRedpandaUp = "start_redpanda_up"
	EventStartDFCUp      = "start_dfc_up"
	EventStartRetry      = "start_retry"

	EventStop     = "stop"
	EventStopDone = "stop_done"

	EventStartFailedDFC = "start_failed_dfc"

	// EventRedpandaHealthy is either idle or active
	EventRedpandaHealthy  = "redpanda_healthy"
	EventRedpandaDegraded = "redpanda_degraded"

	// Running phase events
	EventDFCActive   = "dfc_active"
	EventDFCIdle     = "dfc_idle"
	EventDFCDegraded = "dfc_degraded"

	EventRecovered = "recovered"

	EventDegradedOther = "degraded_other"
)

// IsOperationalState returns whether the given state is a valid operational state
func IsOperationalState(state string) bool {
	switch state {
	case OperationalStateStopping,
		OperationalStateStopped,
		OperationalStateStartingRedpanda,
		OperationalStateStartingDFC,
		OperationalStateStartingFailedDFC,
		OperationalStateIdle,
		OperationalStateActive,
		OperationalStateDegradedRedpanda,
		OperationalStateDegradedDFC,
		OperationalStateDegradedOther:
		return true
	}
	return false
}

// IsStartingState returns whether the given state is a starting state
func IsStartingState(state string) bool {
	switch state {
	case OperationalStateStartingRedpanda,
		OperationalStateStartingDFC,
		OperationalStateStartingFailedDFC:
		return true
	}
	return false
}

// IsRunningState returns whether the given state is a running state
func IsRunningState(state string) bool {
	switch state {
	case OperationalStateIdle,
		OperationalStateActive,
		OperationalStateDegradedRedpanda,
		OperationalStateDegradedDFC,
		OperationalStateDegradedOther:
		return true
	}
	return false
}

// ObservedState contains the observed runtime state of a Stream Processor instance
type ObservedState struct {

	// ObservedSpecConfig contains the observed Stream Processor service config spec with variables
	// it is here for the purpose of the UI to display the variables and the location
	ObservedSpecConfig streamprocessorserviceconfig.StreamProcessorServiceConfigSpec

	// ObservedRuntimeConfig contains the observed Stream Processor service config
	ObservedRuntimeConfig streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime

	// ServiceInfo contains information about the Stream Processor service
	ServiceInfo spsvc.ServiceInfo
}

// IsObservedState implements the ObservedState interface
func (b ObservedState) IsObservedState() {}

// Instance implements the FSMInstance interface
// If Instance does not implement the FSMInstance interface, this will
// be detected at compile time
var _ publicfsm.FSMInstance = (*Instance)(nil)

// Instance is a state-machine managed instance of a Stream Processor service.
type Instance struct {

	// service is the Stream Processor service implementation to use
	// It has a manager that manages the stream processor service instances
	service spsvc.IStreamProcessorService

	baseFSMInstance *internalfsm.BaseFSMInstance

	// specConfig contains all the configuration spec for this service
	specConfig streamprocessorserviceconfig.StreamProcessorServiceConfigSpec

	// runtimeConfig is the last fully-rendered runtime configuration.
	// It is **zero-value** when the instance is first created; the real
	// configuration is rendered during the *first* Reconcile() cycle
	// once the instance has access to SystemSnapshot (agent location,
	// global variables, node name, …).
	runtimeConfig streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime

	// dfcRuntimeConfig is the last fully-rendered DFC runtime configuration.
	// It is **zero-value** when the instance is first created; the real
	// configuration is rendered during the *first* Reconcile() cycle
	// once the instance has access to SystemSnapshot (agent location,
	// global variables, node name, …).
	dfcRuntimeConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig

	// ObservedState represents the observed state of the service
	// ObservedState contains all metrics, logs, etc.
	// that are updated at the beginning of Reconcile and then used to
	// determine the next state
	ObservedState ObservedState
}

// GetLastObservedState returns the last known state of the instance
func (i *Instance) GetLastObservedState() publicfsm.ObservedState {
	return i.ObservedState
}

// SetService sets the StreamProcessor service implementation to use
// This is a testing-only utility to access the private service field
func (i *Instance) SetService(service spsvc.IStreamProcessorService) {
	i.service = service
}

// GetConfig returns the ServiceConfig for this service
// This is a testing-only utility to access the private service field
func (i *Instance) GetConfig() streamprocessorserviceconfig.StreamProcessorServiceConfigSpec {
	return i.specConfig
}

// GetLastError returns the last error of the instance
// This is a testing-only utility to access the private baseFSMInstance field
func (i *Instance) GetLastError() error {
	return i.baseFSMInstance.GetLastError()
}

// IsTransientStreakCounterMaxed returns whether the transient streak counter
// has reached the maximum number of ticks, which means that the FSM is stuck in a state
// and should be removed
func (i *Instance) IsTransientStreakCounterMaxed() bool {
	return i.baseFSMInstance.IsTransientStreakCounterMaxed()
}
