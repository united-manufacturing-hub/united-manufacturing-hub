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
//
// DESIGN DECISION: Single function handles all save operations with role-specific configuration.
// WHY: Eliminates ~60% code duplication across SaveIdentity, SaveDesired, SaveObserved.
// Each role has slightly different CSE semantics (versioning, timestamps, delta checking),
// but the core write flow is identical.
//
// FLOW:
// 1. Load existing document (for delta check and version preservation)
// 2. Compute diff if enabled (skip for identity)
// 3. If no changes and !UpdateTimestampOnNoChange, return early
// 4. Inject CSE metadata (_sync_id, _version, timestamps)
// 5. Save full snapshot to persistence
// 6. Return changed status and diff for event streaming
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type (e.g., "container", "relay")
//   - id: Unique worker identifier
//   - doc: Document to save (will be mutated with CSE metadata)
//   - opts: Role-specific save options
//
// Returns:
//   - changed: true if business data changed (or first save)
//   - diff: Field-level changes for delta streaming (nil if no changes)
//   - err: Non-nil if operation failed
func (ts *TriangularStore) saveWithDelta(
	ctx context.Context,
	workerType, id string,
	doc persistence.Document,
	opts SaveOptions,
) (changed bool, diff *Diff, err error) {
	if doc == nil {
		return false, nil, errors.New("document cannot be nil")
	}

	// Validate document has required 'id' field that matches the parameter
	docID, ok := doc["id"].(string)
	if !ok || docID == "" {
		return false, nil, errors.New("document must have non-empty string 'id' field")
	}

	if docID != id {
		return false, nil, fmt.Errorf("document id %q does not match parameter id %q", docID, id)
	}

	// Collection name follows convention: {workerType}_{role}
	collectionName := workerType + "_" + opts.Role

	// Step 1: Load existing document
	existing, err := ts.store.Get(ctx, collectionName, id)

	isNew := err != nil && errors.Is(err, persistence.ErrNotFound)
	if err != nil && !isNew {
		return false, nil, fmt.Errorf("failed to load existing %s for %s/%s: %w", opts.Role, workerType, id, err)
	}

	// Step 2: Delta checking
	var changes []FieldChange

	if !opts.SkipDeltaCheck {
		if isNew {
			// For new documents, create a "created" delta with all business fields as "added"
			diff = ts.computeCreatedDiff(doc, opts.Role)

			// Get hierarchy path for unified logging
			// For identity saves, use hierarchy_path from doc being saved
			// For desired/observed, load identity to get hierarchy_path
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

			// Log creation for observability
			ts.logger.Debugw(opts.Role+"_created",
				"worker", hierarchyPath)
		} else {
			var hasChanges bool

			hasChanges, changes, diff = ts.performDeltaCheck(existing, doc, opts.Role)

			if !hasChanges {
				// No business data changed
				if opts.UpdateTimestampOnNoChange && existing != nil {
					// Update CSE timestamp for staleness detection (observed state)
					// Use time.Time for consistency with injectMetadataWithOptions (line 270)
					now := time.Now().UTC()
					existing[FieldUpdatedAt] = now

					err = ts.store.Update(ctx, collectionName, id, existing)
					if err != nil {
						return false, nil, fmt.Errorf("failed to update timestamp for %s/%s: %w", workerType, id, err)
					}
				}

				return false, nil, nil
			}

			// Get hierarchy path for unified logging
			var hierarchyPath string
			if identity, err := ts.LoadIdentity(ctx, workerType, id); err == nil {
				if hp, ok := identity["hierarchy_path"].(string); ok {
					hierarchyPath = hp
				}
			}

			// Log changes for observability
			ts.logger.Debugw(opts.Role+"_changed",
				"worker", hierarchyPath,
				"changes", changes)
		}
	}

	// Step 3: Preserve version from existing (observed doesn't increment version)
	if !isNew && existing != nil && !opts.IncrementVersion {
		if version, ok := existing[FieldVersion]; ok {
			doc[FieldVersion] = version
		}
	}

	// Step 4: Assign sync ID BEFORE any database write
	// This ensures the sync ID is included atomically in the single write operation.
	// If we assigned it after the write, a crash between write and sync ID update
	// would leave the document without a sync ID.
	syncID := ts.syncID.Add(1)
	doc[FieldSyncID] = syncID

	// Step 5: Inject CSE metadata (version, timestamps)
	ts.injectMetadataWithOptions(doc, opts, isNew)

	// Step 6: Save to persistence (single atomic write with sync ID already set)
	if isNew {
		_, err = ts.store.Insert(ctx, collectionName, doc)
	} else {
		err = ts.store.Update(ctx, collectionName, id, doc)
	}

	if err != nil {
		return false, nil, fmt.Errorf("failed to save %s for %s/%s: %w", opts.Role, workerType, id, err)
	}

	// Step 7: Append to delta store if there are changes
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
			// Get hierarchy path for unified logging
			var hierarchyPath string
			if identity, loadErr := ts.LoadIdentity(ctx, workerType, id); loadErr == nil {
				if hp, ok := identity["hierarchy_path"].(string); ok {
					hierarchyPath = hp
				}
			}

			// Log but don't fail the operation - snapshot was saved successfully
			ts.logger.Warnw("failed to append delta",
				"worker", hierarchyPath,
				"role", opts.Role,
				"error", appendErr)
		}
	}

	// Step 8: Invalidate cache
	cacheKey := workerType + "_" + id

	ts.cacheMutex.Lock()
	delete(ts.snapshotCache, cacheKey)
	ts.cacheMutex.Unlock()

	return true, diff, nil
}

// injectMetadataWithOptions adds or updates CSE metadata fields based on SaveOptions.
// This is a more flexible version of injectMetadata that uses SaveOptions.
func (ts *TriangularStore) injectMetadataWithOptions(doc persistence.Document, opts SaveOptions, isNew bool) {
	if doc == nil {
		return
	}

	now := time.Now().UTC()

	if isNew {
		// First save: set creation timestamp and initial version
		doc[FieldCreatedAt] = now
		doc[FieldVersion] = int64(1)
	} else {
		// Update: set update timestamp
		doc[FieldUpdatedAt] = now

		// Increment version only if configured (for desired state optimistic locking)
		if opts.IncrementVersion {
			currentVersion, ok := doc[FieldVersion].(int64)
			if !ok {
				currentVersion = 0
			}

			doc[FieldVersion] = currentVersion + 1
		}
	}
	// NOTE: collected_at is a business field set by FSM v2 workers, not injected by CSE.
}

// computeCreatedDiff creates a Diff representing document creation.
// All non-CSE fields are marked as "Added" since there was no previous document.
// This enables frontend sync to see the initial state of newly created workers.
//
// Parameters:
//   - doc: The new document being created
//   - role: The triangular role (identity, desired, observed)
//
// Returns:
//   - *Diff with all business fields in Added map, or nil if no business fields
func (ts *TriangularStore) computeCreatedDiff(doc persistence.Document, role string) *Diff {
	if doc == nil {
		return nil
	}

	cseFields := getCSEFields(role)

	cseFieldSet := make(map[string]bool, len(cseFields))
	for _, f := range cseFields {
		cseFieldSet[f] = true
	}

	// Filter out CSE metadata fields - only include business data
	added := make(map[string]interface{})

	for key, val := range doc {
		if !cseFieldSet[key] && key != "id" && key != FieldVersion {
			added[key] = val
		}
	}

	// If no business fields, return nil (no delta needed)
	if len(added) == 0 {
		return nil
	}

	return &Diff{
		Added:    added,
		Modified: make(map[string]ModifiedField),
		Removed:  []string{},
	}
}
