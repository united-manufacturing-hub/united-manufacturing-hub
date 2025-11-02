# FSMv2 Async Action Executor Pattern - Design Document

**Created:** 2025-11-01
**Status:** Design Proposal
**Author:** Claude Code

---

## 1. Pattern Overview

### Core Principle

The Async Action Executor pattern enables FSMv2 to execute long-running actions (e.g., starting Benthos, waiting for S6 readiness) without blocking the tick loop. This follows **Kubernetes level-based reconciliation**: states emit actions describing desired outcomes, the executor handles asynchronous execution, and future ticks observe the results.

### Why Async > Sync

**Current (Sync) Problem:**
```go
func (s TryingToStartState) Next(snapshot Snapshot) (State, Signal, Action) {
    return s, SignalNone, &StartBenthosAction{} // BLOCKS tick until done
}
```

- Tick loop blocks during action execution
- Worker cannot respond to new observations
- Timeout failures leave system in unknown state
- No visibility into action progress

**Proposed (Async) Solution:**
```go
func (s TryingToStartState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Observed.ActionInProgress {
        return s, SignalNone, nil // Wait for completion
    }
    if snapshot.Observed.ActionSucceeded {
        return RunningState{}, SignalNone, nil // Transition
    }
    if snapshot.Observed.ActionFailed {
        // Retry with backoff or escalate
    }
    return s, SignalNone, &StartBenthosAction{} // Emit action
}
```

- Tick loop never blocks
- Worker continues observing system state
- Action progress visible in observations
- Retries handled by executor with backoff

### Leveraging Existing Collector Pattern

FSMv2 already has async observation collection via `Collector`:
- Separate goroutine per worker
- Timeout protection
- Restart/backoff logic
- Storage integration

**Async actions mirror this design:**
```
Collector (Existing)          ActionExecutor (New)
├─ Goroutine per worker       ├─ Goroutine per worker
├─ Collects ObservedState     ├─ Executes Actions
├─ Saves to TriangularStore   ├─ Updates ActionStatus
├─ Timeout protection         ├─ Timeout protection
└─ Exponential backoff        └─ Exponential backoff
```

Both integrate seamlessly with the tick loop:
- **Collector** provides fresh data → States decide transitions
- **ActionExecutor** executes actions → States observe results

---

## 2. Architecture

### 2.1 Per-Worker Action Executor Goroutine

Each worker gets its own action executor (similar to Collector pattern):

```go
type WorkerContext struct {
    mu             sync.RWMutex
    tickInProgress atomic.Bool
    identity       fsmv2.Identity
    worker         fsmv2.Worker
    currentState   fsmv2.State
    collector      *Collector        // Existing
    executor       *ActionExecutor   // NEW
}
```

### 2.2 Action Queue Structure

**Size-1 channel ensures exactly one action per worker:**

```go
type ActionExecutor struct {
    config          ActionExecutorConfig
    actionQueue     chan fsmv2.Action  // Size 1 (only accept 1 action at a time)
    currentAction   atomic.Value       // Currently executing action
    mu              sync.RWMutex
    ctx             context.Context
    cancel          context.CancelFunc
    goroutineDone   chan struct{}
}
```

**Why size 1?**
- Enforces "1 action per worker" constraint
- Back-pressure: if action queue full, supervisor doesn't emit new actions
- Prevents action pile-up
- Matches FSMv2 philosophy: one operation at a time

### 2.3 Worker Coordination Mechanism

**Blocking Prevention:**

```go
// In supervisor/supervisor.go tickWorker()
workerCtx.mu.RLock()
actionInProgress := workerCtx.executor.HasActionInProgress()
workerCtx.mu.RUnlock()

if actionInProgress {
    // Skip calling state.Next() - worker blocked
    s.logger.Debugf("Worker %s blocked by action: %s",
        workerID, workerCtx.executor.CurrentActionName())
    return nil
}
```

**Status in Snapshot:**

Actions update a special `ActionStatus` observable that gets included in snapshots:

```go
type ActionStatus struct {
    InProgress   bool
    ActionName   string
    StartedAt    time.Time
    Succeeded    bool
    Failed       bool
    ErrorMessage string
    Retries      int
}

// Workers can access this in CollectObservedState
type MyObservedState struct {
    // ... other fields
    ActionStatus *ActionStatus  // Injected by supervisor
}
```

### 2.4 Integration with Existing Tick Loop

**Modified tickWorker() flow:**

```go
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // 1. Load snapshot (unchanged)
    snapshot := loadSnapshot(...)

    // 2. Check data freshness (unchanged)
    if !s.CheckDataFreshness(snapshot) {
        return nil
    }

    // 3. NEW: Check if action in progress
    if workerCtx.executor.HasActionInProgress() {
        s.logger.Debug("Worker blocked by async action")
        return nil  // Skip state.Next() this tick
    }

    // 4. Call state.Next() (unchanged)
    nextState, signal, action := currentState.Next(*snapshot)

    // 5. NEW: Enqueue action instead of blocking execution
    if action != nil {
        if err := workerCtx.executor.EnqueueAction(action); err != nil {
            s.logger.Errorf("Failed to enqueue action: %v", err)
            // Action queue full - worker still blocked by previous action
        }
    }

    // 6. Transition to next state (unchanged)
    if nextState != currentState {
        workerCtx.currentState = nextState
    }

    // 7. Process signal (unchanged)
    s.processSignal(ctx, workerID, signal)

    return nil
}
```

---

## 3. Constraints Satisfaction

### C1: Always Execute 1 Action Per Worker (or No Action)

**Enforcement:**
- Action queue size = 1 (channel buffer)
- `EnqueueAction()` returns error if queue full
- `HasActionInProgress()` prevents state.Next() from emitting new actions

**Verification:**
```go
func TestOneActionAtATime(t *testing.T) {
    executor := NewActionExecutor(...)

    // First action accepted
    err1 := executor.EnqueueAction(action1)
    assert.NoError(t, err1)

    // Second action rejected (queue full)
    err2 := executor.EnqueueAction(action2)
    assert.Error(t, err2)
    assert.Contains(t, err2.Error(), "action queue full")
}
```

### C2: Worker Blocked While Action Running

**Enforcement:**
- `tickWorker()` checks `HasActionInProgress()` before calling `state.Next()`
- Action status included in `ObservedState` for visibility
- Supervisor logs indicate blocking

**Verification:**
```go
func TestWorkerBlockedDuringAction(t *testing.T) {
    supervisor := NewSupervisor(...)

    // Enqueue long-running action
    executor.EnqueueAction(&SlowAction{duration: 5 * time.Second})

    // Tick multiple times
    for i := 0; i < 10; i++ {
        supervisor.Tick(ctx)
        time.Sleep(100 * time.Millisecond)
    }

    // State.Next() should NOT have been called during action
    assert.Equal(t, 0, stateNextCallCount)
}
```

### C3: Stuck Detection (Timeout Mechanism)

**Implementation:**

```go
func (e *ActionExecutor) executeAction(ctx context.Context, action fsmv2.Action) error {
    // Default timeout: 5 minutes
    timeout := e.config.ActionTimeout
    if timeout == 0 {
        timeout = DefaultActionTimeout // 5 * time.Minute
    }

    actionCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    e.logger.Infof("Executing action: %s (timeout=%v)", action.Name(), timeout)

    err := action.Execute(actionCtx)

    if errors.Is(err, context.DeadlineExceeded) {
        e.logger.Errorf("Action %s timed out after %v", action.Name(), timeout)
        return fmt.Errorf("action timeout: %w", err)
    }

    return err
}
```

**Timeout triggers:**
- Action marked as `Failed`
- Retry logic applies (see C4)
- State observes timeout via `ActionStatus.Failed`

### C4: Retry Logic (Exponential Backoff)

**Implementation:**

```go
func (e *ActionExecutor) actionLoop() {
    for {
        select {
        case <-e.ctx.Done():
            return

        case action := <-e.actionQueue:
            e.executeWithRetry(action)
        }
    }
}

func (e *ActionExecutor) executeWithRetry(action fsmv2.Action) {
    maxRetries := e.config.MaxRetries
    if maxRetries == 0 {
        maxRetries = DefaultMaxRetries // 3
    }

    for attempt := 0; attempt <= maxRetries; attempt++ {
        // Update status: InProgress
        e.updateActionStatus(ActionStatus{
            InProgress: true,
            ActionName: action.Name(),
            StartedAt:  time.Now(),
            Retries:    attempt,
        })

        err := e.executeAction(e.ctx, action)

        if err == nil {
            // Success
            e.updateActionStatus(ActionStatus{
                Succeeded: true,
                ActionName: action.Name(),
            })
            return
        }

        // Failure
        e.logger.Errorf("Action %s failed (attempt %d/%d): %v",
            action.Name(), attempt+1, maxRetries+1, err)

        if attempt < maxRetries {
            // Exponential backoff: 2^attempt seconds
            backoff := time.Duration(1<<uint(attempt)) * time.Second
            e.logger.Warnf("Retrying action %s after %v backoff",
                action.Name(), backoff)
            time.Sleep(backoff)
        } else {
            // Max retries reached
            e.updateActionStatus(ActionStatus{
                Failed:       true,
                ActionName:   action.Name(),
                ErrorMessage: err.Error(),
                Retries:      maxRetries,
            })
        }
    }
}
```

**Backoff schedule:**
- Attempt 1: Immediate
- Attempt 2: 1s delay
- Attempt 3: 2s delay
- Attempt 4: 4s delay

**Retriable vs Terminal Errors:**
- All errors are retriable by default
- Actions can return `ErrNonRetriable` to skip retries:

```go
var ErrNonRetriable = errors.New("non-retriable error")

func (a *CreateFileAction) Execute(ctx context.Context) error {
    if invalidPath(a.path) {
        return fmt.Errorf("%w: invalid path %s", ErrNonRetriable, a.path)
    }
    // ...
}
```

---

## 4. Code Specification

### 4.1 ActionExecutor Struct Definition

```go
package supervisor

import (
    "context"
    "fmt"
    "sync"
    "sync/atomic"
    "time"

    "go.uber.org/zap"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

type ActionExecutorConfig struct {
    WorkerID       string
    Logger         *zap.SugaredLogger
    ActionTimeout  time.Duration  // Per-action timeout (default: 5 min)
    MaxRetries     int            // Max retry attempts (default: 3)
}

type ActionExecutor struct {
    config          ActionExecutorConfig
    actionQueue     chan fsmv2.Action  // Size 1
    currentAction   atomic.Value       // *fsmv2.Action
    currentStatus   atomic.Value       // *ActionStatus
    mu              sync.RWMutex
    ctx             context.Context
    cancel          context.CancelFunc
    goroutineDone   chan struct{}
}

type ActionStatus struct {
    InProgress   bool
    Succeeded    bool
    Failed       bool
    ActionName   string
    StartedAt    time.Time
    ErrorMessage string
    Retries      int
}

func NewActionExecutor(config ActionExecutorConfig) *ActionExecutor {
    return &ActionExecutor{
        config:      config,
        actionQueue: make(chan fsmv2.Action, 1),  // Size 1 enforces constraint
    }
}

func (e *ActionExecutor) Start(ctx context.Context) error {
    e.mu.Lock()
    defer e.mu.Unlock()

    e.ctx, e.cancel = context.WithCancel(ctx)
    e.goroutineDone = make(chan struct{})

    go e.actionLoop()

    return nil
}

func (e *ActionExecutor) Stop(ctx context.Context) {
    e.mu.Lock()
    e.cancel()
    doneChan := e.goroutineDone
    e.mu.Unlock()

    select {
    case <-doneChan:
        e.config.Logger.Info("ActionExecutor stopped")
    case <-ctx.Done():
        e.config.Logger.Warn("Context cancelled while waiting for executor to stop")
    case <-time.After(5 * time.Second):
        e.config.Logger.Error("Timeout waiting for executor to stop")
    }
}

func (e *ActionExecutor) EnqueueAction(action fsmv2.Action) error {
    select {
    case e.actionQueue <- action:
        e.config.Logger.Infof("Enqueued action: %s", action.Name())
        return nil
    default:
        return fmt.Errorf("action queue full, worker blocked by: %s",
            e.CurrentActionName())
    }
}

func (e *ActionExecutor) HasActionInProgress() bool {
    status := e.GetActionStatus()
    return status.InProgress
}

func (e *ActionExecutor) CurrentActionName() string {
    if action := e.currentAction.Load(); action != nil {
        return action.(fsmv2.Action).Name()
    }
    return ""
}

func (e *ActionExecutor) GetActionStatus() ActionStatus {
    if status := e.currentStatus.Load(); status != nil {
        return *status.(*ActionStatus)
    }
    return ActionStatus{}
}

func (e *ActionExecutor) updateActionStatus(status ActionStatus) {
    e.currentStatus.Store(&status)
}

// Private methods
func (e *ActionExecutor) actionLoop() {
    defer close(e.goroutineDone)

    for {
        select {
        case <-e.ctx.Done():
            e.config.Logger.Info("ActionExecutor loop stopped")
            return

        case action := <-e.actionQueue:
            e.currentAction.Store(&action)
            e.executeWithRetry(action)
            e.currentAction.Store((*fsmv2.Action)(nil))
        }
    }
}

func (e *ActionExecutor) executeWithRetry(action fsmv2.Action) {
    // See C4 implementation above
}

func (e *ActionExecutor) executeAction(ctx context.Context, action fsmv2.Action) error {
    // See C3 implementation above
}
```

### 4.2 Integration Points with Supervisor

**Changes to Supervisor struct:**

```go
type WorkerContext struct {
    mu             sync.RWMutex
    tickInProgress atomic.Bool
    identity       fsmv2.Identity
    worker         fsmv2.Worker
    currentState   fsmv2.State
    collector      *Collector
    executor       *ActionExecutor  // NEW
}
```

**Changes to AddWorker():**

```go
func (s *Supervisor) AddWorker(identity fsmv2.Identity, worker fsmv2.Worker) error {
    // ... existing collector setup ...

    executor := NewActionExecutor(ActionExecutorConfig{
        WorkerID: identity.ID,
        Logger:   s.logger,
    })

    s.workers[identity.ID] = &WorkerContext{
        identity:     identity,
        worker:       worker,
        currentState: worker.GetInitialState(),
        collector:    collector,
        executor:     executor,  // NEW
    }

    return nil
}
```

**Changes to Start():**

```go
func (s *Supervisor) Start(ctx context.Context) <-chan struct{} {
    // ... existing code ...

    // Start executors for all workers
    s.mu.RLock()
    for _, workerCtx := range s.workers {
        if err := workerCtx.executor.Start(ctx); err != nil {
            s.logger.Errorf("Failed to start executor for worker %s: %v",
                workerCtx.identity.ID, err)
        }
    }
    s.mu.RUnlock()

    // ... existing tick loop ...
}
```

**Changes to tickWorker():**

```go
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error {
    // ... existing snapshot loading ...

    // NEW: Check if action in progress
    if workerCtx.executor.HasActionInProgress() {
        s.logger.Debugf("Worker %s blocked by action: %s",
            workerID, workerCtx.executor.CurrentActionName())
        return nil
    }

    // ... existing state.Next() call ...

    // NEW: Enqueue action instead of blocking execution
    if action != nil {
        if err := workerCtx.executor.EnqueueAction(action); err != nil {
            s.logger.Errorf("Failed to enqueue action: %v", err)
        }
    }

    // ... existing state transition ...
}
```

### 4.3 Integration Points with Worker Interface

**No changes needed to Worker interface!**

Workers continue to implement:
- `CollectObservedState(ctx) (ObservedState, error)`
- `DeriveDesiredState(spec) (DesiredState, error)`
- `GetInitialState() State`

**Optional enhancement**: Workers can expose action status in observations:

```go
type BenthosObservedState struct {
    ProcessRunning   bool
    ConfigValid      bool
    ActionStatus     *supervisor.ActionStatus  // Optional
    // ...
}

func (w *BenthosWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    observed := &BenthosObservedState{
        // ... collect process state ...
    }

    // Optionally include action status for visibility
    if w.executor != nil {
        status := w.executor.GetActionStatus()
        observed.ActionStatus = &status
    }

    return observed, nil
}
```

### 4.4 State Machine Changes Needed

**Existing states work unchanged!**

States that emit actions continue to work:

```go
func (s TryingToStartState) Next(snapshot Snapshot) (State, Signal, Action) {
    if snapshot.Desired.ShutdownRequested() {
        return StoppingState{}, SignalNone, nil
    }

    // Emit action (will be executed async)
    return s, SignalNone, &StartBenthosAction{}
}
```

**Optional enhancement**: States can observe action status:

```go
func (s TryingToStartState) Next(snapshot Snapshot) (State, Signal, Action) {
    observed := snapshot.Observed.(*BenthosObservedState)

    // Check if action in progress
    if observed.ActionStatus != nil && observed.ActionStatus.InProgress {
        s.logger.Debugf("Waiting for action: %s", observed.ActionStatus.ActionName)
        return s, SignalNone, nil  // Stay in state, wait
    }

    // Check if action succeeded
    if observed.ProcessRunning {
        return RunningState{}, SignalNone, nil
    }

    // Check if action failed
    if observed.ActionStatus != nil && observed.ActionStatus.Failed {
        s.logger.Errorf("Action failed: %s", observed.ActionStatus.ErrorMessage)
        // Retry or escalate
    }

    // Emit action if no action in progress
    if observed.ActionStatus == nil || !observed.ActionStatus.InProgress {
        return s, SignalNone, &StartBenthosAction{}
    }

    return s, SignalNone, nil
}
```

### 4.5 Example State That Emits Async Action

```go
// pkg/fsmv2/benthos/state_trying_to_start.go

type TryingToStartState struct {
    reason string
}

func (s TryingToStartState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
    // Always check shutdown first
    if snapshot.Desired.ShutdownRequested() {
        return StoppingState{}, fsmv2.SignalNone, nil
    }

    observed := snapshot.Observed.(*BenthosObservedState)

    // If action in progress, wait
    if observed.ActionStatus != nil && observed.ActionStatus.InProgress {
        return s, fsmv2.SignalNone, nil
    }

    // If process is running, transition to Running
    if observed.ProcessRunning && observed.ConfigValid {
        return RunningState{}, fsmv2.SignalNone, nil
    }

    // If action failed repeatedly, escalate
    if observed.ActionStatus != nil && observed.ActionStatus.Failed {
        if observed.ActionStatus.Retries >= 3 {
            return FailedState{
                reason: fmt.Sprintf("failed to start after %d retries: %s",
                    observed.ActionStatus.Retries, observed.ActionStatus.ErrorMessage),
            }, fsmv2.SignalNone, nil
        }
    }

    // Emit action to start Benthos
    return s, fsmv2.SignalNone, &StartBenthosAction{
        identity:   snapshot.Identity,
        configPath: observed.ConfigPath,
    }
}

func (s TryingToStartState) String() string {
    return "TryingToStart"
}

func (s TryingToStartState) Reason() string {
    if s.reason != "" {
        return s.reason
    }
    return "attempting to start Benthos process"
}
```

---

## 5. Error Handling

### 5.1 Retriable vs Terminal Errors

**Retriable Errors** (will retry with backoff):
- Network timeouts
- Temporary file system issues
- Process spawn failures (e.g., resource limits)
- Configuration validation errors (might be transient)

**Terminal Errors** (no retry):
- Invalid configuration (malformed YAML)
- Missing required files
- Permission denied (permanent)
- Explicitly marked with `ErrNonRetriable`

**Detection:**

```go
var ErrNonRetriable = errors.New("non-retriable error")

func (e *ActionExecutor) executeWithRetry(action fsmv2.Action) {
    for attempt := 0; attempt <= maxRetries; attempt++ {
        err := e.executeAction(e.ctx, action)

        if err == nil {
            // Success
            return
        }

        // Check if error is non-retriable
        if errors.Is(err, ErrNonRetriable) {
            e.logger.Errorf("Non-retriable error for action %s: %v",
                action.Name(), err)
            e.updateActionStatus(ActionStatus{
                Failed:       true,
                ActionName:   action.Name(),
                ErrorMessage: fmt.Sprintf("non-retriable: %v", err),
            })
            return
        }

        // Retry with backoff
        if attempt < maxRetries {
            backoff := time.Duration(1<<uint(attempt)) * time.Second
            time.Sleep(backoff)
        }
    }

    // Max retries reached
    e.updateActionStatus(ActionStatus{
        Failed:       true,
        ActionName:   action.Name(),
        ErrorMessage: fmt.Sprintf("max retries reached: %v", err),
        Retries:      maxRetries,
    })
}
```

### 5.2 Max Retry Limits

**Configuration:**

```go
const (
    DefaultMaxRetries = 3
    DefaultActionTimeout = 5 * time.Minute
)

type ActionExecutorConfig struct {
    // ...
    MaxRetries    int            // Default: 3
    ActionTimeout time.Duration  // Default: 5 minutes
}
```

**Escalation after max retries:**

States observe `ActionStatus.Failed` and `ActionStatus.Retries` to decide:

```go
if observed.ActionStatus != nil && observed.ActionStatus.Failed {
    if observed.ActionStatus.Retries >= 3 {
        // Escalate to failed state or request shutdown
        return FailedState{
            reason: fmt.Sprintf("action %s failed after %d retries",
                observed.ActionStatus.ActionName, observed.ActionStatus.Retries),
        }, fsmv2.SignalNone, nil
    }
}
```

### 5.3 Backoff Strategies

**Exponential backoff** (default):
- Attempt 1: Immediate
- Attempt 2: 1s delay (2^0)
- Attempt 3: 2s delay (2^1)
- Attempt 4: 4s delay (2^2)

**Jitter** (optional enhancement for future):

```go
func (e *ActionExecutor) backoffWithJitter(attempt int) time.Duration {
    base := time.Duration(1<<uint(attempt)) * time.Second
    jitter := time.Duration(rand.Intn(500)) * time.Millisecond
    return base + jitter
}
```

### 5.4 Timeout Handling

**Per-action timeout:**

```go
func (e *ActionExecutor) executeAction(ctx context.Context, action fsmv2.Action) error {
    timeout := e.config.ActionTimeout
    if timeout == 0 {
        timeout = DefaultActionTimeout  // 5 minutes
    }

    actionCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    err := action.Execute(actionCtx)

    if errors.Is(err, context.DeadlineExceeded) {
        return fmt.Errorf("action %s timed out after %v: %w",
            action.Name(), timeout, err)
    }

    return err
}
```

**Timeout is retriable:**
- First timeout → retry with backoff
- Second timeout → retry with longer backoff
- Third timeout → escalate to failed state

**State observability:**

```go
if observed.ActionStatus.Failed &&
   strings.Contains(observed.ActionStatus.ErrorMessage, "timed out") {
    // Action is stuck, consider escalating to shutdown
}
```

---

## 6. Comparison Table

### Current (Sync) vs Proposed (Async)

| Aspect | Current (Sync) | Proposed (Async) |
|--------|----------------|------------------|
| **Tick loop** | Blocks during action execution | Never blocks, continues ticking |
| **Action execution** | Inline in `tickWorker()` | Separate goroutine per worker |
| **Timeout handling** | Context cancellation only | Context cancellation + retry + backoff |
| **Retry logic** | Per-action in `executeActionWithRetry()` | Per-action in `ActionExecutor` |
| **Worker blocking** | Implicit (tick blocked) | Explicit (checked before state.Next()) |
| **Action visibility** | Logs only | `ActionStatus` in observations |
| **Progress tracking** | None | `ActionStatus.InProgress`, `StartedAt`, `Retries` |
| **Concurrency** | 1 action total | 1 action per worker (N workers = N concurrent) |
| **State awareness** | States don't know action status | States can observe `ActionStatus` |
| **Error handling** | Limited (3 retries, exponential backoff) | Rich (retriable vs terminal, configurable) |
| **Migration** | N/A | No changes to Worker or State interfaces |

### Trade-offs and Benefits

**Benefits:**

1. **Non-blocking tick loop**: Worker continues responding to observations
2. **Better observability**: Action progress visible in `ActionStatus`
3. **Consistent patterns**: Mirrors existing `Collector` design
4. **Worker isolation**: One worker's stuck action doesn't block others
5. **Graceful degradation**: Failed actions → `ActionStatus.Failed` → State decides
6. **Testability**: Can inject mock actions, verify status updates
7. **Zero interface changes**: Existing Worker and State code works unchanged

**Trade-offs:**

1. **Additional goroutine**: One more goroutine per worker (acceptable, mirrors Collector)
2. **Slightly more complex**: Need to understand action queue + status propagation
3. **Status propagation**: States need to check `ActionStatus` (optional, but recommended)
4. **Memory overhead**: `ActionStatus` stored per worker (minimal, single struct)

### Migration Path

**Phase 1: Add Infrastructure (Non-breaking)**
- Add `ActionExecutor` struct
- Integrate with `Supervisor` (parallel with existing sync execution)
- Add `ActionStatus` to observations (optional field)

**Phase 2: Migrate States (Gradual)**
- Existing states continue to work unchanged
- New states can observe `ActionStatus` for better control
- Update critical states (e.g., `TryingToStartState`) to check action status

**Phase 3: Remove Sync Execution (Clean-up)**
- Once all states migrated, remove `executeActionWithRetry()` from tick loop
- All actions now async

**Rollback Safety:**
- Can revert to sync execution by disabling executor in config
- No state machine changes required for rollback

---

## 7. Implementation Checklist

- [ ] Create `ActionExecutor` struct with size-1 channel
- [ ] Add `HasActionInProgress()` and `GetActionStatus()` methods
- [ ] Integrate `ActionExecutor` into `WorkerContext`
- [ ] Modify `tickWorker()` to check action status before `state.Next()`
- [ ] Modify `tickWorker()` to enqueue actions instead of blocking
- [ ] Add `ActionStatus` to example `ObservedState` structs
- [ ] Update `TryingToStartState` to observe `ActionStatus`
- [ ] Write tests for action queue constraint (size 1)
- [ ] Write tests for worker blocking behavior
- [ ] Write tests for retry logic with exponential backoff
- [ ] Write tests for timeout handling
- [ ] Write integration test for full async flow
- [ ] Document in `worker.go` godoc
- [ ] Update FSMv2 design document with async pattern

---

## 8. Future Enhancements

**8.1 Action Cancellation**

Allow states to cancel in-progress actions:

```go
func (e *ActionExecutor) CancelCurrentAction() error {
    // Cancel action context, move to next action in queue
}
```

**8.2 Action Priority Queue**

If multiple actions needed, use priority queue instead of size-1 channel.

**8.3 Action Telemetry**

Expose action metrics:
- Action duration histogram
- Retry rate per action type
- Timeout rate per action type

**8.4 Jittered Backoff**

Add jitter to exponential backoff to prevent thundering herd.

---

## 9. Open Questions

1. **Should ActionStatus be part of TriangularStore?**
   - Pros: Persisted across restarts, visible in database
   - Cons: More storage, might not be needed
   - Decision: Start with in-memory, add persistence if needed

2. **Should actions be cancellable mid-execution?**
   - Pros: Allows aborting stuck operations
   - Cons: Complicates idempotency, partial execution
   - Decision: Start without cancellation, add if users request

3. **Should action queue be configurable size?**
   - Pros: Could batch actions if needed
   - Cons: Violates "1 action per worker" constraint
   - Decision: Keep size 1, revisit if use case emerges

---

## References

- FSMv2 Collector pattern: `pkg/fsmv2/supervisor/collector.go`
- Kubernetes reconciliation: https://kubernetes.io/docs/concepts/architecture/controller/
- Existing action retry logic: `pkg/fsmv2/supervisor/supervisor.go:824`
