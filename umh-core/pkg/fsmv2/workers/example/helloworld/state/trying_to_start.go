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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/snapshot"
)

// TryingToStartState is a transitional state that emits the SayHelloAction.
// Once hello is said, transitions to RunningState.
type TryingToStartState struct{}

func (s *TryingToStartState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseStarting
}

// Next implements state transition logic for TryingToStartState.
//
// TRANSITIONAL STATE PATTERN:
//   - Check shutdown first
//   - Check if the action we need has completed (observe the effect)
//   - If completed, transition to next state
//   - If not completed, emit the action and stay in this state
func (s *TryingToStartState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.HelloworldObservedState, *snapshot.HelloworldDesiredState](snapAny)

	// 1. ALWAYS check shutdown first
	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to stopped")
	}

	// 2. Check if action has already completed (observe the effect)
	// This makes the state machine resilient to action replay
	if snap.Observed.HelloSaid {
		return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil, "Hello has been said, transitioning to running")
	}

	// 3. Emit action and stay in this state
	// The action will set HelloSaid=true, which we'll observe next tick
	return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.SayHelloAction{}, "Saying hello to the world")
}

// String returns the state name for logging and metrics.
func (s *TryingToStartState) String() string {
	return helpers.DeriveStateName(s)
}
