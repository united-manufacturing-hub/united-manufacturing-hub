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

package redpanda

import (
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	redpandaserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"

	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	redpandasvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
)

// Operational state constants (using internal_fsm compatible naming)
const (
	// OperationalStateStopped is the initial state and also the state when the service is stopped
	OperationalStateStopped = "stopped"

	// Starting phase states
	// OperationalStateStarting is the state when s6 is starting the service
	OperationalStateStarting = "starting"

	// Running phase states
	// OperationalStateIdle is the state when the service is running but not actively processing data
	OperationalStateIdle = "idle"
	// OperationalStateActive is the state when the service is running and actively processing data
	OperationalStateActive = "active"
	// OperationalStateDegraded is the state when the service is running but has encountered issues
	OperationalStateDegraded = "degraded"

	// OperationalStateStopping is the state when the service is in the process of stopping
	OperationalStateStopping = "stopping"
)

// Operational event constants
const (
	// Basic lifecycle events
	EventStart     = "start"
	EventStartDone = "start_done"
	EventStop      = "stop"
	EventStopDone  = "stop_done"

	// Starting phase events
	EventStartFailed = "start_failed"

	// Running phase events
	EventDataReceived  = "data_received"
	EventNoDataTimeout = "no_data_timeout"
	EventDegraded      = "degraded"
	EventRecovered     = "recovered"
)

// IsOperationalState returns whether the given state is a valid operational state
func IsOperationalState(state string) bool {
	switch state {
	case OperationalStateStopped,
		OperationalStateStarting,
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
	return state == OperationalStateStarting
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

// RedpandaObservedState contains the observed runtime state of a Redpanda instance
type RedpandaObservedState struct {
	// ServiceInfo contains information about the S6 service
	ServiceInfo redpandasvc.ServiceInfo

	// ObservedRedpandaServiceConfig contains the observed Redpanda service config
	ObservedRedpandaServiceConfig redpandaserviceconfig.RedpandaServiceConfig
}

// IsObservedState implements the ObservedState interface
func (b RedpandaObservedState) IsObservedState() {}

// RedpandaInstance implements the FSMInstance interface
// If RedpandaInstance does not implement the FSMInstance interface, this will
// be detected at compile time
var _ publicfsm.FSMInstance = (*RedpandaInstance)(nil)

// RedpandaInstance is a state-machine managed instance of a Redpanda service
type RedpandaInstance struct {
	baseFSMInstance *internalfsm.BaseFSMInstance

	// PreviousObservedState represents the observed state of the service
	// PreviousObservedState contains all metrics, logs, etc.
	// that are updated at the beginning of Reconcile and then used to
	// determine the next state
	PreviousObservedState RedpandaObservedState

	// service is the Redpanda service implementation to use
	// It has a manager that manages the S6 service instances
	service redpandasvc.IRedpandaService

	// config contains all the configuration for this service
	config redpandaserviceconfig.RedpandaServiceConfig

	// schemaRegistry manages JSON schema registration for data models and contracts
	schemaRegistry redpandasvc.ISchemaRegistry

	// Schema registry configuration data
	dataModels    []config.DataModelsConfig
	dataContracts []config.DataContractsConfig
	payloadShapes map[string]config.PayloadShape

	// transitionToRunningTime tracks when we first transitioned to a running state
	// This is used to ignore errors that occurred before we were fully running
	transitionToRunningTime time.Time
}

// GetLastObservedState returns the last known state of the instance
func (r *RedpandaInstance) GetLastObservedState() publicfsm.ObservedState {
	return r.PreviousObservedState
}

// SetService sets the Redpanda service implementation
// This is a testing-only utility to access the private field
func (r *RedpandaInstance) SetService(service redpandasvc.IRedpandaService) {
	r.service = service
}

// GetConfig returns the RedpandaServiceConfig of the instance
// This is a testing-only utility to access the private field
func (r *RedpandaInstance) GetConfig() redpandaserviceconfig.RedpandaServiceConfig {
	return r.config
}

// IsTransientStreakCounterMaxed returns whether the transient streak counter
// has reached the maximum number of ticks, which means that the FSM is stuck in a state
// and should be removed
func (r *RedpandaInstance) IsTransientStreakCounterMaxed() bool {
	return r.baseFSMInstance.IsTransientStreakCounterMaxed()
}

// Getter methods for schema registry configuration
func (r *RedpandaInstance) getDataModels() []config.DataModelsConfig {
	return r.dataModels
}

func (r *RedpandaInstance) getDataContracts() []config.DataContractsConfig {
	return r.dataContracts
}

func (r *RedpandaInstance) getPayloadShapes() map[string]config.PayloadShape {
	return r.payloadShapes
}
