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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/snapshot"
)

// TryingToStopState represents the state during graceful shutdown.
// Children without "TryingToStop" in ChildStartStates will have desired state "stopped".
// Waits for all children to stop before transitioning to StoppedState.
type TryingToStopState struct {
	helpers.StoppingBase
}

func (s *TryingToStopState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[snapshot.ExampleparentConfig, snapshot.ExampleparentStatus](snapAny)

	children := RenderChildren(snap)

	// All children must be stopped before transitioning.
	if snap.Observed.ChildrenHealthy == 0 && snap.Observed.ChildrenUnhealthy == 0 {
		return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil, "All children stopped, transitioning to Stopped", children)
	}

	// Self-return WITH an action (StopAction) — ValidateStoppingStateNoCatchAllSelfReturn
	// forbids self-return with nil in a stopping base. StopAction drives children down.
	return fsmv2.Transition(s, fsmv2.SignalNone, &action.StopAction{}, "Gracefully stopping all children", children)
}

func (s *TryingToStopState) String() string {
	return helpers.DeriveStateName(s)
}
