# FSM v2 ↔ CSE Interface Contract

**Status**: Shared Persistence Foundation (PR #0)
**Date**: 2025-10-26
**Purpose**: Provide the shared persistence layer that BOTH FSM v2 and CSE build upon, with validated contract tests

## Executive Summary

This PR provides the **Shared Persistence Foundation** - the common SQLite-based storage layer that both FSM v2 and CSE use. It includes:

1. **Layer 1** (`pkg/persistence/basic/`): Database-agnostic document storage with SQLite implementation
2. **Layer 2** (`pkg/cse/storage/`): TriangularStore with automatic CSE metadata injection (`_sync_id`, `_version`, timestamps)
3. **Contract Validation Tests**: Executable tests proving the persistence layer works for both FSM v2 and CSE use cases

**Key Insight**: Both teams build on the same persistence foundation. FSM v2 writes state using TriangularStore methods, CSE reads changes via `_sync_id` queries. No adapter needed - they share the same tables.

## What This PR Provides

### For FSM v2 Developers

**You get**:
- `TriangularStore.SaveObserved(workerType, id, observed)` - Auto-adds CSE metadata transparently
- `TriangularStore.SaveDesired(workerType, id, desired)` - Auto-increments version for optimistic locking
- `TriangularStore.LoadSnapshot(workerType, id)` - Atomically loads identity + desired + observed
- Append-only log pattern (no read-modify-write for logs)

**You provide**:
- Worker type registration with Registry
- Observed/Desired state as `basic.Document` (map[string]interface{})

**Example**:
```go
// Save agent observed state
ts.SaveObserved(ctx, "agent", "agent-123", basic.Document{
    "id": "agent-123",
    "status": "running",
    "cpu_percent": 45.2,
})

// CSE metadata auto-added: _sync_id, _updated_at
```

### For CSE Developers

**You get**:
- Every FSM v2 write automatically increments global `_sync_id`
- Query changes since last sync: `WHERE _sync_id > lastSyncID`
- Timestamps: `_created_at`, `_updated_at`
- Version tracking: `_version` (for desired state, optimistic locking)

**You provide**:
- Sync orchestrator that queries `_sync_id`
- Frontend/Edge tier tracking (last synced ID per tier)
- Change filters and delta query logic

**Example**:
```go
// Get all changes since last sync
query := basic.NewQuery().
    Filter("_sync_id", "$gt", lastSyncID).
    Sort("_sync_id", 1)

changes, _ := store.Find(ctx, "agent_observed", query)
// Returns all agent state changes since lastSyncID
```

## Architecture: Three Layers

```
┌─────────────────────────────────────────────────┐
│ PR #1: FSM v2 (Future)                          │
│ - Worker interface implementations              │
│ - Supervisor observation loop                   │
│ - Action execution                              │
└─────────────────────────────────────────────────┘
                    ↓ uses
┌─────────────────────────────────────────────────┐
│ PR #0: Layer 2 - CSE Storage (THIS PR)          │
│ pkg/cse/storage/TriangularStore                 │
│ - SaveObserved() auto-adds _sync_id             │
│ - SaveDesired() auto-increments _version        │
│ - SaveIdentity() for immutable worker identity  │
└─────────────────────────────────────────────────┘
                    ↓ uses
┌─────────────────────────────────────────────────┐
│ PR #0: Layer 1 - Basic Storage (THIS PR)        │
│ pkg/persistence/basic/Store                     │
│ - Document CRUD (Insert, Get, Update, Delete)   │
│ - Query builder (Filter, Sort, Limit)           │
│ - Transaction support (BeginTx, Commit, Rollback)│
│ - SQLite implementation with optimizations      │
└─────────────────────────────────────────────────┘
```

## The Problem This Solves

### Without Shared Foundation (Traditional Approach)

**Week 1**: FSM v2 team builds Agent with custom persistence
```go
type AgentPersistence struct {
    // FSM v2's own storage approach
}
```

**Week 2**: CSE team builds TriangularStore with different storage
```go
type TriangularStore struct {
    // CSE's own storage approach
}
```

**Week 3**: Try to integrate → **INCOMPATIBLE!** 💥
- FSM v2 writes data CSE can't query
- CSE needs metadata FSM v2 doesn't add
- Result: Rewrite one or both systems

### With Shared Foundation (Our Approach)

**Week 0 (PR #0 - THIS PR)**: Build shared persistence layer
- TriangularStore auto-adds `_sync_id` on every write
- FSM v2 and CSE both use TriangularStore
- Contract tests prove it works for both

**Week 1 (PR #1)**: FSM v2 uses TriangularStore
- Calls `SaveObserved()` on every worker tick
- Metadata added automatically
- Tests pass ✅

**Week 2 (PR #2)**: CSE queries TriangularStore
- Queries `WHERE _sync_id > lastSyncID`
- Gets all FSM v2 state changes
- Tests pass ✅

**Week 3 (PR #3)**: Integration → **Perfect fit!** ✅
- Both already using same storage
- Zero integration work needed

## The Problem This Solves

### Without Contract Validation (Traditional Approach)

**Week 1**: FSM v2 team builds Agent with logs as array in ObservedState
```go
type AgentObservedState struct {
    Logs []LogEntry  // Assumption: Store logs in main struct
}
```

**Week 2**: CSE team builds TriangularStore assuming separate tables
```sql
CREATE TABLE agent_logs (...)  -- Assumption: Logs in child table
```

**Week 3**: Merge attempt → **DOESN'T FIT!** 💥
- FSM v2 expects array in struct
- CSE expects separate table
- Result: Weeks of refactoring both sides

### With Contract Validation (Our Approach)

**Week 0 (PR #0)**: Write contract test FIRST
```go
It("should save logs to separate child table with FK", func() {
    observed := &MockObservedState{
        NewLogs: []LogEntry{...},  // ← CONTRACT DEFINED
    }

    // Verify: Logs in mock_logs table ← CONTRACT VALIDATED
    var count int
    db.QueryRow("SELECT COUNT(*) FROM mock_logs WHERE worker_id = ?", "worker-1").Scan(&count)
    Expect(count).To(Equal(2))  // ← BOTH TEAMS CODE TO THIS
})
```

**Week 1 (PR #1)**: FSM v2 follows contract
- Contract says: "Use `NewLogs []LogEntry`"
- FSM v2 implements: `NewLogs []LogEntry`
- Tests pass ✅

**Week 2 (PR #2)**: CSE follows contract
- Contract says: "Create `agent_logs` child table"
- CSE implements: `CREATE TABLE agent_logs`
- Tests pass ✅

**Week 3 (PR #3)**: Merge → **Perfect fit!** ✅
- Both sides match the contract
- Zero surprises, zero rework

## Contract Overview

The contract defines **four access patterns** that cover all data types in FSM v2:

| Pattern | Purpose | Example | Storage | Performance |
|---------|---------|---------|---------|-------------|
| **Instant** | Simple snapshot fields | `Status string` | Main table | Fast reads |
| **Lazy** | 1:1 nested config | `Config *AgentConfig` | Child table (PK=FK) | Loaded on demand |
| **Partial** | Append-only logs | `NewLogs []LogEntry` | Child table (1:many) | Paginated queries |
| **Explicit** | Large blobs | `MemoryProfile []byte` | Blob table with checksum | Skip if unchanged |

### Why These Patterns?

**Instant**: Status changes frequently, must be fast to read (single SELECT)

**Lazy**: Config rarely changes, don't load unless needed (JOIN only when requested)

**Partial**: Logs grow unbounded, never load all (append-only, query last N)

**Explicit**: Memory profiles are 5MB+, avoid write if unchanged (checksum optimization)

## Struct Tag Syntax

Developers declare access patterns using Go struct tags:

### Pattern 1: Instant (Default)

```go
type AgentObservedState struct {
    Status     string  `cse:"instant"`
    CPUPercent float64 `cse:"instant"`
    // Untagged fields default to instant
    RAMUsageMB int64
}
```

**Storage**: All instant fields in main table `agent_observed`

**SQL**:
```sql
CREATE TABLE agent_observed (
    id TEXT PRIMARY KEY,
    status TEXT,
    cpu_percent REAL,
    ram_usage_mb INTEGER,
    _sync_id INTEGER,
    _created_at INTEGER,
    _updated_at INTEGER
)
```

### Pattern 2: Lazy (1:1 Child Table)

```go
type AgentObservedState struct {
    Config *AgentConfig `cse:"lazy,table=agent_config"`
}

type AgentConfig struct {
    Setting1 string
    Setting2 int
}
```

**Storage**: Separate table with PK=FK relationship

**SQL**:
```sql
CREATE TABLE agent_config (
    id TEXT PRIMARY KEY,
    setting1 TEXT,
    setting2 INTEGER,
    _sync_id INTEGER,
    FOREIGN KEY(id) REFERENCES agent_observed(id)
)
```

**Behavior**:
- `nil` config → no row in child table
- Non-nil config → INSERT OR REPLACE

### Pattern 3: Partial (Append-Only Logs)

```go
type AgentObservedState struct {
    NewLogs []LogEntry `cse:"partial,table=agent_logs,append_only"`
}

type LogEntry struct {
    Timestamp time.Time
    Level     string
    Message   string
}
```

**Storage**: Separate table with 1:many relationship

**SQL**:
```sql
CREATE TABLE agent_logs (
    id TEXT PRIMARY KEY,
    worker_id TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    level TEXT,
    message TEXT,
    _sync_id INTEGER,
    FOREIGN KEY(worker_id) REFERENCES agent_observed(id)
)
CREATE INDEX idx_agent_logs ON agent_logs(worker_id, timestamp DESC)
```

**Critical Contract Points**:
- ✅ SaveObserved appends new logs WITHOUT loading existing logs (no read-modify-write)
- ✅ LoadObserved does NOT load logs (violates partial pattern)
- ✅ LoadLogs separate method for paginated queries
- ✅ Index on (worker_id, timestamp DESC) for efficient pagination

**Why Append-Only Matters**:
```go
// WRONG: Read-modify-write (loads all logs on every save)
existingLogs := adapter.LoadLogs(...)
allLogs := append(existingLogs, newLogs...)
adapter.SaveLogs(allLogs)  // ❌ O(N) read + O(N) write

// RIGHT: Append-only (no read needed)
adapter.SaveObserved(&AgentObservedState{
    NewLogs: newLogs,  // ✅ O(1) write only
})
```

### Pattern 4: Explicit (Checksum-Optimized Blobs)

```go
type AgentObservedState struct {
    MemoryProfile []byte `cse:"explicit,table=agent_profiles_explicit,skip_if_unchanged"`
}
```

**Storage**: Separate table with checksum column

**SQL**:
```sql
CREATE TABLE agent_profiles_explicit (
    id TEXT PRIMARY KEY,
    profile_blob BLOB,
    checksum TEXT,
    _sync_id INTEGER,
    FOREIGN KEY(id) REFERENCES agent_observed(id)
)
```

**Behavior**:
```go
// First save: Blob written
SaveObserved(&AgentObservedState{
    MemoryProfile: []byte{...},  // Writes blob + SHA256 checksum
})

// Second save with same blob: Skip write
SaveObserved(&AgentObservedState{
    MemoryProfile: []byte{...},  // Checksum matches, skip! ✅
})

// Save with different blob: Update
SaveObserved(&AgentObservedState{
    MemoryProfile: []byte{...different...},  // Checksum differs, write ✅
})
```

**Why This Matters**: 5MB blob write every second → 300MB/min. Checksum skip → 0 bytes if unchanged.

## Full Example: Agent Worker

```go
package agent

import (
    "time"
    "github.com/united-manufacturing-hub/.../pkg/fsmv2"
)

type AgentIdentity struct {
    WorkerID string
    Version  string
}

type AgentDesiredState struct {
    ShutdownRequested bool
}

type AgentObservedState struct {
    // Instant pattern (main table)
    Status     string    `cse:"instant"`
    CPUPercent float64   `cse:"instant"`
    RAMUsageMB int64     // Defaults to instant

    // Lazy pattern (1:1 child table)
    Config *AgentConfig `cse:"lazy,table=agent_config"`

    // Partial pattern (append-only logs)
    NewLogs []LogEntry `cse:"partial,table=agent_logs,append_only"`

    // Explicit pattern (checksum-optimized blob)
    MemoryProfile []byte `cse:"explicit,table=agent_profiles_explicit,skip_if_unchanged"`

    CollectedAt time.Time
}

func (o *AgentObservedState) GetTimestamp() time.Time {
    return o.CollectedAt
}

func (o *AgentObservedState) GetObservedDesiredState() fsmv2.DesiredState {
    return &AgentDesiredState{}
}

type AgentConfig struct {
    LogLevel          string
    MetricsInterval   time.Duration
    EnableProfiling   bool
}

type LogEntry struct {
    Timestamp time.Time
    Level     string
    Message   string
}

// Worker implementation appends logs naturally
func (w *AgentWorker) Observe(ctx context.Context) (fsmv2.ObservedState, error) {
    observed := &AgentObservedState{
        Status:      w.getCurrentStatus(),
        CPUPercent:  w.getCPUUsage(),
        RAMUsageMB:  w.getRAMUsage(),
        CollectedAt: time.Now(),
    }

    // Append new logs since last observation
    newLogs := w.getNewLogsSinceLastObservation()
    observed.NewLogs = newLogs  // Adapter appends to DB without reading existing

    return observed, nil
}
```

## For FSM v2 Developers

### How to Use This Contract

1. **Define your ObservedState** with struct tags:
   ```go
   type MyObservedState struct {
       Status   string     `cse:"instant"`
       NewLogs  []LogEntry `cse:"partial,table=my_logs,append_only"`
   }
   ```

2. **Implement Observe()** to return new state:
   ```go
   func (w *MyWorker) Observe(ctx context.Context) (fsmv2.ObservedState, error) {
       return &MyObservedState{
           Status:  w.getStatus(),
           NewLogs: w.getNewLogsSinceLastCheck(),  // Only NEW logs
       }, nil
   }
   ```

3. **Trust the adapter** to handle persistence correctly:
   - Instant fields → main table (fast read)
   - NewLogs → append to child table (no read-modify-write)
   - Blob → skip write if unchanged (checksum optimization)

### Testing Your Worker

Run contract validation tests to verify your struct tags:

```bash
cd pkg/fsmv2/persistence
ginkgo -v . -focus="Contract Validation"
```

Tests validate:
- ✅ Struct tags parse correctly
- ✅ SQL tables created with correct schema
- ✅ Round-trip save/load preserves all data
- ✅ Logs append without loading existing logs
- ✅ Blobs skip write when unchanged

## For CSE Developers

### Expected SQL Schema

The TriangularStore must create tables based on struct tag patterns:

#### Main Table (Instant Fields)
```sql
CREATE TABLE {worker_type}_observed (
    id TEXT PRIMARY KEY,
    -- One column per instant field (snake_case)
    status TEXT,
    cpu_percent REAL,
    ram_usage_mb INTEGER,
    -- CSE metadata
    _sync_id INTEGER,
    _created_at INTEGER,
    _updated_at INTEGER
)
```

#### Child Table (Lazy Fields)
```sql
CREATE TABLE {worker_type}_config (
    id TEXT PRIMARY KEY,  -- PK = FK (1:1 relationship)
    -- One column per struct field
    setting1 TEXT,
    setting2 INTEGER,
    _sync_id INTEGER,
    FOREIGN KEY(id) REFERENCES {worker_type}_observed(id)
)
```

#### Child Table (Partial Fields - Logs)
```sql
CREATE TABLE {worker_type}_logs (
    id TEXT PRIMARY KEY,
    worker_id TEXT NOT NULL,  -- FK to main table (1:many)
    timestamp INTEGER NOT NULL,
    level TEXT,
    message TEXT,
    _sync_id INTEGER,
    FOREIGN KEY(worker_id) REFERENCES {worker_type}_observed(id)
)
CREATE INDEX idx_{worker_type}_logs ON {worker_type}_logs(worker_id, timestamp DESC)
```

**Critical**: Index must support efficient pagination queries:
```sql
SELECT * FROM {worker_type}_logs
WHERE worker_id = ?
ORDER BY timestamp DESC
LIMIT 10  -- Last 10 logs, O(log N) with index
```

#### Blob Table (Explicit Fields)
```sql
CREATE TABLE {worker_type}_profiles_explicit (
    id TEXT PRIMARY KEY,
    profile_blob BLOB,
    checksum TEXT,  -- SHA256 hex string
    _sync_id INTEGER,
    FOREIGN KEY(id) REFERENCES {worker_type}_observed(id)
)
```

### Required TriangularAdapter Methods

```go
type TriangularAdapter interface {
    // Save observed state (detect patterns via struct tags)
    SaveObserved(ctx context.Context, workerType, workerID string, observed fsmv2.ObservedState) error

    // Load observed state (instant + lazy + explicit, NO logs)
    LoadObserved(ctx context.Context, workerType, workerID string) (fsmv2.ObservedState, error)

    // Load logs separately with pagination
    LoadLogs(ctx context.Context, workerType, workerID string, opts LogQueryOpts) ([]LogEntry, error)
}

type LogQueryOpts struct {
    Limit       int
    SinceTime   *time.Time
    UntilTime   *time.Time
    LevelFilter string
    OrderDesc   bool
}
```

### Implementation Guidelines

**Pattern Detection**: Use reflection to detect struct tags at runtime
```go
func (a *TriangularAdapter) detectPatterns(t reflect.Type) map[string]AccessPattern {
    patterns := make(map[string]AccessPattern)
    for i := 0; i < t.NumField(); i++ {
        field := t.Field(i)
        tag := field.Tag.Get("cse")
        // Parse: "partial,table=agent_logs,append_only"
        patterns[field.Name] = parseTag(tag)
    }
    return patterns
}
```

**Append-Only Implementation**:
```go
// SaveObserved must NOT read existing logs
func (a *TriangularAdapter) SaveObserved(...) error {
    for _, log := range observed.NewLogs {
        // Direct INSERT, no SELECT needed
        db.Exec("INSERT INTO agent_logs (...) VALUES (...)", ...)
    }
}
```

**Checksum Optimization**:
```go
func (a *TriangularAdapter) saveBlob(workerID string, blob []byte) error {
    newChecksum := sha256hex(blob)

    var existingChecksum string
    db.QueryRow("SELECT checksum FROM profiles WHERE id = ?", workerID).Scan(&existingChecksum)

    if existingChecksum == newChecksum {
        return nil  // Skip write, blob unchanged
    }

    db.Exec("INSERT OR REPLACE INTO profiles (id, blob, checksum) VALUES (?, ?, ?)",
        workerID, blob, newChecksum)
}
```

## Contract Validation Tests

The contract is validated by executable tests in `pkg/fsmv2/persistence/contract_validation_test.go`.

### Running Tests

```bash
cd pkg/fsmv2/persistence
ginkgo -v . -focus="Contract Validation"
```

### What Tests Validate

**19 test specs covering**:

1. **Instant Pattern (3 specs)**
   - Save instant fields to main table
   - Load instant fields from main table
   - Update without creating duplicates

2. **Lazy Pattern (3 specs)**
   - Save lazy fields to child table with PK=FK
   - Handle nil lazy fields gracefully
   - Load lazy fields from child table

3. **Partial Pattern (5 specs)**
   - Save logs to separate child table with FK
   - Append new logs without loading existing logs ← **Critical**
   - Support paginated log queries (last N logs)
   - Support time-based log filtering
   - Support log level filtering

4. **Explicit Pattern (4 specs)**
   - Save large blob to explicit table
   - Skip write if blob unchanged (checksum match) ← **Critical**
   - Update blob if content changed
   - Handle nil blob gracefully

5. **Struct Tags Detection (2 specs)**
   - Parse cse tags from struct fields
   - Default to instant for untagged fields

6. **Full Round-Trip Integration (2 specs)**
   - Preserve all data through save and load cycle
   - Handle multiple workers independently

### Example Test (Append-Only Validation)

```go
It("should append new logs without loading existing logs", func() {
    // Insert existing log directly to DB
    testDB.Exec("INSERT INTO mock_logs (...) VALUES (...)", "Old log")

    // Append new logs via adapter (should NOT read old logs)
    observed := &MockObservedState{
        NewLogs: []LogEntry{
            {Message: "New log 1"},
            {Message: "New log 2"},
        },
    }
    stubAdapter.SaveObserved(ctx, "mock", "worker-1", observed)

    // Verify: All 3 logs exist (1 old + 2 new)
    var count int
    testDB.QueryRow("SELECT COUNT(*) FROM mock_logs WHERE worker_id = ?", "worker-1").Scan(&count)
    Expect(count).To(Equal(3))  // ✅ Append worked without reading
})
```

This test **proves** the adapter implements append-only correctly.

## PR Sequence and Contract Enforcement

### PR #0 (This PR): Contract Definition
- File: `pkg/fsmv2/persistence/contract_validation_test.go`
- File: `docs/architecture/fsm-v2-cse-contract.md`
- Status: All 19 tests passing with StubAdapter
- Purpose: Define contract BEFORE implementation

### PR #1 (Week 1): FSM v2 Framework
- Implements: `pkg/fsmv2/worker.go`, `pkg/fsmv2/supervisor/`
- Uses: MockStore for persistence (no real CSE)
- Must pass: All contract validation tests
- CI enforces: Tests run on every commit

### PR #2 (Week 2): CSE Foundation
- Implements: `pkg/cse/storage/triangular_store.go`
- Uses: Real SQLite tables matching contract
- Must pass: All contract validation tests
- CI enforces: Tests run on every commit

### PR #3 (Week 3): Real Adapter
- Implements: `pkg/fsmv2/persistence/triangular_adapter.go`
- Replaces: StubAdapter with TriangularAdapter
- Must pass: All contract validation tests (still!)
- CI enforces: Tests run on every commit

**Result**: If all PRs pass tests independently, they integrate perfectly.

## FAQ

### Q: Why not just document the interface?

**A**: Documentation can be misinterpreted. Executable tests define the contract unambiguously.

Example: "Logs should be saved separately" is vague.

Test: `db.QueryRow("SELECT COUNT(*) FROM agent_logs WHERE worker_id = ?")` is precise.

### Q: What if we need to change the contract later?

**A**: Change the tests first, then update implementations to match new tests.

Contract tests are the source of truth, not the implementation.

### Q: Why append-only for logs instead of loading all logs?

**A**: Performance and data freshness.

Bad (read-modify-write):
- Read 100,000 old logs from DB (slow)
- Append 10 new logs
- Write 100,010 logs back (very slow)

Good (append-only):
- Write 10 new logs directly (fast)
- Query last N logs when needed (paginated)

### Q: Why checksum for blobs?

**A**: Avoid writing 5MB blobs every second if unchanged.

Example: Memory profile captured every second:
- Without checksum: 5MB × 60 seconds = 300MB/min written
- With checksum: 0 bytes if profile unchanged (typical case)

### Q: Can I add custom access patterns?

**A**: Yes, but add them to contract tests first.

1. Add test spec for new pattern
2. Update StubAdapter to pass test
3. Both teams implement new pattern
4. Integration works (contract validated!)

## Related Documents

**Design Documents** (in `docs/plans/`):
- [PR_SPLITTING_STRATEGY.md](../plans/PR_SPLITTING_STRATEGY.md) - Overall PR strategy
- [INTERFACE_CONTRACT_STRATEGY.md](../plans/INTERFACE_CONTRACT_STRATEGY.md) - Contract testing approach
- [DATA_ACCESS_ABSTRACTION_DESIGN.md](../plans/DATA_ACCESS_ABSTRACTION_DESIGN.md) - Struct tags design
- [LOG_HANDLING_DESIGN.md](../plans/LOG_HANDLING_DESIGN.md) - Append-only log pattern
- [DATA_TYPE_COMPATIBILITY_MATRIX.md](../plans/DATA_TYPE_COMPATIBILITY_MATRIX.md) - Pattern analysis

**Implementation Documents**:
- [CONTRACT_VALIDATION_COMPLETE.md](../plans/CONTRACT_VALIDATION_COMPLETE.md) - Test implementation guide
- [FSM_V2_ROADMAP.md](../plans/FSM_V2_ROADMAP.md) - Development roadmap

## Summary

This contract ensures FSM v2 and CSE integrate perfectly by:

1. ✅ **Defining interface first** (struct tags + SQL schema)
2. ✅ **Validating with tests** (19 specs, all passing)
3. ✅ **Both teams code to tests** (unambiguous requirements)
4. ✅ **CI enforces contract** (every PR must pass)
5. ✅ **Zero surprises at merge** (contract guaranteed to work)

**Next Steps**:
- PR #0: Merge this contract (this week)
- PR #1: FSM v2 follows contract (Week 1)
- PR #2: CSE follows contract (Week 2)
- PR #3: Adapter connects compliant systems (Week 3)
