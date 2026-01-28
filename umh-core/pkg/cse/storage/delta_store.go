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
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// DeltaCollectionName is the single unified collection for all deltas.
const DeltaCollectionName = "_deltas"

// DeltaStore persists deltas using the existing persistence.Store.
// This is NOT a new storage backend - it uses the same persistence layer
// as TriangularStore, just with a single unified delta collection.
//
// DESIGN DECISION: Single unified delta collection (not per-workerType).
// WHY: When a new worker is created, that delta goes into the event log.
// Subscribers learn about new workers through the deltas themselves.
// This makes "get all deltas since X" trivial - just query one collection.
//
// DESIGN DECISION: Persist deltas to DB (not just in-memory buffer)
// WHY: Allows clients to reconnect after longer periods without requiring
// full bootstrap. Supports offline-first sync patterns.
//
// TRADE-OFF: More storage usage, but deltas are small (just changes, not full docs).
// Compaction can clean up old deltas periodically.
type DeltaStore struct {
	store persistence.Store
}

// NewDeltaStore creates a new DeltaStore using the provided persistence.Store.
// The same store instance can be shared with TriangularStore.
func NewDeltaStore(store persistence.Store) *DeltaStore {
	return &DeltaStore{store: store}
}

// Append adds a delta to persistent storage.
func (ds *DeltaStore) Append(ctx context.Context, entry DeltaEntry) error {
	if ds.store == nil {
		return nil
	}

	doc := persistence.Document{
		"id":          strconv.FormatInt(entry.SyncID, 10),
		"sync_id":     entry.SyncID,
		"worker_type": entry.WorkerType,
		"worker_id":   entry.ID,
		"role":        entry.Role,
		"timestamp":   entry.Timestamp.UnixMilli(),
	}

	if entry.Changes != nil {
		changesJSON, err := json.Marshal(entry.Changes)
		if err != nil {
			return fmt.Errorf("failed to marshal changes: %w", err)
		}

		doc["changes"] = string(changesJSON)
	}

	_, err := ds.store.Insert(ctx, DeltaCollectionName, doc)
	if err != nil {
		return fmt.Errorf("failed to insert delta: %w", err)
	}

	return nil
}

// GetSince returns deltas for a specific worker type and role since sinceSyncID.
func (ds *DeltaStore) GetSince(ctx context.Context, workerType, role string, sinceSyncID int64, limit int) ([]DeltaEntry, error) {
	if ds.store == nil {
		return []DeltaEntry{}, nil
	}

	docs, err := ds.store.Find(ctx, DeltaCollectionName, persistence.Query{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deltas: %w", err)
	}

	entries := make([]DeltaEntry, 0, limit)

	for _, doc := range docs {
		if wt, ok := doc["worker_type"].(string); !ok || wt != workerType {
			continue
		}

		if r, ok := doc["role"].(string); !ok || r != role {
			continue
		}

		syncID := extractSyncID(doc)
		if syncID <= sinceSyncID {
			continue
		}

		entry, err := ds.documentToEntry(doc)
		if err != nil {
			continue
		}

		entries = append(entries, entry)
	}

	sortEntriesBySyncID(entries)

	if limit > 0 && len(entries) > limit {
		return entries[:limit], nil
	}

	return entries, nil
}

// GetAllSince returns deltas across all worker types and roles since sinceSyncID.
func (ds *DeltaStore) GetAllSince(ctx context.Context, sinceSyncID int64, limit int) ([]DeltaEntry, error) {
	if ds.store == nil {
		return []DeltaEntry{}, nil
	}

	docs, err := ds.store.Find(ctx, DeltaCollectionName, persistence.Query{})
	if err != nil {
		return nil, fmt.Errorf("failed to query deltas: %w", err)
	}

	entries := make([]DeltaEntry, 0, limit)

	for _, doc := range docs {
		syncID := extractSyncID(doc)
		if syncID <= sinceSyncID {
			continue
		}

		entry, err := ds.documentToEntry(doc)
		if err != nil {
			continue
		}

		entries = append(entries, entry)
	}

	sortEntriesBySyncID(entries)

	if limit > 0 && len(entries) > limit {
		return entries[:limit], nil
	}

	return entries, nil
}

// DeleteBefore removes old deltas for compaction.
// If workerType and role are empty, deletes all deltas before beforeSyncID.
func (ds *DeltaStore) DeleteBefore(ctx context.Context, workerType, role string, beforeSyncID int64) error {
	if ds.store == nil {
		return nil
	}

	docs, err := ds.store.Find(ctx, DeltaCollectionName, persistence.Query{})
	if err != nil {
		return fmt.Errorf("failed to list deltas for deletion: %w", err)
	}

	for _, doc := range docs {
		if workerType != "" {
			if wt, ok := doc["worker_type"].(string); !ok || wt != workerType {
				continue
			}
		}

		if role != "" {
			if r, ok := doc["role"].(string); !ok || r != role {
				continue
			}
		}

		syncID := extractSyncID(doc)
		if syncID < beforeSyncID {
			id, ok := doc["id"].(string)
			if !ok {
				continue
			}

			if err := ds.store.Delete(ctx, DeltaCollectionName, id); err != nil {
				return fmt.Errorf("failed to delete delta %s: %w", id, err)
			}
		}
	}

	return nil
}

// documentToEntry converts a persistence.Document to DeltaEntry.
func (ds *DeltaStore) documentToEntry(doc persistence.Document) (DeltaEntry, error) {
	entry := DeltaEntry{}

	if syncID, ok := doc["sync_id"].(int64); ok {
		entry.SyncID = syncID
	} else if f, ok := doc["sync_id"].(float64); ok {
		entry.SyncID = int64(f)
	}

	if wt, ok := doc["worker_type"].(string); ok {
		entry.WorkerType = wt
	}

	if wid, ok := doc["worker_id"].(string); ok {
		entry.ID = wid
	}

	if r, ok := doc["role"].(string); ok {
		entry.Role = r
	}

	if ts, ok := doc["timestamp"].(int64); ok {
		entry.Timestamp = time.UnixMilli(ts)
	} else if f, ok := doc["timestamp"].(float64); ok {
		entry.Timestamp = time.UnixMilli(int64(f))
	}

	if changesStr, ok := doc["changes"].(string); ok && changesStr != "" {
		var diff Diff
		if err := json.Unmarshal([]byte(changesStr), &diff); err == nil {
			entry.Changes = &diff
		}
	}

	return entry, nil
}

// extractSyncID handles both int64 and float64 types from JSON unmarshaling.
func extractSyncID(doc persistence.Document) int64 {
	if syncID, ok := doc["sync_id"].(int64); ok {
		return syncID
	}

	if f, ok := doc["sync_id"].(float64); ok {
		return int64(f)
	}

	return 0
}

// sortEntriesBySyncID sorts entries by SyncID ascending.
func sortEntriesBySyncID(entries []DeltaEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].SyncID < entries[j].SyncID
	})
}
