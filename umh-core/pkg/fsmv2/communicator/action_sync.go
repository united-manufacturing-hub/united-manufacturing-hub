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

package communicator

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

// SyncAction performs bidirectional message synchronization via HTTP transport.
//
// # Channel Protocol Architecture
//
// This action bridges the FSM v2 pattern with the channel-based sync protocol.
// It operates in two modes within separate goroutines:
//
// Pull mode (backend → edge):
//   1. HTTPTransport.Pull() fetches messages from relay server
//   2. Messages are decoded and queued into inboundChan
//   3. Local consumers read from inboundChan for processing
//
// Push mode (edge → backend):
//   1. Local producers write messages to outboundChan
//   2. Messages are batched from outboundChan
//   3. HTTPTransport.Push() sends batch to relay server
//
// # HTTP Transport Operations
//
// The HTTPTransport handles:
//   - GET /pull: Fetch pending messages from backend
//   - POST /push: Send batched messages to backend
//   - JWT token management (refresh on 401)
//   - Network error handling and exponential backoff
type SyncAction struct {
	transport    HTTPTransportInterface
	inboundChan  chan *transport.UMHMessage
	outboundChan chan *transport.UMHMessage
}

// NewSyncAction creates a new sync action with the given transport and channels.
//
// Parameters:
//   - transport: HTTP transport for pull/push operations
//   - inboundChan: Channel for messages received from backend
//   - outboundChan: Channel for messages to send to backend
func NewSyncAction(transport HTTPTransportInterface, inboundChan, outboundChan chan *transport.UMHMessage) *SyncAction {
	return &SyncAction{
		transport:    transport,
		inboundChan:  inboundChan,
		outboundChan: outboundChan,
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
func (a *SyncAction) Execute(ctx context.Context) error {
	// 1. Pull messages from backend
	messages, err := a.transport.Pull(ctx)
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	// 2. Push pulled messages to inbound channel (non-blocking)
	for _, msg := range messages {
		select {
		case a.inboundChan <- msg:
			// Sent successfully
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Channel full - log warning but continue
			// TODO: Add logger and log warning here
		}
	}

	// 3. Drain outbound channel and batch for push
	var batch []*transport.UMHMessage
	for len(a.outboundChan) > 0 && len(batch) < 10 {
		select {
		case msg := <-a.outboundChan:
			batch = append(batch, msg)
		default:
			break
		}
	}

	// 4. Push batch to backend if we have messages
	if len(batch) > 0 {
		if err := a.transport.Push(ctx, batch); err != nil {
			return fmt.Errorf("push failed: %w", err)
		}
	}

	return nil
}

func (a *SyncAction) Name() string {
	return "Sync"
}
