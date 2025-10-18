// Package basic provides a database-agnostic persistence layer (Layer 1).
//
// DESIGN DECISION: Three-layer architecture separating concerns
// WHY: Enable testing without database, support multiple backends (SQLite, Postgres, MongoDB),
// and provide type-safe high-level APIs without coupling to storage implementation.
//
// TRADE-OFF: Additional abstraction layers increase code complexity, but reduce coupling
// and make the system more maintainable and testable.
//
// INSPIRED BY: MongoDB driver API, Linear's metadata-driven schema system,
// CSE (Common Sync Engine) pattern from reverse-engineering Linear's sync approach.
//
// Architecture:
//   - Layer 1 (basic): Database-agnostic collection/document API (this package)
//   - Layer 2 (cse): CSE conventions - metadata fields, sync tracking
//   - Layer 3 (repo): Type-safe domain models (Asset, DataPoint, etc.)
package basic

import (
	"context"
)

// Document represents a JSON-serializable document stored in a collection.
//
// DESIGN DECISION: Use map[string]interface{} instead of custom struct types
// WHY: Maximum flexibility - any JSON-serializable value works. Different backends
// can store this efficiently: JSON column (SQLite), JSONB (Postgres), or native (MongoDB).
// This mirrors how Linear stores flexible metadata alongside typed fields.
//
// TRADE-OFF: Runtime type errors instead of compile-time safety. Layer 3 (repo)
// provides type-safe wrappers around this flexible representation.
//
// INSPIRED BY: MongoDB's document model, Linear's flexible data structures,
// bson.M pattern from MongoDB Go driver.
//
// Example:
//
//	doc := basic.Document{
//	    "id": "asset-123",
//	    "name": "Press Machine A",
//	    "tags": []string{"production", "critical"},
//	    "metadata": map[string]interface{}{
//	        "lastMaintenance": time.Now(),
//	    },
//	}
type Document map[string]interface{}

// Schema defines the structure and validation rules for a collection.
//
// DESIGN DECISION: Schema is optional and validation-focused
// WHY: Support both schema-less (MongoDB-style) and schema-enforced (SQL) backends.
// Schema can be nil for backends that don't require upfront schema definition.
//
// TRADE-OFF: Optional validation at Layer 1. Layer 2 (CSE) enforces metadata
// conventions (syncedAt, syncStatus, version), but Layer 1 remains flexible.
//
// INSPIRED BY: MongoDB's validator concept, JSON Schema standard.
//
// Note: Currently a placeholder. Will be expanded to support:
//   - Field type definitions (string, number, boolean, etc.)
//   - Validation rules (required, unique, range, pattern)
//   - Index definitions for query performance
type Schema struct {
	// Fields will be added as needed
	// Example: Fields map[string]FieldType
	// Example: Indexes []Index
	// Example: Validators []ValidationRule
}

// Query represents filtering, sorting, and pagination criteria for finding documents.
//
// DESIGN DECISION: Database-agnostic query representation
// WHY: Abstract SQL WHERE clauses and NoSQL queries into a common format.
// Backends translate this to their native query language (SQL, MongoDB query language).
//
// TRADE-OFF: Limited to common query patterns (equality, comparison, logical operators).
// Complex SQL JOINs or aggregations require backend-specific extensions.
//
// INSPIRED BY: MongoDB query syntax, ORM query builders (GORM, SQLAlchemy).
//
// Note: Currently a placeholder. Will be expanded to support:
//   - Filter: map[string]interface{} for field conditions
//   - Sort: []SortField for ordering
//   - Limit/Skip: for pagination
//   - Projection: []string for selecting specific fields
type Query struct {
	// Fields will be added as needed
	// Example: Filter map[string]interface{}
	// Example: Sort []SortField
	// Example: Limit int
	// Example: Skip int
}

// Store provides database-agnostic CRUD operations on collections of documents.
//
// DESIGN DECISION: Collection-based API instead of table-based SQL API
// WHY: Abstracts the difference between SQL tables and NoSQL collections.
// Collections map to tables in SQL databases or collections in NoSQL databases.
// This enables switching backends without changing application code.
//
// TRADE-OFF: Cannot use SQL-specific features (JOINs, complex queries, triggers)
// directly. These must be implemented in backend-specific extensions or Layer 3.
//
// INSPIRED BY: MongoDB Collection API, Linear's entity storage abstraction.
//
// Concurrency: All methods are safe for concurrent use. Implementations must
// handle synchronization internally.
//
// Error Handling: Methods return errors for:
//   - context.Canceled / context.DeadlineExceeded: operation was cancelled
//   - ErrNotFound: document or collection doesn't exist
//   - ErrConflict: version mismatch, unique constraint violation
//   - Backend-specific errors: connection failures, query syntax errors
//
// Example usage:
//
//	store, _ := sqlite.NewStore("./data.db")
//	defer store.Close()
//
//	// Create collection
//	store.CreateCollection(ctx, "assets", nil)
//
//	// Insert document
//	doc := basic.Document{"name": "Machine A", "status": "active"}
//	id, _ := store.Insert(ctx, "assets", doc)
//
//	// Query documents
//	results, _ := store.Find(ctx, "assets", basic.Query{})
type Store interface {
	// CreateCollection creates a new collection with an optional schema.
	//
	// DESIGN DECISION: Schema parameter is optional (can be nil)
	// WHY: Support both schema-less backends (MongoDB) and schema-required backends (SQL).
	// For SQL databases, schema defines table structure (columns, types, constraints).
	// For NoSQL databases, schema may define validation rules or be ignored entirely.
	//
	// TRADE-OFF: Schema enforcement varies by backend. SQLite requires schema upfront,
	// MongoDB can validate optionally, but Layer 1 doesn't mandate it.
	//
	// INSPIRED BY: MongoDB's createCollection with validator option,
	// SQL CREATE TABLE with schema definition.
	//
	// Parameters:
	//   - ctx: cancellation context
	//   - name: collection name (maps to table name in SQL)
	//   - schema: optional structure definition (nil for schema-less)
	//
	// Returns:
	//   - error: if collection already exists or schema is invalid
	CreateCollection(ctx context.Context, name string, schema *Schema) error

	// DropCollection removes a collection and all its documents.
	//
	// DESIGN DECISION: Irreversible operation without confirmation
	// WHY: Mirror SQL DROP TABLE and MongoDB dropCollection behavior.
	// Caller is responsible for confirmation/backup before calling.
	//
	// TRADE-OFF: Potential data loss if called accidentally. Layer 3 should
	// implement confirmation prompts for user-facing operations.
	//
	// Parameters:
	//   - ctx: cancellation context
	//   - name: collection name to drop
	//
	// Returns:
	//   - error: if collection doesn't exist or operation fails
	DropCollection(ctx context.Context, name string) error

	// Insert adds a document to a collection and returns its unique ID.
	//
	// DESIGN DECISION: Auto-generate ID on server side, not client side
	// WHY: Prevent ID collisions, ensure uniqueness across distributed systems.
	// Backends generate IDs using their native mechanisms (SQLite AUTOINCREMENT,
	// MongoDB ObjectId, UUID).
	//
	// TRADE-OFF: Client doesn't control IDs. If client needs specific IDs,
	// store them as regular fields and use Update instead.
	//
	// INSPIRED BY: MongoDB's insertOne returning insertedId, Linear's server-generated IDs.
	//
	// Parameters:
	//   - ctx: cancellation context
	//   - collection: collection name
	//   - doc: document to insert
	//
	// Returns:
	//   - id: unique identifier for the inserted document
	//   - error: if validation fails or insertion fails
	Insert(ctx context.Context, collection string, doc Document) (id string, err error)

	// Get retrieves a document by its unique ID.
	//
	// DESIGN DECISION: Return ErrNotFound instead of (nil, nil)
	// WHY: Explicit error handling - caller knows whether document exists.
	// Go convention: (value, nil) means success, (zero, error) means failure.
	//
	// TRADE-OFF: Caller must handle ErrNotFound explicitly. Alternative would be
	// (nil, nil) for "not found" but that's ambiguous with empty document.
	//
	// INSPIRED BY: MongoDB's FindOne, GORM's First method.
	//
	// Parameters:
	//   - ctx: cancellation context
	//   - collection: collection name
	//   - id: document identifier
	//
	// Returns:
	//   - Document: the found document
	//   - error: ErrNotFound if document doesn't exist
	Get(ctx context.Context, collection string, id string) (Document, error)

	// Update replaces a document entirely by its ID.
	//
	// DESIGN DECISION: Full replacement, not partial update
	// WHY: Simple and unambiguous - entire document is replaced.
	// Partial updates are complex (nested field updates, array operations)
	// and can be added later as UpdatePartial if needed.
	//
	// TRADE-OFF: Caller must read-modify-write for partial updates, which has
	// race conditions without transactions. Layer 2 (CSE) handles versioning
	// to detect conflicts.
	//
	// INSPIRED BY: MongoDB's replaceOne, Linear's optimistic locking pattern.
	//
	// Parameters:
	//   - ctx: cancellation context
	//   - collection: collection name
	//   - id: document identifier
	//   - doc: new document content (replaces existing)
	//
	// Returns:
	//   - error: ErrNotFound if document doesn't exist, ErrConflict if version mismatch
	Update(ctx context.Context, collection string, id string, doc Document) error

	// Delete removes a document by its ID.
	//
	// DESIGN DECISION: Delete by ID, not by query
	// WHY: Explicit and safe - caller knows exactly what's being deleted.
	// Bulk deletes can be dangerous and should be explicit operations.
	//
	// TRADE-OFF: Cannot delete multiple documents in one call. DeleteMany
	// can be added if needed, but single-delete is safer default.
	//
	// Parameters:
	//   - ctx: cancellation context
	//   - collection: collection name
	//   - id: document identifier
	//
	// Returns:
	//   - error: ErrNotFound if document doesn't exist
	Delete(ctx context.Context, collection string, id string) error

	// Find queries documents in a collection with optional filtering, sorting, and pagination.
	//
	// DESIGN DECISION: Return all results in-memory, not cursor/iterator
	// WHY: Simple API for common use cases (fetching 10-1000 documents).
	// Most FSM queries are small result sets (current assets, recent data points).
	//
	// TRADE-OFF: Cannot handle millions of results efficiently. For large datasets,
	// backend-specific cursors or streaming APIs would be needed.
	//
	// INSPIRED BY: MongoDB's find().toArray(), GORM's Find method.
	//
	// Parameters:
	//   - ctx: cancellation context
	//   - collection: collection name
	//   - query: filtering/sorting/pagination criteria (empty Query returns all)
	//
	// Returns:
	//   - []Document: matching documents (empty slice if none match)
	//   - error: if query is invalid or execution fails
	Find(ctx context.Context, collection string, query Query) ([]Document, error)

	// BeginTx starts a transaction for atomic multi-document operations.
	//
	// DESIGN DECISION: Explicit transactions, not implicit/auto-commit
	// WHY: Make atomicity requirements explicit in code. Caller controls
	// transaction boundaries and can group related operations.
	//
	// TRADE-OFF: Requires explicit Commit/Rollback handling. Forgot to commit?
	// Changes are lost. Layer 3 should use defer patterns to prevent leaks.
	//
	// INSPIRED BY: SQL BEGIN TRANSACTION, MongoDB sessions with transactions.
	//
	// Usage pattern:
	//
	//	tx, err := store.BeginTx(ctx)
	//	if err != nil {
	//	    return err
	//	}
	//	defer tx.Rollback() // Safe to call after Commit
	//
	//	tx.Insert(ctx, "assets", doc1)
	//	tx.Insert(ctx, "datapoints", doc2)
	//
	//	return tx.Commit()
	//
	// Parameters:
	//   - ctx: cancellation context
	//
	// Returns:
	//   - Tx: transaction handle (also implements Store interface)
	//   - error: if transaction cannot be started
	BeginTx(ctx context.Context) (Tx, error)

	// Close closes the store and releases resources (connections, files).
	//
	// DESIGN DECISION: Explicit cleanup, not finalization/GC
	// WHY: Deterministic resource release. Database connections, file handles
	// should be closed promptly, not wait for garbage collection.
	//
	// TRADE-OFF: Caller must remember to call Close. Use defer patterns to ensure cleanup.
	//
	// Parameters: none
	//
	// Returns:
	//   - error: if cleanup fails (usually safe to ignore)
	Close() error
}

// Tx represents a database transaction for atomic multi-document operations.
//
// DESIGN DECISION: Tx embeds Store interface
// WHY: All Store operations work within transaction using same API.
// No need for separate TxInsert, TxUpdate methods - just use tx.Insert, tx.Update.
//
// TRADE-OFF: Tx methods operate on transaction context, not global store.
// Caller must use tx instance, not original store, for transactional operations.
//
// INSPIRED BY: sql.Tx from database/sql package, MongoDB ClientSession.
//
// Lifecycle:
//  1. BeginTx() creates transaction
//  2. Use tx.Insert, tx.Update, etc. for operations
//  3. Commit() makes changes permanent, or Rollback() discards changes
//  4. After Commit/Rollback, tx cannot be reused
//
// Example:
//
//	tx, _ := store.BeginTx(ctx)
//	defer tx.Rollback() // No-op if Commit succeeds
//
//	tx.Insert(ctx, "assets", doc1)
//	tx.Delete(ctx, "datapoints", "old-id")
//
//	if err := tx.Commit(); err != nil {
//	    // Rollback already called by defer
//	    return err
//	}
type Tx interface {
	Store

	// Commit makes all transaction changes permanent.
	//
	// DESIGN DECISION: Commit invalidates transaction
	// WHY: Prevent accidental reuse of committed transaction.
	// SQL semantics: transaction ends after COMMIT.
	//
	// TRADE-OFF: Cannot continue using tx after Commit. Must call BeginTx again.
	//
	// Returns:
	//   - error: if commit fails (e.g., constraint violation, deadlock)
	Commit() error

	// Rollback discards all transaction changes.
	//
	// DESIGN DECISION: Rollback is idempotent and safe to call multiple times
	// WHY: Enable defer tx.Rollback() pattern without checking if Commit succeeded.
	// Calling Rollback after Commit is a no-op.
	//
	// TRADE-OFF: Silent no-op if already committed/rolled back. Caller must track
	// transaction state if they need to know whether rollback actually happened.
	//
	// Returns:
	//   - error: if rollback fails (rare, usually safe to ignore)
	Rollback() error
}

// Common errors returned by Store implementations.
// These can be checked using errors.Is(err, basic.ErrNotFound).

// ErrNotFound indicates a document or collection was not found.
//
// DESIGN DECISION: Sentinel error, not custom error type
// WHY: Simple and works with errors.Is for checking.
// Custom error type would allow attaching collection/id info but adds complexity.
//
// TRADE-OFF: Error message must include context (collection name, id) as string.
// Cannot programmatically extract collection/id from error.
//
// INSPIRED BY: sql.ErrNoRows, gorm.ErrRecordNotFound.
var ErrNotFound = &storeError{msg: "document not found"}

// ErrConflict indicates a version mismatch or constraint violation.
//
// DESIGN DECISION: Single error for both version conflicts and constraint violations
// WHY: Both represent "cannot complete operation due to data conflict".
// Layer 2 (CSE) uses version fields to detect concurrent modifications.
//
// TRADE-OFF: Cannot distinguish version conflict from unique constraint violation
// without checking error message. Could split into separate errors if needed.
//
// INSPIRED BY: HTTP 409 Conflict, Optimistic Locking pattern.
var ErrConflict = &storeError{msg: "document conflict"}

// storeError implements error interface for basic package errors.
type storeError struct {
	msg string
}

func (e *storeError) Error() string {
	return e.msg
}
