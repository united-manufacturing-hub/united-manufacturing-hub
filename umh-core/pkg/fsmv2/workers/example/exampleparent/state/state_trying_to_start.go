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

func (s *TryingToStartState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExampleparentObservedState, *snapshot.ExampleparentDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixTryingToStart, "children")

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&TryingToStopState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to TryingToStop")
	}

	if snap.Observed.ID == "" {
		return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.StartAction{}, "ID not set, executing StartAction")
	}

	// All children must be running (healthy > 0, unhealthy == 0) before transitioning.
	if snap.Observed.ChildrenHealthy > 0 && snap.Observed.ChildrenUnhealthy == 0 {
		return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil, "All children healthy, transitioning to Running")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "Waiting for all children to become healthy")
}

func (s *TryingToStartState) String() string {
	return helpers.DeriveStateName(s)
}
