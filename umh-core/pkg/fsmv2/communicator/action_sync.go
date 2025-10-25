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

	csesync "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/sync"
)

// SyncAction performs delta synchronization using the CSE sync orchestrator.
//
// # Purpose
//
// This action is the bridge between the FSM v2 pattern and the CSE sync protocol.
// It delegates all sync operations to the orchestrator, which handles:
//   - Checking for pending changes
//   - Building and sending sync messages
//   - Receiving and processing incoming messages
//   - Tracking sync progress
//
// # Idempotency
//
// This action is idempotent and safe to call repeatedly:
//   - If no new changes exist, Tick() returns immediately
//   - If no incoming messages exist, Tick() returns immediately
//   - Calling Execute() multiple times only syncs NEW changes
//
// This is critical for the FSM v2 pattern, which may retry actions on failure.
// The action won't resend already-synced changes or reprocess messages.
//
// # When This Action Runs
//
// The FSM executes this action in the Syncing state, which is the steady-state
// for a connected and authenticated communicator. The action runs on every
// supervisor tick (typically 100ms intervals).
//
// This frequent execution enables:
//   - Low-latency sync (changes propagate quickly)
//   - Responsive message processing
//   - Quick failure detection
//
// # Error Handling
//
// Errors from the orchestrator are returned to the FSM, which will:
//   - Log the error
//   - Potentially transition to an error state
//   - Retry based on FSM state machine logic
//
// The orchestrator handles its own internal error recovery (retries, backoff).
// Only critical failures that should stop the sync loop are returned.
type SyncAction struct {
	orchestrator csesync.OrchestratorInterface
}

// NewSyncAction creates a new sync action with the given orchestrator.
//
// The orchestrator must be fully configured with:
//   - Transport for sending/receiving messages
//   - Crypto for encryption/decryption (optional)
//   - Authorizer for permission checks (optional)
//   - SyncState for tracking progress
//   - Storage for applying changes
//
// The action does not take ownership of the orchestrator - it may be shared
// with other components (e.g., for status monitoring).
func NewSyncAction(orchestrator csesync.OrchestratorInterface) *SyncAction {
	return &SyncAction{
		orchestrator: orchestrator,
	}
}

// Execute performs a sync tick, checking for pending changes and
// processing incoming messages.
//
// This method is called by the FSM v2 Supervisor when in the Syncing state.
// It delegates to the orchestrator's Tick() method, which:
//  1. Checks for pending changes via SyncState
//  2. Calls StartSync() if changes exist
//  3. Polls Transport for incoming messages
//  4. Calls ProcessIncoming() for each message
//
// Idempotency guarantee:
//   - Calling Execute() multiple times is safe
//   - Only NEW changes are synced (not already-sent changes)
//   - Messages are not reprocessed
//
// Returns an error if:
//   - Context is cancelled
//   - A critical failure occurs (e.g., transport disconnected)
//
// Non-critical failures (e.g., individual message processing errors) are
// logged by the orchestrator but not returned. This allows the sync loop
// to continue even if some operations fail.
func (a *SyncAction) Execute(ctx context.Context) error {
	// Delegate to orchestrator's tick method
	// This will check for pending changes, retry failed syncs,
	// and process incoming messages
	return a.orchestrator.Tick(ctx)
}

func (a *SyncAction) Name() string {
	return "Sync"
}
