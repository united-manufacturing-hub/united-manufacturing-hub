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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// TriangularStore provides high-level operations for FSM v2's triangular model.
//
// DESIGN DECISION: Auto-inject CSE metadata transparently
// WHY: Callers shouldn't manage _sync_id, _version, timestamps manually.
// Reduces boilerplate and prevents mistakes (forgetting to increment sync ID).
//
// TRADE-OFF: Less control over metadata fields, but fewer bugs.
// If fine-grained control is needed, callers can use persistence.Store directly.
//
// INSPIRED BY: ORM auto-timestamps (created_at, updated_at in Rails/Django),
// Linear's transparent sync metadata injection.
//
// ARCHITECTURE: Single-Node Assumption
//
// Worker state is organized by workerType and id, with each worker having three
// collections (identity, desired, observed). This collection-based storage (not
// worker-based) allows efficient querying by role and supports the triangular model
// where identity, desired, and observed are independent concepts.
//
// This implementation assumes all workers run on a single node. The underlying
// persistence.Store (currently in-memory) keeps all collections in the same process
// memory space. For distributed deployments, replace the persistence layer with
// a distributed store that maintains collection-based organization.
//
// The triangular model separates each worker into three parts:
//   - Identity: Immutable worker identification (ID, Name, WorkerType)
//   - Desired: User intent / configuration (what we want)
//   - Observed: System reality (what actually exists)
//
// Each part is stored in a separate collection:
//   - container_identity (immutable, created once)
//   - container_desired (user configuration, increments version on change)
//   - container_observed (system state, increments sync ID but not version)
//
// Example usage:
//
//	ts := cse.NewTriangularStore(sqliteStore, logger)
//
//	// Create worker
//	ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
//	    "id": "worker-123",
//	    "name": "Container A",
//	    "workerType": "container",
//	})
//
//	// Save user intent
//	ts.SaveDesired(ctx, "container", "worker-123", persistence.Document{
//	    "id": "worker-123",
//	    "config": "production",
//	})
//
//	// Save system reality (called on every FSM tick)
//	ts.SaveObserved(ctx, "container", "worker-123", persistence.Document{
//	    "id": "worker-123",
//	    "status": "running",
//	    "cpu": 45.2,
//	})
//
//	// FSM supervisor loads complete snapshot
//	snapshot, _ := ts.LoadSnapshot(ctx, "container", "worker-123")
//	// Use snapshot.Identity, snapshot.Desired, snapshot.Observed for Next() decision
type TriangularStore struct {
	store      persistence.Store
	syncID     *atomic.Int64
	logger     *zap.SugaredLogger
	deltaStore *DeltaStore

	// Memory cache for hot path performance (LoadSnapshot is called 100-1000+ times/sec)
	snapshotCache map[string]*cachedSnapshot // key: "{workerType}_{id}"

	// Known worker types - automatically populated as workers are saved
	// Used by buildBootstrap to discover all worker types for full state sync
	knownWorkerTypes map[string]struct{}
	cacheMutex       sync.RWMutex

	knownWorkerTypesMu sync.RWMutex
}

// cachedSnapshot stores a snapshot with its syncID for cache invalidation.
type cachedSnapshot struct {
	snapshot *Snapshot
	syncID   int64 // syncID when cached - used to detect staleness
}

// NewTriangularStore creates a new TriangularStore.
//
// DESIGN DECISION: Convention-based system (no registry needed)
// WHY: Collection names follow naming convention: {workerType}_{role}
//
// CSE metadata fields are hardcoded constants (see getCSEFields()).
//
// Parameters:
//   - store: Backend storage implementation (SQLite, Postgres, etc.)
//   - logger: Logger for observation change logging (use zap.NewNop().Sugar() for tests)
//
// Returns:
//   - *TriangularStore: Ready-to-use triangular store instance
func NewTriangularStore(store persistence.Store, logger *zap.SugaredLogger) *TriangularStore {
	return &TriangularStore{
		store:            store,
		syncID:           &atomic.Int64{},
		logger:           logger,
		deltaStore:       NewDeltaStore(store),
		snapshotCache:    make(map[string]*cachedSnapshot),
		knownWorkerTypes: make(map[string]struct{}),
	}
}

// registerWorkerType records a worker type for use in bootstrap.
// Called automatically during Save operations.
func (ts *TriangularStore) registerWorkerType(workerType string) {
	ts.knownWorkerTypesMu.Lock()
	ts.knownWorkerTypes[workerType] = struct{}{}
	ts.knownWorkerTypesMu.Unlock()
}

// SaveIdentity stores immutable worker identity (runtime polymorphic API).
//
// CONVENTION-BASED NAMING: Collection name is derived from naming convention: {workerType}_identity
// This allows supervisors to handle multiple worker types at runtime without type-specific code.
//
// DESIGN DECISION: Identity is created once and never updated
// WHY: Identity fields (ID, name, worker type) don't change.
// Immutability simplifies reasoning about worker lifecycle.
//
// TRADE-OFF: Can't update identity after creation. If identity needs to change,
// must delete worker and recreate with new identity.
//
// INSPIRED BY: FSM v2 worker.go identity semantics, database primary keys.
//
// CSE metadata injected:
//   - _sync_id: Global sync version (for delta sync queries)
//   - _version: Set to 1 (identity version never changes)
//   - _created_at: Timestamp of creation
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type (e.g., "container", "relay")
//   - id: Unique worker identifier
//   - identity: Identity document (id, name, workerType)
//
// Returns:
//   - error: If worker type not registered or insertion fails
//
// Example (runtime polymorphic code):
//
//	err := ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
//	    "id": "worker-123",
//	    "name": "Container A",
//	    "workerType": "container",
//	})
func (ts *TriangularStore) SaveIdentity(ctx context.Context, workerType string, id string, identity persistence.Document) error {
	ts.registerWorkerType(workerType)
	_, _, err := ts.saveWithDelta(ctx, workerType, id, identity, IdentitySaveOptions)

	return err
}

// LoadIdentity retrieves worker identity (runtime polymorphic API).
//
// CONVENTION-BASED NAMING: Collection name is derived from naming convention: {workerType}_identity
// This allows supervisors to handle multiple worker types at runtime without type-specific code.
//
// DESIGN DECISION: Return ErrNotFound if worker doesn't exist
// WHY: Explicit error handling - caller knows whether worker exists.
// Matches persistence.Store.Get semantics.
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type (e.g., "container")
//   - id: Unique worker identifier
//
// Returns:
//   - persistence.Document: Identity document with CSE metadata
//   - error: ErrNotFound if worker doesn't exist
func (ts *TriangularStore) LoadIdentity(ctx context.Context, workerType string, id string) (persistence.Document, error) {
	// Collection name follows convention: {workerType}_identity
	collectionName := workerType + "_identity"

	doc, err := ts.store.Get(ctx, collectionName, id)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// SaveDesired stores user intent/configuration with automatic delta checking (runtime polymorphic API).
//
// CONVENTION-BASED NAMING: Collection name is derived from naming convention: {workerType}_desired
// This allows supervisors to handle multiple worker types at runtime without type-specific code.
//
// For compile-time type safety, use SaveDesiredTyped[T]() instead.
//
// Example (runtime polymorphic code):
//
//	changed, err := ts.SaveDesired(ctx, "container", id, doc)
//
// Example (compile-time typed code):
//
//	changed, err := storage.SaveDesiredTyped[ContainerDesiredState](ts, ctx, id, desired)
//
// DESIGN DECISION: Built-in delta checking skips unchanged writes
// WHY: Desired state can be written frequently. Skipping redundant writes reduces
// database load and prevents unnecessary sync_id increments, enabling efficient
// delta streaming to clients.
//
// DESIGN DECISION: Increment _version for optimistic locking
// WHY: Desired state represents user configuration. Version prevents lost updates
// when multiple clients modify configuration concurrently.
//
// TRADE-OFF: Additional LoadDesired() call adds one database read per save.
// Acceptable because in-memory reads are fast and writes are more expensive.
//
// INSPIRED BY: Optimistic locking in ORMs (Hibernate, Entity Framework),
// Linear's version-based conflict resolution.
//
// Delta checking behavior:
//   - First save (worker doesn't exist): Always writes, returns (true, nil)
//   - Data changed: Writes to database, returns (true, nil)
//   - Data unchanged: Skips write, returns (false, nil)
//   - Error: Returns (false, err)
//
// CSE metadata fields (_sync_id, _version, _created_at, _updated_at) are
// excluded from change detection. Only user data fields are compared.
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type
//   - id: Unique worker identifier
//   - desired: Desired state document
//
// Returns:
//   - changed: true if data was written to database, false if write was skipped
//   - err: non-nil if operation failed (changed is always false when err != nil)
func (ts *TriangularStore) SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) (changed bool, err error) {
	changed, _, err = ts.saveWithDelta(ctx, workerType, id, desired, DesiredSaveOptions)

	return changed, err
}

// LoadDesired retrieves user intent/configuration.
//
// Deprecated: Use LoadDesiredTyped[T]() instead for type safety and no registry dependency.
// This method will be removed in a future version.
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type
//   - id: Unique worker identifier
//
// Returns:
//   - interface{}: Desired state as Document
//   - error: ErrNotFound if not found
func (ts *TriangularStore) LoadDesired(ctx context.Context, workerType string, id string) (interface{}, error) {
	collectionName := workerType + "_desired"

	doc, err := ts.store.Get(ctx, collectionName, id)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// LoadDesiredTyped loads desired state and deserializes into provided pointer.
//
// This method supports reflection-based code that doesn't know types at compile time.
// For compile-time type safety, use LoadDesiredTyped[T]() package-level function instead.
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type (e.g., "parent")
//   - id: Unique worker identifier
//   - dest: Pointer to destination struct (will be populated via JSON deserialization)
//
// Returns:
//   - error: ErrNotFound if not found, or deserialization error
//
// Example (reflection-based code):
//
//	var dest ParentDesiredState
//	err := ts.LoadDesiredTyped(ctx, "parent", "parent-001", &dest)
func (ts *TriangularStore) LoadDesiredTyped(ctx context.Context, workerType string, id string, dest interface{}) error {
	// Guard against nil or non-pointer dest to prevent reflect panics
	if dest == nil {
		return errors.New("dest must be non-nil pointer, got nil")
	}

	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr {
		return fmt.Errorf("dest must be pointer, got %s", destVal.Kind())
	}

	result, err := ts.LoadDesired(ctx, workerType, id)
	if err != nil {
		return err
	}

	// If result is already the correct type, copy it
	if reflect.TypeOf(result) == reflect.TypeOf(dest).Elem() {
		reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(result))

		return nil
	}

	// Otherwise, deserialize Document to dest
	doc, ok := result.(persistence.Document)
	if !ok {
		return fmt.Errorf("LoadDesired returned %T, cannot deserialize", result)
	}

	return documentToStruct(doc, dest)
}

// diffExcludedFields are CSE-internal timestamp fields that should not trigger
// diff logging (they add noise without meaningful observability value).
// NOTE: collected_at is NOT excluded - it's a business field from FSM v2 workers,
// and changes to it SHOULD trigger deltas for frontend sync.
var diffExcludedFields = map[string]bool{
	// Dependencies only contains pointers and shouldn't trigger deltas.
	"Dependencies": true,
}

// FieldChange represents a single field change for logging.
// CSE semantics: Fields are never removed, only "added" or "modified".
// old=nil means this is a new field being added for the first time.
type FieldChange struct {
	Old        interface{} `json:"old"`
	New        interface{} `json:"new"`
	Field      string      `json:"field"`
	ChangeType string      `json:"change_type"` // "added" or "modified" (CSE never removes fields)
}

// formatValueForLog safely formats a value for logging with truncation.
func formatValueForLog(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	s := fmt.Sprintf("%v", v)
	if len(s) > 100 {
		return s[:97] + "..."
	}

	return v
}

// getChangedFieldsWithValues returns detailed field changes with old and new values,
// excluding timestamp fields that change every tick.
// Each change includes a change_type: "added" or "modified".
//
// CSE SEMANTICS: Fields are NEVER removed in CSE - only added or modified.
// If a field is missing from the new document, it means the field was not included
// in this update, NOT that it should be deleted. The triangular store preserves
// existing field values when they're not included in an update.
// This is fundamental to CSE's schema evolution model.
func (ts *TriangularStore) getChangedFieldsWithValues(current, new persistence.Document) []FieldChange {
	var changes []FieldChange

	// Fields added or modified
	// Note: We only track fields present in 'new' because CSE never removes fields.
	// Fields missing from 'new' retain their existing values in the store.
	for k, newVal := range new {
		if diffExcludedFields[k] {
			continue // Skip timestamp fields that change every tick
		}

		oldVal, exists := current[k]
		if !exists {
			// Field is new (added)
			changes = append(changes, FieldChange{
				Field:      k,
				ChangeType: "added",
				Old:        nil,
				New:        formatValueForLog(newVal),
			})
		} else if !reflect.DeepEqual(oldVal, newVal) {
			// Field exists but value changed (modified)
			changes = append(changes, FieldChange{
				Field:      k,
				ChangeType: "modified",
				Old:        formatValueForLog(oldVal),
				New:        formatValueForLog(newVal),
			})
		}
	}

	return changes
}

// SaveObserved stores system reality with automatic delta checking (runtime polymorphic API).
//
// CONVENTION-BASED NAMING: Collection name is derived from naming convention: {workerType}_observed
// This allows supervisors to handle multiple worker types at runtime without type-specific code.
//
// For compile-time type safety, use SaveObservedTyped[T]() instead.
//
// Example (runtime polymorphic code):
//
//	changed, err := ts.SaveObserved(ctx, "container", id, doc)
//
// Example (compile-time typed code):
//
//	changed, err := storage.SaveObservedTyped[ContainerObservedState](ts, ctx, id, observed)
//
// DESIGN DECISION: Built-in delta checking skips unchanged writes
// WHY: Observed state is polled frequently (500ms default). Most polls
// return the same data. Skipping redundant writes reduces database load
// and prevents unnecessary sync_id increments.
//
// TRADE-OFF: Additional LoadObserved() call adds one database read per save.
// Acceptable because in-memory reads are fast and writes are more expensive.
//
// Delta checking behavior:
//   - First save (worker doesn't exist): Always writes, returns (true, nil)
//   - Data changed: Writes to database, returns (true, nil)
//   - Data unchanged: Skips write, returns (false, nil)
//   - Error: Returns (false, err)
//
// CSE metadata fields (_sync_id, _version, _created_at, _updated_at) are
// excluded from change detection. Only user data fields are compared.
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type
//   - id: Unique worker identifier
//   - observed: Observed state (persistence.Document or any struct/map)
//
// Returns:
//   - changed: true if data was written to database, false if write was skipped
//   - err: non-nil if operation failed (changed is always false when err != nil)
func (ts *TriangularStore) SaveObserved(ctx context.Context, workerType string, id string, observed interface{}) (changed bool, err error) {
	// Convert observed to Document if needed
	observedDoc, err := ts.toDocument(observed)
	if err != nil {
		return false, fmt.Errorf("failed to convert observed to document: %w", err)
	}

	// Ensure id field is set
	observedDoc["id"] = id

	changed, _, err = ts.saveWithDelta(ctx, workerType, id, observedDoc, ObservedSaveOptions)

	return changed, err
}

// LoadObserved retrieves system reality.
//
// Deprecated: Use LoadObservedTyped[T]() instead for type safety and no registry dependency.
// This method will be removed in a future version.
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type
//   - id: Unique worker identifier
//
// Returns:
//   - persistence.Document: Observed state document with CSE metadata
//   - error: ErrNotFound if not found
func (ts *TriangularStore) LoadObserved(ctx context.Context, workerType string, id string) (interface{}, error) {
	collectionName := workerType + "_observed"

	doc, err := ts.store.Get(ctx, collectionName, id)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// LoadObservedTyped loads observed state and deserializes into provided pointer.
//
// This method supports reflection-based code that doesn't know types at compile time.
// For compile-time type safety, use LoadObservedTyped[T]() package-level function instead.
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type (e.g., "parent")
//   - id: Unique worker identifier
//   - dest: Pointer to destination struct (will be populated via JSON deserialization)
//
// Returns:
//   - error: ErrNotFound if not found, or deserialization error
//
// Example (reflection-based code):
//
//	var dest ParentObservedState
//	err := ts.LoadObservedTyped(ctx, "parent", "parent-001", &dest)
func (ts *TriangularStore) LoadObservedTyped(ctx context.Context, workerType string, id string, dest interface{}) error {
	// Guard against nil or non-pointer dest to prevent reflect panics
	if dest == nil {
		return errors.New("dest must be non-nil pointer, got nil")
	}

	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr {
		return fmt.Errorf("dest must be pointer, got %s", destVal.Kind())
	}

	result, err := ts.LoadObserved(ctx, workerType, id)
	if err != nil {
		return err
	}

	// If result is already the correct type, copy it
	if reflect.TypeOf(result) == reflect.TypeOf(dest).Elem() {
		reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(result))

		return nil
	}

	// Otherwise, deserialize Document to dest
	doc, ok := result.(persistence.Document)
	if !ok {
		return fmt.Errorf("LoadObserved returned %T, cannot deserialize", result)
	}

	return documentToStruct(doc, dest)
}

func (ts *TriangularStore) filterCSEFields(doc persistence.Document, cseFields []string) persistence.Document {
	filtered := make(persistence.Document)

	for k, v := range doc {
		isCSEField := false

		for _, cseField := range cseFields {
			if k == cseField {
				isCSEField = true

				break
			}
		}

		if !isCSEField {
			filtered[k] = v
		}
	}

	return filtered
}

// performDeltaCheck compares two documents and returns change information.
// Filters out CSE fields, ID, and version before comparison.
//
// Returns:
//   - hasChanges: true if business data changed (new document or fields modified)
//   - changes: slice of FieldChange for logging (nil for new documents)
//   - diff: Diff struct for event storage (nil if no changes or new document)
func (ts *TriangularStore) performDeltaCheck(
	currentDoc, newDoc persistence.Document,
	role string,
) (hasChanges bool, changes []FieldChange, diff *Diff) {
	if currentDoc == nil {
		return true, nil, nil // New document = changes
	}

	cseFields := getCSEFields(role)

	currentFiltered := ts.filterCSEFields(currentDoc, cseFields)
	delete(currentFiltered, "id")
	delete(currentFiltered, FieldVersion)

	newFiltered := ts.filterCSEFields(newDoc, cseFields)
	delete(newFiltered, "id")
	delete(newFiltered, FieldVersion)

	changes = ts.getChangedFieldsWithValues(currentFiltered, newFiltered)
	hasChanges = len(changes) > 0

	if hasChanges {
		diff = computeDiff(currentFiltered, newFiltered)
	}

	return hasChanges, changes, diff
}

// Snapshot represents the complete state of a worker.
//
// DESIGN DECISION: Separate struct instead of map[string]Document
// WHY: Type-safe access to three parts. Prevents mistakes like
// accessing snapshot["identity"] instead of snapshot.Identity.
//
// TRADE-OFF: More verbose than map, but self-documenting.
//
// INSPIRED BY: Domain-driven design value objects, Linear's entity snapshots.
type Snapshot struct {
	Identity persistence.Document
	Desired  persistence.Document
	Observed interface{} // Can be persistence.Document or any ObservedState type (for type validation)
}

// LoadSnapshot atomically loads all three parts of the triangular model.
//
// DESIGN DECISION: Use transaction for atomic read
// WHY: Ensure consistent view of worker state. Don't mix old desired with new observed.
// Critical for FSM correctness - state machine needs snapshot at single point in time.
//
// TRADE-OFF: Slight overhead from transaction, but essential for correctness.
// Without transaction, FSM might see inconsistent state (e.g., desired says "stop"
// but observed says "running" from before the desired change).
//
// INSPIRED BY: Database MVCC (multi-version concurrency control),
// Linear's snapshot isolation for sync operations.
//
// TYPE INFORMATION LOSS (Acceptable for MVP):
// Loaded states are returned as persistence.Document (map[string]interface{}), NOT typed structs.
// Even if SaveObserved() was called with a typed struct, LoadSnapshot returns Document.
// This is because we persist as JSON and don't store type metadata for deserialization.
//
// WHY ACCEPTABLE:
// - Communicator can work with Documents directly (uses reflection/type assertion)
// - FSM's type check explicitly skips Documents (line 590-592 in supervisor.go)
// - For MVP, we prioritize simplicity over type safety at persistence boundary
//
// FUTURE ENHANCEMENT:
// Could add type registry to deserialize back to typed structs, but adds complexity.
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type
//   - id: Unique worker identifier
//
// Returns:
//   - *Snapshot: Complete worker state (identity, desired, observed as Documents)
//   - error: ErrNotFound if any part is missing, or transaction fails
//
// Example:
//
//	snapshot, err := ts.LoadSnapshot(ctx, "container", "worker-123")
//	if err != nil {
//	    return err
//	}
//
//	// FSM uses snapshot for decision
//	if snapshot.Desired["status"] == "stopped" && snapshot.Observed["status"] == "running" {
//	    // Transition to stopping state
//	}
func (ts *TriangularStore) LoadSnapshot(ctx context.Context, workerType string, id string) (*Snapshot, error) {
	cacheKey := workerType + "_" + id
	currentSyncID := ts.syncID.Load()

	ts.cacheMutex.RLock()

	if cached, exists := ts.snapshotCache[cacheKey]; exists && cached.syncID == currentSyncID {
		ts.cacheMutex.RUnlock()

		// Return a deep copy to prevent callers from corrupting the cache
		return cloneSnapshot(cached.snapshot), nil
	}

	ts.cacheMutex.RUnlock()

	// Collection names follow convention: {workerType}_{role}
	identityCollectionName := workerType + "_identity"
	desiredCollectionName := workerType + "_desired"
	observedCollectionName := workerType + "_observed"

	// Use transaction for atomic read
	tx, err := ts.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	// Load all three parts
	identity, err := tx.Get(ctx, identityCollectionName, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load identity: %w", err)
	}

	desired, err := tx.Get(ctx, desiredCollectionName, id)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return nil, fmt.Errorf("failed to load desired: %w", err)
	}

	observed, err := tx.Get(ctx, observedCollectionName, id)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return nil, fmt.Errorf("failed to load observed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	snapshot := &Snapshot{
		Identity: identity,
		Desired:  desired,
		Observed: observed,
	}

	// Capture syncID AFTER loading data to avoid TOCTOU race.
	// If we use the syncID captured before loading, it may be stale by now.
	syncIDAtCacheTime := ts.syncID.Load()

	ts.cacheMutex.Lock()
	ts.snapshotCache[cacheKey] = &cachedSnapshot{
		snapshot: snapshot,
		syncID:   syncIDAtCacheTime,
	}
	ts.cacheMutex.Unlock()

	// Return a deep copy to prevent callers from corrupting the cache
	return cloneSnapshot(snapshot), nil
}

func (ts *TriangularStore) GetLastSyncID(_ context.Context) (int64, error) {
	return ts.syncID.Load(), nil
}

func (ts *TriangularStore) IncrementSyncID(_ context.Context) (int64, error) {
	newID := ts.syncID.Add(1)

	return newID, nil
}

func (ts *TriangularStore) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return ts.store.Close(ctx)
}

// cloneSnapshot creates a deep copy of a Snapshot to prevent cache corruption.
// Callers may mutate returned snapshots, so we must return copies, not references.
func cloneSnapshot(s *Snapshot) *Snapshot {
	if s == nil {
		return nil
	}

	return &Snapshot{
		Identity: deepCopyDocument(s.Identity),
		Desired:  deepCopyDocument(s.Desired),
		Observed: deepCopyObserved(s.Observed),
	}
}

// deepCopyDocument creates a shallow copy of a persistence.Document.
// For FSM v2's use case, documents contain only primitive types (strings, numbers, bools)
// so a shallow copy is sufficient. If nested maps/slices are needed, this should be
// extended to use json.Marshal/Unmarshal for true deep copy.
func deepCopyDocument(doc persistence.Document) persistence.Document {
	if doc == nil {
		return nil
	}

	copy := make(persistence.Document, len(doc))
	for k, v := range doc {
		copy[k] = v
	}

	return copy
}

// deepCopyObserved creates a copy of the Observed field which can be either
// a persistence.Document or a typed struct.
func deepCopyObserved(observed interface{}) interface{} {
	if observed == nil {
		return nil
	}

	// If it's a Document, use the document copy function
	if doc, ok := observed.(persistence.Document); ok {
		return deepCopyDocument(doc)
	}

	// For typed structs, use JSON round-trip for deep copy
	// This handles any nested fields properly
	data, err := json.Marshal(observed)
	if err != nil {
		// If marshal fails, return original (shouldn't happen for valid structs)
		return observed
	}

	// Create a new instance of the same type
	newVal := reflect.New(reflect.TypeOf(observed)).Interface()
	if err := json.Unmarshal(data, newVal); err != nil {
		return observed
	}

	// Return the value, not the pointer
	return reflect.ValueOf(newVal).Elem().Interface()
}

// toDocument converts various input types to persistence.Document.
//
// DESIGN DECISION: Accept interface{} and handle common cases
// WHY: Allows TriangularStore to work with both Documents and typed structs.
// Provides flexibility without requiring callers to marshal manually.
//
// Supported types:
//   - persistence.Document: Pass through as-is
//   - map[string]interface{}: Convert to Document
//   - structs with json tags: Marshal via encoding/json
//
// Returns:
//   - persistence.Document: Converted document
//   - error: If conversion fails
func (ts *TriangularStore) toDocument(v interface{}) (persistence.Document, error) {
	if v == nil {
		return nil, errors.New("cannot convert nil to document")
	}

	// Already a Document
	if doc, ok := v.(persistence.Document); ok {
		return doc, nil
	}

	// map[string]interface{}
	if m, ok := v.(map[string]interface{}); ok {
		return persistence.Document(m), nil
	}

	// For structs and other types, use JSON marshaling
	// This handles most Go types including time.Time, nested structs, etc.
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal to JSON: %w", err)
	}

	var doc persistence.Document
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to document: %w", err)
	}

	// Ensure the document is not nil (can happen if JSON was "null")
	if doc == nil {
		return nil, errors.New("unmarshaled document is nil")
	}

	return doc, nil
}

// documentToStruct deserializes a Document into a typed struct.
//
// DESIGN DECISION: Use JSON marshaling for proper type conversion
// WHY: Handles complex types like time.Time, nested structs, and slices correctly.
// JSON round-trip (Document → JSON → struct) ensures proper type conversion.
//
// TRADE-OFF: Slightly slower than direct field mapping, but much more robust.
// Acceptable because this is a persistence boundary operation (infrequent).
//
// INSPIRED BY: encoding/json Unmarshal, GORM scan pattern.
//
// Parameters:
//   - doc: Source document (can be nil)
//   - dest: Pointer to destination struct
//
// Returns:
//   - error: If dest is not a pointer, JSON marshaling fails, or unmarshal fails
func documentToStruct(doc persistence.Document, dest interface{}) error {
	if doc == nil {
		return persistence.ErrNotFound
	}

	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr {
		return fmt.Errorf("dest must be pointer, got %s", destVal.Kind())
	}

	// Marshal Document to JSON, then unmarshal to typed struct
	// This handles time.Time, nested structs, and complex types properly
	jsonBytes, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document to JSON: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, dest); err != nil {
		return fmt.Errorf("failed to unmarshal JSON to struct: %w", err)
	}

	return nil
}

// LoadDesiredTyped loads desired state using generics (no registry dependency).
//
// DESIGN DECISION: Fully independent generic implementation
// WHY: Type parameter T provides collection name via DeriveCollectionName[T](RoleDesired).
//
//	No registry lookup needed.
//
// Parameters:
//   - ts: TriangularStore instance
//   - ctx: Cancellation context
//   - id: Unique worker identifier
//
// Returns:
//   - Desired state struct (strongly typed)
//   - error: ErrNotFound if worker doesn't exist
//
// Example:
//
//	result, err := storage.LoadDesiredTyped[ParentDesiredState](ts, ctx, "parent-001")
//	// result is ParentDesiredState (not interface{})
func LoadDesiredTyped[T any](ts *TriangularStore, ctx context.Context, id string) (T, error) {
	var result T

	collectionName, err := DeriveCollectionName[T](RoleDesired)
	if err != nil {
		return result, fmt.Errorf("failed to derive collection name: %w", err)
	}

	// Load from persistence
	doc, err := ts.store.Get(ctx, collectionName, id)
	if err != nil {
		return result, err
	}

	// Unmarshal Document to typed struct
	if err := documentToStruct(doc, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal to type %T: %w", result, err)
	}

	return result, nil
}

// SaveDesiredTyped saves desired state with delta checking using generics (no registry dependency).
//
// DESIGN DECISION: Fully independent generic implementation
// WHY: Type parameter T provides all information needed:
//   - workerType from DeriveWorkerType[T]()
//   - collectionName from DeriveCollectionName[T](RoleDesired)
//   - CSE fields from getCSEFields(RoleDesired)
//
// DESIGN DECISION: Built-in delta checking skips unchanged writes
// WHY: Desired state can be written frequently. Skipping redundant writes reduces
// database load and prevents unnecessary sync_id increments, enabling efficient
// delta streaming to clients.
//
// Delta checking behavior:
//   - First save (worker doesn't exist): Always writes, returns (true, nil)
//   - Data changed: Writes to database, returns (true, nil)
//   - Data unchanged: Skips write, returns (false, nil)
//   - Error: Returns (false, err)
//
// CSE metadata auto-injected (only when data changes):
//   - _sync_id: Incremented after successful save
//   - _version: Incremented on every update (optimistic locking)
//   - _created_at: Set on first save
//   - _updated_at: Set on every save
//
// Parameters:
//   - ts: TriangularStore instance
//   - ctx: Cancellation context
//   - id: Unique worker identifier
//   - desired: Desired state struct
//
// Returns:
//   - changed: true if data was written to database, false if write was skipped
//   - err: non-nil if operation failed (changed is always false when err != nil)
//
// Example:
//
//	desired := ParentDesiredState{Name: "Worker1", Command: "start"}
//	changed, err := storage.SaveDesiredTyped[ParentDesiredState](ts, ctx, "parent-001", desired)
func SaveDesiredTyped[T any](ts *TriangularStore, ctx context.Context, id string, desired T) (changed bool, err error) {
	// Marshal struct to Document
	bytes, err := json.Marshal(desired)
	if err != nil {
		return false, fmt.Errorf("failed to marshal desired state: %w", err)
	}

	desiredDoc := make(persistence.Document)
	if err := json.Unmarshal(bytes, &desiredDoc); err != nil {
		return false, fmt.Errorf("failed to unmarshal to Document: %w", err)
	}

	// Add ID if not present
	if _, ok := desiredDoc["id"]; !ok {
		desiredDoc["id"] = id
	}

	workerType, err := DeriveWorkerType[T]()
	if err != nil {
		return false, fmt.Errorf("failed to derive worker type: %w", err)
	}

	changed, _, err = ts.saveWithDelta(ctx, workerType, id, desiredDoc, DesiredSaveOptions)

	return changed, err
}

// LoadObservedTyped loads observed state using generics (no registry dependency).
//
// DESIGN DECISION: Fully independent generic implementation
// WHY: Type parameter T provides collection name via DeriveCollectionName[T](RoleObserved).
//
//	No registry lookup needed.
//
// Parameters:
//   - ts: TriangularStore instance
//   - ctx: Cancellation context
//   - id: Unique worker identifier
//
// Returns:
//   - Observed state struct (strongly typed)
//   - error: ErrNotFound if worker doesn't exist
//
// Example:
//
//	result, err := storage.LoadObservedTyped[ParentObservedState](ts, ctx, "parent-001")
//	// result is ParentObservedState (not interface{})
func LoadObservedTyped[T any](ts *TriangularStore, ctx context.Context, id string) (T, error) {
	var result T

	collectionName, err := DeriveCollectionName[T](RoleObserved)
	if err != nil {
		return result, fmt.Errorf("failed to derive collection name: %w", err)
	}

	// Load from persistence
	doc, err := ts.store.Get(ctx, collectionName, id)
	if err != nil {
		return result, err
	}

	// Use documentToStruct helper for deserialization
	if err := documentToStruct(doc, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal to type %T: %w", result, err)
	}

	return result, nil
}

// SaveObservedTyped saves observed state using generics (no registry dependency).
//
// DESIGN DECISION: Fully independent generic implementation
// WHY: Type parameter T provides all information needed:
//   - workerType from DeriveWorkerType[T]()
//   - collectionName from DeriveCollectionName[T](RoleObserved)
//   - CSE fields from getCSEFields(RoleObserved)
//
// DELTA CHECKING: Compares new data against existing data (excluding CSE fields).
// Returns true if business data changed, false if only CSE metadata updated.
//
// CSE metadata auto-injected:
//   - _sync_id: Incremented after successful save
//   - _version: Preserved from existing (observed doesn't increment version)
//   - _created_at: Set on first save
//   - _updated_at: Set on every save
//
// Parameters:
//   - ts: TriangularStore instance
//   - ctx: Cancellation context
//   - id: Unique worker identifier
//   - observed: Observed state struct
//
// Returns:
//   - changed: True if business data changed (not just CSE metadata)
//   - error: If save fails
//
// Example:
//
//	observed := ParentObservedState{Name: "Worker1", Status: "running"}
//	changed, err := storage.SaveObservedTyped[ParentObservedState](ts, ctx, "parent-001", observed)
//	// changed=true if this is a new write or data changed, false if identical to previous
func SaveObservedTyped[T any](ts *TriangularStore, ctx context.Context, id string, observed T) (bool, error) {
	// Marshal struct to Document
	observedDoc := make(persistence.Document)

	bytes, err := json.Marshal(observed)
	if err != nil {
		return false, fmt.Errorf("failed to marshal observed state: %w", err)
	}

	if err := json.Unmarshal(bytes, &observedDoc); err != nil {
		return false, fmt.Errorf("failed to unmarshal to Document: %w", err)
	}

	// Add ID if not present (convenience)
	if _, ok := observedDoc["id"]; !ok {
		observedDoc["id"] = id
	}

	// Use unified write path with observed-specific options
	workerType, err := DeriveWorkerType[T]()
	if err != nil {
		return false, fmt.Errorf("failed to derive worker type: %w", err)
	}

	changed, _, err := ts.saveWithDelta(ctx, workerType, id, observedDoc, ObservedSaveOptions)

	return changed, err
}

func DeriveWorkerType[T any]() (string, error) {
	// Use TypeOf on interface to avoid nil pointer issues with pointer types
	t := reflect.TypeOf((*T)(nil)).Elem()
	// Handle pointer types by getting the element type
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	typeName := t.Name()

	if typeName == "" {
		return "", errors.New("deriveWorkerType: type has empty name")
	}

	if strings.HasSuffix(typeName, "DesiredState") {
		workerType := strings.TrimSuffix(typeName, "DesiredState")

		return strings.ToLower(workerType), nil
	}

	if strings.HasSuffix(typeName, "ObservedState") {
		workerType := strings.TrimSuffix(typeName, "ObservedState")

		return strings.ToLower(workerType), nil
	}

	return "", fmt.Errorf("deriveWorkerType: type %q does not end with DesiredState or ObservedState", typeName)
}

// DeriveCollectionName derives the collection name for a given type and role.
//
// DESIGN DECISION: Use reflection to derive workerType, then apply naming convention
// WHY: Type parameter T already identifies the worker type uniquely.
//
//	Convention-based naming eliminates need for registry lookup.
//
// CONVENTION: {workerType}_{role}
//   - container_observed
//   - relay_desired
//   - communicator_identity
//
// Parameters:
//   - role: The triangular role (identity, desired, observed)
//
// Returns:
//   - Collection name following convention
//
// Example:
//
//	type ContainerObservedState struct { ... }
//	collectionName, err := DeriveCollectionName[ContainerObservedState](RoleObserved)
//	// Returns: "container_observed", nil
func DeriveCollectionName[T any](role string) (string, error) {
	workerType, err := DeriveWorkerType[T]()
	if err != nil {
		return "", err
	}

	var suffix string

	switch role {
	case RoleIdentity:
		suffix = "_identity"
	case RoleDesired:
		suffix = "_desired"
	case RoleObserved:
		suffix = "_observed"
	default:
		return "", fmt.Errorf("unknown role: %s", role)
	}

	return workerType + suffix, nil
}

// GetDeltas returns delta changes for sync clients.
//
// DESIGN DECISION: Return deltas or bootstrap based on client position.
// WHY: Clients may be too far behind to receive incremental deltas.
// In that case, return full bootstrap data to re-sync.
//
// FLOW:
//  1. If client is at current position → return empty deltas
//  2. Query delta store for changes since client's last position
//  3. If deltas found → return Delta array
//  4. If no deltas (too far behind or no delta store) → return bootstrap
//
// Parameters:
//   - ctx: Cancellation context
//   - sub: Client's subscription (last sync position, optional filters)
//
// Returns:
//   - DeltasResponse with either deltas or bootstrap data
//   - error if operation fails
func (ts *TriangularStore) GetDeltas(ctx context.Context, sub Subscription) (DeltasResponse, error) {
	currentSyncID := ts.syncID.Load()

	// If client is at current position, no changes needed
	if sub.LastSyncID >= currentSyncID {
		return DeltasResponse{
			Deltas:       []Delta{},
			LatestSyncID: currentSyncID,
			HasMore:      false,
		}, nil
	}

	// Query delta store for changes since client's position
	const deltaLimit = 100

	if ts.deltaStore != nil {
		entries, err := ts.deltaStore.GetAllSince(ctx, sub.LastSyncID, deltaLimit)
		if err != nil {
			ts.logger.Warnw("failed to query deltas, falling back to bootstrap",
				"error", err,
				"lastSyncID", sub.LastSyncID)
		} else if len(entries) > 0 {
			// Convert DeltaEntry to Delta
			deltas := make([]Delta, 0, len(entries))
			for _, entry := range entries {
				deltas = append(deltas, Delta{
					SyncID:      entry.SyncID,
					WorkerType:  entry.WorkerType,
					WorkerID:    entry.ID,
					Role:        entry.Role,
					Changes:     entry.Changes,
					TimestampMs: entry.Timestamp.UnixMilli(),
				})
			}

			return DeltasResponse{
				Deltas:       deltas,
				LatestSyncID: currentSyncID,
				HasMore:      len(deltas) >= deltaLimit,
			}, nil
		}
	}

	// Fallback to bootstrap if no deltas available
	bootstrap, err := ts.buildBootstrap(ctx, currentSyncID)
	if err != nil {
		return DeltasResponse{}, fmt.Errorf("failed to build bootstrap: %w", err)
	}

	return DeltasResponse{
		RequiresBootstrap: true,
		Bootstrap:         bootstrap,
		LatestSyncID:      currentSyncID,
	}, nil
}

// buildBootstrap creates full state snapshot for clients that need to re-sync.
// It iterates over all known worker types and builds a complete snapshot of each worker.
func (ts *TriangularStore) buildBootstrap(ctx context.Context, atSyncID int64) (*BootstrapData, error) {
	workers := make([]WorkerSnapshot, 0)

	// Get a copy of known worker types to avoid holding the lock during DB operations
	ts.knownWorkerTypesMu.RLock()

	workerTypes := make([]string, 0, len(ts.knownWorkerTypes))
	for wt := range ts.knownWorkerTypes {
		workerTypes = append(workerTypes, wt)
	}

	ts.knownWorkerTypesMu.RUnlock()

	// Iterate over all known worker types
	for _, workerType := range workerTypes {
		// Query identity collection to get all worker IDs for this type
		identityCollection := workerType + "_identity"

		identityDocs, err := ts.store.Find(ctx, identityCollection, persistence.Query{})
		if err != nil {
			// Collection may not exist or be empty - skip silently
			continue
		}

		// Build snapshot for each worker
		for _, identityDoc := range identityDocs {
			id, ok := identityDoc["id"].(string)
			if !ok {
				continue // Skip invalid documents
			}

			// Load the full snapshot (includes identity, desired, observed)
			snapshot, err := ts.LoadSnapshot(ctx, workerType, id)
			if err != nil {
				// Skip workers that can't be loaded
				continue
			}

			// Convert Observed to persistence.Document (may be nil or interface{})
			var observedDoc persistence.Document
			if snapshot.Observed != nil {
				if doc, ok := snapshot.Observed.(persistence.Document); ok {
					observedDoc = doc
				}
			}

			workers = append(workers, WorkerSnapshot{
				WorkerType: workerType,
				WorkerID:   id,
				Identity:   snapshot.Identity,
				Desired:    snapshot.Desired,
				Observed:   observedDoc,
			})
		}
	}

	return &BootstrapData{
		Workers:     workers,
		AtSyncID:    atSyncID,
		TimestampMs: time.Now().UnixMilli(),
	}, nil
}

// GetLatestSyncID returns the current sync_id.
// This can be used by clients to establish their initial sync position.
func (ts *TriangularStore) GetLatestSyncID(ctx context.Context) (int64, error) {
	return ts.syncID.Load(), nil
}
