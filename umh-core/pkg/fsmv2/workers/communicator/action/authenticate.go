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

package action

import (
	"context"
	"errors"
	"time"

	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// CommunicatorDependencies represents the dependencies needed by communicator actions.
// This is an interface to avoid import cycles (action -> communicator -> snapshot -> action).
type CommunicatorDependencies interface {
	depspkg.Dependencies
	GetTransport() transport.Transport
	// SetTransport sets the transport instance (mutex protected).
	// Called by AuthenticateAction on first execution.
	SetTransport(t transport.Transport)
	// SetJWT stores the JWT token and expiry from authentication response.
	// Called by AuthenticateAction after successful authentication.
	SetJWT(token string, expiry time.Time)
	// SetPulledMessages stores the messages retrieved from the backend.
	// Called by SyncAction after successful pull operation.
	SetPulledMessages(messages []*transport.UMHMessage)

	// Error tracking methods:
	// RecordError increments consecutive error count after HTTP failure.
	RecordError()
	// RecordSuccess resets consecutive error count after HTTP success.
	RecordSuccess()
	// GetConsecutiveErrors returns the current consecutive error count.
	GetConsecutiveErrors() int

	// Typed error tracking methods for intelligent backoff:
	// RecordTypedError records error type and retry-after for intelligent backoff.
	RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration)
	// GetLastErrorType returns the last recorded error type.
	GetLastErrorType() httpTransport.ErrorType
	// GetLastRetryAfter returns the Retry-After duration from the last error.
	GetLastRetryAfter() time.Duration
	// SetLastAuthAttemptAt records the timestamp of the last authentication attempt.
	SetLastAuthAttemptAt(t time.Time)
	// GetLastAuthAttemptAt returns the timestamp of the last authentication attempt.
	GetLastAuthAttemptAt() time.Time

	// Channel access methods for FSMv1 integration
	// GetInboundChan returns channel to write received messages.
	// May return nil if no channel provider was set.
	GetInboundChan() chan<- *transport.UMHMessage
	// GetOutboundChan returns channel to read messages for pushing.
	// May return nil if no channel provider was set.
	GetOutboundChan() <-chan *transport.UMHMessage

	// Sync metrics recording methods (store per-tick results)
	// RecordPullSuccess records a successful pull with latency and message count.
	RecordPullSuccess(latency time.Duration, msgCount int)
	// RecordPullFailure records a failed pull with latency.
	RecordPullFailure(latency time.Duration)
	// RecordPushSuccess records a successful push with latency and message count.
	RecordPushSuccess(latency time.Duration, msgCount int)
	// RecordPushFailure records a failed push with latency.
	RecordPushFailure(latency time.Duration)

	// MetricsRecorder returns the MetricsRecorder for actions to record metrics.
	// Actions call IncrementCounter/SetGauge with typed constants.
	MetricsRecorder() *depspkg.MetricsRecorder

	// SetAuthenticatedUUID stores the UUID returned by the backend after successful authentication.
	// Called by AuthenticateAction after successful authentication.
	// The UUID is then exposed via CollectObservedState in ObservedState.AuthenticatedUUID.
	SetAuthenticatedUUID(uuid string)
}

const AuthenticateActionName = "authenticate"

// AuthenticateAction obtains a JWT token from the relay server for sync operations.
//
// Flow: POST instanceUUID + authToken to relay → receive JWT token + expiry → store in deps.
// Idempotent: safe to retry on failure, multiple calls won't create multiple tokens.
// Creates transport on first execution if not present.
//
// Returns error on network failure, invalid credentials (non-200), or malformed response.
// See worker.go C1 (authentication precedence) and C3 (transport lifecycle).
type AuthenticateAction struct {
	RelayURL     string
	InstanceUUID string
	AuthToken    string
	Timeout      time.Duration // From CommunicatorUserSpec.Timeout
	// Dependencies are received via Execute() parameter, not stored in struct
}

type AuthenticateActionResult struct {
	JWTTokenExpiry time.Time
	JWTToken       string
}

// NewAuthenticateAction creates a new authentication action.
// Timeout defaults to 10s if 0. Dependencies injected via Execute().
func NewAuthenticateAction(relayURL, instanceUUID, authToken string, timeout time.Duration) *AuthenticateAction {
	if timeout == 0 {
		timeout = 10 * time.Second // Default timeout
	}

	return &AuthenticateAction{
		RelayURL:     relayURL,
		InstanceUUID: instanceUUID,
		AuthToken:    authToken,
		Timeout:      timeout,
	}
}

// Execute performs authentication with the relay server.
// Creates transport if not present, then POSTs auth request and stores JWT in deps.
// Records auth attempt timestamp and error type for intelligent backoff.
func (a *AuthenticateAction) Execute(ctx context.Context, depsAny any) error {
	// Cast dependencies from supervisor-injected parameter
	deps := depsAny.(CommunicatorDependencies)

	// Create transport if not yet created (first run)
	if deps.GetTransport() == nil {
		newTransport := httpTransport.NewHTTPTransport(a.RelayURL, a.Timeout)
		deps.SetTransport(newTransport)
	}

	authReq := transport.AuthRequest{
		InstanceUUID: a.InstanceUUID,
		Email:        a.AuthToken,
	}

	// Record auth attempt timestamp BEFORE the attempt (for backoff calculation)
	deps.SetLastAuthAttemptAt(time.Now())

	authResp, err := deps.GetTransport().Authenticate(ctx, authReq)
	if err != nil {
		// Extract error type and record typed error (with Retry-After if present)
		var transportErr *httpTransport.TransportError
		if errors.As(err, &transportErr) {
			deps.RecordTypedError(transportErr.Type, transportErr.RetryAfter)
			// Record metric for error type
			deps.MetricsRecorder().IncrementCounter(counterForErrorType(transportErr.Type), 1)
		} else {
			// Non-transport error (e.g., context canceled) - treat as network error
			deps.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			deps.MetricsRecorder().IncrementCounter(depspkg.CounterNetworkErrorsTotal, 1)
		}

		return err
	}

	// Success - clear error tracking
	deps.RecordSuccess()

	// Store JWT token in dependencies (will be read by CollectObservedState)
	deps.SetJWT(authResp.Token, time.Unix(authResp.ExpiresAt, 0))

	// Store authenticated UUID from backend response
	// This UUID is exposed via ObservedState.AuthenticatedUUID for polling by consumers
	logger := deps.GetLogger()
	logger.Infow("Authentication response received",
		"instanceUUID", authResp.InstanceUUID,
		"instanceName", authResp.InstanceName,
		"hasToken", authResp.Token != "",
	)

	if authResp.InstanceUUID != "" {
		logger.Infow("Storing authenticated UUID from backend",
			"uuid", authResp.InstanceUUID,
		)
		deps.SetAuthenticatedUUID(authResp.InstanceUUID)
	} else {
		logger.Warnw("Backend did not return instance UUID in auth response")
	}

	return nil
}

// counterForErrorType maps ErrorType to Prometheus counter.
// This function MUST handle ALL error types to avoid metric gaps.
func counterForErrorType(t httpTransport.ErrorType) depspkg.CounterName {
	switch t {
	case httpTransport.ErrorTypeCloudflareChallenge:
		return depspkg.CounterCloudflareErrorsTotal
	case httpTransport.ErrorTypeBackendRateLimit:
		return depspkg.CounterBackendRateLimitErrorsTotal
	case httpTransport.ErrorTypeInvalidToken:
		return depspkg.CounterAuthFailuresTotal
	case httpTransport.ErrorTypeInstanceDeleted:
		return depspkg.CounterInstanceDeletedTotal
	case httpTransport.ErrorTypeServerError:
		return depspkg.CounterServerErrorsTotal
	case httpTransport.ErrorTypeProxyBlock:
		return depspkg.CounterProxyBlockErrorsTotal
	case httpTransport.ErrorTypeNetwork:
		return depspkg.CounterNetworkErrorsTotal
	case httpTransport.ErrorTypeUnknown:
		return depspkg.CounterNetworkErrorsTotal // Unknown → network bucket
	default:
		return depspkg.CounterNetworkErrorsTotal
	}
}

func (a *AuthenticateAction) Name() string {
	return AuthenticateActionName
}
