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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/snapshot"
)

// StoppedWaitDuration is how long to wait before transitioning to TryingToStart.
// Declared as var (not const) to allow test overrides. Production workers should use dependency injection instead.
var StoppedWaitDuration = 5 * time.Second

func init() {
	fsmv2.RegisterInitialState("exampleparent", &StoppedState{})
}

// StoppedState represents the initial state before any children are spawned.
// It waits for StoppedWaitDuration before transitioning to TryingToStart.
type StoppedState struct {
	helpers.StoppedBase
}

func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[snapshot.ExampleparentConfig, snapshot.ExampleparentStatus](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil, "Shutdown requested, signaling removal", nil)
	}

	children := RenderChildren(snap)

	// Wait StoppedWaitDuration before transitioning. The supervisor tracks
	// time-in-state via FrameworkMetrics.TimeInCurrentStateMs, injected into
	// every observation and surfaced on the typed WorkerSnapshot.
	elapsed := time.Duration(snap.FrameworkMetrics.TimeInCurrentStateMs) * time.Millisecond
	if elapsed >= StoppedWaitDuration {
		return fsmv2.Transition(&TryingToStartState{}, fsmv2.SignalNone, nil, "Wait duration elapsed, transitioning to TryingToStart", children)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "Parent is stopped, no children spawned", children)
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
