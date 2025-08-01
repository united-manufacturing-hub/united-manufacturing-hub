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

package dataflowcomponent

import (
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	dataflowcomponentconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dataflowcomponentsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
)

// Operational state constants (using internal_fsm compatible naming)
const (
	// OperationalStateStopping is the state when the service is in the process of stopping
	OperationalStateStopping = "stopping"
	// OperationalStateStopped is the initial state and also the state when the service is stopped
	OperationalStateStopped = "stopped"

	// Starting phase states
	// OperationalStateStarting is the state when benthos starts
	OperationalStateStarting = "starting"
	// OperationalStateStartingFailed is the state when benthos failed to start
	OperationalStateStartingFailed = "starting_failed"

	// Running phase states
	// OperationalStateIdle is the state when the service is running but not actively processing data
	OperationalStateIdle = "idle"
	// OperationalStateActive is the state when the service is running and actively processing data
	OperationalStateActive = "active"
	// OperationalStateDegraded is the state when the service is running but has encountered issues
	OperationalStateDegraded = "degraded"
)

// Operational event constants
const (
	// Basic lifecycle events
	EventStart       = "start"
	EventStartDone   = "start_done"
	EventStop        = "stop"
	EventStopDone    = "stop_done"
	EventStartFailed = "start_failed"

	// Running phase events
	EventBenthosDataReceived   = "benthos_data_received"
	EventBenthosNoDataReceived = "benthos_no_data_received"
	EventBenthosDegraded       = "benthos_degraded"
	EventBenthosRecovered      = "benthos_recovered"
)

// IsOperationalState returns whether the given state is a valid operational state
func IsOperationalState(state string) bool {
	switch state {
	case OperationalStateStopped,
		OperationalStateStarting,
		OperationalStateStartingFailed,
		OperationalStateIdle,
		OperationalStateActive,
		OperationalStateDegraded,
		OperationalStateStopping:
		return true
	}
	return false
}

// IsStartingState returns whether the given state is a starting state
func IsStartingState(state string) bool {
	switch state {
	case OperationalStateStarting,
		OperationalStateStartingFailed:
		return true
	}
	return false
}

// IsRunningState returns whether the given state is a running state
func IsRunningState(state string) bool {
	switch state {
	case OperationalStateIdle,
		OperationalStateActive,
		OperationalStateDegraded:
		return true
	}
	return false
}

// DataflowComponentObservedState contains the observed runtime state of a DataflowComponent instance
type DataflowComponentObservedState struct {

	// ObservedDataflowComponentConfig contains the observed DataflowComponent service config
	ObservedDataflowComponentConfig dataflowcomponentconfig.DataflowComponentServiceConfig
	// ServiceInfo contains information about the S6 service
	ServiceInfo dataflowcomponentsvc.ServiceInfo
}

// IsObservedState implements the ObservedState interface
func (b DataflowComponentObservedState) IsObservedState() {}

// BenthosInstance implements the FSMInstance interface
// If BenthosInstance does not implement the FSMInstance interface, this will
// be detected at compile time
var _ publicfsm.FSMInstance = (*DataflowComponentInstance)(nil)

// DataflowComponentInstance is a state-machine managed instance of a DataflowComponent service.
type DataflowComponentInstance struct {

	// service is the DataflowComponent service implementation to use
	// It has a manager that manages the benthos service instances
	service dataflowcomponentsvc.IDataFlowComponentService

	baseFSMInstance *internalfsm.BaseFSMInstance

	// config contains all the configuration for this service
	config dataflowcomponentconfig.DataflowComponentServiceConfig

	// ObservedState represents the observed state of the service
	// ObservedState contains all metrics, logs, etc.
	// that are updated at the beginning of Reconcile and then used to
	// determine the next state
	ObservedState DataflowComponentObservedState
}

// GetLastObservedState returns the last known state of the instance
func (d *DataflowComponentInstance) GetLastObservedState() publicfsm.ObservedState {
	return d.ObservedState
}

// SetService sets the DataflowComponent service implementation to use
// This is a testing-only utility to access the private service field
func (d *DataflowComponentInstance) SetService(service dataflowcomponentsvc.IDataFlowComponentService) {
	d.service = service
}

// GetConfig returns the DataflowComponentServiceConfig for this service
// This is a testing-only utility to access the private service field
func (d *DataflowComponentInstance) GetConfig() dataflowcomponentconfig.DataflowComponentServiceConfig {
	return d.config
}

// GetLastError returns the last error of the instance
// This is a testing-only utility to access the private baseFSMInstance field
func (d *DataflowComponentInstance) GetLastError() error {
	return d.baseFSMInstance.GetLastError()
}

// IsTransientStreakCounterMaxed returns whether the transient streak counter
// has reached the maximum number of ticks, which means that the FSM is stuck in a state
// and should be removed
func (d *DataflowComponentInstance) IsTransientStreakCounterMaxed() bool {
	return d.baseFSMInstance.IsTransientStreakCounterMaxed()
}
