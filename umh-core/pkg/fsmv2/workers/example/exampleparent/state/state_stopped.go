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

// StoppedWaitDuration is how long to wait before transitioning to TryingToStart.
// Declared as var (not const) to allow test overrides. Production workers should use dependency injection instead.
var StoppedWaitDuration = 5 * time.Second

// StoppedState represents the initial state before any children are spawned.
// It waits for StoppedWaitDuration before transitioning to TryingToStart.
type StoppedState struct {
	BaseParentState
}

func (s *StoppedState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseStopped
}

func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExampleparentObservedState, *snapshot.ExampleparentDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](s, fsmv2.SignalNeedsRemoval, nil, "Shutdown requested, signaling removal")
	}

	// Wait StoppedWaitDuration before transitioning. Direct field access required for CSE serializability.
	if snap.Desired.ShouldBeRunning() {
		elapsed := time.Duration(snap.Observed.Metrics.Framework.TimeInCurrentStateMs) * time.Millisecond
		if elapsed >= StoppedWaitDuration {
			return fsmv2.Result[any, any](&TryingToStartState{}, fsmv2.SignalNone, nil, "Wait duration elapsed, transitioning to TryingToStart")
		}
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "Parent is stopped, no children spawned")
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
