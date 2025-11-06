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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
)

// StoppedState represents the initial dormant state before any communication with the relay server.
//
// # Purpose
//
// This is the entry point for all communicator workers, returned by worker.GetInitialState().
// In this state, no authentication attempts occur, no sync operations run, and no network
// communication happens. The state serves as a clean initialization point and a target for
// graceful shutdown transitions.
//
// # Entry Conditions
//
//   - Worker initialization via worker.GetInitialState() (first entry, always)
//   - Graceful shutdown from any operational state (TryingToAuthenticate, Syncing, Degraded)
//     when desired.ShutdownRequested() returns true
//
// Never re-entered after the initial startup transition to TryingToAuthenticate, except
// during shutdown sequences.
//
// # Exit Conditions
//
// Transition to TryingToAuthenticateState on first supervisor tick:
//   - Preconditions: None (always transitions immediately)
//   - Triggers: Supervisor calls Next() with any valid snapshot
//   - Exception: If desired.ShutdownRequested() is true, emits SignalNeedsRemoval instead
//
// # Actions
//
// None. This state performs no operations and emits no actions. It is a pure transition state
// that immediately moves to TryingToAuthenticate unless shutdown is requested.
//
// # State Transitions
//
//	StoppedState → TryingToAuthenticateState (always, unless shutdown requested)
//	StoppedState → (removed via SignalNeedsRemoval) (if shutdown requested)
//
// # Invariants
//
//   - Enforces C1 (authentication precedence): No sync operations possible before authentication.
//     The state machine MUST pass through TryingToAuthenticate before reaching Syncing.
//
//   - Enforces C4 (shutdown check priority): Next() checks shutdown signal before any transitions,
//     ensuring graceful termination takes precedence over initialization.
//
// Related invariants (not directly enforced here):
//   - C3 (transport lifecycle): Dependencies validation happens in worker construction,
//     not in state transitions. Stopped assumes transport exists if worker was created.
//
// See worker.go invariants block (C1-C5) for complete defense-in-depth details.
type StoppedState struct {
	BaseCommunicatorState
}

func (s *StoppedState) Next(snapshot snapshot.CommunicatorSnapshot) (BaseCommunicatorState, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired

	if desired.ShutdownRequested() {
		return s, fsmv2.SignalNeedsRemoval, nil
	}

	return &TryingToAuthenticateState{}, fsmv2.SignalNone, nil
}

func (s *StoppedState) String() string {
	return "Stopped"
}

func (s *StoppedState) Reason() string {
	return "Communicator is stopped"
}
