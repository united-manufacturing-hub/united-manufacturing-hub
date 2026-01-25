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

// ticksBeforeNextCycle is how long to stay Connected before triggering the next failure cycle.
const ticksBeforeNextCycle = 2

// ConnectedState represents the stable running state where the worker has an active connection.
type ConnectedState struct {
	BaseFailingState
}

func (s *ConnectedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExamplefailingObservedState, *snapshot.ExamplefailingDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixRunning, "connected")

	if snap.Observed.IsStopRequired() {
		return fsmv2.Result[any, any](&TryingToStopState{}, fsmv2.SignalNone, nil, "stop required, transitioning to stop state")
	}

	if snap.Observed.ConnectionHealth == "no connection" {
		return fsmv2.Result[any, any](&DisconnectedState{}, fsmv2.SignalNone, nil, "connection lost unexpectedly")
	}

	// Simulate failures: stay healthy for a few ticks, then disconnect
	if snap.Observed.ShouldFail && !snap.Observed.AllCyclesComplete {
		if snap.Observed.TicksInConnectedState >= ticksBeforeNextCycle {
			return fsmv2.Result[any, any](&TriggeringNextCycleState{}, fsmv2.SignalNone, nil, "reached tick threshold, triggering next failure cycle")
		}

		return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.IncrementTicksAction{}, "incrementing ticks in connected state")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "connected and ready, no action needed")
}

func (s *ConnectedState) String() string {
	return helpers.DeriveStateName(s)
}
