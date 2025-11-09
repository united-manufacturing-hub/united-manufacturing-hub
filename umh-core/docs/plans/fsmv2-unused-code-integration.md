# FSMv2 Unused Code Integration Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/executing-plans/SKILL.md` to implement this plan task-by-task.

**Goal:** Integrate currently unused ActionExecutor infrastructure and metrics functions into the FSMv2 supervisor lifecycle.

**Architecture:** This plan extends `fsmv2-package-restructuring.md` by completing two partially-implemented subsystems: (1) ActionExecutor async action execution per worker, and (2) metrics instrumentation for action lifecycle and supervisor operations.

**Tech Stack:** Go 1.23+, Prometheus client, FSMv2 supervisor pattern, Ginkgo v2 testing

**Created:** 2025-11-08 14:30

**Last Updated:** 2025-11-08 14:30

---

## Overview

This plan addresses two categories of unused/incomplete code in FSMv2:

1. **ActionExecutor Infrastructure** - Currently exists as stub only (enqueues stub actions, but no real action routing or execution)
2. **Unused Metrics Functions** - 10 metric recording functions defined but never called in production code

**Investigation Findings:**

From `docs/design/fsmv2-async-action-executor.md`:
- ActionExecutor design is complete and documented
- Infrastructure partially exists (ActionExecutor struct, size-1 channel, retry logic)
- **Missing:** Per-worker executor instances, action blocking checks, real action routing
- Current supervisor uses old `executeActionWithRetry()` method instead

From `pkg/fsmv2/supervisor/metrics/metrics.go` analysis:
- 18 metrics functions defined, only 8 currently used
- 10 unused functions: `RecordActionQueued`, `RecordActionQueueSize`, `RecordActionExecutionDuration`, `RecordActionTimeout`, `RecordWorkerPoolUtilization`, `RecordWorkerPoolQueueSize`, `RecordChildCount`, `RecordTickPropagationDepth`, `RecordTickPropagationDuration`
- All have tests (metrics_test.go) but no production callers

**Architecture Alignment:**

Both integrations fit the existing FSMv2 architecture:
- ActionExecutor mirrors Collector pattern (per-worker goroutine, async execution)
- Metrics instrument supervisor lifecycle (circuit breaker, reconciliation, template rendering)

---

## Part 1: ActionExecutor Full Integration

### Current State

**What exists:**
- `pkg/fsmv2/supervisor/execution/` package structure
- ActionExecutor struct with size-1 channel (enforces 1 action per worker)
- `EnqueueAction()`, `HasActionInProgress()`, `GetActionStatus()` methods
- Retry logic with exponential backoff
- Timeout handling with context cancellation
- Complete design document: `docs/design/fsmv2-async-action-executor.md`

**What's missing:**
- WorkerContext doesn't have `executor *execution.ActionExecutor` field
- AddWorker() doesn't initialize per-worker executors
- tickWorker() doesn't check if action is in progress before calling state.Next()
- tickWorker() still uses old `executeActionWithRetry()` method instead of EnqueueAction()
- No cleanup in RemoveWorker() or Stop()

**Current flow (synchronous):**
```
tickWorker() → state.Next() → returns action → executeActionWithRetry() → BLOCKS TICK
```

**Target flow (asynchronous):**
```
tickWorker() → check HasActionInProgress() → if blocked, skip state.Next()
            → state.Next() → returns action → EnqueueAction() → continues ticking
ActionExecutor goroutine → dequeues action → executeWithRetry() → updates status
Next tick → state observes ActionStatus → transitions based on result
```

### Target Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Supervisor (supervisor.go)                                  │
├─────────────────────────────────────────────────────────────┤
│  workers map[string]*WorkerContext                          │
│    ├─ identity:     fsmv2.Identity                          │
│    ├─ worker:       fsmv2.Worker                            │
│    ├─ currentState: fsmv2.State                             │
│    ├─ collector:    *Collector      [EXISTING]             │
│    └─ executor:     *ActionExecutor [NEW]                   │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ ActionExecutor (execution/action_executor.go)               │
├─────────────────────────────────────────────────────────────┤
│  actionQueue:   chan fsmv2.Action (size 1)                  │
│  currentAction: atomic.Value                                 │
│  currentStatus: atomic.Value (*ActionStatus)                │
│  goroutine:     actionLoop() running in background          │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ Action (fsmv2.Action interface)                             │
├─────────────────────────────────────────────────────────────┤
│  Execute(ctx context.Context) error                         │
│  Name() string                                               │
└─────────────────────────────────────────────────────────────┘
```

### Phase 1: WorkerContext Modifications

**File:** `pkg/fsmv2/supervisor/supervisor.go`
**Lines:** 198-211 (WorkerContext struct definition)

**Changes:**
- Add `executor *execution.ActionExecutor` field to WorkerContext
- Add import for `github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/execution`

**Before:**
```go
type WorkerContext struct {
    mu             sync.RWMutex
    tickInProgress atomic.Bool
    identity       fsmv2.Identity
    worker         fsmv2.Worker
    currentState   fsmv2.State
    collector      *collection.Collector
    // executor is missing
}
```

**After:**
```go
type WorkerContext struct {
    mu             sync.RWMutex
    tickInProgress atomic.Bool
    identity       fsmv2.Identity
    worker         fsmv2.Worker
    currentState   fsmv2.State
    collector      *collection.Collector
    executor       *execution.ActionExecutor  // NEW: Per-worker action executor
}
```

**Test Requirements:**
- [ ] Verify WorkerContext can be created with executor field
- [ ] Verify field is accessible after creation
- [ ] No breaking changes to existing worker context tests

**Breaking Changes:** None (additive only)

**Migration Notes:** Existing code that creates WorkerContext will need to initialize executor field (handled in Phase 2).

---

### Phase 2: Initialize Per-Worker Executors

**File:** `pkg/fsmv2/supervisor/supervisor.go`
**Lines:** ~450-480 (AddWorker function)

**Changes:**
- Create ActionExecutor instance for each worker
- Pass supervisor logger and workerID to executor config
- Set default timeout and retry values

**Before:**
```go
func (s *Supervisor) AddWorker(identity fsmv2.Identity, worker fsmv2.Worker) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // ... existing validation ...

    collector := collection.NewCollector(collection.CollectorConfig{
        WorkerID: identity.ID,
        Worker:   worker,
        Logger:   s.logger,
        Storage:  s.storage,
    })

    s.workers[identity.ID] = &WorkerContext{
        identity:     identity,
        worker:       worker,
        currentState: worker.GetInitialState(),
        collector:    collector,
        // executor missing
    }

    return nil
}
```

**After:**
```go
func (s *Supervisor) AddWorker(identity fsmv2.Identity, worker fsmv2.Worker) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // ... existing validation ...

    collector := collection.NewCollector(collection.CollectorConfig{
        WorkerID: identity.ID,
        Worker:   worker,
        Logger:   s.logger,
        Storage:  s.storage,
    })

    // NEW: Create per-worker action executor
    executor := execution.NewActionExecutor(execution.ActionExecutorConfig{
        WorkerID:      identity.ID,
        Logger:        s.logger,
        ActionTimeout: 5 * time.Minute,  // Default from design doc
        MaxRetries:    3,                 // Default from design doc
    })

    s.workers[identity.ID] = &WorkerContext{
        identity:     identity,
        worker:       worker,
        currentState: worker.GetInitialState(),
        collector:    collector,
        executor:     executor,  // NEW: Attach executor to worker context
    }

    return nil
}
```

**Test Requirements:**
- [ ] Verify executor is created for each worker
- [ ] Verify executor has correct workerID
- [ ] Verify executor is not nil after AddWorker()
- [ ] Verify multiple workers get separate executor instances

**Breaking Changes:** None

**Migration Notes:** Existing workers will automatically get executors on next AddWorker() call.

---

### Phase 3: Start Executors During Supervisor Start

**File:** `pkg/fsmv2/supervisor/supervisor.go`
**Lines:** ~520-570 (Start function)

**Changes:**
- Start executor goroutine for each worker after starting collectors
- Pass supervisor context to executor
- Log any executor start failures

**Before:**
```go
func (s *Supervisor) Start(ctx context.Context) <-chan struct{} {
    s.mu.Lock()
    defer s.mu.Unlock()

    // ... existing collector start logic ...
    for workerID, workerCtx := range s.workers {
        if err := workerCtx.collector.Start(ctx); err != nil {
            s.logger.Errorf("Failed to start collector for worker %s: %v", workerID, err)
        }
    }

    // Executor start missing

    // ... existing tick loop ...
}
```

**After:**
```go
func (s *Supervisor) Start(ctx context.Context) <-chan struct{} {
    s.mu.Lock()
    defer s.mu.Unlock()

    // ... existing collector start logic ...
    for workerID, workerCtx := range s.workers {
        if err := workerCtx.collector.Start(ctx); err != nil {
            s.logger.Errorf("Failed to start collector for worker %s: %v", workerID, err)
        }

        // NEW: Start executor goroutine for each worker
        if err := workerCtx.executor.Start(ctx); err != nil {
            s.logger.Errorf("Failed to start executor for worker %s: %v", workerID, err)
        }
    }

    // ... existing tick loop ...
}
```

**Test Requirements:**
- [ ] Verify executor.Start() is called for all workers
- [ ] Verify executor goroutines are running
- [ ] Verify executor stops when context is cancelled
- [ ] Integration test: tick loop + executor both running

**Breaking Changes:** None

**Migration Notes:** Executors will start automatically when supervisor starts.

---

### Phase 4: Add Action Blocking Check

**File:** `pkg/fsmv2/supervisor/supervisor.go`
**Lines:** ~650-720 (tickWorker function)

**Changes:**
- Check if executor has action in progress BEFORE calling state.Next()
- Skip state transition if action is blocking
- Log which action is blocking

**Before:**
```go
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... load snapshot ...

    // ... freshness check ...

    // Missing: Check if action in progress

    // Call state.Next() without checking if worker is blocked
    nextState, signal, action := currentState.Next(*snapshot)

    // ... rest of tick logic ...
}
```

**After:**
```go
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... load snapshot ...

    // ... freshness check ...

    // NEW: Check if action in progress - if so, skip state.Next()
    workerCtx.mu.RLock()
    actionInProgress := workerCtx.executor.HasActionInProgress()
    workerCtx.mu.RUnlock()

    if actionInProgress {
        s.logger.Debugf("Worker %s blocked by action: %s",
            workerID, workerCtx.executor.CurrentActionName())
        return nil  // Skip this tick, wait for action to complete
    }

    // Only call state.Next() if no action is blocking
    nextState, signal, action := currentState.Next(*snapshot)

    // ... rest of tick logic ...
}
```

**Test Requirements:**
- [ ] Verify state.Next() is NOT called when action in progress
- [ ] Verify state.Next() IS called when no action in progress
- [ ] Verify debug log appears when worker is blocked
- [ ] Integration test: Long-running action blocks multiple ticks

**Breaking Changes:** None (behavioral change but compatible)

**Migration Notes:** Workers will automatically be blocked during action execution.

---

### Phase 5: Route Actions to Executor

**File:** `pkg/fsmv2/supervisor/supervisor.go`
**Lines:** ~650-720 (tickWorker function, after state.Next() call)

**Changes:**
- Replace `executeActionWithRetry()` call with `EnqueueAction()`
- Handle queue full error (action already in progress)
- Remove blocking execution

**Before:**
```go
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... snapshot, freshness check, blocking check ...

    nextState, signal, action := currentState.Next(*snapshot)

    // OLD: Synchronous execution (blocks tick)
    if action != nil {
        if err := s.executeActionWithRetry(ctx, action); err != nil {
            s.logger.Errorf("Action failed: %v", err)
        }
    }

    // ... state transition, signal processing ...
}
```

**After:**
```go
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... snapshot, freshness check, blocking check ...

    nextState, signal, action := currentState.Next(*snapshot)

    // NEW: Asynchronous execution (enqueue and continue)
    if action != nil {
        if err := workerCtx.executor.EnqueueAction(action); err != nil {
            // Queue full means action already in progress (shouldn't happen due to Phase 4 check)
            s.logger.Warnf("Failed to enqueue action for worker %s: %v", workerID, err)
        }
    }

    // ... state transition, signal processing ...
}
```

**Test Requirements:**
- [ ] Verify action is enqueued (not executed inline)
- [ ] Verify tick loop continues immediately after enqueue
- [ ] Verify action executes in background
- [ ] Verify error handling when queue is full
- [ ] Integration test: Action completes while tick loop continues

**Breaking Changes:** Behavioral change - actions are now async

**Migration Notes:** States that emit actions will see actions complete in future ticks (not same tick).

---

### Phase 6: Remove Old Execution Method

**File:** `pkg/fsmv2/supervisor/supervisor.go`
**Lines:** ~820-880 (executeActionWithRetry function)

**Changes:**
- Delete `executeActionWithRetry()` method entirely
- Remove any remaining references to synchronous execution
- Verify no other callers exist

**Before:**
```go
func (s *Supervisor) executeActionWithRetry(ctx context.Context, action fsmv2.Action) error {
    maxRetries := 3
    for attempt := 0; attempt <= maxRetries; attempt++ {
        // ... retry logic with exponential backoff ...
    }
    return nil
}
```

**After:**
```go
// Method deleted - functionality moved to ActionExecutor
```

**Test Requirements:**
- [ ] Verify no compilation errors after deletion
- [ ] Verify no test failures
- [ ] Grep codebase for any remaining references

**Breaking Changes:** Internal only (method was private)

**Migration Notes:** None needed (internal implementation change).

---

### Phase 7: Add Executor Cleanup

**File:** `pkg/fsmv2/supervisor/supervisor.go`
**Lines:** Multiple locations (RemoveWorker, Stop functions)

**Changes:**
- Stop executor when removing worker
- Stop all executors when supervisor stops
- Wait for executor goroutines to complete

**Location 1: RemoveWorker()**

**Before:**
```go
func (s *Supervisor) RemoveWorker(workerID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    workerCtx := s.workers[workerID]

    // Stop collector
    workerCtx.collector.Stop(context.Background())

    // Executor stop missing

    delete(s.workers, workerID)
    return nil
}
```

**After:**
```go
func (s *Supervisor) RemoveWorker(workerID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    workerCtx := s.workers[workerID]

    // Stop collector
    workerCtx.collector.Stop(context.Background())

    // NEW: Stop executor (give 5s to complete current action)
    stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    workerCtx.executor.Stop(stopCtx)

    delete(s.workers, workerID)
    return nil
}
```

**Location 2: Stop()**

**Before:**
```go
func (s *Supervisor) Stop(ctx context.Context) {
    // ... cancel supervisor context ...

    // Stop collectors
    for _, workerCtx := range s.workers {
        workerCtx.collector.Stop(ctx)
    }

    // Executor stop missing
}
```

**After:**
```go
func (s *Supervisor) Stop(ctx context.Context) {
    // ... cancel supervisor context ...

    // Stop collectors and executors
    for workerID, workerCtx := range s.workers {
        workerCtx.collector.Stop(ctx)

        // NEW: Stop executor (context deadline enforced by caller)
        workerCtx.executor.Stop(ctx)
    }
}
```

**Test Requirements:**
- [ ] Verify executor stops when RemoveWorker() is called
- [ ] Verify executor stops when supervisor stops
- [ ] Verify executor goroutine exits cleanly
- [ ] Verify timeout is respected (doesn't hang forever)
- [ ] Integration test: In-flight action is cancelled on stop

**Breaking Changes:** None

**Migration Notes:** Executors will clean up automatically on worker removal.

---

### Phase 8: Integration Testing

**File:** `pkg/fsmv2/supervisor/supervisor_integration_test.go` (new file)

**Test Scenarios:**

1. **Action Execution Flow:**
   - Worker emits action via state.Next()
   - Action is enqueued successfully
   - Action executes in background
   - Next tick observes action completed

2. **Worker Blocking:**
   - Long-running action in progress
   - Multiple ticks occur
   - state.Next() is NOT called until action completes
   - Worker unblocks after action finishes

3. **Action Retry:**
   - Action fails first attempt
   - Executor retries with backoff
   - Action succeeds on second attempt
   - ActionStatus reflects retry count

4. **Action Timeout:**
   - Action exceeds timeout (5 minutes)
   - Executor marks as failed
   - State observes timeout in ActionStatus
   - Worker can decide to retry or escalate

5. **Concurrent Workers:**
   - 3 workers, each with separate executor
   - Worker 1 action blocks only worker 1
   - Workers 2 and 3 continue ticking normally
   - All actions complete successfully

6. **Cleanup:**
   - Supervisor stops during action execution
   - Executor goroutines exit cleanly
   - No goroutine leaks
   - Context cancellation propagates

**Test Implementation:**

```go
package supervisor_test

import (
    "context"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("ActionExecutor Integration", func() {
    var (
        supervisor *supervisor.Supervisor
        ctx        context.Context
        cancel     context.CancelFunc
    )

    BeforeEach(func() {
        ctx, cancel = context.WithCancel(context.Background())
        supervisor = setupTestSupervisor()
    })

    AfterEach(func() {
        cancel()
        supervisor.Stop(ctx)
    })

    Context("Action Execution Flow", func() {
        It("should execute action asynchronously", func() {
            // Add worker that emits action
            worker := &MockWorker{
                initialState: &StateEmitsAction{},
            }
            supervisor.AddWorker(testIdentity, worker)

            // Start supervisor
            supervisor.Start(ctx)

            // Wait for tick
            time.Sleep(100 * time.Millisecond)

            // Verify action was enqueued
            workerCtx := supervisor.GetWorker(testIdentity.ID)
            Eventually(workerCtx.executor.HasActionInProgress).Should(BeTrue())

            // Wait for action to complete
            Eventually(workerCtx.executor.HasActionInProgress).Should(BeFalse())

            // Verify action succeeded
            status := workerCtx.executor.GetActionStatus()
            Expect(status.Succeeded).To(BeTrue())
        })
    })

    Context("Worker Blocking", func() {
        It("should block state.Next() during action execution", func() {
            // Add worker with long-running action
            callCount := 0
            worker := &MockWorker{
                initialState: &StateEmitsSlowAction{
                    duration: 2 * time.Second,
                },
                onNextCalled: func() { callCount++ },
            }
            supervisor.AddWorker(testIdentity, worker)

            // Start supervisor
            supervisor.Start(ctx)

            // Wait for action to start
            time.Sleep(100 * time.Millisecond)

            // Record initial call count
            initialCalls := callCount

            // Wait 1 second (multiple ticks should occur)
            time.Sleep(1 * time.Second)

            // Verify state.Next() was NOT called again
            Expect(callCount).To(Equal(initialCalls))

            // Wait for action to complete
            time.Sleep(1500 * time.Millisecond)

            // Verify state.Next() is now called again
            Eventually(func() int { return callCount }).Should(BeNumerically(">", initialCalls))
        })
    })

    // ... remaining test scenarios ...
})
```

**Test Requirements:**
- [ ] All 6 scenarios pass
- [ ] No goroutine leaks detected
- [ ] No race conditions detected
- [ ] Coverage > 80% for ActionExecutor integration

---

## Part 2: Metrics System Integration

### Unused Metrics Analysis

**Currently Used Metrics (8):**
1. `RecordCircuitOpen` - Called in supervisor.go when circuit breaker opens/closes
2. `RecordInfrastructureRecovery` - Called when infrastructure recovers
3. `RecordChildHealthCheck` - (Planned for supervisor composition patterns)
4. `RecordReconciliation` - Called at end of each reconciliation cycle
5. `RecordVariablePropagation` - Called when variables propagate to children
6. `RecordTemplateRenderingDuration` - Called after template rendering
7. `RecordTemplateRenderingError` - Called when template rendering fails

**Unused Metrics (10):**
1. `RecordActionQueued` - When action is enqueued to executor
2. `RecordActionQueueSize` - Current size of action queue
3. `RecordActionExecutionDuration` - How long action took to execute
4. `RecordActionTimeout` - When action exceeds timeout
5. `RecordWorkerPoolUtilization` - (Future: worker pool pattern)
6. `RecordWorkerPoolQueueSize` - (Future: worker pool pattern)
7. `RecordChildCount` - Number of child supervisors
8. `RecordTickPropagationDepth` - Depth of tick in supervisor hierarchy
9. `RecordTickPropagationDuration` - How long tick propagation took

**All metrics documented in:** `pkg/fsmv2/supervisor/METRICS.md`

### Integration Strategy

#### High Priority Metrics (Integrate with ActionExecutor)

These metrics are critical for observing ActionExecutor behavior:

**1. RecordActionQueued**
- **Purpose:** Track action queue activity per worker type
- **Where:** `pkg/fsmv2/supervisor/supervisor.go`, tickWorker() after EnqueueAction()
- **When:** After action successfully enqueued
- **Labels:** `supervisor_id` (worker type), `action_type` (action name)

**Integration Point:**
```go
// In tickWorker(), after Phase 5 changes
if action != nil {
    if err := workerCtx.executor.EnqueueAction(action); err != nil {
        s.logger.Warnf("Failed to enqueue action: %v", err)
    } else {
        // NEW: Record action queued
        metrics.RecordActionQueued(s.workerType, action.Name())
    }
}
```

**2. RecordActionQueueSize**
- **Purpose:** Monitor queue capacity (should always be 0 or 1 due to size-1 channel)
- **Where:** `pkg/fsmv2/supervisor/execution/action_executor.go`, EnqueueAction() and actionLoop()
- **When:** After enqueue and after dequeue
- **Labels:** `supervisor_id`

**Integration Point:**
```go
// In ActionExecutor.EnqueueAction()
func (e *ActionExecutor) EnqueueAction(action fsmv2.Action) error {
    select {
    case e.actionQueue <- action:
        e.config.Logger.Infof("Enqueued action: %s", action.Name())
        // NEW: Record queue size increased
        metrics.RecordActionQueueSize(e.config.WorkerID, 1)
        return nil
    default:
        return fmt.Errorf("action queue full")
    }
}

// In ActionExecutor.actionLoop()
case action := <-e.actionQueue:
    // NEW: Record queue size decreased
    metrics.RecordActionQueueSize(e.config.WorkerID, 0)
    e.currentAction.Store(&action)
    e.executeWithRetry(action)
```

**3. RecordActionExecutionDuration**
- **Purpose:** Measure action execution time (P95, P99 latency)
- **Where:** `pkg/fsmv2/supervisor/execution/action_executor.go`, executeAction()
- **When:** After action completes (success, failure, or timeout)
- **Labels:** `supervisor_id`, `action_type`, `status` ("success", "failure", "timeout")

**Integration Point:**
```go
// In ActionExecutor.executeAction()
func (e *ActionExecutor) executeAction(ctx context.Context, action fsmv2.Action) error {
    startTime := time.Now()

    timeout := e.config.ActionTimeout
    if timeout == 0 {
        timeout = DefaultActionTimeout
    }

    actionCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    err := action.Execute(actionCtx)

    // NEW: Record execution duration
    duration := time.Since(startTime)
    status := "success"
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            status = "timeout"
        } else {
            status = "failure"
        }
    }
    metrics.RecordActionExecutionDuration(e.config.WorkerID, action.Name(), status, duration)

    return err
}
```

**4. RecordActionTimeout**
- **Purpose:** Track timeout frequency per action type
- **Where:** `pkg/fsmv2/supervisor/execution/action_executor.go`, executeAction()
- **When:** When action exceeds timeout
- **Labels:** `supervisor_id`, `action_type`

**Integration Point:**
```go
// In ActionExecutor.executeAction(), after timeout check
if errors.Is(err, context.DeadlineExceeded) {
    e.logger.Errorf("Action %s timed out after %v", action.Name(), timeout)
    // NEW: Record timeout
    metrics.RecordActionTimeout(e.config.WorkerID, action.Name())
    return fmt.Errorf("action timeout: %w", err)
}
```

#### Medium Priority Metrics (Integrate with Supervisor Lifecycle)

**5. RecordChildCount**
- **Purpose:** Monitor supervisor hierarchy size
- **Where:** `pkg/fsmv2/supervisor/supervisor.go`, AddChild() and RemoveChild()
- **When:** After child supervisor added/removed
- **Labels:** `supervisor_id`

**Integration Point:**
```go
// In Supervisor.AddChild() (if method exists)
func (s *Supervisor) AddChild(childID string, child *Supervisor) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.children[childID] = child

    // NEW: Record child count
    metrics.RecordChildCount(s.workerType, len(s.children))

    return nil
}

// In Supervisor.RemoveChild() (if method exists)
func (s *Supervisor) RemoveChild(childID string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    delete(s.children, childID)

    // NEW: Record child count
    metrics.RecordChildCount(s.workerType, len(s.children))

    return nil
}
```

**6. RecordTickPropagationDepth**
- **Purpose:** Monitor tick propagation in supervisor hierarchy
- **Where:** `pkg/fsmv2/supervisor/supervisor.go`, Tick() method
- **When:** During tick propagation to children
- **Labels:** `supervisor_id`

**Integration Point:**
```go
// In Supervisor.Tick() when propagating to children
func (s *Supervisor) Tick(ctx context.Context) error {
    // ... existing tick logic ...

    // If this supervisor has children, calculate depth
    if len(s.children) > 0 {
        depth := 1  // This supervisor is depth 1

        // Propagate tick to children
        for childID, child := range s.children {
            if err := child.Tick(ctx); err != nil {
                s.logger.Errorf("Child %s tick failed: %v", childID, err)
            }
            // Depth increases for each child level
            depth++
        }

        // NEW: Record max depth reached
        metrics.RecordTickPropagationDepth(s.workerType, depth)
    }

    return nil
}
```

**7. RecordTickPropagationDuration**
- **Purpose:** Measure how long tick propagation takes
- **Where:** `pkg/fsmv2/supervisor/supervisor.go`, Tick() method
- **When:** After tick completes (including children)
- **Labels:** `supervisor_id`

**Integration Point:**
```go
// In Supervisor.Tick()
func (s *Supervisor) Tick(ctx context.Context) error {
    tickStart := time.Now()

    // ... existing tick logic ...

    // Propagate to children (if any)
    for _, child := range s.children {
        if err := child.Tick(ctx); err != nil {
            s.logger.Errorf("Child tick failed: %v", err)
        }
    }

    // NEW: Record propagation duration
    duration := time.Since(tickStart)
    metrics.RecordTickPropagationDuration(s.workerType, duration)

    return nil
}
```

#### Low Priority Metrics (Future Implementation)

**8. RecordWorkerPoolUtilization**
- **Purpose:** Worker pool utilization (when worker pool pattern implemented)
- **Status:** Not needed until worker pool infrastructure exists
- **Decision:** Skip for now, implement when worker pool is added

**9. RecordWorkerPoolQueueSize**
- **Purpose:** Worker pool queue size (when worker pool pattern implemented)
- **Status:** Not needed until worker pool infrastructure exists
- **Decision:** Skip for now, implement when worker pool is added

---

### Metrics Integration Implementation

**Task 1: ActionExecutor Metrics**

**File:** `pkg/fsmv2/supervisor/execution/action_executor.go`

**Changes:**
1. Add metrics package import
2. Call `RecordActionQueueSize()` in EnqueueAction() and actionLoop()
3. Call `RecordActionExecutionDuration()` in executeAction()
4. Call `RecordActionTimeout()` when timeout occurs

**Code:**
```go
package execution

import (
    // ... existing imports ...
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
)

func (e *ActionExecutor) EnqueueAction(action fsmv2.Action) error {
    select {
    case e.actionQueue <- action:
        e.config.Logger.Infof("Enqueued action: %s", action.Name())
        metrics.RecordActionQueueSize(e.config.WorkerID, 1)  // Queue now has 1 item
        return nil
    default:
        return fmt.Errorf("action queue full, worker blocked by: %s",
            e.CurrentActionName())
    }
}

func (e *ActionExecutor) actionLoop() {
    defer close(e.goroutineDone)

    for {
        select {
        case <-e.ctx.Done():
            e.config.Logger.Info("ActionExecutor loop stopped")
            return

        case action := <-e.actionQueue:
            metrics.RecordActionQueueSize(e.config.WorkerID, 0)  // Queue now empty
            e.currentAction.Store(&action)
            e.executeWithRetry(action)
            e.currentAction.Store((*fsmv2.Action)(nil))
        }
    }
}

func (e *ActionExecutor) executeAction(ctx context.Context, action fsmv2.Action) error {
    startTime := time.Now()

    timeout := e.config.ActionTimeout
    if timeout == 0 {
        timeout = DefaultActionTimeout
    }

    actionCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    e.config.Logger.Infof("Executing action: %s (timeout=%v)", action.Name(), timeout)

    err := action.Execute(actionCtx)

    // Record execution duration with status
    duration := time.Since(startTime)
    status := "success"
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            status = "timeout"
            metrics.RecordActionTimeout(e.config.WorkerID, action.Name())
        } else {
            status = "failure"
        }
    }
    metrics.RecordActionExecutionDuration(e.config.WorkerID, action.Name(), status, duration)

    if errors.Is(err, context.DeadlineExceeded) {
        e.config.Logger.Errorf("Action %s timed out after %v", action.Name(), timeout)
        return fmt.Errorf("action timeout: %w", err)
    }

    return err
}
```

**Test Requirements:**
- [ ] Verify RecordActionQueueSize called with correct values
- [ ] Verify RecordActionExecutionDuration called after execution
- [ ] Verify RecordActionTimeout called only on timeout
- [ ] Verify metrics labels are correct (workerID, action name)

---

**Task 2: Supervisor Action Queued Metric**

**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Changes:**
- Call `RecordActionQueued()` in tickWorker() after successful EnqueueAction()

**Code:**
```go
// In tickWorker(), after Phase 5 integration
if action != nil {
    if err := workerCtx.executor.EnqueueAction(action); err != nil {
        s.logger.Warnf("Failed to enqueue action for worker %s: %v", workerID, err)
    } else {
        // NEW: Record action queued
        metrics.RecordActionQueued(s.workerType, action.Name())
    }
}
```

**Test Requirements:**
- [ ] Verify metric increments when action enqueued
- [ ] Verify metric does NOT increment when enqueue fails
- [ ] Verify correct labels (supervisor ID, action type)

---

**Task 3: Supervisor Hierarchy Metrics (Optional)**

**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Changes:**
- Add `RecordChildCount()` to AddChild() and RemoveChild() (if methods exist)
- Add `RecordTickPropagationDepth()` to Tick() (if children exist)
- Add `RecordTickPropagationDuration()` to Tick() (if children exist)

**Status:** Optional - only implement if supervisor composition patterns are active

**Test Requirements:**
- [ ] Skip if supervisor composition not implemented
- [ ] Otherwise, verify metrics track hierarchy correctly

---

### Performance Impact Considerations

**Metrics overhead:**
- Prometheus metrics are highly optimized (atomic operations, lock-free counters)
- Histogram observations: ~200ns per call
- Counter increments: ~50ns per call
- Gauge sets: ~50ns per call

**Expected impact:**
- Per action: ~500ns (4 metric calls: queued, queue size x2, execution duration)
- Per tick: ~100ns (1 metric call: action queued, if action emitted)
- **Total overhead: < 1μs per action, negligible**

**Mitigation:**
- Metrics are only recorded, not queried (no blocking)
- Labels are pre-computed (no string allocation during recording)
- No conditionals (metrics always recorded)

**Validation:**
- Benchmark before/after integration
- Verify tick latency P99 < 10ms (with metrics)
- Verify no memory leaks from label cardinality

---

## Part 3: Combined Testing Strategy

### Unit Tests

**ActionExecutor Unit Tests:**
- `pkg/fsmv2/supervisor/execution/action_executor_test.go`
- Test single action execution
- Test retry logic with exponential backoff
- Test timeout handling
- Test queue full behavior
- Test graceful shutdown

**Metrics Unit Tests:**
- `pkg/fsmv2/supervisor/metrics/metrics_test.go` (already exists)
- Verify all metrics can be recorded
- Verify label values are correct
- Verify histogram buckets are appropriate

### Integration Tests

**ActionExecutor Integration:**
- `pkg/fsmv2/supervisor/supervisor_integration_test.go` (new file)
- Test action execution flow (emit → enqueue → execute → complete)
- Test worker blocking during action
- Test concurrent workers with separate executors
- Test cleanup on supervisor stop

**Metrics Integration:**
- Add to existing `pkg/fsmv2/supervisor/supervisor_test.go`
- Verify metrics are recorded during real execution
- Verify metric values match expected behavior
- Verify no metric recording errors

### Performance Benchmarks

**ActionExecutor Benchmarks:**
- Benchmark action throughput (actions/second)
- Benchmark tick latency with action in progress
- Benchmark concurrent executor performance

**Metrics Benchmarks:**
- Benchmark metric recording overhead
- Verify P99 < 1μs for metric recording
- Verify no memory allocation during recording

**Test Cases:**
```go
func BenchmarkActionEnqueue(b *testing.B) {
    executor := setupTestExecutor()
    action := &MockAction{}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        executor.EnqueueAction(action)
        <-time.After(1 * time.Millisecond)  // Drain queue
    }
}

func BenchmarkMetricsRecording(b *testing.B) {
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        metrics.RecordActionQueued("test", "TestAction")
    }
}

func BenchmarkTickWithMetrics(b *testing.B) {
    supervisor := setupTestSupervisor()
    ctx := context.Background()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        supervisor.Tick(ctx)
    }
}
```

### Backward Compatibility Verification

**Breaking Changes Check:**
- [ ] Verify existing Worker implementations still work
- [ ] Verify existing State machines still work
- [ ] Verify existing Action implementations still work
- [ ] Verify no API changes to public interfaces

**Migration Testing:**
- [ ] Test upgrade path: old supervisor → new supervisor
- [ ] Test rollback path: new supervisor → old supervisor (if needed)
- [ ] Verify no data loss during migration

---

## Implementation Checklist

### Phase 1: ActionExecutor Infrastructure

- [ ] **Phase 1.1:** Add executor field to WorkerContext
- [ ] **Phase 1.2:** Initialize executors in AddWorker()
- [ ] **Phase 1.3:** Start executors in Start()
- [ ] **Phase 1.4:** Add action blocking check to tickWorker()
- [ ] **Phase 1.5:** Route actions to executor (replace old method)
- [ ] **Phase 1.6:** Remove old executeActionWithRetry() method
- [ ] **Phase 1.7:** Add executor cleanup to RemoveWorker() and Stop()
- [ ] **Phase 1.8:** Write integration tests

### Phase 2: Metrics Integration

- [ ] **Phase 2.1:** Add metrics calls to ActionExecutor (4 metrics)
- [ ] **Phase 2.2:** Add RecordActionQueued to supervisor
- [ ] **Phase 2.3:** (Optional) Add supervisor hierarchy metrics
- [ ] **Phase 2.4:** Verify metrics in tests
- [ ] **Phase 2.5:** Benchmark performance impact

### Phase 3: Testing and Validation

- [ ] **Phase 3.1:** Run all unit tests (must pass)
- [ ] **Phase 3.2:** Run integration tests (must pass)
- [ ] **Phase 3.3:** Run performance benchmarks (must meet targets)
- [ ] **Phase 3.4:** Verify backward compatibility
- [ ] **Phase 3.5:** Manual testing with real workers

---

## Success Criteria

### ActionExecutor Integration

✅ **Complete when:**
1. Every worker has its own ActionExecutor instance
2. Actions are enqueued (not executed inline)
3. Workers are blocked during action execution
4. Actions execute in background with retry/timeout
5. Old executeActionWithRetry() method is deleted
6. All integration tests pass
7. No goroutine leaks
8. No race conditions

### Metrics Integration

✅ **Complete when:**
1. All 4 action metrics are recorded correctly
2. RecordActionQueued is called in supervisor
3. Metrics values match observed behavior
4. Performance overhead < 1μs per action
5. No memory leaks from metrics
6. Metrics visible in Prometheus scrape

---

## Open Questions

1. **Should ActionStatus be persisted in TriangularStore?**
   - Current: In-memory only
   - Pro: Persists across restarts
   - Con: More storage overhead
   - **Decision:** Start in-memory, add persistence if needed

2. **Should supervisor hierarchy metrics be mandatory?**
   - Current: Optional (only if supervisor composition exists)
   - Pro: Useful for debugging nested supervisors
   - Con: Not all supervisors have children
   - **Decision:** Make optional, document when to use

3. **Should we add action cancellation in this phase?**
   - Current: No cancellation (action runs to completion)
   - Pro: Allows aborting stuck operations
   - Con: Complicates idempotency
   - **Decision:** Defer to Phase 4 (future enhancement)

4. **Should RecordWorkerPool metrics be removed?**
   - Current: Defined but unused (no worker pool)
   - Pro: Cleaner API surface
   - Con: Will need to re-add later
   - **Decision:** Keep in code, mark as "Future" in comments

---

## References

### Investigation Reports
- **ActionExecutor Design:** `docs/design/fsmv2-async-action-executor.md`
- **Metrics Specification:** `pkg/fsmv2/supervisor/METRICS.md`

### Related Documents
- **Package Restructuring:** `docs/plans/fsmv2-package-restructuring.md`
- **Infrastructure Supervision:** `docs/design/fsmv2-infrastructure-supervision-patterns.md`

### Key Files
- **Supervisor:** `pkg/fsmv2/supervisor/supervisor.go`
- **ActionExecutor:** `pkg/fsmv2/supervisor/execution/action_executor.go`
- **Metrics:** `pkg/fsmv2/supervisor/metrics/metrics.go`

---

## Changelog

### 2025-11-08 14:30 - Plan created
Initial plan created covering ActionExecutor full integration (8 phases) and metrics integration (7 unused metrics). Plan extends fsmv2-package-restructuring.md with implementation details for completing partially-built subsystems. Includes comprehensive testing strategy and performance benchmarks.
