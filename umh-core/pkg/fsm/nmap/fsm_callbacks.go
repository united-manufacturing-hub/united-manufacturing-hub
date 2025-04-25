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

package nmap

import (
	"context"

	"github.com/looplab/fsm"
	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
)

// registerCallbacks sets up logging or side effects on entering states if desired.
func (n *NmapInstance) registerCallbacks() {
	// Operational state callbacks
	n.baseFSMInstance.AddCallback("enter_"+OperationalStateStopped, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Infof("Nmap %s entered state: stopped", n.baseFSMInstance.GetID())
	})

	n.baseFSMInstance.AddCallback("enter_"+OperationalStateDegraded, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Warnf("Nmap %s entered state: degraded", n.baseFSMInstance.GetID())
	})

	n.baseFSMInstance.AddCallback("enter_"+OperationalStateClosed, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Infof("Nmap %s entered state: closed", n.baseFSMInstance.GetID())
	})

	n.baseFSMInstance.AddCallback("enter_"+OperationalStateFiltered, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Infof("Nmap %s entered state: filtered", n.baseFSMInstance.GetID())
	})

	n.baseFSMInstance.AddCallback("enter_"+OperationalStateOpen, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Infof("Nmap %s entered state: open", n.baseFSMInstance.GetID())
	})

	n.baseFSMInstance.AddCallback("enter_"+OperationalStateUnfiltered, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Infof("Nmap %s entered state: unfiltered", n.baseFSMInstance.GetID())
	})

	n.baseFSMInstance.AddCallback("enter_"+OperationalStateOpenFiltered, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Infof("Nmap %s entered state: open_filtered", n.baseFSMInstance.GetID())
	})

	n.baseFSMInstance.AddCallback("enter_"+OperationalStateClosedFiltered, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Infof("Nmap %s entered state: closed_filtered", n.baseFSMInstance.GetID())
	})

	// Lifecycle state callbacks
	n.baseFSMInstance.AddCallback("enter_"+internal_fsm.LifecycleStateToBeCreated, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Infof("Nmap %s is to_be_created...", n.baseFSMInstance.GetID())
	})

	n.baseFSMInstance.AddCallback("enter_"+internal_fsm.LifecycleStateCreating, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Infof("Nmap %s is creating...", n.baseFSMInstance.GetID())
	})

	n.baseFSMInstance.AddCallback("enter_"+internal_fsm.LifecycleStateRemoving, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Infof("Nmap %s is removing...", n.baseFSMInstance.GetID())
	})

	n.baseFSMInstance.AddCallback("enter_"+internal_fsm.LifecycleStateRemoved, func(ctx context.Context, e *fsm.Event) {
		n.baseFSMInstance.GetLogger().Infof("Nmap %s has been removed", n.baseFSMInstance.GetID())
	})
}
