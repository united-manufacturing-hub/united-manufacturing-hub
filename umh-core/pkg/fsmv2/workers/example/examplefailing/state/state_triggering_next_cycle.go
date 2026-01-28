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

func (s *TriggeringNextCycleState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningDegraded
}

func (s *TriggeringNextCycleState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExamplefailingObservedState, *snapshot.ExamplefailingDesiredState](snapAny)

	if snap.Observed.IsStopRequired() {
		return fsmv2.Result[any, any](&TryingToStopState{}, fsmv2.SignalNone, nil, "stop required, transitioning to stop state")
	}

	if snap.Observed.ConnectionHealth == "healthy" {
		return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.DisconnectAction{}, "disconnecting to trigger next failure cycle")
	}

	if snap.Observed.ConnectionHealth == "no connection" {
		return fsmv2.Result[any, any](&DisconnectedState{}, fsmv2.SignalNone, nil, "cycle triggered, connection lost")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "waiting for disconnect to complete")
}

func (s *TriggeringNextCycleState) String() string {
	return helpers.DeriveStateName(s)
}
