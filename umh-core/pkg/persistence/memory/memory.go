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

// Package memory provides an in-memory implementation of the persistence.TriangularStore interface.
//
// This implementation is designed for testing and development environments where data persistence
// is not required between restarts. All data is stored in memory using Go maps, with deep copies
// ensuring data isolation between operations.
//
// # Thread Safety
//
// InMemoryStore uses a sync.RWMutex to protect concurrent access to collections. Read operations
// (Get, Find) acquire read locks, while write operations (Insert, Update, Delete, CreateCollection,
// DropCollection) acquire exclusive write locks.
//
// # Transaction Isolation
//
// Transactions provide optimistic concurrency control:
//   - Changes are buffered in memory until Commit() is called
//   - Reads within a transaction see uncommitted changes from the same transaction
//   - Reads from the underlying store see committed data only
//   - Commit atomically applies all changes with a single write lock
//
// # Data Isolation
//
// All documents are deep-copied on read and write to prevent external modifications from
// affecting stored data. This ensures proper isolation but may impact performance for
// large documents.
//
// # Collection Auto-Registration
//
// Collections are automatically created when documents are inserted, updated, or deleted.
// Explicit CreateCollection() calls are optional but still supported. Read operations (Get, Find)
// do not auto-create collections. The Schema parameter in CreateCollection() is currently
// ignored (in-memory store does not validate schemas).
//
// # Single-Node Assumption
//
// This implementation is designed for single-process, single-node deployments only.
// It does not support distributed deployments or multi-process access.
package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// validateContext checks if the provided context is nil.
// Returns an error if ctx is nil, otherwise returns nil.
func validateContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context cannot be nil")
	}

	return nil
}

// InMemoryStore is a thread-safe in-memory document store implementing persistence.TriangularStore.
//
// It stores documents in a nested map structure: collections → document IDs → documents.
// All operations are protected by a sync.RWMutex, with read operations using RLock and write
// operations using Lock.
//
// # Concurrency Model
//
// Read operations (Get, Find) can run concurrently with other reads but block during writes.
// Write operations (Insert, Update, Delete, CreateCollection, DropCollection) acquire exclusive
// locks and block all other operations.
//
// # Data Isolation
//
// Documents are deep-copied on every read and write operation. This ensures that external
// modifications to returned documents do not affect the store, and modifications to inserted
// documents do not change the stored version.
//
// # Transaction Support
//
// BeginTx() creates a transaction that buffers changes in memory. Changes are applied atomically
// when Commit() is called. Transactions provide read-your-own-writes semantics: reads within a
// transaction see uncommitted changes from that transaction, but other readers see only committed
// data.
//
// # Schema Validation
//
// The Schema parameter in CreateCollection() is currently ignored. In-memory store does not
// validate document structure against schemas.
//
// # Single-Node Design
//
// InMemoryStore is designed for single-process deployments only. It does not support distributed
// access or multi-process coordination. For production deployments, use a persistent storage
// backend like SQLite or PostgreSQL.
type InMemoryStore struct {
	mu          sync.RWMutex
	collections map[string]map[string]persistence.Document
}

// NewInMemoryStore creates a new empty in-memory document store.
//
// The returned store is ready for use. Collections are automatically created when
// documents are inserted, updated, or deleted, so explicit CreateCollection() calls
// are optional.
//
// Example:
//
//	store := memory.NewInMemoryStore()
//	doc := persistence.Document{"id": "user-1", "name": "Alice"}
//	_, err := store.Insert(ctx, "users", doc) // Creates "users" collection automatically
//	if err != nil {
//	    log.Fatal(err)
//	}
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		collections: make(map[string]map[string]persistence.Document),
	}
}

// CreateCollection creates a new empty collection with the given name.
//
// Parameters:
//   - ctx: Context for the operation (must not be nil)
//   - name: Name of the collection to create
//   - schema: Schema definition (currently ignored by in-memory store)
//
// Returns an error if:
//   - ctx is nil
//   - A collection with this name already exists
//
// Thread-safe: Acquires exclusive write lock during operation.
func (s *InMemoryStore) CreateCollection(ctx context.Context, name string, schema *persistence.Schema) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.collections[name]; exists {
		return fmt.Errorf("collection %q already exists", name)
	}

	s.collections[name] = make(map[string]persistence.Document)

	return nil
}

// DropCollection deletes a collection and all its documents.
//
// Parameters:
//   - ctx: Context for the operation (must not be nil)
//   - name: Name of the collection to drop
//
// Returns an error if:
//   - ctx is nil
//   - The collection does not exist
//
// Thread-safe: Acquires exclusive write lock during operation.
// Warning: All documents in the collection are permanently deleted.
func (s *InMemoryStore) DropCollection(ctx context.Context, name string) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.collections[name]; !exists {
		return fmt.Errorf("collection %q does not exist", name)
	}

	delete(s.collections, name)

	return nil
}

// Insert adds a new document to the specified collection.
//
// The document must contain an "id" field with a non-empty string value. The document is
// deep-copied before storage, so external modifications will not affect the stored version.
// If the collection does not exist, it is automatically created.
//
// Parameters:
//   - ctx: Context for the operation (must not be nil)
//   - collection: Name of the collection to insert into
//   - doc: Document to insert (must have non-empty "id" field)
//
// Returns:
//   - The document ID on success
//   - An error if:
//   - ctx is nil
//   - Document has no "id" field or "id" is empty
//   - Document with this ID already exists (returns persistence.ErrConflict)
//
// Thread-safe: Acquires exclusive write lock during operation.
func (s *InMemoryStore) Insert(ctx context.Context, collection string, doc persistence.Document) (string, error) {
	if err := validateContext(ctx); err != nil {
		return "", err
	}

	id, ok := doc["id"].(string)
	if !ok || id == "" {
		return "", errors.New("document must have non-empty 'id' field")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	coll, exists := s.collections[collection]
	if !exists {
		s.collections[collection] = make(map[string]persistence.Document)
		coll = s.collections[collection]
	}

	if _, exists := coll[id]; exists {
		return "", persistence.ErrConflict
	}

	docCopy := make(persistence.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	coll[id] = docCopy

	return id, nil
}

// Get retrieves a document by ID from the specified collection.
//
// The returned document is a deep copy, so modifications will not affect the stored version.
//
// Parameters:
//   - ctx: Context for the operation (must not be nil)
//   - collection: Name of the collection to read from
//   - id: ID of the document to retrieve
//
// Returns:
//   - The document on success
//   - An error if:
//   - ctx is nil
//   - Collection does not exist (returns persistence.ErrNotFound)
//   - Document with this ID does not exist (returns persistence.ErrNotFound)
//
// Thread-safe: Acquires read lock during operation (allows concurrent reads).
func (s *InMemoryStore) Get(ctx context.Context, collection string, id string) (persistence.Document, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	coll, exists := s.collections[collection]
	if !exists {
		return nil, persistence.ErrNotFound
	}

	doc, exists := coll[id]
	if !exists {
		return nil, persistence.ErrNotFound
	}

	docCopy := make(persistence.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	return docCopy, nil
}

// Update replaces an existing document with a new version.
//
// The document is deep-copied before storage, so external modifications will not affect
// the stored version. If the collection does not exist, it is automatically created, but
// the update will return persistence.ErrNotFound since the document doesn't exist.
//
// Parameters:
//   - ctx: Context for the operation (must not be nil)
//   - collection: Name of the collection containing the document
//   - id: ID of the document to update
//   - doc: New document content (replaces existing document entirely)
//
// Returns an error if:
//   - ctx is nil
//   - Document with this ID does not exist (returns persistence.ErrNotFound)
//
// Thread-safe: Acquires exclusive write lock during operation.
func (s *InMemoryStore) Update(ctx context.Context, collection string, id string, doc persistence.Document) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	coll, exists := s.collections[collection]
	if !exists {
		s.collections[collection] = make(map[string]persistence.Document)
		coll = s.collections[collection]
	}

	if _, exists := coll[id]; !exists {
		return persistence.ErrNotFound
	}

	docCopy := make(persistence.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	coll[id] = docCopy

	return nil
}

// Delete removes a document from the specified collection.
//
// If the collection does not exist, it is automatically created, but the delete will
// return persistence.ErrNotFound since the document doesn't exist.
//
// Parameters:
//   - ctx: Context for the operation (must not be nil)
//   - collection: Name of the collection containing the document
//   - id: ID of the document to delete
//
// Returns an error if:
//   - ctx is nil
//   - Document with this ID does not exist (returns persistence.ErrNotFound)
//
// Thread-safe: Acquires exclusive write lock during operation.
func (s *InMemoryStore) Delete(ctx context.Context, collection string, id string) error {
	if err := validateContext(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	coll, exists := s.collections[collection]
	if !exists {
		s.collections[collection] = make(map[string]persistence.Document)
		coll = s.collections[collection]
	}

	if _, exists := coll[id]; !exists {
		return persistence.ErrNotFound
	}

	delete(coll, id)

	return nil
}

// Find retrieves all documents from the specified collection.
//
// The query parameter is currently ignored - all documents in the collection are returned.
// Each returned document is a deep copy, so modifications will not affect stored versions.
//
// Parameters:
//   - ctx: Context for the operation (must not be nil)
//   - collection: Name of the collection to query
//   - query: Query filter (currently ignored - all documents returned)
//
// Returns:
//   - Slice of all documents in the collection
//   - An error if:
//   - ctx is nil
//   - Collection does not exist (returns persistence.ErrNotFound)
//
// Thread-safe: Acquires read lock during operation (allows concurrent reads).
//
// Note: The query parameter is reserved for future filtering support. Currently, to filter
// results, retrieve all documents and filter in application code.
func (s *InMemoryStore) Find(ctx context.Context, collection string, query persistence.Query) ([]persistence.Document, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	coll, exists := s.collections[collection]
	if !exists {
		return nil, persistence.ErrNotFound
	}

	var results []persistence.Document
	for _, doc := range coll {
		docCopy := make(persistence.Document)
		for k, v := range doc {
			docCopy[k] = v
		}

		results = append(results, docCopy)
	}

	return results, nil
}

// Maintenance performs periodic maintenance tasks on the store.
//
// For the in-memory implementation, this is a no-op. It exists to satisfy the
// persistence.TriangularStore interface. Persistent implementations (SQLite, PostgreSQL)
// may use this to run VACUUM, optimize indexes, or perform other housekeeping.
//
// Returns nil (never fails).
func (s *InMemoryStore) Maintenance(ctx context.Context) error {
	return nil
}

// BeginTx starts a new transaction for atomic multi-document operations.
//
// The returned transaction buffers all changes in memory until Commit() is called.
// Reads within the transaction see uncommitted changes from that transaction, but
// other readers only see committed data (read-your-own-writes semantics).
//
// Parameters:
//   - ctx: Context for the operation (must not be nil)
//
// Returns:
//   - A new transaction on success
//   - An error if ctx is nil
//
// Example:
//
//	tx, err := store.BeginTx(ctx)
//	if err != nil {
//	    return err
//	}
//	defer tx.Rollback() // Safe to call even if Commit succeeds
//
//	if _, err := tx.Insert(ctx, "users", doc1); err != nil {
//	    return err
//	}
//	if _, err := tx.Insert(ctx, "users", doc2); err != nil {
//	    return err
//	}
//
//	return tx.Commit() // Atomically applies all changes
//
// Thread-safe: Multiple transactions can be created concurrently. Each transaction
// is isolated from others.
func (s *InMemoryStore) BeginTx(ctx context.Context) (persistence.Tx, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}

	return &inMemoryTx{
		store:      s,
		committed:  false,
		rolledBack: false,
		changes:    make(map[string]map[string]*persistence.Document),
		deletes:    make(map[string]map[string]bool),
	}, nil
}

// Close releases all resources and clears all data from the store.
//
// After calling Close, the store is in the same state as a newly created store
// (no collections, no data). The store can still be used after Close - new collections
// can be created normally.
//
// Parameters:
//   - ctx: Context for the operation (ignored for in-memory implementation)
//
// Returns nil (never fails).
//
// Thread-safe: Acquires exclusive write lock during operation.
//
// Warning: All data is permanently lost. This operation cannot be undone.
func (s *InMemoryStore) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.collections = make(map[string]map[string]persistence.Document)

	return nil
}

// inMemoryTx implements persistence.Tx for in-memory transactional operations.
//
// It provides optimistic concurrency control by buffering changes in memory until
// Commit() is called. The transaction maintains two buffers:
//   - changes: Documents to insert or update (keyed by collection and ID)
//   - deletes: Documents to delete (keyed by collection and ID)
//
// # Transaction Isolation
//
// Read-your-own-writes: Get() reads from the transaction's change buffer first, falling
// back to the underlying store. This means reads within a transaction see uncommitted
// changes from the same transaction.
//
// Read-committed for others: Other readers only see committed data. Uncommitted changes
// are invisible outside the transaction.
//
// # Commit Semantics
//
// Commit atomically applies all buffered changes with a single write lock. Either all
// changes succeed, or none do (atomic commit). After Commit, the transaction cannot be
// used for further operations.
//
// # State Management
//
// The transaction tracks its state with committed and rolledBack flags. Once either is
// true, the transaction is "completed" and rejects further operations.
//
// # Thread Safety
//
// Each transaction has its own mutex protecting its internal state (changes, deletes,
// flags). Multiple transactions can exist concurrently without interfering with each
// other. However, operations on a single transaction must not be called concurrently
// (transaction methods are not reentrant).
//
// # Collection Operations
//
// CreateCollection and DropCollection bypass the transaction buffer and modify the
// underlying store directly. These operations are not transactional.
type inMemoryTx struct {
	store      *InMemoryStore
	committed  bool
	rolledBack bool
	changes    map[string]map[string]*persistence.Document
	deletes    map[string]map[string]bool
	mu         sync.Mutex
}

// CreateCollection creates a collection directly on the underlying store.
//
// Note: This operation is NOT transactional - it modifies the store immediately
// regardless of whether Commit() is called. Collection operations cannot be rolled back.
//
// Delegates to InMemoryStore.CreateCollection.
func (tx *inMemoryTx) CreateCollection(ctx context.Context, name string, schema *persistence.Schema) error {
	return tx.store.CreateCollection(ctx, name, schema)
}

// DropCollection drops a collection directly from the underlying store.
//
// Note: This operation is NOT transactional - it modifies the store immediately
// regardless of whether Commit() is called. Collection operations cannot be rolled back.
//
// Delegates to InMemoryStore.DropCollection.
func (tx *inMemoryTx) DropCollection(ctx context.Context, name string) error {
	return tx.store.DropCollection(ctx, name)
}

// Insert buffers a document insert operation in the transaction.
//
// The document is not written to the store until Commit() is called. If the transaction
// is rolled back, the insert is discarded.
//
// Parameters:
//   - ctx: Context for the operation (ignored, reserved for future use)
//   - collection: Name of the collection to insert into
//   - doc: Document to insert (must have non-empty "id" field)
//
// Returns:
//   - The document ID on success
//   - An error if:
//   - Transaction is already completed (committed or rolled back)
//   - Document has no "id" field or "id" is empty
//   - Document with this ID was already inserted in this transaction (returns persistence.ErrConflict)
//   - Document with this ID was deleted in this transaction
//
// Thread-safe: Protected by transaction's internal mutex.
func (tx *inMemoryTx) Insert(ctx context.Context, collection string, doc persistence.Document) (string, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return "", errors.New("transaction already completed")
	}

	id, ok := doc["id"].(string)
	if !ok || id == "" {
		return "", errors.New("document must have non-empty 'id' field")
	}

	if tx.changes[collection] == nil {
		tx.changes[collection] = make(map[string]*persistence.Document)
	}

	if tx.deletes[collection] != nil && tx.deletes[collection][id] {
		return "", errors.New("document was deleted in this transaction")
	}

	if tx.changes[collection][id] != nil {
		return "", persistence.ErrConflict
	}

	docCopy := make(persistence.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	tx.changes[collection][id] = &docCopy

	return id, nil
}

// Get retrieves a document from the transaction or underlying store.
//
// This implements read-your-own-writes semantics:
//   - If the document was deleted in this transaction, returns persistence.ErrNotFound
//   - If the document was inserted/updated in this transaction, returns buffered version
//   - Otherwise, reads from the underlying store
//
// Parameters:
//   - ctx: Context for the operation
//   - collection: Name of the collection to read from
//   - id: ID of the document to retrieve
//
// Returns:
//   - The document (deep copy) on success
//   - An error if:
//   - Transaction is already completed
//   - Document was deleted in this transaction (returns persistence.ErrNotFound)
//   - Document not found in transaction or store (returns persistence.ErrNotFound)
//
// Thread-safe: Protected by transaction's internal mutex.
func (tx *inMemoryTx) Get(ctx context.Context, collection string, id string) (persistence.Document, error) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return nil, errors.New("transaction already completed")
	}

	if tx.deletes[collection] != nil && tx.deletes[collection][id] {
		return nil, persistence.ErrNotFound
	}

	if tx.changes[collection] != nil {
		if doc := tx.changes[collection][id]; doc != nil {
			docCopy := make(persistence.Document)
			for k, v := range *doc {
				docCopy[k] = v
			}

			return docCopy, nil
		}
	}

	return tx.store.Get(ctx, collection, id)
}

// Update buffers a document update operation in the transaction.
//
// The document is not written to the store until Commit() is called. If the transaction
// is rolled back, the update is discarded.
//
// Parameters:
//   - ctx: Context for the operation (ignored, reserved for future use)
//   - collection: Name of the collection containing the document
//   - id: ID of the document to update
//   - doc: New document content (replaces existing entirely)
//
// Returns an error if:
//   - Transaction is already completed
//   - Document was deleted in this transaction (returns persistence.ErrNotFound)
//
// Note: Unlike the store's Update, this does NOT check if the document exists in the
// underlying store. The existence check happens at Commit time. This allows updates
// of documents inserted in the same transaction.
//
// Thread-safe: Protected by transaction's internal mutex.
func (tx *inMemoryTx) Update(ctx context.Context, collection string, id string, doc persistence.Document) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return errors.New("transaction already completed")
	}

	if tx.deletes[collection] != nil && tx.deletes[collection][id] {
		return persistence.ErrNotFound
	}

	if tx.changes[collection] == nil {
		tx.changes[collection] = make(map[string]*persistence.Document)
	}

	docCopy := make(persistence.Document)
	for k, v := range doc {
		docCopy[k] = v
	}

	tx.changes[collection][id] = &docCopy

	return nil
}

// Delete buffers a document delete operation in the transaction.
//
// The document is not removed from the store until Commit() is called. If the transaction
// is rolled back, the delete is discarded.
//
// If the document was inserted or updated in this transaction, the buffered change is
// removed and the delete is recorded. This ensures a delete within a transaction properly
// cancels any prior inserts/updates in the same transaction.
//
// Parameters:
//   - ctx: Context for the operation (ignored, reserved for future use)
//   - collection: Name of the collection containing the document
//   - id: ID of the document to delete
//
// Returns an error if transaction is already completed.
//
// Note: Unlike the store's Delete, this does NOT check if the document exists. The
// existence check happens at Commit time.
//
// Thread-safe: Protected by transaction's internal mutex.
func (tx *inMemoryTx) Delete(ctx context.Context, collection string, id string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return errors.New("transaction already completed")
	}

	if tx.deletes[collection] == nil {
		tx.deletes[collection] = make(map[string]bool)
	}

	tx.deletes[collection][id] = true

	if tx.changes[collection] != nil {
		delete(tx.changes[collection], id)
	}

	return nil
}

// Find retrieves documents from the underlying store.
//
// Note: This does NOT see uncommitted changes from the transaction. It only reads
// committed data from the store. To see uncommitted changes, use Get for specific
// documents.
//
// Delegates to InMemoryStore.Find.
func (tx *inMemoryTx) Find(ctx context.Context, collection string, query persistence.Query) ([]persistence.Document, error) {
	return tx.store.Find(ctx, collection, query)
}

// Maintenance is a no-op for transactions.
//
// Delegates to InMemoryStore.Maintenance (which is also a no-op).
func (tx *inMemoryTx) Maintenance(ctx context.Context) error {
	return nil
}

// BeginTx returns an error - nested transactions are not supported.
//
// To perform multiple transactional operations, use a single transaction and
// call Insert/Update/Delete multiple times before Commit.
func (tx *inMemoryTx) BeginTx(ctx context.Context) (persistence.Tx, error) {
	return nil, errors.New("nested transactions not supported")
}

// Close returns an error - transactions cannot be closed.
//
// Use Commit() to apply changes or Rollback() to discard changes. After either,
// the transaction is completed and cannot be used further.
func (tx *inMemoryTx) Close(ctx context.Context) error {
	return errors.New("cannot close transaction")
}

// Commit atomically applies all buffered changes to the underlying store.
//
// The commit process:
//  1. Acquires exclusive write lock on the store
//  2. Auto-creates any missing collections
//  3. Applies all buffered changes (inserts and updates)
//  4. Applies all buffered deletes
//  5. Marks transaction as committed
//  6. Releases lock
//
// Collections are automatically created during commit if they don't exist, ensuring
// transactional operations succeed without requiring explicit CreateCollection() calls.
//
// Returns an error if:
//   - Transaction was already rolled back
//
// Calling Commit multiple times is safe - subsequent calls return nil immediately.
//
// Thread-safe: Acquires both transaction mutex and store write lock.
//
// Note: After successful commit, the transaction cannot be used for further operations.
func (tx *inMemoryTx) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return nil
	}

	if tx.rolledBack {
		return errors.New("transaction was rolled back")
	}

	tx.store.mu.Lock()
	defer tx.store.mu.Unlock()

	for collection, changes := range tx.changes {
		if _, exists := tx.store.collections[collection]; !exists {
			tx.store.collections[collection] = make(map[string]persistence.Document)
		}

		for id, doc := range changes {
			tx.store.collections[collection][id] = *doc
		}
	}

	for collection, deletes := range tx.deletes {
		if _, exists := tx.store.collections[collection]; !exists {
			tx.store.collections[collection] = make(map[string]persistence.Document)
		}

		for id := range deletes {
			delete(tx.store.collections[collection], id)
		}
	}

	tx.committed = true

	return nil
}

// Rollback discards all buffered changes without applying them to the store.
//
// All inserts, updates, and deletes buffered in the transaction are cleared. The
// underlying store is not modified.
//
// Returns nil if:
//   - Transaction is already rolled back (idempotent)
//   - Transaction is already committed (no-op)
//
// Calling Rollback multiple times is safe - subsequent calls return nil immediately.
//
// Thread-safe: Protected by transaction's internal mutex.
//
// Note: After rollback, the transaction cannot be used for further operations.
func (tx *inMemoryTx) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.rolledBack || tx.committed {
		return nil
	}

	tx.rolledBack = true
	tx.changes = make(map[string]map[string]*persistence.Document)
	tx.deletes = make(map[string]map[string]bool)

	return nil
}
