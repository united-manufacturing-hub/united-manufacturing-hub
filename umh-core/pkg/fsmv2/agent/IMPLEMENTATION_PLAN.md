# Agent FSM v2 Log Handling - Detailed Implementation Plan

## Executive Summary

Implement efficient log handling for Agent FSM v2 that only collects **new logs** since last observation, preventing unbounded memory growth while maintaining compatibility with existing FSM v1.

## Problem Statement

Current implementation collects ALL logs on every tick:
- Memory grows unbounded over time
- Every `SaveObserved()` writes entire log history
- No way to get only new logs since last collection
- Risk of interference with existing FSM v1 that may rely on full log behavior

## Solution Overview

Add "incremental log collection" capability to agent_monitor service:
1. Track last collected log position
2. Return only new logs since that position
3. Use flag to maintain backward compatibility with FSM v1
4. Store logs separately in CSE pattern (future phase)

---

## Stage 1: Planning & Research

### 1.1 Understand Current Log Collection Flow

**Current Flow:**
```
FSM v2 Worker → agent_monitor.Status() → s6.GetLogs() → ALL logs returned
```

**Key Components:**
- `pkg/service/agent_monitor/agent.go:232` - `getAgentLogs()` method
- `pkg/service/s6/s6.go` - S6 service log reading
- `pkg/service/filesystem/` - File system operations

### 1.2 Research Questions (for Subagent)

1. **How does S6 service track log position?**
   - Does it already have concept of "new" vs "old" logs?
   - How are log rotations handled?
   - What about `.s` (rotated) vs `current` files?

2. **FSM v1 Dependencies:**
   - Which FSM v1 components use agent_monitor logs?
   - Do they expect full log history?
   - Can we safely add optional behavior?

3. **Log File Structure:**
   - TAI64N timestamp format parsing
   - Log rotation boundaries
   - How to track "last read position"

### 1.3 Design Decisions

**Option A: Stateful Tracking in Worker**
- Worker tracks last log timestamp/position
- Requests only logs after that position
- Pros: Simple, isolated to FSM v2
- Cons: State management complexity

**Option B: Stateless with Timestamp Parameter**
- Pass "since" timestamp to Status()
- Return logs newer than timestamp
- Pros: Stateless, clean API
- Cons: Timestamp sync issues

**Option C: Offset-Based Collection**
- Track byte offset or line number
- Read from last offset forward
- Pros: Precise, handles rotations
- Cons: Complex with file rotations

---

## Stage 2: Implementation (TDD)

### 2.1 Test-First Development Plan

#### Phase 1: Service Layer Tests
```go
// agent_monitor_test.go
Describe("Incremental Log Collection", func() {
    It("should return only new logs when incremental flag is set")
    It("should return all logs when incremental flag is false (backward compat)")
    It("should handle log rotation boundaries correctly")
    It("should reset position after service restart")
})
```

#### Phase 2: Integration Tests
```go
// fsmv2/agent/worker_test.go
Describe("Worker with Incremental Logs", func() {
    It("should only store new logs in observed state")
    It("should accumulate logs across multiple ticks")
    It("should handle empty log periods")
})
```

### 2.2 Implementation Steps

#### Step 1: Extend IAgentMonitorService Interface
```go
type IAgentMonitorService interface {
    Status(ctx context.Context, systemSnapshot fsm.SystemSnapshot) (*ServiceInfo, error)
    // New method for incremental collection
    StatusIncremental(ctx context.Context, systemSnapshot fsm.SystemSnapshot, since time.Time) (*ServiceInfo, error)
}
```

#### Step 2: Add Tracking to AgentMonitorService
```go
type AgentMonitorService struct {
    // ... existing fields
    lastLogTimestamp time.Time  // Track last collected log
    incrementalMode  bool        // Enable incremental collection
}
```

#### Step 3: Modify getAgentLogs()
```go
func (c *AgentMonitorService) getAgentLogs(ctx context.Context, since *time.Time) ([]s6.LogEntry, error) {
    if since != nil && c.incrementalMode {
        // Get only logs newer than 'since'
        return c.s6Service.GetLogsSince(ctx, servicePath, c.fs, *since)
    }
    // Backward compatible: get all logs
    return c.s6Service.GetLogs(ctx, servicePath, c.fs)
}
```

#### Step 4: Update Worker
```go
type AgentMonitorWorker struct {
    // ... existing fields
    lastLogTimestamp time.Time  // Track for incremental collection
}

func (w *AgentMonitorWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // Use incremental API if available
    serviceInfo, err := w.monitorService.StatusIncremental(ctx, fsm.SystemSnapshot{}, w.lastLogTimestamp)
    // Update last timestamp
    if len(serviceInfo.AgentLogs) > 0 {
        w.lastLogTimestamp = serviceInfo.AgentLogs[len(serviceInfo.AgentLogs)-1].Timestamp
    }
    // ... rest of logic
}
```

---

## Stage 3: Code Review Checklist

### 3.1 Backward Compatibility
- [ ] FSM v1 continues to receive full logs
- [ ] No breaking changes to existing interfaces
- [ ] Feature flag for incremental mode

### 3.2 Edge Cases
- [ ] Empty log periods handled
- [ ] Log rotation during collection
- [ ] Service restart resets position
- [ ] Timestamp precision (nanoseconds)
- [ ] Clock skew handling

### 3.3 Performance
- [ ] Memory usage stays constant over time
- [ ] No regression for FSM v1 performance
- [ ] Efficient log file seeking

### 3.4 Testing
- [ ] Unit tests for all new methods
- [ ] Integration tests with real S6 logs
- [ ] Benchmark tests for large log files
- [ ] Idempotency tests for retries

---

## Stage 4: Future Enhancements

### 4.1 CSE Storage Pattern (Phase 2)
```sql
CREATE TABLE agent_monitor_log_positions (
    worker_id TEXT PRIMARY KEY,
    last_timestamp INTEGER NOT NULL,
    last_offset INTEGER,
    last_file TEXT
);

CREATE TABLE agent_monitor_logs (
    -- As designed in TODO.md
);
```

### 4.2 Log Appender Service
- Separate goroutine for log collection
- Direct write to SQLite
- No memory accumulation in FSM

### 4.3 Monitoring & Alerting
- Metrics for log lag
- Alert on collection failures
- Dashboard for log throughput

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|---------|------------|
| Breaking FSM v1 | High | Feature flag, thorough testing |
| Log rotation edge cases | Medium | Robust file tracking, recovery logic |
| Performance regression | Low | Benchmark tests, profiling |
| Clock skew issues | Low | Use monotonic timestamps where possible |

---

## Success Criteria

1. **Memory Efficiency**: Agent FSM v2 memory usage remains constant over time
2. **Backward Compatibility**: FSM v1 continues working unchanged
3. **Performance**: Log collection <100ms for typical workload
4. **Reliability**: No log loss during rotations or restarts
5. **Testability**: 100% test coverage for new code paths

---

## Timeline Estimate

- **Stage 1 (Research)**: 2-3 hours (subagent investigation)
- **Stage 2 (Implementation)**: 4-6 hours (TDD with tests)
- **Stage 3 (Review)**: 1-2 hours
- **Stage 4 (Future)**: Deferred to Phase 2

**Total**: ~1-2 days for incremental log collection

---

## Next Steps

1. **Immediate**: Spawn subagent to investigate S6 log handling
2. **After Research**: Finalize design decision (A, B, or C)
3. **Implementation**: Follow TDD cycle with tests first
4. **Review**: Validate against checklist
5. **Documentation**: Update TODO.md with implementation details