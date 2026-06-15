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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/snapshot"
)

// StoppedState is where the config worker rests once it should not run. It
// follows the framework's three-way Stopped contract with shutdown taking
// precedence: a shutdown request signals removal so the supervisor reaps the
// worker; a disable (without shutdown) keeps the worker resident so it retains
// its shared registry handle and can resume; otherwise it resumes to Running.
type StoppedState struct {
	helpers.StoppedBase
}

// Next implements state transition logic for StoppedState.
func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[snapshot.ConfigworkerConfig, snapshot.ConfigworkerStatus](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil, "shutdown requested, signaling removal", nil)
	}

	if snap.IsDisabled {
		return fsmv2.Transition(s, fsmv2.SignalNone, nil, "disabled, staying resident", nil)
	}

	return fsmv2.Transition(&RunningState{}, fsmv2.SignalNone, nil, "config worker resuming", nil)
}

// String returns the state name for logging and metrics.
func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
