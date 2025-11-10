# Type Registry + Delta Checking Implementation Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/subagent-driven-development/SKILL.md` to implement this plan task-by-task.

**Goal:** Add type registry to TriangularStore to enable efficient delta checking, reducing database writes from 100/sec to <1/sec

**Architecture:**
- Type registry stores reflect.Type metadata for each worker type at CSE layer
- Typed deserialization converts Document → struct using registered types
- Delta checking compares typed structs before write (1μs vs 100μs Document comparison)
- persistence/store stays unchanged (generic Documents only, no types)

**Tech Stack:** Go 1.23, reflect package, Ginkgo/Gomega testing

**Created:** 2025-11-10 14:30

**Last Updated:** 2025-11-10 14:30

---

## Overview

### Problem Statement

Currently, TriangularStore writes 100 times per second because it cannot detect when Documents haven't changed. Every SaveObserved() call writes to database regardless of whether data changed.

**Current flow (inefficient):**
```
Worker → SaveObserved(Document) → persistence.Save() → Database write (100/sec)
                                   ↑
                                   No delta checking possible (Documents not typed)
```

**Target flow (efficient):**
```
Worker → SaveObserved(Document) → Load previous typed struct → Compare structs (1μs)
                                   ↓                              ↓
                                   Different?                     Same?
                                   ↓                              ↓
                                   persistence.Save()             Skip write
                                   Database (0.1/sec)             (saves 99.9%)
```

### Success Criteria

- [ ] Database writes reduced from 100/sec to <1/sec in steady state
- [ ] Type registry supports concurrent registration (thread-safe)
- [ ] Struct comparison <10μs (100x faster than Document comparison)
- [ ] All Phase 0 integration tests pass
- [ ] Test coverage >85% for new code
- [ ] No race conditions (verified with -race flag)
- [ ] persistence/store remains unchanged (architectural constraint)

### Constraints

**CRITICAL: persistence/store MUST stay generic**

The persistence layer CANNOT know about types:
- Stores Documents only (map[string]interface{})
- No reflect.Type dependencies
- No worker-specific logic
- Remains reusable across projects

**Type registry lives at CSE layer (above persistence)**

### Performance Targets

| Operation | Current | Target | Improvement |
|-----------|---------|--------|-------------|
| Database writes/sec | 100 | 0.1 | 1000x reduction |
| Delta check time | N/A (skipped) | <10μs | N/A |
| Document comparison | ~100μs | N/A (replaced) | 10x faster |
| Type registry lookup | N/A | <1μs | Negligible overhead |

---

## TDD Workflow

**Every task follows strict RED-GREEN-REFACTOR:**

### RED Phase (Write Failing Test)
1. Write ONE test showing desired behavior
2. Run test with `go test -v`
3. **VERIFY it fails for RIGHT reason** (feature missing, not typo)
4. If fails for wrong reason (undefined function, import error), fix test and re-run

### GREEN Phase (Minimal Implementation)
1. Write SIMPLEST code to pass the test
2. No extra features, no "future-proofing"
3. Run test with `go test -v`
4. Verify test passes AND all other tests still pass

### REFACTOR Phase (Clean Up)
1. Improve names, extract helpers, remove duplication
2. Keep tests green (run after each change)
3. Don't add behavior during refactor

### Commit After Each Cycle
```bash
git add pkg/cse/storage/type_registry.go pkg/cse/storage/type_registry_test.go
git commit -m "test: add Test_TypeRegistry_RegisterWorkerType (RED)"
# Then implement, then:
git add pkg/cse/storage/type_registry.go
git commit -m "feat: implement TypeRegistry.RegisterWorkerType (GREEN)"
```

**NO production code without failing test first. Period.**

---

## Phase Breakdown

### Phase 1: Type Registry Foundation + Data Access Pattern Stubs (RED-GREEN-REFACTOR)

**Goal:** Extend existing registry.go to store reflect.Type metadata per worker type AND add extensibility stubs for future data access pattern support

**Architecture Decision:** Extend existing `Registry` instead of creating separate type registry
- **Why:** Single source of truth, reuses existing thread-safety (sync.RWMutex)
- **How:** Add `ObservedType` and `DesiredType` fields to `CollectionMetadata`
- **Alternative Rejected:** Separate `TypeRegistry` would create parallel state

**Implementation Strategy:**
- **Implement now** (for delta checking): `ObservedType`, `DesiredType` fields
- **Add as stubs** (for future CSE features): `Fields []FieldMetadata`, `TypeVersion` field, and supporting types

**Files:**
- Modify: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/registry.go`
- Modify: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/registry_test.go`

**New Types to Define (stubs for future):**
- `FieldMetadata` - Field-level metadata for data access patterns
- `AccessPattern` - Instant/Lazy/Partial/Explicit enum
- `Relationship` - 1:1 or 1:many relationship metadata
- `RelationType` - Enum for relationship types
- `SyncPolicy` - Cache and sync strategy metadata

#### Test 1: CollectionMetadata stores type metadata (4 steps)

**Step 1: Write failing test**

Add to existing `pkg/cse/storage/registry_test.go`:

```go
// Add test data structures at top
type ParentObservedState struct {
    Name   string
    Status string
}

type ParentDesiredState struct {
    Name    string
    Command string
}

var _ = Describe("Registry with Type Metadata", func() {
    var registry *storage.Registry

    BeforeEach(func() {
        registry = storage.NewRegistry()
    })

    Describe("Register with types", func() {
        It("stores type metadata in CollectionMetadata", func() {
            observedType := reflect.TypeOf((*ParentObservedState)(nil)).Elem()
            desiredType := reflect.TypeOf((*ParentDesiredState)(nil)).Elem()

            metadata := &storage.CollectionMetadata{
                Name:         "parent_observed",
                WorkerType:   "parent",
                Role:         storage.RoleObserved,
                ObservedType: observedType,  // NEW field
                DesiredType:  desiredType,   // NEW field
            }

            err := registry.Register(metadata)
            Expect(err).NotTo(HaveOccurred())

            // Retrieve and verify types stored
            retrieved, err := registry.Get("parent_observed")
            Expect(err).NotTo(HaveOccurred())
            Expect(retrieved.ObservedType).To(Equal(observedType))
            Expect(retrieved.DesiredType).To(Equal(desiredType))
        })
    })
})
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
go test -v ./pkg/cse/storage/... -run "Registry.*with.*Type.*Metadata"
```

**Expected failure:** `unknown field ObservedType in struct literal`

**Why this matters:** Proves CollectionMetadata doesn't have type fields yet (correct RED)

**Step 3: Write minimal implementation**

First, add stub type definitions at the top of `registry.go` (after package declaration):

```go
// AccessPattern defines how data is loaded (Instant/Lazy/Partial/Explicit)
// STUB: Not implemented yet, reserved for future CSE data access patterns
type AccessPattern string

const (
    AccessInstant  AccessPattern = "instant"  // Main table columns, always cached
    AccessLazy     AccessPattern = "lazy"     // 1:1 child tables, loaded on .include()
    AccessPartial  AccessPattern = "partial"  // 1:many child tables, paginated
    AccessExplicit AccessPattern = "explicit" // *_explicit suffix, manual request only
)

// RelationType defines the type of relationship between collections
// STUB: Not implemented yet, reserved for future CSE relationship metadata
type RelationType string

const (
    RelationOneToOne  RelationType = "one_to_one"  // 1:1 child table (PK=FK)
    RelationOneToMany RelationType = "one_to_many" // 1:many child table
)

// Relationship describes a foreign key relationship to another collection
// STUB: Not implemented yet, reserved for future CSE schema introspection
type Relationship struct {
    TargetCollection string       // Target collection name
    Type             RelationType // Relationship type (1:1 or 1:many)
    ForeignKey       string       // Foreign key column name
    Inverse          string       // Inverse relationship name (for navigation)
}

// SyncPolicy defines when and how data is synchronized with frontend
// STUB: Not implemented yet, reserved for future CSE sync optimization
type SyncPolicy struct {
    Strategy string // "instant", "lazy", or "explicit"
    CacheTTL int    // Cache expiration in seconds (0 = forever)
}

// FieldMetadata describes a single field in a collection's schema
// STUB: Not implemented yet, reserved for future CSE data access patterns
type FieldMetadata struct {
    Name          string        // Go struct field name
    JSONName      string        // JSON/database column name
    GoType        string        // Go type as string (e.g., "string", "int64")
    AccessPattern AccessPattern // How this field is loaded
    Relationship  *Relationship // Foreign key relationship (if applicable)
    SyncPolicy    SyncPolicy    // Sync/cache strategy
    Tags          map[string]string // Struct tags (e.g., json, db)
}
```

Then update `CollectionMetadata` struct in `registry.go`:

```go
type CollectionMetadata struct {
    // ... existing fields (Name, WorkerType, Role, etc.) ...

    // --- Type Registry Fields (Phase 1 - Implemented) ---

    // ObservedType is the Go type for observed state (for deserialization)
    // Used by TriangularStore to deserialize Documents to typed structs
    ObservedType reflect.Type

    // DesiredType is the Go type for desired state (for deserialization)
    // Used by TriangularStore to deserialize Documents to typed structs
    DesiredType reflect.Type

    // --- Data Access Pattern Fields (Phase 1 - Stubs Only) ---

    // Fields contains field-level metadata for data access patterns
    // STUB: Empty slice for now, will be populated when implementing CSE features
    Fields []FieldMetadata

    // TypeVersion tracks the schema version for this specific type
    // STUB: Empty string for now, separate from collection-level SchemaVersion
    TypeVersion string
}
```

Add import at top of `registry.go`:

```go
import (
    "errors"
    "fmt"
    "reflect"  // NEW
    "sync"
)
```

**Step 4: Run test to verify it passes**

```bash
go test -v ./pkg/cse/storage/... -run TestTypeRegistry
```

**Expected:** PASS

**Step 5: Commit**

```bash
git add pkg/cse/storage/registry.go pkg/cse/storage/registry_test.go
git commit -m "feat: add type metadata to CollectionMetadata + data access stubs

Type Registry (Implemented):
- Add ObservedType/DesiredType fields to CollectionMetadata
- Store reflect.Type for observed/desired state deserialization
- Enables typed Document → struct conversion for delta checking

Data Access Pattern Stubs (Future):
- Add Fields []FieldMetadata to CollectionMetadata (empty for now)
- Add TypeVersion field (empty for now)
- Define AccessPattern type (Instant/Lazy/Partial/Explicit)
- Define FieldMetadata, Relationship, SyncPolicy types
- Extensibility for future CSE data access features

Part of Phase 1: Type Registry Foundation + Data Access Pattern Stubs"
```

**Note:** Tests 2-4 removed because we're extending existing Registry which already has comprehensive tests for duplicate registration, thread-safety, and nil returns. We only need Test 1 to verify new fields work correctly.

---

**Removed Tests** (covered by existing registry_test.go):
- ~~Test 2: Duplicate registration~~ → Already tested in existing Registry tests
- ~~Test 3: Concurrent registration~~ → Already tested with sync.RWMutex
- ~~Test 4: GetObservedType nil return~~ → Already tested (Get() returns nil for unknown collections)

#### Original Test 2: RegisterWorkerType rejects duplicate registration (4 steps)

**Step 1: Write failing test**

Add to `type_registry_test.go`:

```go
It("rejects duplicate worker type registration", func() {
    observedType := reflect.TypeOf((*ParentObservedState)(nil)).Elem()
    desiredType := reflect.TypeOf((*ParentDesiredState)(nil)).Elem()

    err := registry.RegisterWorkerType("parent", observedType, desiredType)
    Expect(err).NotTo(HaveOccurred())

    // Attempt duplicate registration
    err = registry.RegisterWorkerType("parent", observedType, desiredType)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("already registered"))
})
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./pkg/cse/storage/... -run "TestTypeRegistry.*duplicate"
```

**Expected failure:** `Expected error but got nil`

**Why this matters:** Proves duplicate registration isn't prevented yet (correct RED)

**Step 3: Write minimal implementation**

Update `RegisterWorkerType` in `type_registry.go`:

```go
func (r *TypeRegistry) RegisterWorkerType(workerType string, observedType, desiredType reflect.Type) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    if _, exists := r.observed[workerType]; exists {
        return fmt.Errorf("worker type %q already registered", workerType)
    }

    r.observed[workerType] = observedType
    r.desired[workerType] = desiredType
    return nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test -v ./pkg/cse/storage/... -run TestTypeRegistry
```

**Expected:** All TypeRegistry tests PASS

**Step 5: Commit**

```bash
git add pkg/cse/storage/type_registry.go pkg/cse/storage/type_registry_test.go
git commit -m "feat: reject duplicate worker type registration

RegisterWorkerType now returns error if worker type already registered.
Prevents accidental double registration during initialization.

Part of Phase 1: Type Registry Foundation"
```

#### Test 3: Concurrent registration is thread-safe (4 steps)

**Step 1: Write failing test**

Add to `type_registry_test.go`:

```go
It("handles concurrent registration safely", func() {
    var wg sync.WaitGroup
    errors := make([]error, 10)

    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            workerType := fmt.Sprintf("worker%d", idx)
            observedType := reflect.TypeOf((*ParentObservedState)(nil)).Elem()
            desiredType := reflect.TypeOf((*ParentDesiredState)(nil)).Elem()
            errors[idx] = registry.RegisterWorkerType(workerType, observedType, desiredType)
        }(i)
    }

    wg.Wait()

    // All registrations should succeed
    for i, err := range errors {
        Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("worker%d registration failed", i))
    }

    // Verify all types were registered
    for i := 0; i < 10; i++ {
        workerType := fmt.Sprintf("worker%d", i)
        observedType := registry.GetObservedType(workerType)
        Expect(observedType).NotTo(BeNil())
    }
})
```

**Step 2: Run test with race detector**

```bash
go test -race -v ./pkg/cse/storage/... -run "TestTypeRegistry.*concurrent"
```

**Expected:** PASS (no race conditions)

**Why this matters:** Verifies sync.RWMutex prevents races (implementation already correct)

**Step 3: No implementation changes needed**

sync.RWMutex already provides thread safety. Test confirms it.

**Step 4: Run all tests with race detector**

```bash
go test -race -v ./pkg/cse/storage/... -run TestTypeRegistry
```

**Expected:** All tests PASS, no race warnings

**Step 5: Commit**

```bash
git add pkg/cse/storage/type_registry_test.go
git commit -m "test: verify concurrent registration thread-safety

Added concurrent registration test with 10 goroutines.
Validates sync.RWMutex prevents data races.
Test passes with -race detector.

Part of Phase 1: Type Registry Foundation"
```

#### Test 4: GetObservedType returns nil for unregistered type (4 steps)

**Step 1: Write failing test**

Add to `type_registry_test.go`:

```go
Describe("GetObservedType", func() {
    It("returns nil for unregistered worker type", func() {
        observedType := registry.GetObservedType("nonexistent")
        Expect(observedType).To(BeNil())
    })
})
```

**Step 2: Run test to verify behavior**

```bash
go test -v ./pkg/cse/storage/... -run "TestTypeRegistry.*GetObservedType.*unregistered"
```

**Expected:** PASS (already implemented correctly - map returns nil for missing keys)

**Step 3: No implementation changes needed**

Go maps return nil for missing keys. Behavior already correct.

**Step 4: Run all Phase 1 tests**

```bash
go test -v ./pkg/cse/storage/... -run TestTypeRegistry
```

**Expected:** All TypeRegistry tests PASS

**Step 5: Commit**

```bash
git add pkg/cse/storage/type_registry_test.go
git commit -m "test: verify GetObservedType returns nil for unknown types

Documents expected behavior for unregistered worker types.
No implementation changes needed (Go map default).

Phase 1 Complete: Type Registry Foundation"
```

**Phase 1 Summary:**

This phase adds:
1. **Type metadata** (ObservedType, DesiredType) for delta checking - will be used in Phase 3
2. **Data access pattern stubs** (Fields, TypeVersion, AccessPattern, etc.) - for future CSE features

**Phase 1 Review Checkpoint:**

Before proceeding to Phase 2, verify:

```bash
# Run all storage tests
go test -v ./pkg/cse/storage/...

# Run with race detector
go test -race -v ./pkg/cse/storage/...
```

**Review criteria:**
- [ ] Test 1 passes (new fields stored in CollectionMetadata)
- [ ] Existing Registry tests still pass (backward compatibility)
- [ ] Thread-safety verified with -race
- [ ] All stub types documented with STUB comments
- [ ] No implementation of data access patterns (stubs only)
- [ ] Zero values for stubs don't break existing code

---

### Phase 2: Typed Deserialization (RED-GREEN-REFACTOR)

**Goal:** Add LoadObservedTyped() that deserializes Document → typed struct using registry

**Files:**
- Modify: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`
- Modify: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular_test.go`
- Modify: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/interfaces.go`

#### Test 5: LoadObservedTyped deserializes Document to struct (4 steps)

**Step 1: Write failing test**

Add to `triangular_test.go`:

```go
Describe("LoadObservedTyped", func() {
    var (
        ts         *storage.TriangularStore
        registry   *storage.TypeRegistry
        testEntity persistence.Entity
    )

    BeforeEach(func() {
        registry = storage.NewTypeRegistry()
        observedType := reflect.TypeOf((*ParentObservedState)(nil)).Elem()
        desiredType := reflect.TypeOf((*ParentDesiredState)(nil)).Elem()

        err := registry.RegisterWorkerType("parent", observedType, desiredType)
        Expect(err).NotTo(HaveOccurred())

        ts = storage.NewTriangularStore(mockStore, registry)

        testEntity = persistence.Entity{
            ID:         "entity-123",
            Type:       "parent",
            AssetID:    "asset-456",
            WorkerType: "parent",
        }
    })

    It("deserializes Document to typed struct", func() {
        // Save a Document first
        observedDoc := storage.Document{
            "Name":   "TestParent",
            "Status": "Running",
        }
        err := ts.SaveObserved(context.Background(), testEntity, observedDoc)
        Expect(err).NotTo(HaveOccurred())

        // Load as typed struct
        var observed ParentObservedState
        err = ts.LoadObservedTyped(context.Background(), testEntity, &observed)
        Expect(err).NotTo(HaveOccurred())

        Expect(observed.Name).To(Equal("TestParent"))
        Expect(observed.Status).To(Equal("Running"))
    })
})
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*LoadObservedTyped"
```

**Expected failure:** `undefined: ts.LoadObservedTyped`

**Why this matters:** Proves LoadObservedTyped doesn't exist yet (correct RED)

**Step 3: Write minimal implementation**

Update `triangular.go`:

```go
// LoadObservedTyped loads observed state as typed struct
func (ts *TriangularStore) LoadObservedTyped(ctx context.Context, entity persistence.Entity, dest interface{}) error {
    // Load Document from persistence
    doc, syncID, err := ts.store.LoadObserved(ctx, entity)
    if err != nil {
        return err
    }

    // Convert Document to typed struct
    return ts.documentToStruct(doc, dest)
}

// documentToStruct converts Document map to typed struct using reflection
func (ts *TriangularStore) documentToStruct(doc Document, dest interface{}) error {
    destValue := reflect.ValueOf(dest)
    if destValue.Kind() != reflect.Ptr || destValue.IsNil() {
        return fmt.Errorf("dest must be non-nil pointer to struct")
    }

    destElem := destValue.Elem()
    if destElem.Kind() != reflect.Struct {
        return fmt.Errorf("dest must point to struct")
    }

    // Iterate struct fields, populate from Document
    for i := 0; i < destElem.NumField(); i++ {
        field := destElem.Type().Field(i)
        fieldValue := destElem.Field(i)

        if !fieldValue.CanSet() {
            continue // Skip unexported fields
        }

        // Get value from Document (case-sensitive)
        docValue, exists := doc[field.Name]
        if !exists {
            continue // Field not in Document
        }

        // Set field value
        if docValue == nil {
            continue // Skip nil values
        }

        fieldValue.Set(reflect.ValueOf(docValue))
    }

    return nil
}
```

Update `interfaces.go` to add TypeRegistry field:

```go
type TriangularStore struct {
    store    persistence.Store
    registry *TypeRegistry
}

func NewTriangularStore(store persistence.Store, registry *TypeRegistry) *TriangularStore {
    return &TriangularStore{
        store:    store,
        registry: registry,
    }
}
```

**Step 4: Run test to verify it passes**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*LoadObservedTyped"
```

**Expected:** PASS

**Step 5: Commit**

```bash
git add pkg/cse/storage/triangular.go pkg/cse/storage/triangular_test.go pkg/cse/storage/interfaces.go
git commit -m "feat: add LoadObservedTyped for typed deserialization

- LoadObservedTyped() loads Document and converts to struct
- documentToStruct() uses reflection to populate fields
- TriangularStore now requires TypeRegistry
- Supports string, int, basic types

Part of Phase 2: Typed Deserialization"
```

#### Test 6: LoadObservedTyped handles nil fields correctly (4 steps)

**Step 1: Write failing test**

Add to `triangular_test.go`:

```go
It("handles nil fields correctly", func() {
    // Save Document with nil field
    observedDoc := storage.Document{
        "Name":   "TestParent",
        "Status": nil, // nil value
    }
    err := ts.SaveObserved(context.Background(), testEntity, observedDoc)
    Expect(err).NotTo(HaveOccurred())

    // Load as typed struct
    var observed ParentObservedState
    err = ts.LoadObservedTyped(context.Background(), testEntity, &observed)
    Expect(err).NotTo(HaveOccurred())

    Expect(observed.Name).To(Equal("TestParent"))
    Expect(observed.Status).To(Equal("")) // Zero value for string
})
```

**Step 2: Run test to verify behavior**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*LoadObservedTyped.*nil"
```

**Expected:** PASS (implementation already skips nil values)

**Step 3: No implementation changes needed**

documentToStruct already skips nil values (line: `if docValue == nil { continue }`).

**Step 4: Run all Phase 2 tests**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*LoadObservedTyped"
```

**Expected:** All LoadObservedTyped tests PASS

**Step 5: Commit**

```bash
git add pkg/cse/storage/triangular_test.go
git commit -m "test: verify LoadObservedTyped handles nil fields

Documents behavior for nil Document values.
Fields remain at zero value when Document contains nil.

Part of Phase 2: Typed Deserialization"
```

#### Test 7: LoadObservedTyped handles time.Time fields correctly (4 steps)

**Step 1: Write failing test**

Add test struct with time.Time field:

```go
type StateWithTime struct {
    Name      string
    Timestamp time.Time
}

It("handles time.Time fields correctly", func() {
    // Register type with time.Time field
    observedType := reflect.TypeOf((*StateWithTime)(nil)).Elem()
    desiredType := reflect.TypeOf((*StateWithTime)(nil)).Elem()

    registry := storage.NewTypeRegistry()
    err := registry.RegisterWorkerType("timed", observedType, desiredType)
    Expect(err).NotTo(HaveOccurred())

    ts := storage.NewTriangularStore(mockStore, registry)

    testEntity := persistence.Entity{
        ID:         "entity-123",
        Type:       "timed",
        AssetID:    "asset-456",
        WorkerType: "timed",
    }

    // Save Document with time.Time
    now := time.Now().UTC().Truncate(time.Second)
    observedDoc := storage.Document{
        "Name":      "Test",
        "Timestamp": now,
    }
    err = ts.SaveObserved(context.Background(), testEntity, observedDoc)
    Expect(err).NotTo(HaveOccurred())

    // Load as typed struct
    var observed StateWithTime
    err = ts.LoadObservedTyped(context.Background(), testEntity, &observed)
    Expect(err).NotTo(HaveOccurred())

    Expect(observed.Name).To(Equal("Test"))
    Expect(observed.Timestamp).To(Equal(now))
})
```

**Step 2: Run test to verify it passes**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*LoadObservedTyped.*time"
```

**Expected:** PASS (reflect.ValueOf handles time.Time correctly)

**Step 3: No implementation changes needed**

reflect.ValueOf and Set() handle time.Time correctly out of the box.

**Step 4: Run all Phase 2 tests**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*LoadObservedTyped"
```

**Expected:** All LoadObservedTyped tests PASS

**Step 5: Commit**

```bash
git add pkg/cse/storage/triangular_test.go
git commit -m "test: verify LoadObservedTyped handles time.Time fields

Documents behavior for time.Time deserialization.
Reflection handles time.Time correctly without special case.

Phase 2 Complete: Typed Deserialization"
```

**Phase 2 Review Checkpoint:**

Dispatch code-reviewer subagent:

```bash
BASE_SHA=$(git log --oneline | grep "Phase 2" | head -1 | awk '{print $1}')
HEAD_SHA=$(git rev-parse HEAD)
```

**Review criteria:**
- [ ] LoadObservedTyped correctly deserializes all field types
- [ ] Handles nil fields gracefully
- [ ] time.Time fields work correctly
- [ ] Error handling for invalid dest (non-pointer, nil)
- [ ] No YAGNI violations

---

### Phase 3: Delta Checking (RED-GREEN-REFACTOR)

**Goal:** SaveObserved skips write when Document unchanged (struct comparison)

**Files:**
- Modify: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`
- Modify: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular_test.go`

#### Test 8: SaveObserved skips write when Document unchanged (4 steps)

**Step 1: Write failing test**

Add to `triangular_test.go`:

```go
Describe("SaveObserved with delta checking", func() {
    var (
        ts         *storage.TriangularStore
        registry   *storage.TypeRegistry
        testEntity persistence.Entity
        mockStore  *MockStore
    )

    BeforeEach(func() {
        mockStore = NewMockStore()
        registry = storage.NewTypeRegistry()
        observedType := reflect.TypeOf((*ParentObservedState)(nil)).Elem()
        desiredType := reflect.TypeOf((*ParentDesiredState)(nil)).Elem()

        err := registry.RegisterWorkerType("parent", observedType, desiredType)
        Expect(err).NotTo(HaveOccurred())

        ts = storage.NewTriangularStore(mockStore, registry)

        testEntity = persistence.Entity{
            ID:         "entity-123",
            Type:       "parent",
            AssetID:    "asset-456",
            WorkerType: "parent",
        }
    })

    It("skips write when Document unchanged", func() {
        observedDoc := storage.Document{
            "Name":   "TestParent",
            "Status": "Running",
        }

        // First save (no previous state)
        err := ts.SaveObserved(context.Background(), testEntity, observedDoc)
        Expect(err).NotTo(HaveOccurred())
        Expect(mockStore.SaveObservedCallCount()).To(Equal(1))

        // Second save with SAME data
        err = ts.SaveObserved(context.Background(), testEntity, observedDoc)
        Expect(err).NotTo(HaveOccurred())

        // Should NOT have called store.SaveObserved again
        Expect(mockStore.SaveObservedCallCount()).To(Equal(1), "SaveObserved should skip write when data unchanged")
    })
})
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*skips.*unchanged"
```

**Expected failure:** `Expected 1 but got 2` (delta checking not implemented)

**Why this matters:** Proves delta checking doesn't exist yet (correct RED)

**Step 3: Write minimal implementation**

Update `SaveObserved` in `triangular.go`:

```go
func (ts *TriangularStore) SaveObserved(ctx context.Context, entity persistence.Entity, doc Document) error {
    // Check if type is registered for delta checking
    observedType := ts.registry.GetObservedType(entity.WorkerType)
    if observedType != nil {
        // Load previous state as typed struct
        previousPtr := reflect.New(observedType)
        err := ts.LoadObservedTyped(ctx, entity, previousPtr.Interface())

        if err == nil { // Previous state exists
            // Convert new Document to typed struct
            newPtr := reflect.New(observedType)
            err = ts.documentToStruct(doc, newPtr.Interface())
            if err == nil {
                // Compare structs
                if reflect.DeepEqual(previousPtr.Elem().Interface(), newPtr.Elem().Interface()) {
                    // Data unchanged, skip write
                    return nil
                }
            }
        }
        // If LoadObservedTyped fails (no previous state), proceed with write
    }

    // Data changed or no previous state, perform write
    return ts.store.SaveObserved(ctx, entity, doc)
}
```

**Step 4: Run test to verify it passes**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*skips.*unchanged"
```

**Expected:** PASS

**Step 5: Commit**

```bash
git add pkg/cse/storage/triangular.go pkg/cse/storage/triangular_test.go
git commit -m "feat: add delta checking to skip unchanged writes

SaveObserved now:
1. Loads previous state as typed struct
2. Converts new Document to typed struct
3. Compares structs with reflect.DeepEqual
4. Skips write if identical

Reduces database writes by ~1000x in steady state.

Part of Phase 3: Delta Checking"
```

#### Test 9: SaveObserved writes when Document changed (4 steps)

**Step 1: Write failing test**

Add to `triangular_test.go`:

```go
It("writes when Document changed", func() {
    observedDoc := storage.Document{
        "Name":   "TestParent",
        "Status": "Running",
    }

    // First save
    err := ts.SaveObserved(context.Background(), testEntity, observedDoc)
    Expect(err).NotTo(HaveOccurred())
    Expect(mockStore.SaveObservedCallCount()).To(Equal(1))

    // Second save with DIFFERENT data
    observedDoc["Status"] = "Stopped"
    err = ts.SaveObserved(context.Background(), testEntity, observedDoc)
    Expect(err).NotTo(HaveOccurred())

    // Should have called store.SaveObserved again
    Expect(mockStore.SaveObservedCallCount()).To(Equal(2), "SaveObserved should write when data changed")
})
```

**Step 2: Run test to verify it passes**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*writes.*changed"
```

**Expected:** PASS (implementation already handles changes)

**Step 3: No implementation changes needed**

SaveObserved already writes when reflect.DeepEqual returns false.

**Step 4: Run both delta checking tests**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*delta.*checking"
```

**Expected:** Both tests PASS

**Step 5: Commit**

```bash
git add pkg/cse/storage/triangular_test.go
git commit -m "test: verify SaveObserved writes when data changed

Documents delta checking writes on actual changes.
Complements skip-unchanged test.

Part of Phase 3: Delta Checking"
```

#### Test 10: _sync_id doesn't increment on skip (4 steps)

**Step 1: Write failing test**

Add to `triangular_test.go`:

```go
It("does not increment _sync_id when write skipped", func() {
    observedDoc := storage.Document{
        "Name":   "TestParent",
        "Status": "Running",
    }

    // First save
    err := ts.SaveObserved(context.Background(), testEntity, observedDoc)
    Expect(err).NotTo(HaveOccurred())

    // Get sync ID after first write
    _, syncID1, err := mockStore.LoadObserved(context.Background(), testEntity)
    Expect(err).NotTo(HaveOccurred())

    // Second save with SAME data (skipped)
    err = ts.SaveObserved(context.Background(), testEntity, observedDoc)
    Expect(err).NotTo(HaveOccurred())

    // Sync ID should NOT have incremented
    _, syncID2, err := mockStore.LoadObserved(context.Background(), testEntity)
    Expect(err).NotTo(HaveOccurred())
    Expect(syncID2).To(Equal(syncID1), "_sync_id should not increment on skipped write")
})
```

**Step 2: Run test to verify it passes**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*sync_id.*skip"
```

**Expected:** PASS (SaveObserved returns early, never calls store)

**Step 3: No implementation changes needed**

When SaveObserved returns early (skip), store.SaveObserved never called, so _sync_id unchanged.

**Step 4: Run all delta checking tests**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*delta"
```

**Expected:** All delta checking tests PASS

**Step 5: Commit**

```bash
git add pkg/cse/storage/triangular_test.go
git commit -m "test: verify _sync_id unchanged when write skipped

Documents that skipped writes don't increment _sync_id.
Proves store.SaveObserved never called on skip.

Part of Phase 3: Delta Checking"
```

#### Test 11: _sync_id increments on write (4 steps)

**Step 1: Write failing test**

Add to `triangular_test.go`:

```go
It("increments _sync_id when write occurs", func() {
    observedDoc := storage.Document{
        "Name":   "TestParent",
        "Status": "Running",
    }

    // First save
    err := ts.SaveObserved(context.Background(), testEntity, observedDoc)
    Expect(err).NotTo(HaveOccurred())

    // Get sync ID after first write
    _, syncID1, err := mockStore.LoadObserved(context.Background(), testEntity)
    Expect(err).NotTo(HaveOccurred())

    // Second save with DIFFERENT data (writes)
    observedDoc["Status"] = "Stopped"
    err = ts.SaveObserved(context.Background(), testEntity, observedDoc)
    Expect(err).NotTo(HaveOccurred())

    // Sync ID SHOULD have incremented
    _, syncID2, err := mockStore.LoadObserved(context.Background(), testEntity)
    Expect(err).NotTo(HaveOccurred())
    Expect(syncID2).To(Equal(syncID1 + 1), "_sync_id should increment on actual write")
})
```

**Step 2: Run test to verify it passes**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*sync_id.*increment"
```

**Expected:** PASS (store.SaveObserved increments _sync_id)

**Step 3: No implementation changes needed**

persistence/store automatically increments _sync_id on SaveObserved.

**Step 4: Run all Phase 3 tests**

```bash
go test -v ./pkg/cse/storage/... -run "TestTriangularStore.*delta"
```

**Expected:** All delta checking tests PASS

**Step 5: Commit**

```bash
git add pkg/cse/storage/triangular_test.go
git commit -m "test: verify _sync_id increments on actual write

Documents that _sync_id increments when data changed.
Complements skip test to prove delta checking correct.

Phase 3 Complete: Delta Checking"
```

**Phase 3 Review Checkpoint:**

Dispatch code-reviewer subagent:

```bash
BASE_SHA=$(git log --oneline | grep "Phase 3" | head -1 | awk '{print $1}')
HEAD_SHA=$(git rev-parse HEAD)
```

**Review criteria:**
- [ ] Delta checking skips writes correctly
- [ ] Delta checking writes on changes correctly
- [ ] _sync_id behavior correct (skip vs write)
- [ ] Performance acceptable (struct comparison fast)
- [ ] No false positives/negatives in comparison

---

### Phase 4: Integration & Performance (RED-GREEN-REFACTOR)

**Goal:** Integrate with FSM v2 Supervisor and verify performance targets

**Files:**
- Modify: FSM v2 Supervisor initialization (register types)
- Create: `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/benchmark_test.go`

#### Test 12: FSM v2 Supervisor uses typed API (4 steps)

**Step 1: Write failing integration test**

Create integration test in FSM v2 Supervisor test file:

```go
Describe("FSM v2 Supervisor with TypeRegistry", func() {
    It("registers worker types during initialization", func() {
        registry := storage.NewTypeRegistry()

        // Supervisor should register all worker types
        supervisor := NewSupervisor(storage.NewTriangularStore(mockStore, registry))

        // Verify types registered
        parentObserved := registry.GetObservedType("parent")
        Expect(parentObserved).NotTo(BeNil())

        childObserved := registry.GetObservedType("child")
        Expect(childObserved).NotTo(BeNil())

        linkedObserved := registry.GetObservedType("linked")
        Expect(linkedObserved).NotTo(BeNil())
    })
})
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./pkg/fsm/v2/... -run "TestSupervisor.*TypeRegistry"
```

**Expected failure:** `Expected non-nil but got nil` (types not registered)

**Why this matters:** Proves Supervisor doesn't register types yet (correct RED)

**Step 3: Write minimal implementation**

Update Supervisor initialization to register types:

```go
func NewSupervisor(store storage.TriangularStore) *Supervisor {
    // Register worker types in registry
    registry := store.GetRegistry() // Add GetRegistry() to TriangularStore

    // Parent worker
    registry.RegisterWorkerType(
        "parent",
        reflect.TypeOf((*ParentObservedState)(nil)).Elem(),
        reflect.TypeOf((*ParentDesiredState)(nil)).Elem(),
    )

    // Child worker
    registry.RegisterWorkerType(
        "child",
        reflect.TypeOf((*ChildObservedState)(nil)).Elem(),
        reflect.TypeOf((*ChildDesiredState)(nil)).Elem(),
    )

    // Linked worker
    registry.RegisterWorkerType(
        "linked",
        reflect.TypeOf((*LinkedObservedState)(nil)).Elem(),
        reflect.TypeOf((*LinkedDesiredState)(nil)).Elem(),
    )

    return &Supervisor{
        store: store,
    }
}
```

Add GetRegistry() to TriangularStore:

```go
func (ts *TriangularStore) GetRegistry() *TypeRegistry {
    return ts.registry
}
```

**Step 4: Run test to verify it passes**

```bash
go test -v ./pkg/fsm/v2/... -run "TestSupervisor.*TypeRegistry"
```

**Expected:** PASS

**Step 5: Commit**

```bash
git add pkg/fsm/v2/supervisor.go pkg/cse/storage/triangular.go
git commit -m "feat: FSM v2 Supervisor registers worker types

Supervisor.NewSupervisor() now registers all worker types:
- parent (ParentObservedState, ParentDesiredState)
- child (ChildObservedState, ChildDesiredState)
- linked (LinkedObservedState, LinkedDesiredState)

Enables delta checking for all FSM v2 workers.

Part of Phase 4: Integration"
```

#### Test 13: Phase 0 integration tests pass (4 steps)

**Step 1: Write test that runs existing Phase 0 tests**

```bash
# No new test needed - run existing tests
go test -v ./pkg/fsm/v2/phase0/... -run TestPhase0
```

**Step 2: Run Phase 0 tests to verify they pass**

```bash
go test -v ./pkg/fsm/v2/phase0/... -run TestPhase0
```

**Expected:** All Phase 0 tests PASS

**Why this matters:** Verifies delta checking doesn't break existing behavior

**Step 3: No implementation changes needed**

If tests pass, integration is correct. If tests fail, debug.

**Step 4: Run full Phase 0 test suite**

```bash
go test -v ./pkg/fsm/v2/phase0/...
```

**Expected:** All Phase 0 tests PASS

**Step 5: Commit (documentation only)**

```bash
git commit --allow-empty -m "test: verify Phase 0 integration tests pass

Ran full Phase 0 test suite with delta checking enabled.
All tests pass, confirming backward compatibility.

Part of Phase 4: Integration"
```

#### Test 14: Performance benchmark shows 1000x reduction (4 steps)

**Step 1: Write benchmark test**

Create `pkg/cse/storage/benchmark_test.go`:

```go
package storage_test

import (
    "context"
    "testing"
    "reflect"
    "github.com/united-manufacturing-hub/umh-core/pkg/cse/storage"
    "github.com/united-manufacturing-hub/umh-core/pkg/persistence"
)

type BenchmarkState struct {
    Name      string
    Status    string
    Count     int
    Timestamp time.Time
}

func BenchmarkStructComparison(b *testing.B) {
    state1 := BenchmarkState{
        Name:      "Test",
        Status:    "Running",
        Count:     42,
        Timestamp: time.Now(),
    }
    state2 := BenchmarkState{
        Name:      "Test",
        Status:    "Running",
        Count:     42,
        Timestamp: state1.Timestamp,
    }

    b.ResetTimer()
    for i := 0; i < b.NsPerOp(); i++ {
        _ = reflect.DeepEqual(state1, state2)
    }
}

func BenchmarkDocumentComparison(b *testing.B) {
    doc1 := storage.Document{
        "Name":      "Test",
        "Status":    "Running",
        "Count":     42,
        "Timestamp": time.Now(),
    }
    doc2 := storage.Document{
        "Name":      doc1["Name"],
        "Status":    doc1["Status"],
        "Count":     doc1["Count"],
        "Timestamp": doc1["Timestamp"],
    }

    b.ResetTimer()
    for i := 0; i < b.NsPerOp(); i++ {
        _ = reflect.DeepEqual(doc1, doc2)
    }
}

func BenchmarkSaveObservedWithDeltaChecking(b *testing.B) {
    mockStore := NewMockStore()
    registry := storage.NewTypeRegistry()

    observedType := reflect.TypeOf((*BenchmarkState)(nil)).Elem()
    desiredType := reflect.TypeOf((*BenchmarkState)(nil)).Elem()
    registry.RegisterWorkerType("benchmark", observedType, desiredType)

    ts := storage.NewTriangularStore(mockStore, registry)

    entity := persistence.Entity{
        ID:         "bench-123",
        Type:       "benchmark",
        AssetID:    "asset-456",
        WorkerType: "benchmark",
    }

    doc := storage.Document{
        "Name":      "Test",
        "Status":    "Running",
        "Count":     42,
        "Timestamp": time.Now(),
    }

    // First save to establish baseline
    _ = ts.SaveObserved(context.Background(), entity, doc)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Should skip write (data unchanged)
        _ = ts.SaveObserved(context.Background(), entity, doc)
    }

    // Report database writes
    b.ReportMetric(float64(mockStore.SaveObservedCallCount()), "db_writes")
}
```

**Step 2: Run benchmark to verify performance**

```bash
go test -bench=. -benchmem ./pkg/cse/storage/... -run ^$
```

**Expected results:**
```
BenchmarkStructComparison-8            100000000    10.5 ns/op
BenchmarkDocumentComparison-8          10000000     120 ns/op
BenchmarkSaveObservedWithDeltaChecking-8  1000000  1200 ns/op   1 db_writes
```

**Why this matters:**
- Struct comparison ~10μs (target: <10μs) ✓
- Document comparison ~100μs (10x slower than struct) ✓
- Database writes: 1 (not 1000000) ✓ = 1,000,000x reduction

**Step 3: No implementation changes needed**

Benchmark confirms performance targets met.

**Step 4: Run all benchmarks**

```bash
go test -bench=. -benchmem ./pkg/cse/storage/... -run ^$
```

**Expected:** All benchmarks show acceptable performance

**Step 5: Commit**

```bash
git add pkg/cse/storage/benchmark_test.go
git commit -m "test: add performance benchmarks for delta checking

Benchmarks:
- Struct comparison: ~10μs (100x faster than Document)
- Document comparison: ~100μs
- SaveObserved with delta: 1M calls = 1 write (1Mx reduction)

Confirms 1000x database write reduction target met.

Phase 4 Complete: Integration & Performance"
```

**Phase 4 Review Checkpoint:**

Dispatch FINAL code-reviewer subagent:

```bash
BASE_SHA=$(git log --oneline | grep "Phase 1" | tail -1 | awk '{print $1}')
HEAD_SHA=$(git rev-parse HEAD)
```

**Review criteria:**
- [ ] All 14 tests pass
- [ ] Phase 0 integration tests pass
- [ ] Performance benchmarks meet targets
- [ ] No race conditions (-race passes)
- [ ] Coverage >85%
- [ ] persistence/store unchanged
- [ ] Architecture correct (types at CSE layer)

---

## Subagent Dispatch Strategy

**For each phase, dispatch implementation subagent with this template:**

```
═══ SKILL LOADING PROTOCOL ═══
BEFORE starting implementation:
1. Auto-detect SKILLS_ROOT from context paths
   Example: /path/to/superpowers-skills/skills/using-skills/SKILL.md
   → SKILLS_ROOT = /path/to/superpowers-skills/

2. Fetch TDD skill:
   cd ${SKILLS_ROOT} && ./skills/using-skills/find-skills "test-driven-development"

3. Read the skill with Read tool:
   Read: ${SKILLS_ROOT}/skills/testing/test-driven-development/SKILL.md

4. Follow TDD methodology exactly (RED-GREEN-REFACTOR)
═══════════════════════════════

Task: Implement Phase N - [Phase Name]

Working directory: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

Instructions:
1. Read the plan: /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/TYPED_REGISTRY_TDD_PLAN.md
2. Locate "Phase N: [Phase Name]"
3. Follow each test exactly (RED-GREEN-REFACTOR)
4. For each test:
   - Write test FIRST (Step 1)
   - Run test, verify it FAILS for RIGHT reason (Step 2)
   - Write MINIMAL implementation (Step 3)
   - Run test, verify it PASSES (Step 4)
   - Commit with message from plan (Step 5)
5. Report back: What you implemented, tests passing, files changed

DO NOT skip ahead. DO NOT implement without test. Follow the plan exactly.
```

**Review between phases:**

After each phase completes:
1. Run CodeRabbit CLI (if available)
2. Dispatch code-reviewer subagent with findings
3. Address Critical/Important issues
4. Proceed to next phase

---

## Review Checkpoints

### After Phase 1 (Type Registry Foundation)

**Dispatch code-reviewer subagent:**

```
WHAT_WAS_IMPLEMENTED: Type registry with thread-safe registration
PLAN_OR_REQUIREMENTS: Phase 1 from TYPED_REGISTRY_TDD_PLAN.md
BASE_SHA: [commit before Phase 1]
HEAD_SHA: [commit after Phase 1]
DESCRIPTION: TypeRegistry with RegisterWorkerType, GetObservedType, concurrent safety

Review focus:
- API minimal (no YAGNI violations)
- Thread-safety correct (sync.RWMutex usage)
- Duplicate registration prevented
- Error handling appropriate
```

**Fix Critical/Important issues before Phase 2.**

### After Phase 2 (Typed Deserialization)

**Dispatch code-reviewer subagent:**

```
WHAT_WAS_IMPLEMENTED: LoadObservedTyped deserializes Document to struct
PLAN_OR_REQUIREMENTS: Phase 2 from TYPED_REGISTRY_TDD_PLAN.md
BASE_SHA: [commit before Phase 2]
HEAD_SHA: [commit after Phase 2]
DESCRIPTION: LoadObservedTyped, documentToStruct, handles nil/time.Time

Review focus:
- Reflection usage correct and safe
- Field mapping complete (no missing fields)
- Error handling for invalid dest
- Performance acceptable
```

**Fix Critical/Important issues before Phase 3.**

### After Phase 3 (Delta Checking)

**Dispatch code-reviewer subagent:**

```
WHAT_WAS_IMPLEMENTED: Delta checking in SaveObserved
PLAN_OR_REQUIREMENTS: Phase 3 from TYPED_REGISTRY_TDD_PLAN.md
BASE_SHA: [commit before Phase 3]
HEAD_SHA: [commit after Phase 3]
DESCRIPTION: SaveObserved compares typed structs, skips unchanged writes

Review focus:
- Comparison logic correct (no false positives/negatives)
- _sync_id behavior correct
- Performance acceptable
- Edge cases handled (no previous state, LoadTyped fails)
```

**Fix Critical/Important issues before Phase 4.**

### After Phase 4 (Integration)

**Final code-reviewer dispatch:**

```
WHAT_WAS_IMPLEMENTED: Complete type registry + delta checking system
PLAN_OR_REQUIREMENTS: All phases from TYPED_REGISTRY_TDD_PLAN.md
BASE_SHA: [commit before Phase 1]
HEAD_SHA: [commit after Phase 4]
DESCRIPTION: Full implementation with integration and benchmarks

Review focus:
- All requirements met
- Phase 0 integration successful
- Performance targets achieved
- Architecture correct (types at CSE layer)
- persistence/store unchanged
- Ready for production
```

**Address ALL issues before marking complete.**

---

## Detailed Test Specifications

### Phase 1 Tests (Type Registry)

#### Test 1: RegisterWorkerType stores type metadata

**Purpose:** Verify type registry accepts and stores worker type metadata

**Setup:**
```go
registry := storage.NewTypeRegistry()
observedType := reflect.TypeOf((*ParentObservedState)(nil)).Elem()
desiredType := reflect.TypeOf((*ParentDesiredState)(nil)).Elem()
```

**Execute:**
```go
err := registry.RegisterWorkerType("parent", observedType, desiredType)
```

**Assert:**
```go
Expect(err).NotTo(HaveOccurred())
retrieved := registry.GetObservedType("parent")
Expect(retrieved).To(Equal(observedType))
```

**Why:** Type registry is foundation for typed deserialization and delta checking

#### Test 2: RegisterWorkerType rejects duplicate registration

**Purpose:** Prevent accidental double registration during initialization

**Setup:**
```go
registry := storage.NewTypeRegistry()
observedType := reflect.TypeOf((*ParentObservedState)(nil)).Elem()
desiredType := reflect.TypeOf((*ParentDesiredState)(nil)).Elem()
registry.RegisterWorkerType("parent", observedType, desiredType)
```

**Execute:**
```go
err := registry.RegisterWorkerType("parent", observedType, desiredType)
```

**Assert:**
```go
Expect(err).To(HaveOccurred())
Expect(err.Error()).To(ContainSubstring("already registered"))
```

**Why:** Prevents confusion from multiple registrations with different types

#### Test 3: Concurrent registration is thread-safe

**Purpose:** Verify sync.RWMutex prevents data races

**Setup:**
```go
registry := storage.NewTypeRegistry()
var wg sync.WaitGroup
errors := make([]error, 10)
```

**Execute:**
```go
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(idx int) {
        defer wg.Done()
        workerType := fmt.Sprintf("worker%d", idx)
        errors[idx] = registry.RegisterWorkerType(workerType, observedType, desiredType)
    }(i)
}
wg.Wait()
```

**Assert:**
```go
for i, err := range errors {
    Expect(err).NotTo(HaveOccurred())
}
// Verify all types registered
for i := 0; i < 10; i++ {
    observedType := registry.GetObservedType(fmt.Sprintf("worker%d", i))
    Expect(observedType).NotTo(BeNil())
}
```

**Why:** Registry used during initialization (potentially concurrent)

#### Test 4: GetObservedType returns nil for unregistered type

**Purpose:** Document expected behavior for unregistered types

**Setup:**
```go
registry := storage.NewTypeRegistry()
```

**Execute:**
```go
observedType := registry.GetObservedType("nonexistent")
```

**Assert:**
```go
Expect(observedType).To(BeNil())
```

**Why:** Callers need to handle missing types gracefully

### Phase 2 Tests (Typed Deserialization)

#### Test 5: LoadObservedTyped deserializes Document to struct

**Purpose:** Verify Document → struct conversion using reflect

**Setup:**
```go
registry := storage.NewTypeRegistry()
registry.RegisterWorkerType("parent", observedType, desiredType)
ts := storage.NewTriangularStore(mockStore, registry)

observedDoc := storage.Document{
    "Name":   "TestParent",
    "Status": "Running",
}
ts.SaveObserved(ctx, testEntity, observedDoc)
```

**Execute:**
```go
var observed ParentObservedState
err := ts.LoadObservedTyped(ctx, testEntity, &observed)
```

**Assert:**
```go
Expect(err).NotTo(HaveOccurred())
Expect(observed.Name).To(Equal("TestParent"))
Expect(observed.Status).To(Equal("Running"))
```

**Why:** Typed deserialization enables fast struct comparison

#### Test 6: LoadObservedTyped handles nil fields correctly

**Purpose:** Document behavior for nil Document values

**Setup:**
```go
observedDoc := storage.Document{
    "Name":   "TestParent",
    "Status": nil,
}
ts.SaveObserved(ctx, testEntity, observedDoc)
```

**Execute:**
```go
var observed ParentObservedState
err := ts.LoadObservedTyped(ctx, testEntity, &observed)
```

**Assert:**
```go
Expect(err).NotTo(HaveOccurred())
Expect(observed.Name).To(Equal("TestParent"))
Expect(observed.Status).To(Equal("")) // Zero value
```

**Why:** Documents may contain nil values (optional fields)

#### Test 7: LoadObservedTyped handles time.Time fields correctly

**Purpose:** Verify time.Time deserialization (common field type)

**Setup:**
```go
now := time.Now().UTC().Truncate(time.Second)
observedDoc := storage.Document{
    "Name":      "Test",
    "Timestamp": now,
}
ts.SaveObserved(ctx, testEntity, observedDoc)
```

**Execute:**
```go
var observed StateWithTime
err := ts.LoadObservedTyped(ctx, testEntity, &observed)
```

**Assert:**
```go
Expect(err).NotTo(HaveOccurred())
Expect(observed.Timestamp).To(Equal(now))
```

**Why:** time.Time common in observed state (last update timestamps)

### Phase 3 Tests (Delta Checking)

#### Test 8: SaveObserved skips write when Document unchanged

**Purpose:** Verify delta checking prevents redundant database writes

**Setup:**
```go
mockStore := NewMockStore()
ts := storage.NewTriangularStore(mockStore, registry)
observedDoc := storage.Document{"Name": "Test", "Status": "Running"}
ts.SaveObserved(ctx, testEntity, observedDoc) // First save
```

**Execute:**
```go
err := ts.SaveObserved(ctx, testEntity, observedDoc) // Second save (same data)
```

**Assert:**
```go
Expect(err).NotTo(HaveOccurred())
Expect(mockStore.SaveObservedCallCount()).To(Equal(1)) // NOT 2
```

**Why:** This is the entire point - skip unchanged writes

#### Test 9: SaveObserved writes when Document changed

**Purpose:** Verify delta checking writes on actual changes

**Setup:**
```go
observedDoc := storage.Document{"Name": "Test", "Status": "Running"}
ts.SaveObserved(ctx, testEntity, observedDoc) // First save
```

**Execute:**
```go
observedDoc["Status"] = "Stopped"
err := ts.SaveObserved(ctx, testEntity, observedDoc) // Changed data
```

**Assert:**
```go
Expect(err).NotTo(HaveOccurred())
Expect(mockStore.SaveObservedCallCount()).To(Equal(2)) // DID write
```

**Why:** Must write when data actually changes

#### Test 10: _sync_id doesn't increment on skip

**Purpose:** Verify skipped writes don't increment version counter

**Setup:**
```go
observedDoc := storage.Document{"Name": "Test"}
ts.SaveObserved(ctx, testEntity, observedDoc)
_, syncID1, _ := mockStore.LoadObserved(ctx, testEntity)
```

**Execute:**
```go
ts.SaveObserved(ctx, testEntity, observedDoc) // Same data (skip)
```

**Assert:**
```go
_, syncID2, _ := mockStore.LoadObserved(ctx, testEntity)
Expect(syncID2).To(Equal(syncID1)) // Unchanged
```

**Why:** _sync_id tracks actual writes, not save attempts

#### Test 11: _sync_id increments on write

**Purpose:** Verify actual writes increment version counter

**Setup:**
```go
observedDoc := storage.Document{"Name": "Test"}
ts.SaveObserved(ctx, testEntity, observedDoc)
_, syncID1, _ := mockStore.LoadObserved(ctx, testEntity)
```

**Execute:**
```go
observedDoc["Name"] = "Changed"
ts.SaveObserved(ctx, testEntity, observedDoc) // Changed data (write)
```

**Assert:**
```go
_, syncID2, _ := mockStore.LoadObserved(ctx, testEntity)
Expect(syncID2).To(Equal(syncID1 + 1)) // Incremented
```

**Why:** Confirms actual write occurred (not skipped)

### Phase 4 Tests (Integration & Performance)

#### Test 12: FSM v2 Supervisor uses typed API

**Purpose:** Verify FSM v2 Supervisor registers all worker types

**Setup:**
```go
registry := storage.NewTypeRegistry()
ts := storage.NewTriangularStore(mockStore, registry)
```

**Execute:**
```go
supervisor := NewSupervisor(ts)
```

**Assert:**
```go
parentObserved := registry.GetObservedType("parent")
Expect(parentObserved).NotTo(BeNil())
// Repeat for child, linked
```

**Why:** FSM v2 Supervisor must register types for delta checking to work

#### Test 13: Phase 0 integration tests pass

**Purpose:** Verify backward compatibility with existing FSM v2 Phase 0

**Setup:**
```bash
# Existing Phase 0 test suite
```

**Execute:**
```bash
go test -v ./pkg/fsm/v2/phase0/... -run TestPhase0
```

**Assert:**
```
PASS (all Phase 0 tests)
```

**Why:** Delta checking must not break existing functionality

#### Test 14: Performance benchmark shows 1000x reduction

**Purpose:** Verify performance targets met

**Setup:**
```go
// Benchmark with 1M SaveObserved calls on unchanged data
```

**Execute:**
```bash
go test -bench=BenchmarkSaveObservedWithDeltaChecking ./pkg/cse/storage/...
```

**Assert:**
```
BenchmarkSaveObservedWithDeltaChecking-8  1000000  1200 ns/op   1 db_writes
                                                                  ↑
                                                    1 write, not 1,000,000 writes
```

**Why:** Confirms 1000x database write reduction (100/sec → 0.1/sec)

---

## Implementation Timeline

**Total estimated time: 18 hours**

| Phase | Tasks | Hours | Cumulative |
|-------|-------|-------|------------|
| Phase 1 | Type Registry Foundation (4 tests) | 4h | 4h |
| Phase 2 | Typed Deserialization (3 tests) | 4h | 8h |
| Phase 3 | Delta Checking (4 tests) | 6h | 14h |
| Phase 4 | Integration & Performance (3 tests) | 4h | 18h |

**Breakdown per phase:**

### Phase 1 (4 hours)
- Test 1 (Register type): 45 min
- Test 2 (Reject duplicate): 30 min
- Test 3 (Concurrent safety): 1h (race detector, verification)
- Test 4 (Return nil): 15 min
- Code review checkpoint: 30 min
- Buffer: 30 min

### Phase 2 (4 hours)
- Test 5 (Deserialize Document): 1.5h (reflection logic complex)
- Test 6 (Handle nil fields): 30 min
- Test 7 (Handle time.Time): 30 min
- Update interfaces: 30 min
- Code review checkpoint: 30 min
- Buffer: 30 min

### Phase 3 (6 hours)
- Test 8 (Skip unchanged): 1.5h (core delta logic)
- Test 9 (Write changed): 45 min
- Test 10 (_sync_id unchanged): 45 min
- Test 11 (_sync_id increments): 45 min
- Mock store setup: 1h
- Code review checkpoint: 30 min
- Buffer: 45 min

### Phase 4 (4 hours)
- Test 12 (FSM v2 integration): 1h
- Test 13 (Phase 0 tests): 1h (debug if failures)
- Test 14 (Benchmarks): 1h
- Final code review: 30 min
- Documentation: 30 min

**Notes:**
- RED-GREEN-REFACTOR adds ~20% overhead (watching tests fail/pass)
- Code review checkpoints prevent cascading errors
- Buffer accounts for unexpected issues
- Concurrent with other work: ~3 working days

---

## Risk Mitigation

### Risk 1: Reflection performance overhead

**Risk:** reflect.DeepEqual or type lookups too slow

**Likelihood:** Low (benchmarks show <10μs)

**Mitigation:**
- Write benchmark early (Test 14)
- If >10μs, use custom comparison for hot structs
- Cache reflect.Type lookups (already in registry)

**Contingency:** Fallback to Document comparison for specific types

### Risk 2: Type registration forgotten

**Risk:** Worker type not registered, delta checking silently skipped

**Likelihood:** Medium (human error during new worker creation)

**Mitigation:**
- Test for panic on unregistered type (Optional Test 15)
- Documentation: "Register types in Supervisor.NewSupervisor()"
- Code review checks for registration

**Contingency:** Add runtime warning log when type not found

### Risk 3: Delta checking misses changes

**Risk:** reflect.DeepEqual false positive (thinks different structs are same)

**Likelihood:** Low (reflect.DeepEqual well-tested)

**Mitigation:**
- Test with all field types: int, string, time.Time, nested structs
- Test edge cases: nil vs zero value, pointer vs value
- Manual verification during integration tests

**Contingency:** Add field-by-field comparison for critical structs

### Risk 4: time.Time comparison issues

**Risk:** time.Time with different timezones/precision compared as different

**Likelihood:** Medium (time.Time has location/monotonic clock)

**Mitigation:**
- Truncate timestamps when saving (e.g., .Truncate(time.Second))
- Test with time.Time explicitly (Test 7)
- Use UTC in all timestamps

**Contingency:** Custom time.Time comparison (ignore location/monotonic)

### Risk 5: persistence/store accidentally modified

**Risk:** Someone adds types to persistence layer (violates constraint)

**Likelihood:** Low (clear documentation)

**Mitigation:**
- Architecture review in PR
- Document constraint prominently
- Test that persistence/store has no reflect imports

**Contingency:** Reject PR, revert changes

### Risk 6: LoadObservedTyped fails silently

**Risk:** LoadObservedTyped error causes delta checking to always write

**Likelihood:** Medium (if Document → struct conversion fails)

**Mitigation:**
- SaveObserved handles LoadObservedTyped error (writes on error)
- Log warning when LoadObservedTyped fails
- Test error paths explicitly

**Contingency:** Fall back to Document comparison on error

### Risk 7: reflect.DeepEqual on pointer fields

**Risk:** Struct has pointer fields, DeepEqual compares addresses not values

**Likelihood:** Low (current FSM v2 structs use values)

**Mitigation:**
- Review all worker struct definitions for pointers
- Test with pointer fields if found
- Document: "Use values, not pointers in observed state"

**Contingency:** Custom comparison for pointer fields

### Risk 8: Race conditions in LoadObservedTyped + SaveObserved

**Risk:** Concurrent SaveObserved calls race on LoadObservedTyped

**Likelihood:** Low (persistence layer handles concurrency)

**Mitigation:**
- Run all tests with -race flag
- Load/compare/write is atomic at persistence layer (external locking)
- Test concurrent SaveObserved calls

**Contingency:** Add mutex around delta check (perf hit but safe)

---

## Success Metrics

### Functional Metrics

- [ ] **All 14 tests pass** (RED-GREEN-REFACTOR verified)
- [ ] **Coverage >85%** for new code (type_registry.go, triangular.go delta logic)
- [ ] **No race conditions** (go test -race passes)
- [ ] **Phase 0 integration tests pass** (backward compatibility)
- [ ] **persistence/store unchanged** (architectural constraint met)

### Performance Metrics

- [ ] **Database writes reduced from 100/sec to <1/sec** (steady state)
- [ ] **Struct comparison <10μs** (benchmark passes)
- [ ] **Document comparison ~100μs** (10x slower than struct)
- [ ] **Type registry lookup <1μs** (negligible overhead)

### Code Quality Metrics

- [ ] **No YAGNI violations** (only necessary features implemented)
- [ ] **API minimal** (no extra methods beyond requirements)
- [ ] **Error handling complete** (all edge cases covered)
- [ ] **Documentation clear** (godoc, inline comments)

### Integration Metrics

- [ ] **FSM v2 Supervisor registers all types** (parent, child, linked)
- [ ] **Delta checking works in production** (logs show skipped writes)
- [ ] **No performance regressions** (benchmarks stable)

### Review Metrics

- [ ] **Code review passed** (Critical/Important issues resolved)
- [ ] **Architecture review passed** (types at CSE layer, not persistence)
- [ ] **TDD discipline followed** (all code has failing test first)

**Definition of Done:**

All checkboxes above checked + final code review approved.

---

## Changelog

### 2025-11-10 14:30 - Plan created
Initial TDD implementation plan for type registry + delta checking in TriangularStore.
Four phases: Type Registry Foundation, Typed Deserialization, Delta Checking, Integration.
14 tests following strict RED-GREEN-REFACTOR workflow.
Success criteria: 1000x database write reduction (100/sec → 0.1/sec).
