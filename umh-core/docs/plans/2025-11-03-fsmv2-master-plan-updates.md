# FSMv2 Master Plan Updates
**Date**: 2025-11-03
**Author**: AI Agent
**Context**: Updates to master implementation plan based on Gap Analysis and three-perspective evaluation (Technical, Developer, User/UX)

## Overview

This plan addresses improvements to `docs/plans/2025-11-02-fsmv2-supervision-and-async-actions.md` based on:
- **Gap Analysis**: Two gaps identified (Action Cancellation, Observation Collection)
- **Technical Evaluation**: Task 3.2/3.4 need enhanced acceptance criteria and test details
- **Developer Evaluation**: 60% implementation readiness → 90% with clarifications (mock boilerplate, Task 1.4, clock injection)
- **User/UX Evaluation**: Functional but needs visibility improvements (feedback, error distinction, recovery UX)

**Priority Tiers**:
1. **Must Have (Blocking)**: Required before any implementation starts
2. **Should Have (Before Phase 2)**: Required before async action work begins
3. **Nice to Have (Before Production)**: UX improvements for production release

**Total Estimated Time**: 3-4 hours for all updates

---

## Phase 1: Must Have Updates (Blocking)

### Task 1.1: Complete Task 1.4 Implementation (Circuit Breaker Integration)

**File**: `docs/plans/2025-11-02-fsmv2-supervision-and-async-actions.md`
**Lines**: ~220-225 (Task 1.4 section)
**Estimated Time**: 30 minutes

#### RED: Write Failing Test

```go
// pkg/infrastructure/supervisor/circuit_breaker_integration_test.go
package supervisor_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("CircuitBreakerIntegration", func() {
    var (
        supervisor        *supervisor.Supervisor
        mockHealthChecker *mocks.MockHealthChecker
        mockBackoff       *mocks.MockBackoff
        ctx               context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        mockHealthChecker = mocks.NewMockHealthChecker()
        mockBackoff = mocks.NewMockBackoff()
        supervisor = supervisor.NewSupervisor(mockHealthChecker, mockBackoff)
    })

    Context("when infrastructure check fails", func() {
        It("opens circuit and skips tick", func() {
            mockHealthChecker.SetCheckResult(errors.New("Redpanda unreachable"))

            err := supervisor.Tick(ctx)

            Expect(err).ToNot(HaveOccurred())
            Expect(supervisor.IsCircuitOpen()).To(BeTrue())
            Expect(supervisor.GetLastActionType()).To(Equal("ChildRestart"))
        })

        It("applies exponential backoff delay", func() {
            mockHealthChecker.SetCheckResult(errors.New("Redpanda unreachable"))
            mockBackoff.SetNextDelay(2 * time.Second)

            start := time.Now()
            _ = supervisor.Tick(ctx)
            elapsed := time.Since(start)

            Expect(elapsed).To(BeNumerically("~", 2*time.Second, 100*time.Millisecond))
        })
    })

    Context("when infrastructure check succeeds after failure", func() {
        It("closes circuit and resets backoff", func() {
            // First tick: open circuit
            mockHealthChecker.SetCheckResult(errors.New("Redpanda unreachable"))
            _ = supervisor.Tick(ctx)
            Expect(supervisor.IsCircuitOpen()).To(BeTrue())

            // Second tick: success
            mockHealthChecker.SetCheckResult(nil)
            err := supervisor.Tick(ctx)

            Expect(err).ToNot(HaveOccurred())
            Expect(supervisor.IsCircuitOpen()).To(BeFalse())
            Expect(mockBackoff.GetAttempts()).To(Equal(0))
        })
    })

    Context("when circuit is closed", func() {
        It("proceeds with normal tick processing", func() {
            mockHealthChecker.SetCheckResult(nil)

            err := supervisor.Tick(ctx)

            Expect(err).ToNot(HaveOccurred())
            Expect(supervisor.GetLastActionType()).To(Equal("NormalTick"))
        })
    })

    Context("when max retry attempts reached", func() {
        It("keeps circuit open and logs escalation", func() {
            mockHealthChecker.SetCheckResult(errors.New("Redpanda unreachable"))
            mockBackoff.SetMaxAttempts(5)

            // Exhaust all attempts
            for i := 0; i < 5; i++ {
                _ = supervisor.Tick(ctx)
            }

            err := supervisor.Tick(ctx)

            Expect(err).ToNot(HaveOccurred())
            Expect(supervisor.IsCircuitOpen()).To(BeTrue())
            Expect(supervisor.GetLastLogMessage()).To(ContainSubstring("ESCALATION REQUIRED"))
        })
    })
})
```

**Expected Failure**: `cannot find package "supervisor"`

#### GREEN: Minimal Implementation

```go
// pkg/infrastructure/supervisor/supervisor.go
package supervisor

import (
    "context"
    "errors"
    "time"
)

type Supervisor struct {
    healthChecker  HealthChecker
    backoff        ExponentialBackoff
    circuitOpen    bool
    lastActionType string
    lastLogMessage string
}

func NewSupervisor(healthChecker HealthChecker, backoff ExponentialBackoff) *Supervisor {
    return &Supervisor{
        healthChecker:  healthChecker,
        backoff:        backoff,
        circuitOpen:    false,
        lastActionType: "",
        lastLogMessage: "",
    }
}

func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Health Check
    if err := s.healthChecker.Check(ctx); err != nil {
        s.circuitOpen = true
        s.lastActionType = "ChildRestart"

        delay := s.backoff.NextDelay()
        time.Sleep(delay)
        s.backoff.RecordFailure()

        if s.backoff.GetAttempts() >= 5 {
            s.lastLogMessage = "ESCALATION REQUIRED: Infrastructure failure after 5 attempts"
        }

        return nil // Skip rest of tick
    }

    // Infrastructure healthy: close circuit and reset
    s.circuitOpen = false
    s.backoff.Reset()
    s.lastActionType = "NormalTick"

    // PRIORITY 2: Normal tick processing
    // (worker derivation logic would go here)

    return nil
}

func (s *Supervisor) IsCircuitOpen() bool {
    return s.circuitOpen
}

func (s *Supervisor) GetLastActionType() string {
    return s.lastActionType
}

func (s *Supervisor) GetLastLogMessage() string {
    return s.lastLogMessage
}
```

**Expected Pass**: All 5 test cases pass

#### REFACTOR: Add Logging and Metrics

```go
// pkg/infrastructure/supervisor/supervisor.go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Health Check
    if err := s.healthChecker.Check(ctx); err != nil {
        s.circuitOpen = true
        s.lastActionType = "ChildRestart"

        // Emit metrics
        metrics.InfrastructureCheckFailures.Inc()
        metrics.CircuitBreakerOpenGauge.Set(1)

        // Log with context
        log.Warn().
            Err(err).
            Bool("circuit_open", true).
            Int("attempt", s.backoff.GetAttempts()).
            Msg("Infrastructure check failed, circuit opened")

        delay := s.backoff.NextDelay()
        time.Sleep(delay)
        s.backoff.RecordFailure()

        if s.backoff.GetAttempts() >= 5 {
            s.lastLogMessage = "ESCALATION REQUIRED: Infrastructure failure after 5 attempts"
            log.Error().
                Dur("total_downtime", s.backoff.GetTotalDowntime()).
                Msg(s.lastLogMessage)
            metrics.EscalationsTotal.Inc()
        }

        return nil
    }

    // Infrastructure healthy: close circuit and reset
    if s.circuitOpen {
        log.Info().Msg("Infrastructure recovered, circuit closed")
        metrics.CircuitBreakerRecoveries.Inc()
    }

    s.circuitOpen = false
    s.backoff.Reset()
    s.lastActionType = "NormalTick"
    metrics.CircuitBreakerOpenGauge.Set(0)

    // PRIORITY 2: Normal tick processing
    // (worker derivation logic would go here)

    return nil
}
```

**Commit**: `feat(supervisor): implement circuit breaker integration with health checks`

**Update Master Plan Section**:
Replace Task 1.4 (lines 220-225) with full RED-GREEN-REFACTOR example above.

---

### Task 1.2: Add Mock Setup Boilerplate Appendix

**File**: `docs/plans/2025-11-02-fsmv2-supervision-and-async-actions.md`
**Location**: New appendix at end of document
**Estimated Time**: 20 minutes

#### Content to Add

```markdown
---

## Appendix A: Mock Setup Boilerplate

### HealthChecker Mock

```go
// pkg/infrastructure/supervisor/mocks/health_checker.go
package mocks

import "context"

type MockHealthChecker struct {
    checkResult error
    callCount   int
}

func NewMockHealthChecker() *MockHealthChecker {
    return &MockHealthChecker{}
}

func (m *MockHealthChecker) SetCheckResult(err error) {
    m.checkResult = err
}

func (m *MockHealthChecker) Check(ctx context.Context) error {
    m.callCount++
    return m.checkResult
}

func (m *MockHealthChecker) GetCallCount() int {
    return m.callCount
}
```

### ExponentialBackoff Mock

```go
// pkg/infrastructure/supervisor/mocks/backoff.go
package mocks

import "time"

type MockBackoff struct {
    nextDelay   time.Duration
    attempts    int
    maxAttempts int
}

func NewMockBackoff() *MockBackoff {
    return &MockBackoff{
        nextDelay:   1 * time.Second,
        maxAttempts: 5,
    }
}

func (m *MockBackoff) SetNextDelay(d time.Duration) {
    m.nextDelay = d
}

func (m *MockBackoff) SetMaxAttempts(max int) {
    m.maxAttempts = max
}

func (m *MockBackoff) NextDelay() time.Duration {
    return m.nextDelay
}

func (m *MockBackoff) RecordFailure() {
    m.attempts++
}

func (m *MockBackoff) Reset() {
    m.attempts = 0
}

func (m *MockBackoff) GetAttempts() int {
    return m.attempts
}

func (m *MockBackoff) GetTotalDowntime() time.Duration {
    total := time.Duration(0)
    for i := 0; i < m.attempts; i++ {
        total += time.Duration(1<<i) * time.Second
    }
    return total
}
```

### TriangularStore Mock

```go
// pkg/fsm/store/mocks/triangular_store.go
package mocks

import (
    "context"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsm/store"
)

type MockTriangularStore struct {
    snapshots map[string]store.Snapshot
}

func NewMockTriangularStore() *MockTriangularStore {
    return &MockTriangularStore{
        snapshots: make(map[string]store.Snapshot),
    }
}

func (m *MockTriangularStore) SetSnapshot(workerType, workerID string, snap store.Snapshot) {
    key := workerType + ":" + workerID
    m.snapshots[key] = snap
}

func (m *MockTriangularStore) LoadSnapshot(ctx context.Context, workerType, workerID string) (store.Snapshot, error) {
    key := workerType + ":" + workerID
    snap, ok := m.snapshots[key]
    if !ok {
        return store.Snapshot{}, store.ErrNotFound
    }
    return snap, nil
}

func (m *MockTriangularStore) SaveSnapshot(ctx context.Context, workerType, workerID string, snap store.Snapshot) error {
    key := workerType + ":" + workerID
    m.snapshots[key] = snap
    return nil
}
```

### ActionExecutor Mock

```go
// pkg/fsm/executor/mocks/action_executor.go
package mocks

import (
    "github.com/united-manufacturing-hub/umh-core/pkg/fsm/executor"
)

type MockActionExecutor struct {
    actionsInProgress map[string]bool
    enqueuedActions   []executor.Action
}

func NewMockActionExecutor() *MockActionExecutor {
    return &MockActionExecutor{
        actionsInProgress: make(map[string]bool),
        enqueuedActions:   []executor.Action{},
    }
}

func (m *MockActionExecutor) SetActionInProgress(workerID string, inProgress bool) {
    m.actionsInProgress[workerID] = inProgress
}

func (m *MockActionExecutor) HasActionInProgress(workerID string) bool {
    return m.actionsInProgress[workerID]
}

func (m *MockActionExecutor) EnqueueAction(workerID string, action executor.Action, registry executor.ActionRegistry) error {
    m.enqueuedActions = append(m.enqueuedActions, action)
    return nil
}

func (m *MockActionExecutor) GetEnqueuedActions() []executor.Action {
    return m.enqueuedActions
}
```

### Usage Example in Tests

```go
var _ = Describe("Supervisor", func() {
    var (
        supervisor        *supervisor.Supervisor
        mockHealthChecker *mocks.MockHealthChecker
        mockBackoff       *mocks.MockBackoff
        mockStore         *mocks.MockTriangularStore
        mockExecutor      *mocks.MockActionExecutor
        ctx               context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        mockHealthChecker = mocks.NewMockHealthChecker()
        mockBackoff = mocks.NewMockBackoff()
        mockStore = mocks.NewMockTriangularStore()
        mockExecutor = mocks.NewMockActionExecutor()

        supervisor = supervisor.NewSupervisor(
            mockHealthChecker,
            mockBackoff,
            mockStore,
            mockExecutor,
        )
    })

    // Tests go here...
})
```
```

**Commit**: `docs(master-plan): add mock setup boilerplate appendix`

---

### Task 1.3: Define Phase 2 Task Breakdown

**File**: `docs/plans/2025-11-02-fsmv2-supervision-and-async-actions.md`
**Lines**: ~295-315 (Phase 2 section)
**Estimated Time**: 40 minutes

#### RED: No test for this (documentation update)

#### GREEN: Add Detailed Task Breakdown

Replace Phase 2 section with:

```markdown
## Phase 2: Async Action Executor (Week 3-4)

**Goal**: Implement global worker pool for non-blocking action execution

**Reference**: `docs/plans/async-action-executor-implementation.md`

### Task 2.1: Action Registry and Types (30 min)

**File**: `pkg/fsm/executor/action.go`

```go
package executor

import "context"

// Action represents an asynchronous operation
type Action interface {
    Name() string
    Execute(ctx context.Context) error
}

// ActionRegistry maps action names to implementations
type ActionRegistry interface {
    Register(name string, factory ActionFactory)
    Get(name string) (ActionFactory, error)
}

type ActionFactory func() Action

// Built-in action types
type NoOpAction struct{}
type RestartChildAction struct{ ChildName string }
type ReconfigureAction struct{ Config map[string]interface{} }
```

**Test**: Verify action registration and factory creation

### Task 2.2: Global Worker Pool (45 min)

**File**: `pkg/fsm/executor/worker_pool.go`

```go
package executor

import (
    "context"
    "sync"
)

type WorkerPool struct {
    workers     int
    actionQueue chan ActionRequest
    wg          sync.WaitGroup
    mu          sync.RWMutex
    inProgress  map[string]bool
}

type ActionRequest struct {
    WorkerID string
    Action   Action
    DoneChan chan error
}

func NewWorkerPool(workers int) *WorkerPool {
    wp := &WorkerPool{
        workers:     workers,
        actionQueue: make(chan ActionRequest, workers*10),
        inProgress:  make(map[string]bool),
    }
    wp.start()
    return wp
}

func (wp *WorkerPool) start() {
    for i := 0; i < wp.workers; i++ {
        wp.wg.Add(1)
        go wp.worker()
    }
}

func (wp *WorkerPool) worker() {
    defer wp.wg.Done()
    for req := range wp.actionQueue {
        wp.setInProgress(req.WorkerID, true)
        err := req.Action.Execute(context.Background())
        wp.setInProgress(req.WorkerID, false)
        req.DoneChan <- err
    }
}
```

**Test**: Verify concurrent action execution, progress tracking

### Task 2.3: ActionExecutor Interface (30 min)

**File**: `pkg/fsm/executor/action_executor.go`

```go
package executor

type ActionExecutor interface {
    HasActionInProgress(workerID string) bool
    EnqueueAction(workerID string, action Action, registry ActionRegistry) error
    Shutdown(timeout time.Duration) error
}

type DefaultActionExecutor struct {
    pool     *WorkerPool
    registry ActionRegistry
}

func NewActionExecutor(workers int, registry ActionRegistry) *DefaultActionExecutor {
    return &DefaultActionExecutor{
        pool:     NewWorkerPool(workers),
        registry: registry,
    }
}

func (ae *DefaultActionExecutor) HasActionInProgress(workerID string) bool {
    return ae.pool.IsInProgress(workerID)
}

func (ae *DefaultActionExecutor) EnqueueAction(workerID string, action Action, registry ActionRegistry) error {
    doneChan := make(chan error, 1)
    ae.pool.actionQueue <- ActionRequest{
        WorkerID: workerID,
        Action:   action,
        DoneChan: doneChan,
    }
    return nil
}
```

**Test**: Verify enqueueing, progress checks, graceful shutdown

### Task 2.4: Integrate with Tick Loop (20 min)

**File**: `pkg/infrastructure/supervisor/supervisor.go`

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Health
    if err := s.healthChecker.Check(ctx); err != nil {
        // ... (circuit breaker logic from Phase 1)
        return nil
    }

    // PRIORITY 2: Per-Worker Actions
    for workerID, workerCtx := range s.workers {
        // Skip if action in progress
        if s.actionExecutor.HasActionInProgress(workerID) {
            continue
        }

        // Load latest snapshot
        snapshot, _ := s.store.LoadSnapshot(ctx, workerType, workerID)

        // Derive next action
        nextState, _, action := currentState.Next(snapshot)

        // Enqueue action
        s.actionExecutor.EnqueueAction(workerID, action, s.registry)
    }

    return nil
}
```

**Test**: Verify tick loop skips workers with in-progress actions

---

**Acceptance Criteria for Phase 2**:
- [ ] All actions implement Action interface
- [ ] Worker pool executes 5 actions concurrently
- [ ] `HasActionInProgress()` returns true during execution
- [ ] Tick loop skips workers with in-progress actions
- [ ] Graceful shutdown waits for in-flight actions (5s timeout)
- [ ] All tests pass with 80%+ coverage
```

**Commit**: `docs(master-plan): add detailed Phase 2 task breakdown`

---

## Phase 2: Should Have Updates (Before Phase 2)

### Task 2.1: Add Clock Injection Pattern

**File**: `docs/plans/2025-11-02-fsmv2-supervision-and-async-actions.md`
**Location**: After Appendix A (Mock Setup), before Appendix B
**Estimated Time**: 15 minutes

#### Content to Add

```markdown
## Appendix B: Clock Injection for Time-Based Tests

### Problem

Exponential backoff, retry timers, and timeout tests need to manipulate time without using `time.Sleep()` (which makes tests slow and flaky).

### Solution

Use a `Clock` interface with real and mock implementations:

```go
// pkg/infrastructure/clock/clock.go
package clock

import "time"

type Clock interface {
    Now() time.Time
    Sleep(d time.Duration)
    After(d time.Duration) <-chan time.Time
}

// RealClock uses actual time
type RealClock struct{}

func (RealClock) Now() time.Time {
    return time.Now()
}

func (RealClock) Sleep(d time.Duration) {
    time.Sleep(d)
}

func (RealClock) After(d time.Duration) <-chan time.Time {
    return time.After(d)
}

// MockClock allows test control
type MockClock struct {
    now time.Time
}

func NewMockClock(start time.Time) *MockClock {
    return &MockClock{now: start}
}

func (m *MockClock) Now() time.Time {
    return m.now
}

func (m *MockClock) Sleep(d time.Duration) {
    m.now = m.now.Add(d)
}

func (m *MockClock) After(d time.Duration) <-chan time.Time {
    m.now = m.now.Add(d)
    ch := make(chan time.Time, 1)
    ch <- m.now
    return ch
}

func (m *MockClock) Advance(d time.Duration) {
    m.now = m.now.Add(d)
}
```

### Usage in ExponentialBackoff

```go
// pkg/infrastructure/supervisor/backoff.go
type ExponentialBackoff struct {
    baseDelay   time.Duration
    maxDelay    time.Duration
    attempts    int
    lastAttempt time.Time
    clock       clock.Clock
}

func NewExponentialBackoff(baseDelay, maxDelay time.Duration, clk clock.Clock) *ExponentialBackoff {
    return &ExponentialBackoff{
        baseDelay: baseDelay,
        maxDelay:  maxDelay,
        clock:     clk,
    }
}

func (eb *ExponentialBackoff) NextDelay() time.Duration {
    eb.lastAttempt = eb.clock.Now()
    // ... calculation logic
}
```

### Test Example

```go
var _ = Describe("ExponentialBackoff with MockClock", func() {
    It("calculates delay based on clock time", func() {
        mockClock := clock.NewMockClock(time.Now())
        backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second, mockClock)

        // First delay
        delay1 := backoff.NextDelay()
        Expect(delay1).To(Equal(1 * time.Second))

        // Advance clock by 1 second (simulates sleep)
        mockClock.Advance(1 * time.Second)
        backoff.RecordFailure()

        // Second delay (should be 2s)
        delay2 := backoff.NextDelay()
        Expect(delay2).To(Equal(2 * time.Second))

        // Test took <1ms instead of 3 seconds!
    })
})
```

### Benefits

- **Fast Tests**: No actual sleeping, tests run in milliseconds
- **Deterministic**: Exact time control eliminates flakiness
- **Easy Timeouts**: Test 60-second timeouts in <1ms

### Where to Apply

- Task 1.1: ExponentialBackoff tests (replace `time.Sleep()`)
- Task 2.2: Worker pool timeout tests
- Phase 3: Action timeout and cancellation tests
```

**Commit**: `docs(master-plan): add clock injection pattern for time-based tests`

---

### Task 2.2: Add Acceptance Criteria to Gap Analysis Tasks

**File**: `docs/plans/2025-11-02-fsmv2-supervision-and-async-actions.md`
**Lines**: ~130-147 (Gap Analysis section, Tasks 3.2 and 3.4)
**Estimated Time**: 15 minutes

#### GREEN: Update Task 3.2

Replace lines 130-138 with:

```markdown
**Resolution**: Task 3.2 documents this behavior and adds monitoring metrics:

**Task 3.2: Document Action-Child Lifecycle** (20 minutes)

**Acceptance Criteria**:
- [ ] Documentation added to `docs/design/fsmv2-infrastructure-supervision-patterns.md`
- [ ] Section: "Action Behavior During Circuit Breaker" with examples:
  - Example 1: RestartRedpanda action runs 30s, circuit opens at 10s → action continues 20s more
  - Example 2: TimeoutAction set to 60s, if not complete → worker stuck until timeout fires
- [ ] Add metric: `supervisor_actions_during_circuit_total` (counter)
- [ ] Add metric: `supervisor_action_post_circuit_duration_seconds` (histogram)
- [ ] Add test: Verify circuit opens mid-action, action completes, metrics recorded
- [ ] Update Phase 3 pseudocode: Add comment about in-flight action behavior

**Files to Update**:
1. `docs/design/fsmv2-infrastructure-supervision-patterns.md` (add section)
2. `pkg/infrastructure/supervisor/metrics.go` (add 2 metrics)
3. `pkg/infrastructure/supervisor/supervisor_test.go` (add test)
4. `docs/plans/2025-11-02-fsmv2-supervision-and-async-actions.md` (Phase 3 pseudocode comment)
```

#### GREEN: Update Task 3.4

Replace lines 139-147 with:

```markdown
**Resolution**: Task 3.4 clarifies pseudocode and documents architecture:

**Task 3.4: Clarify Collector-Circuit Independence** (15 minutes)

**Acceptance Criteria**:
- [ ] Documentation added to `docs/design/fsmv2-child-observed-state-usage.md`
- [ ] Section: "Collector Independence from Circuit Breaker" with diagram:
  ```
  [Collectors (5s loop)] → [TriangularStore] ← [Tick Loop (when circuit closed)]
         ↑                      ↓
         |                   [Fresh Data]
         |                      ↓
     [Always Running]    [LoadSnapshot() always succeeds]
  ```
- [ ] Clarify: Circuit breaker does NOT pause collectors, only prevents tick from deriving new actions
- [ ] Clarify: When circuit closes, tick resumes with fresh data (no staleness)
- [ ] Update Phase 1 pseudocode: Add comments about collector independence
- [ ] Add test: Verify collector writes continue during circuit open (verify TriangularStore timestamps)

**Files to Update**:
1. `docs/design/fsmv2-child-observed-state-usage.md` (add section with diagram)
2. `docs/plans/2025-11-02-fsmv2-supervision-and-async-actions.md` (Phase 1 pseudocode comments)
3. `pkg/infrastructure/supervisor/supervisor_test.go` (add test)
```

**Commit**: `docs(master-plan): add acceptance criteria to Gap Analysis tasks`

---

### Task 2.3: Add Phase 3 and Phase 4 Structure Outline

**File**: `docs/plans/2025-11-02-fsmv2-supervision-and-async-actions.md`
**Lines**: ~320-380 (Phase 3 and Phase 4 sections)
**Estimated Time**: 30 minutes

#### GREEN: Update Phase 3 Section

Replace Phase 3 section with:

```markdown
## Phase 3: Integration & Edge Cases (Week 5)

**Goal**: Ensure Circuit Breaker and Async Executor work correctly together

**Reference**: Gap Analysis (lines 97-147)

### Task 3.1: Layered Precedence Implementation (20 min)

**File**: `pkg/infrastructure/supervisor/supervisor.go`

Verify implementation from Phase 1 Task 1.4 correctly prioritizes:
1. Infrastructure checks (circuit breaker) FIRST
2. Action checks (`HasActionInProgress()`) SECOND

**Test**: When both conditions true (infrastructure fail + action in progress), circuit opens (infrastructure wins).

### Task 3.2: Document Action-Child Lifecycle (20 min)

[Content from Task 2.2 above - already defined]

### Task 3.3: Test Circuit Breaker + Action Executor Interaction (30 min)

**File**: `pkg/infrastructure/supervisor/integration_test.go`

**Scenario 1**: Circuit opens while action in progress
```go
It("allows in-progress action to complete after circuit opens", func() {
    // Enqueue long-running action (30s simulated)
    mockExecutor.EnqueueAction("worker-1", slowAction, registry)

    // After 10s (simulated), infrastructure fails
    mockClock.Advance(10 * time.Second)
    mockHealthChecker.SetCheckResult(errors.New("Redpanda down"))

    // Tick: circuit opens
    _ = supervisor.Tick(ctx)
    Expect(supervisor.IsCircuitOpen()).To(BeTrue())

    // Action still in progress
    Expect(mockExecutor.HasActionInProgress("worker-1")).To(BeTrue())

    // After another 20s, action completes
    mockClock.Advance(20 * time.Second)
    Expect(mockExecutor.HasActionInProgress("worker-1")).To(BeFalse())

    // Verify metrics
    Expect(metrics.ActionsDuringCircuitTotal.Value()).To(Equal(1))
})
```

**Scenario 2**: Circuit closes, action queue drains
```go
It("resumes normal derivation after circuit closes", func() {
    // Circuit opens
    mockHealthChecker.SetCheckResult(errors.New("Redpanda down"))
    _ = supervisor.Tick(ctx)

    // Circuit closes
    mockHealthChecker.SetCheckResult(nil)
    _ = supervisor.Tick(ctx)

    // Fresh snapshot from TriangularStore (written by collectors)
    snapshot, _ := mockStore.LoadSnapshot(ctx, "dfc_read", "worker-1")
    Expect(snapshot.Timestamp).To(BeTemporally("~", mockClock.Now(), 100*time.Millisecond))

    // New actions enqueued
    Expect(mockExecutor.GetEnqueuedActions()).To(HaveLen(1))
})
```

### Task 3.4: Clarify Collector-Circuit Independence (15 min)

[Content from Task 2.2 above - already defined]

---

**Acceptance Criteria for Phase 3**:
- [ ] Layered precedence verified (infrastructure > action)
- [ ] Documentation complete (action lifecycle, collector independence)
- [ ] Integration tests pass (2 scenarios)
- [ ] Metrics recorded correctly
- [ ] No race conditions or deadlocks
```

#### GREEN: Update Phase 4 Section

Replace Phase 4 section with:

```markdown
## Phase 4: Monitoring & Observability (Week 6)

**Goal**: Give operators visibility into infrastructure supervision and action execution

### Task 4.1: Core Metrics (30 min)

**File**: `pkg/infrastructure/supervisor/metrics.go`

```go
package supervisor

import "github.com/prometheus/client_golang/prometheus"

var (
    // Circuit Breaker Metrics
    CircuitBreakerOpenGauge = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "supervisor_circuit_breaker_open",
        Help: "1 if circuit is open, 0 if closed",
    })

    InfrastructureCheckFailures = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "supervisor_infrastructure_check_failures_total",
        Help: "Total infrastructure health check failures",
    })

    CircuitBreakerRecoveries = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "supervisor_circuit_breaker_recoveries_total",
        Help: "Total successful circuit closes after failure",
    })

    EscalationsTotal = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "supervisor_escalations_total",
        Help: "Total escalations (max retry attempts exceeded)",
    })

    // Action Executor Metrics
    ActionsEnqueuedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "supervisor_actions_enqueued_total",
        Help: "Total actions enqueued",
    }, []string{"worker_id", "action_type"})

    ActionsCompletedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "supervisor_actions_completed_total",
        Help: "Total actions completed",
    }, []string{"worker_id", "action_type", "status"})

    ActionDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "supervisor_action_duration_seconds",
        Help:    "Action execution duration",
        Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60},
    }, []string{"worker_id", "action_type"})

    // Edge Case Metrics (from Gap Analysis)
    ActionsDuringCircuitTotal = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "supervisor_actions_during_circuit_total",
        Help: "Actions that completed after circuit opened",
    })

    ActionPostCircuitDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name:    "supervisor_action_post_circuit_duration_seconds",
        Help:    "Duration actions ran after circuit opened",
        Buckets: []float64{1, 5, 10, 30, 60},
    })
)

func init() {
    prometheus.MustRegister(
        CircuitBreakerOpenGauge,
        InfrastructureCheckFailures,
        CircuitBreakerRecoveries,
        EscalationsTotal,
        ActionsEnqueuedTotal,
        ActionsCompletedTotal,
        ActionDurationSeconds,
        ActionsDuringCircuitTotal,
        ActionPostCircuitDuration,
    )
}
```

**Test**: Verify all metrics registered, emit values correctly

### Task 4.2: Structured Logging (20 min)

**File**: `pkg/infrastructure/supervisor/supervisor.go`

Add log statements at key points:

```go
// Circuit opens
log.Warn().
    Err(err).
    Str("child_name", childName).
    Int("retry_attempt", backoff.GetAttempts()).
    Dur("backoff_delay", delay).
    Msg("Infrastructure check failed, opening circuit breaker")

// Circuit closes
log.Info().
    Str("child_name", childName).
    Dur("total_downtime", backoff.GetTotalDowntime()).
    Msg("Infrastructure recovered, closing circuit breaker")

// Action enqueued
log.Debug().
    Str("worker_id", workerID).
    Str("action_type", action.Name()).
    Msg("Action enqueued for execution")

// Action completed
log.Info().
    Str("worker_id", workerID).
    Str("action_type", action.Name()).
    Dur("duration", elapsed).
    Bool("success", err == nil).
    Msg("Action completed")

// Escalation
log.Error().
    Str("child_name", childName).
    Int("max_attempts", 5).
    Dur("total_downtime", backoff.GetTotalDowntime()).
    Msg("ESCALATION REQUIRED: Infrastructure failure after max retry attempts. Manual intervention needed.")
```

**Test**: Verify log output in integration tests (capture stderr)

### Task 4.3: Dashboard Configuration (15 min)

**File**: `deployments/grafana/dashboards/supervisor.json`

Create Grafana dashboard with 4 panels:

1. **Circuit Breaker Status** (gauge): `supervisor_circuit_breaker_open`
2. **Infrastructure Failures** (graph): `rate(supervisor_infrastructure_check_failures_total[5m])`
3. **Action Queue Depth** (graph): `supervisor_actions_enqueued_total - supervisor_actions_completed_total`
4. **Action Duration p95** (graph): `histogram_quantile(0.95, supervisor_action_duration_seconds)`

**Test**: Load dashboard in Grafana, verify queries work

### Task 4.4: Alerting Rules (15 min)

**File**: `deployments/prometheus/alerts/supervisor.yml`

```yaml
groups:
  - name: supervisor
    rules:
      - alert: CircuitBreakerStuckOpen
        expr: supervisor_circuit_breaker_open == 1
        for: 5m
        annotations:
          summary: "Circuit breaker stuck open for 5+ minutes"
          description: "Infrastructure checks failing, child restarts not resolving issue"

      - alert: FrequentInfrastructureFailures
        expr: rate(supervisor_infrastructure_check_failures_total[5m]) > 0.1
        for: 2m
        annotations:
          summary: "Frequent infrastructure check failures"
          description: "More than 0.1 failures/sec over 2 minutes"

      - alert: EscalationTriggered
        expr: increase(supervisor_escalations_total[1m]) > 0
        annotations:
          summary: "Infrastructure escalation triggered"
          description: "Max retry attempts exceeded, manual intervention required"
```

**Test**: Trigger alert conditions in test environment, verify Alertmanager receives

---

**Acceptance Criteria for Phase 4**:
- [ ] All metrics emit correctly
- [ ] Logs structured and parseable
- [ ] Grafana dashboard loads and displays data
- [ ] Alerts trigger on failure conditions
- [ ] Operators can diagnose issues from logs/metrics alone (no code inspection)
```

**Commit**: `docs(master-plan): add Phase 3 and Phase 4 structure with tasks`

---

## Phase 3: Nice to Have Updates (Before Production)

These are UX improvements that enhance production operations but are not blocking for initial implementation.

### Task 3.1: Add Recovery Progress Feedback

**File**: `pkg/infrastructure/supervisor/supervisor.go`
**Estimated Time**: 20 minutes
**Priority**: High (operator frustration reducer)

#### GREEN: Add Heartbeat Logging

```go
// pkg/infrastructure/supervisor/supervisor.go
func (s *Supervisor) Tick(ctx context.Context) error {
    if s.circuitOpen {
        // Emit heartbeat log every tick during circuit open
        log.Warn().
            Int("retry_attempt", s.backoff.GetAttempts()).
            Int("max_attempts", 5).
            Dur("elapsed_downtime", s.backoff.GetTotalDowntime()).
            Dur("next_retry_in", s.backoff.NextDelay()).
            Str("recovery_status", s.getRecoveryStatus()).
            Msg("Circuit breaker open, retrying infrastructure checks")
    }

    // ... rest of tick logic
}

func (s *Supervisor) getRecoveryStatus() string {
    attempts := s.backoff.GetAttempts()
    if attempts < 3 {
        return "attempting_recovery"
    } else if attempts < 5 {
        return "persistent_failure"
    } else {
        return "escalation_imminent"
    }
}
```

**Test**: Verify heartbeat log emitted every tick when circuit open

**Benefit**: Operators see progress instead of silent failures

---

### Task 3.2: Add Infrastructure vs Worker Error Distinction

**File**: `pkg/infrastructure/supervisor/supervisor.go`
**Estimated Time**: 15 minutes
**Priority**: High (reduces diagnostic time)

#### GREEN: Add Error Context to Logs

```go
// pkg/infrastructure/supervisor/supervisor.go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Check
    if err := s.healthChecker.Check(ctx); err != nil {
        log.Warn().
            Err(err).
            Str("error_scope", "infrastructure").  // ← NEW
            Str("impact", "all_workers").            // ← NEW
            Str("child_name", childName).
            Msg("Infrastructure check failed")
        // ... circuit breaker logic
    }

    // PRIORITY 2: Per-Worker Actions
    for workerID := range s.workers {
        if s.actionExecutor.HasActionInProgress(workerID) {
            log.Debug().
                Str("worker_id", workerID).
                Str("error_scope", "worker").        // ← NEW
                Str("impact", "single_worker").      // ← NEW
                Msg("Worker action in progress, skipping derivation")
            continue
        }
    }
}
```

**Test**: Verify `error_scope` field present in logs

**Benefit**: Operators immediately know if issue affects entire system or single worker

---

### Task 3.3: Add Staleness Metadata API

**File**: `pkg/fsm/store/triangular_store.go`
**Estimated Time**: 25 minutes
**Priority**: Medium (developer convenience)

#### GREEN: Add Staleness Metadata to Snapshot

```go
// pkg/fsm/store/triangular_store.go
type Snapshot struct {
    WorkerID      string
    Timestamp     time.Time
    Observations  map[string]interface{}
    Metadata      SnapshotMetadata  // ← NEW
}

type SnapshotMetadata struct {
    CollectedAt      time.Time
    LoadedAt         time.Time
    CircuitOpenedAt  *time.Time  // nil if circuit never opened
    AgeSeconds       float64
}

func (ts *TriangularStore) LoadSnapshot(ctx context.Context, workerType, workerID string) (Snapshot, error) {
    snap, err := ts.load(ctx, workerType, workerID)
    if err != nil {
        return Snapshot{}, err
    }

    now := time.Now()
    snap.Metadata.LoadedAt = now
    snap.Metadata.AgeSeconds = now.Sub(snap.Metadata.CollectedAt).Seconds()

    // If circuit was open, record when it opened
    if ts.circuitOpenedAt != nil {
        snap.Metadata.CircuitOpenedAt = ts.circuitOpenedAt
    }

    return snap, nil
}
```

**Test**: Verify metadata populated correctly

**Benefit**: Developers can check `snapshot.Metadata.AgeSeconds` to debug staleness issues

---

### Task 3.4: Add Pre-Escalation Warning

**File**: `pkg/infrastructure/supervisor/supervisor.go`
**Estimated Time**: 10 minutes
**Priority**: Medium (graceful degradation)

#### GREEN: Add Warning at Attempt 4

```go
// pkg/infrastructure/supervisor/supervisor.go
func (s *Supervisor) Tick(ctx context.Context) error {
    if s.circuitOpen {
        attempts := s.backoff.GetAttempts()

        // Warn at attempt 4 (before escalation at 5)
        if attempts == 4 {
            log.Warn().
                Str("child_name", childName).
                Int("attempts_remaining", 1).
                Dur("total_downtime", s.backoff.GetTotalDowntime()).
                Msg("WARNING: One retry attempt remaining before escalation")
        }

        if attempts >= 5 {
            log.Error().
                Str("child_name", childName).
                Msg("ESCALATION REQUIRED: Max retry attempts exceeded")
        }
    }
}
```

**Test**: Verify warning log at attempt 4

**Benefit**: Operators get early warning before escalation

---

### Task 3.5: Embed Escalation Runbook in Logs

**File**: `pkg/infrastructure/supervisor/supervisor.go`
**Estimated Time**: 10 minutes
**Priority**: Low (reduces MTTR)

#### GREEN: Add Runbook to Escalation Log

```go
// pkg/infrastructure/supervisor/supervisor.go
func (s *Supervisor) Tick(ctx context.Context) error {
    if s.backoff.GetAttempts() >= 5 {
        log.Error().
            Str("child_name", childName).
            Dur("total_downtime", s.backoff.GetTotalDowntime()).
            Str("runbook_url", "https://docs.umh.app/runbooks/supervisor-escalation").
            Str("manual_steps", s.getEscalationSteps(childName)).
            Msg("ESCALATION REQUIRED: Infrastructure failure after max retry attempts")
    }
}

func (s *Supervisor) getEscalationSteps(childName string) string {
    steps := map[string]string{
        "dfc_read": "1) Check Redpanda logs 2) Verify network connectivity 3) Restart Redpanda manually",
        "benthos":  "1) Check Benthos logs 2) Verify OPC UA server reachable 3) Restart Benthos manually",
    }
    return steps[childName]
}
```

**Test**: Verify runbook URL and steps in escalation log

**Benefit**: Operators have immediate action items without searching docs

---

## Summary

**Must Have (Blocking)**: 3 tasks, ~1.5 hours
**Should Have (Before Phase 2)**: 3 tasks, ~1 hour
**Nice to Have (Before Production)**: 5 tasks, ~1.5 hours

**Total Time**: 3-4 hours for all updates

**Priority Order**:
1. Task 1.1: Complete Task 1.4 (Circuit Breaker Integration) ← **Start here**
2. Task 1.2: Add Mock Setup Boilerplate Appendix
3. Task 1.3: Define Phase 2 Task Breakdown
4. Task 2.1: Add Clock Injection Pattern
5. Task 2.2: Add Acceptance Criteria to Gap Analysis Tasks
6. Task 2.3: Add Phase 3 and Phase 4 Structure Outline
7. Task 3.1-3.5: UX improvements (production release)

**Next Steps**:
1. Create subagent to implement Must Have updates (Tasks 1.1-1.3)
2. Review updated master plan
3. Create subagent to implement Should Have updates (Tasks 2.1-2.3)
4. Schedule UX improvements for production release

---

## References

- **Original Master Plan**: `docs/plans/2025-11-02-fsmv2-supervision-and-async-actions.md`
- **Gap Analysis**: Lines 97-147 in master plan
- **Technical Evaluation**: Subagent report (2025-11-03)
- **Developer Evaluation**: Subagent report (2025-11-03)
- **User/UX Evaluation**: Subagent report (2025-11-03)
- **Consolidated Recommendations**: Summary from all three evaluations
