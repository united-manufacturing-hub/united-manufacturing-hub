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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/snapshot"
)

// DegradedState represents the state when some children have failed.
type DegradedState struct {
	BaseParentState
}

func (s *DegradedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExampleparentObservedState, *snapshot.ExampleparentDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixRunning, "degraded")

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&TryingToStopState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to TryingToStop")
	}

	if snap.Observed.ChildrenUnhealthy == 0 {
		return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil, "All children now healthy, transitioning to Running")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "Some children are unhealthy")
}

func (s *DegradedState) String() string {
	return helpers.DeriveStateName(s)
}
