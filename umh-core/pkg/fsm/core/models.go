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

package core

import (
	"context"
	"fmt"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
)

// Lifecycle state constants
const (
	// LifecycleStateCreate is the state when the instance is being created
	LifecycleStateCreate = "livecycle_create"
	// LifecycleStateRemove is the state when the instance is being removed
	LifecycleStateRemove = "livecycle_remove"
)

// Operational state constants
const (
	// OperationalStateMonitoringStopped is the state when monitoring is stopped
	OperationalStateMonitoringStopped = "monitoring_stopped"
	// OperationalStateActive is the state when the component is active and functioning properly
	OperationalStateActive = "active"
	// OperationalStateDegraded is the state when the component is running but has issues
	OperationalStateDegraded = "degraded"
)

// Event constants for state transitions
const (
	// Lifecycle events
	EventCreate = "create"
	EventRemove = "remove"

	// Operational events
	EventStart    = "event_start"
	EventStop     = "event_stop"
	EventDegraded = "degraded"
)

// IsOperationalState returns whether the given state is an operational state
func IsOperationalState(state string) bool {
	switch state {
	case OperationalStateMonitoringStopped,
		OperationalStateActive,
		OperationalStateDegraded:
		return true
	}
	return false
}

// IsLifecycleState returns whether the given state is a lifecycle state
func IsLifecycleState(state string) bool {
	switch state {
	case LifecycleStateCreate,
		LifecycleStateRemove:
		return true
	}
	return false
}

// CoreObservedState contains the observed runtime state of a Core instance
type CoreObservedState struct {
	// Add relevant fields based on what's being monitored
	IsMonitoring  bool
	ErrorCount    int
	LastErrorTime int64
}

// IsObservedState implements the ObservedState interface
func (c CoreObservedState) IsObservedState() {}

// CoreInstance implements the FSMInstance interface
// If CoreInstance does not implement the FSMInstance interface, this will
// be detected at compile time
var _ publicfsm.FSMInstance = (*CoreInstance)(nil)

// CoreInstance is a state-machine managed instance of a Core component
type CoreInstance struct {
	baseFSMInstance *internalfsm.BaseFSMInstance

	// ObservedState represents the observed state of the component
	// ObservedState contains all metrics, logs, etc.
	// that are updated at the beginning of Reconcile and then used to
	// determine the next state
	ObservedState CoreObservedState

	// Add any additional fields needed for the core component
	componentName string
	componentID   string
}

// GetLastObservedState returns the last known state of the instance
func (c *CoreInstance) GetLastObservedState() publicfsm.ObservedState {
	return c.ObservedState
}

// SetDesiredFSMState safely updates the desired state
func (c *CoreInstance) SetDesiredFSMState(state string) error {
	// For Core, we only allow setting MonitoringStopped or Active as desired states
	if state != OperationalStateMonitoringStopped &&
		state != OperationalStateActive {
		return fmt.Errorf("invalid desired state: %s. valid states are %s and %s",
			state,
			OperationalStateMonitoringStopped,
			OperationalStateActive)
	}

	c.baseFSMInstance.SetDesiredFSMState(state)
	return nil
}

// GetCurrentFSMState returns the current state of the FSM
func (c *CoreInstance) GetCurrentFSMState() string {
	return c.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (c *CoreInstance) GetDesiredFSMState() string {
	return c.baseFSMInstance.GetDesiredFSMState()
}

// Remove starts the removal process
func (c *CoreInstance) Remove(ctx context.Context) error {
	return c.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (c *CoreInstance) IsRemoved() bool {
	return c.baseFSMInstance.IsRemoved()
}

// IsMonitoringStopped returns true if the instance is in the monitoring_stopped state
func (c *CoreInstance) IsMonitoringStopped() bool {
	return c.baseFSMInstance.GetCurrentFSMState() == OperationalStateMonitoringStopped
}

// IsActive returns true if the instance is in the active state
func (c *CoreInstance) IsActive() bool {
	return c.baseFSMInstance.GetCurrentFSMState() == OperationalStateActive
}

// IsDegraded returns true if the instance is in the degraded state
func (c *CoreInstance) IsDegraded() bool {
	return c.baseFSMInstance.GetCurrentFSMState() == OperationalStateDegraded
}

// PrintState prints the current state of the FSM for debugging
func (c *CoreInstance) PrintState() {
	c.baseFSMInstance.GetLogger().Debugf("Current state: %s", c.baseFSMInstance.GetCurrentFSMState())
	c.baseFSMInstance.GetLogger().Debugf("Desired state: %s", c.baseFSMInstance.GetDesiredFSMState())
	c.baseFSMInstance.GetLogger().Debugf("Observed state: %+v", c.ObservedState)
}
