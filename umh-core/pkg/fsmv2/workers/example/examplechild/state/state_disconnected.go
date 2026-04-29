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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/snapshot"
)

// DisconnectedState represents the state where connection has been lost and will be retried.
type DisconnectedState struct {
	helpers.RunningDegradedBase
}

func (s *DisconnectedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExamplechildObservedState, *snapshot.ExamplechildDesiredState](snapAny)

	if snap.Observed.ShouldStop() {
		return fsmv2.Transition(&TryingToStopState{}, fsmv2.SignalNone, nil, "Stop required, initiating shutdown", nil)
	}

	if !snap.Observed.ShouldStop() {
		return fsmv2.Transition(&TryingToConnectState{}, fsmv2.SignalNone, nil, "Attempting to reconnect", nil)
	}

	// The catch-all return below is logically dead code: the two branches above
	// (`ShouldStop()` and `!ShouldStop()`) partition the boolean's domain
	// completely. We keep the canonical 3-branch FSM idiom anyway because the
	// architecture validator's MISSING_CATCHALL_RETURN check is syntactic, not
	// semantic — see PR2 cascade pattern memory #3 (validator-syntactic vs
	// migration-semantic). Collapsing to 2 branches would trip the validator
	// even though the simplification is behavior-preserving.
	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "Connection lost, will retry", nil)
}

func (s *DisconnectedState) String() string {
	return helpers.DeriveStateName(s)
}
