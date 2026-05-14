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

package snapshot

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps/retry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// TransportDependencies is the dependencies interface for transport actions (avoids import cycles).
type TransportDependencies interface {
	deps.Dependencies
	MetricsRecorder() *deps.MetricsRecorder

	// Transport management
	GetTransport() types.Transport
	SetTransport(t types.Transport)

	// JWT token management
	SetJWT(token string, expiry time.Time)
	GetJWTToken() string
	GetJWTExpiry() time.Time

	// Error tracking for intelligent backoff
	RecordError()
	RecordSuccess()
	RecordTypedError(errType types.ErrorType, retryAfter time.Duration)
	RecordAuthError(errType types.ErrorType, retryAfter time.Duration)
	GetConsecutiveErrors() int
	GetPersistentAuthErrorCount() int
	GetLastErrorType() types.ErrorType
	GetLastRetryAfter() time.Duration

	// Auth attempt tracking
	SetLastAuthAttemptAt(t time.Time)
	GetLastAuthAttemptAt() time.Time

	// Instance identity from backend
	SetAuthenticatedUUID(uuid string)
	GetAuthenticatedUUID() string

	// Reset generation for parent-level transport reset signaling to children
	GetResetGeneration() uint64
	IncrementResetGeneration()

	// RetryTracker for backoff and modulo-trigger breaking after transport reset
	RetryTracker() retry.Tracker

	// Failed auth config tracking for AuthFailedState config-change detection
	SetFailedAuthConfig(token, relayURL, uuid string)
	GetFailedAuthConfig() (token, relayURL, uuid string)
}

// Compile-time check that TransportDesiredState implements fsmv2.DesiredState.
var _ fsmv2.DesiredState = (*TransportDesiredState)(nil)

// Deprecated: TransportDesiredState.ChildrenSpecs and GetChildrenSpecs are forward-deletion
// candidates. DeriveDesiredState now populates WrappedDesiredState.ChildrenSpecs (the wrapper
// field), not TransportDesiredState.ChildrenSpecs (the inner config field). The supervisor reads
// child specs via the ChildSpecProvider type assertion on the WrappedDesiredState wrapper, so
// this compile-time check is no longer load-bearing. Remove once all callers are confirmed gone.
var _ config.ChildSpecProvider = (*TransportDesiredState)(nil)

// TransportDesiredState represents the target configuration for the transport worker.
type TransportDesiredState struct {
	InstanceUUID string `json:"instanceUUID"` // Used by AuthenticateAction for backend authentication
	// TODO(security): AuthToken included in CSE sync payloads. ENG-4405 tracks
	// adding a CSE secret tier to persist locally but exclude from delta sync.
	AuthToken               string `json:"authToken"`
	RelayURL                string `json:"relayURL"`
	config.BaseDesiredState        // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

	// Deprecated: ChildrenSpecs on TransportDesiredState is never populated. Child specs are
	// set on WrappedDesiredState.ChildrenSpecs by DeriveDesiredState. This field exists only
	// to satisfy the legacy config.ChildSpecProvider interface check above.
	ChildrenSpecs []config.ChildSpec `json:"childrenSpecs,omitempty"`

	Timeout time.Duration `json:"timeout"`

	// State is the desired lifecycle state ("stopped" or "running").
	//
	// Deprecated: Never populated in production. Lifecycle is owned by
	// WrappedDesiredState.State, set from TransportUserSpec.GetState() in
	// DeriveDesiredState. Forward-deletion candidate alongside ChildrenSpecs /
	// GetChildrenSpecs / GetState / ShouldBeRunning — slated for the L9
	// transport-snapshot cleanup.
	State string `json:"state" yaml:"state"`
}

// GetChildrenSpecs returns the children specifications.
//
// Deprecated: This method is a forward-deletion candidate. The supervisor resolves child specs
// via WrappedDesiredState.GetChildrenSpecs(), not via TransportDesiredState.GetChildrenSpecs().
// ChildrenSpecs on TransportDesiredState is always empty in production code.
func (d *TransportDesiredState) GetChildrenSpecs() []config.ChildSpec {
	return d.ChildrenSpecs
}

// GetState returns the desired lifecycle state ("running" or "stopped").
func (d *TransportDesiredState) GetState() string {
	if d.State == "" {
		return config.DesiredStateRunning
	}

	return d.State
}

// ShouldBeRunning returns true if the transport should be running.
func (d *TransportDesiredState) ShouldBeRunning() bool {
	if d.ShutdownRequested {
		return false
	}

	return d.GetState() == config.DesiredStateRunning
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
// NOTE: JWTToken must NOT use json:"-" — the supervisor reconciliation loop
// serializes observed state to CSE storage between ticks and deserializes it
// via LoadObservedTyped(). Excluding JWTToken from JSON would force
// re-authentication on every tick (~10ms), hammering the relay server.
// TODO(security): JWTToken included in CSE sync payloads. ENG-4405 tracks
// adding a CSE secret tier to persist locally but exclude from delta sync.
type TransportStatus struct {
	JWTExpiry         time.Time        `json:"jwt_expiry,omitempty"`
	LastAuthAttemptAt time.Time        `json:"last_auth_attempt_at,omitempty"`
	FailedAuthConfig  FailedAuthConfig `json:"failed_auth_config,omitempty"`
	JWTToken          string           `json:"jwt_token,omitempty"`
	AuthenticatedUUID string           `json:"authenticated_uuid,omitempty"`
	LastErrorType     types.ErrorType  `json:"last_error_type"`
	LastRetryAfter    time.Duration    `json:"last_retry_after,omitempty"`
	TotalMessagesPushed int64          `json:"total_messages_pushed"`
	TotalMessagesPulled int64          `json:"total_messages_pulled"`
	ConsecutiveErrors   int            `json:"consecutive_errors"`
}

// IsTokenExpired returns true if the JWT token is expired or will expire within 10 minutes.
//
// Token buffer architecture: the parent TransportWorker uses a 10-minute buffer (proactive
// refresh trigger) while child workers (push/pull) use a 1-minute buffer via IsTokenValid().
// The 9-minute gap is safe by design: when IsTokenExpired triggers here, the parent
// transitions Running → Starting, which causes children to stop (they are not in ChildStartStates
// while the parent is Starting). Children never push or pull during the refresh window.
// The children's 1-minute buffer is a last-resort safety net for edge cases only.
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
