// Package basic provides SQLite implementation of the Store interface.
//
// ⚠️ IMPLEMENTATION DEFERRED - Design Documentation Only ⚠️
//
// This file documents the design requirements for the SQLite backend implementation.
// A separate design session is needed to finalize WAL mode configuration, connection
// pooling strategy, and maintenance scheduling before implementing.
//
// WHY DEFERRED: SQLite configuration is highly sensitive to FSM performance requirements.
// Wrong settings (e.g., auto_vacuum=FULL, synchronous=FULL, no WAL mode) would cause
// unacceptable write latency and block FSM state transitions. Careful design needed.

package basic

// ============================================================================
// HIGH-LEVEL DESIGN REQUIREMENTS
// ============================================================================

// Target Use Case: FSM v2 Persistence + CSE Sync Engine
//
// Write Pattern:
//   - 10-100 FSM workers, each writing observed state every 1-10 seconds
//   - Sustained load: 10-1000 writes/second
//   - Burst load during CSE sync: 100-10000 writes/second
//   - Document size: 100-10KB typical (JSON-encoded FSM state)
//
// Read Pattern:
//   - FSM reads snapshot (identity + desired + observed) every tick
//   - Typically 3 reads per tick per worker (or 1 read if using LoadSnapshot JOIN)
//   - CSE sync reads for delta computation (WHERE _sync_id > last_synced)
//
// Latency Requirements:
//   - Write latency: <10ms p99, <5ms p50 (FSM cannot wait long)
//   - Read latency: <5ms p99 for single-document Get
//   - LoadSnapshot JOIN: <10ms p99 (critical path for FSM tick loop)
//
// Concurrency:
//   - 100+ concurrent writers (one per FSM worker goroutine)
//   - Readers and writers operate simultaneously
//   - No write-write conflicts (each worker owns its documents)
//
// Reliability:
//   - Must survive process crashes (durable writes)
//   - Must detect corruption (integrity checks)
//   - Must recover from power loss (WAL journaling)
//
// TRADE-OFF ANALYSIS:
// - SQLite is single-file, embedded, no separate server → simple deployment
// - But: single writer at a time without WAL mode → would block FSM workers
// - Solution: WAL mode enables concurrent readers + single writer
// - But: WAL checkpoints block writers briefly → need tuning

// ============================================================================
// CRITICAL DESIGN DECISION: WAL MODE
// ============================================================================

// REQUIREMENT: Enable Write-Ahead Logging (WAL) mode
//
// DESIGN DECISION: Use WAL mode instead of DELETE or TRUNCATE journal mode
// WHY:
//   1. Concurrent readers don't block writers (critical for FSM reads during writes)
//   2. Readers see consistent snapshot without blocking (MVCC-like behavior)
//   3. Writes append to WAL file, no need to update main database file immediately
//   4. Crash recovery is automatic (WAL replay on next open)
//
// TRADE-OFF:
//   1. Three files on disk (main DB, WAL file, shared-memory file) vs one file
//   2. Checkpointing needed to merge WAL into main DB (adds complexity)
//   3. Checkpoint operation blocks writers briefly (10-100ms typical)
//   4. WAL file grows unbounded if checkpoints don't run (disk space risk)
//
// INSPIRED BY: Linear's use of SQLite with WAL mode for offline-first sync,
// Litestream's recommendation for production SQLite deployments.
//
// SQLite Configuration:
//
//   PRAGMA journal_mode=WAL;
//   PRAGMA synchronous=NORMAL;  // Not FULL - see durability section below
//   PRAGMA busy_timeout=5000;   // Wait up to 5 seconds for locks
//   PRAGMA cache_size=-64000;   // 64MB page cache (negative = KB)
//
// References:
//   - https://www.sqlite.org/wal.html
//   - https://www.sqlite.org/pragma.html#pragma_journal_mode

// ============================================================================
// CHECKPOINT STRATEGY
// ============================================================================

// REQUIREMENT: Manage WAL checkpoint timing to avoid blocking FSM writers
//
// DESIGN DECISION: Use PRAGMA wal_autocheckpoint=1000 (default)
// WHY:
//   - SQLite automatically checkpoints when WAL reaches 1000 pages (~4MB)
//   - Checkpoint runs in background thread (doesn't block readers)
//   - But checkpoint blocks writers briefly when acquiring exclusive lock
//
// ALTERNATIVE CONSIDERED: Manual checkpoint triggering
//   - Run PRAGMA wal_checkpoint(PASSIVE) from separate goroutine every 60 seconds
//   - PASSIVE mode doesn't block readers or writers, just tries to checkpoint
//   - If busy, checkpoint skips and tries again later (non-blocking)
//
// TRADE-OFF:
//   - Auto-checkpoint (default): Simpler, but checkpoint timing is unpredictable
//   - Manual PASSIVE checkpoint: More control, but requires background goroutine
//   - Manual RESTART checkpoint: Forces checkpoint even if busy (blocks writers!)
//   - Manual TRUNCATE checkpoint: Resets WAL size to zero (blocks writers longer!)
//
// RECOMMENDATION: Start with auto-checkpoint=1000, add manual PASSIVE if needed
// Monitor write latency p99. If periodic spikes at 10-100ms, switch to manual PASSIVE.
//
// INSPIRED BY: PostgreSQL autovacuum scheduling, Linear's background sync tasks.
//
// References:
//   - https://www.sqlite.org/pragma.html#pragma_wal_autocheckpoint
//   - https://www.sqlite.org/c3ref/wal_checkpoint_v2.html

// ============================================================================
// DURABILITY VS PERFORMANCE TRADE-OFF
// ============================================================================

// REQUIREMENT: Balance crash recovery with write performance
//
// DESIGN DECISION: Use synchronous=NORMAL instead of synchronous=FULL
// WHY:
//   - synchronous=FULL: fsync after every write (30-50ms latency on HDD, 5-10ms on SSD)
//   - synchronous=NORMAL: fsync only at critical moments (WAL commit, checkpoint)
//   - Risk: Power loss during WAL commit loses last transaction
//   - Acceptable: FSM observed state is ephemeral (reconstructed on next poll)
//
// TRADE-OFF:
//   - synchronous=FULL: Maximum durability, but 5-10ms write latency on SSD
//   - synchronous=NORMAL: 1-2ms write latency, but small risk of data loss
//   - synchronous=OFF: <1ms write latency, but corruption risk after crash (NOT RECOMMENDED)
//
// DECISION: Use synchronous=NORMAL for FSM observed state (ephemeral, reconstructed)
//           Use synchronous=FULL for CSE desired state (user intent, must not lose)
//
// OPEN QUESTION: Can we set synchronous per-transaction?
//   - Answer: No, synchronous is database-wide setting
//   - Solution: Use two databases? One for ephemeral (NORMAL), one for durable (FULL)?
//   - Alternative: Accept synchronous=NORMAL for all, rely on WAL for durability
//
// INSPIRED BY: PostgreSQL synchronous_commit=off for async replicas,
// Linear's optimistic UI with pessimistic storage (eventual consistency).
//
// References:
//   - https://www.sqlite.org/pragma.html#pragma_synchronous
//   - https://www.sqlite.org/atomiccommit.html

// ============================================================================
// SCHEMA DESIGN
// ============================================================================

// REQUIREMENT: Store Documents (map[string]interface{}) in SQLite tables
//
// DESIGN DECISION: One table per collection, JSON BLOB for document data
//
// Schema:
//
//   CREATE TABLE IF NOT EXISTS <collection_name> (
//       id TEXT PRIMARY KEY,
//       data BLOB NOT NULL  -- JSON-encoded Document
//   )
//
// WHY:
//   - Simple schema, no need for ALTER TABLE when document structure changes
//   - JSON encoding handles nested maps, arrays, null values
//   - BLOB type stores bytes efficiently (no string escaping overhead)
//
// TRADE-OFF:
//   - Cannot index individual JSON fields without JSON1 extension
//   - Cannot use SQL WHERE clauses on document fields (must load and filter in Go)
//   - Layer 2 (CSE) can add indexed columns for common fields (_sync_id, _version)
//
// ALTERNATIVE CONSIDERED: SQLite JSON1 extension with virtual columns
//
//   CREATE TABLE <collection> (
//       id TEXT PRIMARY KEY,
//       data BLOB NOT NULL,
//       _sync_id INTEGER GENERATED ALWAYS AS (json_extract(data, '$._sync_id')) STORED,
//       _version INTEGER GENERATED ALWAYS AS (json_extract(data, '$._version')) STORED
//   )
//   CREATE INDEX idx_sync ON <collection>(_sync_id)
//
// WHY NOT: Adds complexity at Layer 1. Layer 2 should manage CSE-specific columns.
//
// INSPIRED BY: PostgreSQL JSONB columns, MongoDB document storage,
// Linear's flexible schema with indexed metadata fields.
//
// References:
//   - https://www.sqlite.org/json1.html
//   - https://www.sqlite.org/gencol.html

// ============================================================================
// ID GENERATION STRATEGY
// ============================================================================

// REQUIREMENT: Generate unique document IDs on Insert()
//
// DESIGN DECISION: Use UUID v4 (random) for document IDs
// WHY:
//   - Globally unique without coordination (no central ID server needed)
//   - Works in distributed CSE scenarios (edge/relay/frontend generate IDs independently)
//   - Random distribution avoids hotspots in indexes
//
// TRADE-OFF:
//   - 36 characters (32 hex + 4 hyphens) vs 8-byte integer (longer IDs)
//   - TEXT PRIMARY KEY slower than INTEGER PRIMARY KEY (string comparison)
//   - But: Flexibility for distributed systems outweighs performance cost
//
// ALTERNATIVE CONSIDERED: INTEGER AUTOINCREMENT
//   - Pros: Smaller IDs, faster lookups, sequential allocation
//   - Cons: Doesn't work in distributed systems (ID collisions)
//   - Verdict: Not suitable for CSE multi-tier sync
//
// ALTERNATIVE CONSIDERED: ULID (Universally Unique Lexicographically Sortable ID)
//   - Pros: UUID compatibility + timestamp prefix (sortable by creation time)
//   - Cons: Reveals creation time (privacy concern? probably not for FSM state)
//   - Verdict: Good option, but UUID v4 simpler for MVP
//
// IMPLEMENTATION:
//
//   import "github.com/google/uuid"
//
//   func (s *SQLiteStore) Insert(ctx context.Context, collection string, doc Document) (string, error) {
//       id := uuid.New().String()
//       // ... insert with id
//       return id, nil
//   }
//
// INSPIRED BY: Linear's custom base62 IDs (but UUID simpler for us),
// Stripe API's random IDs (sk_test_...).
//
// References:
//   - https://github.com/google/uuid
//   - https://github.com/ulid/spec

// ============================================================================
// CONNECTION POOLING (OR NOT)
// ============================================================================

// REQUIREMENT: Handle 100+ concurrent goroutines calling Store methods
//
// DESIGN DECISION: Use database/sql package with SetMaxOpenConns(1)
// WHY:
//   - SQLite has no built-in connection pooling (embedded database)
//   - Multiple connections to same database file cause lock contention in WAL mode
//   - Single connection + connection pool in database/sql serializes writes
//   - Readers can still operate concurrently (WAL mode allows this)
//
// TRADE-OFF:
//   - SetMaxOpenConns(1): All writes serialize through one connection
//   - But: Write operations are fast (<5ms), so serialization is acceptable
//   - Alternative: Multiple connections (SetMaxOpenConns(10)) for read concurrency
//   - But: Writes would still serialize at SQLite level (BUSY errors)
//
// OPEN QUESTION: Should we use SetMaxOpenConns(1) or SetMaxOpenConns(10)?
//   - Benchmark both configurations with 100 concurrent writers
//   - Measure BUSY error rate and write latency p99
//   - If BUSY errors are common, stick with SetMaxOpenConns(1)
//
// IMPLEMENTATION:
//
//   db, err := sql.Open("sqlite3", dbPath + "?cache=shared&mode=rwc&_journal_mode=WAL")
//   db.SetMaxOpenConns(1)     // Single writer connection
//   db.SetMaxIdleConns(1)     // Keep connection alive
//   db.SetConnMaxLifetime(0)  // No connection recycling
//
// INSPIRED BY: Litestream's recommendation for production SQLite,
// Tailscale's SQLite usage patterns (single writer, multiple readers).
//
// References:
//   - https://github.com/mattn/go-sqlite3#connection-string
//   - https://github.com/tailscale/tailscale/blob/main/ipn/store/sqlite/store.go

// ============================================================================
// VACUUM STRATEGY
// ============================================================================

// REQUIREMENT: Reclaim disk space without blocking FSM writes
//
// DESIGN DECISION: Manual VACUUM triggered during maintenance windows only
// WHY:
//   - VACUUM rewrites entire database, blocks all access (writers AND readers)
//   - Can take seconds to minutes depending on database size
//   - Running VACUUM during FSM operation would cause cascading timeouts
//
// TRADE-OFF:
//   - Disk space grows over time (deleted documents leave empty pages)
//   - Must schedule periodic VACUUM (weekly? monthly?) during low-traffic periods
//   - Alternative: auto_vacuum=INCREMENTAL (reclaims space incrementally)
//   - But: auto_vacuum adds overhead to every transaction (slower writes)
//
// ALTERNATIVE CONSIDERED: PRAGMA auto_vacuum=FULL
//   - Automatically reclaims space on DELETE
//   - But: Adds overhead to every DELETE operation (moves pages around)
//   - Verdict: Not suitable for high-write FSM use case
//
// ALTERNATIVE CONSIDERED: PRAGMA auto_vacuum=INCREMENTAL
//   - Reclaims space in small chunks when explicitly triggered
//   - PRAGMA incremental_vacuum(N) reclaims N pages at a time
//   - Could run from background goroutine every hour
//   - But: Still adds overhead, needs testing
//
// RECOMMENDATION: Start with auto_vacuum=NONE, manual VACUUM monthly
// Monitor database file size. If growth is problematic, consider incremental vacuum.
//
// IMPLEMENTATION NOTE: Expose Maintenance() method on Store interface?
//
//   type Store interface {
//       // ... existing methods
//       Maintenance(ctx context.Context, opts MaintenanceOptions) error
//   }
//
//   type MaintenanceOptions struct {
//       Vacuum bool  // Run VACUUM (blocks all access)
//       Analyze bool // Update query planner statistics
//   }
//
// INSPIRED BY: PostgreSQL VACUUM scheduling, MySQL OPTIMIZE TABLE,
// Linear's maintenance window scheduling for database operations.
//
// References:
//   - https://www.sqlite.org/lang_vacuum.html
//   - https://www.sqlite.org/pragma.html#pragma_auto_vacuum

// ============================================================================
// ERROR HANDLING
// ============================================================================

// REQUIREMENT: Map SQLite errors to Store interface errors
//
// DESIGN DECISION: Detect specific SQLite error codes and return sentinel errors
//
// Mapping:
//   - sqlite3.ErrConstraintPrimaryKey → basic.ErrConflict (duplicate ID)
//   - sqlite3.ErrBusy → retry with exponential backoff (temporary lock contention)
//   - sqlite3.ErrLocked → retry with exponential backoff (database locked)
//   - sql.ErrNoRows → basic.ErrNotFound (document doesn't exist)
//
// TRADE-OFF:
//   - Error mapping adds complexity (need to import mattn/go-sqlite3)
//   - But: Provides consistent error handling across backends
//   - Alternative: Return raw SQLite errors (leaks implementation details)
//
// IMPLEMENTATION:
//
//   import "github.com/mattn/go-sqlite3"
//
//   func mapSQLiteError(err error) error {
//       var sqliteErr sqlite3.Error
//       if errors.As(err, &sqliteErr) {
//           switch sqliteErr.ExtendedCode {
//           case sqlite3.ErrConstraintPrimaryKey:
//               return basic.ErrConflict
//           case sqlite3.ErrBusy, sqlite3.ErrLocked:
//               // Retry logic here
//           }
//       }
//       if errors.Is(err, sql.ErrNoRows) {
//           return basic.ErrNotFound
//       }
//       return err
//   }
//
// References:
//   - https://www.sqlite.org/rescode.html
//   - https://pkg.go.dev/github.com/mattn/go-sqlite3#Error

// ============================================================================
// TRANSACTION IMPLEMENTATION
// ============================================================================

// REQUIREMENT: Implement BeginTx() with proper isolation
//
// DESIGN DECISION: Use database/sql.Tx with DEFERRED transactions
// WHY:
//   - DEFERRED: Transaction doesn't acquire lock until first write
//   - Allows multiple read-only transactions concurrently
//   - Write transaction upgrades to exclusive lock when needed
//
// TRADE-OFF:
//   - IMMEDIATE: Acquires write lock immediately (prevents BUSY errors later)
//   - EXCLUSIVE: Acquires exclusive lock (blocks all other transactions)
//   - DEFERRED: Best for FSM use case (mostly reads, occasional writes)
//
// IMPLEMENTATION:
//
//   type sqliteTx struct {
//       tx *sql.Tx
//       store *SQLiteStore
//   }
//
//   func (s *SQLiteStore) BeginTx(ctx context.Context) (Tx, error) {
//       tx, err := s.db.BeginTx(ctx, &sql.TxOptions{
//           Isolation: sql.LevelDefault, // Maps to DEFERRED in SQLite
//       })
//       if err != nil {
//           return nil, err
//       }
//       return &sqliteTx{tx: tx, store: s}, nil
//   }
//
//   func (t *sqliteTx) Insert(ctx context.Context, collection string, doc Document) (string, error) {
//       // Use t.tx instead of t.store.db
//       // ... same logic as SQLiteStore.Insert but with transaction connection
//   }
//
// References:
//   - https://www.sqlite.org/lang_transaction.html
//   - https://pkg.go.dev/database/sql#TxOptions

// ============================================================================
// TESTING STRATEGY
// ============================================================================

// REQUIREMENT: Verify SQLite backend satisfies Store interface contract
//
// Tests to implement:
//   1. Basic CRUD (Create, Insert, Get, Update, Delete)
//   2. Error cases (duplicate ID, not found, invalid collection name)
//   3. Concurrency (100 goroutines writing simultaneously)
//   4. Transactions (commit, rollback, isolation)
//   5. WAL checkpoint (manual trigger, verify no data loss)
//   6. Crash recovery (kill process mid-write, verify WAL replay)
//   7. Performance (write latency p50/p99, sustained throughput)
//
// Benchmark tests:
//   - BenchmarkInsert: Single-threaded insert throughput
//   - BenchmarkInsertConcurrent: 100 goroutines inserting simultaneously
//   - BenchmarkGet: Single-document read latency
//   - BenchmarkLoadSnapshot: JOIN query for FSM snapshot
//
// Chaos tests:
//   - Random process kills during writes (verify no corruption)
//   - Disk full errors (verify graceful degradation)
//   - Concurrent checkpoint + writes (verify no deadlocks)
//
// References:
//   - Use t.TempDir() for test databases (automatic cleanup)
//   - Use testcontainers for integration tests (if needed)
//   - Use pprof for profiling bottlenecks

// ============================================================================
// OPEN QUESTIONS FOR DESIGN SESSION
// ============================================================================

// 1. Connection Pooling: SetMaxOpenConns(1) or SetMaxOpenConns(10)?
//    → Need to benchmark both with 100 concurrent writers
//    → Measure BUSY error rate and write latency distribution
//
// 2. Synchronous Setting: Can we use NORMAL for ephemeral, FULL for durable?
//    → If not, should we use two separate databases?
//    → Or accept NORMAL for all and rely on WAL durability?
//
// 3. Checkpoint Strategy: Auto-checkpoint=1000 or manual PASSIVE every 60s?
//    → Need to measure write latency spikes during checkpoint
//    → If p99 > 10ms, switch to manual PASSIVE
//
// 4. Vacuum Strategy: Manual VACUUM monthly or auto_vacuum=INCREMENTAL?
//    → Need to measure disk space growth rate with typical FSM workload
//    → If growth is <1GB/month, manual VACUUM is fine
//
// 5. Query Implementation: How to implement Find() without JSON1 extension?
//    → Option A: Load all documents, filter in Go (simple but inefficient)
//    → Option B: Use JSON1 extension with WHERE json_extract(...) (complex but fast)
//    → Option C: Layer 2 adds indexed columns for common filters (hybrid approach)
//
// 6. Maintenance API: Should Store interface expose Maintenance() method?
//    → Allows scheduling VACUUM, ANALYZE from higher layers
//    → But breaks abstraction (other backends might not need maintenance)
//
// 7. Corruption Detection: Should we run PRAGMA integrity_check on startup?
//    → Adds 1-10 seconds to startup time depending on database size
//    → But detects corruption early before it spreads
//
// 8. Backup Strategy: Should SQLiteStore provide Backup() method?
//    → SQLite has online backup API (backup while database is in use)
//    → Useful for CSE snapshot creation before sync
//
// ============================================================================
// IMPLEMENTATION CHECKLIST (For Future Developer)
// ============================================================================

// TODO: Implement NewSQLiteStore(dbPath string) constructor
//   - Open database with WAL mode, synchronous=NORMAL, busy_timeout=5000
//   - Set connection pool limits (SetMaxOpenConns, SetMaxIdleConns)
//   - Run PRAGMA integrity_check on first open (optional, measure startup time)
//
// TODO: Implement CreateCollection(name, schema)
//   - Validate collection name (alphanumeric + underscore only, SQL injection prevention)
//   - CREATE TABLE IF NOT EXISTS <name> (id TEXT PRIMARY KEY, data BLOB NOT NULL)
//   - Ignore schema parameter for now (Layer 2 will add CSE columns)
//
// TODO: Implement Insert(collection, doc)
//   - Generate UUID: id := uuid.New().String()
//   - Marshal doc to JSON: data, err := json.Marshal(doc)
//   - INSERT INTO <collection> (id, data) VALUES (?, ?)
//   - Return id on success, map errors (constraint → ErrConflict)
//
// TODO: Implement Get(collection, id)
//   - SELECT data FROM <collection> WHERE id = ?
//   - Unmarshal JSON: json.Unmarshal(data, &doc)
//   - Return ErrNotFound if sql.ErrNoRows
//
// TODO: Implement Update(collection, id, doc)
//   - Marshal doc to JSON
//   - UPDATE <collection> SET data = ? WHERE id = ?
//   - Return ErrNotFound if RowsAffected == 0
//
// TODO: Implement Delete(collection, id)
//   - DELETE FROM <collection> WHERE id = ?
//   - Return ErrNotFound if RowsAffected == 0
//
// TODO: Implement DropCollection(name)
//   - DROP TABLE IF EXISTS <name>
//
// TODO: Implement Find(collection, query)
//   - For MVP: SELECT * FROM <collection> and filter in Go
//   - For production: Use JSON1 extension with WHERE json_extract(...)
//
// TODO: Implement BeginTx()
//   - db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelDefault})
//   - Return sqliteTx wrapper
//
// TODO: Implement sqliteTx struct
//   - Embed Store interface methods, use tx.Exec/tx.Query instead of db
//   - Implement Commit() and Rollback()
//
// TODO: Implement Close()
//   - db.Close()
//
// TODO: Add comprehensive tests (see TESTING STRATEGY above)
//
// TODO: Add benchmarks (measure write latency p50/p99, throughput)
//
// TODO: Document performance tuning based on benchmark results

// ============================================================================
// DEPENDENCIES
// ============================================================================

// Required Go modules (add to go.mod):
//   github.com/mattn/go-sqlite3 v1.14.22
//   github.com/google/uuid v1.6.0

// CGO requirement:
//   SQLite driver requires CGO (gcc must be available)
//   Cross-compilation needs CGO_ENABLED=1 and target-specific gcc
//   Consider using modernc.org/sqlite (pure Go) if CGO is problematic

// ============================================================================
// REFERENCES
// ============================================================================

// SQLite Documentation:
//   - WAL Mode: https://www.sqlite.org/wal.html
//   - PRAGMA Reference: https://www.sqlite.org/pragma.html
//   - Atomic Commit: https://www.sqlite.org/atomiccommit.html
//   - JSON1 Extension: https://www.sqlite.org/json1.html
//
// Go SQLite Driver:
//   - mattn/go-sqlite3: https://github.com/mattn/go-sqlite3
//   - Connection Strings: https://github.com/mattn/go-sqlite3#connection-string
//
// Best Practices:
//   - Litestream Guide: https://litestream.io/guides/
//   - Tailscale SQLite Usage: https://github.com/tailscale/tailscale/blob/main/ipn/store/sqlite/store.go
//   - rqlite (distributed SQLite): https://github.com/rqlite/rqlite
//
// Linear Sync Engine:
//   - Reverse Engineering: https://github.com/wzhudev/reverse-linear-sync-engine
//   - Offline-First Sync: https://linear.app/blog/how-we-built-linear-sync-engine

// End of design documentation
