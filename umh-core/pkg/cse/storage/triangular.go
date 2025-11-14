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
	"sync/atomic"
	"time"

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
//   - Identity: Immutable worker identification (ID, Name, IP)
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
//	ts := cse.NewTriangularStore(sqliteStore, globalRegistry)
//
//	// Create worker
//	ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
//	    "id": "worker-123",
//	    "name": "Container A",
//	    "ip": "192.168.1.100",
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
	store        persistence.Store
	registry     *Registry
	typeRegistry *TypeRegistry
	syncID       *atomic.Int64
}

// NewTriangularStore creates a new TriangularStore.
//
// DESIGN DECISION: Require explicit registry injection
// WHY: Makes dependencies explicit, supports testing with custom registries.
// Registry defines which collections exist and their metadata conventions.
//
// TRADE-OFF: More verbose than using global registry, but more testable.
//
// INSPIRED BY: Dependency injection pattern, avoiding global state in constructors.
//
// Parameters:
//   - store: Backend storage implementation (SQLite, Postgres, etc.)
//   - registry: Schema registry with triangular collection metadata
//
// Returns:
//   - *TriangularStore: Ready-to-use triangular store instance
func NewTriangularStore(store persistence.Store, registry *Registry) *TriangularStore {
	return &TriangularStore{
		store:        store,
		registry:     registry,
		typeRegistry: NewTypeRegistry(),
		syncID:       &atomic.Int64{},
	}
}

// TypeRegistry returns the type registry for worker type registration.
func (ts *TriangularStore) TypeRegistry() *TypeRegistry {
	return ts.typeRegistry
}

// SaveIdentity stores immutable worker identity.
//
// DESIGN DECISION: Identity is created once and never updated
// WHY: Identity fields (IP, hostname, bootstrap config) don't change.
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
//   - identity: Identity document (id, name, ip, etc.)
//
// Returns:
//   - error: If worker type not registered or insertion fails
//
// Example:
//
//	err := ts.SaveIdentity(ctx, "container", "worker-123", persistence.Document{
//	    "id": "worker-123",
//	    "name": "Container A",
//	    "ip": "192.168.1.100",
//	})
func (ts *TriangularStore) SaveIdentity(ctx context.Context, workerType string, id string, identity persistence.Document) error {
	// Validate document has required fields
	if err := ts.validateDocument(identity); err != nil {
		return fmt.Errorf("invalid identity document: %w", err)
	}

	// Look up identity collection for this worker type
	identityMeta, _, _, err := ts.registry.GetTriangularCollections(workerType)
	if err != nil {
		return fmt.Errorf("worker type %q not registered: %w", workerType, err)
	}

	// Inject CSE metadata (without sync ID - will be set after successful insert)
	ts.injectMetadata(identity, RoleIdentity, true)

	// Insert identity (first time creation)
	_, err = ts.store.Insert(ctx, identityMeta.Name, identity)
	if err != nil {
		return fmt.Errorf("failed to save identity for %s/%s: %w", workerType, id, err)
	}

	// Increment sync ID ONLY after successful database commit
	// This prevents gaps in sync ID sequence when operations fail
	syncID := ts.syncID.Add(1)
	identity[FieldSyncID] = syncID

	// Update the document in database with sync ID
	// This is safe because we know the document exists (we just inserted it)
	err = ts.store.Update(ctx, identityMeta.Name, id, identity)
	if err != nil {
		// This is a critical error - document exists but we couldn't set sync ID
		// The sync ID counter is already incremented, creating a gap
		// Log this but don't fail the operation (identity was successfully created)
		return fmt.Errorf("failed to set sync ID after identity creation for %s/%s: %w", workerType, id, err)
	}

	return nil
}

// LoadIdentity retrieves worker identity.
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
	identityMeta, _, _, err := ts.registry.GetTriangularCollections(workerType)
	if err != nil {
		return nil, fmt.Errorf("worker type %q not registered: %w", workerType, err)
	}

	doc, err := ts.store.Get(ctx, identityMeta.Name, id)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// SaveDesired stores user intent/configuration.
//
// DESIGN DECISION: Increment _version for optimistic locking
// WHY: Desired state represents user configuration. Version prevents lost updates
// when multiple clients modify configuration concurrently.
//
// TRADE-OFF: Callers must handle version conflicts (retry logic).
// Alternative would be last-write-wins, but that loses concurrent updates.
//
// INSPIRED BY: Optimistic locking in ORMs (Hibernate, Entity Framework),
// Linear's version-based conflict resolution.
//
// CSE metadata injected/updated:
//   - _sync_id: Incremented (for delta sync)
//   - _version: Incremented (for optimistic locking)
//   - _updated_at: Current timestamp
//   - _created_at: Set if first save
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type
//   - id: Unique worker identifier
//   - desired: Desired state document
//
// Returns:
//   - error: If worker type not registered or save fails
func (ts *TriangularStore) SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) error {
	// Validate document has required fields
	if err := ts.validateDocument(desired); err != nil {
		return fmt.Errorf("invalid desired document: %w", err)
	}

	// Collection name follows convention: {workerType}_desired
	collectionName := workerType + "_desired"

	// Check if this is first save or update
	_, err := ts.store.Get(ctx, collectionName, id)
	isNew := err != nil && errors.Is(err, persistence.ErrNotFound)

	// Inject CSE metadata (without sync ID - will be set after successful operation)
	ts.injectMetadata(desired, RoleDesired, isNew)

	if isNew {
		_, err = ts.store.Insert(ctx, collectionName, desired)
	} else {
		err = ts.store.Update(ctx, collectionName, id, desired)
	}

	if err != nil {
		return fmt.Errorf("failed to save desired for %s/%s: %w", workerType, id, err)
	}

	// Increment sync ID ONLY after successful database commit
	// This prevents gaps in sync ID sequence when operations fail
	syncID := ts.syncID.Add(1)
	desired[FieldSyncID] = syncID

	// Update the document in database with sync ID
	err = ts.store.Update(ctx, collectionName, id, desired)
	if err != nil {
		// This is a critical error - document exists but we couldn't set sync ID
		// The sync ID counter is already incremented, creating a gap
		return fmt.Errorf("failed to set sync ID after desired save for %s/%s: %w", workerType, id, err)
	}

	return nil
}

// LoadDesired retrieves user intent/configuration.
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

// SaveObserved stores system reality.
//
// DESIGN DECISION: Accept interface{} for flexibility with typed states
// WHY: Allows storing either persistence.Document OR typed ObservedState structs.
// Auto-marshals typed states to Documents transparently.
//
// DESIGN DECISION: Increment _sync_id but NOT _version
// WHY: Observed state is ephemeral (reconstructed from polling external systems).
// It doesn't participate in optimistic locking - only user intent (desired) does.
//
// TRADE-OFF: Can't detect concurrent observed updates, but not needed.
// Observed state is always overwritten by latest poll results.
//
// INSPIRED BY: FSM v2 design (desired is user intent, observed is system reality),
// CQRS pattern (write side doesn't version read models).
//
// CSE metadata injected/updated:
//   - _sync_id: Incremented (for delta sync)
//   - _version: NOT incremented (observed is ephemeral)
//   - _updated_at: Current timestamp
//   - _created_at: Set if first save
//
// Parameters:
//   - ctx: Cancellation context
//   - workerType: Worker type
//   - id: Unique worker identifier
//   - observed: Observed state (persistence.Document or any struct/map)
//
// Returns:
//   - error: If worker type not registered or save fails
func (ts *TriangularStore) saveObservedInternal(ctx context.Context, workerType string, id string, observed interface{}) error {
	// Convert observed to Document if needed
	observedDoc, err := ts.toDocument(observed)
	if err != nil {
		return fmt.Errorf("failed to convert observed to document: %w", err)
	}

	if observedDoc == nil {
		return errors.New("toDocument returned nil document")
	}

	// Validate that 'id' field exists in the observed document
	if _, hasID := observedDoc["id"]; !hasID {
		return fmt.Errorf("observed document must have 'id' field")
	}

	// Add required 'id' field for document validation
	observedDoc["id"] = id

	// Validate document has required fields
	if err := ts.validateDocument(observedDoc); err != nil {
		return fmt.Errorf("invalid observed document: %w", err)
	}

	// Collection name follows convention: {workerType}_observed
	collectionName := workerType + "_observed"

	// Check if this is first save or update
	currentDoc, err := ts.store.Get(ctx, collectionName, id)
	isNew := err != nil && errors.Is(err, persistence.ErrNotFound)

	// For updates, preserve the version field (observed state doesn't increment version)
	if !isNew && currentDoc != nil {
		if version, ok := currentDoc[FieldVersion]; ok {
			observedDoc[FieldVersion] = version
		}
	}

	// Inject CSE metadata (without sync ID - will be set after successful operation)
	ts.injectMetadata(observedDoc, RoleObserved, isNew)

	if isNew {
		_, err = ts.store.Insert(ctx, collectionName, observedDoc)
	} else {
		err = ts.store.Update(ctx, collectionName, id, observedDoc)
	}

	if err != nil {
		return fmt.Errorf("failed to save observed for %s/%s: %w", workerType, id, err)
	}

	// Increment sync ID ONLY after successful database commit
	// This prevents gaps in sync ID sequence when operations fail
	syncID := ts.syncID.Add(1)
	observedDoc[FieldSyncID] = syncID

	// Update the document in database with sync ID
	err = ts.store.Update(ctx, collectionName, id, observedDoc)
	if err != nil {
		// This is a critical error - document exists but we couldn't set sync ID
		// The sync ID counter is already incremented, creating a gap
		return fmt.Errorf("failed to set sync ID after observed save for %s/%s: %w", workerType, id, err)
	}

	return nil
}

// SaveObserved stores system reality with automatic delta checking.
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
	// Try to load current state
	currentState, err := ts.LoadObserved(ctx, workerType, id)
	if errors.Is(err, persistence.ErrNotFound) {
		// First save - always write
		err = ts.saveObservedInternal(ctx, workerType, id, observed)
		return true, err
	}
	if err != nil {
		return false, err
	}

	// Convert current state to Document (might be typed struct or Document)
	currentDoc, err := ts.toDocument(currentState)
	if err != nil {
		return false, fmt.Errorf("failed to convert current state to document: %w", err)
	}

	// Convert new state to Document
	newDoc, err := ts.toDocument(observed)
	if err != nil {
		return false, err
	}

	// Filter CSE fields from observed data for comparison (using constants, not registry)
	cseFields := getCSEFields(RoleObserved)
	currentFiltered := ts.filterCSEFields(currentDoc, cseFields)
	delete(currentFiltered, "id")
	delete(currentFiltered, FieldVersion)
	newFiltered := ts.filterCSEFields(newDoc, cseFields)
	delete(newFiltered, "id")
	delete(newFiltered, FieldVersion)

	// Compare filtered documents
	if reflect.DeepEqual(currentFiltered, newFiltered) {
		return false, nil // No changes, skip write
	}

	// Data changed, perform write
	err = ts.saveObservedInternal(ctx, workerType, id, observed)
	return true, err
}

// LoadObserved retrieves system reality.
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

func observedCollectionName(workerType string) string {
	return workerType + "_observed"
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
	identityMeta, desiredMeta, observedMeta, err := ts.registry.GetTriangularCollections(workerType)
	if err != nil {
		return nil, fmt.Errorf("worker type %q not registered: %w", workerType, err)
	}

	// Use transaction for atomic read
	tx, err := ts.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Load all three parts
	identity, err := tx.Get(ctx, identityMeta.Name, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load identity: %w", err)
	}

	desired, err := tx.Get(ctx, desiredMeta.Name, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load desired: %w", err)
	}

	observed, err := tx.Get(ctx, observedMeta.Name, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load observed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &Snapshot{
		Identity: identity,
		Desired:  desired,
		Observed: observed,
	}, nil
}

// injectMetadata adds or updates CSE metadata fields in a document.
//
// DESIGN DECISION: Mutate document in-place instead of returning new document
// WHY: Simple and efficient - caller already provides document to save.
// No need to create defensive copies.
//
// TRADE-OFF: Modifies caller's document, but this is expected behavior
// (caller explicitly calls Save, expects metadata to be added).
//
// INSPIRED BY: ORM before_save callbacks (Rails, Django),
// Linear's transparent metadata injection.
//
// Metadata injected/updated based on role:
//   - RoleIdentity: _version=1, _created_at (immutable after creation)
//   - RoleDesired: _version++, _updated_at (increments version)
//   - RoleObserved: _updated_at (does NOT increment version)
//
// NOTE: _sync_id is NOT set here - it's incremented AFTER successful database commit
// to prevent gaps in sync ID sequence when operations fail.
//
// Parameters:
//   - doc: Document to inject metadata into (mutated in-place)
//   - role: Triangular model role (identity, desired, observed)
//   - isNew: True if first save, false if update
func (ts *TriangularStore) injectMetadata(doc persistence.Document, role string, isNew bool) {
	now := time.Now().UTC()

	if isNew {
		// First save: set creation timestamp and initial version
		doc[FieldCreatedAt] = now
		doc[FieldVersion] = int64(1)
	} else {
		// Update: set update timestamp
		doc[FieldUpdatedAt] = now

		// Increment version only for desired state (optimistic locking)
		// Observed state is ephemeral and doesn't participate in versioning
		if role == RoleDesired {
			currentVersion, ok := doc[FieldVersion].(int64)
			if !ok {
				// If version field doesn't exist or wrong type, start at 1
				currentVersion = 0
			}

			doc[FieldVersion] = currentVersion + 1
		}
	}
}

// validateDocument checks that a document has the required "id" field.
//
// DESIGN DECISION: Fail fast with validation before save
// WHY: Prevent invalid documents from being stored. ID is required for
// all triangular model documents (used as primary key).
//
// TRADE-OFF: Additional validation overhead, but prevents data corruption.
//
// INSPIRED BY: "Parse, don't validate" principle - ensure valid state.
func (ts *TriangularStore) validateDocument(doc persistence.Document) error {
	if doc == nil {
		return errors.New("document cannot be nil")
	}

	if doc["id"] == nil {
		return errors.New("document must have 'id' field")
	}

	return nil
}

func (ts *TriangularStore) GetLastSyncID(_ context.Context) (int64, error) {
	return ts.syncID.Load(), nil
}

func (ts *TriangularStore) IncrementSyncID(_ context.Context) (int64, error) {
	newID := ts.syncID.Add(1)

	return newID, nil
}

// Registry returns the underlying registry for auto-registration.
// Used by Supervisor to register worker types at initialization.
//
// DESIGN DECISION: Expose registry for auto-registration by Supervisor
// WHY: Eliminates worker-specific registry boilerplate (e.g., communicator/registry.go)
// Supervisor auto-registers collections at startup based on worker type.
//
// TRADE-OFF: Exposes internal registry reference, but necessary for auto-registration pattern.
//
// INSPIRED BY: Dependency injection pattern, HTTP router registration (gin.Engine.Routes()).
func (ts *TriangularStore) Registry() *Registry {
	return ts.registry
}

func (ts *TriangularStore) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return ts.store.Close(ctx)
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

func structToDocument(v interface{}) (persistence.Document, error) {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var doc persistence.Document
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, err
	}

	return doc, nil
}

// LoadDesired retrieves user intent/configuration using generic type parameter.
//
// This is a package-level generic function that provides type-safe access to desired state.
// It derives the workerType from the type parameter T and delegates to TriangularStore.LoadDesired.
//
// Type parameter T must be a struct ending in "DesiredState" (e.g., ParentDesiredState).
// The workerType is derived by removing "DesiredState" suffix and lowercasing.
//
// Parameters:
//   - ts: TriangularStore instance
//   - ctx: Cancellation context
//   - id: Unique worker identifier
//
// Returns:
//   - T: Desired state as typed struct
//   - error: ErrNotFound if not found, or deserialization error
//
// Example:
//
//	result, err := storage.LoadDesired[ParentDesiredState](ts, ctx, "parent-001")
//	// result is ParentDesiredState (not interface{})
func LoadDesiredTyped[T any](ts *TriangularStore, ctx context.Context, id string) (T, error) {
	var zero T
	workerType := DeriveWorkerType[T]()

	result, err := ts.LoadDesired(ctx, workerType, id)
	if err != nil {
		return zero, err
	}

	// If result is already the correct type, return it directly
	if typed, ok := result.(T); ok {
		return typed, nil
	}

	// Otherwise, deserialize Document to typed struct
	doc, ok := result.(persistence.Document)
	if !ok {
		return zero, fmt.Errorf("expected persistence.Document but got %T", result)
	}

	var dest T
	if err := documentToStruct(doc, &dest); err != nil {
		return zero, fmt.Errorf("failed to deserialize to %T: %w", zero, err)
	}

	return dest, nil
}

// SaveDesiredTyped saves user intent/configuration using generic type parameter.
//
// This is a package-level generic function that provides type-safe saving of desired state.
// It derives the workerType from the type parameter T and delegates to TriangularStore.SaveDesired.
//
// Type parameter T must be a struct ending in "DesiredState" (e.g., ParentDesiredState).
// The workerType is derived by removing "DesiredState" suffix and lowercasing.
//
// Parameters:
//   - ts: TriangularStore instance
//   - ctx: Cancellation context
//   - id: Unique worker identifier
//   - desired: Desired state as typed struct
//
// Returns:
//   - error: Serialization error or database error
//
// Example:
//
//	desired := ParentDesiredState{Name: "Worker1", Command: "start"}
//	err := storage.SaveDesiredTyped[ParentDesiredState](ts, ctx, "parent-001", desired)
func SaveDesiredTyped[T any](ts *TriangularStore, ctx context.Context, id string, desired T) error {
	workerType := DeriveWorkerType[T]()

	doc, err := structToDocument(desired)
	if err != nil {
		return fmt.Errorf("failed to serialize %T: %w", desired, err)
	}

	doc["id"] = id

	return ts.SaveDesired(ctx, workerType, id, doc)
}

// LoadObservedTyped retrieves system reality using generic type parameter.
//
// This is a package-level generic function that provides type-safe access to observed state.
// It derives the workerType from the type parameter T and delegates to TriangularStore.LoadObserved.
//
// Type parameter T must be a struct ending in "ObservedState" (e.g., ParentObservedState).
// The workerType is derived by removing "ObservedState" suffix and lowercasing.
//
// Parameters:
//   - ts: TriangularStore instance
//   - ctx: Cancellation context
//   - id: Unique worker identifier
//
// Returns:
//   - T: Observed state as typed struct
//   - error: ErrNotFound if not found, or deserialization error
//
// Example:
//
//	result, err := storage.LoadObservedTyped[ParentObservedState](ts, ctx, "parent-001")
//	// result is ParentObservedState (not interface{})
func LoadObservedTyped[T any](ts *TriangularStore, ctx context.Context, id string) (T, error) {
	var zero T
	workerType := DeriveWorkerType[T]()

	result, err := ts.LoadObserved(ctx, workerType, id)
	if err != nil {
		return zero, err
	}

	// If result is already the correct type, return it directly
	if typed, ok := result.(T); ok {
		return typed, nil
	}

	// Otherwise, deserialize Document to typed struct
	doc, ok := result.(persistence.Document)
	if !ok {
		return zero, fmt.Errorf("expected persistence.Document but got %T", result)
	}

	var dest T
	if err := documentToStruct(doc, &dest); err != nil {
		return zero, fmt.Errorf("failed to deserialize to %T: %w", zero, err)
	}

	return dest, nil
}

// SaveObservedTyped saves system reality using generic type parameter.
//
// This is a package-level generic function that provides type-safe saving of observed state.
// It derives the workerType from the type parameter T and delegates to TriangularStore.SaveObserved.
//
// Type parameter T must be a struct ending in "ObservedState" (e.g., ParentObservedState).
// The workerType is derived by removing "ObservedState" suffix and lowercasing.
//
// SaveObserved implements delta detection: it only writes to the database if the observed state
// has actually changed (excluding CSE metadata fields). This reduces unnecessary I/O and prevents
// spurious updates in change-sensitive workflows.
//
// Parameters:
//   - ts: TriangularStore instance
//   - ctx: Cancellation context
//   - id: Unique worker identifier
//   - observed: Observed state as typed struct
//
// Returns:
//   - changed: true if data was written (state changed), false if write was skipped (no change)
//   - error: Serialization error or database error
//
// Example:
//
//	observed := ParentObservedState{Name: "Worker1", Status: "running"}
//	changed, err := storage.SaveObservedTyped[ParentObservedState](ts, ctx, "parent-001", observed)
//	// changed=true if this is a new write or data changed, false if identical to previous
func SaveObservedTyped[T any](ts *TriangularStore, ctx context.Context, id string, observed T) (bool, error) {
	workerType := DeriveWorkerType[T]()

	doc, err := structToDocument(observed)
	if err != nil {
		return false, fmt.Errorf("failed to serialize %T: %w", observed, err)
	}

	doc["id"] = id

	return ts.SaveObserved(ctx, workerType, id, doc)
}

func DeriveWorkerType[T any]() string {
	var zero T
	typeName := reflect.TypeOf(zero).Name()

	if typeName == "" {
		panic("deriveWorkerType: type has empty name")
	}

	if strings.HasSuffix(typeName, "DesiredState") {
		workerType := strings.TrimSuffix(typeName, "DesiredState")
		return strings.ToLower(workerType)
	}

	if strings.HasSuffix(typeName, "ObservedState") {
		workerType := strings.TrimSuffix(typeName, "ObservedState")
		return strings.ToLower(workerType)
	}

	panic(fmt.Sprintf("deriveWorkerType: type %q does not end with DesiredState or ObservedState", typeName))
}
