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

package dfc

import (
	"context"

	"github.com/looplab/fsm"
)

// registerCallbacks sets up the callbacks for state machine events
// These callbacks are executed synchronously and should not have any network calls or other operations that could fail
func (d *DataFlowComponent) registerCallbacks() {
	d.baseFSMInstance.AddCallback("enter_"+OperationalStateStarting, func(ctx context.Context, e *fsm.Event) {
		d.baseFSMInstance.GetLogger().Infof("DataFlowComponent %s entering state Starting", d.Config.Name)
	})

	d.baseFSMInstance.AddCallback("enter_"+OperationalStateActive, func(ctx context.Context, e *fsm.Event) {
		d.baseFSMInstance.GetLogger().Infof("DataFlowComponent %s entering state Active", d.Config.Name)
	})

	d.baseFSMInstance.AddCallback("enter_"+OperationalStateDegraded, func(ctx context.Context, e *fsm.Event) {
		d.baseFSMInstance.GetLogger().Infof("DataFlowComponent %s entering state Degraded", d.Config.Name)
	})

	d.baseFSMInstance.AddCallback("enter_"+OperationalStateStopping, func(ctx context.Context, e *fsm.Event) {
		d.baseFSMInstance.GetLogger().Infof("DataFlowComponent %s entering state Stopping", d.Config.Name)
	})

	d.baseFSMInstance.AddCallback("enter_"+OperationalStateStopped, func(ctx context.Context, e *fsm.Event) {
		d.baseFSMInstance.GetLogger().Infof("DataFlowComponent %s entering state Stopped", d.Config.Name)
	})
}

// handleEnterStarting handles the transition to the Starting state
func (d *DataFlowComponent) handleEnterStarting(e *fsm.Event) {
	d.baseFSMInstance.GetLogger().Infof("DataFlowComponent %s entering state Starting", d.Config.Name)
}

// handleEnterActive handles the transition to the Active state
func (d *DataFlowComponent) handleEnterActive(e *fsm.Event) {
	d.baseFSMInstance.GetLogger().Infof("DataFlowComponent %s entering state Active", d.Config.Name)
}

// handleEnterDegraded handles the transition to the Degraded state
func (d *DataFlowComponent) handleEnterDegraded(e *fsm.Event) {
	d.baseFSMInstance.GetLogger().Infof("DataFlowComponent %s entering state Degraded", d.Config.Name)
}

// handleEnterStopping handles the transition to the Stopping state
func (d *DataFlowComponent) handleEnterStopping(e *fsm.Event) {
	d.baseFSMInstance.GetLogger().Infof("DataFlowComponent %s entering state Stopping", d.Config.Name)
}

// handleEnterStopped handles the transition to the Stopped state
func (d *DataFlowComponent) handleEnterStopped(e *fsm.Event) {
	d.baseFSMInstance.GetLogger().Infof("DataFlowComponent %s entering state Stopped", d.Config.Name)
}
