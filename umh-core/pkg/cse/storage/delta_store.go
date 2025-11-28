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
// Using a single collection makes "get all deltas since X" trivial.
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
// Uses: store.Insert(ctx, collection, doc)
//
// All deltas are stored in a single unified collection (_deltas).
// SyncID is used as the document ID for efficient querying.
func (ds *DeltaStore) Append(ctx context.Context, entry DeltaEntry) error {
	if ds.store == nil {
		return nil // No-op if store not configured
	}

	// Convert DeltaEntry to Document for persistence
	doc := persistence.Document{
		"id":          strconv.FormatInt(entry.SyncID, 10), // Use SyncID as document ID
		"sync_id":     entry.SyncID,
		"worker_type": entry.WorkerType,
		"worker_id":   entry.ID,
		"role":        entry.Role,
		"timestamp":   entry.Timestamp.UnixMilli(),
	}

	// Serialize Changes (Diff) to JSON for storage
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
// Uses the unified delta collection with filtering.
//
// Returns deltas ordered by sync_id ascending.
func (ds *DeltaStore) GetSince(ctx context.Context, workerType, role string, sinceSyncID int64, limit int) ([]DeltaEntry, error) {
	if ds.store == nil {
		return []DeltaEntry{}, nil
	}

	// Query the unified delta collection
	docs, err := ds.store.Find(ctx, DeltaCollectionName, persistence.Query{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deltas: %w", err)
	}

	// Filter by worker_type, role, and sync_id
	entries := make([]DeltaEntry, 0, limit)

	for _, doc := range docs {
		// Filter by worker_type and role
		if wt, ok := doc["worker_type"].(string); !ok || wt != workerType {
			continue
		}

		if r, ok := doc["role"].(string); !ok || r != role {
			continue
		}

		syncID := extractSyncID(doc)
		if syncID <= sinceSyncID {
			continue // Skip deltas we've already seen
		}

		entry, err := ds.documentToEntry(doc)
		if err != nil {
			continue // Skip malformed documents
		}

		entries = append(entries, entry)
	}

	// Sort by SyncID ascending
	sortEntriesBySyncID(entries)

	// Apply limit
	if limit > 0 && len(entries) > limit {
		return entries[:limit], nil
	}

	return entries, nil
}

// GetAllSince returns deltas across all worker types and roles since sinceSyncID.
// This is used by GetDeltas to serve sync clients.
//
// With a single unified delta collection, this is now a simple query.
func (ds *DeltaStore) GetAllSince(ctx context.Context, sinceSyncID int64, limit int) ([]DeltaEntry, error) {
	if ds.store == nil {
		return []DeltaEntry{}, nil
	}

	// Query the unified delta collection
	docs, err := ds.store.Find(ctx, DeltaCollectionName, persistence.Query{})
	if err != nil {
		return nil, fmt.Errorf("failed to query deltas: %w", err)
	}

	// Filter by sync_id > sinceSyncID and convert to entries
	entries := make([]DeltaEntry, 0, limit)

	for _, doc := range docs {
		syncID := extractSyncID(doc)
		if syncID <= sinceSyncID {
			continue // Skip deltas we've already seen
		}

		entry, err := ds.documentToEntry(doc)
		if err != nil {
			continue // Skip malformed documents
		}

		entries = append(entries, entry)
	}

	// Sort by SyncID ascending for consistent ordering
	sortEntriesBySyncID(entries)

	// Apply limit
	if limit > 0 && len(entries) > limit {
		return entries[:limit], nil
	}

	return entries, nil
}

// DeleteBefore removes old deltas (for compaction).
// Uses the unified delta collection.
//
// If workerType and role are empty, deletes all deltas before beforeSyncID.
// Otherwise, deletes only deltas matching the workerType and role.
func (ds *DeltaStore) DeleteBefore(ctx context.Context, workerType, role string, beforeSyncID int64) error {
	if ds.store == nil {
		return nil
	}

	// Query the unified delta collection
	docs, err := ds.store.Find(ctx, DeltaCollectionName, persistence.Query{})
	if err != nil {
		return fmt.Errorf("failed to list deltas for deletion: %w", err)
	}

	for _, doc := range docs {
		// Filter by worker_type and role if specified
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

	// Extract sync_id
	if syncID, ok := doc["sync_id"].(int64); ok {
		entry.SyncID = syncID
	} else if f, ok := doc["sync_id"].(float64); ok {
		entry.SyncID = int64(f)
	}

	// Extract worker_type
	if wt, ok := doc["worker_type"].(string); ok {
		entry.WorkerType = wt
	}

	// Extract worker_id
	if wid, ok := doc["worker_id"].(string); ok {
		entry.ID = wid
	}

	// Extract role
	if r, ok := doc["role"].(string); ok {
		entry.Role = r
	}

	// Extract timestamp
	if ts, ok := doc["timestamp"].(int64); ok {
		entry.Timestamp = time.UnixMilli(ts)
	} else if f, ok := doc["timestamp"].(float64); ok {
		entry.Timestamp = time.UnixMilli(int64(f))
	}

	// Extract and deserialize changes
	if changesStr, ok := doc["changes"].(string); ok && changesStr != "" {
		var diff Diff
		if err := json.Unmarshal([]byte(changesStr), &diff); err == nil {
			entry.Changes = &diff
		}
	}

	return entry, nil
}

// extractSyncID extracts sync_id from a document, handling both int64 and float64 types.
func extractSyncID(doc persistence.Document) int64 {
	if syncID, ok := doc["sync_id"].(int64); ok {
		return syncID
	}

	if f, ok := doc["sync_id"].(float64); ok {
		return int64(f)
	}

	return 0
}

// sortEntriesBySyncID sorts delta entries by SyncID in ascending order.
func sortEntriesBySyncID(entries []DeltaEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].SyncID < entries[j].SyncID
	})
}
