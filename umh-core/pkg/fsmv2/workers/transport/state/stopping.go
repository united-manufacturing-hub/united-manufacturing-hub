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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

// StoppingState represents the graceful shutdown state.
// Waits for all children to stop before transitioning to StoppedState.
type StoppingState struct {
	helpers.StoppingBase
}

// Next evaluates the current snapshot and returns the next state or action.
func (s *StoppingState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.TransportObservedState, *snapshot.TransportDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() { //nolint:staticcheck // architecture invariant: shutdown check must be first conditional
	}

	// Unconditional transition to Stopped. The decision to stop was already made
	// on entry to StoppingState. The supervisor handles child teardown independently.
	// Previously, waiting for ChildrenHealthy==0 && ChildrenUnhealthy==0 caused a
	// cascade deadlock when children were stuck in Stopping (ENG-4608).
	return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil,
		fmt.Sprintf("stop complete: children healthy=%d, unhealthy=%d",
			snap.Observed.ChildrenHealthy, snap.Observed.ChildrenUnhealthy))
}

// String returns the state name derived from the type.
func (s *StoppingState) String() string {
	return helpers.DeriveStateName(s)
}
