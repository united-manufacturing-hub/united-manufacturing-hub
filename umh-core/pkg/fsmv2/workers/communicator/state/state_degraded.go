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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// DegradedState handles error recovery with exponential backoff and periodic transport resets.
type DegradedState struct {
	BaseCommunicatorState
}

func (s *DegradedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixRunning, "degraded")

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "Shutdown requested during degraded state")
	}

	// If token is invalid, re-authenticate
	if snap.Observed.LastErrorType == httpTransport.ErrorTypeInvalidToken {
		return fsmv2.Result[any, any](&TryingToAuthenticateState{}, fsmv2.SignalNone, nil, "Token invalid, re-authenticating")
	}

	if snap.Observed.IsSyncHealthy() && snap.Observed.GetConsecutiveErrors() == 0 {
		return fsmv2.Result[any, any](&SyncingState{}, fsmv2.SignalNone, nil, "Recovered from degraded state")
	}

	consecutiveErrors := snap.Observed.GetConsecutiveErrors()
	backoffDelay := backoff.CalculateDelayForErrorType(
		snap.Observed.LastErrorType,
		consecutiveErrors,
		snap.Observed.LastRetryAfter,
	)

	// Build dynamic reason based on current error state
	reason := buildDegradedReason(snap.Observed.LastErrorType, consecutiveErrors, backoffDelay)

	enteredAt := snap.Observed.DegradedEnteredAt
	if !enteredAt.IsZero() && time.Since(enteredAt) < backoffDelay {
		return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, reason)
	}

	// Reset transport periodically to recover from connection-level issues
	if backoff.ShouldResetTransport(snap.Observed.LastErrorType, consecutiveErrors) {
		return fsmv2.Result[any, any](s, fsmv2.SignalNone, action.NewResetTransportAction(), "Resetting transport due to repeated failures")
	}

	syncAction := action.NewSyncAction(snap.Observed.JWTToken)

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, syncAction, reason)
}

func (s *DegradedState) String() string {
	return "Degraded"
}

// buildDegradedReason creates a dynamic reason string based on the current error state.
func buildDegradedReason(errorType httpTransport.ErrorType, consecutiveErrors int, backoffDelay time.Duration) string {
	errorTypeStr := mapErrorTypeToReason(errorType)

	return fmt.Sprintf("sync degraded: %d consecutive errors (%s), backoff %s",
		consecutiveErrors, errorTypeStr, backoffDelay.Round(time.Second))
}

// mapErrorTypeToReason maps error types to human-readable strings.
func mapErrorTypeToReason(errType httpTransport.ErrorType) string {
	switch errType {
	case httpTransport.ErrorTypeUnknown:
		return "unclassified_error"
	case httpTransport.ErrorTypeInvalidToken:
		return "authentication_failure"
	case httpTransport.ErrorTypeBackendRateLimit:
		return "backend_rate_limited"
	case httpTransport.ErrorTypeNetwork:
		return "network_connectivity_failure"
	case httpTransport.ErrorTypeServerError:
		return "server_error_response"
	case httpTransport.ErrorTypeCloudflareChallenge:
		return "cloudflare_challenge_encountered"
	case httpTransport.ErrorTypeProxyBlock:
		return "proxy_block_page_received"
	case httpTransport.ErrorTypeInstanceDeleted:
		return "instance_not_found_on_backend"
	default:
		return "consecutive_errors_threshold_exceeded"
	}
}
