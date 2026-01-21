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
var RunningDuration = 10 * time.Second

// RunningState represents normal operational state with all children healthy.
// It stays in this state for RunningDuration before automatically transitioning to TryingToStop.
type RunningState struct {
	BaseParentState
}

func (s *RunningState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.ExampleparentObservedState, *snapshot.ExampleparentDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixRunning, "healthy")

	if snap.Desired.IsShutdownRequested() {
		return &TryingToStopState{}, fsmv2.SignalNone, nil
	}

	// Transition to degraded if any child becomes unhealthy
	if snap.Observed.ChildrenUnhealthy > 0 {
		return &DegradedState{}, fsmv2.SignalNone, nil
	}

	// After RunningDuration in running state, initiate stop cycle.
	// TimeInCurrentStateMs is copied from deps by worker via Metrics.Framework.
	// Direct field access is required for CSE serializability - getter methods don't work.
	elapsed := time.Duration(snap.Observed.Metrics.Framework.TimeInCurrentStateMs) * time.Millisecond
	if elapsed >= RunningDuration {
		return &TryingToStopState{}, fsmv2.SignalNone, nil
	}

	return s, fsmv2.SignalNone, nil
}

func (s *RunningState) String() string {
	return helpers.DeriveStateName(s)
}

func (s *RunningState) Reason() string {
	return "All children healthy and running"
}
