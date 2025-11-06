# Async Action Executor Implementation Plan

**Status:** Planning
**Created:** 2025-02-11
**Target:** FSM v2 System
**Related Design Doc:** `docs/design/fsmv2-async-action-executor.md`

## Executive Summary

This plan implements asynchronous action execution for the FSM v2 system, introducing a **global worker pool pattern** that prevents tick loop blocking during long-running actions. The implementation enables multiple supervisors to share a single configurable ActionExecutor, allowing resource-efficient deployments across edge and cloud environments.

**Key Benefits:**
- Non-blocking tick loop (fresh observations always collected)
- Bounded resource usage (configurable worker pool)
- Tunable for deployment (edge: 10 workers, cloud: 50+ workers)
- Zero state-level boilerplate (supervisor handles action-in-progress logic)
- Compatible with registry pattern (parallel refactor)

**Estimated Effort:** 31-41 hours (5-6 days focused work)

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Architectural Decisions](#2-architectural-decisions)
3. [Tradeoffs Analysis](#3-tradeoffs-analysis)
4. [Registry Pattern Integration](#4-registry-pattern-integration)
5. [Interface Changes](#5-interface-changes)
6. [Detailed Implementation Plan](#6-detailed-implementation-plan)
7. [Migration Guide](#7-migration-guide)
8. [Testing Strategy](#8-testing-strategy)
9. [Configuration](#9-configuration)
10. [Timeline and Effort](#10-timeline-and-effort)
11. [Success Criteria](#11-success-criteria)
12. [Risks and Mitigations](#12-risks-and-mitigations)

---

## 1. Problem Statement

### Current Behavior

The FSM v2 system currently executes actions synchronously during the tick loop:

```go
func (s *Supervisor) tickWorker(ctx context.Context, workerCtx *WorkerContext) {
    // 1. Collect observed state
    observed, _ := workerCtx.collector.GetLatestObservation(ctx)

    // 2. Derive next state
    nextState, signal, action := currentState.Next(snapshot)

    // 3. Execute action synchronously (BLOCKS HERE)
    if action != nil {
        action.Execute(ctx)  // May take seconds/minutes
    }

    // 4. Update state
    workerCtx.currentState = nextState
}
```

**Problems:**
1. **Tick loop blocks** during action execution (e.g., HTTP calls, database operations)
2. **No fresh observations** collected while action runs
3. **Unbounded goroutines** in complex architectures (1 action = 1 goroutine in some proposals)
4. **State machines must check** `HasActionInProgress()` themselves (boilerplate)

### Desired Behavior

Actions execute asynchronously in a **global worker pool**:

```go
func (s *Supervisor) tickWorker(ctx context.Context, workerCtx *WorkerContext) {
    // 1. ALWAYS collect fresh observed state
    observed, _ := workerCtx.collector.GetLatestObservation(ctx)

    // 2. Check if action already running (supervisor-level, no state boilerplate)
    if s.actionExecutor.HasActionInProgress(workerCtx.identity.ID) {
        // Skip action derivation, continue to next tick
        return
    }

    // 3. Derive next state and action
    nextState, signal, action := currentState.Next(snapshot)

    // 4. Enqueue action for async execution (non-blocking)
    if action != nil {
        s.actionExecutor.EnqueueAction(
            workerCtx.identity.ID,
            action,
            workerCtx.registry,  // Injected at execution time
        )
    }

    // 5. Update state immediately (no waiting)
    workerCtx.currentState = nextState
}
```

---

## 2. Architectural Decisions

### Decision Summary Table

| Decision | Chosen Approach | Rationale |
|----------|-----------------|-----------|
| **Executor Scope** | Global instance (non-singleton) | Resource pooling, bounded goroutines |
| **Worker Pool** | Configurable goroutine pool | Tunable per deployment (edge vs cloud) |
| **Registry Injection** | At execution time | Aligns with pooled executor, actions stateless |
| **Tick Skip Logic** | Supervisor-level check | Avoids boilerplate in every state |
| **Observation Collection** | Always collect, skip derivation | Fresh observations critical for decisions |
| **Pool Size Default** | Dynamic/configurable | Edge: 10, Cloud: 50+, production tunable |

### Decision Details

#### D1: Global ActionExecutor (Non-Singleton)

**Decision:** One ActionExecutor instance shared across all supervisors, managed by control loop.

**NOT a singleton:** Control loop creates and passes to supervisors, allowing:
- Dependency injection (testability)
- Multiple instances in test scenarios
- Explicit lifecycle management

```go
// main.go (control loop)
func main() {
    executor := fsmv2.NewActionExecutor(config, logger)
    executor.Start(ctx)
    defer executor.Stop()

    // Pass to all supervisors
    commSupervisor := supervisor.NewSupervisor(commConfig, executor)
    benthosSupervisor := supervisor.NewSupervisor(benthosConfig, executor)
}
```

#### D2: Worker Pool Pattern

**Decision:** Fixed-size goroutine pool (configurable), unbounded task queue.

**Architecture:**
```go
type ActionExecutor struct {
    workerPool chan struct{}      // Semaphore (size N)
    taskQueue  chan *actionTask   // Unbounded
    statusMap  sync.Map           // workerID -> *ActionStatus
}

// N goroutines in pool
func (e *ActionExecutor) Start(ctx context.Context) {
    for i := 0; i < e.config.PoolSize; i++ {
        go e.executeWorker(ctx)
    }
}
```

**NOT per-worker executors:** Avoids unbounded goroutines in complex architectures.

#### D3: Registry Injection at Execution Time

**Decision:** Actions don't store registry, receive it during `Execute(ctx, registry)`.

**Interface change:**
```go
// Before (construction-time):
type Action interface {
    Execute(ctx context.Context) error
    Name() string
}

// After (execution-time):
type Action interface {
    Execute(ctx context.Context, registry Registry) error
    Name() string
}
```

**Benefits:**
- Actions remain stateless (registry not stored)
- Pool can inject correct registry per worker
- Aligns with parallel registry refactor

#### D4: Supervisor-Level Action-in-Progress Check

**Decision:** Supervisor checks `HasActionInProgress()` before deriving actions, not states.

**Eliminates state-level boilerplate:**
```go
// ❌ Old: Every state must check
func (s *MyState) Next(snap Snapshot) (State, Signal, Action) {
    if snap.Observed.ActionStatus.InProgress {
        return s, SignalNone, nil  // Stay in current state
    }
    // ... derive action
}

// ✅ New: Supervisor handles automatically
func (s *Supervisor) tickWorker(ctx, workerCtx) {
    if s.actionExecutor.HasActionInProgress(workerCtx.identity.ID) {
        return  // Skip derivation automatically
    }
    // ... proceed with state transition
}
```

#### D5: Always Collect Observations

**Decision:** Observation collection continues even when action is running.

**Critical for decision-making:**
- Fresh data informs future state transitions
- Prevents stale observations accumulating
- Enables monitoring/metrics during long actions

---

## 3. Tradeoffs Analysis

### 3.1 Executor Scope: Global vs Per-Worker vs Per-Supervisor

**Options Considered:**

| Approach | Pros | Cons | Decision |
|----------|------|------|----------|
| **Per-Worker** (original design) | Simple, isolated | Unbounded goroutines, no resource pooling | ❌ Rejected |
| **Per-Supervisor** | Supervisor-level isolation | Still unbounded across supervisors | ❌ Rejected |
| **Global Singleton** | One instance guaranteed | Hard to test, implicit dependency | ❌ Rejected |
| **Global Instance** (control-loop managed) | Resource pooling, testable, explicit | Shared state requires careful design | ✅ **Chosen** |

**Why Global Instance?**
- Control loop has visibility into deployment (edge vs cloud)
- Can configure pool size appropriately (10 for edge, 50 for cloud)
- Supervisors don't need to know about resource constraints
- Testable (inject mock executor in tests)

**Example:**
```go
// Edge deployment (limited resources)
executor := NewActionExecutor(ActionExecutorConfig{PoolSize: 10}, logger)

// Cloud deployment (more resources)
executor := NewActionExecutor(ActionExecutorConfig{PoolSize: 50}, logger)
```

### 3.2 Tick Behavior: Skip Entire Tick vs Skip Derivation Only

**Options Considered:**

| Approach | Pros | Cons | Decision |
|----------|------|------|----------|
| **Skip entire tick** | Simple implementation | Stale observations accumulate | ❌ Rejected |
| **Collect observations, skip derivation** | Fresh data, simple | Slightly more complex | ✅ **Chosen** |
| **State-level checks** | Maximum flexibility | Boilerplate in every state | ❌ Rejected |

**Why Skip Derivation Only?**
- Observations inform monitoring/metrics even during actions
- State machines may use fresh observations in future transitions
- Minimal complexity increase vs significant benefit

**Example:**
```go
// ❌ Skip entire tick (bad)
if s.actionExecutor.HasActionInProgress(workerID) {
    return  // No observation collected
}
observed, _ := collector.GetLatestObservation(ctx)

// ✅ Skip derivation only (good)
observed, _ := collector.GetLatestObservation(ctx)
if s.actionExecutor.HasActionInProgress(workerID) {
    return  // Observation collected, stored
}
nextState, _, action := currentState.Next(snapshot)
```

### 3.3 Registry Injection: Construction-Time vs Execution-Time

**Options Considered:**

| Approach | Pros | Cons | Decision |
|----------|------|------|----------|
| **Construction-time** (current) | Explicit dependencies | Actions store registry, not stateless | ❌ Rejected |
| **Execution-time injection** | Actions stateless, pool-compatible | Interface change required | ✅ **Chosen** |

**Why Execution-Time?**
- Pooled executor needs to inject correct registry per worker
- Actions become truly stateless (no registry field)
- Aligns with parallel registry refactor (see Section 4)

**Migration Impact:**
```go
// Before:
type AuthenticateAction struct {
    registry *CommunicatorRegistry  // Stored
}
func NewAuthenticateAction(reg *CommunicatorRegistry, ...) *AuthenticateAction

// After:
type AuthenticateAction struct {
    // No registry field
}
func NewAuthenticateAction(...) *AuthenticateAction  // No registry param
func (a *AuthenticateAction) Execute(ctx context.Context, registry Registry) error
```

### 3.4 Pool Size: Fixed vs Dynamic vs Configurable

**Options Considered:**

| Approach | Pros | Cons | Decision |
|----------|------|------|----------|
| **Fixed small (10)** | Conservative, safe | May bottleneck in cloud | ❌ Rejected |
| **Fixed large (50)** | High throughput | Wastes resources on edge | ❌ Rejected |
| **Dynamic (auto-scale)** | Optimal resource usage | Complex implementation | ❌ Future work |
| **Configurable** (static per deployment) | Simple, tunable | Requires config management | ✅ **Chosen** |

**Why Configurable?**
- Simple implementation (no auto-scaling logic)
- Control loop knows deployment type (edge vs cloud)
- Can be tuned in production via environment variables
- Future: Can upgrade to dynamic scaling without interface changes

**Configuration:**
```go
// Environment-based
poolSize := os.Getenv("ACTION_POOL_SIZE")  // "10" for edge, "50" for cloud
config := ActionExecutorConfig{PoolSize: poolSize}
```

---

## 4. Registry Pattern Integration

### Overview

The registry pattern (parallel refactor) provides **worker-specific tools** to actions. The AsyncActionExecutor must integrate cleanly with this pattern.

### Registry Pattern Recap

```go
// Registry provides tools (logger, HTTP clients, etc.)
type CommunicatorRegistry struct {
    *fsmv2.BaseRegistry
    transport transport.Transport
}

// Worker creates registry once
registry := NewCommunicatorRegistry(transport, logger)
worker := NewCommunicatorWorker(id, name, registry)

// Registry passes through desired state
func (w *CommunicatorWorker) DeriveDesiredState(...) (DesiredState, error) {
    return &CommunicatorDesiredState{
        Registry: w.GetRegistry(),  // Tools available to states
    }, nil
}

// State creates action with registry tools
action := NewAuthenticateAction(
    desired.Registry.GetTransport(),  // Access tools
    desired.RelayURL,
)
```

### Integration with ActionExecutor

**Key change:** Registry injection moves from **action construction** to **action execution**.

```go
// State creates action WITHOUT registry
action := NewAuthenticateAction(
    desired.RelayURL,    // Config only
    desired.InstanceUUID,
    desired.AuthToken,
)

// Supervisor enqueues WITH registry
s.actionExecutor.EnqueueAction(
    workerID,
    action,
    workerCtx.registry,  // Registry stored in WorkerContext
)

// ActionExecutor injects at execution
func (e *ActionExecutor) executeWorker(ctx context.Context) {
    for task := range e.taskQueue {
        task.action.Execute(ctx, task.registry)  // Inject here
    }
}
```

### WorkerContext Changes

```go
type WorkerContext struct {
    identity   fsmv2.Identity
    worker     fsmv2.Worker
    collector  *Collector
    registry   fsmv2.Registry  // NEW: Store registry for action execution
    // ...
}

// AddWorker stores registry
func (s *Supervisor) AddWorker(identity Identity, worker Worker) error {
    workerCtx := &WorkerContext{
        identity: identity,
        worker:   worker,
        registry: worker.GetRegistry(),  // Store for async execution
    }
}
```

### Benefits of This Integration

1. **Separation of concerns:** States don't manage registry lifecycle
2. **Stateless actions:** Actions don't store registry as field
3. **Testability:** Can inject mock registries per test
4. **Pool compatibility:** Executor can inject different registries per worker

---

## 5. Interface Changes

### 5.1 Action Interface (Breaking Change)

**Current:**
```go
package fsmv2

type Action interface {
    Execute(ctx context.Context) error
    Name() string
}
```

**New:**
```go
package fsmv2

type Action interface {
    Execute(ctx context.Context, registry Registry) error
    Name() string
}
```

**Migration required for ALL actions:**
- pkg/fsmv2/communicator/action/*.go
- All test mock actions
- Any custom actions in other packages

### 5.2 Supervisor Constructor

**Current:**
```go
func NewSupervisor(config Config) *Supervisor
```

**New:**
```go
func NewSupervisor(config Config, executor *ActionExecutor) *Supervisor
```

**Migration required:**
- All supervisor creation sites must pass executor
- Test setups must create and inject executor

### 5.3 WorkerContext Structure

**Current:**
```go
type WorkerContext struct {
    identity   fsmv2.Identity
    worker     fsmv2.Worker
    collector  *Collector
    currentState fsmv2.State
}
```

**New:**
```go
type WorkerContext struct {
    identity     fsmv2.Identity
    worker       fsmv2.Worker
    collector    *Collector
    registry     fsmv2.Registry  // NEW
    currentState fsmv2.State
}
```

---

## 6. Detailed Implementation Plan

### Phase 1: Core Infrastructure (~11-14 hours)

#### 1.1 Action Interface Change
**File:** `pkg/fsmv2/action.go`

**Changes:**
```go
type Action interface {
    // Execute performs the action with access to worker registry.
    // Returns error if action fails (will be retried with backoff).
    Execute(ctx context.Context, registry Registry) error

    // Name returns human-readable action name for logging/metrics.
    Name() string
}
```

**Testing:** Update all existing action tests to pass mock registry.

**Estimated:** 2 hours (including test updates)

#### 1.2 ActionExecutor Core
**File:** `pkg/fsmv2/action_executor.go`

**Key types:**
```go
// ActionExecutor manages async action execution with worker pool.
type ActionExecutor struct {
    config      ActionExecutorConfig
    workerPool  chan struct{}           // Semaphore for pool size
    taskQueue   chan *actionTask        // Unbounded queue
    statusMap   sync.Map                // workerID -> *ActionStatus
    ctx         context.Context
    cancel      context.CancelFunc
    wg          sync.WaitGroup
    logger      *zap.SugaredLogger
}

// ActionExecutorConfig configures the executor.
type ActionExecutorConfig struct {
    PoolSize       int           // Worker pool size (default: 10)
    ActionTimeout  time.Duration // Per-action timeout (default: 30s)
    MaxRetries     int           // Max retry attempts (default: 3)
    InitialBackoff time.Duration // Initial backoff (default: 1s)
    MaxBackoff     time.Duration // Max backoff (default: 30s)
}

// actionTask wraps an action for execution.
type actionTask struct {
    workerID   string
    action     Action
    registry   Registry
    resultChan chan error  // For waiting on completion (testing)
}

// ActionStatus represents observable action state.
type ActionStatus struct {
    InProgress   bool
    Succeeded    bool
    Failed       bool
    ActionName   string
    StartedAt    time.Time
    CompletedAt  time.Time
    ErrorMessage string
    Retries      int
}
```

**Key methods:**
```go
// NewActionExecutor creates executor with config.
func NewActionExecutor(config ActionExecutorConfig, logger *zap.SugaredLogger) *ActionExecutor

// Start launches worker pool goroutines.
func (e *ActionExecutor) Start(ctx context.Context) error

// Stop gracefully shuts down, waits for in-flight actions.
func (e *ActionExecutor) Stop()

// EnqueueAction queues action for async execution.
// Non-blocking. Returns error if executor stopped.
func (e *ActionExecutor) EnqueueAction(workerID string, action Action, registry Registry) error

// HasActionInProgress checks if worker has running action.
func (e *ActionExecutor) HasActionInProgress(workerID string) bool

// GetStatus returns current action status for worker.
func (e *ActionExecutor) GetStatus(workerID string) ActionStatus

// executeWorker runs in pool goroutine.
func (e *ActionExecutor) executeWorker(ctx context.Context)

// executeActionWithRetry handles timeout and retry logic.
func (e *ActionExecutor) executeActionWithRetry(ctx context.Context, task *actionTask)
```

**Estimated:** 6-8 hours

#### 1.3 ActionExecutor Tests
**File:** `pkg/fsmv2/action_executor_test.go`

**Test coverage:**
- Pool size constraint (N concurrent, N+1 queues)
- Task queueing (unbounded queue works)
- Timeout handling (action times out after config.ActionTimeout)
- Retry with exponential backoff (verify timing)
- Status per worker (multiple workers independent)
- Registry injection (action receives correct registry)
- Graceful shutdown (waits for in-flight)
- Concurrent enqueue safety

**Estimated:** 5-6 hours

### Phase 2: Supervisor Integration (~4-5 hours)

#### 2.1 Modify Supervisor Constructor
**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Changes:**
```go
type Supervisor struct {
    // ... existing fields
    actionExecutor *ActionExecutor  // NEW
}

func NewSupervisor(config Config, executor *ActionExecutor) *Supervisor {
    return &Supervisor{
        // ... existing initialization
        actionExecutor: executor,
    }
}
```

**Estimated:** 1 hour

#### 2.2 Modify Tick Loop
**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Changes to `tickWorker()`:**
```go
func (s *Supervisor) tickWorker(ctx context.Context, workerCtx *WorkerContext) {
    // 1. ALWAYS collect fresh observations
    observed, err := workerCtx.collector.GetLatestObservation(ctx)
    if err != nil {
        s.logger.Errorw("Failed to collect observations",
            "worker", workerCtx.identity.ID,
            "error", err)
        return
    }

    // 2. Check if action in progress (NEW)
    if s.actionExecutor.HasActionInProgress(workerCtx.identity.ID) {
        // Skip action derivation, continue to next tick
        s.logger.Debugw("Action in progress, skipping tick",
            "worker", workerCtx.identity.ID)
        return
    }

    // 3. Derive desired state
    desired, err := workerCtx.worker.DeriveDesiredState(nil)

    // 4. Get current state and derive next
    currentState := workerCtx.currentState
    snapshot := createSnapshot(workerCtx.identity, observed, desired)
    nextState, signal, action := currentState.Next(snapshot)

    // 5. Enqueue action if present (NEW)
    if action != nil {
        err := s.actionExecutor.EnqueueAction(
            workerCtx.identity.ID,
            action,
            workerCtx.registry,  // Registry from WorkerContext
        )
        if err != nil {
            s.logger.Errorw("Failed to enqueue action",
                "worker", workerCtx.identity.ID,
                "action", action.Name(),
                "error", err)
        }
    }

    // 6. Update state
    workerCtx.currentState = nextState
    s.handleSignal(ctx, workerCtx, signal)
}
```

**Estimated:** 2-3 hours

#### 2.3 Update WorkerContext
**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Changes:**
```go
type WorkerContext struct {
    identity     fsmv2.Identity
    worker       fsmv2.Worker
    collector    *Collector
    registry     fsmv2.Registry  // NEW
    currentState fsmv2.State
    // ...
}

// In AddWorker():
func (s *Supervisor) AddWorker(identity Identity, worker Worker) error {
    workerCtx := &WorkerContext{
        identity:     identity,
        worker:       worker,
        collector:    NewCollector(...),
        registry:     worker.GetRegistry(),  // NEW
        currentState: worker.GetInitialState(),
    }
    // ...
}
```

**Estimated:** 1 hour

### Phase 3: Action Migration (~4-6 hours)

#### 3.1 Update Communicator Actions
**Files:**
- `pkg/fsmv2/communicator/action/authenticate.go`
- `pkg/fsmv2/communicator/action/sync.go`
- Mock actions in tests

**Example migration:**
```go
// Before:
type AuthenticateAction struct {
    registry     *registry.CommunicatorRegistry  // REMOVE
    relayURL     string
    instanceUUID string
    authToken    string
}

func NewAuthenticateAction(
    reg *registry.CommunicatorRegistry,  // REMOVE
    relayURL string,
    instanceUUID string,
    authToken string,
) *AuthenticateAction {
    return &AuthenticateAction{
        registry:     reg,  // REMOVE
        relayURL:     relayURL,
        instanceUUID: instanceUUID,
        authToken:    authToken,
    }
}

func (a *AuthenticateAction) Execute(ctx context.Context) error {
    transport := a.registry.GetTransport()  // Use stored registry
    // ...
}

// After:
type AuthenticateAction struct {
    // No registry field
    relayURL     string
    instanceUUID string
    authToken    string
}

func NewAuthenticateAction(
    relayURL string,
    instanceUUID string,
    authToken string,
) *AuthenticateAction {
    return &AuthenticateAction{
        relayURL:     relayURL,
        instanceUUID: instanceUUID,
        authToken:    authToken,
    }
}

func (a *AuthenticateAction) Execute(ctx context.Context, registry fsmv2.Registry) error {
    commRegistry := registry.(*CommunicatorRegistry)  // Type assertion
    transport := commRegistry.GetTransport()
    // ...
}
```

**Estimated:** 2-3 hours

#### 3.2 Update State Machines
**Files:**
- `pkg/fsmv2/communicator/state/*.go`

**Example migration:**
```go
// Before:
func (s *StoppedState) Next(snap snapshot.CommunicatorSnapshot) (State, fsmv2.Signal, fsmv2.Action) {
    action := action.NewAuthenticateAction(
        snap.Desired.Registry,  // REMOVE registry from constructor
        snap.Desired.RelayURL,
        snap.Desired.InstanceUUID,
        snap.Desired.AuthToken,
    )
    return &AuthenticatingState{}, fsmv2.SignalNone, action
}

// After:
func (s *StoppedState) Next(snap snapshot.CommunicatorSnapshot) (State, fsmv2.Signal, fsmv2.Action) {
    action := action.NewAuthenticateAction(
        snap.Desired.RelayURL,    // No registry parameter
        snap.Desired.InstanceUUID,
        snap.Desired.AuthToken,
    )
    return &AuthenticatingState{}, fsmv2.SignalNone, action
}
```

**Estimated:** 1-2 hours

#### 3.3 Update Test Mocks
**Files:**
- All `*_test.go` files with mock actions

**Example:**
```go
// Before:
type mockAction struct {
    registry fsmv2.Registry
}

func (m *mockAction) Execute(ctx context.Context) error {
    // ...
}

// After:
type mockAction struct {
    // No registry field
}

func (m *mockAction) Execute(ctx context.Context, registry fsmv2.Registry) error {
    // ...
}
```

**Estimated:** 1 hour

### Phase 4: Integration Testing (~6-8 hours)

#### 4.1 Create Integration Tests
**File:** `pkg/fsmv2/integration_async_test.go`

**Test scenarios:**
```go
var _ = Describe("Async Action Executor Integration", func() {
    var (
        executor   *ActionExecutor
        supervisor *Supervisor
    )

    BeforeEach(func() {
        config := ActionExecutorConfig{PoolSize: 5}
        executor = NewActionExecutor(config, logger)
        executor.Start(ctx)

        supervisor = NewSupervisor(supervisorConfig, executor)
    })

    It("should execute actions asynchronously", func() {
        // Test full lifecycle
    })

    It("should enforce pool size limit", func() {
        // Enqueue N+1 actions, verify N execute, 1 waits
    })

    It("should maintain independent status per worker", func() {
        // Multiple workers with different actions
    })

    It("should inject correct registry per worker", func() {
        // Verify action receives right registry
    })

    It("should handle graceful shutdown", func() {
        // Stop executor, verify in-flight actions complete
    })
})
```

**Estimated:** 4-5 hours

#### 4.2 Update Existing Tests
**Files:**
- `pkg/fsmv2/supervisor/*_test.go`

**Changes:**
- Create ActionExecutor in test setup
- Pass executor to NewSupervisor()
- Update mock actions for registry parameter

**Estimated:** 2-3 hours

### Phase 5: Configuration and Documentation (~3-4 hours)

#### 5.1 Add Configuration Support
**File:** `pkg/fsmv2/action_executor.go`

**Default config:**
```go
var DefaultActionExecutorConfig = ActionExecutorConfig{
    PoolSize:       10,
    ActionTimeout:  30 * time.Second,
    MaxRetries:     3,
    InitialBackoff: 1 * time.Second,
    MaxBackoff:     30 * time.Second,
}
```

**Environment-based config:**
```go
func NewActionExecutorConfigFromEnv() ActionExecutorConfig {
    return ActionExecutorConfig{
        PoolSize:       getEnvInt("ACTION_POOL_SIZE", 10),
        ActionTimeout:  getEnvDuration("ACTION_TIMEOUT", 30*time.Second),
        MaxRetries:     getEnvInt("ACTION_MAX_RETRIES", 3),
        InitialBackoff: getEnvDuration("ACTION_INITIAL_BACKOFF", 1*time.Second),
        MaxBackoff:     getEnvDuration("ACTION_MAX_BACKOFF", 30*time.Second),
    }
}
```

**Estimated:** 1-2 hours

#### 5.2 Update Design Document
**File:** `docs/design/fsmv2-async-action-executor.md`

**Changes:**
- Mark as "Implemented"
- Document global pooled architecture (vs original per-worker design)
- Explain registry injection pattern
- Add configuration examples (edge vs cloud)
- Migration guide for existing actions

**Estimated:** 2 hours

### Phase 6: Control Loop Integration (~3-4 hours)

#### 6.1 Create ActionExecutor in Main
**File:** Control loop entry point (TBD based on architecture)

**Example integration:**
```go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Determine deployment type
    deploymentType := os.Getenv("DEPLOYMENT_TYPE")  // "edge" or "cloud"

    // Configure executor based on deployment
    var executorConfig ActionExecutorConfig
    if deploymentType == "edge" {
        executorConfig = ActionExecutorConfig{
            PoolSize:       10,  // Conservative for edge
            ActionTimeout:  30 * time.Second,
            MaxRetries:     3,
            InitialBackoff: 1 * time.Second,
            MaxBackoff:     30 * time.Second,
        }
    } else {
        executorConfig = ActionExecutorConfig{
            PoolSize:       50,  // Higher for cloud
            ActionTimeout:  60 * time.Second,
            MaxRetries:     5,
            InitialBackoff: 1 * time.Second,
            MaxBackoff:     60 * time.Second,
        }
    }

    // Create global executor
    executor := fsmv2.NewActionExecutor(executorConfig, logger)
    if err := executor.Start(ctx); err != nil {
        logger.Fatalw("Failed to start action executor", "error", err)
    }
    defer executor.Stop()

    // Create supervisors sharing the executor
    commSupervisor := supervisor.NewSupervisor(commConfig, executor)
    benthosSupervisor := supervisor.NewSupervisor(benthosConfig, executor)

    // Start supervisors
    if err := commSupervisor.Start(ctx); err != nil {
        logger.Fatalw("Failed to start communicator supervisor", "error", err)
    }
    if err := benthosSupervisor.Start(ctx); err != nil {
        logger.Fatalw("Failed to start benthos supervisor", "error", err)
    }

    // Wait for shutdown signal
    <-ctx.Done()

    // Graceful shutdown (executor.Stop() waits for in-flight actions)
    logger.Info("Shutting down gracefully...")
}
```

**Estimated:** 1-2 hours

#### 6.2 Production Testing
**Environment:** Real deployment (edge or cloud)

**Verification:**
- Actions execute asynchronously
- Pool size respected
- Fresh observations collected
- Graceful shutdown works
- Metrics/monitoring functional

**Estimated:** 2 hours

---

## 7. Migration Guide

### For Action Developers

**Step 1:** Remove registry from action struct
```go
// Before:
type MyAction struct {
    registry *MyRegistry
    param1   string
}

// After:
type MyAction struct {
    param1 string  // No registry field
}
```

**Step 2:** Remove registry from constructor
```go
// Before:
func NewMyAction(registry *MyRegistry, param1 string) *MyAction

// After:
func NewMyAction(param1 string) *MyAction
```

**Step 3:** Add registry parameter to Execute()
```go
// Before:
func (a *MyAction) Execute(ctx context.Context) error {
    tool := a.registry.GetTool()
    // ...
}

// After:
func (a *MyAction) Execute(ctx context.Context, registry fsmv2.Registry) error {
    myRegistry := registry.(*MyRegistry)  // Type assertion
    tool := myRegistry.GetTool()
    // ...
}
```

### For State Developers

**Step 1:** Remove registry from action constructor calls
```go
// Before:
action := NewMyAction(desired.Registry, param1)

// After:
action := NewMyAction(param1)  // No registry
```

**Step 2:** Remove HasActionInProgress() checks (if any)
```go
// Before:
func (s *MyState) Next(snap Snapshot) (State, Signal, Action) {
    if snap.Observed.ActionStatus.InProgress {
        return s, SignalNone, nil  // REMOVE THIS
    }
    // ...
}

// After:
func (s *MyState) Next(snap Snapshot) (State, Signal, Action) {
    // Supervisor handles action-in-progress automatically
    // ...
}
```

### For Test Writers

**Step 1:** Create ActionExecutor in test setup
```go
var _ = Describe("My Test", func() {
    var executor *fsmv2.ActionExecutor

    BeforeEach(func() {
        config := fsmv2.ActionExecutorConfig{PoolSize: 5}
        executor = fsmv2.NewActionExecutor(config, logger)
        executor.Start(ctx)
    })

    AfterEach(func() {
        executor.Stop()
    })
})
```

**Step 2:** Pass executor to NewSupervisor()
```go
supervisor := supervisor.NewSupervisor(config, executor)
```

**Step 3:** Update mock actions
```go
type mockAction struct{}

func (m *mockAction) Execute(ctx context.Context, registry fsmv2.Registry) error {
    // Add registry parameter
    return nil
}
```

---

## 8. Testing Strategy

### 8.1 Unit Tests

**ActionExecutor Core:**
- Pool size enforcement
- Task queueing (unbounded)
- Timeout handling
- Retry with exponential backoff
- Status updates (InProgress/Succeeded/Failed)
- Registry injection
- Graceful shutdown

**Target coverage:** ≥80%

### 8.2 Integration Tests

**End-to-End Scenarios:**
- Full lifecycle (enqueue → execute → complete)
- Multiple supervisors sharing executor
- Pool exhaustion (N+1 actions)
- Worker isolation (status independent)
- Graceful shutdown (in-flight completion)

**Test file:** `pkg/fsmv2/integration_async_test.go`

### 8.3 Regression Tests

**Existing Tests:**
- All supervisor tests must pass
- All communicator tests must pass
- No behavioral changes to state machines

**Validation:**
```bash
go test ./pkg/fsmv2/supervisor/... -v
go test ./pkg/fsmv2/communicator/... -v
```

### 8.4 Performance Tests

**Benchmarks:**
- Action throughput (actions/second)
- Queue latency (enqueue → execute delay)
- Pool efficiency (utilization %)

**Example:**
```go
func BenchmarkActionThroughput(b *testing.B) {
    executor := NewActionExecutor(config, logger)
    executor.Start(ctx)
    defer executor.Stop()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        executor.EnqueueAction(workerID, action, registry)
    }
}
```

---

## 9. Configuration

### 9.1 Configuration Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `PoolSize` | int | 10 | Number of worker goroutines |
| `ActionTimeout` | duration | 30s | Per-action timeout |
| `MaxRetries` | int | 3 | Max retry attempts |
| `InitialBackoff` | duration | 1s | Initial retry backoff |
| `MaxBackoff` | duration | 30s | Maximum retry backoff |

### 9.2 Environment Variables

```bash
# Edge deployment
export ACTION_POOL_SIZE=10
export ACTION_TIMEOUT=30s
export ACTION_MAX_RETRIES=3

# Cloud deployment
export ACTION_POOL_SIZE=50
export ACTION_TIMEOUT=60s
export ACTION_MAX_RETRIES=5
```

### 9.3 Deployment Recommendations

**Edge Devices:**
- `PoolSize: 10` (conservative)
- `ActionTimeout: 30s`
- `MaxRetries: 3`

**Cloud/Server:**
- `PoolSize: 50` (higher throughput)
- `ActionTimeout: 60s`
- `MaxRetries: 5`

**Testing:**
- `PoolSize: 5` (fast test execution)
- `ActionTimeout: 5s` (quick failure detection)
- `MaxRetries: 1` (minimal retries)

---

## 10. Timeline and Effort

### Phase Breakdown

| Phase | Description | Estimated Hours | Estimated Days |
|-------|-------------|-----------------|----------------|
| **Phase 1** | Action interface + ActionExecutor core + tests | 11-14 | 1.5-2 |
| **Phase 2** | Supervisor integration | 4-5 | 0.5-1 |
| **Phase 3** | Action migration | 4-6 | 0.5-1 |
| **Phase 4** | Integration testing | 6-8 | 1-1.5 |
| **Phase 5** | Configuration + docs | 3-4 | 0.5 |
| **Phase 6** | Control loop integration + production testing | 3-4 | 0.5-1 |
| **Total** | | **31-41** | **5-6** |

### Implementation Order

1. **Phase 1.1:** Action interface change (2h) ← **Must be first (breaking change)**
2. **Phase 1.2:** ActionExecutor core (6-8h)
3. **Phase 1.3:** ActionExecutor tests (5-6h)
4. **Phase 2:** Supervisor integration (4-5h)
5. **Phase 3:** Action migration (4-6h)
6. **Phase 4:** Integration testing (6-8h)
7. **Phase 5:** Configuration + docs (3-4h)
8. **Phase 6:** Control loop integration (3-4h)

---

## 11. Success Criteria

### Functional Requirements

- [ ] Worker pool enforces max concurrent actions
- [ ] Task queue unbounded (no blocking on enqueue)
- [ ] Timeout and retry work correctly
- [ ] Registry injected correctly at execution
- [ ] Multiple supervisors share one executor
- [ ] Status independent per worker
- [ ] Tick loop skips derivation when action running
- [ ] Fresh observations always collected

### Code Quality Requirements

- [ ] All existing tests pass (zero regressions)
- [ ] Test coverage ≥80% for new code
- [ ] No focused tests (`FIt`, `FDescribe`)
- [ ] Passes `golangci-lint run`
- [ ] Passes `go vet ./...`
- [ ] No data races (`go test -race`)

### Documentation Requirements

- [ ] Design doc updated with implementation status
- [ ] Configuration guide provided
- [ ] Migration guide for actions
- [ ] Examples in control loop integration

### Performance Requirements

- [ ] Action throughput ≥100 actions/second (benchmark)
- [ ] Queue latency <10ms p99 (benchmark)
- [ ] Pool utilization >80% under load (benchmark)

---

## 12. Risks and Mitigations

### Risk 1: Breaking Change to Action Interface

**Impact:** All existing actions must be updated in same PR.

**Mitigation:**
- Do interface change first (Phase 1.1)
- Update all actions in Phase 3 before merging
- No partial state (either all old or all new)

**Rollback plan:** Revert entire PR if issues found.

### Risk 2: Pool Exhaustion

**Impact:** Actions queue indefinitely, increasing latency.

**Mitigation:**
- Configurable pool size (tune per deployment)
- Monitoring via `GetStatus()` per worker
- Alerts on high queue depth (future)

**Detection:** Benchmark tests validate pool behavior.

### Risk 3: Registry Injection Complexity

**Impact:** Type assertions in every action.

**Mitigation:**
- Clear migration guide with examples
- Helper functions for common registry types
- Comprehensive tests for injection

**Example helper:**
```go
func GetCommunicatorRegistry(registry fsmv2.Registry) *CommunicatorRegistry {
    commReg, ok := registry.(*CommunicatorRegistry)
    if !ok {
        panic("expected CommunicatorRegistry")  // Fail fast
    }
    return commReg
}
```

### Risk 4: Multiple Supervisors Sharing Executor

**Impact:** Status isolation issues if workerID collisions.

**Mitigation:**
- WorkerID must be globally unique (include supervisor ID)
- Tests validate isolation with multiple supervisors
- Use `fmt.Sprintf("%s-%s", supervisorID, workerID)` for uniqueness

**Validation:** Integration test (Phase 4.1) covers multi-supervisor scenario.

### Risk 5: Graceful Shutdown Race Conditions

**Impact:** In-flight actions may not complete during shutdown.

**Mitigation:**
- `executor.Stop()` blocks until all actions complete
- Context cancellation propagates to actions
- Tests validate shutdown behavior

**Example:**
```go
func (e *ActionExecutor) Stop() {
    e.cancel()       // Cancel context
    e.wg.Wait()      // Wait for all goroutines
    close(e.taskQueue)  // Close task queue
}
```

---

## Appendix A: Code Examples

### A.1 Complete ActionExecutor Implementation

See `pkg/fsmv2/action_executor.go` after Phase 1.2.

### A.2 Supervisor Integration

See `pkg/fsmv2/supervisor/supervisor.go` after Phase 2.

### A.3 Action Migration Example

See Phase 3.1 for complete before/after.

### A.4 Control Loop Integration

See Phase 6.1 for complete main.go example.

---

## Appendix B: Alignment with Other Refactors

### B.1 Registry Pattern (Parallel Refactor)

**Status:** In progress
**Alignment:** Execution-time registry injection (this plan)
**Integration point:** WorkerContext stores registry, passed to EnqueueAction()

### B.2 Collector Pattern (Existing)

**Status:** Implemented
**Alignment:** ActionExecutor mirrors Collector pattern
**Similarity:** Both use goroutines per resource, both provide observable status

### B.3 CSE/Persistence (Future)

**Status:** Planned
**Alignment:** ActionStatus can be stored in CSE for recovery
**Consideration:** Future enhancement, not required for MVP

---

## Appendix C: Open Questions

### Q1: Should ActionExecutor support priority queues?

**Context:** Some actions may be more urgent than others.

**Decision:** No, MVP uses FIFO. Can add priority in future if needed.

### Q2: Should pool size be dynamic (auto-scaling)?

**Context:** Optimal pool size depends on load.

**Decision:** No, configurable static size for MVP. Auto-scaling is future enhancement.

### Q3: Should ActionStatus be persisted to survive restarts?

**Context:** In-flight actions lost on restart.

**Decision:** No, MVP does not persist. Actions are idempotent, retry on restart.

---

**End of Implementation Plan**
