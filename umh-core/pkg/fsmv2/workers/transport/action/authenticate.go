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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

const (
	AuthenticateActionName = "authenticate"
	// DefaultAuthenticateTimeout is the timeout for authentication requests when not specified.
	DefaultAuthenticateTimeout = 10 * time.Second
)

// AuthenticateAction obtains a JWT token from the relay server for sync operations.
//
// Flow: POST instanceUUID + authToken to relay -> receive JWT token + expiry -> store in deps.
// Idempotent: safe to retry on failure, multiple calls won't create multiple tokens.
// Creates transport on first execution if not present.
//
// Returns error on network failure, invalid credentials (non-200), or malformed response.
//
// Architecture compliance:
//   - Struct fields are read-only config (no mutable state) - Stateless Actions
//   - Checks ctx.Done() in Execute - Context Cancellation in Actions
//   - NO internal retry loops - Supervisor Manages Retries
//   - NO channel operations - Synchronous Actions
//   - Uses structured logging only (Infow, not Infof) - Structured Logging
type AuthenticateAction struct {
	RelayURL     string
	InstanceUUID string
	AuthToken    string
	Timeout      time.Duration
}

// NewAuthenticateAction creates a new authentication action.
// Timeout defaults to DefaultAuthenticateTimeout if 0. Dependencies injected via Execute().
func NewAuthenticateAction(relayURL, instanceUUID, authToken string, timeout time.Duration) *AuthenticateAction {
	if timeout == 0 {
		timeout = DefaultAuthenticateTimeout
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
	// Check context cancellation first (Architecture: Context Cancellation in Actions)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	deps, ok := depsAny.(snapshot.TransportDependencies)
	if !ok {
		return errors.New("invalid dependencies type: expected TransportDependencies")
	}

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
