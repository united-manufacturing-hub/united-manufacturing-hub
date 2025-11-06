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

// DegradedState represents the error recovery mode when sync operations repeatedly fail.
//
// # Purpose
//
// This state provides graceful degradation and recovery when sync health deteriorates due to
// consecutive errors (network failures, transport errors, backend unavailability). Instead of
// rapid retry loops that could overwhelm the system, DegradedState implements backoff logic
// and controlled retry attempts to allow transient issues to resolve.
//
// The state continues attempting sync operations but with awareness that the system is not
// operating at full health. It transitions back to SyncingState once health recovers, or to
// TryingToAuthenticateState if authentication becomes the root cause.
//
// # Entry Conditions
//
//   - Transition from SyncingState when sync health degrades:
//     observed.IsSyncHealthy() returns false (consecutive errors exceed threshold)
//   - Expected causes: Network instability, relay server temporary unavailability,
//     HTTP transport errors, partial message delivery failures
//
// # Exit Conditions
//
// Transition to SyncingState on health recovery:
//   - Preconditions: observed.IsSyncHealthy() == true AND observed.GetConsecutiveErrors() == 0
//   - Triggers: Successful SyncAction execution resets error counter and health status
//   - Result: Return to normal operational sync loop
//
// Transition to TryingToAuthenticateState on auth issues:
//   - Currently NOT implemented in this state (would need to detect auth-specific errors)
//   - Future: If errors are determined to be authentication-related, should transition to auth
//   - Expected: Error classification logic in action to distinguish network vs auth failures
//
// Transition to StoppedState on shutdown:
//   - Triggers: desired.ShutdownRequested() returns true
//   - Result: Graceful shutdown even during degraded operation
//
// # Actions
//
// Emits SyncAction with retry semantics:
//   - Action parameters: Same as SyncingState (Dependencies, JWTToken)
//   - Action responsibilities: Attempt sync operations, update health metrics
//   - Action idempotency: Safe to retry, critical for recovery attempts
//   - Backoff logic: Implemented via action's error counting (future: exponential backoff)
//
// Current behavior loops on self emitting SyncAction until health recovers. This provides
// continuous retry without state transitions, allowing action-layer backoff to control rate.
//
// # State Transitions
//
//	DegradedState → SyncingState (when health recovers: IsSyncHealthy && consecutive errors = 0)
//	DegradedState → DegradedState (loop: retry sync until success or shutdown)
//	DegradedState → StoppedState (when shutdown requested)
//	DegradedState → TryingToAuthenticateState (future: when errors indicate auth failure)
//
// # Invariants
//
//   - Enforces C4 (shutdown check priority): desired.ShutdownRequested() checked first in Next(),
//     ensuring graceful shutdown is possible even during degraded operation.
//
// Related invariants (not directly enforced here):
//   - C1 (authentication precedence): DegradedState assumes prior successful authentication
//   - C2 (token expiry handling): Token expiry should be caught by SyncingState before degradation,
//     but DegradedState could check this as additional safety (currently not implemented)
//   - C3 (transport lifecycle): Assumes Dependencies.Transport exists for retry attempts
//   - C5 (syncing loop): DegradedState is outside the primary sync loop; recovery returns to C5 loop
//
// # Recovery Strategy
//
// Error recovery follows this pattern:
//  1. SyncingState detects consecutive failures (e.g., 3 in a row)
//  2. Transition to DegradedState to signal unhealthy state
//  3. DegradedState continues retry attempts (action may implement backoff)
//  4. Single successful sync resets error counter and health status
//  5. Transition back to SyncingState resumes normal operation
//
// This provides resilience against transient network issues, temporary backend outages,
// and intermittent relay server problems without manual intervention.
//
// # Future Enhancements
//
// Potential improvements for robust error handling:
//   - Exponential backoff between retry attempts (currently action-layer responsibility)
//   - Error classification (network vs auth vs transport) for smarter transitions
//   - Maximum retry limit before escalation or alert generation
//   - Circuit breaker pattern to prevent thundering herd on backend recovery
//
// See worker.go invariants block (C1-C5) for complete defense-in-depth details.
type DegradedState struct {
	BaseCommunicatorState
}

func (s *DegradedState) Next(snapshot snapshot.CommunicatorSnapshot) (BaseCommunicatorState, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	if desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	if observed.IsSyncHealthy() && observed.GetConsecutiveErrors() == 0 {
		return &SyncingState{}, fsmv2.SignalNone, nil
	}

	// Create SyncAction with registry (retry sync in degraded state)
	syncAction := action.NewSyncAction(desired.Dependencies, observed.JWTToken)

	return s, fsmv2.SignalNone, syncAction
}

func (s *DegradedState) String() string {
	return "Degraded"
}

func (s *DegradedState) Reason() string {
	return "Sync is experiencing errors"
}
