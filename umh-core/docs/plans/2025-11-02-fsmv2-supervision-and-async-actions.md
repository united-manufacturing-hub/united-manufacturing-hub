# FSMv2 Infrastructure Supervision & Async Actions - Master Implementation Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/executing-plans/SKILL.md` to implement this plan task-by-task.

**Goal:** Implement FSMv2 infrastructure supervision with circuit breaker pattern and async action executor with global worker pool, maintaining clean separation between infrastructure and business logic layers.

**Architecture:** Two-layer approach with Infrastructure Supervision (Pattern 1: Circuit Breaker) handling child consistency checks and restarts, and Async Action Executor providing non-blocking action execution. Infrastructure checks run first (affect all workers), action checks run second (affect individual workers).

**Tech Stack:** Go 1.21+, Ginkgo v2 testing framework, exponential backoff from existing FSMv2 patterns, TriangularStore for state persistence

**Created:** 2025-11-02 15:00

**Last Updated:** 2025-11-02 15:00

---

## Design Documents Reference

This implementation integrates three design documents:

1. **`docs/design/fsmv2-infrastructure-supervision-patterns.md`**
   - Pattern 1 (Circuit Breaker with Restart) recommended
   - Supervisor detects infrastructure issues, stops ticking, restarts children
   - Exponential backoff prevents infinite loops
   - Escalates to parent after max attempts

2. **`docs/design/fsmv2-developer-expectations-child-state.md`**
   - Developers expect simple API: `GetChildState("connection")` returns string
   - Infrastructure completely invisible to business logic
   - No error handling needed in parent FSM
   - Stale state acceptable (last known state returned)

3. **`docs/design/fsmv2-child-observed-state-usage.md`**
   - FSMv1 parents access child metrics for sanity checks
   - Example: Redpanda="active" but BenthosOutputConnections=0
   - Recommendation: Move sanity checks to supervisor infrastructure layer
   - Parents should NOT access child observed state in business logic

4. **`docs/plans/async-action-executor-implementation.md`**
   - Global ActionExecutor instance shared across supervisors
   - Non-blocking tick loop (actions execute asynchronously)
   - Supervisor checks `HasActionInProgress()` and skips derivation
   - Registry injected at execution time

## Compatibility Analysis Summary

**Verdict:** ✅ COMPATIBLE - Both patterns can coexist with clear precedence

**Key Integration Principles:**

1. **Layered Precedence:** Infrastructure checks run FIRST (affect all workers), action checks run SECOND (affect individual workers)

2. **Different Scopes:**
   - Infrastructure: Affects ALL workers (circuit breaker stops everything)
   - Actions: Affects SINGLE worker (just that worker pauses)

3. **Separation Maintained:**
   - Infrastructure failures invisible to business logic ✅
   - Parent FSM only sees state transitions ✅
   - Both maintain "infrastructure invisible" principle ✅

**Integration Strategy:**

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // NOTE: Collectors run independently in separate goroutines.
    // They continuously write observations to TriangularStore every second.
    // Circuit breaker does NOT pause collectors, only prevents tick from acting on observations.

    // PRIORITY 1: Infrastructure Health (affects all workers)
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.restartChild(ctx, "dfc_read")
        time.Sleep(backoff)
        return nil  // Skip entire tick (but collectors keep running)
    }

    // PRIORITY 2: Per-Worker Actions (affects single worker)
    for workerID, workerCtx := range s.workers {
        // Load latest observation from TriangularStore
        // (written by collector goroutine, always fresh even after circuit open)
        snapshot, _ := s.store.LoadSnapshot(ctx, workerType, workerID)

        if s.actionExecutor.HasActionInProgress(workerID) {
            continue  // Skip derivation for this worker
        }

        // Derive and enqueue action
        nextState, _, action := currentState.Next(snapshot)
        s.actionExecutor.EnqueueAction(workerID, action, registry)
    }
}
```

---

## Gap Analysis & Resolution

Two minor gaps were identified during plan alignment review. Both are **edge cases** already handled by the existing architecture and require only documentation/clarification, not code changes.

### Gap 1: Action Cancellation on Child Restart

**Issue:** When Infrastructure Supervision detects child inconsistency and restarts a child, in-progress actions for workers depending on that child are not explicitly cancelled.

**Current Behavior:**
- Infrastructure restarts child (e.g., Redpanda)
- In-progress actions continue execution
- Actions fail with connection errors
- Actions timeout after 30s (ActionExecutorConfig.ActionTimeout)
- Failed actions retry with exponential backoff (1s, 2s, 4s...)
- Actions succeed once child recovers
- Total recovery time: ~60s

**Decision:** Accept timeout/retry behavior (do not implement cancellation)

**Rationale:**
- Child restarts are rare edge cases (infrastructure failures, not normal operation)
- Implementing cancellation requires dependency tracking (which workers depend on which children) - significant scope creep
- Existing retry mechanism already handles recovery
- Can revisit in Phase 4 if production data shows frequent child restarts

**Resolution:** Task 3.2 documents this behavior and adds monitoring metrics:

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

### Gap 2: Observation Collection During Circuit Open

**Issue:** Master plan pseudocode suggests observations are collected inside the per-worker loop, which doesn't execute when circuit is open. This creates ambiguity about whether observations continue during infrastructure failures.

**Current Behavior:**
- Collectors run as **independent goroutines** (1 per worker)
- Collectors call `worker.CollectObservedState()` every second
- Observations written to TriangularStore continuously
- Circuit breaker does NOT pause collectors
- Tick loop reads from TriangularStore, not collectors directly
- When circuit closes, tick loop has fresh observations immediately

**Decision:** Keep current behavior (observations continue during circuit open)

**Rationale:**
- Fresh data available immediately when circuit closes (faster recovery)
- Continuous monitoring valuable even during failures (observability)
- Observations are cheap (1-second intervals, minimal overhead)
- Collectors already work this way (independent goroutines)
- Master plan pseudocode was misleading (showed collection in tick loop)

**Resolution:** Task 3.4 clarifies pseudocode and documents architecture:

**Task 3.4: Clarify Collector-Circuit Independence** (15 minutes)

**Acceptance Criteria**:
- [ ] Documentation added to `docs/design/fsmv2-child-observed-state-usage.md`
- [ ] Section: "Collector Independence from Circuit Breaker" with diagram:
  ```
  [Collectors (1s loop)] → [TriangularStore] ← [Tick Loop (when circuit closed)]
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

---

## Implementation Phases

### Phase 1: Infrastructure Supervision Foundation (Week 1-2)

**Why First:** More foundational, establishes circuit breaker pattern, easier to test in isolation

**Deliverables:**
- InfrastructureHealthChecker with child consistency checks
- Circuit breaker logic in Supervisor.Tick()
- Child restart with exponential backoff
- Sanity check examples (Redpanda vs Benthos)

### Phase 2: Async Action Executor (Week 3-4)

**Why Second:** Builds on existing tick loop structure, integrates with infrastructure layer

**Deliverables:**
- ActionExecutor with global worker pool
- Integration with Supervisor.tickWorker()
- Action queueing and timeout handling
- Non-blocking action execution

### Phase 3: Integration & Edge Cases (Week 5)

**Why Third:** Tests both systems together, handles complex scenarios

**Deliverables:**
- Combined tick loop with both checks
- Action behavior documentation during child restart (Task 3.2)
- Observation collection clarification during circuit open (Task 3.4)
- Edge case testing (both conditions true simultaneously)

### Phase 4: Monitoring & Observability (Week 6)

**Why Fourth:** Provides visibility into both systems for operations

**Deliverables:**
- Prometheus metrics for infrastructure recovery
- Prometheus metrics for action execution
- Detailed logging for debugging
- Runbook for manual intervention

---

## Phase 1: Infrastructure Supervision Foundation

### Task 1.1: ExponentialBackoff Implementation

**Goal:** Implement reusable exponential backoff with max attempts tracking

**Files:**
- Create: `pkg/fsmv2/supervisor/backoff.go`
- Create: `pkg/fsmv2/supervisor/backoff_test.go`

**Step 1: Write the failing test**

```go
// pkg/fsmv2/supervisor/backoff_test.go
package supervisor_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "time"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("ExponentialBackoff", func() {
    Describe("NextDelay", func() {
        It("returns base delay on first attempt", func() {
            backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)

            delay := backoff.NextDelay()

            Expect(delay).To(Equal(1 * time.Second))
        })

        It("doubles delay on each attempt", func() {
            backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)
            backoff.RecordFailure()

            delay := backoff.NextDelay()

            Expect(delay).To(Equal(2 * time.Second))
        })

        It("caps delay at max", func() {
            backoff := supervisor.NewExponentialBackoff(1*time.Second, 5*time.Second)
            // Record 10 failures (2^10 = 1024 seconds)
            for i := 0; i < 10; i++ {
                backoff.RecordFailure()
            }

            delay := backoff.NextDelay()

            Expect(delay).To(Equal(5 * time.Second))
        })
    })

    Describe("RecordFailure", func() {
        It("increments attempts counter", func() {
            backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)

            backoff.RecordFailure()
            backoff.RecordFailure()

            Expect(backoff.Attempts()).To(Equal(2))
        })

        It("records timestamp of last attempt", func() {
            backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)
            before := time.Now()

            backoff.RecordFailure()

            after := time.Now()
            Expect(backoff.LastAttempt()).To(BeTemporally(">=", before))
            Expect(backoff.LastAttempt()).To(BeTemporally("<=", after))
        })
    })

    Describe("Reset", func() {
        It("resets attempts to zero", func() {
            backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)
            backoff.RecordFailure()
            backoff.RecordFailure()

            backoff.Reset()

            Expect(backoff.Attempts()).To(Equal(0))
        })

        It("resets delay to base", func() {
            backoff := supervisor.NewExponentialBackoff(1*time.Second, 60*time.Second)
            backoff.RecordFailure()
            backoff.RecordFailure()

            backoff.Reset()
            delay := backoff.NextDelay()

            Expect(delay).To(Equal(1 * time.Second))
        })
    })
})
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
ginkgo -v pkg/fsmv2/supervisor
```

Expected: FAIL with "undefined: supervisor.NewExponentialBackoff"

**Step 3: Write minimal implementation**

```go
// pkg/fsmv2/supervisor/backoff.go
package supervisor

import (
    "math"
    "time"
)

// ExponentialBackoff implements exponential backoff with max delay cap
type ExponentialBackoff struct {
    baseDelay   time.Duration
    maxDelay    time.Duration
    attempts    int
    lastAttempt time.Time
}

// NewExponentialBackoff creates a new backoff instance
func NewExponentialBackoff(baseDelay, maxDelay time.Duration) *ExponentialBackoff {
    return &ExponentialBackoff{
        baseDelay: baseDelay,
        maxDelay:  maxDelay,
        attempts:  0,
    }
}

// NextDelay returns the delay for the current attempt
func (eb *ExponentialBackoff) NextDelay() time.Duration {
    if eb.attempts == 0 {
        return eb.baseDelay
    }

    delay := time.Duration(math.Pow(2, float64(eb.attempts))) * eb.baseDelay
    if delay > eb.maxDelay {
        delay = eb.maxDelay
    }

    return delay
}

// RecordFailure increments attempts counter and records timestamp
func (eb *ExponentialBackoff) RecordFailure() {
    eb.attempts++
    eb.lastAttempt = time.Now()
}

// Reset resets backoff to initial state
func (eb *ExponentialBackoff) Reset() {
    eb.attempts = 0
}

// Attempts returns current attempt count
func (eb *ExponentialBackoff) Attempts() int {
    return eb.attempts
}

// LastAttempt returns timestamp of last failure
func (eb *ExponentialBackoff) LastAttempt() time.Time {
    return eb.lastAttempt
}
```

**Step 4: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: PASS (all tests green)

**Step 5: Commit**

```bash
git add pkg/fsmv2/supervisor/backoff.go pkg/fsmv2/supervisor/backoff_test.go
git commit -m "feat(fsmv2): add ExponentialBackoff for infrastructure recovery

Implements exponential backoff with:
- Configurable base and max delays
- Attempt tracking with timestamps
- Reset capability for successful recovery

Part of Infrastructure Supervision (Phase 1)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

**Review Checkpoint:** Dispatch code-reviewer subagent to verify:
- Backoff algorithm correctness
- Test coverage of edge cases (max delay cap, reset behavior)
- Thread safety considerations (if needed)

---

### Task 1.2: InfrastructureHealthChecker Structure

**Goal:** Create health checker structure with child consistency check interface

**Files:**
- Create: `pkg/fsmv2/supervisor/infrastructure_health.go`
- Create: `pkg/fsmv2/supervisor/infrastructure_health_test.go`

**Step 1: Write the failing test**

```go
// pkg/fsmv2/supervisor/infrastructure_health_test.go
package supervisor_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "time"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("InfrastructureHealthChecker", func() {
    var (
        checker *supervisor.InfrastructureHealthChecker
        sup     *supervisor.Supervisor // Mock or test double
    )

    BeforeEach(func() {
        // Setup mock supervisor
        checker = supervisor.NewInfrastructureHealthChecker(sup, 5, 5*time.Minute)
    })

    Describe("NewInfrastructureHealthChecker", func() {
        It("initializes with correct max attempts", func() {
            Expect(checker.MaxAttempts()).To(Equal(5))
        })

        It("initializes backoff with 1s base delay", func() {
            backoff := checker.Backoff()
            Expect(backoff.NextDelay()).To(Equal(1 * time.Second))
        })
    })

    Describe("ShouldGiveUp", func() {
        It("returns false when attempts below max", func() {
            checker.Backoff().RecordFailure()
            checker.Backoff().RecordFailure()

            Expect(checker.ShouldGiveUp()).To(BeFalse())
        })

        It("returns true when attempts reach max", func() {
            for i := 0; i < 5; i++ {
                checker.Backoff().RecordFailure()
            }

            Expect(checker.ShouldGiveUp()).To(BeTrue())
        })
    })

    Describe("RecordFailure", func() {
        It("resets window if last attempt was too long ago", func() {
            checker.Backoff().RecordFailure()

            // Simulate 10 minutes passing (beyond 5 min window)
            time.Sleep(10 * time.Millisecond) // Simulated

            checker.RecordFailure(10 * time.Minute) // Pass simulated time

            // Backoff should reset (new window)
            Expect(checker.Backoff().Attempts()).To(Equal(1))
        })
    })
})
```

**Step 2: Run test to verify it fails**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: FAIL with "undefined: supervisor.NewInfrastructureHealthChecker"

**Step 3: Write minimal implementation**

```go
// pkg/fsmv2/supervisor/infrastructure_health.go
package supervisor

import (
    "time"

    "go.uber.org/zap"
)

const (
    // DefaultMaxInfraRecoveryAttempts is the default max attempts before escalation
    DefaultMaxInfraRecoveryAttempts = 5

    // DefaultRecoveryAttemptWindow is the time window for attempt counting
    DefaultRecoveryAttemptWindow = 5 * time.Minute
)

// InfrastructureHealthChecker monitors child infrastructure health
type InfrastructureHealthChecker struct {
    supervisor      *Supervisor
    backoff         *ExponentialBackoff
    maxAttempts     int
    attemptWindow   time.Duration
    windowStart     time.Time
    logger          *zap.SugaredLogger
}

// NewInfrastructureHealthChecker creates a new health checker
func NewInfrastructureHealthChecker(
    supervisor *Supervisor,
    maxAttempts int,
    attemptWindow time.Duration,
) *InfrastructureHealthChecker {
    return &InfrastructureHealthChecker{
        supervisor:    supervisor,
        backoff:       NewExponentialBackoff(1*time.Second, 60*time.Second),
        maxAttempts:   maxAttempts,
        attemptWindow: attemptWindow,
        windowStart:   time.Now(),
        logger:        supervisor.logger,
    }
}

// MaxAttempts returns the configured max attempts
func (ihc *InfrastructureHealthChecker) MaxAttempts() int {
    return ihc.maxAttempts
}

// Backoff returns the backoff instance
func (ihc *InfrastructureHealthChecker) Backoff() *ExponentialBackoff {
    return ihc.backoff
}

// ShouldGiveUp returns true if max attempts reached
func (ihc *InfrastructureHealthChecker) ShouldGiveUp() bool {
    return ihc.backoff.Attempts() >= ihc.maxAttempts
}

// RecordFailure records a failure with window management
func (ihc *InfrastructureHealthChecker) RecordFailure(now time.Time) {
    // Reset window if last attempt was too long ago
    if now.Sub(ihc.windowStart) > ihc.attemptWindow {
        ihc.backoff.Reset()
        ihc.windowStart = now
    }

    ihc.backoff.RecordFailure()
}
```

**Step 4: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: PASS (all tests green)

**Step 5: Commit**

```bash
git add pkg/fsmv2/supervisor/infrastructure_health.go pkg/fsmv2/supervisor/infrastructure_health_test.go
git commit -m "feat(fsmv2): add InfrastructureHealthChecker structure

Implements health checker with:
- Exponential backoff integration
- Attempt window management (5 min default)
- Max attempts tracking (5 default)

Part of Infrastructure Supervision (Phase 1)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

**Review Checkpoint:** Dispatch code-reviewer subagent to verify:
- Window reset logic correctness
- Integration with ExponentialBackoff
- Test coverage of time-based scenarios

---

### Task 1.3: Child Consistency Check Interface

**Goal:** Add CheckChildConsistency method with example sanity checks

**Files:**
- Modify: `pkg/fsmv2/supervisor/infrastructure_health.go`
- Modify: `pkg/fsmv2/supervisor/infrastructure_health_test.go`

**Step 1: Write the failing test**

```go
// Add to infrastructure_health_test.go

Describe("CheckChildConsistency", func() {
    Context("when Redpanda active but Benthos has no output connections", func() {
        It("returns error describing inconsistency", func() {
            // Setup supervisor with mock children
            sup := setupMockSupervisor()
            sup.SetChildState("redpanda", "active")
            sup.SetChildObserved("dfc_read", &mockBenthosObserved{
                OutputConnectionsUp:   0,
                OutputConnectionsLost: 0,
            })

            checker := supervisor.NewInfrastructureHealthChecker(sup, 5, 5*time.Minute)

            err := checker.CheckChildConsistency()

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("redpanda active but benthos has 0 output connections"))
        })
    })

    Context("when both children consistent", func() {
        It("returns nil", func() {
            sup := setupMockSupervisor()
            sup.SetChildState("redpanda", "active")
            sup.SetChildObserved("dfc_read", &mockBenthosObserved{
                OutputConnectionsUp:   2,
                OutputConnectionsLost: 0,
            })

            checker := supervisor.NewInfrastructureHealthChecker(sup, 5, 5*time.Minute)

            err := checker.CheckChildConsistency()

            Expect(err).ToNot(HaveOccurred())
        })
    })
})
```

**Step 2: Run test to verify it fails**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: FAIL with "undefined: CheckChildConsistency"

**Step 3: Write minimal implementation**

```go
// Add to infrastructure_health.go

// CheckChildConsistency runs sanity checks across children
func (ihc *InfrastructureHealthChecker) CheckChildConsistency() error {
    // Example sanity check: Redpanda vs Benthos consistency
    redpandaState, err := ihc.supervisor.GetChildState("redpanda")
    if err != nil {
        // Child not declared, skip check
        return nil
    }

    benthosObserved, err := ihc.supervisor.getChildObservedInternal("dfc_read")
    if err != nil {
        // Can't check, skip
        return nil
    }

    // Type assert to get Benthos metrics
    benthosState, ok := benthosObserved.(DataFlowObservedState)
    if !ok {
        // Can't check metrics, skip
        return nil
    }

    // Sanity check: Redpanda active but Benthos has no output connections
    if redpandaState == "active" || redpandaState == "idle" {
        outputActive := benthosState.OutputConnectionsUp - benthosState.OutputConnectionsLost
        if outputActive == 0 {
            return fmt.Errorf(
                "redpanda active but benthos has %d output connections",
                outputActive,
            )
        }
    }

    return nil
}

// getChildObservedInternal is supervisor-internal method (not exposed to workers)
func (s *Supervisor) getChildObservedInternal(childName string) (interface{}, error) {
    childSpec, exists := s.childDeclarations[childName]
    if !exists {
        return nil, fmt.Errorf("child %s not declared", childName)
    }

    // Load observed state from TriangularStore
    return s.store.LoadObserved(context.Background(), childSpec.WorkerType, childSpec.ID)
}
```

**Step 4: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: PASS (all tests green)

**Step 5: Commit**

```bash
git add pkg/fsmv2/supervisor/infrastructure_health.go pkg/fsmv2/supervisor/infrastructure_health_test.go
git commit -m "feat(fsmv2): implement CheckChildConsistency sanity checks

Adds cross-child consistency checks:
- Redpanda active vs Benthos output connections
- Returns error describing inconsistency
- Skips gracefully if children not declared

Part of Infrastructure Supervision (Phase 1)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

**Review Checkpoint:** Dispatch code-reviewer subagent to verify:
- Sanity check logic correctness
- Error message clarity
- Graceful handling of missing children

---

### Task 1.4: Circuit Breaker Integration

**Goal:** Integrate circuit breaker logic into Supervisor.Tick() with child restart

**Files:**
- Modify: `pkg/fsmv2/supervisor/supervisor.go`
- Create: `pkg/fsmv2/supervisor/supervisor_infra_test.go`

**Step 1: Write the failing test**

```go
// pkg/fsmv2/supervisor/supervisor_infra_test.go
package supervisor_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"
    "errors"
    "time"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("CircuitBreakerIntegration", func() {
    var (
        sup               *supervisor.Supervisor
        mockHealthChecker *mocks.MockHealthChecker
        mockBackoff       *mocks.MockBackoff
        ctx               context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        mockHealthChecker = mocks.NewMockHealthChecker()
        mockBackoff = mocks.NewMockBackoff()
        sup = supervisor.NewSupervisor(mockHealthChecker, mockBackoff)
    })

    Context("when infrastructure check fails", func() {
        It("opens circuit and skips tick", func() {
            mockHealthChecker.SetCheckResult(errors.New("Redpanda unreachable"))

            err := sup.Tick(ctx)

            Expect(err).ToNot(HaveOccurred())
            Expect(sup.IsCircuitOpen()).To(BeTrue())
            Expect(sup.GetLastActionType()).To(Equal("ChildRestart"))
        })

        It("applies exponential backoff delay", func() {
            mockHealthChecker.SetCheckResult(errors.New("Redpanda unreachable"))
            mockBackoff.SetNextDelay(2 * time.Second)

            start := time.Now()
            _ = sup.Tick(ctx)
            elapsed := time.Since(start)

            Expect(elapsed).To(BeNumerically("~", 2*time.Second, 100*time.Millisecond))
        })
    })

    Context("when infrastructure check succeeds after failure", func() {
        It("closes circuit and resets backoff", func() {
            // First tick: open circuit
            mockHealthChecker.SetCheckResult(errors.New("Redpanda unreachable"))
            _ = sup.Tick(ctx)
            Expect(sup.IsCircuitOpen()).To(BeTrue())

            // Second tick: success
            mockHealthChecker.SetCheckResult(nil)
            err := sup.Tick(ctx)

            Expect(err).ToNot(HaveOccurred())
            Expect(sup.IsCircuitOpen()).To(BeFalse())
            Expect(mockBackoff.GetAttempts()).To(Equal(0))
        })
    })

    Context("when circuit is closed", func() {
        It("proceeds with normal tick processing", func() {
            mockHealthChecker.SetCheckResult(nil)

            err := sup.Tick(ctx)

            Expect(err).ToNot(HaveOccurred())
            Expect(sup.GetLastActionType()).To(Equal("NormalTick"))
        })
    })

    Context("when max retry attempts reached", func() {
        It("keeps circuit open and logs escalation", func() {
            mockHealthChecker.SetCheckResult(errors.New("Redpanda unreachable"))
            mockBackoff.SetMaxAttempts(5)

            // Exhaust all attempts
            for i := 0; i < 5; i++ {
                _ = sup.Tick(ctx)
            }

            err := sup.Tick(ctx)

            Expect(err).ToNot(HaveOccurred())
            Expect(sup.IsCircuitOpen()).To(BeTrue())
            Expect(sup.GetLastLogMessage()).To(ContainSubstring("ESCALATION REQUIRED"))
        })
    })
})
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
ginkgo -v pkg/fsmv2/supervisor
```

Expected: FAIL with "undefined: supervisor.NewSupervisor"

**Step 3: Write minimal implementation**

```go
// pkg/fsmv2/supervisor/supervisor.go
package supervisor

import (
    "context"
    "time"

    "go.uber.org/zap"
)

type Supervisor struct {
    healthChecker  *InfrastructureHealthChecker
    backoff        *ExponentialBackoff
    circuitOpen    bool
    lastActionType string
    lastLogMessage string
    logger         *zap.SugaredLogger
}

func NewSupervisor(
    healthChecker *InfrastructureHealthChecker,
    backoff *ExponentialBackoff,
) *Supervisor {
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
    if err := s.healthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.lastActionType = "ChildRestart"

        delay := s.backoff.NextDelay()
        time.Sleep(delay)
        s.backoff.RecordFailure()

        if s.backoff.Attempts() >= 5 {
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

**Step 4: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: PASS (all 5 test cases green)

**Step 5: Refactor with logging and metrics**

```go
// Add to supervisor.go

func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Health Check
    if err := s.healthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.lastActionType = "ChildRestart"

        // Emit metrics
        metrics.InfrastructureCheckFailures.Inc()
        metrics.CircuitBreakerOpenGauge.Set(1)

        // Log with context
        s.logger.Warnw("Infrastructure check failed, circuit opened",
            "error", err,
            "circuit_open", true,
            "attempt", s.backoff.Attempts(),
        )

        delay := s.backoff.NextDelay()
        time.Sleep(delay)
        s.backoff.RecordFailure()

        if s.backoff.Attempts() >= 5 {
            s.lastLogMessage = "ESCALATION REQUIRED: Infrastructure failure after 5 attempts"
            s.logger.Errorw(s.lastLogMessage,
                "total_downtime", s.backoff.LastAttempt().Sub(time.Now()),
            )
            metrics.EscalationsTotal.Inc()
        }

        return nil
    }

    // Infrastructure healthy: close circuit and reset
    if s.circuitOpen {
        s.logger.Info("Infrastructure recovered, circuit closed")
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

**Step 6: Commit**

```bash
git add pkg/fsmv2/supervisor/supervisor.go pkg/fsmv2/supervisor/supervisor_infra_test.go
git commit -m "feat(fsmv2): integrate circuit breaker into Supervisor.Tick()

Implements infrastructure supervision with:
- Circuit breaker opens on health check failures
- Exponential backoff delays for child restarts
- Circuit closes when infrastructure recovers
- Escalation after max attempts reached
- Prometheus metrics and structured logging

Part of Infrastructure Supervision (Phase 1)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

**Review Checkpoint:** Dispatch code-reviewer subagent to verify:
- Circuit breaker logic correctness
- Integration with health checker and backoff
- Test coverage of all edge cases (escalation, recovery, normal operation)

---

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

---

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

**Goal:** Document expected behavior when Infrastructure Supervision restarts a child.

**Decision:** Do not cancel in-progress actions when children restart.

**Behavior:**
1. Infrastructure Supervision detects child inconsistency
2. Circuit breaker opens, pausing tick for all workers
3. Infrastructure restarts child (e.g., `dfc_read`, `redpanda`)
4. In-progress actions continue execution:
   - Actions depending on restarted child will fail with connection errors
   - Actions timeout after 30s (ActionExecutorConfig.ActionTimeout)
   - Failed actions retry with exponential backoff (1s, 2s, 4s...)
   - Actions succeed once child recovers
5. Circuit closes when child healthy
6. Tick resumes for all workers

**Rationale:**
- Child restarts are rare (edge case, not normal operation)
- Timeout + retry = ~60s total recovery (acceptable for edge cases)
- Cancellation would require dependency tracking (worker → child dependencies)
- Dependency tracking is significant scope creep for minimal gain
- Can revisit in Phase 4 if production data shows frequent restarts

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

**Goal:** Clarify and verify observation collection continues when circuit breaker is open.

**Decision:** Keep current behavior (observations continue during circuit open).

**Architecture:**
- Collectors run as independent goroutines (1 per worker)
- Collectors call `worker.CollectObservedState()` every second
- Observations written to TriangularStore continuously
- Circuit breaker does NOT pause collectors

**Behavior when circuit open:**
1. Infrastructure Supervision opens circuit
2. Tick loop skips state transitions (returns early)
3. **Collectors continue running** (independent goroutines)
4. Observations written to TriangularStore every second
5. When circuit closes, tick loop reads fresh observations from store

**Rationale:**
- **Fresh data when recovered:** First tick after circuit closes has current state
- **Continuous monitoring:** Metrics and logs show what's happening during failure
- **Cheap operations:** Collecting observations every second is minimal overhead
- **Decoupled design:** Collectors don't need to know about circuit breaker
- **Observability:** Don't go "dark" during infrastructure failures

**Acceptance Criteria**:
- [ ] Documentation added to `docs/design/fsmv2-child-observed-state-usage.md`
- [ ] Section: "Collector Independence from Circuit Breaker" with diagram:
  ```
  [Collectors (1s loop)] → [TriangularStore] ← [Tick Loop (when circuit closed)]
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

**Code clarification:**

Current master plan shows:
```go
for workerID := range s.workers {
    observed, _ := workerCtx.collector.GetLatestObservation(ctx)  // MISLEADING
}
```

This is pseudocode. Actual implementation:
```go
for workerID := range s.workers {
    snapshot, _ := s.store.LoadSnapshot(ctx, workerType, workerID)  // Read from DB
}
```

Collectors write to DB independently:
```go
// Separate goroutine per worker
func (c *Collector) Start(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    for range ticker.C {
        observed, _ := c.worker.CollectObservedState(ctx)
        c.store.StoreObserved(ctx, workerType, workerID, observed)
    }
}
```

---

**Acceptance Criteria for Phase 3**:
- [ ] Layered precedence verified (infrastructure > action)
- [ ] Documentation complete (action lifecycle, collector independence)
- [ ] Integration tests pass (2 scenarios)
- [ ] Metrics recorded correctly
- [ ] No race conditions or deadlocks

---

## Phase 4: Monitoring & Observability (Week 6)

**Goal**: Give operators visibility into infrastructure supervision and action execution, with enhanced UX for production operations

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

### Task 4.2: Structured Logging with UX Enhancements (35 min)

**File**: `pkg/infrastructure/supervisor/supervisor.go`

Add comprehensive log statements with ALL UX improvements integrated:

```go
// pkg/infrastructure/supervisor/supervisor.go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Health Check
    if err := s.healthChecker.Check(ctx); err != nil {
        s.circuitOpen = true
        attempts := s.backoff.GetAttempts()

        // UX Enhancement 1: Infrastructure vs Worker Error Distinction
        log.Warn().
            Err(err).
            Str("error_scope", "infrastructure").      // ← Distinguishes from worker errors
            Str("impact", "all_workers").              // ← Clarifies blast radius
            Str("child_name", childName).
            Int("retry_attempt", attempts).
            Dur("backoff_delay", s.backoff.NextDelay()).
            Msg("Infrastructure check failed, opening circuit breaker")

        // UX Enhancement 2: Recovery Progress Feedback (Heartbeat)
        if s.circuitOpen {
            log.Warn().
                Int("retry_attempt", attempts).
                Int("max_attempts", 5).
                Dur("elapsed_downtime", s.backoff.GetTotalDowntime()).
                Dur("next_retry_in", s.backoff.NextDelay()).
                Str("recovery_status", s.getRecoveryStatus()).
                Msg("Circuit breaker open, retrying infrastructure checks")
        }

        // UX Enhancement 3: Pre-Escalation Warning
        if attempts == 4 {
            log.Warn().
                Str("child_name", childName).
                Int("attempts_remaining", 1).
                Dur("total_downtime", s.backoff.GetTotalDowntime()).
                Msg("WARNING: One retry attempt remaining before escalation")
        }

        // UX Enhancement 4: Escalation Runbook Embedding
        if attempts >= 5 {
            log.Error().
                Str("child_name", childName).
                Int("max_attempts", 5).
                Dur("total_downtime", s.backoff.GetTotalDowntime()).
                Str("runbook_url", "https://docs.umh.app/runbooks/supervisor-escalation").
                Str("manual_steps", s.getEscalationSteps(childName)).
                Msg("ESCALATION REQUIRED: Infrastructure failure after max retry attempts. Manual intervention needed.")
            metrics.EscalationsTotal.Inc()
        }

        delay := s.backoff.NextDelay()
        time.Sleep(delay)
        s.backoff.RecordFailure()
        metrics.InfrastructureCheckFailures.Inc()
        metrics.CircuitBreakerOpenGauge.Set(1)

        return nil
    }

    // Infrastructure healthy: close circuit and reset
    if s.circuitOpen {
        log.Info().
            Str("child_name", childName).
            Dur("total_downtime", s.backoff.GetTotalDowntime()).
            Msg("Infrastructure recovered, closing circuit breaker")
        metrics.CircuitBreakerRecoveries.Inc()
    }

    s.circuitOpen = false
    s.backoff.Reset()
    metrics.CircuitBreakerOpenGauge.Set(0)

    // PRIORITY 2: Per-Worker Actions
    for workerID := range s.workers {
        if s.actionExecutor.HasActionInProgress(workerID) {
            log.Debug().
                Str("worker_id", workerID).
                Str("error_scope", "worker").         // ← Worker-specific error
                Str("impact", "single_worker").       // ← Limited blast radius
                Msg("Worker action in progress, skipping derivation")
            continue
        }

        // Action enqueued
        log.Debug().
            Str("worker_id", workerID).
            Str("action_type", action.Name()).
            Msg("Action enqueued for execution")
        metrics.ActionsEnqueuedTotal.WithLabelValues(workerID, action.Name()).Inc()

        // Action completed (called from action executor)
        log.Info().
            Str("worker_id", workerID).
            Str("action_type", action.Name()).
            Dur("duration", elapsed).
            Bool("success", err == nil).
            Msg("Action completed")
    }

    return nil
}

// Helper: Recovery status indicator
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

// Helper: Runbook steps per child
func (s *Supervisor) getEscalationSteps(childName string) string {
    steps := map[string]string{
        "dfc_read": "1) Check Redpanda logs 2) Verify network connectivity 3) Restart Redpanda manually",
        "benthos":  "1) Check Benthos logs 2) Verify OPC UA server reachable 3) Restart Benthos manually",
    }
    return steps[childName]
}
```

**UX Improvements Integrated**:
1. **Recovery Progress Feedback**: Heartbeat logs every tick when circuit open (from Task 3.1)
2. **Infrastructure vs Worker Error Distinction**: `error_scope` and `impact` fields (from Task 3.2)
3. **Pre-Escalation Warning**: Warning at attempt 4 (from Task 3.4)
4. **Escalation Runbook Embedding**: Runbook URL and manual steps (from Task 3.5)

**Test**: Verify log output in integration tests (capture stderr), check all UX fields present

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

### Task 4.5: Operator UX Validation (15 min)

**Goal**: Manually verify all UX enhancements work as intended from operator perspective

**Test Scenarios**:

1. **Recovery Progress Feedback**
   - Trigger infrastructure failure (stop Redpanda)
   - Verify heartbeat logs appear every tick
   - Check `recovery_status` field transitions: `attempting_recovery` → `persistent_failure` → `escalation_imminent`

2. **Error Scope Distinction**
   - Trigger infrastructure error (affects all workers)
   - Verify log has `error_scope: infrastructure`, `impact: all_workers`
   - Trigger worker-specific error (single action fails)
   - Verify log has `error_scope: worker`, `impact: single_worker`

3. **Pre-Escalation Warning**
   - Let circuit stay open for 4 attempts
   - Verify WARNING log at attempt 4 with "attempts_remaining: 1"

4. **Escalation Runbook**
   - Let circuit reach attempt 5
   - Verify ERROR log contains `runbook_url` and `manual_steps` fields

5. **Full Recovery Flow**
   - Trigger failure → observe heartbeat logs → fix infrastructure → verify recovery log

**Acceptance Criteria**:
- [ ] Heartbeat logs visible during recovery (not silent)
- [ ] Error logs distinguish infrastructure vs worker scope
- [ ] Pre-escalation warning appears at attempt 4
- [ ] Escalation logs include runbook URL and manual steps
- [ ] All log fields parseable by log aggregators (JSON format)

---

**Acceptance Criteria for Phase 4**:
- [ ] All metrics emit correctly (9 total: circuit, actions, edge cases)
- [ ] Logs structured and parseable (JSON format)
- [ ] Grafana dashboard loads and displays data (4 panels)
- [ ] Alerts trigger on failure conditions (3 alert rules)
- [ ] Operators can diagnose issues from logs/metrics alone (no code inspection)
- [ ] **UX Standards Met**:
  - [ ] Operators receive feedback during recovery (heartbeat logs)
  - [ ] Error logs distinguish infrastructure vs worker scope
  - [ ] Pre-escalation warning at attempt 4
  - [ ] Escalation logs include runbook and manual steps
  - [ ] All UX enhancements validated in Task 4.5

---

## Testing Strategy

### Unit Tests (Per Task)
- TDD: RED → GREEN → REFACTOR
- Each test must fail first (verify failure reason)
- Minimal code to pass test
- Test coverage: happy path + edge cases + error conditions

### Integration Tests (Per Phase)
- Test infrastructure supervision independently
- Test async actions independently
- Test combined system with both conditions

### System Tests (Final)
- ProtocolConverter use case (FSMv1 sanity check migration)
- Both conditions true simultaneously
- Performance under load
- Escalation scenarios

---

## Code Review Checkpoints

**After Each Task:**
1. Dispatch code-reviewer subagent with findings from implementation
2. Address Critical/Important issues
3. Mark task complete only after review approved

**After Each Phase:**
1. CodeRabbit full review: `gh pr comment PR_NUMBER --body "@coderabbitai full review"`
2. Address all Critical issues
3. Document any deferred issues in Linear

**Before Final Merge:**
1. Full integration test suite
2. Performance benchmarks (tick loop latency)
3. Documentation review

---

## Execution Options

**Option 1: Subagent-Driven (this session)**
- Dispatch fresh subagent per task
- Review between tasks
- Fast iteration

**Option 2: Parallel Session (separate)**
- Open new session with executing-plans skill
- Batch execution with checkpoints

---

## Changelog

### 2025-11-02 18:00 - Gap analysis added

Added **Gap Analysis & Resolution** section addressing two minor gaps identified during plan alignment:

1. **Gap 1: Action Cancellation on Child Restart**
   - Decision: Accept timeout/retry behavior (do not implement cancellation)
   - Rationale: Dependency tracking is significant scope creep for rare edge case
   - Resolution: Task 3.2 documents behavior and adds monitoring

2. **Gap 2: Observation Collection During Circuit Open**
   - Decision: Keep current behavior (observations continue)
   - Rationale: Fresh data valuable, collectors already independent goroutines
   - Resolution: Task 3.4 clarifies pseudocode and documents architecture

Updated Phase 3 tasks:
- Task 3.2: Action Behavior During Child Restart (Documentation) - 2-3 hours
- Task 3.4: Observation Collection During Circuit Open (Clarification) - 1-2 hours

Corrected tick loop pseudocode to show collectors as independent goroutines writing to TriangularStore.

---

### 2025-11-02 15:00 - Plan created

Initial master plan combining:
- Infrastructure Supervision (Circuit Breaker Pattern)
- Async Action Executor (Global Worker Pool)
- Integration strategy from compatibility analysis

Phase 1 (Infrastructure) prioritized as foundation, Phase 2 (Async Actions) builds on top. TDD approach integrated throughout with code review checkpoints after each task.

Design documents referenced:
- `fsmv2-infrastructure-supervision-patterns.md`
- `fsmv2-developer-expectations-child-state.md`
- `fsmv2-child-observed-state-usage.md`
- `async-action-executor-implementation.md`

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

---

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
