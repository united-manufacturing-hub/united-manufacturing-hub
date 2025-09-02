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

package s6

import (
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/s6serviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	s6svc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6/s6_default"
)

// Operational state constants represent the runtime states of a service.
const (
	// OperationalStateStarting indicates the service is in the process of starting.
	OperationalStateStarting = "starting"
	// OperationalStateRunning indicates the service is running normally.
	OperationalStateRunning = "running"
	// OperationalStateStopping indicates the service is in the process of stopping.
	OperationalStateStopping = "stopping"
	// OperationalStateStopped indicates the service is not running.
	OperationalStateStopped = "stopped"
	// OperationalStateUnknown indicates the service status is unknown.
	OperationalStateUnknown = "unknown"
)

func IsOperationalState(state string) bool {
	switch state {
	case OperationalStateStopped,
		OperationalStateStarting,
		OperationalStateRunning,
		OperationalStateStopping,
		OperationalStateUnknown:
		return true
	default:
		return false
	}
}

// Operational event constants represent events related to service runtime states.
const (
	// EventStart is triggered to start a service.
	EventStart = "start"
	// EventStartDone is triggered when the service has started.
	EventStartDone = "start_done"
	// EventStop is triggered to stop a service.
	EventStop = "stop"
	// EventStopDone is triggered when the service has stopped.
	EventStopDone = "stop_done"
)

// S6ObservedState represents the state of the service as observed externally.
type S6ObservedState struct {

	// ObservedS6ServiceConfig contains the actual service config from s6
	ObservedS6ServiceConfig s6serviceconfig.S6ServiceConfig

	// ServiceInfo contains the actual service info from s6
	ServiceInfo s6_shared.ServiceInfo

	// LastStateChange is the timestamp of the last observed state change
	LastStateChange int64
}

// IsObservedState implements the ObservedState interface.
func (s S6ObservedState) IsObservedState() {}

// S6Instance implements the FSMInstance interface
// If S6Instance does not implement the FSMInstance interface, this will
// be detected at compile time.
var _ publicfsm.FSMInstance = (*S6Instance)(nil)

// S6Instance represents a single S6 service instance with a state machine.
type S6Instance struct {

	// service is the S6 service implementation to use
	service s6svc.Service

	baseFSMInstance *internalfsm.BaseFSMInstance

	// servicePath is the path to the s6 service directory
	servicePath string

	// config contains all the configuration for this service
	config config.S6FSMConfig

	// ObservedState represents the observed state of the service
	// ObservedState contains all metrics, logs, etc.
	// that are updated at the beginning of Reconcile and then used to
	// determine the next state
	ObservedState S6ObservedState
}

// GetError returns a structured error with backoff information.
func (s *S6Instance) GetError() error {
	return s.baseFSMInstance.GetError()
}

// GetLastObservedState returns the last known state of the instance.
func (s *S6Instance) GetLastObservedState() publicfsm.ObservedState {
	return s.ObservedState
}

// GetServicePath returns the path to the s6 service directory
// This is a testing-only utility to access the private field.
func (s *S6Instance) GetServicePath() string {
	return s.servicePath
}

// GetService returns the S6 service implementation
// This is a testing-only utility to access the private field.
func (s *S6Instance) GetService() s6svc.Service {
	return s.service
}

// SetServicePath sets the path to the s6 service directory
// This is a testing-only utility to access the private field.
func (s *S6Instance) SetServicePath(servicePath string) {
	s.servicePath = servicePath
}

// SetService sets the S6 service implementation
// This is a testing-only utility to access the private field.
func (s *S6Instance) SetService(service s6svc.Service) {
	s.service = service
}

// GetConfig returns the S6FSMConfig of the instance
// This is a testing-only utility to access the private field.
func (s *S6Instance) GetConfig() config.S6FSMConfig {
	return s.config
}

// GetMinimumRequiredTime returns the minimum required time for this instance.
func (s *S6Instance) GetMinimumRequiredTime() time.Duration {
	return constants.S6UpdateObservedStateTimeout
}

// IsTransientStreakCounterMaxed returns whether the transient streak counter
// has reached the maximum number of ticks, which means that the FSM is stuck in a state
// and should be removed.
func (s *S6Instance) IsTransientStreakCounterMaxed() bool {
	return s.baseFSMInstance.IsTransientStreakCounterMaxed()
}
