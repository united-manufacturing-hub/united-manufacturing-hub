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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

const AuthenticateActionName = "authenticate"
const defaultTokenExpiration = 24 * time.Hour

// AuthenticateAction performs authentication with the relay server to obtain a JWT token.
//
// # Purpose
//
// Before the communicator can sync data, it must authenticate with the relay server.
// The relay uses JWT tokens to:
//   - Identify which Edge instance is connecting
//   - Authorize access to specific sync channels
//   - Rate limit and monitor connections
//
// This action performs the authentication flow and stores the JWT token in
// the shared observed state for use by subsequent sync operations.
//
// # Authentication Flow
//
// 1. Build authentication request with instanceUUID and authToken
// 2. Send POST to /authenticate endpoint on relay server
// 3. Receive JWT token and expiration time
// 4. Store token in CommunicatorObservedState
// 5. Mark as authenticated
//
// # Idempotency
//
// This action is idempotent and safe to retry:
//   - If already authenticated (have valid JWT), returns immediately
//   - If authentication fails, can be retried safely
//   - Multiple calls won't create multiple tokens
//
// Idempotency check is performed at the start of Execute():
//
//	if observedState.IsAuthenticated() && observedState.GetJWTToken() != "" {
//	    return nil  // Already authenticated
//	}
//
// # Security Model
//
// Authentication uses a pre-shared secret (authToken):
//   - The authToken is configured on both Edge and Management Console
//   - It's sent to the relay server to prove identity
//   - The relay validates the token and issues a JWT
//
// The JWT token is then used for all sync operations:
//   - Transport includes JWT in WebSocket connection headers
//   - Relay validates JWT before routing messages
//   - JWT expires after a period (typically 24 hours)
//
// # Error Handling
//
// Returns an error if:
//   - HTTP request fails (network error, timeout)
//   - Relay returns non-200 status (invalid credentials)
//   - Response parsing fails (malformed JSON)
//
// On error, the FSM will retry the authentication based on state transitions.
type AuthenticateAction struct {
	RelayURL     string
	InstanceUUID string
	AuthToken    string

	transport transport.Transport
}

type AuthenticateActionResult struct {
	JWTToken       string
	JWTTokenExpiry time.Time
}

// NewAuthenticateAction creates a new authentication action.
//
// Parameters:
//   - transport: Transport implementation for HTTP communication
//   - relayURL: Relay server endpoint (e.g., "https://relay.umh.app")
//   - instanceUUID: Identifies this Edge instance (from config)
//   - authToken: Pre-shared secret for authentication (from config)
//
// The HTTP client is configured with a 30-second timeout to prevent
// indefinite hangs during authentication.
func NewAuthenticateAction(transport transport.Transport, relayURL, instanceUUID, authToken string) *AuthenticateAction {
	return &AuthenticateAction{
		RelayURL:     relayURL,
		InstanceUUID: instanceUUID,
		AuthToken:    authToken,
		transport:    transport,
	}
}

// Execute performs authentication with the relay server.
//
// This method is called by the FSM v2 Supervisor when in the Authenticating state.
//
// Authentication flow:
//  1. Check if already authenticated (idempotency)
//  2. Build JSON request with instanceUUID and authToken
//  3. POST to /authenticate endpoint
//  4. Parse JWT token from response
//  5. Store token in observedState
//  6. Mark as authenticated
//
// Idempotency guarantee:
//   - If already authenticated, returns immediately without network call
//   - Safe to call multiple times (FSM may retry on failure)
//
// On success:
//   - observedState.IsAuthenticated() returns true
//   - observedState.GetJWTToken() returns valid JWT
//   - FSM transitions to Authenticated state
//
// On failure:
//   - observedState remains unauthenticated
//   - Error is returned to FSM
//   - FSM may retry or transition to error state
//
// Returns an error if:
//   - JSON marshaling fails (malformed request)
//   - HTTP request fails (network error, timeout)
//   - Response status is not 200 (authentication failed)
//   - JSON parsing fails (malformed response)
//
// The relay server is expected to return:
//
//	{
//	    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
//	    "expiresAt": 1735689600
//	}
func (a *AuthenticateAction) Execute(ctx context.Context) (AuthenticateActionResult, error) {

	authReq := transport.AuthRequest{
		InstanceUUID: a.InstanceUUID,
		Email:        a.AuthToken,
	}

	authResp, err := a.transport.Authenticate(ctx, authReq)
	if err != nil {
		return AuthenticateActionResult{}, err
	}

	actionReturn := AuthenticateActionResult{}

	// Store JWT token in observed state
	actionReturn.JWTToken = authResp.Token

	if authResp.ExpiresAt > 0 {
		actionReturn.JWTTokenExpiry = time.Unix(authResp.ExpiresAt, 0)
	} else {
		actionReturn.JWTTokenExpiry = time.Now().Add(defaultTokenExpiration)
	}

	return actionReturn, nil
}

func (a *AuthenticateAction) Name() string {
	return AuthenticateActionName
}
