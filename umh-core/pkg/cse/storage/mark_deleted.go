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

package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// MarkDeleted tombstones the worker's stored role documents; the
// TriangularStoreInterface method documents the contract.
//
// Parameters:
//   - workerType: e.g., "container"
//   - deletedBy: Actor responsible for the removal (audit trail)
func (ts *TriangularStore) MarkDeleted(ctx context.Context, workerType string, id string, deletedBy string) error {
	ts.documentWriteMu.Lock()
	defer ts.documentWriteMu.Unlock()

	deletedAt := ts.clock.Now().UTC()

	tx, err := ts.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	// tombstonedRole pairs a document's role with the sync id its tombstone
	// write allocated.
	type tombstonedRole struct {
		role   string
		syncID int64
	}

	// tombstoned holds one entry per document that received a tombstone in
	// this call.
	var tombstoned []tombstonedRole

	for _, role := range []string{RoleIdentity, RoleDesired, RoleObserved} {
		doc, err := tx.Get(ctx, workerType+"_"+role, id)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				continue
			}

			return fmt.Errorf("failed to load %s for %s/%s: %w", role, workerType, id, err)
		}

		// A tombstone is a non-nil _deleted_at: a document that already
		// carries one keeps the first tombstone.
		if existing, ok := doc[FieldDeletedAt]; ok && existing != nil {
			continue
		}

		syncID := ts.syncID.Add(1)
		doc[FieldDeletedAt] = deletedAt
		doc[FieldDeletedBy] = deletedBy
		doc[FieldSyncID] = syncID

		if err := tx.Update(ctx, workerType+"_"+role, id, doc); err != nil {
			return fmt.Errorf("failed to mark %s deleted for %s/%s: %w", role, workerType, id, err)
		}

		tombstoned = append(tombstoned, tombstonedRole{role: role, syncID: syncID})
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Each delta entry carries the sync id its document was allocated.
	for _, s := range tombstoned {
		if ts.deltaStore == nil {
			continue
		}

		entry := DeltaEntry{
			SyncID:     s.syncID,
			WorkerType: workerType,
			ID:         id,
			Role:       s.role,
			Changes: &Diff{
				Added: map[string]interface{}{
					FieldDeletedAt: deletedAt,
					FieldDeletedBy: deletedBy,
				},
				Modified: make(map[string]ModifiedField),
				Removed:  []string{},
			},
			Timestamp: deletedAt,
		}

		if appendErr := ts.deltaStore.Append(ctx, entry); appendErr != nil {
			var hierarchyPath string
			if identity, loadErr := ts.LoadIdentity(ctx, workerType, id); loadErr == nil {
				if hp, ok := identity["hierarchy_path"].(string); ok {
					hierarchyPath = hp
				}
			}

			ts.logger.SentryWarn(deps.FeatureCSE, hierarchyPath, "delta_append_failed",
				deps.Err(appendErr),
				deps.String("role", s.role))
		}
	}

	cacheKey := workerType + "_" + id

	ts.cacheMutex.Lock()
	delete(ts.snapshotCache, cacheKey)
	ts.cacheMutex.Unlock()

	return nil
}
