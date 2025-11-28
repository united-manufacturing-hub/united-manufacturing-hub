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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/snapshot"
)

// TryingToStartState represents the state while loading config and spawning children.
type TryingToStartState struct {
	BaseParentState
}

func (s *TryingToStartState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.ParentObservedState, *snapshot.ParentDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixTryingToStart, "children")

	if snap.Desired.IsShutdownRequested() {
		return &TryingToStopState{}, fsmv2.SignalNone, nil
	}

	// Parent worker is "started" when it has a valid observed state with an ID
	// The StartAction just logs that it's starting, so we can transition immediately
	// to Running state where we monitor children health
	if snap.Observed.ID != "" {
		return &RunningState{}, fsmv2.SignalNone, nil
	}

	return s, fsmv2.SignalNone, &action.StartAction{}
}

func (s *TryingToStartState) String() string {
	return helpers.DeriveStateName(s)
}

func (s *TryingToStartState) Reason() string {
	return "Loading configuration and spawning children"
}
