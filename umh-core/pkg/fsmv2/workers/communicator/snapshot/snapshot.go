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
}

type CommunicatorSnapshot struct {
	Identity fsmv2.Identity
	Observed CommunicatorObservedState
	Desired  CommunicatorDesiredState
}

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
//	MessagesToBeSent: Updated by agent when config changes need to be sent
//	Dependencies: Set at startup, never changes
//	Transport: DEPRECATED (use Dependencies.GetTransport() instead)
//
// # Field Validation
//
//   - RelayURL: MUST be non-empty URL (enforced: Worker constructor validates)
//   - InstanceUUID: MUST be non-empty UUID (enforced: Worker constructor validates)
//   - AuthToken: MUST be non-empty string (enforced: Worker constructor validates)
//   - MessagesToBeSent: May be empty array, elements must be valid UMHMessage
//   - Dependencies: MUST NOT be nil (enforced: C3 invariant)
//   - Transport: Deprecated, may be nil (use Dependencies.GetTransport())
//
// # Invariants
//
//   - Related to C3 (transport lifecycle): Dependencies must not be nil
//   - Related to C1 (auth precedence): RelayURL/AuthToken required for auth
//
// # Immutability
//
// This struct is immutable after creation. Desired state changes require
// new Worker instance or spec updates (future).
type CommunicatorDesiredState struct {
	config.BaseDesiredState          // Provides ShutdownRequested + IsShutdownRequested() + SetShutdownRequested()

	// Authentication
	InstanceUUID string
	AuthToken    string
	RelayURL     string

	// Messages
	MessagesToBeSent []transport.UMHMessage

	// Dependencies (passed from worker to states for action creation)
	Dependencies CommunicatorDependencies

	// Transport (passed from worker to states for action creation)
	// Deprecated: Use Dependencies instead
	Transport transport.Transport
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
	CommunicatorDesiredState

	// Authentication
	Authenticated bool
	JWTToken      string
	JWTExpiry     time.Time

	// Inbound Messages
	MessagesReceived []transport.UMHMessage
}

func (o CommunicatorObservedState) IsTokenExpired() bool {
	return time.Now().After(o.JWTExpiry)
}

func (o CommunicatorObservedState) IsSyncHealthy() bool {
	return o.Authenticated && !o.IsTokenExpired()
}

func (o CommunicatorObservedState) GetConsecutiveErrors() int {
	return 0
}

func (o CommunicatorObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.CommunicatorDesiredState
}

func (o CommunicatorObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}
