# Phase 4: Monitoring & Observability

**Timeline:** Week 11
**Effort:** ~75 lines of code
**Dependencies:** Phase 3 (needs integration)
**Blocks:** None

---

## Overview

Phase 4 adds comprehensive monitoring and observability to FSMv2. Prometheus metrics, structured logging, and runbooks provide visibility into system behavior and enable effective debugging.

**Key Metrics:**
- Infrastructure recovery events
- Action execution statistics
- Child count and reconciliation duration (NEW)
- Template rendering performance (NEW)

---

## Deliverables

### 1. Infrastructure Recovery Metrics
**From Original Plan**

```go
// Circuit breaker state
fsmv2_circuit_open{supervisor_id}

// Recovery events
fsmv2_infrastructure_recovery_total{supervisor_id}
fsmv2_infrastructure_recovery_duration_seconds{supervisor_id}

// Child health
fsmv2_child_health_check_total{supervisor_id, child_name, status}
```

**Usage:**
- Alert when circuit open for >5 minutes
- Track recovery time trends
- Identify unhealthy children

---

### 2. Action Execution Metrics
**From Original Plan**

```go
// Action queueing
fsmv2_action_queued_total{supervisor_id, action_type}
fsmv2_action_queue_size{supervisor_id}

// Action execution
fsmv2_action_execution_duration_seconds{supervisor_id, action_type, status}
fsmv2_action_timeout_total{supervisor_id, action_type}

// Worker pool
fsmv2_worker_pool_utilization{pool_name}
fsmv2_worker_pool_queue_size{pool_name}
```

**Usage:**
- Alert when actions timing out frequently
- Monitor worker pool saturation
- Identify slow action types

---

### 3. Hierarchical Composition Metrics
**NEW from Change Proposal**

```go
// Child count
fsmv2_children_count{supervisor_id}

// Reconciliation
fsmv2_reconciliation_total{supervisor_id, operation} // operation: add, update, remove
fsmv2_reconciliation_duration_seconds{supervisor_id}

// Tick propagation
fsmv2_tick_depth{supervisor_id} // Hierarchy depth
fsmv2_tick_propagation_duration_seconds{supervisor_id}
```

**Usage:**
- Monitor child count growth
- Track reconciliation performance
- Identify deep hierarchy bottlenecks

---

### 4. Template Rendering Metrics
**NEW from Change Proposal**

```go
// Template rendering
fsmv2_template_rendering_duration_seconds{supervisor_id, template_id}
fsmv2_template_rendering_errors_total{supervisor_id, template_id, error_type}

// Variable propagation
fsmv2_variable_propagation_depth{supervisor_id}
fsmv2_variable_merge_duration_seconds{supervisor_id}
```

**Usage:**
- Alert when template rendering >10ms
- Track variable propagation depth
- Identify template errors

---

### 5. Structured Logging
**Enhanced from Original Plan**

```go
// Infrastructure events
log.Info("circuit breaker opened",
    "supervisor_id", s.supervisorID,
    "failed_child", childName,
    "error", err)

// Child reconciliation
log.Info("child reconciliation completed",
    "supervisor_id", s.supervisorID,
    "added", addedCount,
    "updated", updatedCount,
    "removed", removedCount,
    "duration_ms", duration.Milliseconds())

// Template rendering
log.Debug("template rendered",
    "supervisor_id", s.supervisorID,
    "template_id", templateID,
    "duration_ms", duration.Milliseconds())

// Variable propagation
log.Debug("variables propagated",
    "supervisor_id", s.supervisorID,
    "depth", depth,
    "user_vars", len(userVars),
    "global_vars", len(globalVars))
```

**Log Levels:**
- **ERROR**: Circuit open, action timeout, template error
- **WARN**: High reconciliation duration, worker pool saturation
- **INFO**: Circuit close, child added/removed, action completed
- **DEBUG**: Tick start/end, template rendering, variable propagation

---

#### UX Enhancements for Circuit Breaker Logging

Phase 4 integrates UX improvements to reduce MTTR (Mean Time To Resolution) during infrastructure failures:

**1. Heartbeat Logs During Recovery**

Provides continuous feedback while circuit breaker is open (no silent failures):

```go
// UX Enhancement: Heartbeat logs every tick when circuit open
if s.circuitOpen {
    log.Warn().
        Int("retry_attempt", attempts).
        Int("max_attempts", 5).
        Dur("elapsed_downtime", s.backoff.GetTotalDowntime()).
        Dur("next_retry_in", s.backoff.NextDelay()).
        Str("recovery_status", s.getRecoveryStatus()).
        Msg("Circuit breaker open, retrying infrastructure checks")
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
```

**Recovery status transitions:**
- `attempting_recovery` (attempts 1-2): Initial retry phase
- `persistent_failure` (attempts 3-4): Issue not resolving automatically
- `escalation_imminent` (attempts 5+): Manual intervention needed soon

**2. Pre-Escalation Warnings**

Warns operators at retry attempt 4, providing opportunity to intervene before escalation:

```go
// UX Enhancement: Pre-escalation warning at attempt 4
if attempts == 4 {
    log.Warn().
        Str("child_name", childName).
        Int("attempts_remaining", 1).
        Dur("total_downtime", s.backoff.GetTotalDowntime()).
        Msg("WARNING: One retry attempt remaining before escalation")
}
```

**3. Runbook Embedding at Escalation**

Embeds runbook URLs and manual steps directly in error logs when escalation occurs:

```go
// UX Enhancement: Escalation runbook embedding at attempt 5+
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

// Helper: Runbook steps per child type
func (s *Supervisor) getEscalationSteps(childName string) string {
    steps := map[string]string{
        "dfc_read": "1) Check Redpanda logs 2) Verify network connectivity 3) Restart Redpanda manually",
        "benthos":  "1) Check Benthos logs 2) Verify OPC UA server reachable 3) Restart Benthos manually",
    }
    return steps[childName]
}
```

**4. Error Scope Distinction**

Distinguishes infrastructure errors (affects all workers) from worker-specific errors (limited blast radius):

```go
// Infrastructure error (affects all workers)
log.Warn().
    Err(err).
    Str("error_scope", "infrastructure").
    Str("impact", "all_workers").
    Str("child_name", childName).
    Int("retry_attempt", attempts).
    Msg("Infrastructure check failed, opening circuit breaker")

// Worker-specific error (limited blast radius)
log.Debug().
    Str("worker_id", workerID).
    Str("error_scope", "worker").
    Str("impact", "single_worker").
    Msg("Worker action in progress, skipping derivation")
```

**5. Recovery Feedback**

Shows progress when infrastructure recovers:

```go
// Infrastructure recovered
if s.circuitOpen {
    log.Info().
        Str("child_name", childName).
        Dur("total_downtime", s.backoff.GetTotalDowntime()).
        Msg("Infrastructure recovered, closing circuit breaker")
    metrics.CircuitBreakerRecoveries.Inc()
}
```

---

### 6. Runbook
**Enhanced from Original Plan**

**Common Scenarios:**

**Scenario 1: Circuit Stuck Open**
```
Symptom: fsmv2_circuit_open{supervisor_id="xyz"} = 1 for >5 min
Action:
1. Check logs for failed child: grep "circuit breaker opened" logs/
2. Verify child health: check child metrics
3. Manual recovery: restart failed child or parent supervisor
```

**UX Enhancements in This Scenario:**

When circuit breaker is stuck open, operators receive:

**Heartbeat Logs (every tick):**
```json
{
  "level": "warn",
  "msg": "Circuit breaker open, retrying infrastructure checks",
  "retry_attempt": 3,
  "max_attempts": 5,
  "elapsed_downtime": "45s",
  "next_retry_in": "8s",
  "recovery_status": "persistent_failure"
}
```

**Pre-Escalation Warning (at attempt 4):**
```json
{
  "level": "warn",
  "msg": "WARNING: One retry attempt remaining before escalation",
  "child_name": "benthos",
  "attempts_remaining": 1,
  "total_downtime": "53s"
}
```

**Escalation with Runbook (at attempt 5+):**
```json
{
  "level": "error",
  "msg": "ESCALATION REQUIRED: Infrastructure failure after max retry attempts. Manual intervention needed.",
  "child_name": "benthos",
  "max_attempts": 5,
  "total_downtime": "61s",
  "runbook_url": "https://docs.umh.app/runbooks/supervisor-escalation",
  "manual_steps": "1) Check Benthos logs 2) Verify OPC UA server reachable 3) Restart Benthos manually"
}
```

**Operators can:**
- Monitor recovery progress via `recovery_status` field transitions
- Intervene at attempt 4 before escalation occurs
- Follow embedded runbook steps directly from error logs
- Distinguish infrastructure errors (`error_scope: infrastructure`) from worker errors (`error_scope: worker`)

**Scenario 2: Actions Timing Out**
```
Symptom: fsmv2_action_timeout_total increasing
Action:
1. Check action type: which actions timing out?
2. Increase timeout: adjust ActionExecutor config
3. Check worker pool: verify pool not saturated
```

**Scenario 3: High Reconciliation Duration**
```
Symptom: fsmv2_reconciliation_duration_seconds > 100ms
Action:
1. Check child count: fsmv2_children_count
2. Check template complexity: fsmv2_template_rendering_duration_seconds
3. Optimize templates or reduce child count
```

**Scenario 4: Template Rendering Errors**
```
Symptom: fsmv2_template_rendering_errors_total increasing
Action:
1. Check logs for error type: "missing variable" or "syntax error"
2. Verify variable propagation: check parent variables
3. Fix template or add missing variables
```

**Scenario 5: Deep Hierarchy Performance**
```
Symptom: fsmv2_tick_propagation_duration_seconds > 50ms
Action:
1. Check hierarchy depth: fsmv2_tick_depth
2. Check child count at each level
3. Consider flattening hierarchy or optimizing tick loop
```

---

## UX Integration

Phase 4 monitoring follows UMH UX standards for operational visibility:

### Recovery Feedback (UX-001)

Provides continuous feedback during infrastructure recovery (no silent failures):

- **Heartbeat logs** every tick when circuit open
- **Retry attempt counter** visible in all recovery logs
- **Recovery status indicators**: `attempting_recovery` → `persistent_failure` → `escalation_imminent`
- **Downtime tracking**: `elapsed_downtime` and `next_retry_in` fields show timing

**Example:**
```json
{
  "level": "warn",
  "msg": "Circuit breaker open, retrying infrastructure checks",
  "retry_attempt": 3,
  "max_attempts": 5,
  "elapsed_downtime": "45s",
  "next_retry_in": "8s",
  "recovery_status": "persistent_failure"
}
```

### Error Distinction (UX-002)

Distinguishes infrastructure errors (recoverable) from application errors (requires code fix):

- **Infrastructure errors**: `error_scope: infrastructure`, `impact: all_workers`
  - Circuit breaker open (affects all workers)
  - Redpanda unavailable
  - Network connectivity issues

- **Application errors**: `error_scope: worker`, `impact: single_worker`
  - Action execution failures
  - Worker-specific timeouts
  - Invalid configurations

**Example:**
```json
// Infrastructure error
{
  "level": "warn",
  "error_scope": "infrastructure",
  "impact": "all_workers",
  "msg": "Infrastructure check failed, opening circuit breaker"
}

// Worker error
{
  "level": "debug",
  "error_scope": "worker",
  "impact": "single_worker",
  "msg": "Worker action in progress, skipping derivation"
}
```

### Pre-Escalation Warnings (UX-003)

Warns operators before escalation, providing opportunity to intervene:

- **Warning at attempt 4**: "One retry attempt remaining before escalation"
- **Shows remaining attempts**: Operators know how much time they have
- **Total downtime visible**: Helps assess severity

**Example:**
```json
{
  "level": "warn",
  "msg": "WARNING: One retry attempt remaining before escalation",
  "child_name": "benthos",
  "attempts_remaining": 1,
  "total_downtime": "53s"
}
```

### Runbook Embedding (UX-004)

Embeds runbooks directly in error logs (no searching for documentation):

- **Runbook URLs** in escalation logs
- **Manual escalation steps** in structured fields
- **Child-specific instructions** for common scenarios
- **Links to documentation** for detailed troubleshooting

**Example:**
```json
{
  "level": "error",
  "msg": "ESCALATION REQUIRED: Infrastructure failure after max retry attempts. Manual intervention needed.",
  "child_name": "benthos",
  "max_attempts": 5,
  "total_downtime": "61s",
  "runbook_url": "https://docs.umh.app/runbooks/supervisor-escalation",
  "manual_steps": "1) Check Benthos logs 2) Verify OPC UA server reachable 3) Restart Benthos manually"
}
```

### Design Rationale

These UX enhancements reduce MTTR (Mean Time To Resolution) by:

1. **Providing continuous feedback during recovery** - Operators see progress, not silent failures
2. **Warning operators before escalation** - Opportunity to intervene at attempt 4
3. **Embedding runbooks directly in logs** - No searching for documentation during incidents
4. **Distinguishing error types** - Infrastructure vs application errors have different remediation strategies

### UX Standards Compliance

Phase 4 monitoring implements the following UX standards:

- **UX-001**: Recovery feedback with heartbeat logs and status indicators
- **UX-002**: Error distinction with `error_scope` and `impact` fields
- **UX-003**: Pre-escalation warnings at retry attempt 4
- **UX-004**: Runbook embedding with URLs and manual steps

For complete UX standards documentation, see: **[../../UX_STANDARDS.md](../../UX_STANDARDS.md)** (if exists)

---

## Detailed Implementation

**For original monitoring tasks**, see the master plan:
**[../2025-11-02-fsmv2-supervision-and-async-actions.md](../2025-11-02-fsmv2-supervision-and-async-actions.md)** (lines 1652-1864)

**For hierarchical composition metrics**, see:
**[archive/fsmv2-change-proposal-section8-11.md](archive/fsmv2-change-proposal-section8-11.md)** (section on monitoring)

---

## Task Breakdown

### Task 4.1: Infrastructure Recovery Metrics (2 hours)
**Effort:** 1 hour implementation + 1 hour testing
**LOC:** ~20 lines (metrics + tests)

### Task 4.2: Action Execution Metrics (2 hours)
**Effort:** 1 hour implementation + 1 hour testing
**LOC:** ~20 lines (metrics + tests)

### Task 4.3: Hierarchical Composition Metrics (2 hours)
**Effort:** 1 hour implementation + 1 hour testing
**LOC:** ~15 lines (metrics + tests)

### Task 4.4: Template Rendering Metrics (2 hours)
**Effort:** 1 hour implementation + 1 hour testing
**LOC:** ~15 lines (metrics + tests)

### Task 4.5: Structured Logging (1 hour)
**Effort:** 1 hour implementation
**LOC:** ~5 lines (log statements)

### Task 4.6: Runbook (1 hour)
**Effort:** 1 hour documentation
**LOC:** N/A (documentation)

**Total:** 10 hours (1.5 days) → Buffer to 1 week

---

## Acceptance Criteria

### Functionality
- [ ] All metrics exposed via Prometheus endpoint
- [ ] Structured logging at appropriate levels
- [ ] Runbook covers common scenarios
- [ ] Metrics correlate with logs

### Quality
- [ ] Metric names follow Prometheus conventions
- [ ] Log messages structured and parsable
- [ ] Runbook tested with real scenarios

### Performance
- [ ] Metrics collection adds <1ms latency
- [ ] Logging non-blocking

### Documentation
- [ ] Metric descriptions documented
- [ ] Log format documented
- [ ] Runbook actionable

---

## Prometheus Query Examples

**Circuit breaker status:**
```promql
fsmv2_circuit_open{supervisor_id="protocol_converter_123"}
```

**Action execution rate:**
```promql
rate(fsmv2_action_execution_duration_seconds_count[5m])
```

**Child count over time:**
```promql
fsmv2_children_count{supervisor_id="protocol_converter_123"}
```

**Template rendering p99 latency:**
```promql
histogram_quantile(0.99, fsmv2_template_rendering_duration_seconds_bucket)
```

**Worker pool utilization:**
```promql
fsmv2_worker_pool_utilization{pool_name="global_action_pool"}
```

---

## Review Checkpoint

**After Phase 4 completion:**
- Verify all metrics exposed
- Confirm runbook scenarios work
- Check log structured format
- Validate metric cardinality (<1000 unique label combinations)

**Next Steps:**
- Migrate existing FSMs to FSMv2 (2-3 weeks)
- Deploy to production with gradual rollout
- Monitor metrics during rollout
- Update documentation based on production learnings
