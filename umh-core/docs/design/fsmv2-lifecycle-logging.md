# FSMv2 Lifecycle Logging

**Status:** Phase 1 Complete (Commit: 4bd62c38e)
**Date:** 2025-11-13

## Overview

Phase 1 adds comprehensive debug-level logging for all FSM supervisor operations. This provides visibility into worker lifecycle, state transitions, mutex operations, and child management without any production overhead.

## Enabling Lifecycle Logging

Set the `LOG_LEVEL` environment variable to `debug`:

```bash
LOG_LEVEL=debug go run ./cmd/agent
```

All lifecycle events are emitted at debug level using structured logging (zap). When debug is disabled, there is **zero performance overhead** (zap short-circuits the debug calls).

## What Gets Logged

### Tick Operations (3 events)
- `tick_start` - Worker tick begins (includes `worker_id`, `state`)
- `tick_skip` - Tick skipped (includes `reason`)
- `tick_complete` - Tick finished (includes `final_state`, `duration`)

### State Transitions (1 event)
- `state_transition` - Worker changed state (includes `from_state`, `to_state`, `reason`, `worker_id`)

### Mutex Operations (3 events)
- `mutex_lock_acquire` - About to acquire lock (includes `mutex_name`, `lock_type`, `worker_id`)
- `mutex_lock_acquired` - Lock acquired successfully (includes `mutex_name`, `worker_id`)
- `mutex_unlock` - Lock released (includes `mutex_name`, `worker_id`)

### Signal Processing (1 event)
- `signal_processing` - Processing signal (includes `signal_type`, `worker_id`)

### Child Lifecycle (5 events)
- `child_update` - Child spec evaluated (includes `child_name`, `worker_type`, `action`)
- `child_add_start` - Starting child creation (includes `child_name`, `child_type`)
- `child_add_complete` - Child created (includes `child_name`, `child_type`, `duration`)
- `child_remove_start` - Starting child removal (includes `child_name`)
- `child_remove_complete` - Child removed (includes `child_name`, `duration`)

### Shutdown Sequence (6 events)
- `shutdown_start` - Supervisor shutdown initiated (includes `worker_type`)
- `shutdown_skip` - Shutdown skipped (includes `reason`)
- `shutdown_complete` - Supervisor stopped (includes `worker_type`, `duration`)
- `child_shutdown_start` - Starting child shutdown (includes `child_count`)
- `child_shutdown_complete` - All children stopped (includes `child_count`, `duration`)
- `child_shutdown_skip` - Child shutdown skipped (includes `reason`)

## Use Cases

### Debugging Deadlocks

**Problem:** Supervisor appears frozen, no state transitions happening.

**Investigation:**
```bash
# Find locks that were acquired but never released
grep 'mutex_lock_acquired' logs/*.log > acquired.txt
grep 'mutex_unlock' logs/*.log > released.txt
comm -23 <(sort acquired.txt) <(sort released.txt)
```

**Example output:**
```json
{"ts":"2025-11-13T20:10:05Z","level":"debug","msg":"lifecycle","lifecycle_event":"mutex_lock_acquired","mutex_name":"supervisor.mu","worker_id":"worker-1"}
```

This shows `worker-1` acquired `supervisor.mu` but never released it.

### Tracing State Transitions

**Problem:** Worker stuck in unexpected state, need to understand state flow.

**Investigation:**
```bash
# Show all state changes for specific worker
grep 'state_transition.*worker-123' logs/*.log | jq '{ts:.ts, from:.from_state, to:.to_state, reason:.reason}'
```

**Example output:**
```json
{"ts":"2025-11-13T20:10:05Z","from":"Starting","to":"Running","reason":"first_tick_success"}
{"ts":"2025-11-13T20:15:42Z","from":"Running","to":"Stopping","reason":"signal_received"}
```

### Monitoring Child Lifecycle

**Problem:** Children not being created or removed as expected.

**Investigation:**
```bash
# Track child supervisor creation/removal
grep -E 'child_(add|remove)' logs/*.log | jq '{ts:.ts, event:.lifecycle_event, child:.child_name}'
```

**Example output:**
```json
{"ts":"2025-11-13T20:10:06Z","event":"child_add_start","child":"modbus-child"}
{"ts":"2025-11-13T20:10:06Z","event":"child_add_complete","child":"modbus-child"}
{"ts":"2025-11-13T20:15:00Z","event":"child_remove_start","child":"modbus-child"}
{"ts":"2025-11-13T20:15:01Z","event":"child_remove_complete","child":"modbus-child"}
```

### Analyzing Performance

**Problem:** Ticks taking too long, need to identify bottleneck.

**Investigation:**
```bash
# Find longest tick durations
grep 'tick_complete' logs/*.log | jq 'select(.duration > 1000) | {worker:.worker_id, duration:.duration, state:.final_state}'
```

**Example output:**
```json
{"worker":"worker-5","duration":1523,"state":"Connected"}
{"worker":"worker-12","duration":2341,"state":"Running"}
```

## Log Format

All lifecycle logs are emitted as structured JSON with consistent fields:

```json
{
  "ts": "2025-11-13T20:10:05.123Z",
  "level": "debug",
  "msg": "lifecycle",
  "lifecycle_event": "tick_start",
  "worker_id": "worker-1",
  "state": "Running"
}
```

**Common fields:**
- `ts` - ISO 8601 timestamp
- `level` - Always "debug"
- `msg` - Always "lifecycle"
- `lifecycle_event` - Event type (see list above)
- `worker_id` - Worker identifier (when applicable)

**Event-specific fields:**
- State transitions: `from_state`, `to_state`, `reason`
- Mutex operations: `mutex_name`, `lock_type`
- Child operations: `child_name`, `child_type`
- Duration: `duration` (milliseconds)

## Performance Impact

When `LOG_LEVEL != debug`:
- **Zero overhead** - Zap short-circuits all debug calls
- No string formatting, no reflection, no I/O
- Verified by Phase 1 tests (lifecycle_logging_test.go:165-182)

When `LOG_LEVEL = debug`:
- Minimal overhead - Structured logging is fast
- No impact on critical path (locks, state transitions)
- Logs written to stdout by default (can redirect to file)

## Implementation Details

**File:** `pkg/fsmv2/supervisor/supervisor.go`

**Pattern:** All lifecycle logs use consistent structure:
```go
s.logger.Debugw("lifecycle",
    "lifecycle_event", "tick_start",
    "worker_id", workerID,
    "state", currentState.String())
```

**Key locations:**
- `tickWorker()` - Tick operations (lines 830-1072)
- State transition check - State changes (lines 1024-1030)
- `processSignal()` - Signal handling (line 1365)
- `reconcileChildren()` - Child management (lines 1587-1689)
- `Shutdown()` - Shutdown sequence (lines 1723-1830)

## Testing

**File:** `pkg/fsmv2/supervisor/lifecycle_logging_test.go`

**Coverage:** 4 lifecycle logging specs (all pass)
- Debug level shows lifecycle events
- Info level filters out lifecycle events
- Structured fields are verified
- JSON parsing works correctly

**Run tests:**
```bash
ginkgo -v --focus "Lifecycle Logging" pkg/fsmv2/supervisor/
```

## Future Enhancements (Phases 2-3)

**Phase 2:** Lock documentation
- Add comments documenting what each lock protects
- Document lock acquisition order
- Add runtime assertions for lock order

**Phase 3:** Clean boundaries
- Separate public API from internal implementation
- Extract lock management to dedicated package
- Improve testability with dependency injection

## References

- **Commit:** 4bd62c38e - "feat(fsmv2): Add comprehensive lifecycle debug logging"
- **Issue:** Context from previous deadlock fix (commit 33cfd7bb4) that took hours to debug
- **Design Doc:** `fsmv2-go-idiomatic-debugging.md` - Original debugging improvements plan
