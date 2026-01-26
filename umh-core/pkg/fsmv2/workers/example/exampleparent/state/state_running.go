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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/snapshot"
)

// RunningDuration is how long to stay in running state before initiating the stop cycle.
// Declared as var (not const) to allow test overrides. Production code should use DI instead.
var RunningDuration = 10 * time.Second

// RunningState represents normal operational state with all children healthy.
type RunningState struct {
	BaseParentState
}

func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExampleparentObservedState, *snapshot.ExampleparentDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixRunning, "healthy")

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&TryingToStopState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to TryingToStop")
	}

	if snap.Observed.ChildrenUnhealthy > 0 {
		return fsmv2.Result[any, any](&DegradedState{}, fsmv2.SignalNone, nil, "Some children are unhealthy, transitioning to Degraded")
	}

	elapsed := time.Duration(snap.Observed.Metrics.Framework.TimeInCurrentStateMs) * time.Millisecond
	if elapsed >= RunningDuration {
		return fsmv2.Result[any, any](&TryingToStopState{}, fsmv2.SignalNone, nil, "Running duration elapsed, transitioning to TryingToStop")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "All children healthy and running")
}

func (s *RunningState) String() string {
	return helpers.DeriveStateName(s)
}
