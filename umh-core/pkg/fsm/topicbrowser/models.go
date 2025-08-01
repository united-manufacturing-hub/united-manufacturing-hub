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

package topicbrowser

import (
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/topicbrowserserviceconfig"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	topicbrowsersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
)

// Operational state constants (using internal_fsm compatible naming)
const (
	// OperationalStateStopped is the initial state and also the state when the service is stopped
	OperationalStateStopped = "stopped"

	// Starting phase states
	// OperationalStateStarting is the state when s6 is starting the service
	OperationalStateStarting = "starting"
	// OperationalStateStartingBenthos is the state when s6 is starting benthos
	OperationalStateStartingBenthos = "starting_benthos"
	// OperationalStateStartingRedpanda is the state when s6 is starting redpanda
	OperationalStateStartingRedpanda = "starting_redpanda"

	// Running phase states
	// OperationalStateIdle is the state when the service is running but not actively processing data
	OperationalStateIdle = "idle"
	// OperationalStateActive is the state when the service is running and actively processing data
	OperationalStateActive = "active"
	// OperationalStateDegradedBenthos is the state when the service is running but benthos has encountered issues
	OperationalStateDegradedBenthos = "degraded_benthos"
	// OperationalStateDegradedRedpanda is the state when the service is running but redpanda has encountered issues
	OperationalStateDegradedRedpanda = "degraded_redpanda"

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
	EventBenthosStarted  = "benthos_started"
	EventRedpandaStarted = "redpanda_started"

	// Running phase events
	EventDataReceived     = "data_received"
	EventNoDataTimeout    = "no_data_timeout"
	EventBenthosDegraded  = "benthos_degraded"
	EventRedpandaDegraded = "redpanda_degraded"
	EventRecovered        = "recovered"

	// Startup failure event
	EventStartupFailed = "startup_failed"
)

// IsOperationalState returns whether the given state is a valid operational state
func IsOperationalState(state string) bool {
	switch state {
	case OperationalStateStopped,
		OperationalStateStarting,
		OperationalStateStartingBenthos,
		OperationalStateStartingRedpanda,
		OperationalStateIdle,
		OperationalStateActive,
		OperationalStateDegradedBenthos,
		OperationalStateDegradedRedpanda,
		OperationalStateStopping:
		return true
	}
	return false
}

// IsStartingState returns whether the given state is a starting state
func IsStartingState(state string) bool {
	switch state {
	case OperationalStateStarting,
		OperationalStateStartingBenthos,
		OperationalStateStartingRedpanda:
		return true
	}
	return false
}

// IsRunningState returns whether the given state is a running state
func IsRunningState(state string) bool {
	switch state {
	case OperationalStateIdle,
		OperationalStateActive,
		OperationalStateDegradedBenthos,
		OperationalStateDegradedRedpanda:
		return true
	}
	return false
}

// ObservedState contains the observed runtime state of a Benthos instance
type ObservedState struct {

	// ObservedServiceConfig contains the observed Benthos service config
	ObservedServiceConfig topicbrowserserviceconfig.Config
	// ServiceInfo contains information about the S6 service
	ServiceInfo topicbrowsersvc.ServiceInfo
}

// IsObservedState implements the ObservedState interface
func (o ObservedState) IsObservedState() {}

// BenthosInstance implements the FSMInstance interface
// If BenthosInstance does not implement the FSMInstance interface, this will
// be detected at compile time
var _ publicfsm.FSMInstance = (*TopicBrowserInstance)(nil)

// TopicBrowserInstance is a state-machine managed instance of a Topic Browser service
type TopicBrowserInstance struct {

	// service is the Benthos service implementation to use
	// It has a manager that manages the S6 service instances
	service topicbrowsersvc.ITopicBrowserService

	baseFSMInstance *internalfsm.BaseFSMInstance

	// config contains all the configuration for this service
	config topicbrowserserviceconfig.Config

	// ObservedState represents the observed state of the service
	// ObservedState contains all metrics, logs, etc.
	// that are updated at the beginning of Reconcile and then used to
	// determine the next state
	ObservedState ObservedState
}

// GetLastObservedState returns the last known state of the instance
func (i *TopicBrowserInstance) GetLastObservedState() publicfsm.ObservedState {
	return i.ObservedState
}

// SetService sets the Topic Browser service implementation
// This is a testing-only utility to access the private field
func (i *TopicBrowserInstance) SetService(service topicbrowsersvc.ITopicBrowserService) {
	i.service = service
}

// GetConfig returns the ServiceConfig of the instance
// This is a testing-only utility to access the private field
func (i *TopicBrowserInstance) GetConfig() topicbrowserserviceconfig.Config {
	return i.config
}

// GetLastError returns the last error of the instance
// This is a testing-only utility to access the private baseFSMInstance field
func (i *TopicBrowserInstance) GetLastError() error {
	return i.baseFSMInstance.GetLastError()
}

// IsTransientStreakCounterMaxed returns whether the transient streak counter
// has reached the maximum number of ticks, which means that the FSM is stuck in a state
// and should be removed
func (i *TopicBrowserInstance) IsTransientStreakCounterMaxed() bool {
	return i.baseFSMInstance.IsTransientStreakCounterMaxed()
}
