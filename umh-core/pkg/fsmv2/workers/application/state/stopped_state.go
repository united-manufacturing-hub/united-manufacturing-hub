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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
)

// init registers RunningState as the initial state for the application worker
// type. WorkerBase.GetInitialState looks this up at runtime; the worker
// package blank-imports state/ to ensure this init runs before any tick.
func init() {
	fsmv2.RegisterInitialState("application", &RunningState{})
}

// StoppedState represents the stopped state of the application supervisor.
// In this state, the worker emits SignalNeedsRemoval to indicate it's ready
// to be removed from the supervisor's registry.
type StoppedState struct {
	helpers.StoppedBase
}

func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[snapshot.ApplicationConfig, snapshot.ApplicationStatus](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil, "Application supervisor is stopped and shutdown requested", nil)
	}

	children := RenderChildren(snap)

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "Application supervisor is stopped", children)
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
