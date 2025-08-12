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

package protocolconverter

import (
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	protocolconverterconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/bridgeserviceconfig"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
)

// Operational state constants (using internal_fsm compatible naming)
const (
	// OperationalStateStopping is the state when the service is in the process of stopping
	OperationalStateStopping = "stopping"
	// OperationalStateStopped is the initial state and also the state when the service is stopped
	OperationalStateStopped = "stopped"

	// Starting phase states
	// OperationalStateStartingConnection is the state when the protocol converter waits until the connection fsm returns "up"
	OperationalStateStartingConnection = "starting_connection"
	// OperationalStateStartingRedpanda is the state when the protocol converter waits for redpanda to be either idle or active
	OperationalStateStartingRedpanda = "starting_redpanda"
	// OperationalStateStartingDFC is the state when the protocol converter waits for the DFC to be either idle or active
	OperationalStateStartingDFC = "starting_dfc"
	// OperationalStateStartingFailedDFC is the state when the DFC failed to start and landed up in starting_failed state
	OperationalStateStartingFailedDFC = "starting_failed_dfc"
	// OperationalStateStartingFailedDFCMissing is the state when the DFC is missing (e.g., if only deployed so far the connection)
	OperationalStateStartingFailedDFCMissing = "starting_failed_dfc_missing"

	// Running phase states
	// OperationalStateIdle is the state when the service is running but not actively processing data
	OperationalStateIdle = "idle"
	// OperationalStateActive is the state when the service is running and actively processing data
	OperationalStateActive = "active"
	// OperationalStateDegradedConnection is the state when the protocol converter successfully started up once, but then the connection goes down or degraded
	OperationalStateDegradedConnection = "degraded_connection"
	// OperationalStateDegradedRedpanda is the state when the protocol converter successfully started up once, but then the redpanda goes down or degraded
	OperationalStateDegradedRedpanda = "degraded_redpanda"
	// OperationalStateDegradedDFC is the state when the protocol converter successfully started up once, but then the DFC goes down or degraded
	OperationalStateDegradedDFC = "degraded_dfc"

	// OperationalStateDegradedOther is the state when the protocol converter successfully started up once, but then we detected some state that cannot happen
	// State 1: DFC is idle, but redpanda is active (or vice versa) --> they depend on each other
	// State 2: redpanda is idle or active, but the DFC benthos has no output active (or vice versa) --> they depend on each other
	// State 3: connection is considered down, but DFC input is up --> this should never happen
	OperationalStateDegradedOther = "degraded_other"
)

// Operational event constants
const (
	// Basic lifecycle events
	EventStart             = "start"
	EventStartConnectionUp = "start_connection_up"
	EventStartRedpandaUp   = "start_redpanda_up"
	EventStartDFCUp        = "start_dfc_up"
	EventStartRetry        = "start_retry"

	EventStop     = "stop"
	EventStopDone = "stop_done"

	EventStartFailedDFCMissing = "start_failed_dfc_missing"
	EventStartFailedDFC        = "start_failed_dfc"

	// EventConnectionUp is up
	EventConnectionUp = "connection_up"
	// EventConnectionUnhealthy is down or degraded
	EventConnectionUnhealthy = "connection_unhealthy"

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
		OperationalStateStartingConnection,
		OperationalStateStartingRedpanda,
		OperationalStateStartingDFC,
		OperationalStateStartingFailedDFC,
		OperationalStateStartingFailedDFCMissing,
		OperationalStateIdle,
		OperationalStateActive,
		OperationalStateDegradedConnection,
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
	case OperationalStateStartingConnection,
		OperationalStateStartingRedpanda,
		OperationalStateStartingDFC,
		OperationalStateStartingFailedDFC,
		OperationalStateStartingFailedDFCMissing:
		return true
	}
	return false
}

// IsRunningState returns whether the given state is a running state
func IsRunningState(state string) bool {
	switch state {
	case OperationalStateIdle,
		OperationalStateActive,
		OperationalStateDegradedConnection,
		OperationalStateDegradedRedpanda,
		OperationalStateDegradedDFC,
		OperationalStateDegradedOther:
		return true
	}
	return false
}

// ProtocolConverterObservedState contains the observed runtime state of a ProtocolConverter instance
type ProtocolConverterObservedState struct {
	// ObservedProtocolConverterSpecConfig contains the observed ProtocolConverter service config spec with variables
	// it is here for the purpose of the UI to display the variables and the location
	ObservedProtocolConverterSpecConfig protocolconverterconfig.ConfigSpec

	// ObservedProtocolConverterRuntimeConfig contains the observed ProtocolConverter service config
	ObservedProtocolConverterRuntimeConfig protocolconverterconfig.ConfigRuntime

	// ServiceInfo contains information about the ProtocolConverter service
	ServiceInfo protocolconvertersvc.ServiceInfo
}

// IsObservedState implements the ObservedState interface
func (b ProtocolConverterObservedState) IsObservedState() {}

// ProtocolConverterInstance implements the FSMInstance interface
// If ProtocolConverterInstance does not implement the FSMInstance interface, this will
// be detected at compile time
var _ publicfsm.FSMInstance = (*ProtocolConverterInstance)(nil)

// ProtocolConverterInstance is a state-machine managed instance of a ProtocolConverter service.
type ProtocolConverterInstance struct {
	// service is the ProtocolConverter service implementation to use
	// It has a manager that manages the protocolconverter service instances
	service protocolconvertersvc.IProtocolConverterService

	baseFSMInstance *internalfsm.BaseFSMInstance

	// specConfig contains all the configuration spec for this service
	specConfig protocolconverterconfig.ConfigSpec

	// runtimeConfig is the last fully-rendered runtime configuration.
	// It is **zero-value** when the instance is first created; the real
	// configuration is rendered during the *first* Reconcile() cycle
	// once the instance has access to SystemSnapshot (agent location,
	// global variables, node name, …).
	runtimeConfig protocolconverterconfig.ConfigRuntime

	// ObservedState represents the observed state of the service
	// ObservedState contains all metrics, logs, etc.
	// that are updated at the beginning of Reconcile and then used to
	// determine the next state
	ObservedState ProtocolConverterObservedState
}

// GetLastObservedState returns the last known state of the instance
func (d *ProtocolConverterInstance) GetLastObservedState() publicfsm.ObservedState {
	return d.ObservedState
}

// SetService sets the ProtocolConverter service implementation to use
// This is a testing-only utility to access the private service field
func (d *ProtocolConverterInstance) SetService(service protocolconvertersvc.IProtocolConverterService) {
	d.service = service
}

// GetConfig returns the ProtocolConverterServiceConfig for this service
// This is a testing-only utility to access the private service field
func (d *ProtocolConverterInstance) GetConfig() protocolconverterconfig.ConfigSpec {
	return d.specConfig
}

// GetLastError returns the last error of the instance
// This is a testing-only utility to access the private baseFSMInstance field
func (d *ProtocolConverterInstance) GetLastError() error {
	return d.baseFSMInstance.GetLastError()
}

// IsTransientStreakCounterMaxed returns whether the transient streak counter
// has reached the maximum number of ticks, which means that the FSM is stuck in a state
// and should be removed
func (d *ProtocolConverterInstance) IsTransientStreakCounterMaxed() bool {
	return d.baseFSMInstance.IsTransientStreakCounterMaxed()
}
