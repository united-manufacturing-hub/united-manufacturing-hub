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

// DegradedState handles error recovery with exponential backoff and periodic transport resets.
type DegradedState struct {
	BaseCommunicatorState
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

	consecutiveErrors := snap.Observed.GetConsecutiveErrors()
	backoffDelay := backoff.CalculateDelayForErrorType(
		snap.Observed.LastErrorType,
		consecutiveErrors,
		snap.Observed.LastRetryAfter,
	)

	enteredAt := snap.Observed.DegradedEnteredAt
	if !enteredAt.IsZero() && time.Since(enteredAt) < backoffDelay {
		return s, fsmv2.SignalNone, nil
	}

	// Reset transport at 5, 10, 15... errors to recover from connection-level issues
	if consecutiveErrors > 0 && consecutiveErrors%backoff.TransportResetThreshold == 0 {
		return s, fsmv2.SignalNone, action.NewResetTransportAction()
	}

	syncAction := action.NewSyncAction(snap.Observed.JWTToken)

	return s, fsmv2.SignalNone, syncAction
}

func (s *DegradedState) String() string {
	return "Degraded"
}

func (s *DegradedState) Reason() string {
	return "Sync is experiencing errors"
}
