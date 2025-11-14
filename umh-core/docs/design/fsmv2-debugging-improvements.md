# FSMv2 Debugging Improvements: Comprehensive Analysis

**Status:** Design Document
**Created:** 2025-11-13
**Context:** Post-mortem analysis of FSMv2 supervisor deadlock debugging experience
**Author:** Claude Code Analysis using Creativity Techniques

## Executive Summary

We recently debugged a deadlock in FSMv2 supervisor child cleanup that took significant time to diagnose. This document applies multiple creativity techniques (SCAMPER, Six Thinking Hats, First Principles, Lateral Thinking, Forced Connections) to generate comprehensive solutions for preventing, detecting, and debugging similar issues in the future.

**Key Findings:**

- **Root Problem:** Lock visibility and re-entrancy detection gaps
- **Top Impact Areas:** Prevention (compile-time), Detection (runtime), Forensics (post-mortem)
- **Quick Wins:** 3 improvements implementable in < 4 hours each
- **Moonshots:** 2 ambitious transformative approaches

**Recommendations Priority:**

1. **Immediate (< 4 hours):** Lock operation tracer, test timeout tuning, lifecycle logging
2. **Short-term (1-2 days):** Lock context manager, deadlock detector goroutine
3. **Medium-term (1 week):** Static lock analysis tool, lock profiling infrastructure
4. **Long-term (2+ weeks):** Formal lock hierarchy, message-passing refactor evaluation

---

## 1. Problem Analysis Using Creativity Techniques

### 1.1 What Made This Hard to Debug?

**Timeline of Pain:**

1. **Test failure:** Expected 0 children, got 1 (5s timeout)
2. **Added diagnostics:** processSignal() called reconcileChildren([])
3. **Result:** 120-second deadlock (much worse!)
4. **Root cause:** Nested lock acquisition with blocking operations
   - processSignal() released parent lock
   - reconcileChildren() re-acquired parent lock (looked safe!)
   - child.Shutdown() acquired child lock + waited (blocking)
   - All readers starved waiting for parent write lock

**Missing Information Categories:**

| Category | What We Lacked | Impact |
|----------|---------------|--------|
| **Lock Ownership** | Which goroutine held write lock | Couldn't identify blocker |
| **Re-entrancy** | That reconcileChildren() re-locks | Silent trap |
| **Blocking Operations** | That child.Shutdown() waits | Unexpected deadlock |
| **Hierarchy Visibility** | Parent→Child lock relationship | Nested lock invisible |
| **Early Warning** | No detection before 120s timeout | Too slow |
| **Lifecycle State** | What operations were in progress | Blind debugging |

### 1.2 SCAMPER Analysis

**Substitute:** What if we replaced RWMutex with something else?

- **Lock-free data structures** (atomic operations, CAS loops)
  - Pros: No deadlocks possible, better performance
  - Cons: Complex to implement correctly, harder to reason about

- **Channel-based synchronization** (single goroutine owns data)
  - Pros: Go-idiomatic, eliminates shared state
  - Cons: More verbose, potential channel deadlocks instead

- **Transactional memory** (STM-style)
  - Pros: Composable, automatic retry
  - Cons: No Go stdlib support, requires external library

**Combine:** What if we combined deadlock detection with logging?

- **Lock tracing with timeout detection**
  - Each lock acquisition logs: timestamp, goroutine ID, stack trace
  - Background goroutine monitors acquisitions > 5s
  - Logs warning with full context before deadlock happens

- **Lock graph with cycle detection**
  - Track which locks are held when acquiring others
  - Detect cycles in acquisition graph
  - Alert before blocking indefinitely

**Adapt:** How do other systems handle lock debugging?

- **Linux kernel lockdep**
  - Compile-time + runtime lock ordering verification
  - Detects circular dependencies before they deadlock
  - Tracks lock classes and acquisition contexts

- **Rust borrow checker**
  - Compile-time verification of exclusive access
  - Prevents data races and deadlocks at compile time
  - No runtime overhead

- **Database MVCC**
  - No read locks, readers never block writers
  - Snapshots for consistent reads
  - Conflicts detected and rolled back

**Modify:** What if locks had timeouts or priorities?

- **Timed locks** (TryLockWithTimeout)
  - Lock acquisition fails after N seconds
  - Allows recovery vs. indefinite blocking
  - Requires careful timeout tuning

- **Priority-based scheduling**
  - Reader/writer priorities
  - Prevents starvation
  - More complex lock implementation

**Put to other use:** Could we repurpose existing Go tools?

- **pprof mutex profiling** (already exists!)
  - Shows lock contention hotspots
  - Requires runtime/pprof integration
  - Could add to test suite automatically

- **race detector** (detects data races, not deadlocks)
  - Could annotate with lock expectations
  - Helps find missing locks

**Eliminate:** What if we removed locks in certain paths?

- **Immutable data structures**
  - No locks needed for reads
  - Structural sharing for efficiency
  - Copy-on-write for updates

- **Message passing only**
  - No shared state = no locks
  - Erlang/Actor model style
  - Significant architectural change

**Reverse:** What if locks logged their release instead of acquisition?

- **Lock lifetime tracking**
  - Log: Acquired at X, Released at Y, Duration = Y-X
  - Identify long-held locks
  - Correlate with test failures

### 1.3 Six Thinking Hats Analysis

**White Hat (Facts):** What data would have caught this faster?

- Lock acquisition stack traces (who acquired lock)
- Lock holder goroutine ID (which goroutine has write lock)
- Lock acquisition timestamps (how long held)
- Blocked goroutines list (who's waiting)
- Lock hierarchy graph (parent→child relationship)
- Operation lifecycle states (removing worker X, shutting down child Y)

**Red Hat (Feelings):** What frustrations did we experience?

- **Invisibility frustration:** "Where is the deadlock?" (no visibility)
- **Silent traps:** reconcileChildren() looked safe (re-entrancy not obvious)
- **Slow feedback:** 120s timeout (testing feels interminable)
- **Blind debugging:** Adding logs manually (should be automatic)
- **Uncertainty:** "Will this fix it or make it worse?" (no confidence)
- **Wasted time:** Multiple debugging rounds (could have been prevented)

**Black Hat (Caution):** What risks do solutions introduce?

- **Performance overhead:** Logging every lock acquisition (production impact)
- **Complexity creep:** Adding infrastructure (maintenance burden)
- **False positives:** Timeout-based detection (legitimate long operations)
- **Tool dependency:** External analysis tools (breaks without them)
- **Over-engineering:** Building infrastructure we rarely use (YAGNI violation)
- **Migration cost:** Changing lock patterns (requires refactoring)

**Yellow Hat (Benefits):** What's the ideal debugging experience?

- **Instant feedback:** Test failure shows exact deadlock location immediately
- **Prevention alerts:** "Warning: calling reconcileChildren() will re-lock" (before running)
- **Visual clarity:** Lock hierarchy diagram showing acquisition order
- **Automatic diagnosis:** Test runner detects deadlock and dumps context
- **Confidence:** "This fix won't cause deadlock" (verified before running)
- **Production safety:** Same debugging tools work in production (not just tests)

**Green Hat (Creativity):** Wild ideas without constraints

- **AI-powered lock analyzer:** ML model predicts deadlock likelihood from code changes
- **Time-travel debugging:** Replay lock acquisitions in reverse to find root cause
- **Lock DNA:** Each lock has unique ID, parent/child relationships tracked automatically
- **Quantum debugging:** Observe lock state without perturbing system (Heisenberg-free)
- **Social debugging:** Locks "negotiate" with each other to prevent conflicts
- **Fuzzy locks:** Probabilistic acquisition (fail fast vs. block forever)

**Blue Hat (Process):** How to implement systematically?

1. **Categorize by effort:** Quick wins vs. long-term investments
2. **Layer approach:** Prevention → Detection → Forensics
3. **Incremental adoption:** Start with test suite, expand to production
4. **Measure effectiveness:** Track debugging time before/after
5. **Document patterns:** Build library of safe/unsafe lock patterns
6. **Review gates:** Lock changes require specialized review

### 1.4 First Principles Analysis

**Core Problem:** We cannot observe lock state and ownership relationships.

**Hard Constraints:**
- Go RWMutex has no introspection API (can't ask "who holds this lock?")
- Goroutines have IDs but they're not officially exposed
- Stack traces are expensive to capture continuously
- Runtime overhead must be minimal in production

**Assumed Constraints (questionable):**
- ✗ Must use RWMutex (could use channels, lock-free structures)
- ✗ Must lock at this granularity (could redesign data access)
- ✗ Can't add instrumentation (could for tests/development)
- ✗ Deadlock detection requires external tools (could build in)

**From Scratch Design:**
- Locks would have owners (explicit goroutine tracking)
- Locks would have hierarchies (parent/child relationships)
- Lock violations would fail fast (re-entrancy detected)
- Tests would timeout intelligently (detect blocking vs. slow)
- Operations would be observable (lifecycle logging built-in)

### 1.5 Lateral Thinking: Adjacent Problem Solutions

**How would a database handle this?** (MVCC, transaction logs)

- **MVCC approach:** Readers use snapshots, never block writers
  - Apply: Supervisor methods return snapshot of children list
  - Writers copy-on-write, atomically swap pointer
  - No read locks needed, eliminates deadlock class

- **Transaction log approach:** Record all state changes
  - Apply: Supervisor emits events for all operations
  - Replay events to reconstruct deadlock scenario
  - Automatic audit trail

**How would a kernel handle this?** (lockdep, lock validator)

- **Lock classes:** Each lock type has validation rules
  - Apply: SupervisorLock class with hierarchy rules
  - Compile-time checking of lock order
  - Runtime validation in debug builds

- **Lockdep approach:** Track lock dependencies
  - Apply: Build dependency graph at runtime
  - Detect cycles before they deadlock
  - Fail fast with helpful error

**How would Rust handle this?** (compile-time borrow checker)

- **Ownership model:** Only one mutable reference
  - Apply: Methods consume supervisor, return new one
  - Compiler prevents concurrent access
  - No runtime overhead

- **Lifetime annotations:** Explicit scope tracking
  - Apply: Document lock lifetime expectations in comments
  - Lint tool validates lifetimes
  - Catches mistakes before running

**How would Erlang handle this?** (message passing, no shared state)

- **Actor model:** Each supervisor is isolated
  - Apply: Supervisor goroutine owns all state
  - External API sends messages via channels
  - No locks needed at all

- **Let it crash:** Failures isolated to actor
  - Apply: Child failures don't block parent
  - Supervision tree restart strategies
  - Timeouts built into message passing

### 1.6 Forced Connections

**Locks as Traffic Lights:**
- Red/Yellow/Green states visible
- Traffic cameras record violations
- Timing coordinated to prevent gridlock
- **Insight:** Lock states should be observable, violations recorded

**Locks as Real Estate:**
- Owners have deeds (recorded ownership)
- Zoning laws (lock hierarchy rules)
- Property inspections (validation)
- **Insight:** Formalize ownership, rules, inspections

**Locks as Conversations:**
- Participants take turns speaking
- Facilitator prevents talking over each other
- Minutes recorded for later review
- **Insight:** Lock arbitrator, acquisition log

---

## 2. Solution Categories

### 2.1 Preventive Solutions (Catch Before They Happen)

**P1: Static Lock Analysis Tool**
- Analyze code for lock acquisition patterns
- Detect potential re-entrancy issues
- Warn on calls to functions that re-lock
- Integration: Pre-commit hook or CI

**P2: Lock Hierarchy Enforcement**
- Define parent→child lock ordering
- Compile-time or runtime verification
- Fail on hierarchy violations
- Documentation: Lock graph diagram

**P3: Lock-Free Supervisor State**
- Replace locks with atomic operations
- Copy-on-write child list
- Snapshot-based reads
- Message passing for mutations

**P4: Borrow Checker-Inspired Annotations**
- Document lock expectations in comments
- Linting tool validates patterns
- Review checklist for lock changes
- Training: Safe lock patterns guide

**P5: Code Generation for Safe Access**
- Generated accessor methods
- Automatic lock lifetime management
- Type-safe guarantees
- Reduced manual lock handling

### 2.2 Detective Solutions (Make Issues Visible)

**D1: Lock Operation Tracer**
- Wrap RWMutex to log acquisitions/releases
- Include: goroutine ID, timestamp, stack trace
- Conditional: Enable for tests or debug builds
- Output: Structured log for analysis

**D2: Deadlock Detector Goroutine**
- Background goroutine monitors lock acquisitions
- Detect locks held > threshold (e.g., 5s)
- Alert with context: holder, waiters, stacks
- Automatic in test suite

**D3: Lock Hierarchy Visualizer**
- Real-time display of lock relationships
- Show parent→child acquisitions
- Highlight cycles or violations
- Development tool (not production)

**D4: Lifecycle Logging Infrastructure**
- Structured logging for supervisor operations
- Events: worker added/removed, child shutdown
- Correlation IDs for tracing
- Searchable logs in tests

**D5: Intelligent Test Timeouts**
- Detect blocked goroutines before timeout
- Dump diagnostics automatically
- Differentiate slow vs. deadlocked
- Fail fast with context

### 2.3 Forensic Solutions (Debug After Failure)

**F1: Lock Acquisition History**
- Circular buffer of recent acquisitions
- Dumped on deadlock or timeout
- Shows sequence leading to failure
- Minimal overhead (fixed size buffer)

**F2: Mutex Profiling Integration**
- Enable pprof mutex profiling in tests
- Automatic report generation on failure
- Shows contention hotspots
- Built into test runner

**F3: Deadlock Post-Mortem Tool**
- Parse stack traces to identify deadlock
- Reconstruct lock acquisition order
- Generate HTML report with visualization
- Run automatically on test failures

**F4: Time-Travel Debugging Support**
- Record all state transitions
- Replay backwards from deadlock
- Find trigger operation
- Heavy overhead (test-only)

**F5: Enhanced Stack Trace Analysis**
- Annotate traces with lock state
- Show held locks at each frame
- Identify blocking operations
- Pretty-print for readability

### 2.4 Structural Solutions (Eliminate Issue Class)

**S1: Message-Passing Supervisor**
- Replace locks with channels
- Single goroutine owns state
- External API sends messages
- No shared memory

**S2: Immutable Supervisor State**
- Copy-on-write data structures
- No mutation of shared state
- Functional API design
- No locks needed for reads

**S3: Lock-Free Data Structures**
- Atomic operations only
- CAS loops for updates
- Wait-free reads
- Complex but deadlock-free

**S4: Simplified Lifecycle**
- Reduce operations requiring locks
- Minimize lock scope
- Separate read/write paths
- Less coordination needed

**S5: Supervisor Per Goroutine**
- Thread-local state (goroutine-local)
- No sharing between goroutines
- Reduces contention
- Higher memory usage

---

## 3. Top 5 Concrete Proposals

### Proposal 1: Lock Operation Tracer (Detective)

**Description:** Wrapper around RWMutex that logs all Lock/Unlock operations with context.

**What It Would Have Shown:**

```
[supervisor_test.go:123] [goroutine 45] RLock acquired (GetChildren)
[supervisor_test.go:156] [goroutine 45] RLock acquired (calculateHierarchySize)
[supervisor.go:89] [goroutine 34] Lock acquired (processSignal)
[supervisor.go:95] [goroutine 34] Lock released (processSignal)
[supervisor.go:234] [goroutine 34] Lock acquired (reconcileChildren)  ← Re-lock visible!
[supervisor.go:240] [goroutine 34] waiting on child.Shutdown()...
[supervisor_test.go:123] [goroutine 45] blocked on RLock (parent locked by goroutine 34)
WARNING: Lock held by goroutine 34 for 5 seconds
```

**Implementation Approach:**

```go
// pkg/fsmv2/internal/debug/tracedlock.go
package debug

import (
    "fmt"
    "runtime"
    "sync"
    "time"
)

type TracedRWMutex struct {
    mu              sync.RWMutex
    name            string
    acquisitions    chan Acquisition
    enabled         bool
}

type Acquisition struct {
    Type        string // "Lock", "RLock", "Unlock", "RUnlock"
    GoroutineID uint64
    Timestamp   time.Time
    Stack       []byte
}

func NewTracedRWMutex(name string) *TracedRWMutex {
    t := &TracedRWMutex{
        name:         name,
        acquisitions: make(chan Acquisition, 1000),
        enabled:      os.Getenv("TRACE_LOCKS") == "1",
    }
    if t.enabled {
        go t.monitor()
    }
    return t
}

func (t *TracedRWMutex) Lock() {
    if t.enabled {
        t.trace("Lock", "acquiring")
    }
    t.mu.Lock()
    if t.enabled {
        t.trace("Lock", "acquired")
    }
}

func (t *TracedRWMutex) Unlock() {
    if t.enabled {
        t.trace("Unlock", "released")
    }
    t.mu.Unlock()
}

func (t *TracedRWMutex) trace(op, status string) {
    stack := make([]byte, 4096)
    n := runtime.Stack(stack, false)
    t.acquisitions <- Acquisition{
        Type:        op,
        GoroutineID: getGoroutineID(),
        Timestamp:   time.Now(),
        Stack:       stack[:n],
    }
}

func (t *TracedRWMutex) monitor() {
    holdings := make(map[uint64]Acquisition)
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case acq := <-t.acquisitions:
            if acq.Type == "Lock" || acq.Type == "RLock" {
                holdings[acq.GoroutineID] = acq
            } else {
                delete(holdings, acq.GoroutineID)
            }

        case <-ticker.C:
            now := time.Now()
            for gid, acq := range holdings {
                if now.Sub(acq.Timestamp) > 5*time.Second {
                    fmt.Printf("WARNING: %s held by goroutine %d for %v\n",
                        t.name, gid, now.Sub(acq.Timestamp))
                    fmt.Printf("Stack:\n%s\n", acq.Stack)
                }
            }
        }
    }
}
```

**Usage in Supervisor:**

```go
// Replace:
// mu sync.RWMutex

// With:
mu *debug.TracedRWMutex

// In NewSupervisor:
mu: debug.NewTracedRWMutex("supervisor"),
```

**Effort Estimate:** 4-6 hours
- Implementation: 3 hours
- Testing: 1 hour
- Documentation: 1 hour
- Integration: 1 hour

**Tradeoffs:**
- **Pro:** Immediate visibility into lock operations
- **Pro:** Minimal code changes (drop-in replacement)
- **Pro:** Conditional (only enabled when TRACE_LOCKS=1)
- **Con:** Small performance overhead when enabled (~10% for mutex operations)
- **Con:** Channel buffering can lose events under extreme load
- **Con:** Requires goroutine ID extraction (non-standard)

---

### Proposal 2: Deadlock Detector Goroutine (Detective)

**Description:** Background goroutine that monitors all lock acquisitions and detects when locks are held for too long or cyclic dependencies exist.

**What It Would Have Shown:**

```
DEADLOCK DETECTED (after 5s)

Lock Hierarchy:
  supervisor[parent] - WRITE lock held by goroutine 34 (5.2s)
    ↓ waiting for
  supervisor[child] - WRITE lock acquired by goroutine 34 (4.8s)
    ↓ blocking
  3 goroutines waiting for READ lock on supervisor[parent]
    - goroutine 45 (GetChildren)
    - goroutine 46 (calculateHierarchySize)
    - goroutine 47 (metrics collector)

Root Cause: Nested lock acquisition
  goroutine 34:
    1. Acquired parent lock (processSignal:89)
    2. Released parent lock (processSignal:95)
    3. Re-acquired parent lock (reconcileChildren:234) ← PROBLEM
    4. Acquired child lock (shutdown:345)
    5. Blocked on child completion

Test failure context: supervisor_test.go:123 (GetChildren)
```

**Implementation Approach:**

```go
// pkg/fsmv2/internal/debug/deadlock_detector.go
package debug

import (
    "context"
    "fmt"
    "sync"
    "time"
)

type DeadlockDetector struct {
    mu           sync.Mutex
    acquisitions map[uint64]*LockChain // goroutine ID → lock chain
    locks        map[string]*LockState // lock name → state
    threshold    time.Duration
    ctx          context.Context
    cancel       context.CancelFunc
}

type LockChain struct {
    GoroutineID uint64
    Locks       []LockHold
}

type LockHold struct {
    LockName  string
    Type      string // "read" or "write"
    AcquiredAt time.Time
    Stack     []byte
}

type LockState struct {
    Name       string
    WriteHolder uint64 // goroutine ID (0 = none)
    ReadHolders []uint64
    Waiters    []uint64
}

func NewDeadlockDetector(threshold time.Duration) *DeadlockDetector {
    ctx, cancel := context.WithCancel(context.Background())
    d := &DeadlockDetector{
        acquisitions: make(map[uint64]*LockChain),
        locks:        make(map[string]*LockState),
        threshold:    threshold,
        ctx:          ctx,
        cancel:       cancel,
    }
    go d.monitor()
    return d
}

func (d *DeadlockDetector) RecordAcquisition(lockName string, lockType string, gid uint64, stack []byte) {
    d.mu.Lock()
    defer d.mu.Unlock()

    chain := d.acquisitions[gid]
    if chain == nil {
        chain = &LockChain{GoroutineID: gid}
        d.acquisitions[gid] = chain
    }

    chain.Locks = append(chain.Locks, LockHold{
        LockName:  lockName,
        Type:      lockType,
        AcquiredAt: time.Now(),
        Stack:     stack,
    })

    state := d.locks[lockName]
    if state == nil {
        state = &LockState{Name: lockName}
        d.locks[lockName] = state
    }

    if lockType == "write" {
        state.WriteHolder = gid
    } else {
        state.ReadHolders = append(state.ReadHolders, gid)
    }
}

func (d *DeadlockDetector) RecordRelease(lockName string, lockType string, gid uint64) {
    d.mu.Lock()
    defer d.mu.Unlock()

    chain := d.acquisitions[gid]
    if chain != nil {
        // Remove last lock of this name from chain
        for i := len(chain.Locks) - 1; i >= 0; i-- {
            if chain.Locks[i].LockName == lockName {
                chain.Locks = append(chain.Locks[:i], chain.Locks[i+1:]...)
                break
            }
        }
    }

    state := d.locks[lockName]
    if state != nil {
        if lockType == "write" {
            state.WriteHolder = 0
        } else {
            for i, holder := range state.ReadHolders {
                if holder == gid {
                    state.ReadHolders = append(state.ReadHolders[:i], state.ReadHolders[i+1:]...)
                    break
                }
            }
        }
    }
}

func (d *DeadlockDetector) monitor() {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-d.ctx.Done():
            return
        case <-ticker.C:
            d.checkForDeadlocks()
        }
    }
}

func (d *DeadlockDetector) checkForDeadlocks() {
    d.mu.Lock()
    defer d.mu.Unlock()

    now := time.Now()

    // Check for locks held too long
    for gid, chain := range d.acquisitions {
        for _, hold := range chain.Locks {
            duration := now.Sub(hold.AcquiredAt)
            if duration > d.threshold {
                fmt.Printf("POTENTIAL DEADLOCK: goroutine %d holding %s (%s) for %v\n",
                    gid, hold.LockName, hold.Type, duration)
                fmt.Printf("Stack:\n%s\n", hold.Stack)

                // Show what's waiting
                if state := d.locks[hold.LockName]; state != nil {
                    fmt.Printf("Waiters: %v\n", state.Waiters)
                }

                // Show lock chain
                fmt.Printf("Lock chain for goroutine %d:\n", gid)
                for _, h := range chain.Locks {
                    fmt.Printf("  - %s (%s) held for %v\n",
                        h.LockName, h.Type, now.Sub(h.AcquiredAt))
                }
            }
        }
    }

    // Check for cycles in lock graph
    d.detectCycles()
}

func (d *DeadlockDetector) detectCycles() {
    // Build lock dependency graph
    graph := make(map[string][]string)

    for _, chain := range d.acquisitions {
        for i := 0; i < len(chain.Locks)-1; i++ {
            from := chain.Locks[i].LockName
            to := chain.Locks[i+1].LockName
            graph[from] = append(graph[from], to)
        }
    }

    // DFS to detect cycles
    visited := make(map[string]bool)
    recStack := make(map[string]bool)

    var dfs func(node string) bool
    dfs = func(node string) bool {
        visited[node] = true
        recStack[node] = true

        for _, neighbor := range graph[node] {
            if !visited[neighbor] {
                if dfs(neighbor) {
                    return true
                }
            } else if recStack[neighbor] {
                fmt.Printf("CYCLE DETECTED: %s → %s\n", node, neighbor)
                return true
            }
        }

        recStack[node] = false
        return false
    }

    for node := range graph {
        if !visited[node] {
            if dfs(node) {
                break
            }
        }
    }
}
```

**Integration with TracedRWMutex:**

```go
var globalDetector = debug.NewDeadlockDetector(5 * time.Second)

func (t *TracedRWMutex) Lock() {
    gid := getGoroutineID()
    stack := captureStack()
    globalDetector.RecordAcquisition(t.name, "write", gid, stack)
    t.mu.Lock()
}

func (t *TracedRWMutex) Unlock() {
    gid := getGoroutineID()
    globalDetector.RecordRelease(t.name, "write", gid)
    t.mu.Unlock()
}
```

**Effort Estimate:** 8-12 hours
- Core detector: 4 hours
- Cycle detection: 2 hours
- Integration: 2 hours
- Testing: 2 hours
- Documentation: 2 hours

**Tradeoffs:**
- **Pro:** Detects deadlocks before test timeout
- **Pro:** Shows full context (who holds what, who's waiting)
- **Pro:** Cycle detection catches architectural issues
- **Con:** Higher performance overhead (goroutine + monitoring)
- **Con:** More complex implementation
- **Con:** May have false positives (legitimate long operations)

---

### Proposal 3: Static Lock Analysis Tool (Preventive)

**Description:** Static analysis tool that parses Go code to detect potential lock issues: re-entrancy, incorrect ordering, blocking operations under locks.

**What It Would Have Shown:**

```
$ go run tools/lockcheck/main.go ./pkg/fsmv2/...

pkg/fsmv2/supervisor.go:234 - WARNING: Re-entrancy detected
  Function: reconcileChildren
  Lock: s.mu (Write)
  Problem: Called from processSignal which already holds s.mu
  Stack:
    processSignal (supervisor.go:89) - acquires s.mu
    processSignal (supervisor.go:95) - releases s.mu
    processSignal (supervisor.go:98) - calls reconcileChildren
    reconcileChildren (supervisor.go:234) - re-acquires s.mu

pkg/fsmv2/supervisor.go:240 - ERROR: Blocking operation under lock
  Function: reconcileChildren
  Lock: s.mu (Write)
  Operation: child.Shutdown() (blocks waiting for completion)
  Problem: Blocking with write lock held will deadlock readers
  Suggestion: Release lock before calling Shutdown()

2 issues found (1 error, 1 warning)
```

**Implementation Approach:**

```go
// tools/lockcheck/main.go
package main

import (
    "flag"
    "fmt"
    "go/ast"
    "go/parser"
    "go/token"
    "golang.org/x/tools/go/packages"
)

type LockChecker struct {
    fset    *token.FileSet
    locks   map[string]*LockInfo // function → locks held
    issues  []Issue
}

type LockInfo struct {
    Name      string
    Type      string // "read" or "write"
    Position  token.Pos
}

type Issue struct {
    Severity string // "error", "warning"
    Position token.Position
    Message  string
    Stack    []string
}

func main() {
    flag.Parse()
    pkgs := flag.Args()

    checker := &LockChecker{
        fset:  token.NewFileSet(),
        locks: make(map[string]*LockInfo),
    }

    cfg := &packages.Config{
        Mode: packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes,
        Fset: checker.fset,
    }

    loaded, err := packages.Load(cfg, pkgs...)
    if err != nil {
        fmt.Println("Error loading packages:", err)
        return
    }

    for _, pkg := range loaded {
        for _, file := range pkg.Syntax {
            ast.Inspect(file, checker.inspect)
        }
    }

    for _, issue := range checker.issues {
        fmt.Printf("%s - %s: %s\n", issue.Position, issue.Severity, issue.Message)
        for _, frame := range issue.Stack {
            fmt.Printf("  %s\n", frame)
        }
    }

    if len(checker.issues) > 0 {
        os.Exit(1)
    }
}

func (c *LockChecker) inspect(n ast.Node) bool {
    switch node := n.(type) {
    case *ast.FuncDecl:
        c.checkFunction(node)
    case *ast.CallExpr:
        c.checkCall(node)
    }
    return true
}

func (c *LockChecker) checkFunction(fn *ast.FuncDecl) {
    // Track locks acquired in function
    var locks []LockInfo

    ast.Inspect(fn.Body, func(n ast.Node) bool {
        switch node := n.(type) {
        case *ast.ExprStmt:
            if call, ok := node.X.(*ast.CallExpr); ok {
                if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
                    switch sel.Sel.Name {
                    case "Lock", "RLock":
                        locks = append(locks, LockInfo{
                            Name:     extractLockName(sel.X),
                            Type:     lockType(sel.Sel.Name),
                            Position: sel.Pos(),
                        })
                    case "Unlock", "RUnlock":
                        // Remove from locks
                        name := extractLockName(sel.X)
                        for i, lock := range locks {
                            if lock.Name == name {
                                locks = append(locks[:i], locks[i+1:]...)
                                break
                            }
                        }
                    }
                }
            }
        case *ast.CallExpr:
            // Check if calling function that also locks
            if len(locks) > 0 {
                c.checkReentrancy(call, locks)
            }
        }
        return true
    })
}

func (c *LockChecker) checkReentrancy(call *ast.CallExpr, heldLocks []LockInfo) {
    funcName := extractFuncName(call)

    // Look up if called function acquires same lock
    if targetLocks, exists := c.locks[funcName]; exists {
        for _, held := range heldLocks {
            for _, target := range targetLocks {
                if held.Name == target.Name {
                    c.issues = append(c.issues, Issue{
                        Severity: "warning",
                        Position: c.fset.Position(call.Pos()),
                        Message:  fmt.Sprintf("Re-entrancy detected: %s already holds %s", funcName, held.Name),
                    })
                }
            }
        }
    }
}

func (c *LockChecker) checkCall(call *ast.CallExpr) {
    // Check for blocking operations under lock
    funcName := extractFuncName(call)

    blockingOps := []string{"Shutdown", "Wait", "WaitGroup.Wait", "chan receive"}

    for _, op := range blockingOps {
        if strings.Contains(funcName, op) {
            // Check if any locks held
            // (Would need more sophisticated context tracking)
            c.issues = append(c.issues, Issue{
                Severity: "error",
                Position: c.fset.Position(call.Pos()),
                Message:  fmt.Sprintf("Blocking operation %s may be called under lock", funcName),
            })
        }
    }
}
```

**Integration:**

```makefile
# Makefile
.PHONY: check-locks
check-locks:
	@echo "Checking for lock issues..."
	@go run tools/lockcheck/main.go ./pkg/fsmv2/...

.PHONY: test
test: check-locks
	@go test -v ./...
```

**CI Integration:**

```yaml
# .github/workflows/ci.yml
- name: Static Lock Analysis
  run: make check-locks
```

**Effort Estimate:** 12-20 hours
- Parser implementation: 6 hours
- Re-entrancy detection: 3 hours
- Blocking operation detection: 3 hours
- Testing on real code: 4 hours
- Documentation: 2 hours
- CI integration: 2 hours

**Tradeoffs:**
- **Pro:** Catches issues before running code
- **Pro:** No runtime overhead
- **Pro:** Can be enforced in CI
- **Con:** Complex to implement correctly
- **Con:** May have false positives (static analysis limitations)
- **Con:** Requires maintenance as codebase evolves

---

### Proposal 4: Lifecycle Logging Infrastructure (Detective)

**Description:** Structured logging framework for supervisor operations with correlation IDs, making it easy to trace operations across test failures.

**What It Would Have Shown:**

```
[supervisor] [parent:sup-abc123] Registered worker type: test-worker
[supervisor] [parent:sup-abc123] Created child supervisor: child:sup-def456
[supervisor] [parent:sup-abc123] Processing signal: unregisterWorkerType
[supervisor] [parent:sup-abc123] Removing worker type: test-worker (has 1 child: child:sup-def456)
[supervisor] [parent:sup-abc123] Calling reconcileChildren with empty target list
[supervisor] [parent:sup-abc123] reconcileChildren: Removing child: child:sup-def456
[supervisor] [parent:sup-abc123] Calling child.Shutdown() on child:sup-def456
[supervisor] [child:sup-def456] Shutdown called, waiting for tick loop completion...
[supervisor] [child:sup-def456] Tick loop stopped, shutdown complete
[supervisor] [parent:sup-abc123] Child shutdown complete, removed from children map
[test] GetChildren() called, waiting for read lock...
[test] GetChildren() acquired lock, returning 0 children
PASS (child was removed successfully)
```

**Implementation Approach:**

```go
// pkg/fsmv2/internal/logging/lifecycle.go
package logging

import (
    "fmt"
    "github.com/google/uuid"
    "log/slog"
    "os"
)

type LifecycleLogger struct {
    logger       *slog.Logger
    componentID  string
    enabled      bool
}

func NewLifecycleLogger(componentType string) *LifecycleLogger {
    componentID := fmt.Sprintf("%s:%s", componentType, uuid.New().String()[:8])

    enabled := os.Getenv("LIFECYCLE_LOGGING") == "1"

    var logger *slog.Logger
    if enabled {
        handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
            Level: slog.LevelDebug,
        })
        logger = slog.New(handler)
    }

    return &LifecycleLogger{
        logger:      logger,
        componentID: componentID,
        enabled:     enabled,
    }
}

func (l *LifecycleLogger) Info(operation string, attributes ...any) {
    if !l.enabled {
        return
    }

    attrs := []any{
        "component", l.componentID,
        "operation", operation,
    }
    attrs = append(attrs, attributes...)

    l.logger.Info("lifecycle", attrs...)
}

func (l *LifecycleLogger) Error(operation string, err error, attributes ...any) {
    if !l.enabled {
        return
    }

    attrs := []any{
        "component", l.componentID,
        "operation", operation,
        "error", err,
    }
    attrs = append(attrs, attributes...)

    l.logger.Error("lifecycle", attrs...)
}
```

**Integration in Supervisor:**

```go
type Supervisor struct {
    // ... existing fields
    logger *logging.LifecycleLogger
}

func NewSupervisor(...) *Supervisor {
    return &Supervisor{
        // ... existing initialization
        logger: logging.NewLifecycleLogger("supervisor"),
    }
}

func (s *Supervisor) processSignal(ctx context.Context, sig *SignalHandlerRegistration) error {
    s.logger.Info("processSignal",
        "signal", sig.Signal,
        "children_count", len(s.children),
    )

    // ... existing logic

    s.logger.Info("processSignal_removing_worker",
        "worker_type", sig.WorkerTypeID,
        "children", s.getChildNames(),
    )

    // ... call reconcileChildren

    s.logger.Info("processSignal_complete")
    return nil
}

func (s *Supervisor) reconcileChildren(ctx context.Context, targetChildren []ChildConfig) error {
    s.logger.Info("reconcileChildren",
        "current_count", len(s.children),
        "target_count", len(targetChildren),
    )

    for childID := range toRemove {
        s.logger.Info("removing_child",
            "child_id", childID,
        )

        child := s.children[childID]
        s.logger.Info("calling_child_shutdown",
            "child_id", childID,
        )

        if err := child.Shutdown(ctx); err != nil {
            s.logger.Error("child_shutdown_failed", err,
                "child_id", childID,
            )
            return err
        }

        s.logger.Info("child_shutdown_complete",
            "child_id", childID,
        )
    }

    s.logger.Info("reconcileChildren_complete")
    return nil
}
```

**Test Integration:**

```go
func TestSupervisor(t *testing.T) {
    // Enable lifecycle logging for this test
    t.Setenv("LIFECYCLE_LOGGING", "1")

    // ... existing test code
}
```

**Effort Estimate:** 3-5 hours
- Logger implementation: 1 hour
- Integration in supervisor: 2 hours
- Test validation: 1 hour
- Documentation: 1 hour

**Tradeoffs:**
- **Pro:** Immediate visibility into operation flow
- **Pro:** Minimal performance overhead (conditional)
- **Pro:** Easy to add to existing code
- **Pro:** Works in both tests and production
- **Con:** Requires manual instrumentation
- **Con:** Log noise if not filtered properly
- **Con:** Doesn't prevent issues, only makes them visible

---

### Proposal 5: Lock Hierarchy Enforcement (Preventive)

**Description:** Formal lock hierarchy system that enforces parent→child ordering at runtime, failing fast on violations.

**What It Would Have Shown:**

```
PANIC: Lock hierarchy violation!

Attempted acquisition:
  Lock: supervisor[parent].mu (WRITE)
  Goroutine: 34
  Stack: reconcileChildren (supervisor.go:234)

Current holdings by goroutine 34:
  1. supervisor[parent].mu (WRITE) - acquired at processSignal:89
  2. (released at processSignal:95)
  3. Re-acquiring same lock at reconcileChildren:234

Violation: Cannot re-acquire lock after releasing it in same call chain

Lock hierarchy:
  supervisor[parent].mu
    ↓ allowed children
  supervisor[child].mu

Rule violated: No re-entrancy allowed
```

**Implementation Approach:**

```go
// pkg/fsmv2/internal/lockorder/hierarchy.go
package lockorder

import (
    "fmt"
    "runtime"
    "sync"
)

// LockLevel defines position in lock hierarchy
type LockLevel int

const (
    LevelSupervisorParent LockLevel = iota
    LevelSupervisorChild
    LevelWorker
)

type HierarchicalMutex struct {
    mu          sync.RWMutex
    name        string
    level       LockLevel
    registry    *LockRegistry
}

type LockRegistry struct {
    mu             sync.Mutex
    goroutineLocks map[uint64]*LockChain
}

type LockChain struct {
    Locks []LockHold
}

type LockHold struct {
    Name      string
    Level     LockLevel
    Type      string // "read" or "write"
    Stack     []byte
}

var globalRegistry = &LockRegistry{
    goroutineLocks: make(map[uint64]*LockChain),
}

func NewHierarchicalMutex(name string, level LockLevel) *HierarchicalMutex {
    return &HierarchicalMutex{
        name:     name,
        level:    level,
        registry: globalRegistry,
    }
}

func (h *HierarchicalMutex) Lock() {
    gid := getGoroutineID()
    stack := captureStack()

    h.registry.checkAndRecord(gid, h.name, h.level, "write", stack)
    h.mu.Lock()
}

func (h *HierarchicalMutex) Unlock() {
    gid := getGoroutineID()
    h.registry.release(gid, h.name)
    h.mu.Unlock()
}

func (r *LockRegistry) checkAndRecord(gid uint64, name string, level LockLevel, typ string, stack []byte) {
    r.mu.Lock()
    defer r.mu.Unlock()

    chain := r.goroutineLocks[gid]
    if chain == nil {
        chain = &LockChain{}
        r.goroutineLocks[gid] = chain
    }

    // Rule 1: Cannot acquire lower-level lock while holding higher-level lock
    for _, hold := range chain.Locks {
        if level < hold.Level {
            panic(fmt.Sprintf(
                "Lock hierarchy violation: Cannot acquire %s (level %d) while holding %s (level %d)\n"+
                "Current stack:\n%s\n"+
                "Previous acquisition:\n%s",
                name, level, hold.Name, hold.Level, stack, hold.Stack,
            ))
        }
    }

    // Rule 2: Cannot re-acquire same lock (re-entrancy)
    for _, hold := range chain.Locks {
        if hold.Name == name {
            panic(fmt.Sprintf(
                "Re-entrancy detected: Cannot re-acquire %s\n"+
                "Current stack:\n%s\n"+
                "Previous acquisition:\n%s",
                name, stack, hold.Stack,
            ))
        }
    }

    // Rule 3: Cannot acquire write lock while holding read lock
    for _, hold := range chain.Locks {
        if hold.Name == name && hold.Type == "read" && typ == "write" {
            panic(fmt.Sprintf(
                "Lock upgrade detected: Cannot upgrade %s from read to write\n"+
                "Current stack:\n%s\n"+
                "Previous acquisition:\n%s",
                name, stack, hold.Stack,
            ))
        }
    }

    // Record acquisition
    chain.Locks = append(chain.Locks, LockHold{
        Name:  name,
        Level: level,
        Type:  typ,
        Stack: stack,
    })
}

func (r *LockRegistry) release(gid uint64, name string) {
    r.mu.Lock()
    defer r.mu.Unlock()

    chain := r.goroutineLocks[gid]
    if chain == nil {
        return
    }

    // Remove last occurrence of lock
    for i := len(chain.Locks) - 1; i >= 0; i-- {
        if chain.Locks[i].Name == name {
            chain.Locks = append(chain.Locks[:i], chain.Locks[i+1:]...)
            break
        }
    }
}
```

**Usage in Supervisor:**

```go
type Supervisor struct {
    mu    *lockorder.HierarchicalMutex
    // ... other fields
}

func NewSupervisor(parentSupervisor *Supervisor, ...) *Supervisor {
    level := lockorder.LevelSupervisorParent
    if parentSupervisor != nil {
        level = lockorder.LevelSupervisorChild
    }

    return &Supervisor{
        mu: lockorder.NewHierarchicalMutex("supervisor", level),
        // ... other initialization
    }
}
```

**Effort Estimate:** 6-10 hours
- Core hierarchy system: 3 hours
- Integration in supervisor: 2 hours
- Testing and validation: 3 hours
- Documentation: 2 hours

**Tradeoffs:**
- **Pro:** Catches violations immediately (fail fast)
- **Pro:** Clear error messages with context
- **Pro:** Enforces architectural lock design
- **Con:** Runtime overhead (tracking per goroutine)
- **Con:** Requires defining hierarchy explicitly
- **Con:** Panics may be too aggressive (could make configurable)

---

## 4. Quick Wins (< 4 Hours Each)

### Quick Win 1: Enhanced Test Timeouts with Context

**Problem:** 5s test timeout wasn't enough, 120s was too long.

**Solution:** Use context with timeout and detect blocked goroutines early.

```go
// pkg/fsmv2/supervisor_test.go

func TestSupervisorWithTimeout(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // Start goroutine to detect early blocking
    blockChan := make(chan struct{})
    go func() {
        ticker := time.NewTicker(2 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                // Check if any goroutines are blocked
                profile := pprof.Lookup("goroutine")
                buf := new(bytes.Buffer)
                profile.WriteTo(buf, 2)

                if strings.Contains(buf.String(), "sync.(*RWMutex)") {
                    t.Logf("WARNING: Potential lock contention detected")
                    t.Logf("Goroutine profile:\n%s", buf.String())
                    close(blockChan)
                    return
                }
            }
        }
    }()

    // Run test
    Eventually(func() int {
        return sup.GetChildrenCount()
    }, 5*time.Second, 100*time.Millisecond).Should(Equal(0))

    select {
    case <-blockChan:
        t.Fatal("Deadlock detected early")
    default:
    }
}
```

**Effort:** 2-3 hours
**Impact:** Faster feedback on deadlocks

---

### Quick Win 2: Simple Lock Acquisition Logger

**Problem:** No visibility into lock operations during debugging.

**Solution:** Environment variable-controlled lock logging.

```go
// pkg/fsmv2/supervisor.go

func (s *Supervisor) lockDebug(op string) {
    if os.Getenv("DEBUG_LOCKS") != "1" {
        return
    }

    gid := getGoroutineID()
    _, file, line, _ := runtime.Caller(1)

    log.Printf("[LOCK] goroutine=%d op=%s location=%s:%d", gid, op, file, line)
}

func (s *Supervisor) Lock() {
    s.lockDebug("LOCK_ACQUIRE")
    s.mu.Lock()
    s.lockDebug("LOCK_ACQUIRED")
}

func (s *Supervisor) Unlock() {
    s.lockDebug("UNLOCK")
    s.mu.Unlock()
}
```

**Usage:**
```bash
DEBUG_LOCKS=1 go test -v ./pkg/fsmv2/...
```

**Effort:** 1-2 hours
**Impact:** Immediate debugging visibility

---

### Quick Win 3: Lifecycle Event Logging

**Problem:** Couldn't see what operations were happening during failures.

**Solution:** Structured logging for supervisor lifecycle events.

```go
// pkg/fsmv2/supervisor.go

func (s *Supervisor) logLifecycle(event string, attrs ...any) {
    if os.Getenv("LIFECYCLE_LOGGING") != "1" {
        return
    }

    allAttrs := []any{"component", "supervisor", "event", event}
    allAttrs = append(allAttrs, attrs...)

    slog.Info("lifecycle", allAttrs...)
}

func (s *Supervisor) processSignal(ctx context.Context, sig *SignalHandlerRegistration) error {
    s.logLifecycle("processSignal_start",
        "signal", sig.Signal,
        "children_count", len(s.children),
    )
    defer s.logLifecycle("processSignal_end")

    // ... existing logic
}
```

**Usage:**
```bash
LIFECYCLE_LOGGING=1 go test -v ./pkg/fsmv2/...
```

**Effort:** 2-3 hours
**Impact:** Clear operation flow visibility

---

## 5. Moonshots (Ambitious Ideas)

### Moonshot 1: Message-Passing Supervisor Architecture

**Vision:** Eliminate locks entirely by redesigning supervisor as a single goroutine with channel-based message passing.

**Architecture:**

```go
// Message-passing supervisor design
type Supervisor struct {
    // No locks!
    commands chan Command
    queries  chan Query
    state    *SupervisorState // Only accessed by supervisor goroutine
}

type Command interface {
    Execute(state *SupervisorState) error
}

type Query interface {
    Execute(state *SupervisorState) any
}

func (s *Supervisor) Run(ctx context.Context) {
    for {
        select {
        case cmd := <-s.commands:
            cmd.Execute(s.state)
        case query := <-s.queries:
            result := query.Execute(s.state)
            query.Reply(result)
        case <-ctx.Done():
            return
        }
    }
}

// External API becomes message sending
func (s *Supervisor) GetChildren() []Child {
    query := &GetChildrenQuery{
        reply: make(chan []Child),
    }
    s.queries <- query
    return <-query.reply
}
```

**Benefits:**
- **No deadlocks possible** (no locks!)
- **Serialized state access** (single goroutine)
- **Easy to reason about** (message flow)
- **Testable** (mock message channels)

**Challenges:**
- **Complete redesign** (not incremental)
- **API changes** (blocking calls → async)
- **Performance** (channel overhead)
- **Migration complexity** (all callers must change)

**Effort:** 2-3 weeks
**Impact:** Eliminates entire class of concurrency bugs

---

### Moonshot 2: Compile-Time Lock Safety with Rust-Inspired Types

**Vision:** Use Go's type system to enforce lock safety at compile time, catching errors before running code.

**Approach:**

```go
// Rust-inspired ownership types
type Unlocked[T any] struct {
    value T
    mu    sync.RWMutex
}

type LockedRead[T any] struct {
    value *T
    mu    *sync.RWMutex
}

type LockedWrite[T any] struct {
    value *T
    mu    *sync.RWMutex
}

// Lock returns write access
func (u *Unlocked[T]) Lock() *LockedWrite[T] {
    u.mu.Lock()
    return &LockedWrite[T]{value: &u.value, mu: &u.mu}
}

// RLock returns read access
func (u *Unlocked[T]) RLock() *LockedRead[T] {
    u.mu.RLock()
    return &LockedRead[T]{value: &u.value, mu: &u.mu}
}

// Unlock consumes the write lock
func (l *LockedWrite[T]) Unlock() {
    l.mu.Unlock()
    // l is now unusable (no way to access after unlock)
}

// RUnlock consumes the read lock
func (l *LockedRead[T]) Unlock() {
    l.mu.RUnlock()
}

// Access value through locked reference
func (l *LockedRead[T]) Value() *T {
    return l.value
}

func (l *LockedWrite[T]) Value() *T {
    return l.value
}

// Usage enforces lock lifecycle
func (s *Supervisor) GetChildren() []Child {
    locked := s.children.RLock()
    defer locked.Unlock()

    children := make([]Child, len(*locked.Value()))
    copy(children, *locked.Value())
    return children
}

// Cannot accidentally use value after unlock (compile error)
func (s *Supervisor) BadExample() {
    locked := s.children.RLock()
    locked.Unlock()

    // Compile error: cannot use locked after Unlock
    children := *locked.Value() // ERROR
}
```

**Benefits:**
- **Compile-time safety** (catch mistakes before running)
- **No runtime overhead** (types erased at compile time)
- **Clear ownership** (explicit lock lifetimes)
- **Prevents double-unlock** (type consumed by Unlock)

**Challenges:**
- **Type system complexity** (generics + careful design)
- **API ergonomics** (more verbose than plain locks)
- **Migration effort** (rewrite all lock usage)
- **Limited by Go's type system** (not as powerful as Rust's borrow checker)

**Effort:** 3-4 weeks
- Type design: 1 week
- Implementation: 1 week
- Migration: 1-2 weeks
- Testing: 1 week

**Impact:** Prevents entire classes of lock bugs at compile time

---

## 6. Recommendations and Priorities

### Immediate Actions (Start This Week)

**Priority 1: Quick Win #2 - Simple Lock Acquisition Logger**
- **Why:** Provides immediate debugging visibility with minimal effort
- **Effort:** 1-2 hours
- **Next Step:** Implement `lockDebug()` helper and add to all Lock/Unlock calls

**Priority 2: Quick Win #3 - Lifecycle Event Logging**
- **Why:** Makes operation flow visible, helps prevent future debugging sessions
- **Effort:** 2-3 hours
- **Next Step:** Add `logLifecycle()` to processSignal, reconcileChildren, Shutdown

**Priority 3: Quick Win #1 - Enhanced Test Timeouts**
- **Why:** Faster feedback on deadlocks (5s instead of 120s)
- **Effort:** 2-3 hours
- **Next Step:** Add goroutine monitoring to test helpers

---

### Short-Term Improvements (Next 2 Weeks)

**Priority 4: Proposal #1 - Lock Operation Tracer**
- **Why:** Comprehensive lock visibility for development and testing
- **Effort:** 4-6 hours
- **Next Step:** Implement TracedRWMutex wrapper, integrate in supervisor

**Priority 5: Proposal #4 - Lifecycle Logging Infrastructure**
- **Why:** Production-ready structured logging with correlation IDs
- **Effort:** 3-5 hours
- **Next Step:** Design logging schema, implement logger, integrate in supervisor

---

### Medium-Term Investments (Next Month)

**Priority 6: Proposal #2 - Deadlock Detector Goroutine**
- **Why:** Automatic deadlock detection and diagnosis
- **Effort:** 8-12 hours
- **Next Step:** Implement basic detector, integrate with TracedRWMutex

**Priority 7: Proposal #5 - Lock Hierarchy Enforcement**
- **Why:** Prevents architectural lock violations at runtime
- **Effort:** 6-10 hours
- **Next Step:** Define lock levels, implement HierarchicalMutex

---

### Long-Term Exploration (Next Quarter)

**Priority 8: Proposal #3 - Static Lock Analysis Tool**
- **Why:** Catch issues before running code (ultimate prevention)
- **Effort:** 12-20 hours
- **Next Step:** Prototype parser for simple re-entrancy detection

**Priority 9: Moonshot #1 - Message-Passing Supervisor**
- **Why:** Consider if lock issues become recurring problem
- **Effort:** 2-3 weeks
- **Next Step:** Create proof-of-concept for subset of supervisor API

---

### Decision Gates

**After Quick Wins (1 week):**
- Evaluate: Did logging catch the issue faster?
- Decide: Invest in full tracing infrastructure or static analysis?

**After Lock Tracer (2 weeks):**
- Evaluate: How often do we use it? Performance overhead acceptable?
- Decide: Expand to production monitoring or keep test-only?

**After Short-Term Improvements (1 month):**
- Evaluate: Frequency of lock-related issues
- Decide:
  - If rare → Stop here, quick wins sufficient
  - If occasional → Invest in static analysis
  - If recurring → Consider message-passing refactor

---

## 7. Summary Matrix

| Solution | Category | Effort | Impact | Risk | Priority |
|----------|----------|--------|--------|------|----------|
| Simple Lock Logger | Quick Win | 1-2h | Medium | Low | **Immediate** |
| Lifecycle Logging | Quick Win | 2-3h | High | Low | **Immediate** |
| Enhanced Timeouts | Quick Win | 2-3h | Medium | Low | **Immediate** |
| Lock Tracer | Detective | 4-6h | High | Low | Short-term |
| Lifecycle Infrastructure | Detective | 3-5h | High | Low | Short-term |
| Deadlock Detector | Detective | 8-12h | High | Medium | Medium-term |
| Lock Hierarchy | Preventive | 6-10h | High | Medium | Medium-term |
| Static Analysis | Preventive | 12-20h | Very High | Medium | Long-term |
| Message-Passing | Structural | 2-3 weeks | Very High | High | Long-term |
| Type-Safe Locks | Structural | 3-4 weeks | Very High | High | Long-term |

---

## 8. Conclusion

The FSMv2 deadlock debugging experience revealed critical gaps in lock visibility and re-entrancy detection. By applying multiple creativity techniques, we've identified solutions across four categories:

1. **Preventive:** Catch issues before they happen (static analysis, lock hierarchy)
2. **Detective:** Make issues visible when they occur (tracing, lifecycle logging)
3. **Forensic:** Debug issues after failure (profiling, post-mortem tools)
4. **Structural:** Eliminate issue classes entirely (message-passing, type safety)

**Recommended Path:**

1. **This week:** Implement 3 quick wins (< 8 hours total)
2. **Next 2 weeks:** Add lock tracer and lifecycle infrastructure
3. **Next month:** Evaluate effectiveness, decide on further investments
4. **Long-term:** Consider static analysis or architectural refactors if issues recur

The key insight from creativity analysis: **Lock debugging problems stem from invisibility, not inherent complexity.** Making lock state observable eliminates most debugging pain, and simple solutions (environment-variable logging) provide 80% of the benefit with 20% of the effort.

**Next Action:** Start with Quick Win #2 (Simple Lock Logger) - implement in next session, validate against current test suite, iterate based on feedback.
