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
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplebridge"
)

// StopActionName names the bridge's stop action.
const StopActionName = "stop"

// stop observes the bridge's stop. The flows despawn through the child set
// this state already returned, so the action itself only observes the stop.
func stop(_ context.Context, _ register.NoDeps) error {
	return nil
}

// TryingToStopState commits to cleanup: it returns an empty child set so the
// flows despawn, and waits until every child is gone before stopping. Once
// the worker enters this state it runs to completion (Stopped) before any
// resume can begin.
type TryingToStopState struct {
	helpers.StoppingBase
}

// Next implements the state transition logic for TryingToStopState.
func (s *TryingToStopState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[examplebridge.ExamplebridgeConfig, examplebridge.ExamplebridgeStatus](snapAny)

	despawnChildren := []config.ChildSpec{}

	if snap.ChildrenHealthy == 0 && snap.ChildrenUnhealthy == 0 {
		return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil,
			"All flows stopped, transitioning to Stopped", despawnChildren)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone,
		fsmv2.SimpleAction[register.NoDeps](StopActionName, stop),
		"Gracefully stopping all flows", despawnChildren)
}

// String returns the state name for logging and metrics.
func (s *TryingToStopState) String() string {
	return helpers.DeriveStateName(s)
}
