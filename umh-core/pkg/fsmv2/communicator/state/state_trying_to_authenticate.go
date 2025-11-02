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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/snapshot"
)

// TryingToAuthenticateState represents the authentication phase where the worker obtains a JWT token.
//
// # Purpose
//
// This state handles the initial handshake with the relay server to establish authenticated
// communication. It executes AuthenticateAction to obtain a JWT token required for all subsequent
// sync operations. Authentication is the mandatory gateway between initialization and operational
// sync states.
//
// # Entry Conditions
//
//   - Transition from StoppedState on first supervisor tick after initialization
//   - Transition from SyncingState when observed.IsTokenExpired() returns true (token refresh)
//   - Transition from SyncingState when observed.Authenticated is false (auth lost, unexpected)
//   - Transition from DegradedState during error recovery (implicit, if auth was lost)
//
// # Exit Conditions
//
// Transition to SyncingState on successful authentication:
//   - Preconditions: observed.Authenticated == true AND !observed.IsTokenExpired()
//   - Triggers: AuthenticateAction completes successfully and updates observed state
//   - Result: JWT token available, ready for push/pull sync operations
//
// Transition to DegradedState on authentication failure:
//   - Currently NOT implemented (state loops on itself emitting AuthenticateAction)
//   - Future: Failed AuthenticateAction should set error state triggering DegradedState
//   - Expected: Backoff logic and retry limits in DegradedState
//
// # Actions
//
// Emits AuthenticateAction on every tick until authentication succeeds:
//   - Action parameters: Dependencies, RelayURL, InstanceUUID, AuthToken
//   - Action responsibilities: HTTP POST to relay server, JWT token extraction, update observed state
//   - Action idempotency: Safe to retry on network failures or partial completions
//
// # State Transitions
//
//	TryingToAuthenticateState → SyncingState (on success: authenticated && !tokenExpired)
//	TryingToAuthenticateState → TryingToAuthenticateState (loop: emits AuthenticateAction until success)
//	TryingToAuthenticateState → StoppedState (if shutdown requested during auth)
//	TryingToAuthenticateState → DegradedState (future: on repeated auth failures)
//
// # Invariants
//
//   - Enforces C1 (authentication precedence): MUST obtain JWT token before any sync operations.
//     SyncingState is unreachable without passing through this state successfully.
//
//   - Enforces C2 (token expiry handling): Checks observed.IsTokenExpired() before declaring
//     authentication complete. Expired tokens force re-authentication even if Authenticated flag is true.
//
//   - Enforces C4 (shutdown check priority): Next() checks desired.ShutdownRequested() before
//     emitting actions, ensuring graceful shutdown during authentication attempts.
//
// Related invariants (not directly enforced here):
//   - C3 (transport lifecycle): Assumes Dependencies.Transport exists (validated in worker construction)
//   - C5 (syncing loop): Not relevant here; applies to SyncingState operational loop
//
// See worker.go invariants block (C1-C5) for complete defense-in-depth details.
type TryingToAuthenticateState struct {
	BaseCommunicatorState
}

func (s *TryingToAuthenticateState) Next(snapshot snapshot.CommunicatorSnapshot) (BaseCommunicatorState, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	if desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	if observed.Authenticated && !observed.IsTokenExpired() { // TODO: check good abstraction / API
		return &SyncingState{}, fsmv2.SignalNone, nil
	}

	// Create AuthenticateAction with registry from snapshot
	authenticateAction := action.NewAuthenticateAction(
		desired.Dependencies,
		desired.RelayURL,
		desired.InstanceUUID,
		desired.AuthToken,
	)

	return s, fsmv2.SignalNone, authenticateAction
}

func (s *TryingToAuthenticateState) String() string {
	return "TryingToAuthenticate"
}

func (s *TryingToAuthenticateState) Reason() string {
	return "Attempting to authenticate with relay server"
}
