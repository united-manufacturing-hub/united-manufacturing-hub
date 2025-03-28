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

package agent

import (
	"context"
	"fmt"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	publicfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/core"
	agentservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent"
)

// We use the same state constants as core to ensure compatibility
const (
	// Lifecycle states
	LifecycleStateCreate = core.LifecycleStateCreate
	LifecycleStateRemove = core.LifecycleStateRemove

	// Operational states
	OperationalStateMonitoringStopped = core.OperationalStateMonitoringStopped
	OperationalStateActive            = core.OperationalStateActive
	OperationalStateDegraded          = core.OperationalStateDegraded

	// Events - reuse core events
	EventCreate   = core.EventCreate
	EventRemove   = core.EventRemove
	EventStart    = core.EventStart
	EventStop     = core.EventStop
	EventDegraded = core.EventDegraded
)

// IsOperationalState returns whether the given state is a valid operational state
func IsOperationalState(state string) bool {
	return core.IsOperationalState(state)
}

// IsLifecycleState returns whether the given state is a lifecycle state
func IsLifecycleState(state string) bool {
	return core.IsLifecycleState(state)
}

// AgentObservedState contains the observed runtime state of an Agent instance
type AgentObservedState struct {
	// Add relevant fields based on what's being monitored
	IsMonitoring  bool
	ErrorCount    int
	LastErrorTime int64

	// Agent-specific observed state
	LastHeartbeat int64
	IsConnected   bool

	// Location information from config
	Location map[int]string
}

// IsObservedState implements the ObservedState interface
func (a AgentObservedState) IsObservedState() {}

// AgentInstance implements the FSMInstance interface
// If AgentInstance does not implement the FSMInstance interface, this will
// be detected at compile time
var _ publicfsm.FSMInstance = (*AgentInstance)(nil)

// AgentInstance is a state-machine managed instance of an Agent component
// that is owned by a core instance
type AgentInstance struct {
	baseFSMInstance *internalfsm.BaseFSMInstance

	// ObservedState represents the observed state of the component
	ObservedState AgentObservedState

	// Agent-specific fields
	agentID   string
	agentType string

	// Reference to the parent CoreInstance that owns this agent
	parentCore *core.CoreInstance

	// Agent service for monitoring and controlling the agent
	service agentservice.Agent
}

// GetLastObservedState returns the last known state of the instance
func (a *AgentInstance) GetLastObservedState() publicfsm.ObservedState {
	return a.ObservedState
}

// SetDesiredFSMState safely updates the desired state
func (a *AgentInstance) SetDesiredFSMState(state string) error {
	// For Agent, we only allow setting MonitoringStopped or Active as desired states
	if state != OperationalStateMonitoringStopped &&
		state != OperationalStateActive {
		return fmt.Errorf("invalid desired state: %s. valid states are %s and %s",
			state,
			OperationalStateMonitoringStopped,
			OperationalStateActive)
	}

	a.baseFSMInstance.SetDesiredFSMState(state)
	return nil
}

// GetCurrentFSMState returns the current state of the FSM
func (a *AgentInstance) GetCurrentFSMState() string {
	return a.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (a *AgentInstance) GetDesiredFSMState() string {
	return a.baseFSMInstance.GetDesiredFSMState()
}

// Remove starts the removal process
func (a *AgentInstance) Remove(ctx context.Context) error {
	return a.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (a *AgentInstance) IsRemoved() bool {
	return a.baseFSMInstance.IsRemoved()
}

// GetAgentID returns the agent ID
func (a *AgentInstance) GetAgentID() string {
	return a.agentID
}

// GetAgentType returns the agent type
func (a *AgentInstance) GetAgentType() string {
	return a.agentType
}

// GetParentCore returns the parent core instance
func (a *AgentInstance) GetParentCore() *core.CoreInstance {
	return a.parentCore
}

// IsMonitoringStopped returns true if the instance is in the monitoring_stopped state
func (a *AgentInstance) IsMonitoringStopped() bool {
	return a.baseFSMInstance.GetCurrentFSMState() == OperationalStateMonitoringStopped
}

// IsActive returns true if the instance is in the active state
func (a *AgentInstance) IsActive() bool {
	return a.baseFSMInstance.GetCurrentFSMState() == OperationalStateActive
}

// IsDegraded returns true if the instance is in the degraded state
func (a *AgentInstance) IsDegraded() bool {
	return a.baseFSMInstance.GetCurrentFSMState() == OperationalStateDegraded
}

// PrintState prints the current state of the FSM for debugging
func (a *AgentInstance) PrintState() {
	a.baseFSMInstance.GetLogger().Debugf("Current state: %s", a.baseFSMInstance.GetCurrentFSMState())
	a.baseFSMInstance.GetLogger().Debugf("Desired state: %s", a.baseFSMInstance.GetDesiredFSMState())
	a.baseFSMInstance.GetLogger().Debugf("Observed state: %+v", a.ObservedState)
	a.baseFSMInstance.GetLogger().Debugf("Parent core: %s", a.parentCore.GetCurrentFSMState())

	// Log location information
	if len(a.ObservedState.Location) > 0 {
		a.baseFSMInstance.GetLogger().Debugf("Location data: %v", a.ObservedState.Location)
	} else {
		a.baseFSMInstance.GetLogger().Debugf("No location data available")
	}
}
