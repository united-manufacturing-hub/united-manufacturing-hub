# Lock Design and Documentation Plan

## Executive Summary

This document analyzes all locks in the FSMv2 Supervisor package, identifies what each lock protects, documents observed acquisition patterns, and proposes lock order rules to prevent deadlocks.

**Status**: Phase 2 (Analysis Complete, Documentation Pending)

## Lock Inventory

### 1. Supervisor.mu (sync.RWMutex)

**Location**: `supervisor.go:177`

**What it protects**:
- `workers map[string]*WorkerContext` - Worker registry
- `expectedObservedTypes map[string]reflect.Type` - Type validation registry
- `expectedDesiredTypes map[string]reflect.Type` - Desired state type registry
- `children map[string]*Supervisor` - Child supervisor hierarchy
- `childDoneChans map[string]<-chan struct{}` - Child shutdown channels
- `stateMapping map[string]string` - Parent→child state mapping
- `globalVars map[string]any` - Global configuration variables
- `mappedParentState string` - State mapped from parent supervisor

**Why RWMutex**: Allows concurrent reads (e.g., ListWorkers, GetWorker) while ensuring exclusive writes (AddWorker, RemoveWorker). The workers map is read frequently but modified infrequently.

**Current usage patterns**:
- Read lock: `GetWorker()`, `ListWorkers()`, `GetChildren()`, `tickWorker()` (lookup only)
- Write lock: `AddWorker()`, `RemoveWorker()`, `reconcileChildren()`, `SetGlobalVariables()`

### 2. Supervisor.ctxMu (sync.RWMutex)

**Location**: `supervisor.go:201`

**What it protects**:
- `ctx context.Context` - Supervisor lifecycle context
- `ctxCancel context.CancelFunc` - Context cancellation function

**Why separate lock**: Prevents TOCTOU (Time-Of-Check-Time-Of-Use) races between `isStarted()` and `getContext()` calls. Context access needs to be atomic with started state checks.

**Why RWMutex**: Context is read frequently (every goroutine checks it) but written only twice (Start, Shutdown).

**Current usage patterns**:
- Read lock: `getContext()`, `getStartedContext()`
- Write lock: `Start()`, `Shutdown()`

**Independence**: Can be acquired without `Supervisor.mu`. This is intentional - context operations don't need worker registry state.

### 3. WorkerContext.mu (sync.RWMutex)

**Location**: `supervisor.go:212`

**What it protects**:
- `currentState fsmv2.State` - Current FSM state for this worker

**Why per-worker lock**: Each WorkerContext has its own lock to allow parallel state transitions for different workers. This enables true multi-worker concurrency.

**Why RWMutex**: State is read very frequently (every tick) but written only during state transitions.

**Current usage patterns**:
- Read lock: `GetWorkerState()`, `GetCurrentState()`, state checks in `tickWorker()`
- Write lock: State transitions in `tickWorker()` (line 1048-1056)

**Independence**: These locks are completely independent per worker instance. Worker A's lock doesn't interact with Worker B's lock.

### 4. collection.Collector.mu (sync.RWMutex)

**Location**: `collection/collector.go:52`

**What it protects**:
- `state collectorState` - Collector lifecycle state (created/running/stopped)
- `running bool` - Whether observation loop is active
- `ctx context.Context` - Collector context
- `cancel context.CancelFunc` - Cancellation function
- `goroutineDone chan struct{}` - Shutdown signal channel

**Why RWMutex**: State is checked frequently (IsRunning) but modified only during lifecycle changes (Start, Stop, Restart).

**Current usage patterns**:
- Read lock: `IsRunning()`, `Restart()` (state check), `observationLoop()` (context access)
- Write lock: `Start()`, `Stop()`, `observationLoop()` (defer cleanup)

### 5. execution.ActionExecutor.mu (sync.RWMutex)

**Location**: `execution/action_executor.go:33`

**What it protects**:
- `inProgress map[string]bool` - Set of actions currently executing
- `actionQueue chan actionWork` - For queue size checks

**Why RWMutex**: The `inProgress` map is checked very frequently (every tick asks "is action in progress?") but modified only when actions start/complete.

**Current usage patterns**:
- Read lock: `HasActionInProgress()`, `metricsReporter()` (queue size check)
- Write lock: `EnqueueAction()` (adding to inProgress), `worker()` (removing from inProgress)

## Observed Lock Acquisition Patterns

### Pattern 1: Supervisor.mu → WorkerContext.mu

**Where**: `tickWorker()` lines 828-1077

```go
s.mu.RLock()
workerCtx, exists := s.workers[workerID]
s.mu.RUnlock()

// ... later ...

workerCtx.mu.Lock()
workerCtx.currentState = nextState
workerCtx.mu.Unlock()
```

**Purpose**: Lookup worker from registry, then modify its state. This is the most common pattern.

**Critical**: Never acquire in reverse order (would cause deadlock).

### Pattern 2: Supervisor.mu held while calling child methods

**Where**: `reconcileChildren()` lines 1573-1707, `processSignal()` lines 1376-1455

```go
s.mu.Lock()
defer s.mu.Unlock()

// ... code that modifies children map ...

child := s.children[spec.Name]
child.Shutdown()  // DEADLOCK RISK: child might acquire its own mu
```

**Risk**: If child.Shutdown() tries to read parent state (via parent.GetChildren()), it would need parent's mu, but parent already holds it. This creates a circular dependency.

**Mitigation**: Current code extracts children into a local map and releases lock before calling child methods (see lines 1783-1791, 1420-1436).

### Pattern 3: Supervisor.ctxMu independent acquisition

**Where**: `getStartedContext()` lines 1975-1983, `Start()` lines 764-766

```go
s.ctxMu.RLock()
defer s.ctxMu.RUnlock()

if !s.started.Load() {
    return nil, false
}
return s.ctx, true
```

**Purpose**: Check context without touching worker registry. This is intentionally independent.

## Proposed Lock Order Rules

### Rule 1: Supervisor.mu → WorkerContext.mu (MANDATORY)

**Rationale**: When you need both locks, always acquire Supervisor.mu first to lookup the worker, then acquire WorkerContext.mu to modify its state.

**Violation risk**: HIGH - Would cause immediate deadlock if two goroutines try to acquire these locks in different orders.

**Example violations to prevent**:
```go
// WRONG (deadlock risk):
workerCtx.mu.Lock()
s.mu.RLock()  // Another goroutine might have s.mu and be waiting for workerCtx.mu

// CORRECT:
s.mu.RLock()
workerCtx := s.workers[id]
s.mu.RUnlock()
workerCtx.mu.Lock()
```

### Rule 2: Supervisor.mu → Supervisor.ctxMu (ADVISORY)

**Rationale**: If you need both locks, acquire mu first. However, ctxMu can often be acquired alone (getStartedContext).

**Violation risk**: LOW - These locks are rarely needed together. ctxMu is mostly independent.

**When you need both**: During Shutdown() when cleaning up workers and canceling context.

### Rule 3: Never hold Supervisor.mu while calling child/worker methods

**Rationale**: Child supervisors may need to access their own state, and workers may trigger callbacks that need locks. Holding parent lock creates circular dependencies.

**Violation risk**: HIGH - Common source of deadlocks in hierarchical systems.

**Correct pattern**:
```go
// Extract data under lock
s.mu.Lock()
childrenToCleanup := make(map[string]*Supervisor)
for name, child := range s.children {
    childrenToCleanup[name] = child
}
s.mu.Unlock()

// Call methods outside lock
for name, child := range childrenToCleanup {
    child.Shutdown()  // Safe - we don't hold s.mu
}
```

### Rule 4: WorkerContext.mu locks are independent

**Rationale**: Each worker has its own lock. There's no ordering requirement between different workers' locks.

**Implication**: Multiple workers can be ticked in parallel without coordination (as long as each tick respects Rule 1).

## Deadlock Risk Analysis

### Risk 1: Parent-Child Lock Inversion (MITIGATED)

**Scenario**:
1. Parent holds Supervisor.mu
2. Parent calls child.Shutdown()
3. Child's Shutdown() tries to read parent state
4. Child needs parent's Supervisor.mu → DEADLOCK

**Current mitigation**: Code extracts children into local map and releases lock before calling child methods (lines 1783-1791, 1420-1436).

**Status**: MITIGATED (but not enforced by compiler)

### Risk 2: Supervisor.mu → WorkerContext.mu Inversion (HIGH RISK)

**Scenario**:
1. Goroutine A: s.mu.RLock() → workerCtx.mu.Lock()
2. Goroutine B: workerCtx.mu.Lock() → s.mu.RLock()
3. DEADLOCK

**Current mitigation**: None - relies on code review

**Proposed mitigation**: Runtime assertions in Phase 2

### Risk 3: Collector/Executor Lock Independence (NO RISK)

**Analysis**: collection.Collector.mu and execution.ActionExecutor.mu never interact with Supervisor locks. They're completely independent subsystems.

**Status**: SAFE (by design)

## Proposed Runtime Assertions

### Option 1: goroutine-local lock tracking

```go
type lockTracker struct {
    heldLocks []string // Stack of currently held locks
}

func (lt *lockTracker) acquire(lockName string) {
    // Check if we're violating lock order
    for _, held := range lt.heldLocks {
        if !isValidOrder(held, lockName) {
            panic(fmt.Sprintf("Lock order violation: holding %s, trying to acquire %s", held, lockName))
        }
    }
    lt.heldLocks = append(lt.heldLocks, lockName)
}

func (lt *lockTracker) release(lockName string) {
    // Pop from stack
    lt.heldLocks = lt.heldLocks[:len(lt.heldLocks)-1]
}
```

**Pros**: Catches violations immediately
**Cons**: Requires instrumenting every Lock() call
**Performance**: Minimal (goroutine-local, no synchronization)

### Option 2: lock rank system (Linux kernel style)

```go
const (
    lockRankSupervisorMu = 1
    lockRankWorkerContextMu = 2
    lockRankCtxMu = 0 // Can be acquired alone
)

func (s *Supervisor) acquireWithRank(mu *sync.RWMutex, rank int, write bool) {
    currentRank := getCurrentGoroutineLockRank()
    if rank != 0 && rank <= currentRank {
        panic(fmt.Sprintf("Lock rank violation: current=%d, attempting=%d", currentRank, rank))
    }
    if write {
        mu.Lock()
    } else {
        mu.RLock()
    }
    setCurrentGoroutineLockRank(rank)
}
```

**Pros**: Enforces total lock ordering
**Cons**: More invasive, harder to maintain
**Performance**: Slightly higher overhead

### Recommendation: Option 1 (goroutine-local tracking)

Start with goroutine-local lock tracking in debug builds. This catches violations without changing production performance.

## Implementation Plan

### Phase 2.1: Add Godoc Comments (THIS DOCUMENT)

**Tasks**:
- [ ] Document Supervisor.mu: what it protects, why RWMutex
- [ ] Document Supervisor.ctxMu: what it protects, TOCTOU prevention
- [ ] Document WorkerContext.mu: per-worker independence
- [ ] Add package-level LOCK ORDER comment
- [ ] Document lock acquisition rules

**Acceptance**: All tests in `lock_documentation_test.go` pass

### Phase 2.2: Add Runtime Assertions (NEXT)

**Tasks**:
- [ ] Implement goroutine-local lock tracker
- [ ] Add assertions to Supervisor.mu Lock/RLock calls
- [ ] Add assertions to WorkerContext.mu Lock/RLock calls
- [ ] Add tests that verify assertions fire on violations

**Acceptance**: Tests can trigger and verify lock order violation panics

### Phase 2.3: Document Deadlock Scenarios (FUTURE)

**Tasks**:
- [ ] Document common deadlock patterns in package comments
- [ ] Add examples of correct lock patterns
- [ ] Add runbook for diagnosing deadlocks in production

## Testing Strategy

### Test 1: Documentation Existence

Verify godoc comments exist for all locks by parsing supervisor.go AST and checking field documentation.

### Test 2: Documentation Content

Verify comments mention:
- What each lock protects (specific fields)
- Why RWMutex is used (concurrent reads)
- Lock ordering rules

### Test 3: Lock Order Violations

Once runtime assertions are added, verify they fire:
- Test violating Supervisor.mu → WorkerContext.mu order
- Test holding Supervisor.mu while calling child methods
- Verify panic message includes lock names

## References

- **supervisor.go**: Main lock implementations
- **collection/collector.go**: Collector subsystem locks
- **execution/action_executor.go**: Action executor locks
- **lifecycle_logging_test.go**: Example of testing with lock logging

## Revision History

- 2025-01-13: Initial analysis (Phase 2 TDD)
