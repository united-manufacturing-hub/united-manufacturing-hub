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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// CommunicatorDependencies represents the dependencies needed by communicator actions.
// This is an interface to avoid import cycles (communicator -> snapshot -> communicator).
type CommunicatorDependencies interface {
	fsmv2.Dependencies
	GetTransport() transport.Transport
	Metrics() *fsmv2.MetricsRecorder
}

type CommunicatorSnapshot struct {
	Identity fsmv2.Identity
	Observed CommunicatorObservedState
	Desired  CommunicatorDesiredState
}

// Compile-time interface check: CommunicatorDesiredState must implement fsmv2.DesiredState.
var _ fsmv2.DesiredState = (*CommunicatorDesiredState)(nil)

// CommunicatorDesiredState represents the target configuration for the communicator.
//
// # Lifecycle
//
// This structure is created by Worker.DeriveDesiredState() and represents
// what the communicator SHOULD be doing (not necessarily what it IS doing).
// It defines the target configuration that the FSM works toward achieving.
//
// # State Transitions
//
// For MVP, this state is static (always "not shutdown"). Future versions may
// derive from configuration (enable/disable sync, change relay URL).
//
//	RelayURL: Set at startup, never changes
//	InstanceUUID: Set at startup, never changes
//	AuthToken: Set at startup, never changes
//	Timeout: Set at startup, controls HTTP request timeout
//	MessagesToBeSent: Updated by agent when config changes need to be sent
//
// # Field Validation
//
//   - RelayURL: MUST be non-empty URL (enforced: Worker constructor validates)
//   - InstanceUUID: MUST be non-empty UUID (enforced: Worker constructor validates)
//   - AuthToken: MUST be non-empty string (enforced: Worker constructor validates)
//   - Timeout: Defaults to 10s if not specified
//   - MessagesToBeSent: May be empty array, elements must be valid UMHMessage
//
// # Invariants
//
//   - Related to C1 (auth precedence): RelayURL/AuthToken required for auth
//
// # Architecture Note
//
// Dependencies are NOT stored in DesiredState. They are injected via Execute()
// parameter by the supervisor (see reconciliation.go). Actions
// work correctly after DesiredState is loaded from storage (Dependencies can't
// be serialized).
//
// # Immutability
//
// This struct is immutable after creation. Desired state changes require
// new Worker instance or spec updates (future).
type CommunicatorDesiredState struct {
	config.BaseDesiredState // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

	// Authentication - typed fields populated by DeriveDesiredState
	InstanceUUID string        `json:"instanceUUID"`
	AuthToken    string        `json:"authToken"`
	RelayURL     string        `json:"relayURL"`
	Timeout      time.Duration `json:"timeout"`

	// Messages
	MessagesToBeSent []transport.UMHMessage `json:"messagesToBeSent,omitempty"`
	// Dependencies removed: Actions receive deps via Execute() parameter, not DesiredState
}

// GetState returns the desired lifecycle state ("running" or "stopped").
// This method satisfies the fsmv2.DesiredState interface.
func (d *CommunicatorDesiredState) GetState() string {
	return d.State
}

// CommunicatorObservedState represents the current state of the communicator.
//
// # Lifecycle
//
// This structure is created by Worker.CollectObservedState() on every supervisor
// tick and passed to states for decision-making. It is immutable after creation.
//
// # State Transitions
//
// Field changes over communicator lifecycle:
//
//	CollectedAt: Updated on every tick (always current time)
//	Authenticated: false (Stopped) → false (TryingToAuthenticate) → true (Syncing)
//	JWTToken: "" → "token-value" (set by AuthenticateAction)
//	JWTExpiry: zero → timestamp (set by AuthenticateAction, checked before sync)
//	MessagesReceived: empty → populated (updated by SyncAction on Pull)
//
// # Field Validation
//
//   - CollectedAt: MUST be non-zero (enforced: Worker always sets time.Now())
//   - Authenticated: true IFF JWTToken is non-empty AND JWTExpiry is future
//   - JWTToken: empty OR valid JWT string (no validation of format)
//   - JWTExpiry: zero OR future timestamp (checked by C2 invariant)
//   - MessagesReceived: May be empty array, elements must be valid UMHMessage
//   - CommunicatorDesiredState: Embedded desired state (always present)
//
// # Invariants
//
//   - Related to C2 (token expiry): JWTExpiry must be checked before sync
//   - Related to C1 (auth precedence): Authenticated=false blocks sync
//
// # Immutability
//
// This struct is immutable after creation. Actions create NEW observed states
// via Worker.CollectObservedState(), not modify existing instances.
type CommunicatorObservedState struct {
	CollectedAt time.Time

	// DesiredState
	CommunicatorDesiredState `json:",inline"`

	State string `json:"state"` // Observed lifecycle state (e.g., "running_connected")

	// Authentication
	Authenticated bool
	JWTToken      string
	JWTExpiry     time.Time

	// Inbound Messages
	MessagesReceived []transport.UMHMessage

	// Error tracking for health monitoring
	ConsecutiveErrors int

	// Sync metrics for observability
	// Uses standard fsmv2.Metrics structure for automatic Prometheus export
	Metrics fsmv2.Metrics `json:"metrics"`
}

// GetMetrics returns a pointer to the Metrics field.
// Implements fsmv2.MetricsHolder for automatic Prometheus export by supervisor.
func (o *CommunicatorObservedState) GetMetrics() *fsmv2.Metrics {
	return &o.Metrics
}

// IsTokenExpired returns true if the JWT token is expired or will expire soon.
// Uses a 10-minute buffer for proactive refresh to avoid authentication failures
// during sync operations. Returns false if JWTExpiry is zero (no expiration tracking).
func (o CommunicatorObservedState) IsTokenExpired() bool {
	// Zero expiry means no expiration tracking (e.g., token not yet obtained)
	if o.JWTExpiry.IsZero() {
		return false
	}

	// Consider token expired if it expires within 10 minutes (proactive refresh)
	const refreshBuffer = 10 * time.Minute

	return time.Now().Add(refreshBuffer).After(o.JWTExpiry)
}

func (o CommunicatorObservedState) IsSyncHealthy() bool {
	return o.Authenticated && !o.IsTokenExpired()
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
// Called by Collector when StateProvider callback is configured.
func (o CommunicatorObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

// SetShutdownRequested sets the shutdown requested status on this observed state.
// Called by Collector when ShutdownRequestedProvider callback is configured.
func (o CommunicatorObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}
