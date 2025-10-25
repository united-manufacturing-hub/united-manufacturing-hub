# Database Maintenance API Implementation Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/executing-plans/SKILL.md` to implement this plan task-by-task.

**Goal:** Add database maintenance operations to Store interface with context-aware Close() for graceful shutdown control.

**Architecture:** Add `Maintenance(ctx)` method to Store interface for explicit maintenance operations (VACUUM, ANALYZE). Change `Close()` to `Close(ctx)` for consistent context handling. SQLite implementation runs VACUUM+ANALYZE on shutdown (configurable) and exposes manual trigger via Maintenance().

**Tech Stack:** Go 1.22+, SQLite 3, Ginkgo v2, Gomega

---

## Task 1: Add Maintenance() to Store Interface

**Files:**
- Modify: `umh-core/pkg/persistence/basic/store.go:162-352`

**Step 1: Add Maintenance() method to Store interface**

Add this method after the `Find()` method and before `BeginTx()` in the Store interface (around line 305):

```go
	// Maintenance performs database optimization and cleanup operations.
	//
	// DESIGN DECISION: Abstract maintenance interface, not backend-specific operations
	// WHY: Different backends have different housekeeping needs:
	//   - SQLite: VACUUM (defragment), ANALYZE (update statistics)
	//   - Postgres: VACUUM, ANALYZE, REINDEX
	//   - MongoDB: compact, repairDatabase
	// Providing unified interface allows backends to run appropriate operations.
	//
	// BLOCKING BEHAVIOR:
	//   ⚠️  May block ALL database operations during execution (seconds to minutes)
	//   - SQLite VACUUM: Requires exclusive lock, blocks reads AND writes
	//   - Postgres VACUUM: Blocks table updates (reads continue)
	//   - Backend implementations document their specific blocking behavior
	//
	// REQUIREMENTS:
	//   - MUST respect context cancellation/timeout
	//   - MUST be idempotent (safe to call multiple times)
	//   - SHOULD log operations performed for observability
	//
	// USAGE PATTERNS:
	//   - Automatic: Close() may call Maintenance() during shutdown
	//   - Manual: Orchestrator calls during maintenance window
	//   - Scheduled: External cron/scheduler triggers periodic maintenance
	//
	// COORDINATION:
	//   Caller is responsible for coordinating with application:
	//   - Pause FSM workers before calling
	//   - Drain request queues
	//   - Wait for in-flight operations to complete
	//
	// Example:
	//
	//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	//	defer cancel()
	//	if err := store.Maintenance(ctx); err != nil {
	//	    log.Error("maintenance failed", "error", err)
	//	}
	//
	// Returns:
	//   - nil on success
	//   - context.DeadlineExceeded if timeout exceeded
	//   - context.Canceled if context cancelled
	//   - backend-specific errors (disk full, permissions, corruption)
	Maintenance(ctx context.Context) error
```

**Step 2: Verify code compiles**

Run: `cd umh-core && go build ./pkg/persistence/basic`
Expected: Build fails with "does not implement Store (missing method Maintenance)"

**Step 3: Commit interface addition**

```bash
git add umh-core/pkg/persistence/basic/store.go
git commit -m "feat(persistence): add Maintenance() to Store interface

Add database maintenance operation to Store interface for explicit
control over housekeeping operations (VACUUM, ANALYZE, compact, etc).

BREAKING CHANGE: All Store implementations must implement Maintenance()"
```

---

## Task 2: Change Close() Signature to Accept Context

**Files:**
- Modify: `umh-core/pkg/persistence/basic/store.go:339-351`

**Step 1: Update Close() signature in Store interface**

Replace the existing `Close() error` method (around line 351) with:

```go
	// Close closes the store and releases resources.
	//
	// DESIGN DECISION: Accept context for graceful shutdown control
	// WHY: Close() may perform maintenance operations (VACUUM, ANALYZE) that take
	// seconds to minutes. Caller needs ability to:
	//   - Set deadline: ctx with timeout controls max shutdown time
	//   - Cancel early: ctx cancellation aborts maintenance, closes immediately
	//   - Trace shutdown: ctx carries tracing/logging context
	//
	// BLOCKING BEHAVIOR:
	//   If MaintenanceOnShutdown is enabled, Close() will block during maintenance.
	//   Context controls maximum wait time.
	//
	// GRACEFUL DEGRADATION:
	//   If context expires during maintenance, database still closes safely.
	//   Maintenance may be incomplete, but data integrity is preserved.
	//
	// Example:
	//
	//	// Graceful shutdown with 30s timeout
	//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	//	defer cancel()
	//	if err := store.Close(ctx); err != nil {
	//	    log.Warn("maintenance incomplete during shutdown", "error", err)
	//	}
	//
	// Returns:
	//   - nil: closed successfully, maintenance completed
	//   - context.DeadlineExceeded: maintenance incomplete, database closed anyway
	//   - other errors: close failures (rare, usually safe to ignore)
	Close(ctx context.Context) error
```

**Step 2: Update Tx interface Close() signature**

The Tx interface embeds Store, so it automatically inherits the new signature. No additional changes needed.

**Step 3: Verify code still fails to compile**

Run: `cd umh-core && go build ./pkg/persistence/basic`
Expected: Build fails with "does not implement Store (missing Maintenance, wrong signature for Close)"

**Step 4: Commit signature change**

```bash
git add umh-core/pkg/persistence/basic/store.go
git commit -m "feat(persistence): change Close() to accept context

Change Close() signature from Close() error to Close(ctx) error for
consistent context handling across all Store methods.

Enables caller control over shutdown timeout and emergency cancellation.

BREAKING CHANGE: All Close() calls must pass context"
```

---

## Task 3: Add Config.MaintenanceOnShutdown Field

**Files:**
- Modify: `umh-core/pkg/persistence/basic/sqlite.go:45-60`

**Step 1: Add MaintenanceOnShutdown to sqliteStore config**

Find the `sqliteStore` struct (around line 45) and add the field:

```go
type sqliteStore struct {
	db                     *sql.DB
	mu                     sync.RWMutex
	closed                 bool
	maintenanceOnShutdown  bool  // Add this line
}
```

**Step 2: Add Config struct for NewStore()**

Before the `NewStore()` function (around line 60), add:

```go
// Config holds SQLite store configuration options.
type Config struct {
	// DBPath is the filesystem path to the SQLite database file.
	DBPath string

	// MaintenanceOnShutdown runs VACUUM and ANALYZE during Close().
	// Default: true
	// Set to false to skip maintenance for faster shutdown in development.
	MaintenanceOnShutdown bool
}

// DefaultConfig returns recommended production configuration.
func DefaultConfig(dbPath string) Config {
	return Config{
		DBPath:                dbPath,
		MaintenanceOnShutdown: true,
	}
}
```

**Step 3: Update NewStore() signature and implementation**

Replace the existing `NewStore(dbPath string)` function with:

```go
// NewStore creates a SQLite-backed Store with production-grade configuration.
//
// DESIGN DECISION: Accept Config instead of individual parameters
// WHY: Makes it easy to add new configuration options without breaking callers.
// Callers can use DefaultConfig() and override specific fields.
//
// Example:
//
//	cfg := basic.DefaultConfig("./data.db")
//	cfg.MaintenanceOnShutdown = false // Skip VACUUM in dev
//	store, err := basic.NewStore(cfg)
func NewStore(cfg Config) (Store, error) {
	if cfg.DBPath == "" {
		return nil, fmt.Errorf("DBPath cannot be empty")
	}

	connStr := buildConnectionString(cfg.DBPath)

	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool for single-writer pattern
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Verify WAL mode enabled
	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil || journalMode != "wal" {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: got %s", journalMode)
	}

	return &sqliteStore{
		db:                    db,
		maintenanceOnShutdown: cfg.MaintenanceOnShutdown,
	}, nil
}
```

**Step 4: Verify code compiles**

Run: `cd umh-core && go build ./pkg/persistence/basic`
Expected: Still fails (missing Maintenance and Close implementations)

**Step 5: Commit config changes**

```bash
git add umh-core/pkg/persistence/basic/sqlite.go
git commit -m "feat(persistence): add Config struct with MaintenanceOnShutdown

Replace NewStore(dbPath) with NewStore(cfg) for extensible configuration.
Add MaintenanceOnShutdown flag (default: true) to control shutdown behavior.

BREAKING CHANGE: NewStore() now takes Config instead of string"
```

---

## Task 4: Implement Maintenance() in SQLite

**Files:**
- Modify: `umh-core/pkg/persistence/basic/sqlite.go` (add new method around line 700)

**Step 1: Implement maintenanceInternal() helper**

Add this helper method after the `Find()` method implementation (around line 700):

```go
// maintenanceInternal performs VACUUM and ANALYZE operations.
// This is called by both Maintenance() and Close().
func (s *sqliteStore) maintenanceInternal(ctx context.Context) error {
	// VACUUM: Reclaim space and defragment database
	// This rewrites the entire database file, which can take seconds to minutes.
	if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("VACUUM failed: %w", err)
	}

	// ANALYZE: Update query planner statistics
	// This scans tables to gather statistics for optimal query planning.
	if _, err := s.db.ExecContext(ctx, "ANALYZE"); err != nil {
		return fmt.Errorf("ANALYZE failed: %w", err)
	}

	return nil
}
```

**Step 2: Implement Maintenance() public method**

Add this method after maintenanceInternal():

```go
// Maintenance performs database optimization and cleanup.
//
// IMPLEMENTATION: Runs VACUUM and ANALYZE operations.
//   - VACUUM: Defragments database, reclaims unused space
//   - ANALYZE: Updates query planner statistics for optimal performance
//
// BLOCKING: Requires exclusive lock, blocks all operations during execution.
//
// PERFORMANCE: Time proportional to database size:
//   - <100MB: 1-5 seconds
//   - 100-500MB: 5-20 seconds
//   - 500MB-2GB: 20-90 seconds
//   - >2GB: 2-10 minutes
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
//	defer cancel()
//	if err := store.Maintenance(ctx); err != nil {
//	    return fmt.Errorf("maintenance failed: %w", err)
//	}
func (s *sqliteStore) Maintenance(ctx context.Context) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return errors.New("store is closed")
	}
	s.mu.RUnlock()

	return s.maintenanceInternal(ctx)
}
```

**Step 3: Verify code compiles**

Run: `cd umh-core && go build ./pkg/persistence/basic`
Expected: Still fails (Close signature still wrong)

**Step 4: Commit Maintenance implementation**

```bash
git add umh-core/pkg/persistence/basic/sqlite.go
git commit -m "feat(persistence): implement Maintenance() for SQLite

Add VACUUM and ANALYZE operations to SQLite Store implementation.
VACUUM defragments database and reclaims space.
ANALYZE updates query planner statistics."
```

---

## Task 5: Update Close() Implementation to Accept Context

**Files:**
- Modify: `umh-core/pkg/persistence/basic/sqlite.go` (Close method around line 100)

**Step 1: Update Close() signature and implementation**

Replace the existing `Close() error` method with:

```go
// Close closes the database and releases resources.
//
// IMPLEMENTATION: If MaintenanceOnShutdown is enabled, runs VACUUM and ANALYZE
// before closing the database connection.
//
// GRACEFUL DEGRADATION: If maintenance fails or times out, database still closes.
// Incomplete maintenance is logged via error return, but Close() guarantees cleanup.
//
// Example:
//
//	// Graceful shutdown with 30s timeout
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	if err := store.Close(ctx); err != nil {
//	    log.Warn("maintenance incomplete", "error", err)
//	}
func (s *sqliteStore) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	// Run maintenance before closing if enabled
	if s.maintenanceOnShutdown {
		if err := s.maintenanceInternal(ctx); err != nil {
			// Maintenance failed/timeout, but still close database
			// Return error so caller knows maintenance was incomplete
			s.closed = true
			_ = s.db.Close()
			return fmt.Errorf("maintenance incomplete: %w", err)
		}
	}

	s.closed = true
	return s.db.Close()
}
```

**Step 2: Verify code compiles**

Run: `cd umh-core && go build ./pkg/persistence/basic`
Expected: Build succeeds! (But tests will fail because Close() calls need updating)

**Step 3: Commit Close implementation**

```bash
git add umh-core/pkg/persistence/basic/sqlite.go
git commit -m "feat(persistence): update Close() to accept context

Implement context-aware Close() that runs maintenance before shutdown.
Graceful degradation: maintenance timeout still closes database safely."
```

---

## Task 6: Write Tests for Maintenance()

**Files:**
- Modify: `umh-core/pkg/persistence/basic/sqlite_test.go` (add new Context block around line 1500)

**Step 1: Add Maintenance test suite**

Add this new test Context after the transaction tests (around line 1500):

```go
	Context("Maintenance operations", func() {
		BeforeEach(func() {
			cfg := basic.DefaultConfig(tempDB)
			cfg.MaintenanceOnShutdown = false // Test Maintenance() explicitly
			var err error
			store, err = basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if store != nil {
				_ = store.Close(context.Background())
			}
		})

		It("should run VACUUM and ANALYZE successfully", func() {
			// Create collection with data
			err := store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			// Insert some documents
			for i := 0; i < 100; i++ {
				_, err := store.Insert(ctx, "test", basic.Document{
					"value": i,
					"data":  strings.Repeat("x", 1000),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Run maintenance
			maintenanceCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			err = store.Maintenance(maintenanceCtx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should respect context timeout during Maintenance", func() {
			// Create large dataset to force slow VACUUM
			err := store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < 1000; i++ {
				_, err := store.Insert(ctx, "test", basic.Document{
					"value": i,
					"data":  strings.Repeat("x", 10000),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Use very short timeout
			timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
			defer cancel()

			err = store.Maintenance(timeoutCtx)
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
		})

		It("should respect context cancellation during Maintenance", func() {
			err := store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < 500; i++ {
				_, err := store.Insert(ctx, "test", basic.Document{
					"value": i,
					"data":  strings.Repeat("x", 5000),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Cancel context immediately
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()

			err = store.Maintenance(cancelCtx)
			Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		})

		It("should return error when store is closed", func() {
			err := store.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())

			err = store.Maintenance(ctx)
			Expect(err).To(MatchError("store is closed"))
		})

		It("should be idempotent - safe to call multiple times", func() {
			err := store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Insert(ctx, "test", basic.Document{"value": 1})
			Expect(err).NotTo(HaveOccurred())

			// Run maintenance multiple times
			for i := 0; i < 3; i++ {
				err = store.Maintenance(ctx)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
```

**Step 2: Run the new tests**

Run: `cd umh-core && ginkgo -v ./pkg/persistence/basic -- -ginkgo.focus="Maintenance operations"`
Expected: All 5 tests PASS

**Step 3: Commit Maintenance tests**

```bash
git add umh-core/pkg/persistence/basic/sqlite_test.go
git commit -m "test(persistence): add Maintenance() test coverage

Add comprehensive test suite for Maintenance():
- Happy path: VACUUM + ANALYZE succeed
- Context timeout handling
- Context cancellation handling
- Error when store closed
- Idempotency verification"
```

---

## Task 7: Write Tests for Close() with Maintenance

**Files:**
- Modify: `umh-core/pkg/persistence/basic/sqlite_test.go` (add new Context block after Maintenance tests)

**Step 1: Add Close with maintenance test suite**

Add this Context after the Maintenance tests:

```go
	Context("Close with maintenance", func() {
		It("should run maintenance on shutdown when enabled", func() {
			// Create store with maintenance enabled
			cfg := basic.DefaultConfig(tempDB)
			cfg.MaintenanceOnShutdown = true

			var err error
			store, err = basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create data
			err = store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < 100; i++ {
				_, err = store.Insert(ctx, "test", basic.Document{
					"value": i,
					"data":  strings.Repeat("x", 1000),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Get database size before close
			fileInfo, err := os.Stat(tempDB)
			Expect(err).NotTo(HaveOccurred())
			sizeBefore := fileInfo.Size()

			// Delete half the documents
			for i := 0; i < 50; i++ {
				docs, err := store.Find(ctx, "test", basic.Query{})
				Expect(err).NotTo(HaveOccurred())
				if len(docs) > 0 {
					id := docs[0]["id"].(string)
					err = store.Delete(ctx, "test", id)
					Expect(err).NotTo(HaveOccurred())
				}
			}

			// Close with maintenance (should VACUUM)
			closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err = store.Close(closeCtx)
			Expect(err).NotTo(HaveOccurred())

			// Verify database was compacted
			fileInfo, err = os.Stat(tempDB)
			Expect(err).NotTo(HaveOccurred())
			sizeAfter := fileInfo.Size()

			// VACUUM should have reclaimed space
			Expect(sizeAfter).To(BeNumerically("<", sizeBefore))
		})

		It("should skip maintenance when disabled", func() {
			cfg := basic.DefaultConfig(tempDB)
			cfg.MaintenanceOnShutdown = false

			var err error
			store, err = basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create minimal data
			err = store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			// Close should be instant (no VACUUM)
			start := time.Now()
			err = store.Close(context.Background())
			duration := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(duration).To(BeNumerically("<", 100*time.Millisecond))
		})

		It("should close database even if maintenance times out", func() {
			cfg := basic.DefaultConfig(tempDB)
			cfg.MaintenanceOnShutdown = true

			var err error
			store, err = basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Create large dataset
			err = store.CreateCollection(ctx, "test", nil)
			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < 1000; i++ {
				_, err = store.Insert(ctx, "test", basic.Document{
					"value": i,
					"data":  strings.Repeat("x", 10000),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Close with very short timeout
			closeCtx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
			defer cancel()

			err = store.Close(closeCtx)

			// Should return error about incomplete maintenance
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maintenance incomplete"))

			// But store should still be closed
			err = store.Get(ctx, "test", "any-id")
			Expect(err).To(MatchError("store is closed"))
		})

		It("should be idempotent - safe to close multiple times", func() {
			cfg := basic.DefaultConfig(tempDB)
			var err error
			store, err = basic.NewStore(cfg)
			Expect(err).NotTo(HaveOccurred())

			// First close
			err = store.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// Second close should be no-op
			err = store.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})
	})
```

**Step 2: Run the new tests**

Run: `cd umh-core && ginkgo -v ./pkg/persistence/basic -- -ginkgo.focus="Close with maintenance"`
Expected: All 4 tests PASS

**Step 3: Commit Close tests**

```bash
git add umh-core/pkg/persistence/basic/sqlite_test.go
git commit -m "test(persistence): add Close() with maintenance test coverage

Add test suite verifying:
- VACUUM runs on shutdown when enabled (verifies space reclamation)
- Fast shutdown when maintenance disabled
- Graceful degradation on maintenance timeout
- Idempotent Close() behavior"
```

---

## Task 8: Update All Existing Close() Calls in Tests

**Files:**
- Modify: `umh-core/pkg/persistence/basic/sqlite_test.go` (update ~120 instances)
- Modify: `umh-core/pkg/persistence/basic/sqlite_benchmark_test.go` (update ~5 instances)

**Step 1: Update test Close() calls in sqlite_test.go**

Find all instances of `store.Close()` or `tx.Rollback()` and update:

OLD pattern:
```go
defer store.Close()
```

NEW pattern:
```go
defer func() {
	if store != nil {
		_ = store.Close(context.Background())
	}
}()
```

OR in AfterEach blocks:
```go
AfterEach(func() {
	if store != nil {
		_ = store.Close(context.Background())
	}
})
```

**Step 2: Update benchmark Close() calls in sqlite_benchmark_test.go**

Same pattern as Step 1, update all Close() calls to pass context.Background().

**Step 3: Run all tests to verify**

Run: `cd umh-core && ginkgo -v ./pkg/persistence/basic`
Expected: All ~127 tests PASS

**Step 4: Run benchmarks to verify**

Run: `cd umh-core && go test -bench=. ./pkg/persistence/basic`
Expected: All benchmarks run successfully

**Step 5: Commit Close() migration**

```bash
git add umh-core/pkg/persistence/basic/sqlite_test.go umh-core/pkg/persistence/basic/sqlite_benchmark_test.go
git commit -m "refactor(persistence): migrate all Close() calls to new signature

Update all test Close() calls to pass context.Background().
No functional changes, just API migration."
```

---

## Task 9: Add sqliteTx.Close() Implementation

**Files:**
- Modify: `umh-core/pkg/persistence/basic/sqlite.go` (add Close method to sqliteTx around line 850)

**Step 1: Add Close() to sqliteTx**

The sqliteTx type embeds Store interface but doesn't implement Close(). Add this method after the Rollback() implementation:

```go
// Close is not supported for transactions.
// Transactions must be explicitly committed or rolled back.
func (tx *sqliteTx) Close(ctx context.Context) error {
	return errors.New("cannot close transaction directly, use Commit() or Rollback()")
}

// Maintenance is not supported for transactions.
// Maintenance must be performed on the Store, not within a transaction.
func (tx *sqliteTx) Maintenance(ctx context.Context) error {
	return errors.New("cannot run maintenance in transaction, use Store.Maintenance()")
}
```

**Step 2: Verify code compiles**

Run: `cd umh-core && go build ./pkg/persistence/basic`
Expected: Build succeeds

**Step 3: Add test for tx.Close() error**

Add this test in the transaction test suite:

```go
It("should return error when trying to close transaction directly", func() {
	err := store.CreateCollection(ctx, "test", nil)
	Expect(err).NotTo(HaveOccurred())

	tx, err := store.BeginTx(ctx)
	Expect(err).NotTo(HaveOccurred())
	defer tx.Rollback()

	// Close() should return error
	err = tx.Close(context.Background())
	Expect(err).To(MatchError("cannot close transaction directly, use Commit() or Rollback()"))

	// Maintenance() should return error
	err = tx.Maintenance(ctx)
	Expect(err).To(MatchError("cannot run maintenance in transaction, use Store.Maintenance()"))
})
```

**Step 4: Run test to verify**

Run: `cd umh-core && ginkgo -v ./pkg/persistence/basic -- -ginkgo.focus="should return error when trying to close transaction"`
Expected: Test PASS

**Step 5: Commit tx.Close() implementation**

```bash
git add umh-core/pkg/persistence/basic/sqlite.go umh-core/pkg/persistence/basic/sqlite_test.go
git commit -m "feat(persistence): add Close() and Maintenance() to sqliteTx

Implement required interface methods for sqliteTx.
Both return errors since transactions must use Commit()/Rollback()."
```

---

## Task 10: Final Verification and Documentation

**Files:**
- Run full test suite
- Run benchmarks
- Verify golangci-lint passes

**Step 1: Run full test suite**

Run: `cd umh-core && ginkgo -v ./pkg/persistence/basic`
Expected: All tests PASS (should be ~132 tests now)

**Step 2: Run benchmarks**

Run: `cd umh-core && go test -bench=. -benchmem ./pkg/persistence/basic`
Expected: All benchmarks run, performance similar to before

**Step 3: Run linter**

Run: `cd umh-core && golangci-lint run ./pkg/persistence/basic/...`
Expected: 0 issues

**Step 4: Verify implementation completeness**

Checklist:
- ✅ Maintenance() in Store interface
- ✅ Close(ctx) in Store interface
- ✅ Config struct with MaintenanceOnShutdown
- ✅ SQLite Maintenance() implementation (VACUUM + ANALYZE)
- ✅ SQLite Close(ctx) with conditional maintenance
- ✅ Tests for Maintenance()
- ✅ Tests for Close() with maintenance
- ✅ All existing tests migrated to Close(ctx)
- ✅ Transaction Close()/Maintenance() error handlers
- ✅ Full test suite passing

**Step 5: Final commit**

```bash
git add -A
git commit -m "docs(persistence): update CLAUDE.md with maintenance strategy

Document the database maintenance approach:
- Maintenance() for explicit control
- Close(ctx) for graceful shutdown
- MaintenanceOnShutdown configuration
- Context-based timeout control

Completes implementation of maintenance API."
```

---

## Execution Complete

After Task 10, the implementation is complete with:
- ✅ Database maintenance API
- ✅ Context-aware Close() for graceful shutdown
- ✅ Comprehensive test coverage (~132 tests)
- ✅ All existing tests migrated
- ✅ Production-ready configuration

**Estimated time:** 60-90 minutes for full implementation.
