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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/snapshot"
)

func init() {
	fsmv2.RegisterInitialState("examplechild", &StoppedState{})
}

// StoppedState represents the initial state where the child worker is not connected.
type StoppedState struct {
	helpers.StoppedBase
}

func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[snapshot.ExamplechildConfig, snapshot.ExamplechildStatus](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil, "shutdown requested, needs removal", nil)
	}

	if !snap.ShouldStop() {
		return fsmv2.Transition(&TryingToConnectState{}, fsmv2.SignalNone, nil, "parent wants running, transitioning to trying to connect", nil)
	}
	// The catch-all return below is logically dead code: the branch above and the first
	// IsShutdownRequested() branch partition the stop/start domain completely. Kept for the
	// canonical 3-branch FSM idiom per architecture validator MISSING_CATCHALL_RETURN.

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "child is stopped, no connection", nil)
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
