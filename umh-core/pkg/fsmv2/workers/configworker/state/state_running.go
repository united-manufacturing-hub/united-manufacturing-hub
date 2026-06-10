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

// Package state holds the config worker's state machine states.
package state

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/snapshot"
)

func init() {
	fsmv2.RegisterInitialState("configworker", &RunningState{})
}

// RunningState represents the config worker actively running. This is the
// steady state - the worker stays here until shutdown.
type RunningState struct {
	helpers.RunningHealthyBase
}

// Next implements state transition logic for RunningState.
func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[snapshot.ConfigworkerConfig, snapshot.ConfigworkerStatus](snapAny)

	if snap.ShouldStop() {
		return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("stop required: shutdown=%t disabled=%t", snap.IsShutdownRequested, snap.IsDisabled), nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "config worker is running", nil)
}

// String returns the state name for logging and metrics.
func (s *RunningState) String() string {
	return helpers.DeriveStateName(s)
}
