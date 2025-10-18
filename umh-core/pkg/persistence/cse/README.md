# CSE (Control Sync Engine) Package

Package `cse` provides CSE conventions for FSM v2 persistence layer.

## What is CSE?

CSE (Control Sync Engine) is a three-tier sync system inspired by Linear's sync engine:
- **Frontend ↔ Relay ↔ Edge**: Three-tier architecture with sync at each level
- **Sync State**: Each tier maintains `_sync_id` and `_version` for tracking changes
- **Triangular Model**: Three collections per worker type:
  - **Identity**: Immutable worker identity (IP, hostname, bootstrap config)
  - **Desired**: User intent and configuration (what should happen)
  - **Observed**: System reality and current state (what is happening)
- **Optimistic UI with Pessimistic Storage**: Fast UI updates, safe database writes

## Package Structure

```
umh-core/pkg/persistence/
├── basic/              # Layer 1: Database operations (CRUD, transactions, queries)
│   ├── store.go
│   ├── query.go
│   ├── transaction.go
│   └── sqlite.go
└── cse/                # Layer 2: CSE conventions (this package)
    ├── registry.go          # Schema registry implementation
    ├── registry_test.go
    ├── triangular.go        # Triangular store (identity/desired/observed)
    ├── triangular_test.go
    ├── tx_cache.go          # Transaction cache for batching
    ├── tx_cache_test.go
    ├── sync_state.go        # Sync state tracking (_sync_id/_version)
    ├── sync_state_test.go
    ├── pool.go              # Object pool for singleton management
    ├── pool_test.go
    ├── example_test.go
    └── README.md
```

## Schema Registry

The Schema Registry tracks metadata about CSE-aware collections:
- Which collections follow CSE conventions?
- Which fields are CSE metadata (_sync_id, _version, etc.)?
- Which fields are indexed for sync queries?
- How do collections relate in the triangular model?

### Core Types

```go
// CSE metadata field constants
const (
    FieldSyncID    = "_sync_id"     // Global sync version
    FieldVersion   = "_version"     // Document version (optimistic locking)
    FieldCreatedAt = "_created_at"  // Creation timestamp
    FieldUpdatedAt = "_updated_at"  // Last update timestamp
    FieldDeletedAt = "_deleted_at"  // Soft delete timestamp (optional)
    FieldDeletedBy = "_deleted_by"  // User who deleted (optional)
)

// Triangular model roles
const (
    RoleIdentity  = "identity"   // Immutable worker identity
    RoleDesired   = "desired"    // User intent / configuration
    RoleObserved  = "observed"   // System reality / current state
)

// CollectionMetadata describes a CSE-aware collection
type CollectionMetadata struct {
    Name           string   // Collection name (e.g., "container_identity")
    WorkerType     string   // FSM worker type (e.g., "container")
    Role           string   // "identity", "desired", or "observed"
    CSEFields      []string // CSE metadata fields
    IndexedFields  []string // Fields with indexes
    RelatedTo      []string // Related collection names
}

// Registry tracks all CSE-aware collections
type Registry struct {
    // Thread-safe with RWMutex
}
```

### Quick Start

```go
import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"

// Register collections at application startup
cse.Register(&cse.CollectionMetadata{
    Name:          "container_identity",
    WorkerType:    "container",
    Role:          cse.RoleIdentity,
    CSEFields:     []string{cse.FieldSyncID, cse.FieldVersion},
    IndexedFields: []string{cse.FieldSyncID},
})

// Later: Look up metadata
metadata, err := cse.Get("container_identity")
if err != nil {
    log.Fatal(err)
}

// Use metadata to build SQL schema
for _, field := range metadata.CSEFields {
    // Add CSE columns to CREATE TABLE statement
}
```

### API Overview

**Global Registry Functions** (convenience wrappers):
- `Register(metadata)` - Register a collection
- `Get(name)` - Retrieve collection metadata
- `IsRegistered(name)` - Check if collection exists
- `List()` - Get all registered collections
- `GetTriangularCollections(workerType)` - Get identity/desired/observed

**Registry Instance Methods** (for testing/advanced use):
- `NewRegistry()` - Create separate registry instance
- Same methods as global functions, but on instance

### Complete Example

```go
package main

import (
    "log"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"
)

func main() {
    // Register triangular model for "container" worker type
    registerContainerCollections()

    // Retrieve all three collections
    identity, desired, observed, err := cse.GetTriangularCollections("container")
    if err != nil {
        log.Fatalf("Incomplete triangular model: %v", err)
    }

    // Use metadata to create SQL schema
    createTable(identity)
    createTable(desired)
    createTable(observed)
}

func registerContainerCollections() {
    collections := []*cse.CollectionMetadata{
        {
            Name:       "container_identity",
            WorkerType: "container",
            Role:       cse.RoleIdentity,
            CSEFields:  []string{
                cse.FieldSyncID,
                cse.FieldVersion,
                cse.FieldCreatedAt,
            },
            IndexedFields: []string{cse.FieldSyncID},
            RelatedTo:     []string{"container_desired", "container_observed"},
        },
        {
            Name:       "container_desired",
            WorkerType: "container",
            Role:       cse.RoleDesired,
            CSEFields:  []string{
                cse.FieldSyncID,
                cse.FieldVersion,
                cse.FieldCreatedAt,
                cse.FieldUpdatedAt,
            },
            IndexedFields: []string{cse.FieldSyncID},
            RelatedTo:     []string{"container_identity", "container_observed"},
        },
        {
            Name:       "container_observed",
            WorkerType: "container",
            Role:       cse.RoleObserved,
            CSEFields:  []string{
                cse.FieldSyncID,
                cse.FieldVersion,
                cse.FieldCreatedAt,
                cse.FieldUpdatedAt,
            },
            IndexedFields: []string{cse.FieldSyncID},
            RelatedTo:     []string{"container_identity", "container_desired"},
        },
    }

    for _, metadata := range collections {
        if err := cse.Register(metadata); err != nil {
            log.Fatalf("Failed to register %s: %v", metadata.Name, err)
        }
    }
}

func createTable(metadata *cse.CollectionMetadata) {
    // Example: Use metadata to build CREATE TABLE statement
    log.Printf("Creating table %s with CSE fields: %v",
        metadata.Name, metadata.CSEFields)

    // TODO: Build SQL schema using Layer 1 (basic package)
}
```

## Object Pool

The Object Pool provides singleton management for FSM workers, sync state, and shared resources:
- **Identity Map Pattern**: Same key always returns same object reference
- **Reference Counting**: Multiple consumers can safely share objects
- **Automatic Cleanup**: Closeable objects are closed when removed
- **Thread-Safe**: Concurrent access with `sync.RWMutex`

### Core Types

```go
// Closeable represents objects that need cleanup
type Closeable interface {
    Close() error
}

// Factory creates new instances of objects
type Factory func() (interface{}, error)

// ObjectPool manages singleton object instances
type ObjectPool struct {
    // Thread-safe with RWMutex
}
```

### Quick Start

```go
import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"

// Create pool
pool := cse.NewObjectPool()

// Store object (initializes ref count to 1)
pool.Put("container-123", worker)

// Retrieve object
obj, found := pool.Get("container-123")
if found {
    worker := obj.(*ContainerWorker)
}

// Get or create with factory
obj, err := pool.GetOrCreate("container-123", func() (interface{}, error) {
    return NewContainerWorker("container-123", config)
})

// Acquire reference (increment ref count)
pool.Acquire("container-123")

// Release reference (decrement ref count, remove if zero)
pool.Release("container-123")

// Cleanup (closes Closeable objects)
pool.Clear()
```

### API Overview

**Basic Operations**:
- `NewObjectPool()` - Create new pool
- `Put(key, obj)` - Store object (ref count = 1)
- `Get(key)` - Retrieve object
- `Has(key)` - Check if object exists
- `Size()` - Get pool size
- `Remove(key)` - Remove and close object
- `Clear()` - Remove all objects

**Factory Pattern**:
- `GetOrCreate(key, factory)` - Get existing or create new (ref count = 1)

**Reference Counting**:
- `Acquire(key)` - Increment ref count
- `Release(key)` - Decrement ref count (remove if zero)

### Complete Example

```go
package main

import (
    "log"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"
)

type ContainerWorker struct {
    id     string
    config Config
}

func (w *ContainerWorker) Close() error {
    log.Printf("Closing worker %s", w.id)
    return nil
}

func main() {
    pool := cse.NewObjectPool()

    // FSM creates worker on first use
    worker1, err := pool.GetOrCreate("container-123", func() (interface{}, error) {
        return &ContainerWorker{id: "container-123"}, nil
    })
    if err != nil {
        log.Fatal(err)
    }

    // API also needs this worker (shares same instance)
    pool.Acquire("container-123")
    worker2, _ := pool.Get("container-123")

    // Both point to same object (identity map)
    if worker1 != worker2 {
        log.Fatal("Should be same instance!")
    }

    // API done with worker
    pool.Release("container-123")

    // FSM done with worker (ref count = 0, worker closed and removed)
    pool.Release("container-123")
}
```

## Design Decisions

### 1. Metadata-Driven Registration (Not Code Generation)

**Decision**: Manual registration via `Register()` function, not automatic code generation.

**Why**: Simplicity for MVP. No build-time code generation needed, just runtime registration.

**Trade-off**: Must manually register collections (could forget), but explicit and clear.

**Inspired by**: Linear's decorator pattern (adapted for Go without decorators).

**Future**: Could add code generation from struct tags if needed.

### 2. In-Memory Registry (Not Database-Backed)

**Decision**: Registry stored in memory, populated at startup.

**Why**: Schema is static (defined at startup), doesn't change at runtime.

**Trade-off**: Must re-register on restart, but acceptable (fast startup, ~milliseconds).

**Inspired by**: HTTP router registration patterns (gin, echo, chi).

### 3. Thread-Safe with RWMutex

**Decision**: Concurrent access allowed via `sync.RWMutex`.

**Why**: Registry accessed from multiple FSM workers simultaneously.

**Trade-off**: Lock overhead, but reads are fast (`RLock`) and writes are rare (startup only).

**Inspired by**: `sync.Map` pattern, but simpler with `map + mutex`.

### 4. Triangular Model Grouping

**Decision**: `GetTriangularCollections()` helper returns all three collections.

**Why**: Common pattern for FSM (always need identity + desired + observed together).

**Trade-off**: Assumes naming convention (`workerType_role`), but this is enforced.

**Inspired by**: FSM v2 triangular model architecture.

### 5. Global Registry Pattern

**Decision**: Package-level global registry with convenience functions.

**Why**: Most applications only need one registry, simplifies common case.

**Trade-off**: Global state, but acceptable (registry is read-only after startup).

**Inspired by**: Go's `http.DefaultServeMux` pattern.

### 6. Fail-Fast Validation

**Decision**: Validate metadata at registration time (startup), not runtime.

**Why**: Catch configuration errors early (at startup), not during sync operations.

**Trade-off**: More upfront validation code, but prevents runtime errors.

**Inspired by**: "Parse, don't validate" principle - ensure valid state.

### 7. Identity Map Pattern (Object Pool)

**Decision**: Same key always returns same object reference, not new instances.

**Why**: Prevents duplicate FSM workers for same container ID (memory efficiency, consistency).

**Trade-off**: Must manually remove objects when done, but ensures identity consistency.

**Inspired by**: Linear's object pool, JPA entity manager.

**Benefit**: Only one Container FSM instance per worker ID exists in memory.

### 8. Reference Counting (Not Weak References)

**Decision**: Acquire/Release pattern instead of immediate removal or weak references.

**Why**: Multiple consumers may use same object (FSM + API both need worker reference).

**Trade-off**: Requires careful acquire/release pairing, but Go doesn't support weak references natively.

**Inspired by**: C++ `shared_ptr`, Objective-C retain/release.

**Alternative**: Could use finalizers, but unpredictable timing (GC-dependent).

### 9. Closeable Interface (Not defer-based cleanup)

**Decision**: Pool closes objects on removal via `Closeable` interface.

**Why**: FSM workers need cleanup (close DB connections, stop goroutines).

**Trade-off**: Objects must implement `Close()` if cleanup needed, but explicit and clear.

**Inspired by**: `io.Closer`, `database/sql` connection pools.

**Benefit**: Automatic cleanup when reference count reaches zero.

### 10. Factory Pattern for GetOrCreate

**Decision**: GetOrCreate takes factory function, not new() constructor.

**Why**: Flexible object creation (may need context, config, DB connection).

**Trade-off**: Caller must provide factory function each time, but enables dependency injection.

**Inspired by**: `sync.Pool`, Linear's object construction.

**Alternative**: Could use type parameter `[T any]`, but less flexible for initialization.

### 11. Thread-Safe with RWMutex (Object Pool)

**Decision**: Concurrent access allowed via `sync.RWMutex`.

**Why**: Pool accessed from multiple FSM workers simultaneously.

**Trade-off**: Lock overhead, but acceptable for infrequent access (workers cached long-term).

**Inspired by**: `sync.Map`, thread-safe collections.

**Optimization**: Read-heavy workload benefits from `RLock` (multiple readers).

## Testing

The package was developed using Test-Driven Development (TDD):

**Test Coverage**: 96.4% (421 lines implementation, 393 lines tests)

**Test Categories**:
- Unit tests (`TestNewRegistry`, `TestRegistry_Register`, etc.)
- Validation tests (`TestRegistry_Register_Validation`)
- Concurrency tests (`TestRegistry_ConcurrentAccess`)
- Integration tests (global registry functions)
- Example tests (runnable documentation)

**Run Tests**:
```bash
cd umh-core/pkg/persistence/cse
go test -v              # All tests
go test -cover          # With coverage
go test -run Example    # Just examples
```

## Future Enhancements

Planned for future layers:

1. **Layer 3**: Collection Builder (use registry metadata to create SQL tables)
2. **Layer 4**: Sync Engine (use registry metadata for sync queries)
3. **Code Generation**: Generate registration code from struct tags
4. **Validation**: Enforce CSE field presence in actual documents
5. **Migration**: Schema versioning and migration support

## Integration with Layer 1 (Basic Package)

The CSE package builds on top of the `basic` package:

```go
import (
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/cse"
)

// Register CSE metadata
cse.Register(&cse.CollectionMetadata{...})

// Use metadata with Layer 1 operations
metadata, _ := cse.Get("container_identity")

store := basic.NewStore(db)
store.CreateCollection(ctx, metadata.Name, schema)

// Add CSE fields to schema based on metadata.CSEFields
```

## References

- **Linear Sync Engine**: https://linear.app/docs/sync-engine
- **FSM v2 Architecture**: See `umh-core/docs/fsm-v2-architecture.md`
- **Layer 1 (Basic)**: See `umh-core/pkg/persistence/basic/README.md`
