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
)

// ResetTransportActionName is the unique identifier for this action.
const ResetTransportActionName = "reset_transport"

// ResetTransportAction resets the HTTP transport to establish fresh connections.
//
// # Purpose
//
// When the communicator enters a degraded state due to persistent failures (e.g.,
// 5 consecutive errors), this action resets the underlying HTTP transport. This can
// help resolve connection-level issues like:
//   - Stale TCP connections that appear alive but are actually dead
//   - DNS caching issues where the server IP has changed
//   - Corrupted connection pool state
//   - TLS session issues
//
// # When This Action Is Triggered
//
// DegradedState triggers this action when consecutive errors reach a multiple of
// the TransportResetThreshold (5). This means:
//   - At 5 errors: first reset
//   - At 10 errors: second reset
//   - At 15 errors: third reset, etc.
//
// This periodic reset approach allows multiple recovery attempts rather than
// a one-time reset that might not help.
//
// # Idempotency
//
// This action is idempotent and safe to call multiple times:
//   - Each call creates a fresh HTTP client
//   - Previous connections are closed before new client creation
//   - No persistent state is affected (JWT tokens, etc. remain valid)
//
// # Example
//
//	// DegradedState detects 5 consecutive errors
//	if consecutiveErrors > 0 && consecutiveErrors%TransportResetThreshold == 0 {
//	    return s, SignalNone, NewResetTransportAction()
//	}
type ResetTransportAction struct{}

// NewResetTransportAction creates a new transport reset action.
func NewResetTransportAction() *ResetTransportAction {
	return &ResetTransportAction{}
}

// Name returns the action name for logging and metrics.
func (a *ResetTransportAction) Name() string {
	return ResetTransportActionName
}

// Execute resets the transport to establish fresh connections.
//
// This method is called by the FSM v2 Supervisor when in DegradedState and
// the reset threshold is reached.
//
// Reset flow:
//  1. Get transport from dependencies
//  2. Call Reset() on the transport (closes idle connections, creates new client)
//  3. Return success (transport is ready for new requests)
//
// On success:
//   - Transport has a fresh HTTP client
//   - Next sync operation will use new connections
//
// Transport nil safety:
//   - ResetTransportAction is ONLY called from DegradedState
//   - DegradedState is only reachable AFTER SyncingState
//   - SyncingState is only reachable AFTER TryingToAuthenticateState
//   - TryingToAuthenticateState runs AuthenticateAction which creates the transport
//   - Therefore, transport is GUARANTEED to be non-nil when this action executes
//
// Error handling:
//   - Transport.Reset() does not return errors (best-effort operation)
func (a *ResetTransportAction) Execute(ctx context.Context, depsAny any) error {
	deps := depsAny.(CommunicatorDependencies)

	transport := deps.GetTransport()
	transport.Reset()
	deps.GetLogger().Infow("Transport reset completed", "reason", "degraded_state_threshold")

	return nil
}
