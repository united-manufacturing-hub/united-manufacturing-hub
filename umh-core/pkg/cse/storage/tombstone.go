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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var allRoles = []string{RoleIdentity, RoleDesired, RoleObserved}

type roleWrite struct {
	role   string
	syncID int64
	diff   *Diff
}

func hasTombstone(doc persistence.Document) bool {
	return doc[FieldDeletedAt] != nil
}

// MarkDeleted implements TriangularStoreInterface.MarkDeleted.
func (ts *TriangularStore) MarkDeleted(ctx context.Context, workerType string, id string, deletedBy string) error {
	err := ts.editRoleDocuments(ctx, workerType, id, func(doc persistence.Document, at time.Time) *Diff {
		if hasTombstone(doc) {
			return nil
		}

		doc[FieldDeletedAt] = at
		doc[FieldDeletedBy] = deletedBy

		return &Diff{
			Added:    map[string]interface{}{FieldDeletedAt: at, FieldDeletedBy: deletedBy},
			Modified: make(map[string]ModifiedField),
			Removed:  []string{},
		}
	})
	if err != nil {
		return fmt.Errorf("failed to mark %s/%s deleted: %w", workerType, id, err)
	}

	return nil
}

// ClearDeleted implements TriangularStoreInterface.ClearDeleted.
func (ts *TriangularStore) ClearDeleted(ctx context.Context, workerType string, id string) error {
	err := ts.editRoleDocuments(ctx, workerType, id, func(doc persistence.Document, _ time.Time) *Diff {
		if !hasTombstone(doc) {
			return nil
		}

		delete(doc, FieldDeletedAt)
		delete(doc, FieldDeletedBy)

		return &Diff{
			Added:    map[string]interface{}{},
			Modified: make(map[string]ModifiedField),
			Removed:  []string{FieldDeletedAt, FieldDeletedBy},
		}
	})
	if err != nil {
		return fmt.Errorf("failed to clear tombstone of %s/%s: %w", workerType, id, err)
	}

	return nil
}

// editRoleDocuments calls edit on each stored role document in one
// transaction. edit changes doc and returns its delta, or returns nil to
// leave doc unwritten.
func (ts *TriangularStore) editRoleDocuments(
	ctx context.Context,
	workerType, id string,
	edit func(doc persistence.Document, at time.Time) *Diff,
) error {
	ts.documentWriteMu.Lock()
	defer ts.documentWriteMu.Unlock()

	at := ts.clock.Now().UTC()

	tx, err := ts.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	var written []roleWrite

	for _, role := range allRoles {
		collection := workerType + "_" + role

		doc, err := tx.Get(ctx, collection, id)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				continue
			}

			return fmt.Errorf("failed to load %s for %s/%s: %w", role, workerType, id, err)
		}

		diff := edit(doc, at)
		if diff == nil {
			continue
		}

		syncID := ts.syncID.Add(1)
		doc[FieldSyncID] = syncID

		if err := tx.Update(ctx, collection, id, doc); err != nil {
			return fmt.Errorf("failed to update %s: %w", role, err)
		}

		written = append(written, roleWrite{role: role, syncID: syncID, diff: diff})
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if ts.deltaStore != nil {
		for _, w := range written {
			ts.appendDeltaOrWarn(ctx, DeltaEntry{
				SyncID:     w.syncID,
				WorkerType: workerType,
				ID:         id,
				Role:       w.role,
				Changes:    w.diff,
				Timestamp:  at,
			})
		}
	}

	ts.invalidateSnapshot(workerType, id)

	return nil
}

func (ts *TriangularStore) appendDeltaOrWarn(ctx context.Context, entry DeltaEntry) {
	appendErr := ts.deltaStore.Append(ctx, entry)
	if appendErr == nil {
		return
	}

	var hierarchyPath string

	if identity, loadErr := ts.LoadIdentity(ctx, entry.WorkerType, entry.ID); loadErr == nil {
		if hp, ok := identity["hierarchy_path"].(string); ok {
			hierarchyPath = hp
		}
	}

	ts.logger.SentryWarn(deps.FeatureCSE, hierarchyPath, "delta_append_failed",
		deps.Err(appendErr),
		deps.String("role", entry.Role))
}

func (ts *TriangularStore) invalidateSnapshot(workerType string, id string) {
	ts.cacheMutex.Lock()
	delete(ts.snapshotCache, workerType+"_"+id)
	ts.cacheMutex.Unlock()
}
