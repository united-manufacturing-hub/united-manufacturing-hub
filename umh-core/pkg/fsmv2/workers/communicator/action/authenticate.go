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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps/retry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// CommunicatorDependencies is an interface to avoid import cycles (action -> communicator -> snapshot -> action).
type CommunicatorDependencies interface {
	depspkg.Dependencies
	GetTransport() transport.Transport
	SetTransport(t transport.Transport)
	SetJWT(token string, expiry time.Time)
	SetPulledMessages(messages []*transport.UMHMessage)

	RecordError()
	RecordSuccess()
	GetConsecutiveErrors() int

	RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration)
	GetLastErrorType() httpTransport.ErrorType
	GetLastRetryAfter() time.Duration
	SetLastAuthAttemptAt(t time.Time)
	GetLastAuthAttemptAt() time.Time

	RetryTracker() retry.Tracker

	GetInboundChan() chan<- *transport.UMHMessage  // May return nil
	GetOutboundChan() <-chan *transport.UMHMessage // May return nil

	RecordPullSuccess(latency time.Duration, msgCount int)
	RecordPullFailure(latency time.Duration)
	RecordPushSuccess(latency time.Duration, msgCount int)
	RecordPushFailure(latency time.Duration)

	MetricsRecorder() *depspkg.MetricsRecorder

	// UUID exposed via CollectObservedState in ObservedState.AuthenticatedUUID
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
	Timeout      time.Duration
}

type AuthenticateActionResult struct {
	JWTTokenExpiry time.Time
	JWTToken       string
}

// NewAuthenticateAction creates a new authentication action.
// Timeout defaults to 10s if 0. Dependencies injected via Execute().
func NewAuthenticateAction(relayURL, instanceUUID, authToken string, timeout time.Duration) *AuthenticateAction {
	if timeout == 0 {
		timeout = 10 * time.Second
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
	deps := depsAny.(CommunicatorDependencies)

	if deps.GetTransport() == nil {
		newTransport := httpTransport.NewHTTPTransport(a.RelayURL, a.Timeout)
		deps.SetTransport(newTransport)
	}

	authReq := transport.AuthRequest{
		InstanceUUID: a.InstanceUUID,
		Email:        a.AuthToken,
	}

	deps.SetLastAuthAttemptAt(time.Now()) // Before attempt for backoff calculation

	authResp, err := deps.GetTransport().Authenticate(ctx, authReq)
	if err != nil {
		var transportErr *httpTransport.TransportError
		if errors.As(err, &transportErr) {
			deps.RecordTypedError(transportErr.Type, transportErr.RetryAfter)
			deps.MetricsRecorder().IncrementCounter(counterForErrorType(transportErr.Type), 1)
		} else {
			deps.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
			deps.MetricsRecorder().IncrementCounter(depspkg.CounterNetworkErrorsTotal, 1)
		}

		return err
	}

	deps.RecordSuccess()
	deps.SetJWT(authResp.Token, time.Unix(authResp.ExpiresAt, 0))

	logger := deps.GetLogger()
	logger.Infow("authentication_response_received",
		"instance_uuid", authResp.InstanceUUID,
		"instance_name", authResp.InstanceName,
		"has_token", authResp.Token != "",
	)

	if authResp.InstanceUUID != "" {
		logger.Infow("authenticated_uuid_stored",
			"uuid", authResp.InstanceUUID,
		)
		deps.SetAuthenticatedUUID(authResp.InstanceUUID)
	} else {
		logger.Warnw("instance_uuid_missing_in_auth_response")
	}

	return nil
}

// counterForErrorType maps ErrorType to Prometheus counter.
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
		return depspkg.CounterNetworkErrorsTotal
	default:
		return depspkg.CounterNetworkErrorsTotal
	}
}

func (a *AuthenticateAction) Name() string {
	return AuthenticateActionName
}
