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
	"fmt"
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
// Returns nil on classified TransportError (tracked via deps for snapshot-based backoff).
// Returns error on context cancellation, invalid dependency type, or non-TransportError
// (programming bugs that must propagate to the executor).
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
		Email:        a.AuthToken, // Email field matches backend API JSON contract; carries auth token
	}

	deps.SetLastAuthAttemptAt(time.Now()) // Before attempt for backoff calculation

	authResp, err := deps.GetTransport().Authenticate(ctx, authReq)
	if err != nil {
		// Context cancellation propagates immediately for shutdown.
		if ctx.Err() != nil {
			return fmt.Errorf("authentication failed (context canceled): %w", ctx.Err())
		}

		// Only suppress classified TransportErrors. Non-TransportErrors are programming
		// bugs that must propagate to the executor for SentryError.
		// Note: unlike push/pull (which only suppress transient errors), auth suppresses
		// ALL classified TransportErrors because persistent auth errors are handled via
		// snapshot-based state transitions (AuthFailedState), not error propagation.
		var transportErr *httpTransport.TransportError
		if !errors.As(err, &transportErr) {
			return err
		}

		errType, retryAfter := transportErr.Type, transportErr.RetryAfter
		deps.RecordTypedError(errType, retryAfter)
		deps.MetricsRecorder().IncrementCounter(httpTransport.CounterForErrorType(errType), 1)

		// First-occurrence SentryWarn: alert once per failure episode, not on every retry.
		// Resets when RecordSuccess() clears consecutiveErrors to 0.
		if deps.GetConsecutiveErrors() == 1 {
			deps.GetLogger().SentryWarn(depspkg.FeatureCommunicator, deps.GetHierarchyPath(), "authentication_failed",
				depspkg.Err(err), depspkg.String("errorType", errType.String()))
		}

		// Return nil for classified TransportErrors. The state machine reads ConsecutiveErrors
		// and LastErrorType from the snapshot for backoff decisions (StartingState.Next()).
		// Returning nil suppresses the executor's SentryError("action_failed"), which is
		// appropriate because auth failures are expected business errors, not programming
		// errors.
		return nil
	}

	deps.RecordSuccess()
	deps.SetJWT(authResp.Token, time.Unix(authResp.ExpiresAt, 0))

	logger := deps.GetLogger()
	logger.Info("authentication_response_received",
		depspkg.String("instance_uuid", authResp.InstanceUUID),
		depspkg.String("instance_name", authResp.InstanceName),
		depspkg.Bool("has_token", authResp.Token != ""),
	)

	if authResp.InstanceUUID != "" {
		logger.Info("authenticated_uuid_stored",
			depspkg.String("uuid", authResp.InstanceUUID),
		)
		deps.SetAuthenticatedUUID(authResp.InstanceUUID)
	} else {
		logger.SentryWarn(depspkg.FeatureCommunicator, deps.GetHierarchyPath(), "instance_uuid_missing_in_auth_response",
			depspkg.String("instance_name", authResp.InstanceName),
			depspkg.Bool("has_token", authResp.Token != ""))
	}

	return nil
}

func (a *AuthenticateAction) Name() string {
	return AuthenticateActionName
}
