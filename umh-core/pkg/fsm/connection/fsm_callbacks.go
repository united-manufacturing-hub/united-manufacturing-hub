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
	"context"
	"fmt"

	"github.com/looplab/fsm"
)

// registerCallbacks registers common callbacks for state transitions
// These callbacks are executed synchronously and should not have any network calls or other operations that could fail.
func (instance *ConnectionInstance) registerCallbacks() {
	// Basic operational state callbacks
	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStarting, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering starting state for %s", instance.baseFSMInstance.GetID())
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStopping, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering stopping state for %s", instance.baseFSMInstance.GetID())
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStopped, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering stopped state for %s", instance.baseFSMInstance.GetID())
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateUp, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering up state for %s", instance.baseFSMInstance.GetID())
	})

	// Running phase state callbacks
	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateDown, func(ctx context.Context, e *fsm.Event) {
		failure, failed := instance.describeLastCheckFailure()
		if !failed {
			instance.baseFSMInstance.GetLogger().Infof("Entering down state for %s", instance.baseFSMInstance.GetID())

			return
		}

		instance.baseFSMInstance.GetLogger().Infof("Entering down state for %s: the connection check failed with %s", instance.baseFSMInstance.GetID(), failure)
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateDegraded, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Warnf("Entering degraded state for %s", instance.baseFSMInstance.GetID())
	})
}

func (instance *ConnectionInstance) describeLastCheckFailure() (description string, failed bool) {
	lastScan := instance.ObservedState.ServiceInfo.NmapObservedState.ServiceInfo.NmapStatus.LastScan
	if lastScan == nil {
		return "", false
	}

	dial := lastScan.Dial
	if dial == nil {
		return "", false
	}

	if dial.Failure == "" {
		return "", false
	}

	return fmt.Sprintf("%s (lookup %.0f ms, connect %.0f ms)", dial.Failure, dial.ResolveMs, dial.ConnectMs), true
}
