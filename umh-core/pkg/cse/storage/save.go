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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// SaveOptions configures role-specific behavior for saveWithDelta.
// Different roles (identity, desired, observed) have different CSE semantics.
type SaveOptions struct {
	// Role is the triangular model role (identity, desired, observed).
	Role string

	// IncrementVersion controls whether _version is incremented on update.
	// True for desired (optimistic locking), false for observed (ephemeral state).
	IncrementVersion bool

	// SkipDeltaCheck disables change detection.
	// True for identity (write-once, no delta checking needed).
	SkipDeltaCheck bool

	// UpdateTimestampOnNoChange updates _updated_at even when data hasn't changed.
	// True for observed (enables staleness detection - "when was this worker last seen?").
	UpdateTimestampOnNoChange bool
}

// Pre-configured SaveOptions for each triangular role.
var (
	// IdentitySaveOptions for immutable worker identity.
	// Identity is created once and never updated.
	IdentitySaveOptions = SaveOptions{
		Role:           RoleIdentity,
		SkipDeltaCheck: false, // Enable delta for creation events
	}

	// DesiredSaveOptions for user intent/configuration.
	// Increments version for optimistic locking to prevent lost updates.
	DesiredSaveOptions = SaveOptions{
		Role:             RoleDesired,
		IncrementVersion: true, // Optimistic locking
	}

	// ObservedSaveOptions for system reality/state.
	// Always updates timestamp even on no change (staleness detection).
	ObservedSaveOptions = SaveOptions{
		Role:                      RoleObserved,
		UpdateTimestampOnNoChange: true, // Staleness detection
	}
)

// saveWithDelta is the unified write path for all triangular store save operations.
// Eliminates ~60% code duplication across SaveIdentity, SaveDesired, SaveObserved
// by using role-specific configuration for versioning, timestamps, and delta checking.
func (ts *TriangularStore) saveWithDelta(
	ctx context.Context,
	workerType, id string,
	doc persistence.Document,
	opts SaveOptions,
) (changed bool, diff *Diff, err error) {
	if doc == nil {
		return false, nil, errors.New("document cannot be nil")
	}

	docID, ok := doc["id"].(string)
	if !ok || docID == "" {
		return false, nil, errors.New("document must have non-empty string 'id' field")
	}

	if docID != id {
		return false, nil, fmt.Errorf("document id %q does not match parameter id %q", docID, id)
	}

	collectionName := workerType + "_" + opts.Role

	existing, err := ts.store.Get(ctx, collectionName, id)

	isNew := err != nil && errors.Is(err, persistence.ErrNotFound)
	if err != nil && !isNew {
		return false, nil, fmt.Errorf("failed to load existing %s for %s/%s: %w", opts.Role, workerType, id, err)
	}

	var changes []FieldChange

	if !opts.SkipDeltaCheck {
		if isNew {
			diff = ts.computeCreatedDiff(doc, opts.Role)

			var hierarchyPath string
			if opts.Role == RoleIdentity {
				if hp, ok := doc["hierarchy_path"].(string); ok {
					hierarchyPath = hp
				}
			} else {
				if identity, err := ts.LoadIdentity(ctx, workerType, id); err == nil {
					if hp, ok := identity["hierarchy_path"].(string); ok {
						hierarchyPath = hp
					}
				}
			}

			ts.logger.Debugw(opts.Role+"_created",
				"worker", hierarchyPath)
		} else {
			var hasChanges bool

			hasChanges, changes, diff = ts.performDeltaCheck(existing, doc, opts.Role)

			if !hasChanges {
				if opts.UpdateTimestampOnNoChange && existing != nil {
					// Staleness detection for observed state
					now := time.Now().UTC()
					existing[FieldUpdatedAt] = now

					err = ts.store.Update(ctx, collectionName, id, existing)
					if err != nil {
						return false, nil, fmt.Errorf("failed to update timestamp for %s/%s: %w", workerType, id, err)
					}

					// Invalidate cache so LoadSnapshot returns updated timestamp
					cacheKey := workerType + "_" + id

					ts.cacheMutex.Lock()
					delete(ts.snapshotCache, cacheKey)
					ts.cacheMutex.Unlock()
				}

				return false, nil, nil
			}

			var hierarchyPath string
			if identity, err := ts.LoadIdentity(ctx, workerType, id); err == nil {
				if hp, ok := identity["hierarchy_path"].(string); ok {
					hierarchyPath = hp
				}
			}

			ts.logger.Debugw(opts.Role+"_changed",
				"worker", hierarchyPath,
				"changes", changes)
		}
	}

	// Preserve version for roles that don't increment (observed state)
	if !isNew && existing != nil && !opts.IncrementVersion {
		if version, ok := existing[FieldVersion]; ok {
			doc[FieldVersion] = version
		}
	}

	// Assign sync ID before write for atomicity (crash between write and ID update would lose ID)
	syncID := ts.syncID.Add(1)
	doc[FieldSyncID] = syncID

	ts.injectMetadataWithOptions(doc, opts, isNew)

	if isNew {
		_, err = ts.store.Insert(ctx, collectionName, doc)
	} else {
		err = ts.store.Update(ctx, collectionName, id, doc)
	}

	if err != nil {
		return false, nil, fmt.Errorf("failed to save %s for %s/%s: %w", opts.Role, workerType, id, err)
	}

	if diff != nil && ts.deltaStore != nil {
		entry := DeltaEntry{
			SyncID:     syncID,
			WorkerType: workerType,
			ID:         id,
			Role:       opts.Role,
			Changes:    diff,
			Timestamp:  time.Now(),
		}

		if appendErr := ts.deltaStore.Append(ctx, entry); appendErr != nil {
			var hierarchyPath string
			if identity, loadErr := ts.LoadIdentity(ctx, workerType, id); loadErr == nil {
				if hp, ok := identity["hierarchy_path"].(string); ok {
					hierarchyPath = hp
				}
			}

			// Snapshot saved successfully, delta append is best-effort
			ts.logger.Warnw("delta_append_failed",
				"worker", hierarchyPath,
				"role", opts.Role,
				"error", appendErr)
		}
	}

	cacheKey := workerType + "_" + id

	ts.cacheMutex.Lock()
	delete(ts.snapshotCache, cacheKey)
	ts.cacheMutex.Unlock()

	return true, diff, nil
}

// injectMetadataWithOptions adds or updates CSE metadata fields based on SaveOptions.
func (ts *TriangularStore) injectMetadataWithOptions(doc persistence.Document, opts SaveOptions, isNew bool) {
	if doc == nil {
		return
	}

	now := time.Now().UTC()

	if isNew {
		doc[FieldCreatedAt] = now
		doc[FieldVersion] = int64(1)
	} else {
		doc[FieldUpdatedAt] = now

		// Optimistic locking for desired state
		if opts.IncrementVersion {
			// Handle both int64 (native) and float64 (after JSON round-trip)
			var currentVersion int64

			switch v := doc[FieldVersion].(type) {
			case int64:
				currentVersion = v
			case float64:
				currentVersion = int64(v)
			default:
				currentVersion = 0
			}

			doc[FieldVersion] = currentVersion + 1
		}
	}
	// NOTE: collected_at is a business field set by FSM v2 workers, not injected by CSE.
}

// computeCreatedDiff creates a Diff for document creation with all business fields as Added.
func (ts *TriangularStore) computeCreatedDiff(doc persistence.Document, role string) *Diff {
	if doc == nil {
		return nil
	}

	cseFields := getCSEFields(role)

	cseFieldSet := make(map[string]bool, len(cseFields))
	for _, f := range cseFields {
		cseFieldSet[f] = true
	}

	added := make(map[string]interface{})

	for key, val := range doc {
		if !cseFieldSet[key] && key != "id" && key != FieldVersion {
			added[key] = val
		}
	}

	if len(added) == 0 {
		return nil
	}

	return &Diff{
		Added:    added,
		Modified: make(map[string]ModifiedField),
		Removed:  []string{},
	}
}
