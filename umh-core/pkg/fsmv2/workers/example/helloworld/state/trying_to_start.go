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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	hello_world "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
)

// TryingToStartState is a transitional state that emits the SayHelloAction.
// Once hello is said, transitions to RunningState.
type TryingToStartState struct {
	helpers.StartingBase
}

// Next implements state transition logic for TryingToStartState.
func (s *TryingToStartState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[hello_world.HelloworldConfig, hello_world.HelloworldStatus](snapAny)

	// 1. Check shutdown first
	if snap.IsShutdownRequested {
		return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to stopped")
	}

	// 2. Check if action has already completed (observe the effect)
	// This makes the state machine resilient to action replay
	if snap.Status.HelloSaid {
		return fsmv2.Transition(&RunningState{}, fsmv2.SignalNone, nil, "Hello has been said, transitioning to running")
	}

	// 3. Emit action and stay in this state
	return fsmv2.Transition(s, fsmv2.SignalNone,
		fsmv2.SimpleAction[*hello_world.HelloworldDependencies](hello_world.SayHelloActionName, hello_world.SayHello),
		"Saying hello to the world", nil)
}

// String returns the state name for logging and metrics.
func (s *TryingToStartState) String() string {
	return helpers.DeriveStateName(s)
}
