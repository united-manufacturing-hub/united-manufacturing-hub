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

// StoppedWaitDuration is how long to wait in stopped state before transitioning to TryingToStart.
//
// EXAMPLE/TEST WORKER ONLY: This is declared as var (not const) to allow tests to override
// the duration for fast test execution without real time delays.
//
// WARNING: Real production workers should NOT follow this pattern. Production code should use:
//   - Dependency injection (pass duration via constructor or config struct)
//   - Configuration objects that can be mocked in tests
//   - Interface-based time abstractions
//
// Package-level mutable variables create global state that can cause test interference
// and make code harder to reason about. This pattern is acceptable here only because
// this is an example/test worker demonstrating FSM concepts, not production code.
var StoppedWaitDuration = 5 * time.Second

// StoppedState represents the initial state before any children are spawned.
// It waits for StoppedWaitDuration before transitioning to TryingToStart.
type StoppedState struct {
	BaseParentState
}

func (s *StoppedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.ExampleparentObservedState, *snapshot.ExampleparentDesiredState](snapAny)
	snap.Observed.State = config.PrefixStopped

	if snap.Desired.IsShutdownRequested() {
		return s, fsmv2.SignalNeedsRemoval, nil
	}

	// Wait StoppedWaitDuration in stopped state before transitioning to starting.
	// TimeInCurrentStateMs is injected by supervisor via FrameworkMetrics.
	if snap.Desired.ShouldBeRunning() {
		elapsed := time.Duration(snap.Observed.GetFrameworkMetrics().TimeInCurrentStateMs) * time.Millisecond
		if elapsed >= StoppedWaitDuration {
			return &TryingToStartState{}, fsmv2.SignalNone, nil
		}
		// Still waiting - remain in stopped state
	}

	return s, fsmv2.SignalNone, nil
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}

func (s *StoppedState) Reason() string {
	return "Parent is stopped, no children spawned"
}
