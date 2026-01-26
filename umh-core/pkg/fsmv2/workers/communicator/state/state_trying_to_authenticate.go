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

// TryingToAuthenticateState obtains a JWT token via AuthenticateAction.
//
// Transitions:
//   - → SyncingState: when authenticated && !tokenExpired
//   - → StoppedState: if shutdown requested
//   - → self: loops emitting AuthenticateAction until success
//
// Enforces C1 (auth precedence), C2 (token expiry), C4 (shutdown priority).
type TryingToAuthenticateState struct {
	BaseCommunicatorState
}

func (s *TryingToAuthenticateState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixTryingToStart, "authentication")

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "Shutdown requested during authentication")
	}

	if snap.Observed.Authenticated && !snap.Observed.IsTokenExpired() {
		return fsmv2.Result[any, any](&SyncingState{}, fsmv2.SignalNone, nil, "Authentication successful, starting sync")
	}

	// Backoff to avoid hammering backend on repeated auth failures
	if snap.Observed.ConsecutiveErrors > 0 && !snap.Observed.LastAuthAttemptAt.IsZero() {
		delay := backoff.CalculateDelayForErrorType(
			snap.Observed.LastErrorType,
			snap.Observed.ConsecutiveErrors,
			snap.Observed.LastRetryAfter, // Respect server's Retry-After
		)
		if time.Since(snap.Observed.LastAuthAttemptAt) < delay {
			return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "Waiting for backoff to expire before retry")
		}
	}

	authenticateAction := action.NewAuthenticateAction(
		snap.Desired.RelayURL,
		snap.Desired.InstanceUUID,
		snap.Desired.AuthToken,
		snap.Desired.Timeout,
	)

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, authenticateAction, "Attempting to authenticate with relay server")
}

func (s *TryingToAuthenticateState) String() string {
	return "TryingToAuthenticate"
}
