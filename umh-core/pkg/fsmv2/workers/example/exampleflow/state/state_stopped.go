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

// Package state defines the FSM states for the example flow worker.
package state

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleflow"
)

func init() {
	fsmv2.RegisterInitialState(exampleflow.WorkerTypeName, &StoppedState{})
}

// StoppedState is the initial state where the worker is not running.
type StoppedState struct {
	helpers.StoppedBase
}

// Next implements the state transition logic for StoppedState.
func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[exampleflow.ExampleflowConfig, exampleflow.ExampleflowStatus](snapAny)

	// The shutdown check comes first: the validators require it.
	if snap.ShouldStop() {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil,
			"stop required: "+snap.StopReason(), nil)
	}

	return fsmv2.Transition(&TryingToStartState{}, fsmv2.SignalNone, nil, "Starting the flow", nil)
}

// String returns the state name for logging and metrics.
func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
