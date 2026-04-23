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

// TryingToStartState represents the state while spawning children.
// Children with ChildStartStates containing "TryingToStart" will have desired state "running".
// Waits for all children to become healthy before transitioning to RunningState.
type TryingToStartState struct {
	helpers.StartingBase
}

func (s *TryingToStartState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[snapshot.ExampleparentConfig, snapshot.ExampleparentStatus](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Transition(&TryingToStopState{}, fsmv2.SignalNone, nil, "shutdown requested, transitioning to TryingToStop")
	}

	// All children must be running (healthy > 0, unhealthy == 0) before transitioning.
	if snap.ChildrenHealthy > 0 && snap.ChildrenUnhealthy == 0 {
		return fsmv2.Transition(&RunningState{}, fsmv2.SignalNone, nil, "all children healthy, transitioning to Running")
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, &action.StartAction{}, "waiting for children to become healthy, running StartAction")
}

func (s *TryingToStartState) String() string {
	return helpers.DeriveStateName(s)
}
