# Agent FSM v2 - TODO

## Current Status: Phase 1 Partially Complete (2025-01-20)

### ✅ Completed (Stages 1-4)

1. **Planning** - IMPLEMENTATION_PLAN.md created with detailed approach
2. **S6 GetLogsSince() Implementation**
   - Method implemented in `pkg/service/s6/s6.go:1144-1245`
   - 20 comprehensive tests in `pkg/service/s6/s6_incremental_test.go`
   - All tests passing (130/130)
3. **FSM v2 Worker Updates**
   - Worker tracks `lastLogTimestamp` in `pkg/fsmv2/agent/worker.go`
   - Mock services updated to support incremental testing
   - 8 new worker tests validate incremental behavior
4. **Code Review**
   - Implementation reviewed and approved
   - Performance optimization identified (not critical for v1)

### ⏳ Remaining Phase 1 Work

#### Stage 5: Integration Tests
- [ ] Write end-to-end integration tests with real FSM v2 supervisor
- [ ] Test incremental collection with actual S6 logs (not mocks)
- [ ] Verify timestamp persistence across worker restarts
- [ ] Test behavior with log rotation during collection
- [ ] Validate memory usage remains bounded over time

#### Stage 6: Final Code Review & Cleanup
- [ ] Review entire implementation against original requirements
- [ ] Remove or update outdated TODO comments
- [ ] Ensure all edge cases are covered
- [ ] Verify no regression in FSM v1 behavior
- [ ] Update documentation

---

## 📋 Phase 2: CSE Integration (Future)

### Overview
Currently, logs are stored inline in `ServiceInfo`. Phase 2 will implement the CSE storage pattern where logs are stored separately in SQLite with 1:many partial loading.

### Implementation Tasks

#### 1. Update Agent Monitor Service Interface
```go
// Current (Phase 1 - prepared but not integrated)
type Service interface {
    Status(ctx context.Context, snapshot fsm.SystemSnapshot) (*ServiceInfo, error)
    // TODO: Add method that accepts 'since' parameter
    // StatusIncremental(ctx context.Context, snapshot fsm.SystemSnapshot, since time.Time) (*ServiceInfo, error)
}
```

- [ ] Add `StatusIncremental()` method to interface
- [ ] Update implementation to call `s6.GetLogsSince()` instead of `s6.GetLogs()`
- [ ] Maintain backward compatibility for FSM v1 using original `Status()`

#### 2. Integrate GetLogsSince() in Agent Monitor
```go
// In pkg/service/agent_monitor/agent.go:232
func (c *AgentMonitorService) getAgentLogs(ctx context.Context, since *time.Time) ([]s6.LogEntry, error) {
    if since != nil {
        // Phase 2: Use incremental collection
        return c.s6Service.GetLogsSince(ctx, servicePath, c.fs, *since)
    }
    // Phase 1/FSM v1: Get all logs (current behavior)
    return c.s6Service.GetLogs(ctx, servicePath, c.fs)
}
```

- [ ] Modify `getAgentLogs()` to accept optional timestamp
- [ ] Call appropriate S6 method based on timestamp presence
- [ ] Update worker to pass `lastLogTimestamp` to service

#### 3. Implement CSE Storage Pattern

##### Schema
```sql
-- Track last collected position per worker
CREATE TABLE agent_monitor_log_positions (
    worker_id TEXT PRIMARY KEY,
    last_timestamp INTEGER NOT NULL,  -- Unix nano timestamp
    last_offset INTEGER,               -- File offset for optimization
    last_file TEXT                     -- Track which file we last read
);

-- Store logs separately (not inline in ServiceInfo)
CREATE TABLE agent_monitor_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT NOT NULL,
    service_name TEXT NOT NULL,
    timestamp INTEGER NOT NULL,       -- Unix nano timestamp
    level TEXT,                       -- INFO, WARN, ERROR
    message TEXT NOT NULL,
    collected_at INTEGER NOT NULL,    -- When we collected this log
    INDEX idx_worker_timestamp (worker_id, timestamp),
    INDEX idx_service_timestamp (service_name, timestamp)
);
```

- [ ] Create CSE storage tables
- [ ] Implement persistence layer for log positions
- [ ] Implement log appender that writes directly to SQLite
- [ ] Add methods to query logs by time range

##### Supervisor Integration
- [ ] Update supervisor to persist log position after each collection
- [ ] Implement recovery of last position on restart
- [ ] Add log retention policy (e.g., keep last 7 days)
- [ ] Implement log query API for retrieving historical logs

#### 4. Update Snapshot Structure
```go
// In pkg/fsmv2/agent/snapshot.go
type AgentMonitorObservedState struct {
    ServiceInfo ServiceInfo `json:"service_info"`
    CollectedAt time.Time   `json:"collected_at"`
    // Phase 2: Remove logs from ServiceInfo, add reference
    // LogsStoredInCSE bool   `json:"logs_stored_in_cse"`
    // LatestLogTime time.Time `json:"latest_log_time"`
}
```

- [ ] Remove logs from inline storage in ServiceInfo
- [ ] Add metadata about log storage location
- [ ] Update serialization to handle new structure

---

## 🚀 Performance Optimizations (Future)

### Identified in Code Review

#### 1. Skip Old Rotated Files
**Issue**: Currently reads ALL rotated files even if they're older than `since` timestamp

**Solution**:
```go
// Check rotated file timestamp before reading
rotatedFileTime := extractTimestampFromFilename(rotatedFile) // from @TIMESTAMP.s
if rotatedFileTime.Before(since) {
    continue // Skip entire file
}
```

- [ ] Implement timestamp extraction from rotated filenames
- [ ] Add early skip logic for old files
- [ ] Add metrics to track skipped vs read files

#### 2. Streaming Log Parser
**Issue**: Loads entire log files into memory before filtering

**Solution**:
- [ ] Implement streaming parser that filters while reading
- [ ] Use buffered reader with line-by-line processing
- [ ] Stop reading once we've collected enough recent logs

#### 3. Add Performance Metrics
- [ ] Track bytes read vs bytes returned ratio
- [ ] Monitor time spent in GetLogsSince()
- [ ] Add alerts for excessive log reading
- [ ] Dashboard for log collection efficiency

---

## 📊 Testing Gaps

### Unit Tests Needed
- [ ] Test worker behavior when S6 service returns error
- [ ] Test timestamp persistence across supervisor restarts
- [ ] Test concurrent log collection (race conditions)
- [ ] Test with actual TAI64N formatted timestamps (not simplified format)

### Integration Tests Needed
- [ ] Full FSM v2 supervisor with agent worker
- [ ] Real S6 service with actual log files
- [ ] Log rotation during collection
- [ ] Performance benchmarks with large log files
- [ ] Memory profiling over extended runtime

### End-to-End Tests Needed
- [ ] Deploy test instance with FSM v2 agent
- [ ] Monitor memory usage over 24-48 hours
- [ ] Verify no log loss during rotations
- [ ] Test recovery after agent crash/restart

---

## 🔄 Migration Path

### From Phase 1 to Phase 2
1. Deploy Phase 1 (current implementation) - memory bounded in worker
2. Add CSE tables via migration
3. Update agent monitor service with incremental method
4. Update worker to use new method
5. Run both inline and CSE storage briefly (rollback safety)
6. Remove inline storage once CSE proven stable

### Rollback Plan
- Phase 1 can be rolled back to original behavior (GetLogs)
- Phase 2 can fall back to Phase 1 (inline storage)
- FSM v1 unaffected throughout (uses original GetLogs)

---

## 📝 Documentation Needed

- [ ] Update agent FSM v2 architecture docs with log handling
- [ ] Document CSE storage pattern for logs
- [ ] Add operational guide for log retention/cleanup
- [ ] Create troubleshooting guide for log collection issues
- [ ] Update API docs with new incremental methods

---

## 🎯 Success Metrics

### Phase 1 (Current)
- ✅ Worker memory usage bounded (no accumulation)
- ✅ Backward compatible with FSM v1
- ✅ All tests passing

### Phase 2 (Future)
- [ ] Logs persisted to CSE, not in memory
- [ ] <100ms collection time for typical workload
- [ ] No log loss during rotations
- [ ] Successful recovery after restarts
- [ ] Memory usage constant over weeks of runtime

---

## 📅 Timeline

- **Phase 1**: ✅ Core implementation complete (2025-01-20)
- **Phase 1 Completion**: ~2-4 hours (Stages 5-6)
- **Phase 2**: ~2-3 days (CSE integration, testing, migration)
- **Optimizations**: ~1 day (as needed based on metrics)

---

## 🔗 Related Files

- Implementation Plan: `IMPLEMENTATION_PLAN.md`
- S6 Service: `pkg/service/s6/s6.go:1144-1245`
- S6 Tests: `pkg/service/s6/s6_incremental_test.go`
- Agent Worker: `pkg/fsmv2/agent/worker.go`
- Worker Tests: `pkg/fsmv2/agent/worker_test.go`
- Agent Monitor: `pkg/service/agent_monitor/agent.go`

---

## 👥 Contact

For questions about this implementation:
- Review IMPLEMENTATION_PLAN.md for design decisions
- Check git history for context on changes
- FSM v2 architecture questions: See pkg/fsmv2/README.md

Last Updated: 2025-01-20 by Claude (Stage 4 complete)