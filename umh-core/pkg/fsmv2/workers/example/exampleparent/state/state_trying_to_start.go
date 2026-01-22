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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/snapshot"
)

// TryingToStartState represents the state while loading config and spawning children.
// Children with ChildStartStates containing "TryingToStart" will have desired state "running".
// Waits for all children to become healthy before transitioning to RunningState.
type TryingToStartState struct {
	BaseParentState
}

func (s *TryingToStartState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.ExampleparentObservedState, *snapshot.ExampleparentDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixTryingToStart, "children")

	if snap.Desired.IsShutdownRequested() {
		return &TryingToStopState{}, fsmv2.SignalNone, nil
	}

	// First, ensure we have a valid ID (StartAction sets this up)
	if snap.Observed.ID == "" {
		return s, fsmv2.SignalNone, &action.StartAction{}
	}

	// Wait for ALL children to be running (healthy > 0, unhealthy == 0).
	// ChildStartStates ensures children have desired state = "running".
	// We only transition when all expected children are healthy.
	if snap.Observed.ChildrenHealthy > 0 && snap.Observed.ChildrenUnhealthy == 0 {
		return &RunningState{}, fsmv2.SignalNone, nil
	}

	// Children not ready yet - stay in this state
	return s, fsmv2.SignalNone, nil
}

func (s *TryingToStartState) String() string {
	return helpers.DeriveStateName(s)
}

func (s *TryingToStartState) Reason() string {
	return "Waiting for all children to become healthy"
}
