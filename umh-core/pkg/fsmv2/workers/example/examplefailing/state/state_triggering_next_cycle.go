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

// TriggeringNextCycleState is an intermediate state used to trigger the next failure cycle.
// It emits the DisconnectAction which advances the cycle counter, then transitions to Disconnected.
type TriggeringNextCycleState struct {
	BaseFailingState
}

func (s *TriggeringNextCycleState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.ExamplefailingObservedState, *snapshot.ExamplefailingDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixRunning, "triggering_next_cycle")

	// Check for stop requests first
	if snap.Observed.IsStopRequired() {
		return &TryingToStopState{}, fsmv2.SignalNone, nil
	}

	// Check if DisconnectAction has run by looking at ConnectionHealth.
	// If we're still "healthy", the disconnect action hasn't run yet - emit it.
	// If we're "no connection", the disconnect has run - transition to Disconnected.
	if snap.Observed.ConnectionHealth == "healthy" {
		// Disconnect action hasn't run yet - emit it
		return s, fsmv2.SignalNone, &action.DisconnectAction{}
	}

	// After disconnect action has run (ConnectionHealth = "no connection"),
	// transition to Disconnected which will then try to reconnect.
	return s, fsmv2.SignalNone, nil
}

func (s *TriggeringNextCycleState) String() string {
	return helpers.DeriveStateName(s)
}

func (s *TriggeringNextCycleState) Reason() string {
	return "Triggering next failure cycle for testing"
}
