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

// ClearDeleted removes the tombstone MarkDeleted wrote from a worker's stored
// role documents, so a worker added again starts without one; the
// TriangularStoreInterface method documents the contract.
func (ts *TriangularStore) ClearDeleted(ctx context.Context, workerType string, id string) error {
	ts.documentWriteMu.Lock()
	defer ts.documentWriteMu.Unlock()

	clearedAt := ts.clock.Now().UTC()

	tx, err := ts.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	// clearedRole pairs a document's role with the sync id its clearing
	// delta allocated.
	type clearedRole struct {
		role   string
		syncID int64
	}

	// cleared holds one entry per document that lost its tombstone in this
	// call.
	var cleared []clearedRole

	for _, role := range tombstoneRoles {
		doc, err := tx.Get(ctx, workerType+"_"+role, id)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				continue
			}

			return fmt.Errorf("failed to load %s for %s/%s: %w", role, workerType, id, err)
		}

		if deletedAt, ok := doc[FieldDeletedAt]; !ok || deletedAt == nil {
			continue
		}

		syncID := ts.syncID.Add(1)
		delete(doc, FieldDeletedAt)
		delete(doc, FieldDeletedBy)
		doc[FieldSyncID] = syncID

		if err := tx.Update(ctx, workerType+"_"+role, id, doc); err != nil {
			return fmt.Errorf("failed to clear %s deleted for %s/%s: %w", role, workerType, id, err)
		}

		cleared = append(cleared, clearedRole{role: role, syncID: syncID})
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	for _, c := range cleared {
		if ts.deltaStore == nil {
			continue
		}

		entry := DeltaEntry{
			SyncID:     c.syncID,
			WorkerType: workerType,
			ID:         id,
			Role:       c.role,
			Changes: &Diff{
				Added:    map[string]interface{}{},
				Modified: make(map[string]ModifiedField),
				Removed:  []string{FieldDeletedAt, FieldDeletedBy},
			},
			Timestamp: clearedAt,
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
				deps.String("role", c.role))
		}
	}

	cacheKey := workerType + "_" + id

	ts.cacheMutex.Lock()
	delete(ts.snapshotCache, cacheKey)
	ts.cacheMutex.Unlock()

	return nil
}
