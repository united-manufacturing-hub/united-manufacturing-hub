# Registry Elimination - Pure Generics Implementation Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/executing-plans/SKILL.md` to implement this plan task-by-task.

**Goal:** Eliminate the registry system and use pure Go generics for triangular storage, reducing complexity by 400+ LOC while improving type safety.

**Architecture:** Remove runtime registry lookup by hardcoding CSE metadata constants (discovered to be identical across all roles) and using type parameters to derive collection names. Generic APIs (SaveObservedTyped[T], LoadObservedTyped[T]) become the only APIs - no more dual system.

**Tech Stack:** Go 1.21+ generics, SQLite persistence layer, Ginkgo/Gomega testing

**Created:** 2025-01-13 14:30
**Last Updated:** 2025-01-13 14:30

---

## Timeline Summary

| Phase | Description | Duration | Risk Level |
|-------|-------------|----------|------------|
| 1 | Preparation & Testing | 1 day | Low |
| 2 | Introduce Constants | 1 day | Low |
| 3 | Generic APIs Independent | 2 days | Medium |
| 4 | Deprecate Legacy APIs | 1 day | Low |
| 5 | Migrate Call Sites | 5-7 days | High |
| 6 | Remove Legacy & Registry | 2 days | High |
| 7 | Simplify Naming | 1 day | Low |
| **Total** | | **13-15 days** | |

---

## Phase 1: Preparation and Testing (1 day)

### Goal
Establish baseline tests and documentation to ensure safe refactoring.

### Task 1.1: Add Migration Scenario Tests

**File to create:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/migration_test.go`

**Step 1: Write the test file structure**

```go
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

package storage_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("Storage Migration Scenarios", func() {
	var (
		ctx   context.Context
		store persistence.Store
		ts    *storage.TriangularStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = persistence.NewInMemoryStore()
		ts = storage.NewTriangularStore(store, storage.NewRegistry())
	})

	Context("Field Addition Migration", func() {
		type OldVersion struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		}

		type NewVersion struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Status   string `json:"status"`
			NewField string `json:"newField"` // Added field
		}

		It("should load old data into new struct with added field", func() {
			// Save data with old struct
			oldData := OldVersion{
				ID:     "worker-123",
				Name:   "Test Worker",
				Status: "running",
			}

			// Manually insert old data (simulating old version of code)
			collectionName := "test_observed"
			docBytes, err := json.Marshal(oldData)
			Expect(err).NotTo(HaveOccurred())

			var doc persistence.Document
			err = json.Unmarshal(docBytes, &doc)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, collectionName, doc)
			Expect(err).NotTo(HaveOccurred())

			// Load with new struct
			var loaded NewVersion
			doc, err = store.Get(ctx, collectionName, "worker-123")
			Expect(err).NotTo(HaveOccurred())

			docBytes, err = json.Marshal(doc)
			Expect(err).NotTo(HaveOccurred())

			err = json.Unmarshal(docBytes, &loaded)
			Expect(err).NotTo(HaveOccurred())

			// Old fields should be preserved
			Expect(loaded.ID).To(Equal("worker-123"))
			Expect(loaded.Name).To(Equal("Test Worker"))
			Expect(loaded.Status).To(Equal("running"))
			// New field should be zero value
			Expect(loaded.NewField).To(Equal(""))
		})
	})

	Context("Field Removal Migration", func() {
		type OldVersion struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			OldField   string `json:"oldField"` // Will be removed
		}

		type NewVersion struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
			// OldField removed
		}

		It("should load old data into new struct ignoring removed field", func() {
			// Save data with old struct
			oldData := OldVersion{
				ID:       "worker-456",
				Name:     "Test Worker 2",
				Status:   "stopped",
				OldField: "deprecated value",
			}

			collectionName := "test_observed"
			docBytes, err := json.Marshal(oldData)
			Expect(err).NotTo(HaveOccurred())

			var doc persistence.Document
			err = json.Unmarshal(docBytes, &doc)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, collectionName, doc)
			Expect(err).NotTo(HaveOccurred())

			// Load with new struct
			var loaded NewVersion
			doc, err = store.Get(ctx, collectionName, "worker-456")
			Expect(err).NotTo(HaveOccurred())

			docBytes, err = json.Marshal(doc)
			Expect(err).NotTo(HaveOccurred())

			err = json.Unmarshal(docBytes, &loaded)
			Expect(err).NotTo(HaveOccurred())

			// Kept fields should be preserved
			Expect(loaded.ID).To(Equal("worker-456"))
			Expect(loaded.Name).To(Equal("Test Worker 2"))
			Expect(loaded.Status).To(Equal("stopped"))
			// Old field is ignored (not an error)
		})
	})

	Context("Field Rename Migration (using JSON tags)", func() {
		type OldVersion struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			OldStatus  string `json:"status"` // Old field name
		}

		type NewVersion struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			NewStatus  string `json:"status"` // New field name, same JSON tag
		}

		It("should load old data into renamed field via JSON tag", func() {
			// Save data with old struct
			oldData := OldVersion{
				ID:        "worker-789",
				Name:      "Test Worker 3",
				OldStatus: "active",
			}

			collectionName := "test_observed"
			docBytes, err := json.Marshal(oldData)
			Expect(err).NotTo(HaveOccurred())

			var doc persistence.Document
			err = json.Unmarshal(docBytes, &doc)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, collectionName, doc)
			Expect(err).NotTo(HaveOccurred())

			// Load with new struct
			var loaded NewVersion
			doc, err = store.Get(ctx, collectionName, "worker-789")
			Expect(err).NotTo(HaveOccurred())

			docBytes, err = json.Marshal(doc)
			Expect(err).NotTo(HaveOccurred())

			err = json.Unmarshal(docBytes, &loaded)
			Expect(err).NotTo(HaveOccurred())

			// Field renamed in struct but JSON tag unchanged
			Expect(loaded.ID).To(Equal("worker-789"))
			Expect(loaded.Name).To(Equal("Test Worker 3"))
			Expect(loaded.NewStatus).To(Equal("active"))
		})
	})
})
```

**Step 2: Run tests to verify baseline**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
go test -v ./pkg/cse/storage/... -run TestStorageMigrationScenarios
```

**Expected output:** All 3 migration scenarios pass

**Step 3: Commit**

```bash
git add pkg/cse/storage/migration_test.go
git commit -m "test: add storage migration scenario tests

Establishes baseline for safe struct evolution:
- Field addition (new fields get zero values)
- Field removal (old fields ignored)
- Field rename (via JSON tags)

These tests prove that changing struct definitions
is safe when using JSON serialization.

Part of registry elimination refactoring."
```

---

### Task 1.2: Document Current API Surface

**File to update:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/README.md`

**Step 1: Create or update README**

```markdown
# CSE Storage API Reference

## Current API Surface (Before Registry Elimination)

### Legacy APIs (workerType parameter required)

These APIs require runtime registry lookup:

**SaveObserved(ctx, workerType, id, observed) (changed bool, err error)**
- File: `triangular.go:510`
- Looks up collection name via registry.GetTriangularCollections(workerType)
- Returns true if data changed (delta checking)
- Auto-injects CSE metadata (_sync_id, _version, timestamps)

**LoadObserved(ctx, workerType, id) (interface{}, error)**
- File: `triangular.go:570`
- Returns Document (map[string]interface{})
- Caller must type assert result

**SaveDesired(ctx, workerType, id, desired) error**
- File: `triangular.go:266`
- Similar to SaveObserved but for desired state

**LoadDesired(ctx, workerType, id) (interface{}, error)**
- File: `triangular.go:320`
- Returns Document

### Generic APIs (type parameter, no workerType)

These APIs use reflection for collection name:

**SaveObservedTyped[T](ts, ctx, id, observed T) (bool, error)**
- File: `triangular.go:1119`
- Type parameter T determines workerType via reflection
- Currently still calls legacy SaveObserved internally
- Goal: Make fully independent

**LoadObservedTyped[T](ts, ctx, id) (T, error)**
- File: `triangular.go:1064`
- Returns strongly-typed result
- Currently calls legacy LoadObserved internally

### Call Sites

**Supervisor**
- `/pkg/fsmv2/supervisor/supervisor.go:532` - SaveObserved for snapshot creation
- Auto-registers collections in NewSupervisor() lines 344-391

**Collector**
- `/pkg/fsmv2/supervisor/collection/collector.go:221` - SaveObserved for observed state

## Migration Path

Phase 5 will convert all call sites from legacy APIs to generic APIs.

## CSE Metadata Fields

All roles use these fields (hardcoded in Phase 2):

**Identity:**
- _sync_id (int64)
- _version (int64, always 1)
- _created_at (timestamp)

**Desired:**
- _sync_id (int64)
- _version (int64, increments on update)
- _created_at (timestamp)
- _updated_at (timestamp)

**Observed:**
- _sync_id (int64)
- _version (int64, increments on update)
- _created_at (timestamp)
- _updated_at (timestamp)
```

**Step 2: Commit**

```bash
git add pkg/cse/storage/README.md
git commit -m "docs: document current CSE storage API surface

Comprehensive reference for:
- Legacy APIs (SaveObserved, LoadObserved, etc.)
- Generic APIs (SaveObservedTyped[T], etc.)
- All call sites in codebase
- CSE metadata field conventions

Baseline documentation before registry elimination."
```

---

### Task 1.3: Baseline Performance Benchmarks

**File to update:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular_test.go`

**Step 1: Add benchmark functions**

Find the end of the test file and add:

```go
// Benchmarks for SaveObserved delta checking

func BenchmarkSaveObservedNoChange(b *testing.B) {
	store := persistence.NewInMemoryStore()
	registry := NewRegistry()
	ts := NewTriangularStore(store, registry)
	ctx := context.Background()

	// Register collection
	registry.Register(&CollectionMetadata{
		Name:       "benchmark_observed",
		WorkerType: "benchmark",
		Role:       RoleObserved,
		CSEFields:  []string{FieldSyncID, FieldVersion, FieldCreatedAt, FieldUpdatedAt},
	})

	// Create initial document
	doc := persistence.Document{
		"id":     "worker-123",
		"status": "running",
		"cpu":    45.2,
	}

	_, _ = ts.SaveObserved(ctx, "benchmark", "worker-123", doc)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Same data - should detect no change
		_, _ = ts.SaveObserved(ctx, "benchmark", "worker-123", doc)
	}
}

func BenchmarkSaveObservedWithChange(b *testing.B) {
	store := persistence.NewInMemoryStore()
	registry := NewRegistry()
	ts := NewTriangularStore(store, registry)
	ctx := context.Background()

	registry.Register(&CollectionMetadata{
		Name:       "benchmark_observed",
		WorkerType: "benchmark",
		Role:       RoleObserved,
		CSEFields:  []string{FieldSyncID, FieldVersion, FieldCreatedAt, FieldUpdatedAt},
	})

	// Create initial document
	doc := persistence.Document{
		"id":     "worker-123",
		"status": "running",
		"cpu":    45.2,
	}

	_, _ = ts.SaveObserved(ctx, "benchmark", "worker-123", doc)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Changing data each iteration
		doc["cpu"] = float64(i) * 0.1
		_, _ = ts.SaveObserved(ctx, "benchmark", "worker-123", doc)
	}
}

func BenchmarkRegistryLookup(b *testing.B) {
	registry := NewRegistry()
	registry.Register(&CollectionMetadata{
		Name:       "benchmark_observed",
		WorkerType: "benchmark",
		Role:       RoleObserved,
		CSEFields:  []string{FieldSyncID, FieldVersion, FieldCreatedAt, FieldUpdatedAt},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = registry.GetTriangularCollections("benchmark")
	}
}
```

**Step 2: Run benchmarks and record baseline**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
go test -bench=. -benchmem ./pkg/cse/storage/... > /tmp/baseline_benchmarks.txt
cat /tmp/baseline_benchmarks.txt
```

**Expected output:**
```
BenchmarkSaveObservedNoChange-8          500000    2500 ns/op    800 B/op   15 allocs/op
BenchmarkSaveObservedWithChange-8        300000    4000 ns/op   1200 B/op   20 allocs/op
BenchmarkRegistryLookup-8              5000000     300 ns/op    150 B/op    3 allocs/op
```

**Step 3: Save benchmark results**

```bash
cp /tmp/baseline_benchmarks.txt /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/docs/plans/baseline_benchmarks.txt
```

**Step 4: Commit**

```bash
git add pkg/cse/storage/triangular_test.go docs/plans/baseline_benchmarks.txt
git commit -m "perf: add baseline benchmarks for storage operations

Measures:
- SaveObserved with no data change (delta check cost)
- SaveObserved with data change (full update cost)
- Registry lookup overhead (will be eliminated)

Baseline saved to docs/plans/baseline_benchmarks.txt
for comparison after registry elimination."
```

---

### Phase 1 Verification

Run all tests:
```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
make test
```

Expected: All tests pass, no focused tests

**Deliverables:**
- ✅ migration_test.go with 3 scenarios
- ✅ README.md documenting API surface
- ✅ Baseline benchmarks recorded

---

## Phase 2: Introduce Constants (1 day)

### Goal
Hardcode CSE metadata fields as constants, prove registry lookups can be replaced.

### Task 2.1: Create Constants File

**File to create:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/constants.go`

**Step 1: Write constants file**

```go
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

// CSE Metadata Field Constants
//
// DESIGN DECISION: Hardcode CSE fields per role (discovered all roles use identical fields)
// WHY: Eliminates need for registry lookup to determine which fields to inject.
//      All roles follow same CSE convention: _sync_id, _version, timestamps.
//
// INSPIRED BY: Investigation that revealed CSEFields are identical across all worker types.
//              Registry was providing runtime lookup for compile-time constants.
//
// TRADE-OFF: Less flexible than registry, but we don't need flexibility here.
//            CSE metadata conventions are stable and don't vary per worker type.

// Identity CSE fields (immutable, created once)
var IdentityCSEFields = []string{
	FieldSyncID,    // Global sync version for delta queries
	FieldVersion,   // Always 1 for identity (never changes)
	FieldCreatedAt, // Timestamp of identity creation
}

// Desired CSE fields (user intent, versioned for optimistic locking)
var DesiredCSEFields = []string{
	FieldSyncID,    // Global sync version
	FieldVersion,   // Increments on each update (optimistic locking)
	FieldCreatedAt, // Timestamp of first save
	FieldUpdatedAt, // Timestamp of last save
}

// Observed CSE fields (system reality, frequently updated)
var ObservedCSEFields = []string{
	FieldSyncID,    // Global sync version
	FieldVersion,   // Increments on each update
	FieldCreatedAt, // Timestamp of first observation
	FieldUpdatedAt, // Timestamp of last observation
}

// getCSEFields returns the appropriate CSE fields for a given role.
// This replaces registry lookup with compile-time constant selection.
func getCSEFields(role Role) []string {
	switch role {
	case RoleIdentity:
		return IdentityCSEFields
	case RoleDesired:
		return DesiredCSEFields
	case RoleObserved:
		return ObservedCSEFields
	default:
		// Should never happen with typed enum
		panic("unknown role: " + role)
	}
}
```

**Step 2: Commit**

```bash
git add pkg/cse/storage/constants.go
git commit -m "refactor: introduce CSE metadata field constants

Hardcodes CSE fields per role (identity, desired, observed).
Investigation revealed registry was doing runtime lookup
for what are effectively compile-time constants.

No behavioral change - constants match existing registry values.

Next step: Replace registry.GetTriangularCollections() calls
with these constants."
```

---

### Task 2.2: Replace Registry Lookups in SaveObserved

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`

**Lines to change:** ~543-556 (inside SaveObserved function)

**Step 1: Find the filterCSEFields call**

Current code around line 543:
```go
// Filter CSE fields from observed data for comparison
observedMeta, _, _, err := ts.registry.GetTriangularCollections(workerType)
if err != nil {
	return false, fmt.Errorf("worker type %q not registered: %w", workerType, err)
}

existingFiltered := filterCSEFields(existing, observedMeta.CSEFields)
observedFiltered := filterCSEFields(observedDoc, observedMeta.CSEFields)
```

**Step 2: Replace with constants**

```go
// Filter CSE fields from observed data for comparison (using constants, not registry)
cseFields := getCSEFields(RoleObserved)
existingFiltered := filterCSEFields(existing, cseFields)
observedFiltered := filterCSEFields(observedDoc, cseFields)
```

**Step 3: Find the collection name lookup**

Current code around line 520:
```go
_, _, observedMeta, err := ts.registry.GetTriangularCollections(workerType)
if err != nil {
	return false, fmt.Errorf("worker type %q not registered: %w", workerType, err)
}

collectionName := observedMeta.Name
```

**Step 4: Replace with convention**

```go
// Collection name follows convention: {workerType}_observed
collectionName := workerType + "_observed"
```

**Step 5: Run tests**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
go test -v ./pkg/cse/storage/... -run TestSaveObserved
```

**Expected:** All SaveObserved tests pass

**Step 6: Commit**

```bash
git add pkg/cse/storage/triangular.go
git commit -m "refactor: use constants in SaveObserved, remove registry lookup

Changes in SaveObserved():
- Collection name from convention (workerType + '_observed')
- CSE fields from getCSEFields(RoleObserved) constant
- No more registry.GetTriangularCollections() call

No behavioral change - same CSE fields, same logic.

Performance impact: ~300ns faster per call (no registry map lookup)."
```

---

### Task 2.3: Replace Registry Lookups in LoadObserved

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`

**Lines to change:** ~570-580 (inside LoadObserved function)

**Step 1: Find the registry lookup**

Current code around line 570:
```go
func (ts *TriangularStore) LoadObserved(ctx context.Context, workerType string, id string) (interface{}, error) {
	_, _, observedMeta, err := ts.registry.GetTriangularCollections(workerType)
	if err != nil {
		return nil, fmt.Errorf("worker type %q not registered: %w", workerType, err)
	}

	doc, err := ts.store.Get(ctx, observedMeta.Name, id)
	// ...
}
```

**Step 2: Replace with convention**

```go
func (ts *TriangularStore) LoadObserved(ctx context.Context, workerType string, id string) (interface{}, error) {
	collectionName := workerType + "_observed"

	doc, err := ts.store.Get(ctx, collectionName, id)
	if err != nil {
		return nil, err
	}

	return doc, nil
}
```

**Step 3: Run tests**

```bash
go test -v ./pkg/cse/storage/... -run TestLoadObserved
```

**Step 4: Commit**

```bash
git add pkg/cse/storage/triangular.go
git commit -m "refactor: use convention in LoadObserved, remove registry lookup

Collection name derived from workerType + '_observed'.
No registry lookup needed.

No behavioral change."
```

---

### Task 2.4: Replace Registry Lookups in SaveDesired/LoadDesired

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`

**Lines to change:**
- SaveDesired: ~266-290
- LoadDesired: ~320-340

**Step 1: Update SaveDesired**

Find current code around line 272:
```go
_, desiredMeta, _, err := ts.registry.GetTriangularCollections(workerType)
if err != nil {
	return fmt.Errorf("worker type %q not registered: %w", workerType, err)
}

// Check if this is first save or update
_, err = ts.store.Get(ctx, desiredMeta.Name, id)
```

Replace with:
```go
collectionName := workerType + "_desired"

// Check if this is first save or update
_, err = ts.store.Get(ctx, collectionName, id)
```

**Step 2: Update LoadDesired**

Find current code around line 320:
```go
_, desiredMeta, _, err := ts.registry.GetTriangularCollections(workerType)
if err != nil {
	return nil, fmt.Errorf("worker type %q not registered: %w", workerType, err)
}

doc, err := ts.store.Get(ctx, desiredMeta.Name, id)
```

Replace with:
```go
collectionName := workerType + "_desired"

doc, err := ts.store.Get(ctx, collectionName, id)
```

**Step 3: Run tests**

```bash
go test -v ./pkg/cse/storage/... -run "TestSaveDesired|TestLoadDesired"
```

**Step 4: Commit**

```bash
git add pkg/cse/storage/triangular.go
git commit -m "refactor: use convention in SaveDesired/LoadDesired

Collection names from convention:
- Desired: workerType + '_desired'
- No registry lookups

No behavioral change."
```

---

### Phase 2 Verification

Run full test suite:
```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
make test
```

Run benchmarks to compare:
```bash
go test -bench=. -benchmem ./pkg/cse/storage/... > /tmp/phase2_benchmarks.txt
diff /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/docs/plans/baseline_benchmarks.txt /tmp/phase2_benchmarks.txt
```

Expected: Tests pass, benchmarks show ~300ns improvement (no registry lookup)

**Deliverables:**
- ✅ constants.go with CSE field definitions
- ✅ Legacy APIs no longer call registry.GetTriangularCollections()
- ✅ All tests pass
- ✅ Performance improved

---

## Phase 3: Make Generic APIs Independent (2 days)

### Goal
Rewrite generic APIs to work without calling legacy APIs internally.

### Task 3.1: Add DeriveCollectionName[T]() Helper

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`

**Location:** After DeriveWorkerType[T]() function (around line 960)

**Step 1: Add new helper function**

```go
// DeriveCollectionName derives the collection name for a given type and role.
//
// DESIGN DECISION: Use reflection to derive workerType, then apply naming convention
// WHY: Type parameter T already identifies the worker type uniquely.
//      Convention-based naming eliminates need for registry lookup.
//
// CONVENTION: {workerType}_{role}
//   - container_observed
//   - relay_desired
//   - communicator_identity
//
// INSPIRED BY: Rails convention-over-configuration, HTTP routing conventions.
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
//	collectionName := DeriveCollectionName[ContainerObservedState](RoleObserved)
//	// Returns: "container_observed"
func DeriveCollectionName[T any](role Role) string {
	workerType := DeriveWorkerType[T]()

	var suffix string
	switch role {
	case RoleIdentity:
		suffix = "_identity"
	case RoleDesired:
		suffix = "_desired"
	case RoleObserved:
		suffix = "_observed"
	default:
		panic("unknown role: " + role)
	}

	return workerType + suffix
}
```

**Step 2: Add unit test for helper**

In `triangular_test.go`, add:

```go
var _ = Describe("DeriveCollectionName", func() {
	It("should derive collection name for identity", func() {
		name := storage.DeriveCollectionName[ExampleObservedState](storage.RoleIdentity)
		Expect(name).To(Equal("example_identity"))
	})

	It("should derive collection name for desired", func() {
		name := storage.DeriveCollectionName[ExampleObservedState](storage.RoleDesired)
		Expect(name).To(Equal("example_desired"))
	})

	It("should derive collection name for observed", func() {
		name := storage.DeriveCollectionName[ExampleObservedState](storage.RoleObserved)
		Expect(name).To(Equal("example_observed"))
	})
})
```

**Step 3: Run tests**

```bash
go test -v ./pkg/cse/storage/... -run TestDeriveCollectionName
```

**Step 4: Commit**

```bash
git add pkg/cse/storage/triangular.go pkg/cse/storage/triangular_test.go
git commit -m "feat: add DeriveCollectionName[T]() helper

Generic helper to derive collection name from type parameter.
Uses DeriveWorkerType[T]() + role suffix.

Convention: {workerType}_{role}

Enables generic APIs to work without registry lookups."
```

---

### Task 3.2: Rewrite SaveObservedTyped[T]()

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`

**Lines to rewrite:** 1119-1151 (entire SaveObservedTyped function)

**Step 1: Read current implementation**

Current code around line 1119:
```go
func SaveObservedTyped[T any](ts *TriangularStore, ctx context.Context, id string, observed T) (bool, error) {
	workerType := DeriveWorkerType[T]()

	// Marshal to Document
	bytes, err := json.Marshal(observed)
	if err != nil {
		return false, fmt.Errorf("failed to marshal observed state: %w", err)
	}

	var doc persistence.Document
	if err := json.Unmarshal(bytes, &doc); err != nil {
		return false, fmt.Errorf("failed to unmarshal to Document: %w", err)
	}

	// Call legacy SaveObserved
	return ts.SaveObserved(ctx, workerType, id, doc)
}
```

**Step 2: Replace with independent implementation**

```go
// SaveObservedTyped saves observed state using generics (no registry dependency).
//
// DESIGN DECISION: Fully independent generic implementation
// WHY: Type parameter T provides all information needed:
//      - workerType from DeriveWorkerType[T]()
//      - collectionName from DeriveCollectionName[T](RoleObserved)
//      - CSE fields from getCSEFields(RoleObserved)
//
// DELTA CHECKING: Compares new data against existing data (excluding CSE fields).
// Returns true if business data changed, false if only CSE metadata updated.
//
// CSE metadata auto-injected:
//   - _sync_id: Incremented after successful save
//   - _version: Incremented if data changed
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
func SaveObservedTyped[T any](ts *TriangularStore, ctx context.Context, id string, observed T) (bool, error) {
	collectionName := DeriveCollectionName[T](RoleObserved)

	// Marshal struct to Document
	bytes, err := json.Marshal(observed)
	if err != nil {
		return false, fmt.Errorf("failed to marshal observed state: %w", err)
	}

	var observedDoc persistence.Document
	if err := json.Unmarshal(bytes, &observedDoc); err != nil {
		return false, fmt.Errorf("failed to unmarshal to Document: %w", err)
	}

	// Add ID if not present (convenience)
	if _, ok := observedDoc["id"]; !ok {
		observedDoc["id"] = id
	}

	// Load existing document for delta checking
	existing, err := ts.store.Get(ctx, collectionName, id)
	isNew := err != nil && errors.Is(err, persistence.ErrNotFound)

	if err != nil && !isNew {
		return false, fmt.Errorf("failed to load existing observed state: %w", err)
	}

	// Delta check: Compare business data (exclude CSE fields)
	cseFields := getCSEFields(RoleObserved)
	changed := false

	if isNew {
		changed = true // New document = always changed
	} else {
		existingFiltered := filterCSEFields(existing, cseFields)
		observedFiltered := filterCSEFields(observedDoc, cseFields)

		// Deep equality check
		existingBytes, _ := json.Marshal(existingFiltered)
		observedBytes, _ := json.Marshal(observedFiltered)
		changed = string(existingBytes) != string(observedBytes)
	}

	// Inject CSE metadata
	ts.injectMetadata(observedDoc, RoleObserved, isNew)

	// Save or update
	if isNew {
		_, err = ts.store.Insert(ctx, collectionName, observedDoc)
	} else {
		// Increment version only if data changed
		if changed {
			if version, ok := observedDoc[FieldVersion].(int64); ok {
				observedDoc[FieldVersion] = version + 1
			}
		}
		err = ts.store.Update(ctx, collectionName, id, observedDoc)
	}

	if err != nil {
		return false, fmt.Errorf("failed to save observed state: %w", err)
	}

	// Increment sync ID after successful save
	syncID := ts.syncID.Add(1)
	observedDoc[FieldSyncID] = syncID

	// Update with sync ID
	err = ts.store.Update(ctx, collectionName, id, observedDoc)
	if err != nil {
		// Non-fatal: document saved, but sync ID update failed
		return changed, fmt.Errorf("failed to update sync ID: %w", err)
	}

	return changed, nil
}
```

**Step 3: Run tests**

```bash
go test -v ./pkg/cse/storage/... -run TestSaveObservedTyped
go test -v ./pkg/cse/storage/... -run TestGenericAPIDeltaChecking
```

**Step 4: Commit**

```bash
git add pkg/cse/storage/triangular.go
git commit -m "refactor: rewrite SaveObservedTyped[T]() without legacy API

Now fully independent:
- Collection name from DeriveCollectionName[T](RoleObserved)
- CSE fields from getCSEFields(RoleObserved)
- No calls to legacy SaveObserved()

Preserves delta checking, CSE metadata injection, sync ID logic.

No behavioral change - same semantics, different implementation."
```

---

### Task 3.3: Rewrite LoadObservedTyped[T]()

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`

**Lines to rewrite:** 1064-1090

**Step 1: Read current implementation**

Current code around line 1064:
```go
func LoadObservedTyped[T any](ts *TriangularStore, ctx context.Context, id string) (T, error) {
	var result T
	workerType := DeriveWorkerType[T]()

	doc, err := ts.LoadObserved(ctx, workerType, id)
	if err != nil {
		return result, err
	}

	bytes, err := json.Marshal(doc)
	if err != nil {
		return result, fmt.Errorf("failed to marshal document: %w", err)
	}

	if err := json.Unmarshal(bytes, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal to type: %w", err)
	}

	return result, nil
}
```

**Step 2: Replace with independent implementation**

```go
// LoadObservedTyped loads observed state using generics (no registry dependency).
//
// DESIGN DECISION: Fully independent generic implementation
// WHY: Type parameter T provides collection name via DeriveCollectionName[T].
//      No registry lookup needed.
//
// Parameters:
//   - ts: TriangularStore instance
//   - ctx: Cancellation context
//   - id: Unique worker identifier
//
// Returns:
//   - Observed state struct (strongly typed)
//   - error: ErrNotFound if worker doesn't exist
func LoadObservedTyped[T any](ts *TriangularStore, ctx context.Context, id string) (T, error) {
	var result T
	collectionName := DeriveCollectionName[T](RoleObserved)

	// Load from persistence
	doc, err := ts.store.Get(ctx, collectionName, id)
	if err != nil {
		return result, err
	}

	// Unmarshal Document to typed struct
	bytes, err := json.Marshal(doc)
	if err != nil {
		return result, fmt.Errorf("failed to marshal document: %w", err)
	}

	if err := json.Unmarshal(bytes, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal to type %T: %w", result, err)
	}

	return result, nil
}
```

**Step 3: Run tests**

```bash
go test -v ./pkg/cse/storage/... -run TestLoadObservedTyped
```

**Step 4: Commit**

```bash
git add pkg/cse/storage/triangular.go
git commit -m "refactor: rewrite LoadObservedTyped[T]() without legacy API

Now fully independent:
- Collection name from DeriveCollectionName[T](RoleObserved)
- Direct store.Get() call
- No calls to legacy LoadObserved()

No behavioral change."
```

---

### Task 3.4: Rewrite SaveDesiredTyped[T]() and LoadDesiredTyped[T]()

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`

**Lines to rewrite:**
- SaveDesiredTyped: 1030-1062
- LoadDesiredTyped: 981-1007

**Step 1: Rewrite SaveDesiredTyped**

Similar pattern to SaveObservedTyped, but for desired state (no delta checking).

```go
func SaveDesiredTyped[T any](ts *TriangularStore, ctx context.Context, id string, desired T) error {
	collectionName := DeriveCollectionName[T](RoleDesired)

	// Marshal to Document
	bytes, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("failed to marshal desired state: %w", err)
	}

	var desiredDoc persistence.Document
	if err := json.Unmarshal(bytes, &desiredDoc); err != nil {
		return fmt.Errorf("failed to unmarshal to Document: %w", err)
	}

	// Add ID if not present
	if _, ok := desiredDoc["id"]; !ok {
		desiredDoc["id"] = id
	}

	// Check if new or update
	_, err = ts.store.Get(ctx, collectionName, id)
	isNew := err != nil && errors.Is(err, persistence.ErrNotFound)

	if err != nil && !isNew {
		return fmt.Errorf("failed to check existing desired state: %w", err)
	}

	// Inject CSE metadata
	ts.injectMetadata(desiredDoc, RoleDesired, isNew)

	// Save or update
	if isNew {
		_, err = ts.store.Insert(ctx, collectionName, desiredDoc)
	} else {
		// Increment version on update
		if version, ok := desiredDoc[FieldVersion].(int64); ok {
			desiredDoc[FieldVersion] = version + 1
		}
		err = ts.store.Update(ctx, collectionName, id, desiredDoc)
	}

	if err != nil {
		return fmt.Errorf("failed to save desired state: %w", err)
	}

	// Increment sync ID after successful save
	syncID := ts.syncID.Add(1)
	desiredDoc[FieldSyncID] = syncID

	// Update with sync ID
	err = ts.store.Update(ctx, collectionName, id, desiredDoc)
	if err != nil {
		return fmt.Errorf("failed to update sync ID: %w", err)
	}

	return nil
}
```

**Step 2: Rewrite LoadDesiredTyped**

```go
func LoadDesiredTyped[T any](ts *TriangularStore, ctx context.Context, id string) (T, error) {
	var result T
	collectionName := DeriveCollectionName[T](RoleDesired)

	doc, err := ts.store.Get(ctx, collectionName, id)
	if err != nil {
		return result, err
	}

	bytes, err := json.Marshal(doc)
	if err != nil {
		return result, fmt.Errorf("failed to marshal document: %w", err)
	}

	if err := json.Unmarshal(bytes, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal to type %T: %w", result, err)
	}

	return result, nil
}
```

**Step 3: Run tests**

```bash
go test -v ./pkg/cse/storage/... -run "TestSaveDesiredTyped|TestLoadDesiredTyped"
```

**Step 4: Commit**

```bash
git add pkg/cse/storage/triangular.go
git commit -m "refactor: rewrite SaveDesiredTyped[T] and LoadDesiredTyped[T]

Both now fully independent from legacy APIs:
- Collection names from DeriveCollectionName[T](RoleDesired)
- Direct store operations
- CSE metadata injection

No behavioral change."
```

---

### Phase 3 Verification

Run full test suite:
```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
make test
```

Verify generic APIs work without legacy:
```bash
go test -v ./pkg/cse/storage/... -run "TestSaveObservedTyped|TestLoadObservedTyped|TestSaveDesiredTyped|TestLoadDesiredTyped"
```

Expected: All tests pass

**Deliverables:**
- ✅ DeriveCollectionName[T]() helper
- ✅ SaveObservedTyped[T]() independent
- ✅ LoadObservedTyped[T]() independent
- ✅ SaveDesiredTyped[T]() independent
- ✅ LoadDesiredTyped[T]() independent
- ✅ All tests pass

---

## Phase 4: Deprecate Legacy APIs (1 day)

### Goal
Mark legacy APIs as deprecated, create migration guide, document all call sites.

### Task 4.1: Add Deprecation Comments

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`

**Step 1: Add deprecation to SaveObserved (line 510)**

Before:
```go
// SaveObserved stores observed state with delta checking.
```

After:
```go
// SaveObserved stores observed state with delta checking.
//
// DEPRECATED: Use SaveObservedTyped[T]() instead for type safety and no registry dependency.
// This method will be removed in a future version.
//
// Migration guide: docs/plans/MIGRATION.md
//
// Example:
//   // Old (deprecated):
//   changed, err := ts.SaveObserved(ctx, "container", id, doc)
//
//   // New (recommended):
//   changed, err := storage.SaveObservedTyped[ContainerObservedState](ts, ctx, id, observed)
```

**Step 2: Add deprecation to LoadObserved (line 570)**

```go
// LoadObserved retrieves worker observed state.
//
// DEPRECATED: Use LoadObservedTyped[T]() instead for type safety and no registry dependency.
// This method will be removed in a future version.
//
// Migration guide: docs/plans/MIGRATION.md
```

**Step 3: Add deprecation to SaveDesired (line 266)**

```go
// SaveDesired stores user intent/configuration.
//
// DEPRECATED: Use SaveDesiredTyped[T]() instead.
// Migration guide: docs/plans/MIGRATION.md
```

**Step 4: Add deprecation to LoadDesired (line 320)**

```go
// LoadDesired retrieves desired state.
//
// DEPRECATED: Use LoadDesiredTyped[T]() instead.
// Migration guide: docs/plans/MIGRATION.md
```

**Step 5: Commit**

```bash
git add pkg/cse/storage/triangular.go
git commit -m "docs: deprecate legacy storage APIs

Marked as deprecated:
- SaveObserved(ctx, workerType, id, doc)
- LoadObserved(ctx, workerType, id)
- SaveDesired(ctx, workerType, id, doc)
- LoadDesired(ctx, workerType, id)

Use generic typed APIs instead:
- SaveObservedTyped[T](ts, ctx, id, observed)
- LoadObservedTyped[T](ts, ctx, id)

See MIGRATION.md for migration examples."
```

---

### Task 4.2: Create Migration Guide

**File to create:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/MIGRATION.md`

**Step 1: Write migration guide**

```markdown
# Storage API Migration Guide

## Overview

Legacy storage APIs (SaveObserved, LoadObserved, SaveDesired, LoadDesired) are deprecated and will be removed.

**Why migrate:**
- Type safety (compile-time checks instead of runtime assertions)
- No registry dependency (simpler, faster)
- Cleaner API (type parameter conveys intent)

**Timeline:**
- Phase 4 (current): Legacy APIs deprecated
- Phase 5: All internal code migrated
- Phase 6: Legacy APIs removed

## Migration Examples

### Example 1: SaveObserved with Document

**Before (deprecated):**
```go
observed := persistence.Document{
    "id":     workerID,
    "status": "running",
    "cpu":    45.2,
}
changed, err := ts.SaveObserved(ctx, "container", workerID, observed)
```

**After (recommended):**
```go
// Define typed struct (if not already defined)
type ContainerObservedState struct {
    ID     string  `json:"id"`
    Status string  `json:"status"`
    CPU    float64 `json:"cpu"`
}

// Use typed API
observed := ContainerObservedState{
    ID:     workerID,
    Status: "running",
    CPU:    45.2,
}
changed, err := storage.SaveObservedTyped[ContainerObservedState](ts, ctx, workerID, observed)
```

### Example 2: LoadObserved with Type Assertion

**Before (deprecated):**
```go
doc, err := ts.LoadObserved(ctx, "relay", workerID)
if err != nil {
    return err
}

// Manual type assertion (error-prone!)
statusRaw, ok := doc["status"]
if !ok {
    return errors.New("status field missing")
}
status, ok := statusRaw.(string)
if !ok {
    return errors.New("status is not string")
}
```

**After (recommended):**
```go
type RelayObservedState struct {
    ID     string `json:"id"`
    Status string `json:"status"`
}

observed, err := storage.LoadObservedTyped[RelayObservedState](ts, ctx, workerID)
if err != nil {
    return err
}

// Type-safe field access!
status := observed.Status
```

### Example 3: SaveDesired

**Before (deprecated):**
```go
desired := persistence.Document{
    "id":     workerID,
    "config": "production",
}
err := ts.SaveDesired(ctx, "communicator", workerID, desired)
```

**After (recommended):**
```go
type CommunicatorDesiredState struct {
    ID     string `json:"id"`
    Config string `json:"config"`
}

desired := CommunicatorDesiredState{
    ID:     workerID,
    Config: "production",
}
err := storage.SaveDesiredTyped[CommunicatorDesiredState](ts, ctx, workerID, desired)
```

### Example 4: LoadDesired

**Before (deprecated):**
```go
doc, err := ts.LoadDesired(ctx, "parent", workerID)
if err != nil {
    return err
}

// Manual extraction
configRaw, _ := doc["config"]
config, _ := configRaw.(string)
```

**After (recommended):**
```go
type ParentDesiredState struct {
    ID     string `json:"id"`
    Config string `json:"config"`
}

desired, err := storage.LoadDesiredTyped[ParentDesiredState](ts, ctx, workerID)
if err != nil {
    return err
}

config := desired.Config // Type-safe!
```

## Common Pitfalls

### Pitfall 1: Struct Naming Convention

**Type parameter determines workerType via reflection.**

```go
// WRONG: Struct name doesn't match convention
type MyCustomState struct { ... }
storage.SaveObservedTyped[MyCustomState](...)
// Derives workerType = "mycustom" (wrong!)

// CORRECT: Follow convention
type ContainerObservedState struct { ... }
storage.SaveObservedTyped[ContainerObservedState](...)
// Derives workerType = "container" (correct!)
```

**Naming convention:**
- `{WorkerType}ObservedState` for observed
- `{WorkerType}DesiredState` for desired
- `{WorkerType}IdentityState` for identity

### Pitfall 2: Forgetting ID Field

Generic APIs expect `id` field in struct:

```go
// WRONG: Missing ID field
type ObservedState struct {
    Status string `json:"status"`
}

// CORRECT: Include ID field
type ObservedState struct {
    ID     string `json:"id"`
    Status string `json:"status"`
}
```

### Pitfall 3: Mixed Legacy and Generic APIs

Don't mix old and new APIs for same worker:

```go
// WRONG: Mixing APIs
storage.SaveObservedTyped[ContainerObservedState](ts, ctx, id, obs)
doc, _ := ts.LoadObserved(ctx, "container", id) // Legacy API!

// CORRECT: Use generics consistently
storage.SaveObservedTyped[ContainerObservedState](ts, ctx, id, obs)
obs, _ := storage.LoadObservedTyped[ContainerObservedState](ts, ctx, id)
```

## Struct Definition Best Practices

### Reuse Existing Structs

Many worker types already have typed structs defined:

```go
// Check if already defined in worker package:
import "github.com/.../fsmv2/workers/container"

// Use existing struct
observed, err := storage.LoadObservedTyped[container.ObservedState](ts, ctx, id)
```

### Define New Structs

If no struct exists, define in worker package:

```go
// File: pkg/fsmv2/workers/myworker/models.go

package myworker

type ObservedState struct {
    ID              string    `json:"id"`
    Status          string    `json:"status"`
    LastHeartbeat   time.Time `json:"lastHeartbeat"`
}

type DesiredState struct {
    ID     string `json:"id"`
    Config string `json:"config"`
}
```

### JSON Tags Required

Always add JSON tags matching Document field names:

```go
// WRONG: No JSON tags
type ObservedState struct {
    ID     string
    Status string
}

// CORRECT: JSON tags match Document keys
type ObservedState struct {
    ID     string `json:"id"`
    Status string `json:"status"`
}
```

## Testing Migration

After migrating, verify:

```bash
# Compile check
go build ./pkg/fsmv2/workers/...

# Run worker tests
go test -v ./pkg/fsmv2/workers/myworker/...

# Run integration tests
go test -v ./pkg/fsmv2/integration/... -run TestMyWorker
```

## Questions?

- Check examples in Phase 5 of this implementation plan
- Review existing generic API usage in tests: `pkg/cse/storage/triangular_test.go`
- See worker struct definitions: `pkg/fsmv2/workers/*/models.go`
```

**Step 2: Commit**

```bash
git add pkg/cse/storage/MIGRATION.md
git commit -m "docs: create storage API migration guide

Comprehensive guide with:
- 4 migration examples (SaveObserved, LoadObserved, SaveDesired, LoadDesired)
- 3 common pitfalls (naming, ID field, mixing APIs)
- Struct definition best practices
- Testing checklist

For engineers migrating from legacy to generic APIs."
```

---

### Task 4.3: Document All Call Sites

**Step 1: Find all legacy API call sites**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

# SaveObserved calls
grep -rn "\.SaveObserved(" pkg/fsmv2 --include="*.go" | grep -v "_test.go" > /tmp/save_observed_calls.txt

# LoadObserved calls
grep -rn "\.LoadObserved(" pkg/fsmv2 --include="*.go" | grep -v "_test.go" > /tmp/load_observed_calls.txt

# SaveDesired calls
grep -rn "\.SaveDesired(" pkg/fsmv2 --include="*.go" | grep -v "_test.go" > /tmp/save_desired_calls.txt

# LoadDesired calls
grep -rn "\.LoadDesired(" pkg/fsmv2 --include="*.go" | grep -v "_test.go" > /tmp/load_desired_calls.txt

# Combine results
cat /tmp/*_calls.txt > /tmp/all_legacy_calls.txt
cat /tmp/all_legacy_calls.txt
```

**Step 2: Create call site inventory**

```bash
cat > /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/docs/plans/legacy_api_call_sites.md << 'EOF'
# Legacy API Call Sites Inventory

Generated: 2025-01-13

## SaveObserved Calls

1. `/pkg/fsmv2/supervisor/supervisor.go:532`
   - Context: Supervisor creating snapshot
   - Worker type: Dynamic (s.workerType)
   - Data: observedDoc (Document)

2. `/pkg/fsmv2/supervisor/collection/collector.go:221`
   - Context: Collector saving observed state
   - Worker type: Dynamic (c.config.WorkerType)
   - Data: observed (interface{})

## LoadObserved Calls

(Currently none found - good!)

## SaveDesired Calls

(To be documented during Phase 5 migration)

## LoadDesired Calls

(To be documented during Phase 5 migration)

## Migration Priority

1. **High Priority**: Supervisor (line 532) - Called on every snapshot
2. **High Priority**: Collector (line 221) - Called on every observation
3. **Medium Priority**: Desired state operations (less frequent)

## Migration Notes

- Supervisor uses dynamic workerType (passed via config)
- Collector uses dynamic workerType (passed via config)
- Both need generic type parameter determined at compile time
- Solution: Use type registry to map workerType to Go type
EOF
```

**Step 3: Commit**

```bash
git add docs/plans/legacy_api_call_sites.md
git commit -m "docs: inventory legacy API call sites

Found 2 SaveObserved call sites:
- supervisor.go:532 (snapshot creation)
- collector.go:221 (observed state save)

Priority order for Phase 5 migration documented."
```

---

### Phase 4 Verification

Check deprecation comments added:
```bash
grep -A5 "DEPRECATED" /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go
```

Expected: 4 deprecation comments (SaveObserved, LoadObserved, SaveDesired, LoadDesired)

**Deliverables:**
- ✅ Deprecation comments on legacy APIs
- ✅ MIGRATION.md with examples and pitfalls
- ✅ Call site inventory documented

---

## Phase 5: Migrate All Call Sites (5-7 days)

### Goal
Convert all legacy API calls to generic APIs. This is the critical phase.

### Task 5.1: Migrate Supervisor (supervisor.go:532)

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/supervisor/supervisor.go`

**Challenge:** Supervisor uses dynamic workerType (passed via config), but generics need compile-time type.

**Solution:** Use TypeRegistry to dispatch to generic function.

**Step 1: Add type registry method to Supervisor struct**

Around line 140 (in Supervisor struct definition):
```go
type Supervisor struct {
	workerType            string
	workers               map[string]*WorkerContext
	mu                    sync.RWMutex
	store                 *storage.TriangularStore
	// ... other fields

	// Add this field:
	observedTypeRegistry  map[string]reflect.Type // Maps workerType to Go type
}
```

**Step 2: Initialize registry in NewSupervisor**

Around line 393 (in NewSupervisor return statement):
```go
return &Supervisor{
	workerType:            cfg.WorkerType,
	workers:               make(map[string]*WorkerContext),
	mu:                    sync.RWMutex{},
	store:                 cfg.Store,
	// ... other fields

	// Initialize registry:
	observedTypeRegistry:  make(map[string]reflect.Type),
}
```

**Step 3: Add RegisterObservedType method**

After NewSupervisor function (around line 420):
```go
// RegisterObservedType registers the Go type for this supervisor's observed state.
//
// This enables the Supervisor to use generic storage APIs while maintaining
// dynamic workerType configuration.
//
// Example:
//   supervisor := NewSupervisor(cfg)
//   supervisor.RegisterObservedType(ContainerObservedState{})
func (s *Supervisor) RegisterObservedType(example interface{}) {
	t := reflect.TypeOf(example)
	s.observedTypeRegistry[s.workerType] = t
}
```

**Step 4: Create generic save helper**

After RegisterObservedType:
```go
// saveObservedGeneric saves observed state using the registered type.
func (s *Supervisor) saveObservedGeneric(ctx context.Context, id string, observed interface{}) (bool, error) {
	t, ok := s.observedTypeRegistry[s.workerType]
	if !ok {
		// Fallback to legacy API if type not registered
		return s.store.SaveObserved(ctx, s.workerType, id, observed)
	}

	// Use reflection to call SaveObservedTyped with correct type parameter
	// This is a bridge during migration - will be simplified in Phase 6
	bytes, err := json.Marshal(observed)
	if err != nil {
		return false, err
	}

	observedValue := reflect.New(t).Interface()
	if err := json.Unmarshal(bytes, observedValue); err != nil {
		return false, err
	}

	// Call SaveObservedTyped via reflection (temporary during migration)
	// TODO Phase 6: Remove this reflection and use direct generic call
	workerType := storage.DeriveWorkerTypeFromValue(observedValue)
	collectionName := workerType + "_observed"

	// Direct implementation (avoiding reflection for now)
	observedDoc, ok := observed.(persistence.Document)
	if !ok {
		return false, errors.New("observed must be Document during migration")
	}

	return storage.SaveObservedTyped[persistence.Document](s.store, ctx, id, observedDoc)
}
```

**WAIT - This approach is too complex. Let me reconsider.**

**Better Solution:** Keep legacy API during Phase 5, defer supervisor migration to Phase 6 when we have more context.

**Revised Step 1: Skip supervisor for now**

The Supervisor uses runtime polymorphism (dynamic workerType). This is the hardest migration.

**Decision:** Migrate simpler call sites first (Collector), leave Supervisor for last.

---

### Task 5.1 (Revised): Migrate Collector (collector.go:221)

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/supervisor/collection/collector.go`

**Current code around line 221:**
```go
changed, err := c.config.Store.SaveObserved(ctx, c.config.WorkerType, c.config.Identity.ID, observed)
```

**Challenge:** Collector also uses dynamic workerType.

**Analysis:** After reviewing the codebase, I realize the call sites are more complex than expected. Both use dynamic dispatch.

**Revised Strategy for Phase 5:**

1. **Document that Supervisor and Collector require architectural change**
2. **Focus migration on simpler call sites first** (if any exist)
3. **Create architectural RFC for handling dynamic dispatch**

Let me search for other call sites:

```bash
# Search entire codebase (not just fsmv2)
grep -rn "\.SaveObserved(" /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg --include="*.go" | grep -v "_test.go" | grep -v "// "
```

**Reality Check:** If ALL call sites use dynamic workerType, then we need a different migration strategy.

**Option A:** Keep legacy APIs alongside generic APIs permanently (dual API system)
**Option B:** Use interface{} + type assertions in Supervisor/Collector
**Option C:** Generate typed wrappers at build time (code generation)
**Option D:** Use TypeRegistry pattern from Phase 3 investigation

**Recommendation:** Given the subagent investigation found "Registry system is unnecessary", but the actual call sites all use dynamic dispatch, we should:

1. **Keep generic APIs as the primary API** (for direct usage)
2. **Keep legacy APIs for dynamic dispatch cases** (Supervisor, Collector)
3. **Document this as intentional dual system** for different use cases

This means **Phase 5 is actually simpler** - we just ensure no NEW code uses legacy APIs.

**Revised Phase 5 Plan:**

---

### Task 5.1 (Re-Revised): Document Dynamic Dispatch Requirement

**File to create:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/docs/plans/dynamic_dispatch_analysis.md`

```markdown
# Dynamic Dispatch Analysis

## Finding

All current call sites (Supervisor, Collector) use **runtime polymorphism** - they don't know the worker type at compile time.

## Call Site Analysis

### Supervisor.go:532
```go
_, err = s.store.SaveObserved(ctx, s.workerType, identity.ID, observedDoc)
```

- `s.workerType` is `string` (runtime value)
- Could be "container", "relay", "communicator", etc.
- Determined by config, not compile time

### Collector.go:221
```go
changed, err := c.config.Store.SaveObserved(ctx, c.config.WorkerType, c.config.Identity.ID, observed)
```

- Same pattern: `c.config.WorkerType` is runtime string
- Generic type parameter can't be determined at compile time

## Design Decision

**Keep dual API system:**

1. **Generic APIs** - For direct usage with known types at compile time
   - Example: Worker action functions know their own type
   - `storage.SaveObservedTyped[ContainerObservedState](ts, ctx, id, obs)`

2. **Legacy APIs** - For dynamic dispatch (Supervisor, Collector)
   - Example: Supervisor orchestrates multiple worker types
   - `ts.SaveObserved(ctx, workerType, id, observed)`

## Why This Is Correct

The subagent investigation found "registry is unnecessary for generic APIs" - TRUE.

But it didn't find "all call sites can use generic APIs" - FALSE.

**Supervisor and Collector are frameworks** - they work with multiple worker types dynamically. They NEED runtime dispatch.

**Worker implementations** know their type at compile time - they SHOULD use generic APIs.

## Migration Strategy (Revised)

Phase 5: **Don't migrate Supervisor/Collector** - they correctly use legacy APIs.

Phase 6: **Remove registry system, but keep legacy APIs** - they use conventions instead.

This is an architectural feature, not technical debt.

## Future: Type-Safe Dynamic Dispatch

If we want type safety for Supervisor later, options:

1. **Type Registry Pattern** - Map workerType string → reflect.Type
2. **Code Generation** - Generate typed wrappers at build time
3. **Interface Abstraction** - Define Worker interface, use vtables

But this is future work - not required for registry elimination.
```

**Step 1: Commit analysis**

```bash
git add docs/plans/dynamic_dispatch_analysis.md
git commit -m "docs: analyze dynamic dispatch requirements

Finding: Supervisor and Collector correctly use legacy APIs
because they work with multiple worker types at runtime.

Generic APIs are for compile-time known types.
Legacy APIs are for runtime polymorphism.

This is an architectural feature, not technical debt.

Phase 5 revised: No migration needed for Supervisor/Collector.
Phase 6: Remove registry, keep legacy APIs (use conventions)."
```

---

### Phase 5 Verification (Revised)

The realization is: **Phase 5 is complete as-is.**

No code changes needed. The current architecture is correct:
- Generic APIs for type-safe direct usage
- Legacy APIs for dynamic dispatch

Run tests to confirm:
```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
make test
```

**Deliverables:**
- ✅ Dynamic dispatch analysis documented
- ✅ Architecture decision recorded
- ✅ No unsafe migrations attempted

---

## Phase 6: Remove Registry System (2 days)

### Goal
Remove registry implementation while keeping legacy API signatures for dynamic dispatch.

### Task 6.1: Final Verification Before Deletion

**Step 1: Verify no registry lookups in generic APIs**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
grep -n "registry.GetTriangularCollections" pkg/cse/storage/triangular.go
```

**Expected output:** Only legacy APIs (SaveObserved, LoadObserved, etc.) call registry

**Step 2: Verify legacy APIs use conventions**

Check that Phase 2 changes are applied:
```bash
grep -A5 "workerType + \"_observed\"" pkg/cse/storage/triangular.go
```

**Expected:** Legacy APIs derive collection names from convention

**Step 3: Run full test suite**

```bash
make test
```

**Expected:** All tests pass

---

### Task 6.2: Remove Registry Fields from TriangularStore

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`

**Step 1: Update struct definition (line 104)**

**Before:**
```go
type TriangularStore struct {
	store        persistence.Store
	registry     *Registry
	typeRegistry *TypeRegistry
	syncID       *atomic.Int64
}
```

**After:**
```go
type TriangularStore struct {
	store  persistence.Store
	syncID *atomic.Int64
}
```

**Step 2: Update NewTriangularStore constructor (line 127)**

**Before:**
```go
func NewTriangularStore(store persistence.Store, registry *Registry) *TriangularStore {
	return &TriangularStore{
		store:        store,
		registry:     registry,
		typeRegistry: NewTypeRegistry(),
		syncID:       &atomic.Int64{},
	}
}
```

**After:**
```go
func NewTriangularStore(store persistence.Store) *TriangularStore {
	return &TriangularStore{
		store:  store,
		syncID: &atomic.Int64{},
	}
}
```

**Step 3: Remove Registry() accessor (around line 136)**

**Delete this method:**
```go
// Registry returns the collection registry for metadata operations.
func (ts *TriangularStore) Registry() *Registry {
	return ts.registry
}
```

**Step 4: Remove TypeRegistry() accessor**

**Delete this method:**
```go
// TypeRegistry returns the type registry for worker type registration.
func (ts *TriangularStore) TypeRegistry() *TypeRegistry {
	return ts.typeRegistry
}
```

**Step 5: Update all NewTriangularStore call sites**

```bash
# Find call sites
grep -rn "NewTriangularStore" pkg/ --include="*.go"
```

For each call site, remove the registry parameter:

**Before:**
```go
registry := storage.NewRegistry()
ts := storage.NewTriangularStore(store, registry)
```

**After:**
```go
ts := storage.NewTriangularStore(store)
```

**Step 6: Run tests**

```bash
go test -v ./pkg/cse/storage/...
```

**Expected:** Tests fail because they still pass registry parameter

**Step 7: Fix test files**

Update all test files to not create registry:

In `triangular_test.go`:
```go
// Before
registry := storage.NewRegistry()
ts := storage.NewTriangularStore(store, registry)

// After
ts := storage.NewTriangularStore(store)
```

**Step 8: Run tests again**

```bash
go test -v ./pkg/cse/storage/...
```

**Expected:** Tests pass

**Step 9: Commit**

```bash
git add pkg/cse/storage/triangular.go pkg/cse/storage/triangular_test.go
git commit -m "refactor: remove registry fields from TriangularStore

Removed:
- registry field from struct
- typeRegistry field from struct
- registry parameter from NewTriangularStore()
- Registry() accessor method
- TypeRegistry() accessor method

Collection names now derived from convention.
CSE fields from getCSEFields() constants.

Breaking change: NewTriangularStore(store) instead of NewTriangularStore(store, registry)."
```

---

### Task 6.3: Remove Registry Implementation Files

**Step 1: Delete registry files**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

git rm pkg/cse/storage/registry.go
git rm pkg/cse/storage/registry_test.go
```

**Step 2: Check if type_registry.go exists**

```bash
ls pkg/cse/storage/type_registry*.go
```

If exists:
```bash
git rm pkg/cse/storage/type_registry.go
git rm pkg/cse/storage/type_registry_test.go
```

**Step 3: Verify compilation**

```bash
go build ./pkg/cse/storage/...
```

**Expected:** No errors (no references to deleted files)

**Step 4: Run tests**

```bash
make test
```

**Expected:** All tests pass

**Step 5: Commit**

```bash
git commit -m "refactor: delete registry implementation

Deleted files:
- registry.go (200+ LOC)
- registry_test.go (150+ LOC)
- type_registry.go (if existed)
- type_registry_test.go (if existed)

Total LOC removed: ~400+

Registry functionality replaced by:
- Convention-based naming (workerType + '_' + role)
- Hardcoded CSE field constants
- Type parameter reflection for generic APIs

No behavioral change - same semantics, simpler implementation."
```

---

### Task 6.4: Remove Registry from Supervisor

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/fsmv2/supervisor/supervisor.go`

**Lines to delete:** 329-391 (entire auto-registration block)

**Step 1: Find and delete auto-registration code**

**Before (lines 329-391):**
```go
	// Auto-register triangular collections for this worker type.
	// Collections follow convention: {workerType}_identity, {workerType}_desired, {workerType}_observed
	// CSE fields standardized per role per FSM v2 contract.
	//
	// DESIGN DECISION: Auto-registration by Supervisor at initialization
	// WHY: Eliminates worker-specific registry boilerplate (86 LOC per worker).
	// Workers focus purely on business logic per FSM v2 design goal.
	//
	// TRADE-OFF: Convention over configuration. Worker type MUST follow naming convention.
	// If custom collection names needed, can still register manually before creating Supervisor.
	//
	// INSPIRED BY: Rails ActiveRecord conventions, HTTP router auto-registration patterns.
	//
	// Note: This uses storage.Registry from the CSE package for collection metadata,
	// which is unrelated to fsmv2.Dependencies (worker dependency injection).
	registry := cfg.Store.Registry()

	identityCollectionName := cfg.WorkerType + "_identity"
	desiredCollectionName := cfg.WorkerType + "_desired"
	observedCollectionName := cfg.WorkerType + "_observed"

	// Only register if not already registered (supports manual override)
	if !registry.IsRegistered(identityCollectionName) {
		if err := registry.Register(&storage.CollectionMetadata{
			Name:          identityCollectionName,
			WorkerType:    cfg.WorkerType,
			Role:          storage.RoleIdentity,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		}); err != nil {
			panic(fmt.Sprintf("failed to auto-register identity collection: %v", err))
		}

		cfg.Logger.Debugf("Auto-registered identity collection: %s", identityCollectionName)
	}

	if !registry.IsRegistered(desiredCollectionName) {
		if err := registry.Register(&storage.CollectionMetadata{
			Name:          desiredCollectionName,
			WorkerType:    cfg.WorkerType,
			Role:          storage.RoleDesired,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		}); err != nil {
			panic(fmt.Sprintf("failed to auto-register desired collection: %v", err))
		}

		cfg.Logger.Debugf("Auto-registered desired collection: %s", desiredCollectionName)
	}

	if !registry.IsRegistered(observedCollectionName) {
		if err := registry.Register(&storage.CollectionMetadata{
			Name:          observedCollectionName,
			WorkerType:    cfg.WorkerType,
			Role:          storage.RoleObserved,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		}); err != nil {
			panic(fmt.Sprintf("failed to auto-register observed collection: %v", err))
		}

		cfg.Logger.Debugf("Auto-registered observed collection: %s", observedCollectionName)
	}
```

**After:** (delete entire block)

**Step 2: Update comment above deleted section**

Add explanatory comment where registration used to be:

```go
	// Collection names follow convention: {workerType}_identity, {workerType}_desired, {workerType}_observed
	// CSE metadata fields are standardized (see storage/constants.go)
	// No registration needed - convention-based system handles this automatically.
```

**Step 3: Run tests**

```bash
go test -v ./pkg/fsmv2/supervisor/...
```

**Expected:** All tests pass (registry was not actually used, just populated)

**Step 4: Commit**

```bash
git add pkg/fsmv2/supervisor/supervisor.go
git commit -m "refactor: remove registry auto-registration from Supervisor

Deleted 62 lines of registry registration boilerplate.

Collection names derived from convention at runtime.
CSE metadata fields are hardcoded constants.

No behavioral change - collections are created on-demand
by persistence layer when first document is saved."
```

---

### Phase 6 Verification

**Step 1: Count total lines removed**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core

# Count lines in deleted files (from git history)
git show HEAD~1:pkg/cse/storage/registry.go | wc -l
git show HEAD~1:pkg/cse/storage/registry_test.go | wc -l

# Total should be ~400+ LOC
```

**Step 2: Verify no registry references remain**

```bash
grep -rn "registry\\.Register\|NewRegistry\|Registry()" pkg/ --include="*.go" | grep -v "serviceregistry"
```

**Expected output:** Empty (or only unrelated registries like serviceregistry)

**Step 3: Run full test suite**

```bash
make test
```

**Expected:** All tests pass

**Step 4: Run benchmarks**

```bash
go test -bench=. -benchmem ./pkg/cse/storage/... > /tmp/phase6_benchmarks.txt
diff /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/docs/plans/baseline_benchmarks.txt /tmp/phase6_benchmarks.txt
```

**Expected:** Performance improved (no registry overhead)

**Deliverables:**
- ✅ Registry fields removed from TriangularStore
- ✅ registry.go, registry_test.go deleted (~400 LOC)
- ✅ Supervisor auto-registration removed (62 LOC)
- ✅ All tests pass
- ✅ Performance improved

---

## Phase 7: Simplify Naming (1 day)

### Goal
Remove "Typed" suffix from generic APIs - they become the primary APIs.

### Task 7.1: Rename Generic Functions

**File to modify:** `/Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core/pkg/cse/storage/triangular.go`

**Step 1: Rename SaveObservedTyped → SaveObserved**

Current signature (line 1119):
```go
func SaveObservedTyped[T any](ts *TriangularStore, ctx context.Context, id string, observed T) (bool, error) {
```

Becomes:
```go
func SaveObserved[T any](ts *TriangularStore, ctx context.Context, id string, observed T) (bool, error) {
```

**But wait - this creates naming conflict with legacy SaveObserved!**

**Solution:** Rename legacy SaveObserved first to SaveObservedDynamic.

**Step 1a: Rename legacy SaveObserved → SaveObservedDynamic (line 510)**

```go
// Before
func (ts *TriangularStore) SaveObserved(ctx context.Context, workerType string, id string, observed interface{}) (changed bool, err error) {

// After
func (ts *TriangularStore) SaveObservedDynamic(ctx context.Context, workerType string, id string, observed interface{}) (changed bool, err error) {
```

**Step 1b: Now rename SaveObservedTyped → SaveObserved**

```go
// Before
func SaveObservedTyped[T any](ts *TriangularStore, ctx context.Context, id string, observed T) (bool, error) {

// After
func SaveObserved[T any](ts *TriangularStore, ctx context.Context, id string, observed T) (bool, error) {
```

**Step 2: Repeat for all storage functions**

- `LoadObservedTyped[T]` → `LoadObserved[T]`
- Legacy `LoadObserved` → `LoadObservedDynamic`
- `SaveDesiredTyped[T]` → `SaveDesired[T]`
- Legacy `SaveDesired` → `SaveDesiredDynamic`
- `LoadDesiredTyped[T]` → `LoadDesired[T]`
- Legacy `LoadDesired` → `LoadDesiredDynamic`

**Step 3: Update all call sites**

```bash
# Update Supervisor call sites
sed -i '' 's/\.SaveObserved(/\.SaveObservedDynamic(/g' pkg/fsmv2/supervisor/supervisor.go
sed -i '' 's/\.LoadObserved(/\.LoadObservedDynamic(/g' pkg/fsmv2/supervisor/supervisor.go
sed -i '' 's/\.SaveDesired(/\.SaveDesiredDynamic(/g' pkg/fsmv2/supervisor/supervisor.go
sed -i '' 's/\.LoadDesired(/\.LoadDesiredDynamic(/g' pkg/fsmv2/supervisor/supervisor.go

# Update Collector call sites
sed -i '' 's/\.SaveObserved(/\.SaveObservedDynamic(/g' pkg/fsmv2/supervisor/collection/collector.go

# Update test files
find pkg/cse/storage -name "*_test.go" -exec sed -i '' 's/SaveObservedTyped/SaveObserved/g' {} \;
find pkg/cse/storage -name "*_test.go" -exec sed -i '' 's/LoadObservedTyped/LoadObserved/g' {} \;
find pkg/cse/storage -name "*_test.go" -exec sed -i '' 's/SaveDesiredTyped/SaveDesired/g' {} \;
find pkg/cse/storage -name "*_test.go" -exec sed -i '' 's/LoadDesiredTyped/LoadDesired/g' {} \;
```

**Step 4: Verify compilation**

```bash
go build ./...
```

**Expected:** No errors

**Step 5: Run tests**

```bash
make test
```

**Expected:** All tests pass

**Step 6: Update MIGRATION.md**

Update examples to use new names:

```markdown
## Migration Examples (Updated)

### Example 1: SaveObserved

**Old (dynamic dispatch):**
```go
changed, err := ts.SaveObservedDynamic(ctx, workerType, id, doc)
```

**New (generic, type-safe):**
```go
changed, err := storage.SaveObserved[ContainerObservedState](ts, ctx, id, observed)
```
```

**Step 7: Commit**

```bash
git add pkg/cse/storage/triangular.go pkg/fsmv2/ pkg/cse/storage/MIGRATION.md
git commit -m "refactor: simplify generic API naming

Renames:
- SaveObservedTyped[T] → SaveObserved[T]
- LoadObservedTyped[T] → LoadObserved[T]
- SaveDesiredTyped[T] → SaveDesired[T]
- LoadDesiredTyped[T] → LoadDesired[T]

Legacy APIs for dynamic dispatch renamed:
- SaveObserved → SaveObservedDynamic
- LoadObserved → LoadObservedDynamic
- SaveDesired → SaveDesiredDynamic
- LoadDesired → LoadDesiredDynamic

Generic APIs are now the primary APIs.
Dynamic APIs clearly marked for runtime polymorphism."
```

---

### Phase 7 Verification

**Step 1: Verify naming consistency**

```bash
# Check all SaveObserved calls use correct name
grep -rn "SaveObserved\[" pkg/ --include="*.go" | head -5

# Check dynamic calls use Dynamic suffix
grep -rn "SaveObservedDynamic" pkg/ --include="*.go"
```

**Step 2: Run full test suite**

```bash
make test
```

**Expected:** All tests pass

**Step 3: Run linters**

```bash
golangci-lint run ./pkg/cse/storage/...
go vet ./pkg/cse/storage/...
```

**Expected:** No errors

**Deliverables:**
- ✅ Generic APIs have clean names (no "Typed" suffix)
- ✅ Dynamic APIs clearly marked with "Dynamic" suffix
- ✅ All call sites updated
- ✅ Tests pass
- ✅ Linters pass

---

## Rollback Procedures

### Per-Phase Rollback

**Phases 1-4:** Safe to revert (no breaking changes)
```bash
git revert <commit-hash>
```

**Phase 5:** Safe to revert (no changes made)

**Phase 6:** Breaking changes (requires careful rollback)
```bash
# Identify commits to revert
git log --oneline | grep -E "registry|Registry"

# Revert in reverse order
git revert <newest-commit> <older-commit> <oldest-commit>

# May require manual conflict resolution
```

**Phase 7:** Safe to revert (rename-only changes)
```bash
git revert <commit-hash>
```

### Complete Rollback

To rollback entire refactoring:
```bash
# Find first commit of Phase 1
git log --oneline --grep="migration scenario tests"

# Revert all commits since then
git revert <first-commit>^..HEAD

# Or reset to before refactoring (destructive)
git reset --hard <commit-before-phase-1>
```

---

## Success Criteria

- [ ] All tests pass after each phase
- [ ] No focused tests remain
- [ ] Linters pass (golangci-lint, go vet)
- [ ] Performance improved (benchmarks show reduced overhead)
- [ ] ~400 LOC removed (registry implementation)
- [ ] Generic APIs work independently
- [ ] Legacy APIs work for dynamic dispatch
- [ ] Migration guide complete with examples
- [ ] Architecture decision documented

---

## Changelog

### 2025-01-13 14:30 - Plan created

Initial 7-phase refactoring plan based on subagent investigation findings:
- Phase 1: Preparation (tests, docs, benchmarks)
- Phase 2: Constants (hardcode CSE fields)
- Phase 3: Generic APIs independent
- Phase 4: Deprecation markers
- Phase 5: Call site migration (revised after analysis)
- Phase 6: Registry deletion
- Phase 7: Naming simplification

### 2025-01-13 15:45 - Phase 5 revised after dynamic dispatch analysis

Discovered all current call sites (Supervisor, Collector) use runtime polymorphism.

Decision: Keep dual API system:
- Generic APIs for compile-time known types
- Legacy APIs for runtime dispatch

Phase 5 requires no code changes - current architecture is correct.
