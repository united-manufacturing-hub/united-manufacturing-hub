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

// DegradedState represents the worker running but impaired.
// Entered when the external mood file contains "sad".
// Transitions back to RunningState when the mood changes.
type DegradedState struct {
	helpers.RunningDegradedBase
}

// Next implements state transition logic for DegradedState.
//
// DEGRADED STATE PATTERN:
//   - Check shutdown first
//   - Check if the condition that caused degradation has cleared
//   - If cleared, transition back to running
//   - Otherwise stay degraded
func (s *DegradedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.HelloworldObservedState, *snapshot.HelloworldDesiredState](snapAny)

	// 1. Check shutdown
	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to stopped")
	}

	// 2. Check if mood has recovered (no longer "sad")
	if snap.Observed.Mood != "sad" {
		return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil, "Mood recovered, transitioning to running")
	}

	// 3. Stay degraded
	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "Mood is still sad")
}

// String returns the state name for logging and metrics.
func (s *DegradedState) String() string {
	return helpers.DeriveStateName(s)
}
