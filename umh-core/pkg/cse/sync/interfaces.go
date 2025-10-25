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

package sync

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

// SyncStateInterface tracks synchronization progress between Frontend and Edge tiers.
//
// # Purpose
//
// This interface manages the "memory" of the sync protocol:
//   - What has been synced to each tier (sync IDs)
//   - What still needs to be synced (pending changes)
//   - How to query for unsent changes (delta queries)
//
// # Responsibilities
//
// Track sync progress:
//   - GetEdgeSyncID/SetEdgeSyncID: Edge tier's highest sync ID
//   - GetFrontendSyncID/SetFrontendSyncID: Frontend tier's highest sync ID
//
// Manage pending changes:
//   - RecordChange: Add a new change to the pending list
//   - MarkSynced: Remove synced changes from pending list
//   - GetPendingChanges: List changes waiting to sync
//
// Build efficient queries:
//   - GetDeltaSince: Create query for changes since last sync
//
// Persist state across restarts:
//   - Flush: Save state to storage
//   - Load: Restore state from storage
//
// # Sync ID Semantics
//
// Each tier maintains its own monotonically increasing sync ID:
//   - Edge creates changes with IDs 1, 2, 3, ...
//   - Frontend creates changes with IDs 1, 2, 3, ...
//
// The sync IDs are INDEPENDENT. Edge sync ID 100 is unrelated to
// Frontend sync ID 100. They represent different change sequences.
//
// Each tier tracks TWO sync IDs:
//   - Its own highest ID (changes created locally)
//   - Remote tier's highest ID (changes received from remote)
//
// Example on Edge tier:
//   - EdgeSyncID = 150 (Edge created 150 changes)
//   - FrontendSyncID = 200 (Edge received changes up to Frontend's ID 200)
//
// # MarkSynced Behavior
//
// CRITICAL: MarkSynced(tier, syncID) updates the OTHER tier's sync ID, not the
// current tier's sync ID.
//
// When Edge calls MarkSynced(TierEdge, 150):
//   - Updates FrontendSyncID to 150 (Frontend now knows Edge sent up to 150)
//   - Does NOT update EdgeSyncID
//
// This is because MarkSynced means "I successfully sent my changes to the
// remote tier, so the remote tier now has my changes up to syncID".
//
// # Testing Strategy
//
// The interface enables testing with mock implementations:
//   - MockSyncState for unit tests
//   - In-memory SyncState for integration tests
//   - Persistent SyncState for production
//
// This separation allows testing sync logic without requiring a database.
//
// # Implementation Notes
//
// Implementations must:
//   - Be thread-safe (called from FSM Supervisor and orchestrator)
//   - Handle missing state gracefully (return 0 for IDs, empty for pending)
//   - Persist state atomically (Flush should be transactional)
type SyncStateInterface interface {
	// GetEdgeSyncID returns the current sync ID for the Edge tier.
	// This represents the last change created or received at the Edge.
	GetEdgeSyncID() int64

	// SetEdgeSyncID updates the Edge tier's sync ID.
	// Used when restoring state or after receiving changes from Frontend.
	SetEdgeSyncID(syncID int64) error

	// GetFrontendSyncID returns the current sync ID for the Frontend tier.
	// This represents the last change created or received at the Frontend.
	GetFrontendSyncID() int64

	// SetFrontendSyncID updates the Frontend tier's sync ID.
	// Used when restoring state or after receiving changes from Edge.
	SetFrontendSyncID(syncID int64) error

	// RecordChange tracks a new change for synchronization to the other tier.
	// The change is added to the pending list for the specified tier.
	RecordChange(syncID int64, tier protocol.Tier) error

	// MarkSynced marks changes up to syncID as successfully synced to the other tier.
	// Removes synced changes from the pending list and updates the other tier's sync ID.
	// IMPORTANT: Updates the OTHER tier's sync ID, not the current tier's.
	MarkSynced(tier protocol.Tier, syncID int64) error

	// GetPendingChanges returns the list of sync IDs pending synchronization for a tier.
	// These are changes that have been recorded but not yet marked as synced.
	GetPendingChanges(tier protocol.Tier) ([]int64, error)

	// GetDeltaSince constructs a query to fetch changes since the last sync for a tier.
	// Returns a query that can be executed against the store to get all changes
	// with _sync_id greater than the last synced ID.
	GetDeltaSince(tier protocol.Tier) (*basic.Query, error)

	// Flush persists the current sync state to storage.
	// Should be called after successful sync operations to ensure state survives restarts.
	Flush(ctx context.Context) error

	// Load restores sync state from storage.
	// Should be called on startup to resume from the last known state.
	Load(ctx context.Context) error
}

// Compile-time check that SyncState implements SyncStateInterface
var _ SyncStateInterface = (*SyncState)(nil)