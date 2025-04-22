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

package agent_monitor

import (
	"context"

	"github.com/looplab/fsm"
	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
)

// registerCallbacks sets up logging or side effects on entering states if desired.
func (a *AgentInstance) registerCallbacks() {
	// Operational state callbacks
	a.baseFSMInstance.AddCallback("enter_"+OperationalStateStopped, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Agent %s entered state: agent_monitoring_stopped", a.baseFSMInstance.GetID())
	})

	a.baseFSMInstance.AddCallback("enter_"+OperationalStateDegraded, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Warnf("Agent %s entered state: degraded", a.baseFSMInstance.GetID())
	})

	a.baseFSMInstance.AddCallback("enter_"+OperationalStateActive, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Agent %s entered state: active", a.baseFSMInstance.GetID())
	})

	// Lifecycle state callbacks
	a.baseFSMInstance.AddCallback("enter_"+internal_fsm.LifecycleStateToBeCreated, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Agent %s is to_be_created...", a.baseFSMInstance.GetID())
	})

	a.baseFSMInstance.AddCallback("enter_"+internal_fsm.LifecycleStateCreating, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Agent %s is creating...", a.baseFSMInstance.GetID())
	})

	a.baseFSMInstance.AddCallback("enter_"+internal_fsm.LifecycleStateRemoving, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Agent %s is removing...", a.baseFSMInstance.GetID())
	})

	a.baseFSMInstance.AddCallback("enter_"+internal_fsm.LifecycleStateRemoved, func(ctx context.Context, e *fsm.Event) {
		a.baseFSMInstance.GetLogger().Infof("Agent %s has been removed", a.baseFSMInstance.GetID())
	})
}
