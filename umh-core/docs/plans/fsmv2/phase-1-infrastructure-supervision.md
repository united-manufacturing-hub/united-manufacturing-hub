# Phase 1: Infrastructure Supervision Foundation

**Timeline:** Weeks 5-6
**Effort:** ~320 lines of code
**Dependencies:** Phase 0 (needs children map)
**Blocks:** None

---

## Overview

Phase 1 implements the circuit breaker pattern for infrastructure health monitoring. When infrastructure components fail (Redpanda, S3, network), the circuit opens to prevent cascading failures and allow graceful recovery.

**Key Pattern:**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // PHASE 1: Infrastructure health check (priority 1)
    if err := s.healthChecker.CheckChildConsistency(s.children); err != nil {
        s.circuitOpen = true

        // Restart failed child (identified by health checker)
        if failedChild, exists := s.children[err.ChildName]; exists {
            failedChild.supervisor.Restart()
        }

        return nil // Skip rest of tick
    }

    // Circuit closed → proceed with normal operations
    // ...
}
```

**Updates from Original Plan:**
- CheckChildConsistency() uses `s.children` map (not hardcoded names)
- Child restart uses `s.children[name].supervisor.Restart()`
- Otherwise identical to original Phase 1

---

## Deliverables

### 1. InfrastructureHealthChecker (Task 1.2)
- Child consistency checks (children exist and are healthy)
- Sanity checks (Redpanda connectivity, storage availability)
- Example: Benthos input requires Redpanda → check Redpanda health

### 2. Circuit Breaker Logic (Task 1.4)
- Circuit opens on infrastructure failure
- Skip deriv/actions when circuit open
- Restart failed child with exponential backoff
- Circuit closes when infrastructure recovers

### 3. ExponentialBackoff (Task 1.1)
- Generic backoff calculator
- Configurable: initialDelay, maxDelay, multiplier
- Prevents thundering herd during recovery

---

## Integration with Hierarchical Composition

**BEFORE (Original Plan):**
```go
// Hardcoded child names
if err := s.healthChecker.CheckChildConsistency("redpanda", "benthos_dfc"); err != nil {
    // ...
}
```

**AFTER (Updated for Phase 0):**
```go
// Use children map from Phase 0
if err := s.healthChecker.CheckChildConsistency(s.children); err != nil {
    s.circuitOpen = true

    // Restart failed child (identified by health checker)
    if failedChild, exists := s.children[err.ChildName]; exists {
        failedChild.supervisor.Restart()
    }

    return nil
}
```

**Key Changes:**
- `s.children` map (from Phase 0) passed to health checker
- Health checker returns which child failed
- Restart uses child supervisor (not hardcoded logic)

---

## Detailed Implementation

**For complete TDD task breakdowns**, see the original master plan:
**[../2025-11-02-fsmv2-supervision-and-async-actions.md](../2025-11-02-fsmv2-supervision-and-async-actions.md)** (lines 219-672)

That document includes:

**Task 1.1: ExponentialBackoff Implementation** (lines 219-363)
- RED: Write tests for backoff calculation
- GREEN: Implement NextDelay() and Reset()
- REFACTOR: Add configurable limits
- Commit: "feat(fsmv2): add ExponentialBackoff utility"

**Task 1.2: InfrastructureHealthChecker** (lines 365-528)
- RED: Write tests for child health checks
- GREEN: Implement CheckChildConsistency()
- REFACTOR: Extract sanity check examples
- Commit: "feat(fsmv2): add InfrastructureHealthChecker"

**Task 1.3: Supervisor Circuit Breaker** (lines 530-672)
- RED: Write tests for circuit breaker scenarios
- GREEN: Integrate health checker into tick loop
- REFACTOR: Add child restart logic
- Commit: "feat(fsmv2): add circuit breaker to supervisor tick"

---

## Phase 0 API Integration Updates

**Key Changes from Original Plan:**

The original master plan used hardcoded child names. Phase 1 now integrates with Phase 0's hierarchical composition APIs:

### Updated Health Checker Signature

**BEFORE (master plan):**
```go
func (h *InfrastructureHealthChecker) CheckChildConsistency() error {
    // Hardcoded child checks
}
```

**AFTER (Phase 0 integration):**
```go
func (h *InfrastructureHealthChecker) CheckChildConsistency(children map[string]*ChildSupervisor) error {
    for name, child := range children {
        if !child.supervisor.IsHealthy() {
            return &ChildHealthError{ChildName: name}
        }
    }
    return nil
}
```

### Updated Child Restart Pattern

**BEFORE (master plan):**
```go
s.restartChild(ctx, "dfc_read") // Hardcoded name
```

**AFTER (Phase 0 integration):**
```go
if failedChild, exists := s.children[err.ChildName]; exists {
    failedChild.supervisor.Restart() // Uses Phase 0 API
}
```

### Impact on TDD Examples

The TDD cycles in the master plan (lines 219-672) remain structurally valid, but:

1. **Task 1.2 (InfrastructureHealthChecker)** - Update signature to accept `children map[string]*ChildSupervisor`
2. **Task 1.3 (Circuit Breaker Integration)** - Use `s.children` map instead of hardcoded names
3. All test mocks should use Phase 0's `ChildSupervisor` structure

**For complete TDD implementation**, follow the master plan task breakdowns with these API updates applied.

---

## Task Breakdown

### Task 1.1: ExponentialBackoff (3 hours)
**Effort:** 2 hours implementation + 1 hour testing
**LOC:** ~40 lines (utility + tests)

### Task 1.2: InfrastructureHealthChecker (6 hours)
**Effort:** 4 hours implementation + 2 hours testing
**LOC:** ~120 lines (checker + sanity checks + tests)

### Task 1.3: Circuit Breaker Integration (6 hours)
**Effort:** 4 hours integration + 2 hours testing
**LOC:** ~160 lines (tick loop update + tests)

**Total:** 15 hours (2 days) → Buffer to 2 weeks

---

## Acceptance Criteria

### Functionality
- [ ] Circuit opens on infrastructure failure
- [ ] Circuit remains open until infrastructure recovers
- [ ] Failed children restart with exponential backoff
- [ ] Circuit closes when all children healthy
- [ ] Deriv/actions skipped when circuit open

### Quality
- [ ] Unit test coverage >80%
- [ ] Integration tests cover infrastructure failure scenarios
- [ ] No focused specs
- [ ] Ginkgo tests use BDD style

### Performance
- [ ] Health check <5ms per tick
- [ ] Circuit breaker adds <1ms latency
- [ ] Backoff calculation O(1) time

### Documentation
- [ ] Circuit breaker pattern documented
- [ ] Health check integration points documented
- [ ] Design docs updated

---

## Review Checkpoint

**After Phase 1 completion:**
- Verify circuit breaker prevents cascading failures
- Confirm health checker works with dynamic children (from Phase 0)
- Check exponential backoff prevents thundering herd
- Validate tick loop structure (health check → deriv → actions)

**Next Phase:** [Phase 2: Async Action Executor](phase-2-async-actions.md)
