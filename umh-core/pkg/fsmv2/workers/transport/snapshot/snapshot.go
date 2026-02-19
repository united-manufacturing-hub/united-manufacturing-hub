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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// TransportDependencies is the dependencies interface for transport actions (avoids import cycles).
type TransportDependencies interface {
	deps.Dependencies
	MetricsRecorder() *deps.MetricsRecorder

	// Transport management
	GetTransport() transport.Transport
	SetTransport(t transport.Transport)

	// JWT token management
	SetJWT(token string, expiry time.Time)
	GetJWTToken() string
	GetJWTExpiry() time.Time

	// Error tracking for intelligent backoff
	RecordError()
	RecordSuccess()
	RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration)
	GetConsecutiveErrors() int
	GetLastErrorType() httpTransport.ErrorType
	GetLastRetryAfter() time.Duration

	// Auth attempt tracking
	SetLastAuthAttemptAt(t time.Time)
	GetLastAuthAttemptAt() time.Time

	// Instance identity from backend
	SetAuthenticatedUUID(uuid string)
	GetAuthenticatedUUID() string
}

// TransportSnapshot represents a point-in-time view of the transport worker state.
type TransportSnapshot struct {
	Desired  *TransportDesiredState
	Identity deps.Identity
	Observed TransportObservedState
}

// Compile-time check that TransportDesiredState implements fsmv2.DesiredState.
var _ fsmv2.DesiredState = (*TransportDesiredState)(nil)

// TransportDesiredState represents the target configuration for the transport worker.
type TransportDesiredState struct {
	InstanceUUID            string `json:"instanceUUID"` // Used by AuthenticateAction for backend authentication
	// TODO(security): AuthToken included in CSE sync payloads. ENG-4405 tracks
	// adding a CSE secret tier to persist locally but exclude from delta sync.
	AuthToken string `json:"authToken"`
	RelayURL                string `json:"relayURL"`
	config.BaseDesiredState        // Provides State, ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

	Timeout time.Duration `json:"timeout"`
}

// GetState returns the desired lifecycle state ("running" or "stopped").
func (d *TransportDesiredState) GetState() string {
	return d.BaseDesiredState.GetState()
}

// ShouldBeRunning returns true if the transport should be running.
func (d *TransportDesiredState) ShouldBeRunning() bool {
	if d.ShutdownRequested {
		return false
	}

	return d.GetState() == config.DesiredStateRunning
}

// TransportObservedState represents the current state of the transport worker.
type TransportObservedState struct {
	CollectedAt time.Time `json:"collected_at"`

	JWTExpiry time.Time `json:"jwt_expiry,omitempty"`

	// Children contains the observed state of child workers (PushWorker, PullWorker).
	Children map[string]fsmv2.ObservedState `json:"children,omitempty"`

	State string `json:"state"` // Observed lifecycle state (e.g., "running_healthy")

	// JWTToken is the current authentication token for relay communication.
	// NOTE: This field must NOT use json:"-" — the supervisor reconciliation loop
	// serializes observed state to CSE storage between ticks and deserializes it
	// via LoadObservedTyped(). Excluding JWTToken from JSON would force
	// re-authentication on every tick (~10ms), hammering the relay server.
	// TODO(security): JWTToken included in CSE sync payloads. ENG-4405 tracks
	// adding a CSE secret tier to persist locally but exclude from delta sync.
	JWTToken string `json:"jwt_token,omitempty"`

	// DesiredState embedded for state consistency
	TransportDesiredState `json:",inline"`

	// LastActionResults contains the action history from the last collection cycle (supervisor-managed).
	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	deps.MetricsEmbedder `json:",inline"`

	// TotalMessagesPushed tracks cumulative messages pushed to backend.
	TotalMessagesPushed int64 `json:"total_messages_pushed"`

	// TotalMessagesPulled tracks cumulative messages pulled from backend.
	TotalMessagesPulled int64 `json:"total_messages_pulled"`

	// ChildrenHealthy is the count of healthy child workers.
	ChildrenHealthy int `json:"children_healthy"`

	// ChildrenUnhealthy is the count of unhealthy child workers.
	ChildrenUnhealthy int `json:"children_unhealthy"`
}

// GetTimestamp returns when this observed state was collected.
func (o TransportObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// GetObservedDesiredState returns the desired state that is actually deployed.
func (o TransportObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.TransportDesiredState
}

// SetState sets the FSM state name on this observed state.
func (o TransportObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
func (o TransportObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}

// IsTokenExpired returns true if the JWT token is expired or will expire within 10 minutes.
//
// Token buffer architecture: the parent TransportWorker uses a 10-minute buffer (proactive
// refresh trigger) while child workers (push/pull) use a 1-minute buffer via IsTokenValid().
// The 9-minute gap is safe by design: when IsTokenExpired triggers here, the parent
// transitions Running → Starting, which causes children to stop (they are not in ChildStartStates
// while the parent is Starting). Children never push or pull during the refresh window.
// The children's 1-minute buffer is a last-resort safety net for edge cases only.
func (o TransportObservedState) IsTokenExpired() bool {
	if o.JWTExpiry.IsZero() {
		return false
	}

	const refreshBuffer = 10 * time.Minute

	return time.Now().Add(refreshBuffer).After(o.JWTExpiry)
}

// SetChildrenCounts sets the children health counts on this observed state.
func (o TransportObservedState) SetChildrenCounts(healthy, unhealthy int) fsmv2.ObservedState {
	o.ChildrenHealthy = healthy
	o.ChildrenUnhealthy = unhealthy

	return o
}

// HasValidToken returns true if there is a valid JWT token that hasn't expired.
func (o TransportObservedState) HasValidToken() bool {
	return o.JWTToken != "" && !o.IsTokenExpired()
}
