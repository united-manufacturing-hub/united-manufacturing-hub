# FSMv2 Troubleshooting Guide

Operational guide for debugging FSMv2 issues in development and production.

## Quick Diagnostic Checklist

When something goes wrong, check in this order:

1. **Worker state**: `supervisor.GetCurrentStateName()` - Where is the FSM stuck?
2. **Log level**: Enable debug logging for detailed tick/action traces
3. **Observation freshness**: Is the collector running? Check timestamps.
4. **Action completion**: Is an action stuck? Check action logs.
5. **Shutdown flag**: Is `ShutdownRequested=true`? Worker might be shutting down.

## Common Issues

### Worker Stuck in TryingTo* State

**Symptom**: Worker stays in `TryingToStart`, `TryingToConnect`, etc. indefinitely.

**Causes**:
1. Action keeps failing (network issue, permission error)
2. Observation not reflecting action result
3. External dependency unavailable

**Debug steps**:
```bash
# Check action execution logs
grep "action" logs/umh-core/current | grep "workerID"

# Look for action errors
grep "ERROR.*action" logs/umh-core/current
```

**Resolution**:
- Check the action's error return - it should explain why it's failing
- Verify external dependencies are available (network, processes, files)
- Check if action is idempotent (repeated calls should succeed if already done)

### Worker Stuck in Running State But Not Working

**Symptom**: State shows "running" but actual system isn't functioning.

**Causes**:
1. Collector not refreshing observations
2. Stale observation accepted during shutdown
3. ObservedState doesn't reflect actual system state

**Debug steps**:
```bash
# Check observation timestamps
grep "observation_collected" logs/umh-core/current | tail -10

# Compare observation timestamp with current time
# Gap > 5 seconds indicates stale data
```

**Resolution**:
- Verify `CollectObservedState()` is actually querying system state
- Check collector timeout (default 500ms) - is external query too slow?
- Verify collector goroutine is running (check infrastructure health logs)

### Worker Won't Shut Down

**Symptom**: `ShutdownRequested=true` but worker never emits `SignalNeedsRemoval`.

**Causes**:
1. State.Next() not checking IsShutdownRequested()
2. Cleanup action failing repeatedly
3. Child workers blocking shutdown

**Debug steps**:
```bash
# Check if shutdown was requested
grep "ShutdownRequested=true" logs/umh-core/current

# Check state transitions
grep "transition.*workerID" logs/umh-core/current
```

**Resolution**:
- Ensure EVERY state's `Next()` checks `IsShutdownRequested()` FIRST
- Check cleanup actions are succeeding
- For parents: verify all children have stopped before emitting SignalNeedsRemoval

### Action Executed Multiple Times

**Symptom**: Same action runs repeatedly even though it should only run once.

**Causes**:
1. Action not idempotent (doesn't check if work already done)
2. Observation doesn't reflect action completion
3. Action-observation gating not working

**Debug steps**:
```bash
# Count action executions
grep "executing action.*ActionName" logs/umh-core/current | wc -l
```

**Resolution**:
- Make action check if work is already done before doing it
- Ensure `CollectObservedState()` captures the action's effect
- Verify observation timestamp advances after action completes

### Child Worker Not Created

**Symptom**: Parent specifies child in `ChildrenSpecs` but child never appears.

**Causes**:
1. Worker type not registered in factory
2. Invalid ChildSpec configuration
3. Parent state not in child's ChildStartStates

**Debug steps**:
```bash
# Check factory registration
grep "registering.*workerType" logs/umh-core/current

# Check child reconciliation
grep "reconcileChildren" logs/umh-core/current
```

**Resolution**:
- Verify child worker's init() calls `factory.RegisterWorkerType()`
- Check ChildSpec has valid Name, WorkerType
- If ChildStartStates is set, verify parent is in listed states

### Deadlock or Hang

**Symptom**: Supervisor stops responding, no tick logs.

**Causes**:
1. Lock ordering violation (child acquiring parent lock)
2. Blocking operation in tick path
3. Infinite loop in State.Next()

**Debug steps**:
```bash
# Enable lock order checking (development only)
export ENABLE_LOCK_ORDER_CHECKS=true

# Check for tick heartbeats (logged every 1000 ticks)
grep "tick heartbeat" logs/umh-core/current | tail -5

# Check for deadlock detection logs
grep "deadlock\|timeout\|stuck" logs/umh-core/current
```

**Resolution**:
- Review State.Next() for any blocking calls (I/O, locks, channels)
- Check for circular method calls between parent and child supervisors
- Ensure all database operations have timeouts

## Error Messages Reference

### "observation too stale, restarting collector"

**Meaning**: Observation timestamp is older than expected. Collector may have crashed.

**Context**: Infrastructure health monitor detected stale data.

**Action**: Usually self-healing. If persistent, check collector goroutine health.

### "circuit breaker open, skipping tick"

**Meaning**: Too many failures in infrastructure health checks. 5+ failures in 5 minutes.

**Context**: Supervisor entering degraded mode to prevent cascading failures.

**Action**: Check logs for root cause of infrastructure failures. May need manual intervention.

### "action timed out after 30s"

**Meaning**: Action.Execute() didn't complete within default timeout.

**Context**: Async action took too long (network issue, slow external system).

**Action**: Check external dependency availability. Consider increasing timeout if legitimately slow.

### "cannot switch state and emit action simultaneously"

**Meaning**: State.Next() returned both a new state AND an action. This violates FSMv2 rules.

**Context**: This is a programming error, supervisor panics intentionally.

**Action**: Fix State.Next() to return EITHER state change OR action, not both.

### "worker type not registered"

**Meaning**: Factory lookup failed for specified WorkerType.

**Context**: Child creation or supervisor instantiation.

**Action**: Ensure worker package is imported and init() calls RegisterWorkerType().

### "failed to derive desired state"

**Meaning**: DeriveDesiredState() returned an error (YAML parse, validation, etc.).

**Context**: User configuration is invalid.

**Action**: Check UserSpec.Config YAML syntax and content.

## Performance Troubleshooting

### High CPU Usage

**Check**:
1. Tick interval too aggressive (< 50ms)
2. CollectObservedState() doing expensive work
3. Too many workers (> 1000)

**Baseline**: ~4µs per worker per tick. 1000 workers = 4ms per tick cycle.

### Memory Growth

**Check**:
1. ObservedState accumulating data (should be constant size)
2. Action queue unbounded (should be 2x worker pool size)
3. Goroutine leak in collectors

### Tick Latency Spikes

**Check**:
1. Database queries in tick path
2. Lock contention on supervisor.mu
3. Child tick() blocking parent tick()

**Rule**: tick() should complete in < 10ms regardless of worker count.

## Debugging Commands

### Enable Trace Logging

```go
// Note: This is a partial example showing the trace logging option.
// Full Config requires WorkerType, Store, and other fields.
supervisor.Config{
    EnableTraceLogging: true,  // Verbose internal logs
    Logger:             logger.Named("supervisor"),
    // ... other required fields (WorkerType, Store)
}
```

### Check Worker State

```go
sup.GetCurrentStateName()      // Current FSM state
sup.GetWorkerState(workerID)   // Detailed state info
sup.ListWorkers()              // All workers
sup.GetChildren()              // Child supervisors
```

### Force State Inspection

```go
// Read directly from store (bypasses caching)
observed, _ := store.GetLatestObserved(ctx, workerType, workerID)
desired, _ := store.GetDesired(ctx, workerType, workerID)
```

### Run Benchmarks

```bash
# Tick loop performance
go test -bench=BenchmarkSupervisorTick -benchmem ./pkg/fsmv2/supervisor/...

# Action isolation (long-running actions shouldn't block)
go test -bench=BenchmarkTickLoopWithSlowWorker -benchmem ./pkg/fsmv2/supervisor/...

# Observation collection latency
go test -bench=BenchmarkObservationCollection -benchmem ./pkg/fsmv2/supervisor/...
```

## Getting Help

1. **Architecture tests**: `ginkgo run --focus="Architecture" -v ./pkg/fsmv2/` - Explains patterns
2. **Example workers**: `pkg/fsmv2/workers/example/` - Reference implementations
3. **doc.go files**: Detailed contracts for each package
4. **API_STABILITY.md**: Migration checklist and known limitations
