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

package connection

import (
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
)

// Operational state constants (using internal_fsm compatible naming)
const (
	// OperationalStateStopping is the state when the service is in the process of stopping
	OperationalStateStopping = "stopping"
	// OperationalStateStopped is the initial state and also the state when the service is stopped
	OperationalStateStopped = "stopped"

	// Starting phase states
	// OperationalStateStarting is the state when connection builds up
	OperationalStateStarting = "starting"

	// Running phase states
	// OperationalStateDown is the state when the service is running but not actively processing data
	OperationalStateDown = "down"
	// OperationalStateUp is the state when the service is running and actively processing data
	OperationalStateUp = "up"
	// OperationalStateDegraded is the state when the service is running but has encountered issues
	OperationalStateDegraded = "degraded"
)

// Operational event constants
const (
	// Basic lifecycle events
	EventStart     = "start"
	EventStartDone = "start_done"
	EventStop      = "stop"
	EventStopDone  = "stop_done"

	EventProbeUp    = "probe_up"
	EventProbeFlaky = "probe_flaky"
	EventProbeDown  = "probe_down"

	// Running phase events
)

// IsOperationalState returns whether the given state is a valid operational state
func IsOperationalState(state string) bool {
	switch state {
	case OperationalStateStopped,
		OperationalStateStarting,
		OperationalStateDown,
		OperationalStateUp,
		OperationalStateDegraded,
		OperationalStateStopping:
		return true
	}
	return false
}

// IsStartingState returns whether the given state is a starting state
func IsStartingState(state string) bool {
	switch state {
	case OperationalStateStarting:
		return true
	}
	return false
}

// IsRunningState returns whether the given state is a running state
func IsRunningState(state string) bool {
	switch state {
	case OperationalStateDown,
		OperationalStateUp,
		OperationalStateDegraded:
		return true
	}
	return false
}

// ConnectionObservedState contains the observed runtime state of a Connection instance
type ConnectionObservedState struct {

	// ObservedConnectionConfig contains the observed Connection service config
	ObservedConnectionConfig connectionserviceconfig.ConnectionServiceConfig
	// ServiceInfo contains information about the S6 service
	ServiceInfo connection.ServiceInfo
}

// IsObservedState implements the ObservedState interface
func (c ConnectionObservedState) IsObservedState() {}

// BenthosInstance implements the FSMInstance interface
// If BenthosInstance does not implement the FSMInstance interface, this will
// be detected at compile time
var _ publicfsm.FSMInstance = (*ConnectionInstance)(nil)

// ConnectionInstance is a state-machine managed instance of a Connection service.
type ConnectionInstance struct {

	// service is the Connection service implementation to use
	// It has a manager that manages the benthos service instances
	service connection.IConnectionService

	baseFSMInstance *internalfsm.BaseFSMInstance

	// config contains all the configuration for this service
	config connectionserviceconfig.ConnectionServiceConfig

	// ObservedState represents the observed state of the service
	// ObservedState contains all metrics, logs, etc.
	// that are updated at the beginning of Reconcile and then used to
	// determine the next state
	ObservedState ConnectionObservedState
}

// GetLastObservedState returns the last known state of the instance
func (c *ConnectionInstance) GetLastObservedState() publicfsm.ObservedState {
	return c.ObservedState
}

// SetService sets the Connection service implementation to use
// This is a testing-only utility to access the private service field
func (c *ConnectionInstance) SetService(service connection.IConnectionService) {
	c.service = service
}

// GetConfig returns the ConnectionServiceConfig for this service
// This is a testing-only utility to access the private service field
func (c *ConnectionInstance) GetConfig() connectionserviceconfig.ConnectionServiceConfig {
	return c.config
}

// GetLastError returns the last error of the instance
// This is a testing-only utility to access the private baseFSMInstance field
func (c *ConnectionInstance) GetLastError() error {
	return c.baseFSMInstance.GetLastError()
}

// IsTransientStreakCounterMaxed returns whether the transient streak counter
// has reached the maximum number of ticks, which means that the FSM is stuck in a state
// and should be removed
func (c *ConnectionInstance) IsTransientStreakCounterMaxed() bool {
	return c.baseFSMInstance.IsTransientStreakCounterMaxed()
}
