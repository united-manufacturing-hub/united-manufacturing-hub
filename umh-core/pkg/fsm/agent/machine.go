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

	"github.com/looplab/fsm"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/core"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// NewAgentInstance creates a new AgentInstance with the given ID and parent core instance
func NewAgentInstance(id string, agentType string, parentCore *core.CoreInstance) *AgentInstance {
	// Use the same FSM transitions as core
	cfg := internalfsm.BaseFSMInstanceConfig{
		ID:                           id,
		DesiredFSMState:              OperationalStateMonitoringStopped,
		OperationalStateAfterCreate:  OperationalStateMonitoringStopped,
		OperationalStateBeforeRemove: OperationalStateMonitoringStopped,
		OperationalTransitions: []fsm.EventDesc{
			// Lifecycle transitions (based on the diagram)
			{Name: EventCreate, Src: []string{LifecycleStateCreate}, Dst: OperationalStateMonitoringStopped},

			// Operational state transitions (based on the diagram)
			// From monitoring_stopped, we can start monitoring and go to active
			{Name: EventStart, Src: []string{OperationalStateMonitoringStopped}, Dst: OperationalStateActive},

			// From active, we can go to degraded or stop monitoring
			{Name: EventDegraded, Src: []string{OperationalStateActive}, Dst: OperationalStateDegraded},
			{Name: EventStop, Src: []string{OperationalStateActive}, Dst: OperationalStateMonitoringStopped},

			// From degraded, we can recover to active or stop monitoring
			{Name: EventStart, Src: []string{OperationalStateDegraded}, Dst: OperationalStateActive},
			{Name: EventStop, Src: []string{OperationalStateDegraded}, Dst: OperationalStateMonitoringStopped},

			// From monitoring_stopped, we can remove the instance
			{Name: EventRemove, Src: []string{OperationalStateMonitoringStopped}, Dst: LifecycleStateRemove},
		},
	}

	instance := &AgentInstance{
		baseFSMInstance: internalfsm.NewBaseFSMInstance(cfg, logger.For(id)),
		agentID:         id,
		agentType:       agentType,
		parentCore:      parentCore,
		ObservedState: AgentObservedState{
			IsMonitoring: false,
			ErrorCount:   0,
			IsConnected:  false,
		},
	}

	instance.registerCallbacks()

	metrics.InitErrorCounter(metrics.ComponentAgentInstance, id)

	return instance
}

// registerCallbacks registers callback functions for FSM state transitions
func (a *AgentInstance) registerCallbacks() {
	// Register callbacks for state transitions
	a.baseFSMInstance.AddCallback("enter_"+OperationalStateMonitoringStopped, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Entering monitoring stopped state for agent %s", a.baseFSMInstance.GetID())
		a.ObservedState.IsMonitoring = false
	})

	a.baseFSMInstance.AddCallback("enter_"+OperationalStateActive, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Entering active state for agent %s", a.baseFSMInstance.GetID())
		a.ObservedState.IsMonitoring = true
	})

	a.baseFSMInstance.AddCallback("enter_"+OperationalStateDegraded, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Warnf("Entering degraded state for agent %s", a.baseFSMInstance.GetID())
		a.ObservedState.IsMonitoring = true
	})

	a.baseFSMInstance.AddCallback("enter_"+LifecycleStateCreate, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Creating agent instance %s", a.baseFSMInstance.GetID())
	})

	a.baseFSMInstance.AddCallback("enter_"+LifecycleStateRemove, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Removing agent instance %s", a.baseFSMInstance.GetID())
	})

	// Add callbacks for parent core state changes
	// These are used to synchronize agent state with core state
	a.baseFSMInstance.AddCallback("check_parent_state", func(ctx context.Context, e *fsm.Event) {
		parentState := a.parentCore.GetCurrentFSMState()
		a.baseFSMInstance.GetLogger().Debugf("Parent core state: %s", parentState)

		// If parent is monitoring_stopped, agent should also be monitoring_stopped
		if parentState == core.OperationalStateMonitoringStopped && a.GetCurrentFSMState() != OperationalStateMonitoringStopped {
			a.baseFSMInstance.GetLogger().Infof("Parent core is stopped, stopping agent %s", a.baseFSMInstance.GetID())
			// This doesn't directly trigger the event but sets the desired state
			// Reconcile will handle the actual transition
			a.SetDesiredFSMState(OperationalStateMonitoringStopped)
		}

		// If parent is removed, agent should also be removed
		if a.parentCore.IsRemoved() && !a.IsRemoved() {
			a.baseFSMInstance.GetLogger().Infof("Parent core is removed, removing agent %s", a.baseFSMInstance.GetID())
			// Again, just set desired state, don't trigger event directly
			a.Remove(ctx)
		}
	})
}
