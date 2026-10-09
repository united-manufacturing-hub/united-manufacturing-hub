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

// Package state defines the FSM states for the example bridge worker.
package state

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplebridge"
)

func init() {
	fsmv2.RegisterInitialState(examplebridge.WorkerTypeName, &StoppedState{})
}

// StoppedState is the initial state where no flows are spawned.
type StoppedState struct {
	helpers.StoppedBase
}

// Next implements the state transition logic for StoppedState.
func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[examplebridge.ExamplebridgeConfig, examplebridge.ExamplebridgeStatus](snapAny)

	// The shutdown check comes first: the validators require it.
	if snap.ShouldStop() {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil,
			"stop required: "+snap.StopReason(), nil)
	}

	// A bridge worker is itself a child of an application supervisor that
	// can disable it.
	if snap.IsDisabled {
		return fsmv2.Transition(s, fsmv2.SignalNone, nil, "Disabled by supervisor, staying stopped", nil)
	}

	return fsmv2.Transition(&RunningState{}, fsmv2.SignalNone, nil, "Starting the flows", nil)
}

// String returns the state name for logging and metrics.
func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
