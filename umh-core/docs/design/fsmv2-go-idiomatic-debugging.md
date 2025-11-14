# FSMv2 Go-Idiomatic Debugging Guide

**Status:** Design Document
**Created:** 2025-11-13
**Context:** Analysis of fsmv2-debugging-improvements.md from Go perspective
**Goal:** Identify Go-idiomatic observability patterns vs. borrowed approaches

## Executive Summary

The previous brainstorming document (fsmv2-debugging-improvements.md) contains 20+ debugging ideas using creativity techniques. However, **many solutions are borrowed from other languages** (Rust borrow checker, Linux lockdep, database MVCC) rather than leveraging Go's strengths.

**Key Insight:** Go's philosophy is **observability through simplicity**, not compile-time guarantees or complex runtime enforcement. The Go way prioritizes:
1. Clear interfaces over type-system tricks
2. Runtime visibility over static analysis
3. Good tooling over language features
4. Explicit simplicity over implicit magic

This document provides **Go-idiomatic recommendations** for the user's four priorities:
- **Lifecycle logging** - Structured, contextual, leveled
- **Some enforcement** - Interface-based, not type-system gymnastics
- **Clean boundaries** - Unexported mutexes, clear ownership
- **Documentation of locks** - Comment conventions from stdlib

---

## Part 1: What's Go-Idiomatic vs. Borrowed?

### ✅ Go-Idiomatic Approaches (Keep These)

From the brainstorming document, these align with Go practices:

**1. Lifecycle Logging Infrastructure (Proposal 4)**
- **Why Go-idiomatic:** Go's `log/slog` (stdlib) designed for structured logging
- **Pattern:** `logger.Info("operation", "key", value)` style
- **Used by:** Kubernetes, CockroachDB, etcd
- **Keep:** Environment-variable gating (`LIFECYCLE_LOGGING=1`)

**2. Lock Operation Tracer (Proposal 1)**
- **Why Go-idiomatic:** Wrapper pattern common in Go (io.Writer, http.Handler)
- **Pattern:** Drop-in replacement preserving interface
- **Used by:** `expvar`, `net/http/httputil`
- **Keep:** Conditional compilation via build tags or env vars

**3. Enhanced Test Timeouts (Quick Win 1)**
- **Why Go-idiomatic:** Context-based cancellation is core Go pattern
- **Pattern:** `context.WithTimeout()`, goroutine-based monitoring
- **Used by:** Every production Go service
- **Keep:** Detect blocked goroutines via runtime.Stack()

**4. Mutex Profiling (Forensic 2)**
- **Why Go-idiomatic:** Built into runtime (`runtime/pprof`)
- **Pattern:** Enable in tests, automatic report generation
- **Used by:** `go test -mutexprofile=mutex.out`
- **Keep:** This is already a solved problem in Go

### ❌ Not Go-Idiomatic (Avoid or Adapt)

**1. Static Lock Analysis Tool (Proposal 3)**
- **Problem:** Go lacks compile-time borrow checking (by design)
- **Why not idiomatic:** Go compiler is intentionally simple
- **Reality:** Re-entrancy cannot be caught at compile time without full program analysis
- **Go way:** Runtime checks + good testing (race detector)
- **Verdict:** ⚠️ **Expensive, limited value.** Go's simplicity is a feature, not a bug.

**2. Lock Hierarchy Enforcement at Runtime (Proposal 5)**
- **Problem:** Too heavyweight, adds per-lock overhead
- **Why not idiomatic:** Go favors "fail fast at API boundaries" over runtime validation
- **Reality:** Panics on hierarchy violations = debugging production crashes
- **Go way:** Document hierarchy in comments, use `-race` detector
- **Verdict:** ⚠️ **Over-engineered.** Comment-based documentation is sufficient.

**3. Message-Passing Supervisor (Moonshot 1)**
- **Problem:** Fights Go's shared-memory concurrency model
- **Why not idiomatic:** Go has both channels AND mutexes for a reason
- **Reality:** Not all problems are best solved with message-passing
- **Go way:** Use mutexes for shared state, channels for coordination
- **Verdict:** ❌ **Wrong tool.** Supervisors manage hierarchical state, not message queues.

**4. Compile-Time Lock Safety with Types (Moonshot 2)**
- **Problem:** Go's type system cannot enforce Rust-style lifetimes
- **Why not idiomatic:** Go explicitly rejects lifetime annotations
- **Reality:** `LockedRead[T]` pattern is clunky and doesn't prevent deadlocks
- **Go way:** Unexported mutexes + clear API design
- **Verdict:** ❌ **Type gymnastics.** Not how Go works.

**5. Time-Travel Debugging (Forensic 4)**
- **Problem:** Not practical for production Go services
- **Why not idiomatic:** Go doesn't have deterministic replay like rr
- **Reality:** Record-replay requires heavyweight infrastructure
- **Go way:** Good logging + stack traces + heap dumps
- **Verdict:** ❌ **Not realistic.** Better logging achieves 90% of benefit.

---

## Part 2: Go Idioms for Observability

### How Go Stdlib Handles Similar Problems

**Pattern 1: Unexported Mutexes (net/http, sync.Map)**

```go
// From sync.Map
type Map struct {
    mu     Mutex  // UNEXPORTED - internal implementation detail
    read   atomic.Pointer[readOnly]
    dirty  map[any]*entry
    misses int
}

// Public API never exposes mu
func (m *Map) Load(key any) (value any, ok bool) {
    // Locking is internal concern, not caller's
}
```

**Why this works:**
- API surface is clean (no `Lock()`/`Unlock()` exposed)
- Caller cannot misuse locks
- Implementation can change locking strategy without breaking users

**FSMv2 Application:**
```go
// Current problematic pattern:
type Supervisor struct {
    mu sync.RWMutex  // EXPORTED - callers see this
}

// Go-idiomatic pattern:
type Supervisor struct {
    mu sync.RWMutex  // Unexported, only supervisor methods access
}

// All access goes through methods that handle locking
func (s *Supervisor) GetChildren() map[string]*Supervisor {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Return copy, not reference (defensive)
    children := make(map[string]*Supervisor, len(s.children))
    for k, v := range s.children {
        children[k] = v
    }
    return children
}
```

---

**Pattern 2: Comment Conventions for Lock Documentation (sync package)**

From `sync/rwmutex.go`:
```go
// RWMutex is a reader/writer mutual exclusion lock.
//
// It should not be used for recursive read locking; a blocked Lock
// call excludes new readers from acquiring the lock.
//
// A RWMutex must not be copied after first use.
```

From `net/http/server.go`:
```go
type Server struct {
    mu         sync.Mutex
    listeners  map[*net.Listener]struct{}  // mu guards listeners
    activeConn map[*conn]struct{}          // mu guards activeConn
    doneChan   chan struct{}               // mu guards doneChan
}
```

**FSMv2 Application:**
```go
type Supervisor struct {
    // LOCK ORDER: parent.mu > child.mu
    // Never acquire child lock while holding parent write lock.
    mu sync.RWMutex

    // mu guards the following fields:
    workers   map[string]*WorkerContext  // Protected by mu
    children  map[string]*Supervisor     // Protected by mu
    userSpec  config.UserSpec            // Protected by mu

    // Immutable after construction (no locking needed):
    logger    *zap.SugaredLogger
    store     storage.TriangularStoreInterface
    workerType string
}
```

**Go conventions:**
- Put lock ordering in type-level comment (top of struct)
- Document protected fields inline: `// mu guards field`
- Mark immutable fields explicitly
- NEVER document lock internals in public godoc (implementation detail)

---

**Pattern 3: Context for Correlation IDs (context package)**

Go's `context.Context` is **the idiomatic way** to propagate request-scoped values:

```go
// Standard Go pattern from Google's production code
type contextKey string

const (
    requestIDKey contextKey = "request-id"
    traceIDKey   contextKey = "trace-id"
)

func WithRequestID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, requestIDKey, id)
}

func GetRequestID(ctx context.Context) string {
    if id, ok := ctx.Value(requestIDKey).(string); ok {
        return id
    }
    return ""
}
```

**FSMv2 Application:**
```go
// Instead of global correlation IDs, use context
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // Attach worker ID to context for logging
    ctx = context.WithValue(ctx, "worker_id", workerID)
    ctx = context.WithValue(ctx, "supervisor_type", s.workerType)

    // All logs in this call chain inherit context
    s.logger.InfoContext(ctx, "starting tick")

    // Pass ctx to child calls
    if err := s.processSignal(ctx, workerID, signal); err != nil {
        s.logger.ErrorContext(ctx, "signal failed", "error", err)
    }
}
```

**Why context over global variables:**
- Type-safe (values scoped to request)
- No global state pollution
- Cancellation propagates automatically
- Works with structured logging (`slog.Logger`)

---

**Pattern 4: Structured Logging with log/slog (Go 1.21+)**

```go
// Go stdlib structured logging (since 1.21)
logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))

// Structured fields, not string interpolation
logger.Info("state transition",
    "from", oldState,
    "to", newState,
    "worker_id", workerID,
    "duration_ms", duration.Milliseconds(),
)

// Output (structured JSON):
// {"time":"2025-11-13T12:34:56Z","level":"INFO","msg":"state transition",
//  "from":"running","to":"stopping","worker_id":"worker-1","duration_ms":42}
```

**FSMv2 Application:**
```go
type Supervisor struct {
    logger *slog.Logger  // NOT *zap.SugaredLogger
}

func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    s.logger.InfoContext(ctx, "lifecycle_event",
        "operation", "tick_start",
        "worker_id", workerID,
        "state", s.GetCurrentState(),
    )

    defer func(start time.Time) {
        s.logger.InfoContext(ctx, "lifecycle_event",
            "operation", "tick_complete",
            "worker_id", workerID,
            "duration_ms", time.Since(start).Milliseconds(),
        )
    }(time.Now())

    // ... tick logic
}
```

**Why slog over zap:**
- **Stdlib** (no external dependency)
- **Context-aware** (`InfoContext`, `ErrorContext`)
- **Structured by default** (key-value pairs)
- **Level-based** (Debug/Info/Warn/Error)

---

## Part 3: Four Focus Areas - Go-Idiomatic Solutions

### 1. Lifecycle Logging (User Priority #1)

**Go-Idiomatic Pattern: Structured Logging with slog**

```go
// File: pkg/fsmv2/supervisor/lifecycle.go

type LifecycleLogger struct {
    logger      *slog.Logger
    supervisorID string
    enabled     bool
}

func NewLifecycleLogger(supervisorID string) *LifecycleLogger {
    enabled := os.Getenv("LIFECYCLE_DEBUG") == "1"

    var logger *slog.Logger
    if enabled {
        // JSON output for grep-ability
        handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
            Level: slog.LevelDebug,
        })
        logger = slog.New(handler)
    } else {
        logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
    }

    return &LifecycleLogger{
        logger:      logger,
        supervisorID: supervisorID,
        enabled:     enabled,
    }
}

// Operation-specific logging methods
func (l *LifecycleLogger) TickStart(ctx context.Context, workerID string) {
    l.logger.InfoContext(ctx, "lifecycle.tick_start",
        "supervisor_id", l.supervisorID,
        "worker_id", workerID,
    )
}

func (l *LifecycleLogger) StateTransition(ctx context.Context, workerID, from, to string) {
    l.logger.InfoContext(ctx, "lifecycle.state_transition",
        "supervisor_id", l.supervisorID,
        "worker_id", workerID,
        "from", from,
        "to", to,
    )
}

func (l *LifecycleLogger) ChildShutdown(ctx context.Context, childID string, duration time.Duration) {
    l.logger.InfoContext(ctx, "lifecycle.child_shutdown",
        "supervisor_id", l.supervisorID,
        "child_id", childID,
        "duration_ms", duration.Milliseconds(),
    )
}
```

**Integration in Supervisor:**
```go
type Supervisor struct {
    lifecycleLog *LifecycleLogger
    // ... other fields
}

func NewSupervisor(cfg Config) *Supervisor {
    return &Supervisor{
        lifecycleLog: NewLifecycleLogger(cfg.WorkerType),
        // ... other initialization
    }
}

func (s *Supervisor) processSignal(ctx context.Context, workerID string, sig fsmv2.Signal) error {
    s.lifecycleLog.TickStart(ctx, workerID)

    switch sig {
    case fsmv2.SignalNeedsRemoval:
        start := time.Now()

        // ... removal logic

        s.lifecycleLog.ChildShutdown(ctx, workerID, time.Since(start))
    }
}
```

**Grep-able Output:**
```bash
# Find all state transitions
grep 'lifecycle.state_transition' logs/*.log

# Find slow operations (>5s)
grep 'duration_ms' logs/*.log | awk -F'"' '{if ($10 > 5000) print}'

# Trace specific worker
grep 'worker-123' logs/*.log | jq .
```

**Effort:** 3-4 hours
**Files:** `pkg/fsmv2/supervisor/lifecycle.go` (new), `supervisor.go` (integrate)

---

### 2. Some Enforcement (User Priority #2)

**Go-Idiomatic Pattern: Interface Segregation + Constructor Functions**

Not compile-time enforcement (Go doesn't do that), but **design-time enforcement** through interfaces.

**Problem:** Re-entrancy is invisible (calling `reconcileChildren()` from `processSignal()` re-locks)

**Go Solution:** Split interfaces to make re-entrancy impossible at call site

```go
// File: pkg/fsmv2/supervisor/internal.go

// splitSupervisor separates internal (locking) from external (locked) operations.
// This makes re-entrancy structurally impossible at compile time.

// External API (acquires locks)
type Supervisor struct {
    internal *supervisorInternal  // Unexported, holds actual state
}

// Internal state (NO locking, assumes already locked)
type supervisorInternal struct {
    workers  map[string]*WorkerContext
    children map[string]*Supervisor
    logger   *slog.Logger
}

// External API method (acquires lock, calls internal)
func (s *Supervisor) ProcessSignal(ctx context.Context, workerID string, sig fsmv2.Signal) error {
    s.internal.mu.Lock()
    defer s.internal.mu.Unlock()

    // Pass lockedView to internal method - no re-locking possible
    return s.internal.processSignalLocked(ctx, workerID, sig)
}

// Internal method (assumes lock held)
func (si *supervisorInternal) processSignalLocked(ctx context.Context, workerID string, sig fsmv2.Signal) error {
    // Can call other *Locked methods freely (no re-entrancy risk)
    si.reconcileChildrenLocked(ctx, []ChildSpec{})  // OK, no lock acquisition

    // CANNOT call external API (no reference to Supervisor)
    // s.ProcessSignal() // Compile error: si doesn't have ProcessSignal method
}
```

**Anti-Pattern Detection via Linting:**
```go
// Use go vet custom checker to enforce naming convention
// File: tools/lockcheck/lockcheck.go

// Check that *Locked methods only call other *Locked methods
func checkLockedMethods(pass *analysis.Pass) {
    for _, file := range pass.Files {
        ast.Inspect(file, func(n ast.Node) bool {
            fn, ok := n.(*ast.FuncDecl)
            if !ok || !strings.HasSuffix(fn.Name.Name, "Locked") {
                return true
            }

            // Ensure *Locked methods don't call non-Locked methods
            ast.Inspect(fn.Body, func(call ast.Node) bool {
                callExpr, ok := call.(*ast.CallExpr)
                if !ok {
                    return true
                }

                // Check if calling non-Locked method
                if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
                    methodName := sel.Sel.Name
                    if !strings.HasSuffix(methodName, "Locked") {
                        pass.Reportf(call.Pos(),
                            "Locked method %s calls non-Locked method %s (potential re-entrancy)",
                            fn.Name.Name, methodName)
                    }
                }
                return true
            })
            return true
        })
    }
}
```

**Build Tags for Debug Builds:**
```go
// File: pkg/fsmv2/supervisor/debug_checks.go
//go:build debug

package supervisor

// In debug builds, add runtime assertions
func (s *Supervisor) ProcessSignal(ctx context.Context, workerID string, sig fsmv2.Signal) error {
    // Runtime check: ensure we don't hold lock already
    if !s.mu.TryLock() {
        panic(fmt.Sprintf("ProcessSignal called while lock already held (goroutine %d)", getGoroutineID()))
    }
    s.mu.Unlock()

    s.mu.Lock()
    defer s.mu.Unlock()

    return s.internal.processSignalLocked(ctx, workerID, sig)
}
```

**Build and test:**
```bash
# Normal build (no checks)
go build ./pkg/fsmv2/...

# Debug build (runtime assertions)
go build -tags=debug ./pkg/fsmv2/...

# CI runs both
make test          # Normal
make test-debug    # With debug tag
```

**Effort:** 6-8 hours
**Files:** `supervisor.go` (refactor), `tools/lockcheck/` (new), `.github/workflows/ci.yml` (add debug build)

---

### 3. Clean Boundaries (User Priority #3)

**Go-Idiomatic Pattern: Unexported Mutexes + Guard Objects**

**Current Problem:**
```go
type Supervisor struct {
    mu       sync.RWMutex  // EXPORTED
    children map[string]*Supervisor
}

// Caller can misuse:
sup.mu.Lock()
// ... forget to unlock
```

**Go Solution: Unexported Lock + Guard Pattern**

```go
type Supervisor struct {
    mu       sync.RWMutex  // Unexported, internal
    children map[string]*Supervisor
}

// ReadGuard ensures RUnlock is called
type ReadGuard struct {
    mu *sync.RWMutex
}

func (g *ReadGuard) Release() {
    g.mu.RUnlock()
}

// Acquire read lock, return guard
func (s *Supervisor) acquireReadLock() *ReadGuard {
    s.mu.RLock()
    return &ReadGuard{mu: &s.mu}
}

// Public API uses guard pattern
func (s *Supervisor) GetChildren() map[string]*Supervisor {
    guard := s.acquireReadLock()
    defer guard.Release()

    // Copy children (defensive)
    children := make(map[string]*Supervisor, len(s.children))
    for k, v := range s.children {
        children[k] = v
    }
    return children
}
```

**Better: Just Use Unexported Mutex (Simpler)**

The guard pattern is overkill for most Go code. **Simplest Go idiom:**

```go
type Supervisor struct {
    mu       sync.RWMutex  // Unexported, period
    children map[string]*Supervisor
}

// All locking hidden in methods
func (s *Supervisor) GetChildren() map[string]*Supervisor {
    s.mu.RLock()
    defer s.mu.RUnlock()

    children := make(map[string]*Supervisor, len(s.children))
    for k, v := range s.children {
        children[k] = v
    }
    return children
}

// Callers CANNOT misuse locks (no access)
```

**API Design Principles:**
1. **Never expose mutexes** - internal implementation detail
2. **Always return copies** - prevent concurrent modification
3. **Use defer for unlocks** - ensures cleanup on panic
4. **Document thread-safety** - in godoc for exported methods

**godoc Example:**
```go
// GetChildren returns a snapshot of child supervisors.
// The returned map is a copy and safe for concurrent iteration.
// This method is safe to call concurrently with other Supervisor methods.
func (s *Supervisor) GetChildren() map[string]*Supervisor {
    // ...
}
```

**Effort:** 2-3 hours
**Files:** `supervisor.go` (unexport mu, update all methods)

---

### 4. Lock Documentation (User Priority #4)

**Go-Idiomatic Pattern: Comment Conventions from Stdlib**

**Example from net/http/server.go:**
```go
type Server struct {
    mu         sync.Mutex
    activeConn map[*conn]struct{} // guarded by mu
    listeners  map[*net.Listener]struct{} // guarded by mu

    // Read by ListenAndServe, overridden by tests
    testHookServerServe func(*Server, net.Listener) // not guarded
}
```

**FSMv2 Application:**
```go
// Supervisor manages worker lifecycle and hierarchical composition.
//
// LOCK ORDERING: parent.mu > child.mu
// NEVER acquire child lock while holding parent write lock.
// Violation causes reader starvation (GetChildren blocked indefinitely).
//
// THREAD SAFETY: All exported methods are safe for concurrent use.
// Internal methods ending with *Locked assume mu is already held.
type Supervisor struct {
    // === LOCKING STRATEGY ===
    // mu protects all fields below except where noted.
    // RLock for reads, Lock for writes.
    // NEVER call child methods while holding write lock (deadlock risk).
    mu sync.RWMutex

    // === PROTECTED FIELDS (guarded by mu) ===
    workers   map[string]*WorkerContext  // mu protects
    children  map[string]*Supervisor     // mu protects
    userSpec  config.UserSpec            // mu protects

    // === IMMUTABLE AFTER CONSTRUCTION (no locking needed) ===
    logger     *slog.Logger
    store      storage.TriangularStoreInterface
    workerType string

    // === ATOMIC FIELDS (use atomic operations) ===
    started atomic.Bool  // atomic operations only

    // === SEPARATE LOCKS ===
    ctxMu sync.RWMutex  // Protects ctx/ctxCancel only (independent of mu)
    ctx   context.Context
    ctxCancel context.CancelFunc
}
```

**Lock Ordering Documentation:**
```go
// LOCK ORDERING RULES (enforce via code review):
//
// 1. Parent before child: parent.mu > child.mu
//    CORRECT: parent.mu.Lock() -> child.mu.Lock()
//    WRONG: child.mu.Lock() -> parent.mu.Lock()
//
// 2. Never re-enter: Do not acquire same lock twice in call stack
//    WRONG: processSignal() locks -> reconcileChildren() re-locks
//
// 3. Release before blocking: Release locks before child.Shutdown()
//    WRONG: Hold parent write lock -> child.Shutdown() blocks
//    CORRECT: Release parent lock -> child.Shutdown() -> re-acquire if needed
//
// ENFORCEMENT: Code review + runtime checks in debug builds
```

**Method Documentation:**
```go
// GetChildren returns a snapshot of child supervisors.
//
// THREAD SAFETY: Safe for concurrent use (acquires read lock internally).
// LOCKING: Acquires s.mu (read lock) for duration of copy.
// RETURNS: Copy of children map, safe to iterate without holding lock.
func (s *Supervisor) GetChildren() map[string]*Supervisor {
    s.mu.RLock()
    defer s.mu.RUnlock()

    children := make(map[string]*Supervisor, len(s.children))
    for k, v := range s.children {
        children[k] = v
    }
    return children
}
```

**Effort:** 1-2 hours
**Files:** `supervisor.go` (add comments), `docs/design/lock-ordering.md` (document patterns)

---

## Part 4: Anti-Patterns to Avoid

### ❌ Don't: Type-System Lock Enforcement

```go
// DON'T DO THIS (not Go-idiomatic)
type Locked[T any] struct {
    value *T
    mu    *sync.RWMutex
}

func (l *Locked[T]) Read(fn func(*T)) {
    l.mu.RLock()
    defer l.mu.RUnlock()
    fn(l.value)
}
```

**Why it's bad:**
- Clunky API (callback hell)
- Doesn't prevent deadlocks
- Not how Go developers think
- stdlib doesn't use this pattern

**Go way:**
```go
// Simple, clear, obvious
type Supervisor struct {
    mu       sync.RWMutex
    children map[string]*Supervisor
}

func (s *Supervisor) GetChildren() map[string]*Supervisor {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return copyMap(s.children)
}
```

---

### ❌ Don't: Panic on Lock Hierarchy Violations

```go
// DON'T DO THIS (too aggressive)
func (s *Supervisor) acquireLock() {
    if s.parent != nil && s.parent.isLocked() {
        panic("lock hierarchy violation: parent locked before child")
    }
    s.mu.Lock()
}
```

**Why it's bad:**
- Panics in production are dangerous
- Checking parent lock state is racy
- Adds overhead to every lock acquisition

**Go way:**
```go
// Document hierarchy, let race detector catch issues
type Supervisor struct {
    // LOCK ORDER: parent.mu > child.mu
    mu sync.RWMutex
}

// In tests:
// go test -race ./pkg/fsmv2/...
```

---

### ❌ Don't: Over-Engineer Static Analysis

```go
// DON'T BUILD THIS (too complex, low ROI)
// Custom AST walker to detect re-entrancy at compile time
// 500+ lines of brittle code for 1-2 bug catches per year
```

**Why it's bad:**
- Go's AST analysis can't track cross-function lock state
- False positives frustrate developers
- High maintenance cost

**Go way:**
```go
// Use existing tools:
// - go test -race (built-in race detector)
// - staticcheck (community standard linter)
// - golangci-lint (meta-linter)

// Add simple vet check for naming convention:
// *Locked methods should only call other *Locked methods
```

---

### ❌ Don't: Message-Passing Everything

```go
// DON'T REFACTOR TO THIS (wrong abstraction)
type Supervisor struct {
    commands chan Command  // No locks!
    state    *State
}

func (s *Supervisor) Run() {
    for cmd := range s.commands {
        cmd.Execute(s.state)  // Single goroutine owns state
    }
}
```

**Why it's bad:**
- Supervisor is not a message queue (it's a state manager)
- Channels add latency and complexity
- Go has both mutexes AND channels for a reason

**Go way:**
```go
// Use mutexes for state protection
// Use channels for work coordination (if needed)
type Supervisor struct {
    mu       sync.RWMutex  // Protect shared state
    children map[string]*Supervisor
}
```

---

## Part 5: Implementation Plan

### Phase 1: Lifecycle Logging (Week 1)

**Goal:** Make supervisor operations visible

**Tasks:**
1. Add `LifecycleLogger` wrapper around `log/slog`
2. Instrument supervisor methods:
   - `tickWorker()` - start/complete
   - `processSignal()` - signal type, outcome
   - `reconcileChildren()` - adds/updates/removes
   - `Shutdown()` - start/complete with duration
3. Add context propagation (worker ID, supervisor type)
4. Enable via `LIFECYCLE_DEBUG=1` environment variable

**Files to modify:**
- `pkg/fsmv2/supervisor/lifecycle.go` (new)
- `pkg/fsmv2/supervisor/supervisor.go` (integrate logger)
- `pkg/fsmv2/supervisor/supervisor_test.go` (test with logging enabled)

**Code Example:**
```go
// In supervisor.go
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    s.lifecycleLog.TickStart(ctx, workerID)
    defer func(start time.Time) {
        s.lifecycleLog.TickComplete(ctx, workerID, time.Since(start))
    }(time.Now())

    // ... existing logic
}
```

**Testing:**
```bash
LIFECYCLE_DEBUG=1 go test -v ./pkg/fsmv2/supervisor/... -run TestSupervisor_Deadlock
# Should show tick operations, lock acquisitions, child shutdowns
```

**Estimated Effort:** 4-6 hours

---

### Phase 2: Lock Documentation (Week 1)

**Goal:** Document lock ordering and protected fields

**Tasks:**
1. Add lock ordering comment to Supervisor type
2. Document protected fields inline
3. Mark immutable fields
4. Add method-level locking documentation
5. Create `docs/design/lock-ordering.md` reference

**Files to modify:**
- `pkg/fsmv2/supervisor/supervisor.go` (add comments)
- `docs/design/lock-ordering.md` (new)

**Code Example:**
```go
// Supervisor manages worker lifecycle and hierarchical composition.
//
// LOCK ORDERING: parent.mu > child.mu
// NEVER acquire child lock while holding parent write lock.
type Supervisor struct {
    // === LOCKING STRATEGY ===
    mu sync.RWMutex  // Protects fields below

    // === PROTECTED (guarded by mu) ===
    workers  map[string]*WorkerContext  // mu protects
    children map[string]*Supervisor     // mu protects

    // === IMMUTABLE (no locking) ===
    logger     *slog.Logger
    workerType string
}
```

**Estimated Effort:** 2-3 hours

---

### Phase 3: Clean Boundaries (Week 2)

**Goal:** Unexport mutex, ensure API safety

**Tasks:**
1. Change `mu sync.RWMutex` to `mu sync.RWMutex` (lowercase)
2. Audit all exported methods ensure they acquire locks
3. Add defensive copying where needed (GetChildren)
4. Add godoc thread-safety comments
5. Run tests with `-race` detector

**Files to modify:**
- `pkg/fsmv2/supervisor/supervisor.go` (unexport mu)
- All files that reference `Supervisor.mu` (fix)

**Code Example:**
```go
// Before:
type Supervisor struct {
    Mu sync.RWMutex  // EXPORTED - bad
}

// After:
type Supervisor struct {
    mu sync.RWMutex  // Unexported - good
}

// Ensure methods lock internally:
func (s *Supervisor) GetChildren() map[string]*Supervisor {
    s.mu.RLock()  // Method handles locking
    defer s.mu.RUnlock()
    // ...
}
```

**Testing:**
```bash
# Run with race detector to catch any missed locks
go test -race ./pkg/fsmv2/supervisor/...
```

**Estimated Effort:** 3-4 hours

---

### Phase 4: Enforcement via Linting (Week 2)

**Goal:** Catch re-entrancy at review time

**Tasks:**
1. Create simple `go vet` checker for `*Locked` naming convention
2. Add to CI pipeline
3. Optionally: Add debug build with `TryLock()` assertions

**Files to create:**
- `tools/lockcheck/lockcheck.go` (custom vet checker)
- `.github/workflows/ci.yml` (add lint step)

**Code Example:**
```go
// tools/lockcheck/lockcheck.go
package main

func checkLockedNaming(pass *analysis.Pass) {
    for _, file := range pass.Files {
        ast.Inspect(file, func(n ast.Node) bool {
            fn, ok := n.(*ast.FuncDecl)
            if !ok {
                return true
            }

            // Check: *Locked methods should only call other *Locked methods
            if strings.HasSuffix(fn.Name.Name, "Locked") {
                // ... AST inspection logic
            }
            return true
        })
    }
}
```

**CI Integration:**
```yaml
# .github/workflows/ci.yml
- name: Run custom lock checker
  run: go vet -vettool=$(which lockcheck) ./pkg/fsmv2/...
```

**Estimated Effort:** 4-6 hours

---

## Part 6: Tools & Workflow

### Essential Go Tools (Already Available)

**1. Race Detector (`go test -race`)**
- **What it does:** Detects data races at runtime
- **When to use:** Every test run in CI
- **Catches:** Missing locks, incorrect lock scope
- **Example:**
  ```bash
  go test -race ./pkg/fsmv2/supervisor/...
  ```

**2. Mutex Profiling (`runtime/pprof`)**
- **What it does:** Shows lock contention hotspots
- **When to use:** Performance investigations
- **Catches:** Locks held too long, high contention
- **Example:**
  ```bash
  go test -mutexprofile=mutex.out ./pkg/fsmv2/...
  go tool pprof mutex.out
  ```

**3. staticcheck**
- **What it does:** Advanced linter (catches common bugs)
- **When to use:** Pre-commit, CI
- **Catches:** Defer in loop, incorrect Lock/Unlock patterns
- **Example:**
  ```bash
  staticcheck ./pkg/fsmv2/...
  ```

**4. golangci-lint (meta-linter)**
- **What it does:** Runs 50+ linters including staticcheck
- **When to use:** Pre-commit via lefthook
- **Catches:** Everything staticcheck + more
- **Example:**
  ```bash
  golangci-lint run ./pkg/fsmv2/...
  ```

---

### Debugging Workflow

**1. Deadlock Investigation**

```bash
# Step 1: Reproduce with lifecycle logging
LIFECYCLE_DEBUG=1 go test -v -run TestDeadlock 2>&1 | tee debug.log

# Step 2: Find stuck operations
grep 'tick_start' debug.log | grep -v 'tick_complete'

# Step 3: Get goroutine dump (if test hangs)
kill -SIGQUIT <test-pid>  # Sends SIGQUIT, prints all goroutine stacks

# Step 4: Analyze stack traces
grep 'sync.(*RWMutex)' /tmp/goroutine-dump.txt
```

**2. Re-Entrancy Detection**

```bash
# Run with debug build (TryLock assertions)
go test -tags=debug -v ./pkg/fsmv2/supervisor/...

# If panic occurs, get full stack trace
GOTRACEBACK=all go test -tags=debug -v -run TestReentrancy
```

**3. Performance Profiling**

```bash
# Profile lock contention
go test -bench=. -mutexprofile=mutex.out ./pkg/fsmv2/supervisor/...

# Analyze hotspots
go tool pprof mutex.out
(pprof) top10
(pprof) list Supervisor.tickWorker
```

---

## Part 7: When to Use Each Tool

| Problem | Go-Idiomatic Solution | NOT Go-Idiomatic |
|---------|----------------------|------------------|
| **Can't see operations** | Lifecycle logging (slog) | Time-travel debugging |
| **Data races** | `-race` detector | Static lock analysis |
| **Re-entrancy** | Naming convention + vet | Runtime hierarchy enforcement |
| **Lock ordering** | Comments + code review | Compile-time borrow checker |
| **Deadlocks** | Lifecycle logs + stack dumps | Dedicated deadlock detector goroutine |
| **Performance** | `-mutexprofile` | Custom tracing infrastructure |

---

## Part 8: Concrete Next Steps

### This Week (4-6 hours total)

**Day 1: Lifecycle Logging (2 hours)**
1. Create `pkg/fsmv2/supervisor/lifecycle.go`
2. Integrate in `tickWorker()`, `processSignal()`, `Shutdown()`
3. Test with `LIFECYCLE_DEBUG=1`

**Day 2: Lock Documentation (1 hour)**
1. Add type-level lock ordering comment
2. Document protected fields inline
3. Mark immutable fields

**Day 3: Clean Boundaries (2 hours)**
1. Unexport `mu` field
2. Audit exported methods for locking
3. Run `go test -race`

### Next Week (4-6 hours total)

**Day 1: Optional Enforcement (4 hours)**
1. Create simple vet checker for `*Locked` naming
2. Add to CI pipeline
3. Optionally add debug build with assertions

**Day 2: Documentation (1 hour)**
1. Create `docs/design/lock-ordering.md`
2. Add examples of correct/incorrect patterns

---

## Conclusion

**Key Takeaways:**

1. **Go favors observability over enforcement** - Good logging beats compile-time checks
2. **Use stdlib patterns** - Comment conventions, unexported mutexes, context propagation
3. **Race detector is your friend** - Catches 90% of concurrency bugs
4. **Keep it simple** - Over-engineering (type gymnastics, message-passing refactors) fights Go's design

**The Go Way:**
- ✅ Lifecycle logging with `log/slog`
- ✅ Unexported mutexes + clear API design
- ✅ Comment-based lock documentation
- ✅ Runtime checks in debug builds (not production)
- ✅ Use `-race` detector religiously

**Not The Go Way:**
- ❌ Compile-time lock safety (Rust-style)
- ❌ Message-passing for everything (Erlang-style)
- ❌ Runtime lock hierarchy enforcement (Linux-style)
- ❌ Heavy static analysis tools (limited ROI in Go)

**Estimated Total Effort:** 8-12 hours for Phases 1-3 (user's priorities)
**Maintenance Burden:** Low (stdlib patterns, no custom infrastructure)
**Long-term Benefit:** High (prevents future deadlocks, improves debuggability)

---

## References

**Go Stdlib Patterns:**
- `sync/rwmutex.go` - Lock documentation conventions
- `net/http/server.go` - Unexported mutex pattern
- `log/slog` - Structured logging
- `context` - Request-scoped values

**Popular Go Projects:**
- Kubernetes: Lifecycle logging with structured fields
- CockroachDB: Comment-based lock ordering
- etcd: Unexported mutexes, defensive copying
- HashiCorp Vault: Context propagation for request IDs

**Go Blog Posts:**
- "Share Memory By Communicating" (but know when to use mutexes)
- "Go's Declaration Syntax" (why simplicity matters)
- "The Go Memory Model" (happens-before relationships)
