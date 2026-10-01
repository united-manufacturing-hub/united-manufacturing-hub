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

// tombstoneRoles are the role records MarkDeleted tombstones and
// ClearDeleted clears.
var tombstoneRoles = []string{RoleIdentity, RoleDesired, RoleObserved}

type roleWrite struct {
	role   string
	syncID int64
}

func hasTombstone(doc persistence.Document) bool {
	return doc[FieldDeletedAt] != nil
}

// tombstoneChange holds the per-operation parts of a tombstone write, so one
// function can serve both MarkDeleted and ClearDeleted.
type tombstoneChange struct {
	appliesTo   func(doc persistence.Document) bool
	edit        func(doc persistence.Document, at time.Time, deletedBy string)
	diff        func(at time.Time, deletedBy string) *Diff
	updateError func(role string, workerType string, id string, err error) error
}

var markDeletedChange = tombstoneChange{
	appliesTo: func(doc persistence.Document) bool {
		return !hasTombstone(doc)
	},
	edit: func(doc persistence.Document, at time.Time, deletedBy string) {
		doc[FieldDeletedAt] = at
		doc[FieldDeletedBy] = deletedBy
	},
	diff: func(at time.Time, deletedBy string) *Diff {
		return &Diff{
			Added: map[string]interface{}{
				FieldDeletedAt: at,
				FieldDeletedBy: deletedBy,
			},
			Modified: make(map[string]ModifiedField),
			Removed:  []string{},
		}
	},
	updateError: func(role string, workerType string, id string, err error) error {
		return fmt.Errorf("failed to mark %s deleted for %s/%s: %w", role, workerType, id, err)
	},
}

var clearDeletedChange = tombstoneChange{
	appliesTo: hasTombstone,
	edit: func(doc persistence.Document, _ time.Time, _ string) {
		delete(doc, FieldDeletedAt)
		delete(doc, FieldDeletedBy)
	},
	diff: func(time.Time, string) *Diff {
		return &Diff{
			Added:    map[string]interface{}{},
			Modified: make(map[string]ModifiedField),
			Removed:  []string{FieldDeletedAt, FieldDeletedBy},
		}
	},
	updateError: func(role string, workerType string, id string, err error) error {
		return fmt.Errorf("failed to clear %s tombstone for %s/%s: %w", role, workerType, id, err)
	},
}

// MarkDeleted implements TriangularStoreInterface.MarkDeleted.
func (ts *TriangularStore) MarkDeleted(ctx context.Context, workerType string, id string, deletedBy string) error {
	return ts.applyTombstoneChange(ctx, workerType, id, deletedBy, markDeletedChange)
}

// ClearDeleted implements TriangularStoreInterface.ClearDeleted.
func (ts *TriangularStore) ClearDeleted(ctx context.Context, workerType string, id string) error {
	return ts.applyTombstoneChange(ctx, workerType, id, "", clearDeletedChange)
}

// applyTombstoneChange edits every role record the change applies to in one
// transaction, bumps each edited record's sync id, and appends one change
// record per edited role.
func (ts *TriangularStore) applyTombstoneChange(
	ctx context.Context,
	workerType string,
	id string,
	deletedBy string,
	change tombstoneChange,
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

	for _, role := range tombstoneRoles {
		collection := workerType + "_" + role

		doc, err := tx.Get(ctx, collection, id)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				continue
			}

			return fmt.Errorf("failed to load %s for %s/%s: %w", role, workerType, id, err)
		}

		if !change.appliesTo(doc) {
			continue
		}

		syncID := ts.syncID.Add(1)

		change.edit(doc, at, deletedBy)
		doc[FieldSyncID] = syncID

		if err := tx.Update(ctx, collection, id, doc); err != nil {
			return change.updateError(role, workerType, id, err)
		}

		written = append(written, roleWrite{role: role, syncID: syncID})
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
				Changes:    change.diff(at, deletedBy),
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
