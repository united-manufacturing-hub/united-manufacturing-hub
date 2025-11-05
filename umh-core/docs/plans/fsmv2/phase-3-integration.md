# Phase 3: Integration & Edge Cases

**Timeline:** Weeks 9-10
**Effort:** ~330 lines of code
**Dependencies:** Phases 0, 0.5, 1, 2 (all must complete)
**Blocks:** None

---

## Overview

Phase 3 integrates all FSMv2 features into a cohesive system and tests edge cases. This phase combines hierarchical composition, templating, infrastructure supervision, and async actions into the complete Supervisor.Tick() loop.

**Complete Tick Loop:**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PHASE 1: Infrastructure Health Check (from Phase 1)
    // NOTE (Task 3.4): Circuit breaker only affects tick loop execution.
    // Collectors run independently (started once in supervisor.Start()).
    // Collectors call worker.CollectObservedState() every second regardless of circuit state.
    // TriangularStore receives continuous updates even when circuit is open.
    // When circuit closes, LoadSnapshot() returns fresh data (no staleness penalty).
    // See: docs/design/fsmv2-child-observed-state-usage.md#collector-independence-from-circuit-breaker
    if err := s.healthChecker.CheckChildConsistency(s.children); err != nil {
        s.circuitOpen = true
        if failedChild, exists := s.children[err.ChildName]; exists {
            failedChild.supervisor.Restart()
        }
        return nil  // ← EARLY RETURN: No new actions derived, but collectors still running
    }

    // PHASE 2: Derive Desired State (from Phase 0)
    userSpec := s.store.GetUserSpec()

    // Inject Global and Internal variables (from Phase 0.5)
    userSpec.Variables.Global = s.globalVars
    userSpec.Variables.Internal = map[string]any{
        "id":         s.supervisorID,
        "created_at": s.createdAt,
    }

    desiredState := s.worker.DeriveDesiredState(userSpec)

    // PHASE 3: Reconcile Children (from Phase 0)
    s.reconcileChildren(desiredState.ChildrenSpecs)

    // PHASE 4: Apply State Mapping (from Phase 0)
    for name, child := range s.children {
        if mappedState, ok := child.stateMapping[desiredState.State]; ok {
            child.supervisor.SetDesiredState(DesiredState{State: mappedState})
        }
    }

    // PHASE 5: Tick Children (from Phase 0)
    for _, child := range s.children {
        child.supervisor.Tick(ctx)
    }

    // PHASE 6: Async Actions (from Phase 2)
    if s.actionExecutor.HasActionInProgress(s.supervisorID) {
        return nil
    }

    // NOTE (Task 3.2): In-progress actions are NOT cancelled when circuit opens.
    // Actions continue execution, may timeout after 30s (ActionExecutorConfig.ActionTimeout),
    // and retry with exponential backoff. This is acceptable for rare infrastructure restarts.
    // See: docs/design/fsmv2-infrastructure-supervision-patterns.md#action-behavior-during-circuit-breaker

    // NOTE (Task 3.4): s.store.GetObservedState() reads from TriangularStore,
    // which is continuously updated by collectors (even during circuit open).
    // This ensures fresh observations are available immediately when circuit closes.
    action := s.worker.NextAction(desiredState, s.store.GetObservedState())
    s.actionExecutor.EnqueueAction(s.supervisorID, action, s.registry)

    return nil
}
```

---

## Deliverables

### 1. Combined Tick Loop (Task 3.1)
- Integrate all phases into single tick function
- Correct ordering: Infrastructure → Deriv → Reconcile → StateMapping → ChildTick → Actions
- Error handling at each phase

### 2. Edge Case: Action During Child Restart (Task 3.2)
**From Original Plan**
- Scenario: Child restarts while action in progress
- Behavior: Cancel action, wait for restart to complete
- Test: Verify action cancellation and retry

### 3. Edge Case: Observation During Circuit Open (Task 3.4)
**From Original Plan**
- Scenario: Circuit open, but observations still needed
- Behavior: CollectObservedState() still runs
- Test: Verify observations collected during circuit open

### 4. NEW: Template Rendering Performance Tests
- Scenario: 100+ children with templates
- Behavior: Template rendering <5ms per child
- Test: Benchmark template rendering at scale

### 5. NEW: Variable Flow Tests (Parent → Child)
- Scenario: Parent variables propagate to children
- Behavior: Child receives parent context
- Test: 3-level hierarchy (grandparent → parent → child)

### 6. NEW: Location Hierarchy Tests
- Scenario: ISA-95 location path computation
- Behavior: Parent + child locations merge correctly
- Test: Verify location_path in VariableBundle

### 7. NEW: ProtocolConverter End-to-End Test
- Scenario: Real-world ProtocolConverter migration
- Behavior: ProtocolConverter → Connection + SourceFlow + SinkFlow
- Test: Full tick cycle with all features

---

## Gap Analysis from Master Plan

This section documents the detailed acceptance criteria and edge case handling from the original master plan.

### Edge Case: Action Behavior During Child Restart (Task 3.2)

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

### Edge Case: Observation Collection During Circuit Open (Task 3.4)

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

## Integration Test Scenarios

### Existing Scenarios (from Original Plan)

**Task 3.2: Action Behavior During Child Restart** (lines 1260-1396)
- Verify action cancellation when child restarts
- Ensure action retries after restart completes

**Task 3.4: Observation Collection During Circuit Open** (lines 1524-1650)
- Verify observations collected even when circuit open
- Ensure TriangularStore updated with observed state

### NEW Scenarios (from Change Proposal)

**Scenario 1: Template Rendering Performance**
```go
It("should render 100 templates in <500ms", func() {
    parent := createProtocolConverter(100) // 100 child flows

    start := time.Now()
    desiredState := parent.DeriveDesiredState(userSpec)
    elapsed := time.Since(start)

    Expect(elapsed).To(BeNumerically("<", 500*time.Millisecond))
    Expect(desiredState.ChildrenSpecs).To(HaveLen(100))
})
```

**Scenario 2: Variable Propagation (3-Level Hierarchy)**
```go
It("should propagate variables through 3-level hierarchy", func() {
    // Grandparent: ProtocolConverter
    // Parent: Connection
    // Child: SourceFlow

    grandparent.Variables.User["IP"] = "192.168.1.100"

    grandparent.Tick(ctx)
    parent := grandparent.children["connection"]
    parent.Tick(ctx)
    child := parent.children["source_flow"]

    // Child receives grandparent's IP variable
    Expect(child.Variables.User["IP"]).To(Equal("192.168.1.100"))
})
```

**Scenario 3: Location Hierarchy**
```go
It("should compute location_path correctly", func() {
    parent.Location = []LocationLevel{
        {Enterprise: "ACME"},
        {Site: "Factory-1"},
    }

    child.Location = []LocationLevel{
        {Line: "Line-A"},
        {Cell: "Cell-5"},
    }

    parent.Tick(ctx)
    childSupervisor := parent.children["child"]

    locationPath := childSupervisor.Variables.Internal["location_path"]
    Expect(locationPath).To(Equal("ACME.Factory-1.Line-A.Cell-5"))
})
```

**Scenario 4: ProtocolConverter End-to-End**
```go
It("should execute full ProtocolConverter tick cycle", func() {
    pc := createProtocolConverter()

    // Tick 1: Create children (Connection + SourceFlow + SinkFlow)
    pc.Tick(ctx)
    Expect(pc.children).To(HaveLen(3))

    // Tick 2: Apply state mapping (idle → stopped)
    pc.SetDesiredState(DesiredState{State: "idle"})
    pc.Tick(ctx)
    Expect(pc.children["connection"].observedState.State).To(Equal("stopped"))

    // Tick 3: Render templates with variables
    connection := pc.children["connection"]
    sourceFlow := pc.children["source_flow"]
    Expect(sourceFlow.userSpec.Config).To(ContainSubstring("192.168.1.100"))

    // Tick 4: Async actions execute
    pc.SetDesiredState(DesiredState{State: "active"})
    pc.Tick(ctx)
    Eventually(func() string {
        return pc.observedState.State
    }).Should(Equal("running"))
})
```

---

## Detailed Implementation

**For original integration tasks**, see the master plan:
**[../2025-11-02-fsmv2-supervision-and-async-actions.md](../2025-11-02-fsmv2-supervision-and-async-actions.md)** (lines 1260-1650)

**For updated integration with hierarchical composition**, see:
**[archive/fsmv2-change-proposal-section6-7.md](archive/fsmv2-change-proposal-section6-7.md)** (lines 300-600)

---

## Task Breakdown

### Task 3.1: Combined Tick Loop (8 hours)
**Effort:** 6 hours integration + 2 hours testing
**LOC:** ~80 lines (tick function + tests)

### Task 3.2: Action During Child Restart (4 hours)
**Effort:** 3 hours implementation + 1 hour testing
**LOC:** ~60 lines (from original plan)

### Task 3.4: Observation During Circuit Open (4 hours)
**Effort:** 3 hours implementation + 1 hour testing
**LOC:** ~60 lines (from original plan)

### Task 3.5: Template Rendering Performance (4 hours)
**Effort:** 2 hours implementation + 2 hours testing
**LOC:** ~30 lines (benchmarks)

### Task 3.6: Variable Flow Tests (4 hours)
**Effort:** 2 hours implementation + 2 hours testing
**LOC:** ~40 lines (integration tests)

### Task 3.7: Location Hierarchy Tests (3 hours)
**Effort:** 2 hours implementation + 1 hour testing
**LOC:** ~30 lines (integration tests)

### Task 3.8: ProtocolConverter End-to-End (5 hours)
**Effort:** 3 hours implementation + 2 hours testing
**LOC:** ~50 lines (system test)

**Total:** 32 hours (4 days) → Buffer to 2 weeks

---

## Acceptance Criteria

### Functionality
- [ ] Complete tick loop integrates all phases
- [ ] Actions cancelled during child restart
- [ ] Observations collected during circuit open
- [ ] Templates render correctly at scale
- [ ] Variables propagate through hierarchy
- [ ] Location paths computed correctly
- [ ] ProtocolConverter end-to-end works

### Quality
- [ ] Integration test coverage >90%
- [ ] All edge cases tested
- [ ] No focused specs
- [ ] System tests pass

### Performance
- [ ] Complete tick cycle <10ms (3-level hierarchy)
- [ ] Template rendering <5ms per child
- [ ] 100+ children supported

### Documentation
- [ ] Tick loop phases documented
- [ ] Integration points documented
- [ ] Edge case handling documented

---

## Review Checkpoint

**After Phase 3 completion:**
- Verify all phases integrate correctly
- Confirm edge cases handled gracefully
- Check performance under load
- Validate ProtocolConverter migration path

**Next Phase:** [Phase 4: Monitoring & Observability](phase-4-monitoring.md)
