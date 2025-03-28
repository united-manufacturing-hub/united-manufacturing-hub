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
	"time"

	"github.com/looplab/fsm"

	agent_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent"
)

// registerCallbacks registers common callbacks for state transitions
// These callbacks should be lightweight and cannot fail
func (a *AgentInstance) registerCallbacks() {
	// Basic state transition logging callbacks
	a.baseFSMInstance.AddCallback("enter_"+OperationalStateMonitoringStopped, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Agent entered monitoring_stopped state")
		// Update agent status when entering stopped state
		if a.ObservedState.AgentInfo.Status != agent_service.AgentStatusStopped {
			a.ObservedState.AgentInfo.Status = agent_service.AgentStatusStopped
			a.ObservedState.AgentInfo.LastChangedAt = time.Now()
		}
	})

	a.baseFSMInstance.AddCallback("enter_"+OperationalStateActive, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Agent entered active state")
		// Update agent status when entering active state
		if a.ObservedState.AgentInfo.Status != agent_service.AgentStatusRunning {
			a.ObservedState.AgentInfo.Status = agent_service.AgentStatusRunning
			a.ObservedState.AgentInfo.LastChangedAt = time.Now()
		}
	})

	a.baseFSMInstance.AddCallback("enter_"+OperationalStateDegraded, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Warnf("Agent entered degraded state")
		// Update agent status when entering degraded state
		if a.ObservedState.AgentInfo.Status != agent_service.AgentStatusError {
			a.ObservedState.AgentInfo.Status = agent_service.AgentStatusError
			a.ObservedState.AgentInfo.LastChangedAt = time.Now()
		}
	})

	// Lifecycle state callbacks
	a.baseFSMInstance.AddCallback("enter_"+LifecycleStateCreate, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Agent entered lifecycle_create state")
	})

	a.baseFSMInstance.AddCallback("enter_"+LifecycleStateRemove, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Agent entered lifecycle_remove state")
	})

	// Event callbacks
	a.baseFSMInstance.AddCallback("before_"+EventStart, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Debugf("Starting agent monitoring...")
	})

	a.baseFSMInstance.AddCallback("before_"+EventStop, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Debugf("Stopping agent monitoring...")
	})

	a.baseFSMInstance.AddCallback("before_"+EventDegrade, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Debugf("Agent monitoring is degrading...")
	})

	a.baseFSMInstance.AddCallback("before_"+EventRecover, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Debugf("Agent monitoring is recovering...")
	})
}
