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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/backoff"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

// StartingState represents the state where the transport worker is authenticating.
// It emits AuthenticateAction to obtain a JWT token from the relay server.
// Once authenticated, transitions to RunningState. Children start via ChildStartStates
// when the parent enters Running, avoiding a deadlock where children can't become healthy
// while the parent waits in Starting.
type StartingState struct {
	helpers.StartingBase
}

// Next evaluates the current snapshot and returns the next state or action.
func (s *StartingState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.TransportObservedState, *snapshot.TransportDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppingState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to Stopping")
	}

	// If we don't have a valid token, authenticate (with backoff on repeated failures)
	if !snap.Observed.HasValidToken() {
		configChanged := authConfigChanged(snap.Desired, snap.Observed)

		// Apply error handling only when config hasn't changed since last attempt.
		// If config changed, stale errors and backoff are irrelevant — go straight to auth dispatch.
		if !configChanged && snap.Observed.ConsecutiveErrors > 0 && !snap.Observed.LastAuthAttemptAt.IsZero() {
			if isPermanentAuthError(snap.Observed.LastErrorType) {
				return fsmv2.Result[any, any](&AuthFailedState{}, fsmv2.SignalNone, nil,
					fmt.Sprintf("permanent auth failure (%s after %d errors), entering AuthFailed",
						snap.Observed.LastErrorType, snap.Observed.ConsecutiveErrors))
			}

			delay := backoff.CalculateDelayForErrorType(
				snap.Observed.LastErrorType,
				snap.Observed.ConsecutiveErrors,
				snap.Observed.LastRetryAfter,
			)
			if time.Since(snap.Observed.LastAuthAttemptAt) < delay {
				return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
					fmt.Sprintf("auth backoff: %d errors (%s), delay %s",
						snap.Observed.ConsecutiveErrors, snap.Observed.LastErrorType, delay.Round(time.Second)))
			}
		}

		authAction := action.NewAuthenticateAction(
			snap.Desired.RelayURL,
			snap.Desired.InstanceUUID,
			snap.Desired.AuthToken,
			snap.Desired.Timeout,
		)

		return fsmv2.Result[any, any](s, fsmv2.SignalNone, authAction, "No valid token, authenticating with relay")
	}

	// Authenticated — transition to Running. Children start via ChildStartStates
	// once parent enters Running; RunningState handles unhealthy children.
	return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil, "Authenticated, transitioning to Running")
}

// isPermanentAuthError returns true for error types that indicate a configuration
// problem requiring human intervention (new token, re-registration).
//
// This is intentionally narrower than !IsTransient(): ProxyBlock, CloudflareChallenge,
// and ErrorTypeUnknown are non-transient but may self-resolve (infrastructure issues,
// not config problems). Only InvalidToken and InstanceDeleted warrant parking in
// AuthFailedState. SetFailedAuthConfig in the action uses the broader !IsTransient()
// guard so that config changes also skip backoff for those error types.
func isPermanentAuthError(errType httpTransport.ErrorType) bool {
	return errType == httpTransport.ErrorTypeInvalidToken ||
		errType == httpTransport.ErrorTypeInstanceDeleted
}

// authConfigChanged returns true if the current desired auth config differs from the
// config that was used in the last permanently-failed auth attempt. Used by StartingState
// to skip stale permanent errors after a config change. AuthFailedState performs the same
// comparison inline to capture per-field diagnostics in the reason string.
func authConfigChanged(desired *snapshot.TransportDesiredState, observed snapshot.TransportObservedState) bool {
	return desired.AuthToken != observed.FailedAuthConfig.AuthToken ||
		desired.RelayURL != observed.FailedAuthConfig.RelayURL ||
		desired.InstanceUUID != observed.FailedAuthConfig.InstanceUUID
}

// String returns the state name derived from the type.
func (s *StartingState) String() string {
	return helpers.DeriveStateName(s)
}
