# Persistence Layer Restructure Plan

## Objective

Simplify persistence package structure from `persistence/basic/` to cleaner `persistence/` with separate `memory/` subdirectory for in-memory implementation.

## Current Structure (BEFORE)

```
pkg/persistence/basic/
├── store.go                          # Store interface, Document, errors
├── query.go                          # Query builder
├── transaction.go                    # Transaction helpers
├── in_memory_store.go               # In-memory implementation
├── sqlite.go                         # SQLite implementation (DELETE)
├── sqlite_test.go                   # SQLite tests (DELETE)
├── sqlite_benchmark_test.go         # SQLite benchmarks (DELETE)
├── filesystem_detection.go          # SQLite utility (DELETE)
├── filesystem_detection_darwin.go   # SQLite utility (DELETE)
├── filesystem_detection_linux.go    # SQLite utility (DELETE)
├── query_test.go                    # Query tests (KEEP)
├── store_test.go                    # Store tests (KEEP)
└── transaction_test.go              # Transaction tests (KEEP)
```

Package: `github.com/.../pkg/persistence/basic`

## Target Structure (AFTER)

```
pkg/persistence/
├── store.go                          # Store interface, Document, errors (MOVED from basic/)
├── query.go                          # Query builder (MOVED from basic/)
├── transaction.go                    # Transaction helpers (MOVED from basic/)
├── query_test.go                    # Query tests (MOVED from basic/)
├── store_test.go                    # Store tests (MOVED from basic/)
├── transaction_test.go              # Transaction tests (MOVED from basic/)
└── memory/
    └── memory.go                     # In-memory implementation (MOVED & RENAMED from basic/in_memory_store.go)
```

Package: `github.com/.../pkg/persistence`
Memory subpackage: `github.com/.../pkg/persistence/memory`

## Rationale

### Why Remove `basic/` Subdirectory?

1. **Architecture Simplification Principle**: "2 implementations don't justify hierarchical structure"
   - Only 2 implementations: InMemoryStore (MVP) and SQLiteStore (future)
   - "basic" naming is confusing - means "base layer" not "simple/demo"
   - Flat structure is clearer: `persistence/` = interface, `persistence/memory/` = implementation

2. **User Feedback**: Jeremy said "why persistence/basic? why not simply persistence? this was why i was confused"
   - Nested structure creates unnecessary cognitive overhead
   - Direct `import "pkg/persistence"` is clearer than `import "pkg/persistence/basic"`

3. **Separation of Interface and Implementation**:
   - `persistence/` contains abstract interface (Store, Query, Tx)
   - `persistence/memory/` contains concrete implementation
   - Future: Add `persistence/sqlite/` when needed (follow-up PR)

### Why Delete SQLite Implementation?

1. **Scope Management**: SQLite is 1,317 LOC of production code that complicates this PR
2. **Available Elsewhere**: Full implementation exists in other branches/PRs
3. **MVP Focus**: In-memory store is sufficient for current FSM v2 implementation
4. **Follow-up PR**: SQLite can be added back cleanly when needed as `persistence/sqlite/sqlite.go`

## Step-by-Step Transformation

### Phase 1: Move Core Files (Preserve Git History)

Use `git mv` to preserve commit history:

```bash
# Move interface and core types to top level
git mv pkg/persistence/basic/store.go pkg/persistence/store.go
git mv pkg/persistence/basic/query.go pkg/persistence/query.go
git mv pkg/persistence/basic/transaction.go pkg/persistence/transaction.go

# Move tests to top level
git mv pkg/persistence/basic/query_test.go pkg/persistence/query_test.go
git mv pkg/persistence/basic/store_test.go pkg/persistence/store_test.go
git mv pkg/persistence/basic/transaction_test.go pkg/persistence/transaction_test.go
```

### Phase 2: Create Memory Subdirectory

```bash
# Create memory subdirectory
mkdir -p pkg/persistence/memory

# Move and rename in-memory implementation
git mv pkg/persistence/basic/in_memory_store.go pkg/persistence/memory/memory.go
```

### Phase 3: Delete SQLite Files

```bash
# Remove SQLite implementation (too complex for this PR)
git rm pkg/persistence/basic/sqlite.go
git rm pkg/persistence/basic/sqlite_test.go
git rm pkg/persistence/basic/sqlite_benchmark_test.go
git rm pkg/persistence/basic/filesystem_detection.go
git rm pkg/persistence/basic/filesystem_detection_darwin.go
git rm pkg/persistence/basic/filesystem_detection_linux.go

# Remove empty basic/ directory
rmdir pkg/persistence/basic/
```

### Phase 4: Update Package Declarations

**In moved files (store.go, query.go, transaction.go, *_test.go):**
```go
// BEFORE:
package basic

// AFTER:
package persistence
```

**In memory/memory.go:**
```go
// BEFORE:
package basic

// AFTER:
package memory

// ADD import at top:
import (
    "github.com/.../pkg/persistence"
)

// UPDATE type references:
// basic.Store → persistence.Store
// basic.Document → persistence.Document
// basic.Query → persistence.Query
// etc.
```

### Phase 5: Update Imports Across Codebase

**Files to update** (found via `grep -r "persistence/basic"`):
- `pkg/fsmv2/supervisor/supervisor.go`
- `pkg/fsmv2/supervisor/supervisor_test.go`
- `pkg/cse/storage/triangular.go`
- `pkg/cse/storage/triangular_test.go`
- `pkg/cse/storage/interfaces.go`
- `pkg/cse/storage/tx_cache.go`
- `pkg/cse/storage/tx_cache_test.go`

**Import changes:**
```go
// BEFORE:
import "github.com/.../pkg/persistence/basic"
// Use as: basic.Store, basic.Document, basic.NewInMemoryStore()

// AFTER:
import (
    "github.com/.../pkg/persistence"
    "github.com/.../pkg/persistence/memory"
)
// Use as: persistence.Store, persistence.Document, memory.NewStore()
```

**Constructor name change:**
```go
// BEFORE:
store := basic.NewInMemoryStore()

// AFTER:
store := memory.NewStore()
```

### Phase 6: Verification

```bash
# Run all tests
go test ./pkg/persistence/...
go test ./pkg/cse/storage/...
go test ./pkg/fsmv2/...

# Check imports
go list -f '{{.ImportPath}}: {{.Imports}}' ./pkg/... | grep persistence

# Verify no references to old package
grep -r "persistence/basic" pkg/
# Should return: (no results)
```

## Import Mapping Reference

| Old Import | New Import | Type Changes |
|------------|------------|--------------|
| `import "pkg/persistence/basic"` | `import "pkg/persistence"` | `basic.Store` → `persistence.Store`<br>`basic.Document` → `persistence.Document`<br>`basic.Query` → `persistence.Query`<br>`basic.Tx` → `persistence.Tx`<br>`basic.Eq` → `persistence.Eq` |
| `import "pkg/persistence/basic"` | `import "pkg/persistence/memory"` | `basic.NewInMemoryStore()` → `memory.NewStore()` |

## Files Affected Summary

**Moved (6 files):**
- store.go → persistence/store.go
- query.go → persistence/query.go
- transaction.go → persistence/transaction.go
- query_test.go → persistence/query_test.go
- store_test.go → persistence/store_test.go
- transaction_test.go → persistence/transaction_test.go

**Moved & Renamed (1 file):**
- in_memory_store.go → persistence/memory/memory.go

**Deleted (6 files):**
- sqlite.go (1,317 LOC)
- sqlite_test.go
- sqlite_benchmark_test.go
- filesystem_detection.go
- filesystem_detection_darwin.go
- filesystem_detection_linux.go

**Updated (7 files):**
- pkg/fsmv2/supervisor/supervisor.go
- pkg/fsmv2/supervisor/supervisor_test.go
- pkg/cse/storage/triangular.go
- pkg/cse/storage/triangular_test.go
- pkg/cse/storage/interfaces.go
- pkg/cse/storage/tx_cache.go
- pkg/cse/storage/tx_cache_test.go

## Subagent Tasks

### Task 1: Move Core Files
**Description**: Move store.go, query.go, transaction.go and their tests from basic/ to persistence/ using git mv
**Files**: 6 files
**Command**: `git mv pkg/persistence/basic/{store,query,transaction}.go pkg/persistence/` and tests

### Task 2: Create Memory Package
**Description**: Create memory/ subdirectory and move in_memory_store.go to memory/memory.go
**Files**: 1 file
**Command**: `mkdir pkg/persistence/memory && git mv pkg/persistence/basic/in_memory_store.go pkg/persistence/memory/memory.go`

### Task 3: Delete SQLite Implementation
**Description**: Remove all SQLite-related files
**Files**: 6 files
**Command**: `git rm pkg/persistence/basic/sqlite*.go pkg/persistence/basic/filesystem_detection*.go`

### Task 4: Update Package Declarations
**Description**: Change `package basic` to `package persistence` in moved files, `package memory` in memory.go
**Files**: 8 files (7 moved + memory.go)

### Task 5: Update Imports in Consumers
**Description**: Update all imports from persistence/basic to persistence + persistence/memory
**Files**: 7 files across cse/storage and fsmv2/supervisor

### Task 6: Verify and Test
**Description**: Run tests, verify no broken imports, confirm all 234 tests still pass
**Command**: `go test ./pkg/...`

## Success Criteria

✅ All files moved with `git mv` (preserves history)
✅ No `pkg/persistence/basic/` directory exists
✅ Package `persistence` contains interface, Query, Transaction helpers
✅ Package `persistence/memory` contains in-memory implementation
✅ SQLite implementation deleted (6 files removed)
✅ All imports updated (`persistence/basic` → `persistence` + `persistence/memory`)
✅ All 234 tests pass
✅ No compiler errors
✅ `grep -r "persistence/basic" pkg/` returns empty
✅ `grep -r "package basic" pkg/persistence/` returns empty
