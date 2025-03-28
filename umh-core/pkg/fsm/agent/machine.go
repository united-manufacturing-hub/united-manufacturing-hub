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
	"time"

	"github.com/looplab/fsm"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	agent_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent"
)

// Component name for metrics and logging
const (
	ComponentAgentInstance = "agent_instance"
)

// State constants for the agent FSM
const (
	// Lifecycle states (taken from internal_fsm)
	LifecycleStateCreate = "lifecycle_create"
	LifecycleStateRemove = "lifecycle_remove"

	// Operational states
	OperationalStateMonitoringStopped = "monitoring_stopped"
	OperationalStateDegraded          = "degraded"
	OperationalStateActive            = "active"
)

// Event constants for the agent FSM
const (
	// Operational events
	EventStart   = "eventStart"
	EventStop    = "eventStop"
	EventDegrade = "eventDegrade"
	EventRecover = "eventRecover"
)

// AgentInstance represents an FSM for an agent
type AgentInstance struct {
	baseFSMInstance *internal_fsm.BaseFSMInstance
	service         agent_service.Service
	config          config.AgentConfig
	configManager   config.ConfigManager
	ObservedState   AgentObservedState
}

// AgentObservedState contains the last observed state of the agent
type AgentObservedState struct {
	AgentInfo     agent_service.AgentInfo
	LastUpdatedAt time.Time
}

// NewAgentInstance creates a new AgentInstance
func NewAgentInstance(initialConfig config.AgentConfig, configManager config.ConfigManager) *AgentInstance {
	cfg := internal_fsm.BaseFSMInstanceConfig{
		ID:                           "agent",
		DesiredFSMState:              OperationalStateMonitoringStopped,
		OperationalStateAfterCreate:  OperationalStateMonitoringStopped,
		OperationalStateBeforeRemove: OperationalStateMonitoringStopped,
		OperationalTransitions: []fsm.EventDesc{
			// Basic operational transitions - based on the diagram
			{Name: EventStart, Src: []string{OperationalStateMonitoringStopped}, Dst: OperationalStateActive},
			{Name: EventStop, Src: []string{OperationalStateActive, OperationalStateDegraded}, Dst: OperationalStateMonitoringStopped},

			// Degraded state transitions
			{Name: EventDegrade, Src: []string{OperationalStateActive}, Dst: OperationalStateDegraded},
			{Name: EventRecover, Src: []string{OperationalStateDegraded}, Dst: OperationalStateActive},
		},
	}

	instance := &AgentInstance{
		baseFSMInstance: internal_fsm.NewBaseFSMInstance(cfg, logger.For("agent-fsm")),
		service:         agent_service.NewDefaultService(),
		config:          initialConfig,
		configManager:   configManager,
		ObservedState: AgentObservedState{
			AgentInfo: agent_service.AgentInfo{
				Status:         agent_service.AgentStatusUnknown,
				Location:       make(map[int]string),
				ReleaseChannel: "",
				LastChangedAt:  time.Time{},
			},
			LastUpdatedAt: time.Time{},
		},
	}

	// Register callbacks for state transitions
	instance.registerCallbacks()

	metrics.InitErrorCounter(ComponentAgentInstance, "agent")

	return instance
}

// SetDesiredFSMState safely updates the desired state
func (a *AgentInstance) SetDesiredFSMState(state string) error {
	// Based on the diagram, we only allow setting MonitoringStopped or Active as desired states
	if state != OperationalStateMonitoringStopped && state != OperationalStateActive {
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

// IsRemoving returns true if the instance is in the removing state
func (a *AgentInstance) IsRemoving() bool {
	return a.baseFSMInstance.IsRemoving()
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
}
