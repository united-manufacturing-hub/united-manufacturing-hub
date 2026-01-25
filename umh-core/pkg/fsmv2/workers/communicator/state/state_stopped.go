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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
)

// StoppedState is the initial state before authentication.
// Transitions to TryingToAuthenticateState on first tick, or emits SignalNeedsRemoval if shutdown.
// No actions emitted. Enforces C1 (auth precedence) and C4 (shutdown priority).
type StoppedState struct {
	BaseCommunicatorState
}

func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](snapAny)
	snap.Observed.State = config.PrefixStopped

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](s, fsmv2.SignalNeedsRemoval, nil, "Communicator is stopped and shutdown was requested")
	}

	return fsmv2.Result[any, any](&TryingToAuthenticateState{}, fsmv2.SignalNone, nil, "Starting authentication")
}

func (s *StoppedState) String() string {
	return "Stopped"
}
