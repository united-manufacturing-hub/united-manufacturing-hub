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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
)

// StoppedState is the initial state.
// Transitions to SyncingState when running, or emits SignalNeedsRemoval if shutdown.
// Authentication is handled by TransportWorker (ENG-4264).
type StoppedState struct {
	helpers.StoppedBase
}

func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[communicator.CommunicatorConfig, communicator.CommunicatorStatus](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Result[any, any](s, fsmv2.SignalNeedsRemoval, nil, "Communicator is stopped and shutdown was requested")
	}

	return fsmv2.Result[any, any](&SyncingState{}, fsmv2.SignalNone, nil, "Starting sync orchestration")
}

func (s *StoppedState) String() string {
	return "Stopped"
}

func init() {
	communicator.RegisterInitialState(&StoppedState{})
}
