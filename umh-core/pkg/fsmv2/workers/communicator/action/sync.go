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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

const SyncActionName = "sync"

// SyncAction performs bidirectional message synchronization via HTTP transport.
//
// # Channel Protocol Architecture
//
// This action bridges the FSM v2 pattern with the channel-based sync protocol.
// It operates in two modes within separate goroutines:
//
// Pull mode (backend → edge):
//  1. HTTPTransport.Pull() fetches messages from relay server
//  2. Messages are decoded and queued into inboundChan
//  3. Local consumers read from inboundChan for processing
//
// Push mode (edge → backend):
//  1. Local producers write messages to outboundChan
//  2. Messages are batched from outboundChan
//  3. HTTPTransport.Push() sends batch to relay server
//
// # HTTP Transport Operations
//
// The HTTPTransport handles:
//   - GET /pull: Fetch pending messages from backend
//   - POST /push: Send batched messages to backend
//   - JWT token management (refresh on 401)
//   - Network error handling and exponential backoff
type SyncAction struct {
	JWTToken           string
	MessagesToBePushed []*transport.UMHMessage
	// Dependencies are received via Execute() parameter, not stored in struct
}

// TODO: docstring.
type SyncActionResult struct {
	PushedMessages []*transport.UMHMessage
	PulledMessages []*transport.UMHMessage
}

// NewSyncAction creates a new sync action.
//
// Parameters:
//   - jwtToken: JWT token for authentication
//
// Dependencies are injected via Execute() parameter by the supervisor,
// not passed to constructor. This ensures actions work correctly after
// DesiredState is loaded from storage (Dependencies can't be serialized).
func NewSyncAction(jwtToken string) *SyncAction {
	return &SyncAction{
		JWTToken: jwtToken,
	}
}

// Execute performs a sync tick using HTTP push/pull operations.
//
// This method is called by the FSM v2 Supervisor when in the Syncing state.
//
// Channel-based behavior:
//  1. Pull messages: HTTPTransport.Pull() → decode → inboundChan
//  2. Push messages: drain outboundChan → batch → HTTPTransport.Push()
//  3. Handle errors: log failures, retry with backoff
//
// Idempotency guarantee:
//   - Calling Execute() multiple times is safe
//   - Duplicate message detection handled by backend (not by FSM)
//   - Messages are not reprocessed within same tick
//
// Returns an error if:
//   - Context is cancelled
//   - HTTP transport fails critically (e.g., network unreachable)
//   - Authentication token expired (triggers re-authentication)
//
// Non-critical failures (e.g., channel full) are logged but not returned.
// This allows the sync loop to continue even if some operations fail.
func (a *SyncAction) Execute(ctx context.Context, depsAny any) error {
	// Cast dependencies from supervisor-injected parameter
	deps := depsAny.(CommunicatorDependencies)

	// 1. Pull messages from backend
	messages, err := deps.GetTransport().Pull(ctx, a.JWTToken)
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	// 2. Store pulled messages (they will be available in next observed state)
	deps.SetPulledMessages(messages)

	// 3. Push batch to backend if we have messages
	if len(a.MessagesToBePushed) > 0 {
		if err := deps.GetTransport().Push(ctx, a.JWTToken, a.MessagesToBePushed); err != nil {
			return fmt.Errorf("push failed: %w", err)
		}
	}

	return nil
}

func (a *SyncAction) Name() string {
	return SyncActionName
}
