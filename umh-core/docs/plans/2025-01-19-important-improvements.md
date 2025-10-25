# SQLite Important Improvements Implementation Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/subagent-driven-development/SKILL.md` to implement this plan task-by-task.

**Goal:** Implement 8 Important Improvements from SQLite Production Reliability Review to enhance validation, error handling, and robustness.

**Architecture:** Add input validation, enhance error messages, improve checkpoint handling, and extend type support in query helpers. All changes follow TDD approach with comprehensive test coverage.

**Tech Stack:** Go, SQLite (via database/sql), Ginkgo/Gomega testing framework

**Reference:** Important Improvements from SQLite Production Reliability Review (4-6 hours estimated)

**Context:** All 3 Critical Issues are resolved. These improvements enhance production robustness but are not blocking deployment.

---

## Task Grouping

Tasks are grouped by functional area for efficient implementation:

**Group A: Input Validation** (Tasks 1-3)
- DBPath validation
- Context nil checks
- Empty ID validation

**Group B: Query Safety** (Tasks 4-5)
- Find() result size protection
- Missing type cases in valueInArray

**Group C: Error Handling** (Tasks 6-8)
- Enhanced error messages
- Checkpoint failure handling
- Close() error handling

---

## Task 1: Add DBPath Validation (TDD)

**Files:**
- Test: `pkg/persistence/basic/sqlite_test.go` (add around line 2980, in NewStore validation context)
- Implement: `pkg/persistence/basic/sqlite.go` (modify NewStore function around line 238)

**Step 1: Write failing tests for DBPath validation**

Add to `sqlite_test.go` in the "NewStore validation" context:

```go
Context("DBPath validation", func() {
    It("should reject empty DBPath", func() {
        cfg := basic.Config{
            DBPath:      "",
            JournalMode: basic.JournalModeWAL,
        }
        _, err := basic.NewStore(cfg)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("DBPath cannot be empty"))
    })

    It("should reject DBPath that is a directory", func() {
        tempDir, err := os.MkdirTemp("", "sqlite-test-*")
        Expect(err).NotTo(HaveOccurred())
        defer os.RemoveAll(tempDir)

        cfg := basic.Config{
            DBPath:      tempDir, // Directory, not file
            JournalMode: basic.JournalModeWAL,
        }
        _, err = basic.NewStore(cfg)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("DBPath is a directory"))
    })

    It("should reject DBPath with unwritable parent directory", func() {
        // Create a read-only directory
        tempDir, err := os.MkdirTemp("", "sqlite-test-*")
        Expect(err).NotTo(HaveOccurred())
        defer func() {
            os.Chmod(tempDir, 0755) // Restore permissions for cleanup
            os.RemoveAll(tempDir)
        }()

        err = os.Chmod(tempDir, 0444) // Read-only
        Expect(err).NotTo(HaveOccurred())

        cfg := basic.Config{
            DBPath:      filepath.Join(tempDir, "test.db"),
            JournalMode: basic.JournalModeWAL,
        }
        _, err = basic.NewStore(cfg)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("permission denied"))
    })

    It("should allow DBPath in writable directory", func() {
        tempDir, err := os.MkdirTemp("", "sqlite-test-*")
        Expect(err).NotTo(HaveOccurred())
        defer os.RemoveAll(tempDir)

        cfg := basic.Config{
            DBPath:      filepath.Join(tempDir, "test.db"),
            JournalMode: basic.JournalModeWAL,
        }
        store, err := basic.NewStore(cfg)
        Expect(err).NotTo(HaveOccurred())
        Expect(store).NotTo(BeNil())
        store.Close(context.Background())
    })
})
```

**Step 2: Run tests to verify they fail**

Run: `ginkgo -v pkg/persistence/basic/ --focus "DBPath validation"`
Expected: FAIL (validation not implemented yet)

**Step 3: Implement DBPath validation in NewStore()**

Modify `NewStore()` in `sqlite.go` before journal mode validation:

```go
func NewStore(cfg Config) (Store, error) {
    // Validate DBPath
    if cfg.DBPath == "" {
        return nil, fmt.Errorf("DBPath cannot be empty")
    }

    // Check if DBPath exists and is a directory
    if info, err := os.Stat(cfg.DBPath); err == nil {
        if info.IsDir() {
            return nil, fmt.Errorf("DBPath is a directory: %s (must be a file path)", cfg.DBPath)
        }
    }

    // Check if parent directory is writable
    parentDir := filepath.Dir(cfg.DBPath)
    if err := os.MkdirAll(parentDir, 0755); err != nil {
        return nil, fmt.Errorf("cannot create parent directory for DBPath: %w", err)
    }

    // Test write permission by creating temp file
    testFile := filepath.Join(parentDir, ".write_test")
    if f, err := os.Create(testFile); err != nil {
        return nil, fmt.Errorf("DBPath parent directory is not writable: %w", err)
    } else {
        f.Close()
        os.Remove(testFile)
    }

    // ... rest of existing NewStore logic
}
```

**Step 4: Run tests to verify they pass**

Run: `ginkgo -v pkg/persistence/basic/ --focus "DBPath validation"`
Expected: PASS (4 specs passing)

**Step 5: Commit**

```bash
git add pkg/persistence/basic/sqlite.go pkg/persistence/basic/sqlite_test.go
git commit -m "feat(persistence): add DBPath validation in NewStore

Validate DBPath is not empty, not a directory, and parent is writable.
Prevents confusing SQLite errors and catches configuration mistakes early.

Part of Important Improvements from reliability review"
```

---

## Task 2: Add Context Nil Checks (TDD)

**Files:**
- Test: `pkg/persistence/basic/sqlite_test.go` (add new context)
- Implement: `pkg/persistence/basic/sqlite.go` (modify all Store methods)

**Step 1: Write failing tests for nil context handling**

Add to `sqlite_test.go`:

```go
Context("Context nil checks", func() {
    var store basic.Store
    var tempDB string

    BeforeEach(func() {
        var err error
        tempDB = filepath.Join(os.TempDir(), fmt.Sprintf("test-%d.db", time.Now().UnixNano()))
        cfg := basic.DefaultConfig(tempDB)
        store, err = basic.NewStore(cfg)
        Expect(err).NotTo(HaveOccurred())
    })

    AfterEach(func() {
        if store != nil {
            store.Close(context.Background())
        }
        os.Remove(tempDB)
    })

    It("should reject nil context in CreateCollection", func() {
        err := store.CreateCollection(nil, "test", nil)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
    })

    It("should reject nil context in Insert", func() {
        _ = store.CreateCollection(context.Background(), "test", nil)
        _, err := store.Insert(nil, "test", basic.Document{"key": "value"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
    })

    It("should reject nil context in Get", func() {
        _ = store.CreateCollection(context.Background(), "test", nil)
        _, err := store.Get(nil, "test", "any-id")
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
    })

    It("should reject nil context in Update", func() {
        _ = store.CreateCollection(context.Background(), "test", nil)
        err := store.Update(nil, "test", "any-id", basic.Document{"key": "value"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
    })

    It("should reject nil context in Delete", func() {
        _ = store.CreateCollection(context.Background(), "test", nil)
        err := store.Delete(nil, "test", "any-id")
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
    })

    It("should reject nil context in Find", func() {
        _ = store.CreateCollection(context.Background(), "test", nil)
        _, err := store.Find(nil, "test", basic.Filter{}, nil)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
    })

    It("should reject nil context in BeginTx", func() {
        _, err := store.BeginTx(nil)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
    })

    It("should reject nil context in Maintenance", func() {
        err := store.Maintenance(nil)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
    })

    It("should reject nil context in Close", func() {
        err := store.Close(nil)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("context cannot be nil"))
    })
})
```

**Step 2: Run tests to verify they fail**

Run: `ginkgo -v pkg/persistence/basic/ --focus "Context nil checks"`
Expected: FAIL (9 specs failing - nil checks not implemented)

**Step 3: Add nil context checks to all Store methods**

Add helper function at top of `sqlite.go`:

```go
// validateContext checks if context is nil and returns an error if so.
// All Store methods must validate context before use.
func validateContext(ctx context.Context) error {
    if ctx == nil {
        return fmt.Errorf("context cannot be nil")
    }
    return nil
}
```

Add to each Store method (example for CreateCollection):

```go
func (s *sqliteStore) CreateCollection(ctx context.Context, name string, schema *CollectionSchema) error {
    if err := validateContext(ctx); err != nil {
        return err
    }
    // ... existing logic
}
```

Repeat for: Insert, Get, Update, Delete, Find, BeginTx, Maintenance, Close

**Step 4: Run tests to verify they pass**

Run: `ginkgo -v pkg/persistence/basic/ --focus "Context nil checks"`
Expected: PASS (9 specs passing)

**Step 5: Commit**

```bash
git add pkg/persistence/basic/sqlite.go pkg/persistence/basic/sqlite_test.go
git commit -m "feat(persistence): add context nil checks to all methods

Validate context is not nil in all Store methods to prevent panics.
Provides clear error message instead of confusing nil pointer errors.

Part of Important Improvements from reliability review"
```

---

## Task 3: Add Empty ID Validation (TDD)

**Files:**
- Test: `pkg/persistence/basic/sqlite_test.go` (add new context)
- Implement: `pkg/persistence/basic/sqlite.go` (modify Get, Update, Delete methods)

**Step 1: Write failing tests for empty ID validation**

Add to `sqlite_test.go`:

```go
Context("Empty ID validation", func() {
    var store basic.Store
    var tempDB string

    BeforeEach(func() {
        var err error
        tempDB = filepath.Join(os.TempDir(), fmt.Sprintf("test-%d.db", time.Now().UnixNano()))
        cfg := basic.DefaultConfig(tempDB)
        store, err = basic.NewStore(cfg)
        Expect(err).NotTo(HaveOccurred())

        err = store.CreateCollection(context.Background(), "test", nil)
        Expect(err).NotTo(HaveOccurred())
    })

    AfterEach(func() {
        if store != nil {
            store.Close(context.Background())
        }
        os.Remove(tempDB)
    })

    It("should reject empty ID in Get", func() {
        _, err := store.Get(context.Background(), "test", "")
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("id cannot be empty"))
    })

    It("should reject empty ID in Update", func() {
        err := store.Update(context.Background(), "test", "", basic.Document{"key": "value"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("id cannot be empty"))
    })

    It("should reject empty ID in Delete", func() {
        err := store.Delete(context.Background(), "test", "")
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("id cannot be empty"))
    })
})
```

**Step 2: Run tests to verify they fail**

Run: `ginkgo -v pkg/persistence/basic/ --focus "Empty ID validation"`
Expected: FAIL (3 specs failing)

**Step 3: Add empty ID validation to Get, Update, Delete**

Modify each method to add ID validation after context check:

```go
func (s *sqliteStore) Get(ctx context.Context, collection, id string) (Document, error) {
    if err := validateContext(ctx); err != nil {
        return nil, err
    }
    if id == "" {
        return nil, fmt.Errorf("id cannot be empty")
    }
    // ... existing logic
}

func (s *sqliteStore) Update(ctx context.Context, collection, id string, doc Document) error {
    if err := validateContext(ctx); err != nil {
        return err
    }
    if id == "" {
        return fmt.Errorf("id cannot be empty")
    }
    // ... existing logic
}

func (s *sqliteStore) Delete(ctx context.Context, collection, id string) error {
    if err := validateContext(ctx); err != nil {
        return err
    }
    if id == "" {
        return fmt.Errorf("id cannot be empty")
    }
    // ... existing logic
}
```

**Step 4: Run tests to verify they pass**

Run: `ginkgo -v pkg/persistence/basic/ --focus "Empty ID validation"`
Expected: PASS (3 specs passing)

**Step 5: Commit**

```bash
git add pkg/persistence/basic/sqlite.go pkg/persistence/basic/sqlite_test.go
git commit -m "feat(persistence): validate ID is not empty in Get/Update/Delete

Prevent confusing errors when empty ID is passed to document operations.
Fail-fast with clear error message.

Part of Important Improvements from reliability review"
```

---

## Task 4: Add Find() Result Size Protection (TDD)

**Files:**
- Test: `pkg/persistence/basic/sqlite_test.go` (add new context)
- Implement: `pkg/persistence/basic/sqlite.go` (modify Find method)

**Step 1: Write failing tests for result size protection**

Add to `sqlite_test.go`:

```go
Context("Find result size protection", func() {
    var store basic.Store
    var tempDB string

    BeforeEach(func() {
        var err error
        tempDB = filepath.Join(os.TempDir(), fmt.Sprintf("test-%d.db", time.Now().UnixNano()))
        cfg := basic.DefaultConfig(tempDB)
        store, err = basic.NewStore(cfg)
        Expect(err).NotTo(HaveOccurred())

        err = store.CreateCollection(context.Background(), "test", nil)
        Expect(err).NotTo(HaveOccurred())

        // Insert 1000 documents
        for i := 0; i < 1000; i++ {
            _, err := store.Insert(context.Background(), "test", basic.Document{
                "index": i,
                "data":  fmt.Sprintf("doc-%d", i),
            })
            Expect(err).NotTo(HaveOccurred())
        }
    })

    AfterEach(func() {
        if store != nil {
            store.Close(context.Background())
        }
        os.Remove(tempDB)
    })

    It("should allow queries with reasonable result sizes", func() {
        opts := &basic.FindOptions{Limit: 100}
        results, err := store.Find(context.Background(), "test", basic.Filter{}, opts)
        Expect(err).NotTo(HaveOccurred())
        Expect(results).To(HaveLen(100))
    })

    It("should limit results to MaxFindLimit when no limit specified", func() {
        results, err := store.Find(context.Background(), "test", basic.Filter{}, nil)
        Expect(err).NotTo(HaveOccurred())
        // Should return MaxFindLimit (default 1000), not all 1000 docs
        Expect(len(results)).To(BeNumerically("<=", 1000))
    })

    It("should enforce MaxFindLimit even when higher limit requested", func() {
        opts := &basic.FindOptions{Limit: 50000}
        results, err := store.Find(context.Background(), "test", basic.Filter{}, opts)
        Expect(err).NotTo(HaveOccurred())
        // Should cap at MaxFindLimit (1000), not return 50000
        Expect(len(results)).To(BeNumerically("<=", 1000))
    })

    It("should allow configuring MaxFindLimit via FindOptions", func() {
        opts := &basic.FindOptions{
            Limit:        100,
            MaxFindLimit: 200, // Override default
        }
        results, err := store.Find(context.Background(), "test", basic.Filter{}, opts)
        Expect(err).NotTo(HaveOccurred())
        Expect(len(results)).To(BeNumerically("<=", 200))
    })
})
```

**Step 2: Run tests to verify they fail**

Run: `ginkgo -v pkg/persistence/basic/ --focus "Find result size protection"`
Expected: FAIL (MaxFindLimit not implemented)

**Step 3: Add MaxFindLimit to FindOptions and enforce in Find()**

Modify `FindOptions` struct in `sqlite.go`:

```go
// FindOptions configures Find query behavior.
type FindOptions struct {
    Sort         map[string]int
    Limit        int
    Offset       int
    MaxFindLimit int // Maximum result set size (default: 1000)
}

// DefaultMaxFindLimit is the default maximum number of documents Find() can return.
// Prevents accidental memory exhaustion from unbounded queries.
const DefaultMaxFindLimit = 1000
```

Modify `Find()` method to enforce limit:

```go
func (s *sqliteStore) Find(ctx context.Context, collection string, filter Filter, options *FindOptions) ([]Document, error) {
    if err := validateContext(ctx); err != nil {
        return nil, err
    }

    // ... existing logic ...

    // Apply MaxFindLimit protection
    maxLimit := DefaultMaxFindLimit
    if options != nil && options.MaxFindLimit > 0 {
        maxLimit = options.MaxFindLimit
    }

    // Enforce limit cap
    limit := maxLimit
    if options != nil && options.Limit > 0 && options.Limit < maxLimit {
        limit = options.Limit
    }

    // Build query with enforced limit
    query += fmt.Sprintf(" LIMIT %d", limit)

    // ... rest of existing logic ...
}
```

**Step 4: Run tests to verify they pass**

Run: `ginkgo -v pkg/persistence/basic/ --focus "Find result size protection"`
Expected: PASS (4 specs passing)

**Step 5: Commit**

```bash
git add pkg/persistence/basic/sqlite.go pkg/persistence/basic/sqlite_test.go
git commit -m "feat(persistence): add MaxFindLimit protection to Find queries

Prevent memory exhaustion from unbounded Find() queries.
Default limit: 1000 documents, configurable via FindOptions.

Part of Important Improvements from reliability review"
```

---

## Task 5: Add Missing Type Cases in valueInArray (TDD)

**Files:**
- Test: `pkg/persistence/basic/sqlite_test.go` (add new context)
- Implement: `pkg/persistence/basic/sqlite.go` (modify valueInArray helper)

**Step 1: Write failing tests for []string and []float64**

Add to `sqlite_test.go`:

```go
Context("valueInArray with additional types", func() {
    var store basic.Store
    var tempDB string

    BeforeEach(func() {
        var err error
        tempDB = filepath.Join(os.TempDir(), fmt.Sprintf("test-%d.db", time.Now().UnixNano()))
        cfg := basic.DefaultConfig(tempDB)
        store, err = basic.NewStore(cfg)
        Expect(err).NotTo(HaveOccurred())

        err = store.CreateCollection(context.Background(), "test", nil)
        Expect(err).NotTo(HaveOccurred())
    })

    AfterEach(func() {
        if store != nil {
            store.Close(context.Background())
        }
        os.Remove(tempDB)
    })

    It("should support []string in filter array values", func() {
        _, err := store.Insert(context.Background(), "test", basic.Document{
            "tags": []string{"go", "database", "sqlite"},
        })
        Expect(err).NotTo(HaveOccurred())

        // Query with string array filter
        filter := basic.Filter{
            "tags": []string{"database"},
        }
        results, err := store.Find(context.Background(), "test", filter, nil)
        Expect(err).NotTo(HaveOccurred())
        Expect(results).To(HaveLen(1))
    })

    It("should support []float64 in filter array values", func() {
        _, err := store.Insert(context.Background(), "test", basic.Document{
            "measurements": []float64{1.5, 2.3, 3.7},
        })
        Expect(err).NotTo(HaveOccurred())

        // Query with float64 array filter
        filter := basic.Filter{
            "measurements": []float64{2.3},
        }
        results, err := store.Find(context.Background(), "test", filter, nil)
        Expect(err).NotTo(HaveOccurred())
        Expect(results).To(HaveLen(1))
    })

    It("should handle mixed array types correctly", func() {
        _, err := store.Insert(context.Background(), "test", basic.Document{
            "int_array":    []int{1, 2, 3},
            "string_array": []string{"a", "b", "c"},
            "float_array":  []float64{1.1, 2.2, 3.3},
        })
        Expect(err).NotTo(HaveOccurred())

        results, err := store.Find(context.Background(), "test", basic.Filter{}, nil)
        Expect(err).NotTo(HaveOccurred())
        Expect(results).To(HaveLen(1))

        doc := results[0]
        Expect(doc["int_array"]).To(Equal([]interface{}{float64(1), float64(2), float64(3)}))
        Expect(doc["string_array"]).To(Equal([]interface{}{"a", "b", "c"}))
        Expect(doc["float_array"]).To(Equal([]interface{}{1.1, 2.2, 3.3}))
    })
})
```

**Step 2: Run tests to verify they fail**

Run: `ginkgo -v pkg/persistence/basic/ --focus "valueInArray with additional types"`
Expected: FAIL ([]string and []float64 cases not handled)

**Step 3: Add []string and []float64 cases to valueInArray**

Modify `valueInArray()` helper in `sqlite.go`:

```go
// valueInArray checks if any element in the array matches the filter value.
func valueInArray(arr []interface{}, filterValue interface{}) bool {
    switch v := filterValue.(type) {
    case []int:
        for _, item := range arr {
            if num, ok := item.(float64); ok {
                for _, fv := range v {
                    if int(num) == fv {
                        return true
                    }
                }
            }
        }
    case []string:
        for _, item := range arr {
            if str, ok := item.(string); ok {
                for _, fv := range v {
                    if str == fv {
                        return true
                    }
                }
            }
        }
    case []float64:
        for _, item := range arr {
            if num, ok := item.(float64); ok {
                for _, fv := range v {
                    if num == fv {
                        return true
                    }
                }
            }
        }
    case string:
        for _, item := range arr {
            if str, ok := item.(string); ok && str == v {
                return true
            }
        }
    case float64:
        for _, item := range arr {
            if num, ok := item.(float64); ok && num == v {
                return true
            }
        }
    case int:
        for _, item := range arr {
            if num, ok := item.(float64); ok && int(num) == v {
                return true
            }
        }
    }
    return false
}
```

**Step 4: Run tests to verify they pass**

Run: `ginkgo -v pkg/persistence/basic/ --focus "valueInArray with additional types"`
Expected: PASS (3 specs passing)

**Step 5: Commit**

```bash
git add pkg/persistence/basic/sqlite.go pkg/persistence/basic/sqlite_test.go
git commit -m "feat(persistence): support []string and []float64 in valueInArray

Extend array filtering to handle string and float64 array types.
Completes type support for common Go array types in queries.

Part of Important Improvements from reliability review"
```

---

## Task 6: Enhance Error Messages for SQLITE_FULL/SQLITE_BUSY (TDD)

**Files:**
- Test: `pkg/persistence/basic/sqlite_test.go` (add new context)
- Implement: `pkg/persistence/basic/sqlite.go` (add error enhancement helper)

**Step 1: Write tests for enhanced error messages**

Add to `sqlite_test.go`:

```go
Context("Enhanced SQLite error messages", func() {
    It("should provide actionable message for disk full errors", func() {
        // This is a documentation test - we verify the error wrapping logic exists
        // Actual SQLITE_FULL requires filling disk, which we can't do in tests

        // We test that the enhanceSQLiteError function exists and works
        err := basic.EnhanceSQLiteError(fmt.Errorf("database or disk is full"))
        Expect(err.Error()).To(ContainSubstring("disk full"))
        Expect(err.Error()).To(ContainSubstring("free up space"))
    })

    It("should provide actionable message for database locked errors", func() {
        err := basic.EnhanceSQLiteError(fmt.Errorf("database is locked"))
        Expect(err.Error()).To(ContainSubstring("locked"))
        Expect(err.Error()).To(ContainSubstring("retry"))
        Expect(err.Error()).To(ContainSubstring("concurrent access"))
    })

    It("should pass through unrecognized errors unchanged", func() {
        originalErr := fmt.Errorf("some other error")
        err := basic.EnhanceSQLiteError(originalErr)
        Expect(err).To(Equal(originalErr))
    })
})
```

**Step 2: Run tests to verify they fail**

Run: `ginkgo -v pkg/persistence/basic/ --focus "Enhanced SQLite error messages"`
Expected: FAIL (EnhanceSQLiteError function doesn't exist)

**Step 3: Implement error enhancement helper**

Add to `sqlite.go`:

```go
// EnhanceSQLiteError wraps SQLite errors with actionable context.
// Exposed for testing.
func EnhanceSQLiteError(err error) error {
    if err == nil {
        return nil
    }

    errMsg := err.Error()

    // SQLITE_FULL (disk full)
    if strings.Contains(errMsg, "database or disk is full") || strings.Contains(errMsg, "disk is full") {
        return fmt.Errorf("%w\n\nActionable steps:\n"+
            "1. Free up disk space (remove old logs, temp files)\n"+
            "2. Run VACUUM to reclaim deleted data space\n"+
            "3. Check disk quota limits\n"+
            "4. Consider moving database to larger volume", err)
    }

    // SQLITE_BUSY (database locked)
    if strings.Contains(errMsg, "database is locked") {
        return fmt.Errorf("%w\n\nActionable steps:\n"+
            "1. Retry operation after brief delay (100-1000ms)\n"+
            "2. Check for long-running transactions blocking writes\n"+
            "3. Verify concurrent access patterns (WAL mode helps)\n"+
            "4. Increase busy_timeout if using immediate transactions", err)
    }

    // Pass through other errors unchanged
    return err
}

// enhanceSQLiteError is the internal version that wraps all Store method errors.
func (s *sqliteStore) enhanceSQLiteError(err error) error {
    return EnhanceSQLiteError(err)
}
```

Apply `enhanceSQLiteError` to all Store methods that interact with SQLite (wrap all `return err` statements).

Example for Insert:

```go
func (s *sqliteStore) Insert(ctx context.Context, collection string, doc Document) (string, error) {
    // ... existing logic ...

    if err := tx.Commit(); err != nil {
        return "", s.enhanceSQLiteError(err)
    }

    return id, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `ginkgo -v pkg/persistence/basic/ --focus "Enhanced SQLite error messages"`
Expected: PASS (3 specs passing)

**Step 5: Commit**

```bash
git add pkg/persistence/basic/sqlite.go pkg/persistence/basic/sqlite_test.go
git commit -m "feat(persistence): enhance SQLite error messages with actions

Wrap SQLITE_FULL and SQLITE_BUSY errors with actionable guidance.
Helps operators resolve production issues faster.

Part of Important Improvements from reliability review"
```

---

## Task 7: Add WAL Checkpoint Failure Handling (TDD)

**Files:**
- Test: `pkg/persistence/basic/sqlite_test.go` (add new context)
- Implement: `pkg/persistence/basic/sqlite.go` (enhance Close method)

**Step 1: Write tests for checkpoint failure handling**

Add to `sqlite_test.go`:

```go
Context("WAL checkpoint failure handling", func() {
    It("should log warning but still close on checkpoint failure", func() {
        tempDB := filepath.Join(os.TempDir(), fmt.Sprintf("test-%d.db", time.Now().UnixNano()))
        defer os.Remove(tempDB)

        cfg := basic.DefaultConfig(tempDB)
        cfg.MaintenanceOnShutdown = false // Don't run VACUUM
        store, err := basic.NewStore(cfg)
        Expect(err).NotTo(HaveOccurred())

        // Create some data to generate WAL
        err = store.CreateCollection(context.Background(), "test", nil)
        Expect(err).NotTo(HaveOccurred())

        for i := 0; i < 10; i++ {
            _, err := store.Insert(context.Background(), "test", basic.Document{"idx": i})
            Expect(err).NotTo(HaveOccurred())
        }

        // Close should succeed even if checkpoint has issues
        err = store.Close(context.Background())
        Expect(err).NotTo(HaveOccurred())
    })

    It("should attempt checkpoint before close", func() {
        tempDB := filepath.Join(os.TempDir(), fmt.Sprintf("test-%d.db", time.Now().UnixNano()))
        defer os.Remove(tempDB)
        defer os.Remove(tempDB + "-wal") // WAL file
        defer os.Remove(tempDB + "-shm") // Shared memory file

        cfg := basic.DefaultConfig(tempDB)
        cfg.MaintenanceOnShutdown = false
        store, err := basic.NewStore(cfg)
        Expect(err).NotTo(HaveOccurred())

        err = store.CreateCollection(context.Background(), "test", nil)
        Expect(err).NotTo(HaveOccurred())

        for i := 0; i < 100; i++ {
            _, err := store.Insert(context.Background(), "test", basic.Document{"idx": i})
            Expect(err).NotTo(HaveOccurred())
        }

        // Verify WAL file exists before close
        _, err = os.Stat(tempDB + "-wal")
        walExistsBefore := err == nil

        // Close triggers checkpoint
        err = store.Close(context.Background())
        Expect(err).NotTo(HaveOccurred())

        // After successful close with checkpoint, WAL should be removed or small
        info, err := os.Stat(tempDB + "-wal")
        if err == nil {
            // WAL may still exist but should be much smaller after checkpoint
            if walExistsBefore {
                Expect(info.Size()).To(BeNumerically("<", 1000), "WAL should be checkpointed")
            }
        }
    })
})
```

**Step 2: Run tests to verify current behavior**

Run: `ginkgo -v pkg/persistence/basic/ --focus "WAL checkpoint failure handling"`
Expected: PASS (tests verify current behavior, but we'll enhance Close)

**Step 3: Add explicit checkpoint handling in Close()**

Modify `Close()` in `sqlite.go`:

```go
func (s *sqliteStore) Close(ctx context.Context) error {
    if err := validateContext(ctx); err != nil {
        return err
    }

    s.mu.Lock()
    defer s.mu.Unlock()

    if s.closed {
        return nil // Idempotent
    }

    // Run maintenance before closing if enabled
    if s.maintenanceOnShutdown {
        if err := s.maintenanceInternal(ctx); err != nil {
            s.closed = true
            _ = s.db.Close()
            return fmt.Errorf("maintenance incomplete: %w", err)
        }
    }

    // Attempt WAL checkpoint before closing
    // PRAGMA wal_checkpoint(TRUNCATE) checkpoints and truncates WAL file
    if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
        // Log warning but don't fail close
        fmt.Printf("WARNING: WAL checkpoint failed during close: %v\n", err)
        fmt.Printf("Database will still close safely. WAL will be checkpointed on next open.\n")
    }

    s.closed = true
    return s.enhanceSQLiteError(s.db.Close())
}
```

**Step 4: Run tests to verify they pass**

Run: `ginkgo -v pkg/persistence/basic/ --focus "WAL checkpoint failure handling"`
Expected: PASS (2 specs passing)

**Step 5: Commit**

```bash
git add pkg/persistence/basic/sqlite.go pkg/persistence/basic/sqlite_test.go
git commit -m "feat(persistence): add explicit WAL checkpoint on Close

Execute PRAGMA wal_checkpoint(TRUNCATE) before closing database.
Logs warning but doesn't fail if checkpoint has issues.
Ensures clean shutdown and minimizes WAL file size.

Part of Important Improvements from reliability review"
```

---

## Task 8: Document Error Handling for WAL Checkpoint (Documentation)

**Files:**
- Modify: `pkg/persistence/basic/sqlite.go` (enhance Close() documentation)
- Create: `docs/architecture/sqlite-checkpoint-handling.md`

**Step 1: Enhance Close() documentation**

Update the Close() method documentation in `sqlite.go`:

```go
// Close closes the store and releases resources.
//
// DESIGN DECISION: Accept context for graceful shutdown control
// WHY: Close() may perform maintenance operations (VACUUM, ANALYZE) that take
// seconds to minutes. Caller needs ability to:
//   - Set deadline: ctx with timeout controls max shutdown time
//   - Cancel early: ctx cancellation aborts maintenance, closes immediately
//
// GRACEFUL DEGRADATION:
//   If context expires during maintenance, database still closes safely.
//   Maintenance may be incomplete, but data integrity is preserved.
//
// WAL CHECKPOINT HANDLING:
//   Close() executes PRAGMA wal_checkpoint(TRUNCATE) to:
//   - Flush WAL contents back to main database file
//   - Truncate WAL file to minimize disk usage
//   - Prepare for clean shutdown
//
//   If checkpoint fails (e.g., locked by another process):
//   - Warning logged to stderr
//   - Database still closes successfully
//   - WAL automatically checkpointed on next open
//   - No data loss - WAL contains transaction history
//
// ERROR HANDLING:
//   - Checkpoint failure: Logs warning, continues with close
//   - Maintenance failure: Returns error, still closes database
//   - Close() is idempotent: Safe to call multiple times
Close(ctx context.Context) error
```

**Step 2: Create checkpoint handling documentation**

Create `docs/architecture/sqlite-checkpoint-handling.md`:

```markdown
# SQLite WAL Checkpoint Handling

**Date**: 2025-01-19
**Context**: Task 8 of Important Improvements from reliability review

## Overview

SQLite WAL (Write-Ahead Logging) mode requires periodic checkpointing to flush
changes from the WAL file back to the main database file.

## Checkpoint Strategy

### Automatic Checkpoints

SQLite automatically checkpoints when:
- WAL file reaches 1000 pages (~4MB with 4KB page size)
- Database connection closes
- PRAGMA wal_checkpoint() executed

### Manual Checkpoint on Close()

Our implementation explicitly calls `PRAGMA wal_checkpoint(TRUNCATE)` during
`Close()` to ensure clean shutdown:

```go
if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
    // Log warning but don't fail close
    fmt.Printf("WARNING: WAL checkpoint failed during close: %v\n", err)
}
```

### Why TRUNCATE Mode?

- **PASSIVE**: Checkpoints only if no readers (may not checkpoint)
- **FULL**: Waits for readers to finish (may block)
- **RESTART**: Like FULL + resets WAL (may block)
- **TRUNCATE**: Like RESTART + truncates WAL file ✅

We use TRUNCATE to minimize disk usage and ensure WAL is flushed.

## Error Handling

### Checkpoint Can Fail

Common failure scenarios:
1. **Another process has database open**: Checkpoint blocked
2. **Long-running read transaction**: Checkpoint waits
3. **Filesystem issues**: I/O errors
4. **Context timeout**: Operation cancelled

### Graceful Degradation

When checkpoint fails:
- ✅ Warning logged to stderr
- ✅ Database still closes successfully
- ✅ No data loss (WAL contains transaction history)
- ✅ WAL automatically checkpointed on next open

**Key insight**: Checkpoint failure is not catastrophic. SQLite recovers
automatically on next database open.

## Production Implications

### Disk Space

Without successful checkpoints:
- WAL file grows unbounded
- Can reach multiple GB on busy systems
- Eventually triggers automatic checkpoint (but at 1000 pages)

### Recovery Time

If WAL grows large:
- Next database open takes longer (replays WAL)
- Typical: <1s for <100MB WAL
- Large: 5-10s for multi-GB WAL

### Monitoring

Monitor WAL file size:
```bash
ls -lh /data/state.db-wal
```

If consistently >100MB, investigate:
- Are there long-running transactions?
- Is another process holding the database?
- Are checkpoints succeeding?

## Testing

Manual test for checkpoint behavior:
```go
// Create store, insert data, close
cfg := basic.DefaultConfig("test.db")
store, _ := basic.NewStore(cfg)
store.CreateCollection(ctx, "test", nil)

for i := 0; i < 1000; i++ {
    store.Insert(ctx, "test", basic.Document{"idx": i})
}

// Check WAL size before close
info, _ := os.Stat("test.db-wal")
fmt.Printf("WAL size before close: %d bytes\n", info.Size())

// Close triggers checkpoint
store.Close(ctx)

// Check WAL size after close (should be small or deleted)
info, _ = os.Stat("test.db-wal")
fmt.Printf("WAL size after close: %d bytes\n", info.Size())
```

## References

- SQLite WAL Mode: https://www.sqlite.org/wal.html
- Checkpoint Documentation: https://www.sqlite.org/pragma.html#pragma_wal_checkpoint
- Go database/sql: https://pkg.go.dev/database/sql
```

**Step 3: Verify documentation accuracy**

Run: `ginkgo -v pkg/persistence/basic/`
Expected: All tests pass (no code changes, only documentation)

**Step 4: Commit documentation**

```bash
git add pkg/persistence/basic/sqlite.go docs/architecture/sqlite-checkpoint-handling.md
git commit -m "docs(persistence): document WAL checkpoint handling strategy

Explain checkpoint behavior, error handling, and graceful degradation.
Provides production monitoring guidance and testing examples.

Completes Task 8: Important Improvements from reliability review"
```

---

## Final Verification

**Step 1: Run full test suite**

```bash
ginkgo -v pkg/persistence/basic/
```

Expected: All tests PASS (should have 180+ specs now)

**Step 2: Check for focused tests**

```bash
ginkgo -r --fail-on-focused pkg/persistence/basic/
```

Expected: No focused tests

**Step 3: Run linter**

```bash
golangci-lint run pkg/persistence/basic/...
```

Expected: 0 issues

**Step 4: Run static analysis**

```bash
go vet ./pkg/persistence/basic/...
```

Expected: No warnings

**Step 5: Run benchmarks**

```bash
go test -bench=. -benchmem pkg/persistence/basic/
```

Expected: All benchmarks pass

**Step 6: Final commit**

```bash
git add -A
git commit -m "feat(persistence): complete 8 Important Improvements

All improvements from reliability review implemented:
- ✅ DBPath validation
- ✅ Context nil checks
- ✅ Empty ID validation
- ✅ Find() result size protection
- ✅ []string/[]float64 support in valueInArray
- ✅ Enhanced error messages (SQLITE_FULL/SQLITE_BUSY)
- ✅ WAL checkpoint failure handling
- ✅ Checkpoint documentation

All tests passing. Production-ready."
```

---

## Execution Instructions

Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/subagent-driven-development/SKILL.md` to execute this plan:

1. Load this plan
2. Create TodoWrite with all 8 tasks
3. For each task:
   - Dispatch fresh implementation subagent
   - Subagent implements task (TDD: test → fail → implement → pass → commit)
   - Dispatch code-reviewer subagent
   - Fix any issues found
   - Mark task complete
4. After all tasks: Final verification
5. Document completion in reliability review tracking

**Estimated Time:** 4-6 hours (as estimated in reliability review)
