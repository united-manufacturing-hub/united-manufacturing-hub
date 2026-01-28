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

// healthyDurationMsBeforeNextCycle is wall-clock time (in ms) to stay Connected before triggering
// the next failure cycle. Using wall-clock time instead of tick counts ensures deterministic
// timing regardless of goroutine scheduling or observation collection delays.
//
// This duration must be long enough for the parent supervisor to:
// 1. Observe children as healthy (within next tick after child enters Connected)
// 2. Transition from TryingToStart to Running
// 3. Remain in Running long enough to observe children become unhealthy in the next cycle
//
// 5000ms (5 seconds) provides sufficient margin for parent observation, accounting for:
// - 1-tick delay (parent observes children's previous tick state)
// - Action-observation gating delays
// - Collector interval (up to 1 second for periodic observations)
// - Multiple observation cycles needed for parent to see consistent healthy state.
const healthyDurationMsBeforeNextCycle = 5000

// ConnectedState represents the stable running state where the worker has an active connection.
type ConnectedState struct {
	BaseFailingState
}

func (s *ConnectedState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

func (s *ConnectedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExamplefailingObservedState, *snapshot.ExamplefailingDesiredState](snapAny)

	if snap.Observed.IsStopRequired() {
		return fsmv2.Result[any, any](&TryingToStopState{}, fsmv2.SignalNone, nil, "stop required, transitioning to stop state")
	}

	if snap.Observed.ConnectionHealth == "no connection" {
		return fsmv2.Result[any, any](&DisconnectedState{}, fsmv2.SignalNone, nil, "connection lost unexpectedly")
	}

	// Simulate failures: stay healthy for a deterministic wall-clock duration, then disconnect.
	// Using TimeInCurrentStateMs (from framework metrics) ensures timing is independent of
	// tick frequency or goroutine scheduling variations.
	//
	// IMPORTANT: We return a TriggerObservationAction even though we use wall-clock
	// time for the transition decision. The action triggers immediate observation via the
	// collector's OnActionComplete callback, which is crucial for parent supervisors to observe
	// this child as healthy. Without an action, observations only happen every 1 second
	// (DefaultObservationInterval), which could cause the parent to miss the healthy window.
	if snap.Observed.ShouldFail && !snap.Observed.AllCyclesComplete {
		timeInStateMs := snap.Observed.Metrics.Framework.TimeInCurrentStateMs
		if timeInStateMs >= healthyDurationMsBeforeNextCycle {
			return fsmv2.Result[any, any](&TriggeringNextCycleState{}, fsmv2.SignalNone, nil, "reached healthy duration threshold, triggering next failure cycle")
		}

		// Use TriggerObservationAction to trigger immediate observation (even though we use wall-clock
		// time for the transition decision). This ensures parent observes child health promptly.
		return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.TriggerObservationAction{}, "waiting for healthy duration, triggering observation")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "connected and ready, no action needed")
}

func (s *ConnectedState) String() string {
	return helpers.DeriveStateName(s)
}
