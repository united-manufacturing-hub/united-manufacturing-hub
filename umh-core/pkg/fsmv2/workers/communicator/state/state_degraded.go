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

// DegradedState handles error recovery when sync operations repeatedly fail.
// Uses exponential backoff and periodic transport resets (at 5, 10, 15... errors).
//
// Transitions:
//   - → SyncingState: when IsSyncHealthy() && consecutive errors = 0
//   - → StoppedState: if shutdown requested
//   - → self: backoff wait or retry via SyncAction/ResetTransportAction
//
// Enforces C4 (shutdown priority). Timing stored in ObservedState.DegradedEnteredAt.
type DegradedState struct {
	BaseCommunicatorState
	// Note: FSM states should be stateless. All timing information is now stored in
	// ObservedState.DegradedEnteredAt, which is populated by the dependencies when
	// the first error occurs.
}

// NewDegradedState creates a new DegradedState.
func NewDegradedState() *DegradedState {
	return &DegradedState{}
}

// NewDegradedStateWithEnteredAt creates a new DegradedState.
// Deprecated: enteredAt is ignored; reads from ObservedState.DegradedEnteredAt instead.
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
	if consecutiveErrors > 0 && consecutiveErrors%backoff.TransportResetThreshold == 0 {
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
