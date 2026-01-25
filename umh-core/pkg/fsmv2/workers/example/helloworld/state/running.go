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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/snapshot"
)

// RunningState represents the worker actively running.
// This is the steady state - worker stays here until shutdown.
type RunningState struct{}

// Next implements state transition logic for RunningState.
//
// RUNNING STATE PATTERN:
//   - Check shutdown first (transition to stopped)
//   - Otherwise stay in running state
//   - No actions needed in steady state
func (s *RunningState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.HelloworldObservedState, *snapshot.HelloworldDesiredState](snapAny)

	// 1. Check shutdown - transition back to stopped
	if snap.Desired.IsShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	// 2. Stay in running state - nothing to do
	return s, fsmv2.SignalNone, nil
}

// String returns the state name for logging and metrics.
func (s *RunningState) String() string {
	return helpers.DeriveStateName(s)
}

// Reason returns a human-readable explanation.
func (s *RunningState) Reason() string {
	return "Worker is running and has said hello"
}
