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

package transport

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// TransportConfig holds the user-provided configuration for the transport worker.
// Embeds BaseUserSpec to expose GetState() for WorkerBase.DeriveDesiredState,
// to extract the desired state from the "state" YAML field.
type TransportConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	RelayURL            string        `json:"relayURL"     yaml:"relayURL"`
	InstanceUUID        string        `json:"instanceUUID" yaml:"instanceUUID"`
	AuthToken           string        `json:"authToken"    yaml:"authToken"`
	Timeout             time.Duration `json:"timeout"      yaml:"timeout"`
}

// FailedAuthConfig captures the auth configuration that was used in the last
// permanently-failed auth attempt (InvalidToken or InstanceDeleted). Stored in
// dependencies, exposed via CollectObservedState, and compared against the current
// desired state by AuthFailedState to detect config changes that warrant a retry.
type FailedAuthConfig struct {
	AuthToken    string `json:"auth_token,omitempty"`
	RelayURL     string `json:"relay_url,omitempty"`
	InstanceUUID string `json:"instance_uuid,omitempty"`
}

// IsEmpty returns true if no failed auth config has been recorded.
func (f FailedAuthConfig) IsEmpty() bool {
	return f.AuthToken == "" && f.RelayURL == "" && f.InstanceUUID == ""
}

// TransportStatus holds the runtime observation data for the transport worker.
type TransportStatus struct {
	// JWTExpiry is when the current JWT token expires.
	JWTExpiry time.Time `json:"jwt_expiry,omitempty"`
	// LastAuthAttemptAt records when the last authentication attempt was made (for backoff gating).
	LastAuthAttemptAt time.Time `json:"last_auth_attempt_at,omitempty"`

	// FailedAuthConfig holds the auth config (token, relay URL, instance UUID) that was
	// used in the last permanently-failed auth attempt. Set by AuthenticateAction on
	// InvalidToken/InstanceDeleted, cleared by RecordSuccess. AuthFailedState compares
	// snap.Desired fields against this to detect config changes that warrant a retry.
	FailedAuthConfig FailedAuthConfig `json:"failed_auth_config,omitempty"`

	// AuthenticatedUUID is the instance UUID returned from the backend after authentication.
	AuthenticatedUUID string `json:"authenticated_uuid,omitempty"`

	// JWTToken is the current authentication token for relay communication.
	// NOTE: This field must NOT use json:"-" — the supervisor reconciliation loop
	// serializes observed state to CSE storage between ticks and deserializes it
	// via LoadObservedTyped(). Excluding JWTToken from JSON would force
	// re-authentication on every tick (~10ms), hammering the relay server.
	// TODO(security): JWTToken included in CSE sync payloads. ENG-4405 tracks
	// adding a CSE secret tier to persist locally but exclude from delta sync.
	JWTToken string `json:"jwt_token,omitempty"`

	// LastRetryAfter holds the server-suggested retry delay from the most recent error (for backoff).
	LastRetryAfter time.Duration `json:"last_retry_after,omitempty"`

	// ConsecutiveErrors tracks consecutive push/pull errors for transport reset decisions.
	ConsecutiveErrors int `json:"consecutive_errors"`
	// LastErrorType tracks the most recent error type for ShouldResetTransport evaluation.
	LastErrorType httpTransport.ErrorType `json:"last_error_type"`
}

// IsTokenExpired returns true if the JWT token is expired or will expire within 10 minutes.
//
// Token buffer architecture: the parent TransportWorker uses a 10-minute buffer (proactive
// refresh trigger) while child workers (push/pull) use a 1-minute buffer via IsTokenValid().
// The 9-minute gap is safe by design: when IsTokenExpired triggers here, the parent
// transitions Running → Starting. Children are always enabled and will continue
// running while the parent refreshes. The 1-minute child buffer is a safety net for edge cases only.
func (s TransportStatus) IsTokenExpired() bool {
	if s.JWTExpiry.IsZero() {
		return false
	}

	const refreshBuffer = 10 * time.Minute

	return time.Now().Add(refreshBuffer).After(s.JWTExpiry)
}

// HasValidToken returns true if there is a valid JWT token that hasn't expired.
func (s TransportStatus) HasValidToken() bool {
	return s.JWTToken != "" && !s.IsTokenExpired()
}
