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
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
)

// Operational state constants
const (
	// OperationalStateStopped is the initial state
	OperationalStateStopped = "stopped"

	// OperationalStateTesting is the state when actively testing a connection
	OperationalStateTesting = "testing"

	// OperationalStateSuccess is the state when the connection test was successful
	OperationalStateSuccess = "success"

	// OperationalStateFailure is the state when the connection test failed
	OperationalStateFailure = "failure"

	// OperationalStateStopping is the state when stopping a connection test
	OperationalStateStopping = "stopping"
)

// Operational event constants
const (
	// Basic lifecycle events
	EventTest       = "test"
	EventTestDone   = "test_done"
	EventTestFailed = "test_failed"
	EventStop       = "stop"
	EventStopDone   = "stop_done"
)

// IsOperationalState returns whether the given state is a valid operational state
func IsOperationalState(state string) bool {
	switch state {
	case OperationalStateStopped,
		OperationalStateTesting,
		OperationalStateSuccess,
		OperationalStateFailure,
		OperationalStateStopping:
		return true
	}
	return false
}

// ConnectionConfig represents the configuration for a connection test component
type ConnectionConfig struct {
	// Basic component configuration
	Name         string `yaml:"name"`
	DesiredState string `yaml:"desiredState"`

	// Connection details
	Target string `yaml:"target"`
	Port   int    `yaml:"port"`
	Type   string `yaml:"type"`

	// Parent DataFlowComponent Name
	ParentDFC string `yaml:"parentDFC"`
}

// ConnectionObservedState contains the observed state of a connection test
type ConnectionObservedState struct {
	// Whether the last test was successful
	TestSuccessful bool

	// Results from the nmap test
	NmapResults string

	// Any error message from the last test
	LastError string

	// Last test completion time
	LastTestTime time.Time
}

// IsObservedState implements the ObservedState interface
func (c ConnectionObservedState) IsObservedState() {}

// Connection implements the FSMInstance interface
var _ publicfsm.FSMInstance = (*Connection)(nil)

// Connection is a state-machine managed instance for connection testing
type Connection struct {
	baseFSMInstance *internalfsm.BaseFSMInstance

	// ObservedState represents the observed state of the connection test
	ObservedState ConnectionObservedState

	// Config contains the configuration for this connection test
	Config ConnectionConfig
}

// GetLastObservedState returns the last known state of the connection test
func (c *Connection) GetLastObservedState() publicfsm.ObservedState {
	return c.ObservedState
}

// GetCurrentFSMState returns the current state of the FSM
func (c *Connection) GetCurrentFSMState() string {
	return c.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (c *Connection) GetDesiredFSMState() string {
	return c.baseFSMInstance.GetDesiredFSMState()
}
