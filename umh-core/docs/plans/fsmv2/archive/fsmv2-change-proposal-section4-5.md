# FSMv2 Master Plan Change Proposal - Sections 4-5

**Date:** 2025-11-03
**Status:** Draft for Review
**Related:**
- Change Proposal: `fsmv2-master-plan-change-proposal.md`
- Gap Analysis: `fsmv2-master-plan-gap-analysis.md`
- Master Plan: `2025-11-02-fsmv2-supervision-and-async-actions.md`

---

## Section 4: Modified Phase 1 Tasks (Infrastructure Supervision)

This section specifies how existing Phase 1 tasks from the master plan are modified to integrate with hierarchical composition. Phase 1 originally implemented Infrastructure Supervision (circuit breaker pattern); now it must work with the new `children` map and hierarchical APIs from Phase 0.

### Task 1.1: ExponentialBackoff

**Action:** KEEP (no changes)

**Reason:** Backoff logic is independent of hierarchical composition. The implementation works identically whether restarting a flat worker or a child supervisor.

**Change:** None

**Code:** As defined in master plan, lines 195-367

---

### Task 1.2: InfrastructureHealthChecker Structure

**Action:** MODIFY

**Reason:** Add child consistency checks using new `s.children` map instead of hardcoded child names.

**Change:**

**BEFORE (master plan assumption):**
```go
// pkg/fsmv2/supervisor/infrastructure_health.go
type InfrastructureHealthChecker struct {
    supervisor      *Supervisor
    backoff         *ExponentialBackoff
    maxAttempts     int
    attemptWindow   time.Duration
    windowStart     time.Time
    logger          *zap.SugaredLogger
}
```

**AFTER (with hierarchical composition):**
```go
// pkg/fsmv2/supervisor/infrastructure_health.go
type InfrastructureHealthChecker struct {
    supervisor      *Supervisor  // ← Access to s.children map
    backoff         *ExponentialBackoff
    maxAttempts     int
    attemptWindow   time.Duration
    windowStart     time.Time
    logger          *zap.SugaredLogger
}

// No structural changes, but CheckChildConsistency now uses s.children
```

**Impact:** Structure stays the same, but methods access `s.children` instead of hardcoded names.

---

### Task 1.3: Child Consistency Check Interface

**Action:** MODIFY

**Reason:** Use new hierarchical API (`s.children["redpanda"]`) instead of placeholder child access.

**Change:**

**BEFORE (master plan pseudocode - lines 659-703):**
```go
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

    // ... rest of sanity check
}
```

**AFTER (with hierarchical composition API):**
```go
// CheckChildConsistency runs sanity checks across children
func (ihc *InfrastructureHealthChecker) CheckChildConsistency() error {
    // Use children map instead of GetChildState
    redpandaChild, exists := ihc.supervisor.children["redpanda"]
    if !exists {
        return nil  // Child not declared, skip check
    }

    benthosChild, exists := ihc.supervisor.children["dfc_read"]
    if !exists {
        return nil  // Child not declared, skip check
    }

    // Get state from child supervisors
    redpandaState := redpandaChild.supervisor.GetDesiredState().State
    benthosObserved := benthosChild.supervisor.GetObservedState()

    // Type assert to get Benthos metrics
    benthosState, ok := benthosObserved.(DataFlowObservedState)
    if !ok {
        return nil  // Can't check metrics, skip
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
```

**Complete Updated Implementation:**

**File:** `pkg/fsmv2/supervisor/infrastructure_health.go`

**Lines to modify:** 659-703

**Changes:**
1. **Line 661:** `ihc.supervisor.GetChildState("redpanda")` → `ihc.supervisor.children["redpanda"]`
2. **Line 667:** `ihc.supervisor.getChildObservedInternal("dfc_read")` → `ihc.supervisor.children["dfc_read"]`
3. **Line 680:** Add child state access via `child.supervisor.GetDesiredState().State`
4. **Line 683:** Add observed state access via `child.supervisor.GetObservedState()`

**Test updates:**

**File:** `pkg/fsmv2/supervisor/infrastructure_health_test.go`

**Lines to modify:** 606-643

**Changes:**
1. Setup mock supervisor with children map
2. Use `sup.children["redpanda"] = &childInstance{...}` instead of `SetChildState`
3. Verify child existence checks work correctly

**TDD Steps (same as original):**
1. Write failing test with new API
2. Run test to verify failure
3. Update implementation to use `s.children` map
4. Run test to verify pass
5. Commit

**Estimated:** Same as original (Task 1.3)

---

### Task 1.4: Circuit Breaker Integration

**Action:** MODIFY

**Reason:** Circuit breaker must not interfere with `reconcileChildren()`, and child restart must use `s.children` map.

**Change:**

**BEFORE (master plan pseudocode):**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Health
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.restartChild(ctx, "dfc_read")
        time.Sleep(backoff)
        return nil
    }

    // PRIORITY 2: Per-Worker Actions
    for workerID, workerCtx := range s.workers {
        // ... action checks
    }
}
```

**AFTER (with hierarchical composition):**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PRIORITY 1: Infrastructure Health (affects all children)
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.restartChild(ctx, "dfc_read")  // ← Uses s.children internally
        return nil  // Skip tick, but reconcileChildren still runs
    }

    s.circuitOpen = false

    // PRIORITY 2: Derive Desired State (with children)
    desiredState := s.worker.DeriveDesiredState(s.userSpec)

    // PRIORITY 3: Reconcile Children (NEW - circuit doesn't block this)
    s.reconcileChildren(desiredState.ChildrenSpecs)

    // PRIORITY 4: Apply State Mapping
    s.applyStateMapping(desiredState.State)

    // PRIORITY 5: Per-Child Actions
    for _, child := range s.children {
        if s.actionExecutor.HasActionInProgress(child.name) {
            continue
        }
        child.supervisor.Tick(ctx)
    }

    return nil
}
```

**Updated Tick() Pseudocode:**

**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Complete implementation:**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // NOTE: Collectors run independently in separate goroutines.
    // They continuously write observations to TriangularStore every 5s.
    // Circuit breaker does NOT pause collectors, only prevents tick from acting on observations.

    // PHASE 1: Infrastructure Health (affects all children)
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.logger.Warnf("Infrastructure health check failed: %v", err)

        // Restart specific child
        if child, exists := s.children["dfc_read"]; exists {
            backoff := s.infraHealthChecker.Backoff()
            delay := backoff.NextDelay()
            s.logger.Infof("Restarting child 'dfc_read' with backoff %v", delay)

            time.Sleep(delay)
            child.supervisor.Restart(ctx)

            backoff.RecordFailure()
        }

        // Circuit open: reconcileChildren still runs, but children don't tick
        return nil
    }

    s.circuitOpen = false

    // PHASE 2: Derive Desired State (with children)
    userSpec := s.triangularStore.GetUserSpec()
    desiredState := s.worker.DeriveDesiredState(userSpec)
    s.triangularStore.SetDesiredState(desiredState)

    // PHASE 3: Reconcile Children (NEW)
    // ← Circuit does NOT block this (children may need updates even when circuit open)
    s.reconcileChildren(desiredState.ChildrenSpecs)

    // PHASE 4: Apply State Mapping
    s.applyStateMapping(desiredState.State)

    // PHASE 5: Tick Children (if circuit closed)
    for _, child := range s.children {
        // Per-child action check
        if s.actionExecutor.HasActionInProgress(child.name) {
            continue
        }

        // Tick child (child has own circuit breaker)
        child.supervisor.Tick(ctx)
    }

    // PHASE 6: Collect Observed State
    observedState, _ := s.worker.CollectObservedState(ctx)
    s.triangularStore.SetObservedState(observedState)

    return nil
}
```

**Key Behavior:**

1. **Circuit open** → Skip child ticking, but `reconcileChildren()` still runs
2. **Circuit closed** → Full tick loop with children
3. **Child restart** → Uses `s.children[name].supervisor.Restart()`

**Test updates:**

**File:** `pkg/fsmv2/supervisor/supervisor_infra_test.go`

**New test:**
```go
Describe("Circuit Breaker with Children", func() {
    It("should skip child tick when circuit open but still reconcile", func() {
        // 1. Setup supervisor with children
        // 2. Trigger infrastructure failure (circuit open)
        // 3. Verify children don't tick
        // 4. Verify reconcileChildren still runs
        // 5. Close circuit
        // 6. Verify children tick resumes
    })

    It("should restart child via children map", func() {
        // 1. Setup supervisor with "dfc_read" child
        // 2. Trigger infrastructure failure
        // 3. Verify child.supervisor.Restart() called
        // 4. Verify backoff recorded
    })
})
```

**Estimated:** Same as original (Task 1.4), plus 1 hour for new tests

---

### NEW Task 1.5: Child Supervisor Restart

**Action:** ADD

**Reason:** When circuit breaker restarts a child, it must use WorkerFactory to recreate the child worker, not just restart the existing supervisor.

**Background:**

The original master plan assumed restarting a child meant calling `child.Restart()`. With hierarchical composition, children are created dynamically from ChildSpecs. When a child crashes or becomes inconsistent, we need to:

1. **Destroy** the old child supervisor
2. **Recreate** the child worker from ChildSpec (via WorkerFactory)
3. **Create** a new child supervisor with the fresh worker

This ensures a clean slate (no corrupted state from failed child).

**Steps:**

This is a NEW task, so we provide the complete TDD breakdown:

---

#### Step 1: Write the failing test

**File:** `pkg/fsmv2/supervisor/supervisor_child_restart_test.go` (NEW)

```go
package supervisor_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"
    "time"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("Child Supervisor Restart", func() {
    var (
        sup     *supervisor.Supervisor
        factory *mockWorkerFactory
    )

    BeforeEach(func() {
        // Setup supervisor with mock factory
        factory = newMockWorkerFactory()
        sup = setupSupervisorWithFactory(factory)

        // Add child via reconcileChildren
        sup.reconcileChildren([]supervisor.ChildSpec{
            {
                Name:       "test-child",
                WorkerType: "TestWorker",
                UserSpec:   mockUserSpec(),
            },
        })
    })

    Describe("RestartChild", func() {
        It("should destroy old supervisor and create new one via factory", func() {
            // Get initial child supervisor
            initialChild := sup.children["test-child"]
            initialSupervisorID := initialChild.supervisor.ID()

            // Restart child
            err := sup.RestartChild(context.Background(), "test-child")

            Expect(err).ToNot(HaveOccurred())

            // Verify factory was called to create new worker
            Expect(factory.CreateWorkerCalls).To(Equal(2)) // Initial + restart

            // Verify new supervisor created
            newChild := sup.children["test-child"]
            Expect(newChild.supervisor.ID()).ToNot(Equal(initialSupervisorID))
        })

        It("should return error if child not declared", func() {
            err := sup.RestartChild(context.Background(), "nonexistent")

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("child nonexistent not declared"))
        })

        It("should preserve child spec during restart", func() {
            // Original spec
            originalSpec := supervisor.ChildSpec{
                Name:       "test-child",
                WorkerType: "TestWorker",
                UserSpec:   mockUserSpec(),
            }

            // Restart
            sup.RestartChild(context.Background(), "test-child")

            // Verify spec unchanged
            newChild := sup.children["test-child"]
            Expect(newChild.spec).To(Equal(originalSpec))
        })
    })

    Describe("Circuit Breaker Integration with Restart", func() {
        It("should use RestartChild when circuit opens", func() {
            // Trigger infrastructure failure
            sup.infraHealthChecker.InjectFailure("dfc_read inconsistent")

            // Tick (circuit should open and restart child)
            sup.Tick(context.Background())

            // Verify RestartChild was called via factory
            Expect(factory.CreateWorkerCalls).To(BeNumerically(">", 1))
        })
    })
})
```

---

#### Step 2: Run test to verify it fails

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
ginkgo -v pkg/fsmv2/supervisor
```

**Expected:** FAIL with "undefined: RestartChild"

---

#### Step 3: Write minimal implementation

**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Add method:**
```go
// RestartChild destroys and recreates a child supervisor
func (s *Supervisor) RestartChild(ctx context.Context, childName string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Get child instance
    child, exists := s.children[childName]
    if !exists {
        return fmt.Errorf("child %s not declared", childName)
    }

    s.logger.Infof("Restarting child %s", childName)

    // Destroy old supervisor
    child.supervisor.Stop(ctx)

    // Recreate worker from spec via factory
    worker, err := s.factory.CreateWorker(child.spec.WorkerType, child.spec.UserSpec)
    if err != nil {
        return fmt.Errorf("failed to recreate worker: %w", err)
    }

    // Create new supervisor
    newSupervisor := NewSupervisor(worker, s.factory, s.logger)

    // Replace child instance
    s.children[childName] = &childInstance{
        name:         childName,
        spec:         child.spec,  // Preserve original spec
        supervisor:   newSupervisor,
        stateMapping: child.stateMapping,
    }

    s.logger.Infof("Child %s restarted successfully", childName)
    return nil
}
```

**Update Task 1.4 circuit breaker code:**

**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Update Tick() to use RestartChild:**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PHASE 1: Infrastructure Health
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true
        s.logger.Warnf("Infrastructure health check failed: %v", err)

        // Restart specific child (NEW: uses RestartChild)
        backoff := s.infraHealthChecker.Backoff()
        delay := backoff.NextDelay()
        s.logger.Infof("Restarting child 'dfc_read' with backoff %v", delay)

        time.Sleep(delay)
        if err := s.RestartChild(ctx, "dfc_read"); err != nil {
            s.logger.Errorf("Failed to restart child: %v", err)
        }

        backoff.RecordFailure()
        return nil
    }

    // ... rest of tick
}
```

---

#### Step 4: Run test to verify it passes

```bash
ginkgo -v pkg/fsmv2/supervisor
```

**Expected:** PASS (all tests green)

---

#### Step 5: Commit

```bash
git add pkg/fsmv2/supervisor/supervisor.go pkg/fsmv2/supervisor/supervisor_child_restart_test.go
git commit -m "feat(fsmv2): add RestartChild for clean child recovery

Implements child restart via WorkerFactory:
- Destroys old supervisor completely
- Recreates worker from ChildSpec
- Creates fresh supervisor with no stale state
- Integrates with circuit breaker

This ensures clean recovery when children become inconsistent.

Part of Infrastructure Supervision (Phase 1)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

**Review Checkpoint:** Dispatch code-reviewer subagent to verify:
- RestartChild is thread-safe (mutex protection)
- Child spec preservation correctness
- Integration with circuit breaker
- Test coverage of error conditions

**Estimated:** 4 hours (new task, requires careful testing)

---

## Section 5: Phase 2 & 3 Modifications

This section specifies how Phase 2 (Async Action Executor) and Phase 3 (Integration & Edge Cases) are modified to integrate with hierarchical composition.

### Phase 2 (Async Action Executor) - MINIMAL CHANGES

**What Stays the Same:**

✅ **Core Structure:**
- ActionExecutor with global worker pool
- Action queueing with timeouts
- HasActionInProgress() check
- Non-blocking tick loop

✅ **Implementation:**
- Worker pool implementation (Task 2.2)
- Action queuing logic (Task 2.3)
- Timeout handling
- Registry injection pattern

**What Integrates with Hierarchical Composition:**

🔄 **Action IDs:**
- **BEFORE:** `workerID` (flat worker identifier)
- **AFTER:** `childName` (hierarchical child identifier)

🔄 **Action Context:**
- **NEW:** Actions can include parent worker ID for debugging
- **NEW:** Child actions inherit parent timeout constraints (optional)

🔄 **Metadata Tracking:**
- **NEW:** Track which child is executing action
- **NEW:** Track parent-child relationships in action logs

**Changes Required:**

**Minimal API Changes:**

**File:** `pkg/fsmv2/supervisor/action_executor.go`

**Lines to modify:** ~100-150 (action ID usage)

**BEFORE:**
```go
func (ae *ActionExecutor) EnqueueAction(
    workerID string,
    action Action,
    registry Registry,
) {
    ae.actionQueue <- &ActionRequest{
        ID:       workerID,
        Action:   action,
        Registry: registry,
    }
}

func (ae *ActionExecutor) HasActionInProgress(workerID string) bool {
    ae.mu.RLock()
    defer ae.mu.RUnlock()
    _, exists := ae.inProgress[workerID]
    return exists
}
```

**AFTER:**
```go
func (ae *ActionExecutor) EnqueueAction(
    childName string,  // ← Changed from workerID
    action Action,
    registry Registry,
) {
    ae.actionQueue <- &ActionRequest{
        ID:       childName,  // ← Child identifier
        Action:   action,
        Registry: registry,
    }
}

func (ae *ActionExecutor) HasActionInProgress(childName string) bool {
    ae.mu.RLock()
    defer ae.mu.RUnlock()
    _, exists := ae.inProgress[childName]
    return exists
}
```

**Impact:** Simple rename, no logic changes. Tests update `workerID` → `childName`.

**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Lines to modify:** Tick loop where actions are enqueued

**BEFORE:**
```go
for workerID, workerCtx := range s.workers {
    if s.actionExecutor.HasActionInProgress(workerID) {
        continue
    }
    s.actionExecutor.EnqueueAction(workerID, action, registry)
}
```

**AFTER:**
```go
for _, child := range s.children {
    if s.actionExecutor.HasActionInProgress(child.name) {
        continue
    }
    child.supervisor.Tick(ctx)  // Child ticks, may enqueue actions
}
```

**No Other Changes:**

- Worker pool logic unchanged
- Timeout handling unchanged
- Action execution unchanged
- Registry injection unchanged

**Estimated Impact:** 1-2 hours (simple rename + test updates)

---

### Phase 3 (Integration & Edge Cases) - HEAVILY UPDATED

Phase 3 originally focused on combining Infrastructure Supervision + Async Actions. With hierarchical composition, we now have THREE systems to integrate:

1. Infrastructure Supervision (circuit breaker)
2. Async Action Executor (global worker pool)
3. Hierarchical Composition (parent-child relationships)

**NEW Integration Points:**

#### Integration Point 1: Circuit Breaker + Child Supervisors

**Behavior:**

- **Circuit open** → Parent stops, but children keep running (reconcileChildren continues)
- **Parent restart** → All child supervisors must be recreated from ChildSpecs
- **Child restart** → Only that child restarts, siblings unaffected

**Test:**

**File:** `pkg/fsmv2/supervisor/integration_circuit_children_test.go` (NEW)

```go
Describe("Circuit Breaker + Child Supervisors", func() {
    It("should pause parent tick but keep reconciling children when circuit open", func() {
        // 1. Setup parent with 3 children
        // 2. Trigger circuit open via infrastructure failure
        // 3. Verify parent tick returns early
        // 4. Verify reconcileChildren still runs
        // 5. Add new child via ChildSpec update
        // 6. Verify new child created even though circuit open
        // 7. Close circuit
        // 8. Verify all children tick
    })

    It("should restart all children when parent restarts", func() {
        // 1. Setup parent with 3 children
        // 2. Trigger parent restart (circuit breaker escalation)
        // 3. Verify all children destroyed
        // 4. Verify parent calls DeriveDesiredState to get ChildSpecs
        // 5. Verify all children recreated via WorkerFactory
        // 6. Verify children have fresh supervisors (new IDs)
    })

    It("should restart only specific child when child fails", func() {
        // 1. Setup parent with 3 children
        // 2. Trigger child-specific failure (dfc_read)
        // 3. Verify only dfc_read restarted
        // 4. Verify siblings (redpanda, dfc_write) unaffected
        // 5. Verify parent tick continues for siblings
    })
})
```

---

#### Integration Point 2: Actions + Hierarchical Composition

**Behavior:**

- **Action on parent** → All children skip tick (parent is busy)
- **Action on child** → Only that child skips tick, siblings continue
- **Action timeout during child restart** → Action fails, retries when child recovers

**Test:**

**File:** `pkg/fsmv2/supervisor/integration_actions_children_test.go` (NEW)

```go
Describe("Actions + Hierarchical Composition", func() {
    It("should skip all children when parent has action in progress", func() {
        // 1. Setup parent with 3 children
        // 2. Enqueue long-running action on parent
        // 3. Tick supervisor
        // 4. Verify parent.DeriveDesiredState not called
        // 5. Verify NO children tick (all skipped)
        // 6. Wait for action to complete
        // 7. Verify children tick resumes
    })

    It("should skip only specific child when child has action in progress", func() {
        // 1. Setup parent with 3 children
        // 2. Enqueue action on child "dfc_read"
        // 3. Tick supervisor
        // 4. Verify dfc_read skips tick
        // 5. Verify siblings (redpanda, dfc_write) tick normally
        // 6. Wait for action to complete
        // 7. Verify dfc_read tick resumes
    })

    It("should handle action timeout during child restart gracefully", func() {
        // 1. Setup parent with child
        // 2. Enqueue action on child
        // 3. Trigger child restart (infrastructure failure)
        // 4. Verify action times out (can't reach restarted child)
        // 5. Verify action retries with backoff
        // 6. Verify action succeeds once child recovered
        // 7. Verify total recovery time < 90s
    })
})
```

---

#### Integration Point 3: Observations + Hierarchical Composition

**Behavior:**

- **Parent CollectObservedState** → Can read child states via `GetChildState()` for sanity checks
- **Child observations** → Independent of parent, collected every 5s
- **Circuit open** → Observations continue for all children (collectors run independently)

**Test:**

**File:** `pkg/fsmv2/supervisor/integration_observations_children_test.go` (NEW)

```go
Describe("Observations + Hierarchical Composition", func() {
    It("should allow parent to read child state for sanity checks", func() {
        // 1. Setup parent with children
        // 2. Parent CollectObservedState reads child states
        // 3. Verify parent can access redpanda state
        // 4. Verify parent can access benthos metrics
        // 5. Verify parent performs sanity check (redpanda vs benthos)
    })

    It("should collect child observations independently of parent", func() {
        // 1. Setup parent with children
        // 2. Verify each child has own collector
        // 3. Verify collectors write to TriangularStore every 5s
        // 4. Verify parent observations don't block child observations
        // 5. Verify child observations don't block parent observations
    })

    It("should continue collecting observations when circuit open", func() {
        // (Existing test from Task 3.4, now with children)
        // 1. Setup parent with children
        // 2. Trigger circuit open
        // 3. Verify parent observations continue
        // 4. Verify child observations continue
        // 5. Verify all observations written to store during circuit open
    })
})
```

---

### NEW Edge Cases

These are NEW edge cases that only exist with hierarchical composition:

#### NEW Task 3.5: Child Restart During Parent Action

**Scenario:**

1. Parent has long-running action in progress (e.g., configuration update)
2. Infrastructure Supervision detects child inconsistency
3. Circuit breaker restarts child while parent action is still running

**Expected Behavior:**

- Action continues execution (not cancelled)
- Child restarted independently
- Action may fail (can't access restarted child), but retries
- Action succeeds once child recovers

**Test:**

**File:** `pkg/fsmv2/supervisor/edge_case_child_restart_during_action_test.go` (NEW)

**TDD Breakdown:**

**Step 1: Write failing test**

```go
var _ = Describe("Child Restart During Parent Action", func() {
    It("should allow child restart while parent action runs", func() {
        // 1. Setup parent with child
        // 2. Enqueue 10s action on parent
        // 3. Wait 2s (action started)
        // 4. Trigger child restart via infrastructure failure
        // 5. Verify child restarted
        // 6. Verify action continues (not cancelled)
        // 7. Action completes (may fail on first try)
        // 8. Verify action retries
        // 9. Verify action succeeds after child recovers
        // 10. Verify total time < 30s
    })
})
```

**Step 2: Run test to verify it fails**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

**Expected:** FAIL (edge case not yet handled)

**Step 3: Write minimal implementation**

No code changes needed! This is already correct behavior:
- Circuit breaker runs in Tick() (separate from action execution)
- Actions execute in worker pool (async)
- RestartChild is independent of action state

**Step 4: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

**Expected:** PASS

**Step 5: Commit**

```bash
git add pkg/fsmv2/supervisor/edge_case_child_restart_during_action_test.go
git commit -m "test(fsmv2): verify child restart during parent action

Tests edge case where infrastructure restarts child while parent
has action in progress. Verifies:
- Child restart not blocked by parent action
- Action continues (not cancelled)
- Action may fail, retries, succeeds after recovery

Part of Integration & Edge Cases (Phase 3)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

**Estimated:** 2 hours (test + verification)

---

#### NEW Task 3.6: Parent Restart with Active Children

**Scenario:**

1. Parent circuit breaker escalates after max attempts
2. Parent supervisor restarts (complete reset)
3. Children have in-progress actions

**Expected Behavior:**

- Children cleaned up (supervisors destroyed)
- In-progress actions timeout (can't reach destroyed children)
- Parent recreates children from ChildSpecs
- Actions retry once children recreated
- Actions succeed once children recovered

**Test:**

**File:** `pkg/fsmv2/supervisor/edge_case_parent_restart_with_children_test.go` (NEW)

**TDD Breakdown:**

**Step 1: Write failing test**

```go
var _ = Describe("Parent Restart with Active Children", func() {
    It("should cleanup children and timeout actions when parent restarts", func() {
        // 1. Setup parent with 3 children
        // 2. Enqueue actions on each child
        // 3. Trigger parent restart (circuit breaker escalation)
        // 4. Verify all children destroyed
        // 5. Verify all actions timeout
        // 6. Verify parent calls DeriveDesiredState
        // 7. Verify parent recreates children from ChildSpecs
        // 8. Verify actions retry
        // 9. Verify actions succeed once children recovered
        // 10. Verify total recovery time < 120s
    })
})
```

**Step 2: Run test to verify it fails**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

**Expected:** FAIL (parent restart doesn't cleanup children)

**Step 3: Write minimal implementation**

**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Add parent restart logic:**
```go
func (s *Supervisor) Restart(ctx context.Context) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.logger.Infof("Restarting parent supervisor")

    // Cleanup all children
    for name, child := range s.children {
        s.logger.Infof("Stopping child %s before parent restart", name)
        child.supervisor.Stop(ctx)
    }

    // Clear children map
    s.children = make(map[string]*childInstance)

    // Reset circuit breaker
    s.circuitOpen = false
    s.infraHealthChecker.Backoff().Reset()

    // Recreate worker
    worker, err := s.factory.CreateWorker(s.workerType, s.userSpec)
    if err != nil {
        return fmt.Errorf("failed to recreate worker: %w", err)
    }

    s.worker = worker

    s.logger.Infof("Parent supervisor restarted successfully")
    return nil
}
```

**Step 4: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

**Expected:** PASS

**Step 5: Commit**

```bash
git add pkg/fsmv2/supervisor/supervisor.go pkg/fsmv2/supervisor/edge_case_parent_restart_with_children_test.go
git commit -m "feat(fsmv2): implement parent restart with child cleanup

When parent restarts (circuit breaker escalation):
- Cleanup all children (destroy supervisors)
- In-progress actions timeout
- Children recreated from ChildSpecs
- Actions retry and succeed after recovery

Part of Integration & Edge Cases (Phase 3)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

**Estimated:** 3 hours (new implementation + test)

---

#### NEW Task 3.7: Multi-Level Hierarchy Circuit Open

**Scenario:**

1. Grandparent → Parent → Child hierarchy (3 levels)
2. Grandparent circuit breaker opens
3. How does this affect parent and children?

**Expected Behavior:**

- Circuit propagates down hierarchy
- Grandparent stops ticking
- Parent doesn't tick (grandparent skipped it)
- Children don't tick (parent skipped them)
- All restart from top when circuit closes

**Test:**

**File:** `pkg/fsmv2/supervisor/edge_case_multilevel_circuit_test.go` (NEW)

**TDD Breakdown:**

**Step 1: Write failing test**

```go
var _ = Describe("Multi-Level Hierarchy Circuit Open", func() {
    It("should propagate circuit open down hierarchy", func() {
        // 1. Setup Grandparent → Parent → Child hierarchy
        // 2. Trigger grandparent circuit open
        // 3. Verify grandparent tick returns early
        // 4. Verify parent NOT ticked (grandparent skipped it)
        // 5. Verify children NOT ticked (parent didn't tick)
        // 6. Verify reconcileChildren still runs at all levels
        // 7. Close circuit
        // 8. Verify all levels tick (top-down)
    })

    It("should restart from top when multi-level circuit closes", func() {
        // 1. Setup 3-level hierarchy
        // 2. Trigger grandparent circuit open
        // 3. Fix infrastructure failure
        // 4. Close circuit
        // 5. Verify grandparent ticks first
        // 6. Verify parent ticks second (via grandparent)
        // 7. Verify children tick third (via parent)
        // 8. Verify all levels reach consistent state
    })

    It("should handle child-level circuit without affecting grandparent", func() {
        // 1. Setup 3-level hierarchy
        // 2. Trigger child-level circuit open
        // 3. Verify only child stops
        // 4. Verify parent continues ticking
        // 5. Verify grandparent continues ticking
        // 6. Verify child restarts independently
    })
})
```

**Step 2: Run test to verify it fails**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

**Expected:** FAIL (multi-level hierarchy not tested)

**Step 3: Write minimal implementation**

No code changes needed! Each supervisor has own circuit breaker, and parent tick calls `child.supervisor.Tick()`. This naturally propagates circuit state.

**Step 4: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

**Expected:** PASS

**Step 5: Commit**

```bash
git add pkg/fsmv2/supervisor/edge_case_multilevel_circuit_test.go
git commit -m "test(fsmv2): verify multi-level hierarchy circuit behavior

Tests circuit breaker behavior in deep hierarchies:
- Circuit propagates down levels (grandparent stops all)
- Circuit at child level doesn't affect parent/grandparent
- Restart from top rebuilds hierarchy correctly

Part of Integration & Edge Cases (Phase 3)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

**Estimated:** 2 hours (test + verification)

---

## Summary of Changes

### Phase 1 (Infrastructure Supervision)

**Modified Tasks:**
- Task 1.2: InfrastructureHealthChecker - No structural changes, accesses `s.children` map
- Task 1.3: CheckChildConsistency - Use `s.children["name"]` instead of `GetChildState()`
- Task 1.4: Circuit Breaker - Doesn't block `reconcileChildren()`, uses RestartChild

**New Tasks:**
- Task 1.5: RestartChild - Clean child recovery via WorkerFactory (4 hours)

**Total Addition:** +4 hours (1 new task)

---

### Phase 2 (Async Action Executor)

**Changes:**
- Rename `workerID` → `childName` in API
- Update tests for new naming

**No Structural Changes:**
- Worker pool unchanged
- Timeout handling unchanged
- Action execution unchanged

**Total Addition:** +1-2 hours (renaming + test updates)

---

### Phase 3 (Integration & Edge Cases)

**New Integration Tests:**
- Circuit Breaker + Child Supervisors (2 hours)
- Actions + Hierarchical Composition (2 hours)
- Observations + Hierarchical Composition (1 hour)

**New Edge Case Tasks:**
- Task 3.5: Child Restart During Parent Action (2 hours)
- Task 3.6: Parent Restart with Active Children (3 hours)
- Task 3.7: Multi-Level Hierarchy Circuit Open (2 hours)

**Total Addition:** +12 hours (3 integration tests + 3 edge cases)

---

### Overall Impact

**Original Phase 1:** 2 weeks
**Modified Phase 1:** 2 weeks + 4 hours = **2.2 weeks**

**Original Phase 2:** 2 weeks
**Modified Phase 2:** 2 weeks + 2 hours = **2.1 weeks**

**Original Phase 3:** 1 week
**Modified Phase 3:** 1 week + 12 hours = **2.5 weeks**

**Total Increase:** +18 hours (~2.2 weeks)

**New Timeline for Phases 1-3:** 6.8 weeks (from 5 weeks originally)

**Note:** This ONLY covers modifications to existing phases. Phase 0 (Hierarchical Composition) and Phase 0.5 (Templating & Variables) are NEW phases that add 4 weeks to the overall timeline (see Section 2 of main change proposal).

---

