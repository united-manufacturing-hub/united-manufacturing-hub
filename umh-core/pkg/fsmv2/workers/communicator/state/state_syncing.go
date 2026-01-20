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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
)

// SyncingState represents the primary operational state for bidirectional message synchronization.
//
// # Purpose
//
// This is the steady-state mode of the communicator worker, where continuous bidirectional
// message exchange occurs between edge and backend tiers. In this state, SyncAction performs
// HTTP push/pull operations in a loop, maintaining real-time communication with the relay server.
//
// This state is designed to run indefinitely once authentication succeeds, only exiting on
// token expiry, authentication loss, sync health degradation, or shutdown requests.
//
// # Entry Conditions
//
//   - Transition from TryingToAuthenticateState when authentication completes successfully:
//     observed.Authenticated == true AND !observed.IsTokenExpired()
//   - Transition from DegradedState when sync health recovers:
//     observed.IsSyncHealthy() == true AND observed.GetConsecutiveErrors() == 0
//
// # Exit Conditions
//
// Transition to TryingToAuthenticateState on token expiry or auth loss:
//   - Triggers: observed.IsTokenExpired() returns true (JWT token expired, needs refresh)
//   - Triggers: observed.Authenticated == false (unexpected auth loss, network issues)
//   - Result: Re-authentication required before resuming sync operations
//
// Transition to DegradedState on sync health degradation:
//   - Triggers: observed.IsSyncHealthy() returns false (consecutive errors exceed threshold)
//   - Result: Error recovery mode with backoff before retry
//
// Transition to StoppedState on shutdown:
//   - Triggers: desired.ShutdownRequested() returns true
//   - Result: Graceful shutdown, cleanup, and FSM termination
//
// # Actions
//
// Emits SyncAction on every tick while healthy:
//   - Action parameters: Dependencies, JWTToken from observed state
//   - Action responsibilities: HTTP push/pull via transport, message queue management
//   - Action idempotency: Safe to retry, handles partial network failures
//   - Action frequency: Continuous loop with minimal supervisor tick delay (10ms default)
//
// SyncAction performs:
//   - Push: Drain outbound channel, batch messages, POST to relay server
//   - Pull: GET from relay server, write messages to inbound channel
//
// # State Transitions
//
//	SyncingState → SyncingState (loop: continuous sync while healthy and authenticated)
//	SyncingState → TryingToAuthenticateState (when token expires or auth lost)
//	SyncingState → DegradedState (when !IsSyncHealthy: consecutive errors accumulate)
//	SyncingState → StoppedState (when shutdown requested)
//
// # Invariants
//
//   - Enforces C2 (token expiry handling): Checks observed.IsTokenExpired() on every tick
//     BEFORE emitting SyncAction. Expired tokens immediately trigger re-authentication.
//
//   - Enforces C4 (shutdown check priority): desired.ShutdownRequested() checked first in Next(),
//     ensuring graceful shutdown takes precedence over sync operations.
//
//   - Enforces C5 (syncing state loop): Returns (self, SignalNone, SyncAction) on success,
//     creating an indefinite loop that is the primary operational mode of the communicator.
//     This loop only exits on explicit conditions (token expiry, health degradation, shutdown).
//
// Related invariants (not directly enforced here):
//   - C1 (authentication precedence): Syncing is only reachable AFTER TryingToAuthenticate succeeds
//   - C3 (transport lifecycle): Assumes Dependencies.Transport exists for SyncAction execution
//
// # Health Monitoring
//
// Sync health is tracked by the action's error counting logic:
//   - Successful sync operations reset consecutive error counter
//   - Failed sync operations increment consecutive error counter
//   - IsSyncHealthy() returns false when errors exceed threshold (e.g., 3 consecutive failures)
//   - Health degradation triggers transition to DegradedState for backoff and recovery
//
// See worker.go invariants block (C1-C5) for complete validation layer details.
type SyncingState struct {
}

func (s *SyncingState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixRunning, "syncing")

	if snap.Desired.IsShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	if snap.Observed.IsTokenExpired() {
		return &TryingToAuthenticateState{}, fsmv2.SignalNone, nil
	}

	if !snap.Observed.Authenticated {
		return &TryingToAuthenticateState{}, fsmv2.SignalNone, nil
	}

	if !snap.Observed.IsSyncHealthy() {
		return NewDegradedState(), fsmv2.SignalNone, nil
	}

	syncAction := action.NewSyncAction(snap.Observed.JWTToken)

	return s, fsmv2.SignalNone, syncAction
}

func (s *SyncingState) String() string {
	return "Syncing"
}

func (s *SyncingState) Reason() string {
	return "Syncing with relay server"
}

// GetBackoffDelay calculates exponential backoff based on consecutive errors.
// Returns 0 if no errors. Caps at 60 seconds.
//
// This method delegates to the backoff.CalculateDelay utility function.
// It is kept for backward compatibility with existing code and tests.
func (s *SyncingState) GetBackoffDelay(observed snapshot.CommunicatorObservedState) time.Duration {
	return backoff.CalculateDelay(observed.ConsecutiveErrors)
}
