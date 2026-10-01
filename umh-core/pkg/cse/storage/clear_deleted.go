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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// ClearDeleted implements TriangularStoreInterface.ClearDeleted.
func (ts *TriangularStore) ClearDeleted(ctx context.Context, workerType string, id string) error {
	ts.documentWriteMu.Lock()
	defer ts.documentWriteMu.Unlock()

	clearedAt := ts.clock.Now().UTC()

	tx, err := ts.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	var cleared []roleWrite

	for _, role := range tombstoneRoles {
		collection := workerType + "_" + role

		doc, err := tx.Get(ctx, collection, id)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				continue
			}

			return fmt.Errorf("failed to load %s for %s/%s: %w", role, workerType, id, err)
		}

		if !hasTombstone(doc) {
			continue
		}

		syncID := ts.syncID.Add(1)
		delete(doc, FieldDeletedAt)
		delete(doc, FieldDeletedBy)
		doc[FieldSyncID] = syncID

		if err := tx.Update(ctx, collection, id, doc); err != nil {
			return fmt.Errorf("failed to clear %s tombstone for %s/%s: %w", role, workerType, id, err)
		}

		cleared = append(cleared, roleWrite{role: role, syncID: syncID})
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if ts.deltaStore != nil {
		for _, w := range cleared {
			ts.appendDeltaOrWarn(ctx, DeltaEntry{
				SyncID:     w.syncID,
				WorkerType: workerType,
				ID:         id,
				Role:       w.role,
				Changes: &Diff{
					Added:    map[string]interface{}{},
					Modified: make(map[string]ModifiedField),
					Removed:  []string{FieldDeletedAt, FieldDeletedBy},
				},
				Timestamp: clearedAt,
			})
		}
	}

	ts.invalidateSnapshot(workerType, id)

	return nil
}
