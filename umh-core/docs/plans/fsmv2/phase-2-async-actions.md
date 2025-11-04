# Phase 2: Async Action Executor

**Timeline:** Weeks 7-8
**Effort:** ~260 lines of code
**Dependencies:** Phase 0 (actions queue per child)
**Blocks:** None

---

## Overview

Phase 2 implements the global worker pool pattern for async action execution. Actions (Start, Stop, Restart) execute non-blocking in background goroutines, preventing tick loop blockage during slow operations.

**Key Pattern:**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // ...infrastructure health check...

    // Use child name as action ID (Phase 0 integration)
    actionID := s.supervisorID // For parent supervisor
    if child != nil {
        actionID = child.Name // For child supervisor
    }

    // Check if action already in progress
    if s.actionExecutor.HasActionInProgress(actionID) {
        return nil // Skip derivation
    }

    // Derive next action
    action := currentState.Next(snapshot)

    // Queue action (non-blocking)
    s.actionExecutor.EnqueueAction(actionID, action, registry)

    return nil
}
```

**Updates from Original Plan:**
- Action IDs use child name (not hardcoded worker ID)
- Otherwise identical to original Phase 2

---

## Deliverables

### 1. ActionExecutor with Global Worker Pool (Task 2.1)
- Fixed-size goroutine pool (configurable, default 10)
- Non-blocking action queueing
- Concurrent action execution
- Context cancellation support

### 2. Integration with Supervisor.Tick() (Task 2.2)
- Check action in progress → skip derivation
- Enqueue action via ActionExecutor
- Action registry for action execution

### 3. Action Timeout Handling (Task 2.3)
- Configurable timeouts per action type
- Cancel action on timeout
- Mark action as failed
- Example: Start timeout = 30s, Stop timeout = 10s

### 4. Non-Blocking Guarantees (Task 2.4)
- Tick never blocks on action execution
- Actions execute in background
- Result checked on next tick

---

## Integration with Hierarchical Composition

**BEFORE (Original Plan):**
```go
// Hardcoded worker ID
if s.actionExecutor.HasActionInProgress("worker-123") {
    return nil
}

s.actionExecutor.EnqueueAction("worker-123", action, registry)
```

**AFTER (Updated for Phase 0):**
```go
// Use child name from supervisor
childID := s.supervisorID // For parent supervisor
if child != nil {
    childID = child.Name // For child supervisor
}

if s.actionExecutor.HasActionInProgress(childID) {
    return nil
}

s.actionExecutor.EnqueueAction(childID, action, registry)
```

**Key Changes:**
- Action IDs use child name (hierarchical identifier)
- Each child has separate action queue
- Parent and children can execute actions concurrently

---

## Detailed Implementation

**For complete TDD task breakdowns**, see the original master plan:
**[../2025-11-02-fsmv2-supervision-and-async-actions.md](../2025-11-02-fsmv2-supervision-and-async-actions.md)** (lines 674-1258)

That document includes:

**Task 2.1: ActionExecutor with Global Worker Pool** (lines 674-900)
- RED: Write tests for worker pool behavior
- GREEN: Implement goroutine pool and action queueing
- REFACTOR: Add context cancellation
- Commit: "feat(fsmv2): add ActionExecutor with global worker pool"

**Task 2.2: Integration with Supervisor** (lines 902-1024)
- RED: Write tests for tick loop integration
- GREEN: Integrate ActionExecutor into Supervisor.Tick()
- REFACTOR: Extract action ID logic
- Commit: "feat(fsmv2): integrate ActionExecutor with supervisor tick"

**Task 2.3: Action Timeout Handling** (lines 1026-1152)
- RED: Write tests for timeout scenarios
- GREEN: Implement timeout cancellation
- REFACTOR: Add configurable timeouts
- Commit: "feat(fsmv2): add action timeout handling"

**Task 2.4: Non-Blocking Guarantees** (lines 1154-1258)
- RED: Write tests for non-blocking behavior
- GREEN: Ensure tick never blocks
- REFACTOR: Add action status checks
- Commit: "feat(fsmv2): ensure tick loop non-blocking guarantees"

---

## Phase 0 API Integration Updates

**Key Changes from Original Plan:**

The original master plan used hardcoded worker IDs like `"worker-123"`. Phase 2 now integrates with Phase 0's child naming system for action identification.

### Action ID Strategy

**BEFORE (master plan):**
```go
// Hardcoded worker ID
if s.actionExecutor.HasActionInProgress("worker-123") {
    return nil
}

s.actionExecutor.EnqueueAction("worker-123", action, registry)
```

**AFTER (Phase 0 integration):**
```go
// Use child name from supervisor
childID := s.supervisorID // For parent supervisor
if child != nil {
    childID = child.Name // For child supervisor
}

if s.actionExecutor.HasActionInProgress(childID) {
    return nil
}

s.actionExecutor.EnqueueAction(childID, action, registry)
```

### Hierarchical Action Execution

With Phase 0's hierarchical composition:

1. **Each child has separate action queue** - Identified by child name
2. **Parent and children execute concurrently** - No blocking between hierarchy levels
3. **Action IDs are hierarchical** - Matches child naming structure from Phase 0

**Example hierarchy:**
```go
// Parent supervisor (protocol converter)
parentID := "protocol_converter_123"
s.actionExecutor.EnqueueAction(parentID, startAction, registry)

// Child supervisor (connection)
childID := "connection"
child.supervisor.actionExecutor.EnqueueAction(childID, connectAction, registry)

// Both actions execute concurrently in worker pool
```

### Impact on TDD Examples

The TDD cycles in the master plan (lines 674-1258) remain structurally valid, but:

1. **Task 2.1 (ActionExecutor Implementation)** - Action IDs use child names, not hardcoded strings
2. **Task 2.2 (Supervisor Integration)** - Extract `actionID` from supervisor/child context
3. **Task 2.3 (Timeout Handling)** - Timeouts tracked per child name
4. **Task 2.4 (Non-Blocking Guarantees)** - Each child's action queue is independent

**For complete TDD implementation**, follow the master plan task breakdowns with these API updates applied.

---

## Task Breakdown

### Task 2.1: ActionExecutor Implementation (6 hours)
**Effort:** 4 hours implementation + 2 hours testing
**LOC:** ~100 lines (executor + pool + tests)

### Task 2.2: Supervisor Integration (4 hours)
**Effort:** 3 hours integration + 1 hour testing
**LOC:** ~60 lines (tick loop update + tests)

### Task 2.3: Timeout Handling (3 hours)
**Effort:** 2 hours implementation + 1 hour testing
**LOC:** ~40 lines (timeout logic + tests)

### Task 2.4: Non-Blocking Guarantees (3 hours)
**Effort:** 2 hours verification + 1 hour testing
**LOC:** ~60 lines (tests + documentation)

**Total:** 16 hours (2 days) → Buffer to 2 weeks

---

## Acceptance Criteria

### Functionality
- [ ] Actions execute in global worker pool (non-blocking)
- [ ] Tick never blocks on action execution
- [ ] Actions time out after configured duration
- [ ] Action status queryable (in progress, completed, failed)
- [ ] Multiple workers can execute actions concurrently

### Quality
- [ ] Unit test coverage >80%
- [ ] Integration tests cover action execution scenarios
- [ ] No focused specs
- [ ] Ginkgo tests use BDD style

### Performance
- [ ] Action queueing <1ms
- [ ] Tick latency <1ms (non-blocking)
- [ ] Worker pool utilization >80% under load

### Documentation
- [ ] Worker pool pattern documented
- [ ] Action timeout configuration documented
- [ ] Non-blocking guarantees documented

---

## Review Checkpoint

**After Phase 2 completion:**
- Verify tick never blocks (even with 100+ concurrent actions)
- Confirm action timeouts work correctly
- Check worker pool size configuration
- Validate action status reporting

**Next Phase:** [Phase 3: Integration & Edge Cases](phase-3-integration.md)
