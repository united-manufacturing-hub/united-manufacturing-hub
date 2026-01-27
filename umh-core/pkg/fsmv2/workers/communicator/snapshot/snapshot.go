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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// CommunicatorDependencies is the dependencies interface for communicator actions (avoids import cycles).
type CommunicatorDependencies interface {
	deps.Dependencies
	GetTransport() transport.Transport
	MetricsRecorder() *deps.MetricsRecorder
}

type CommunicatorSnapshot struct {
	Identity deps.Identity
	Desired  CommunicatorDesiredState
	Observed CommunicatorObservedState
}

var _ fsmv2.DesiredState = (*CommunicatorDesiredState)(nil)

// CommunicatorDesiredState represents the target configuration for the communicator.
type CommunicatorDesiredState struct {
	config.BaseDesiredState // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

	// Authentication - typed fields populated by DeriveDesiredState
	InstanceUUID string `json:"instanceUUID"` // Used by AuthenticateAction for backend authentication
	AuthToken    string `json:"authToken"`
	RelayURL     string `json:"relayURL"`

	// Messages
	MessagesToBeSent []transport.UMHMessage `json:"messagesToBeSent,omitempty"`
	Timeout          time.Duration          `json:"timeout"`
}

// GetState returns the desired lifecycle state ("running" or "stopped").
func (d *CommunicatorDesiredState) GetState() string {
	return d.State
}

// CommunicatorObservedState represents the current state of the communicator.
type CommunicatorObservedState struct {
	CollectedAt time.Time

	JWTExpiry         time.Time
	DegradedEnteredAt time.Time `json:"degradedEnteredAt,omitempty"` // When errors started (zero = not degraded)

	LastAuthAttemptAt time.Time `json:"lastAuthAttemptAt,omitempty"`

	LastErrorAt time.Time `json:"lastErrorAt,omitempty"` // When the last error occurred (for Retry-After timing)

	State string `json:"state"` // Observed lifecycle state (e.g., "running_connected")

	JWTToken          string
	AuthenticatedUUID string `json:"authenticatedUUID,omitempty"`

	// Inbound Messages
	MessagesReceived []transport.UMHMessage

	// DesiredState
	CommunicatorDesiredState `json:",inline"`

	deps.MetricsEmbedder `json:",inline"`

	// Error tracking for health monitoring
	ConsecutiveErrors int

	LastErrorType  httpTransport.ErrorType `json:"lastErrorType,omitempty"`
	LastRetryAfter time.Duration           `json:"lastRetryAfter,omitempty"` // Server-provided Retry-After duration

	// Backpressure indicates inbound channel is near full capacity.
	// When true, pull operations are skipped to prevent message loss.
	IsBackpressured bool `json:"isBackpressured,omitempty"`

	// Authentication
	Authenticated bool
}

// IsTokenExpired returns true if the JWT token is expired or will expire within 10 minutes.
func (o CommunicatorObservedState) IsTokenExpired() bool {
	if o.JWTExpiry.IsZero() {
		return false
	}

	const refreshBuffer = 10 * time.Minute

	return time.Now().Add(refreshBuffer).After(o.JWTExpiry)
}

// IsSyncHealthy returns true if authenticated, token valid, consecutive errors below threshold, and not backpressured.
func (o CommunicatorObservedState) IsSyncHealthy() bool {
	return o.Authenticated && !o.IsTokenExpired() && o.ConsecutiveErrors < backoff.TransportResetThreshold && !o.IsBackpressured
}

func (o CommunicatorObservedState) GetConsecutiveErrors() int {
	return o.ConsecutiveErrors
}

func (o CommunicatorObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.CommunicatorDesiredState
}

func (o CommunicatorObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// SetState sets the FSM state name on this observed state.
func (o CommunicatorObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
func (o CommunicatorObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}
