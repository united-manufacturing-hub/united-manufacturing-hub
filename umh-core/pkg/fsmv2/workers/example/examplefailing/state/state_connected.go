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

package state

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/snapshot"
)

// ticksBeforeNextCycle is the number of ticks to stay in Connected state
// before triggering the next failure cycle. This allows the parent FSM
// to observe the healthy state before children fail again.
const ticksBeforeNextCycle = 2

// ConnectedState represents the stable running state where the worker has an active connection.
type ConnectedState struct {
	BaseFailingState
}

func (s *ConnectedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.ExamplefailingObservedState, *snapshot.ExamplefailingDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixRunning, "connected")

	// Check via Observed.IsStopRequired() since ParentMappedState is injected by collector
	// into the embedded DesiredState within ObservedState.
	if snap.Observed.IsStopRequired() {
		return &TryingToStopState{}, fsmv2.SignalNone, nil
	}

	// Check if we lost the connection
	if snap.Observed.ConnectionHealth == "no connection" {
		return &DisconnectedState{}, fsmv2.SignalNone, nil
	}

	// If we're simulating failures and have more cycles to complete,
	// stay healthy for a few ticks then simulate disconnection.
	// This allows parent FSM to observe healthy children before next failure cycle.
	if snap.Observed.ShouldFail && !snap.Observed.AllCyclesComplete {
		if snap.Observed.TicksInConnectedState >= ticksBeforeNextCycle {
			// Time to trigger the next failure cycle.
			// Go through TriggeringNextCycleState which will run DisconnectAction
			// to advance the cycle counter before transitioning to Disconnected.
			return &TriggeringNextCycleState{}, fsmv2.SignalNone, nil
		}
		// Still in Connected state - increment tick counter
		return s, fsmv2.SignalNone, &action.IncrementTicksAction{}
	}

	return s, fsmv2.SignalNone, nil
}

func (s *ConnectedState) String() string {
	return helpers.DeriveStateName(s)
}

func (s *ConnectedState) Reason() string {
	return "Successfully connected and ready"
}
