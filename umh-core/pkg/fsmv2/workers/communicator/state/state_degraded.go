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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/backoff"
	commconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
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
//   - Not implemented in this state (would need to detect auth-specific errors)
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
//   - Action idempotency: Safe to retry, required for recovery attempts
//   - Backoff logic: Implemented via action's error counting (future: exponential backoff)
//
// The state loops on itself emitting SyncAction until health recovers. This provides
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
// Potential improvements for error recovery:
//   - Exponential backoff between retry attempts (currently action-layer responsibility)
//   - Error classification (network vs auth vs transport) for smarter transitions
//   - Maximum retry limit before escalation or alert generation
//   - Circuit breaker pattern to prevent thundering herd on backend recovery
//
// See worker.go invariants block (C1-C5) for complete validation layer details.
type DegradedState struct {
	BaseCommunicatorState
	// Note: FSM states should be stateless. All timing information is now stored in
	// ObservedState.DegradedEnteredAt, which is populated by the dependencies when
	// the first error occurs.
}

// NewDegradedState creates a new DegradedState.
// The entry timestamp is tracked in ObservedState.DegradedEnteredAt, not in the state itself.
func NewDegradedState() *DegradedState {
	return &DegradedState{}
}

// NewDegradedStateWithEnteredAt creates a new DegradedState.
//
// Deprecated: The enteredAt parameter is ignored. DegradedState now reads the entry
// timestamp from ObservedState.DegradedEnteredAt, which is set by dependencies when
// the first error occurs. This function is kept for backward compatibility with tests.
// Tests should instead set up the dependencies to call RecordError() at the desired time.
func NewDegradedStateWithEnteredAt(_ time.Time) *DegradedState {
	return &DegradedState{}
}

func (s *DegradedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixRunning, "degraded")

	if snap.Desired.IsShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	if snap.Observed.IsSyncHealthy() && snap.Observed.GetConsecutiveErrors() == 0 {
		return &SyncingState{}, fsmv2.SignalNone, nil
	}

	// Calculate backoff using the shared backoff utility
	consecutiveErrors := snap.Observed.GetConsecutiveErrors()
	backoffDelay := backoff.CalculateDelay(consecutiveErrors)

	// Get when we entered degraded mode (first error after success)
	// This timestamp is set by dependencies.RecordError() on the first error
	enteredAt := snap.Observed.DegradedEnteredAt

	// If we're still within the backoff period, wait (no action)
	if !enteredAt.IsZero() && time.Since(enteredAt) < backoffDelay {
		return s, fsmv2.SignalNone, nil
	}

	// Check if we should reset the transport (at 5, 10, 15... consecutive errors)
	// This helps recover from persistent connection-level issues
	if consecutiveErrors > 0 && consecutiveErrors%commconfig.TransportResetThreshold == 0 {
		return s, fsmv2.SignalNone, action.NewResetTransportAction()
	}

	// Backoff expired - create SyncAction to retry
	// deps injected via Execute() by supervisor
	syncAction := action.NewSyncAction(snap.Observed.JWTToken)

	return s, fsmv2.SignalNone, syncAction
}

func (s *DegradedState) String() string {
	return "Degraded"
}

func (s *DegradedState) Reason() string {
	return "Sync is experiencing errors"
}
