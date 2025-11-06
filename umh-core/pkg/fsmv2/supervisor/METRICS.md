# Supervisor Metrics

## Overview

This document defines Prometheus metrics for FSMv2 Supervisor infrastructure supervision and action lifecycle tracking.

**Status:** Specification (Not Yet Implemented)

**Related:**
- `docs/design/fsmv2-infrastructure-supervision-patterns.md` - Circuit breaker design
- `docs/plans/fsmv2/phase-3-integration.md` - Task 3.2 acceptance criteria

---

## Action-Circuit Lifecycle Metrics

### `supervisor_actions_during_circuit_total`

**Type:** Counter

**Description:** Tracks actions that were in progress when the circuit breaker opened due to infrastructure inconsistency.

**Labels:**
- `supervisor_id` - Supervisor instance identifier
- `worker_type` - Worker type (e.g., "protocol_converter", "data_flow")
- `action_type` - Action type (e.g., "RestartRedpanda", "StartBenthos")

**Purpose:**
- Identify how often actions are interrupted by infrastructure issues
- Determine if action cancellation is needed in future phases
- Monitor infrastructure stability impact on operations

**Example PromQL:**
```promql
# Rate of actions interrupted by circuit breaker
rate(supervisor_actions_during_circuit_total[5m])

# Actions per supervisor
sum by (supervisor_id) (supervisor_actions_during_circuit_total)
```

---

### `supervisor_action_post_circuit_duration_seconds`

**Type:** Histogram

**Description:** Measures how long actions continue running after the circuit breaker opens.

**Labels:**
- `supervisor_id` - Supervisor instance identifier
- `worker_type` - Worker type
- `action_type` - Action type
- `result` - Action result ("success", "timeout", "failed")

**Buckets:** `[1, 5, 10, 20, 30, 60, 90, 120]` (seconds)

**Purpose:**
- Measure action continuation time during infrastructure recovery
- Identify actions that timeout due to circuit open
- Calculate P95/P99 latency impact of circuit breaker

**Example PromQL:**
```promql
# P95 action duration after circuit opens
histogram_quantile(0.95,
  rate(supervisor_action_post_circuit_duration_seconds_bucket[5m])
)

# Actions that timed out
sum by (action_type) (
  supervisor_action_post_circuit_duration_seconds_count{result="timeout"}
)
```

---

## Usage Patterns

### Identifying High-Impact Circuit Opens

```promql
# Actions interrupted in last hour
increase(supervisor_actions_during_circuit_total[1h]) > 5
```

**Alert:** If many actions interrupted, investigate infrastructure stability.

### Measuring Recovery Latency

```promql
# Average time actions run after circuit opens
rate(supervisor_action_post_circuit_duration_seconds_sum[5m])
  /
rate(supervisor_action_post_circuit_duration_seconds_count[5m])
```

**Threshold:** If average > 30s, actions frequently timeout during recovery.

### Determining Cancellation Need

```promql
# Percentage of actions that timeout during circuit open
(
  sum(rate(supervisor_action_post_circuit_duration_seconds_count{result="timeout"}[5m]))
    /
  sum(rate(supervisor_action_post_circuit_duration_seconds_count[5m]))
) * 100
```

**Decision Point:** If > 20% timeout, consider implementing action cancellation.

---

## Implementation Notes

**When to Increment `supervisor_actions_during_circuit_total`:**

```go
func (s *Supervisor) Tick(ctx context.Context) error {
    // Check infrastructure health
    if err := s.infraHealthChecker.CheckChildConsistency(); err != nil {
        s.circuitOpen = true

        // Record actions in-progress when circuit opened
        if s.actionExecutor.HasActionInProgress(s.supervisorID) {
            s.metrics.actionsCircuitCounter.Inc()
        }

        // ... restart child
    }
}
```

**When to Observe `supervisor_action_post_circuit_duration_seconds`:**

```go
func (ae *ActionExecutor) completeAction(actionID string, result string) {
    action := ae.getAction(actionID)

    // Check if circuit was open during action
    if action.CircuitOpenedAt != nil {
        duration := time.Since(*action.CircuitOpenedAt).Seconds()
        ae.metrics.actionPostCircuitDuration.
            WithLabelValues(action.SupervisorID, action.WorkerType, action.ActionType, result).
            Observe(duration)
    }
}
```

---

## Future Metrics (Phase 4)

When action cancellation is implemented:

### `supervisor_action_cancellations_total`

**Type:** Counter

**Labels:**
- `reason` - Cancellation reason ("circuit_open", "parent_stopped", "timeout")

**Purpose:** Track action cancellation effectiveness

### `supervisor_action_retry_attempts_total`

**Type:** Counter

**Labels:**
- `action_type` - Action type
- `attempt` - Retry attempt number (1-5)

**Purpose:** Identify actions that require many retries

---

## Testing

Test spec in `pkg/fsmv2/supervisor/supervisor_test.go` should verify:

1. **Circuit opens mid-action** - Action started at T+0s, circuit opens at T+10s
2. **Action completes** - Action runs to completion (not cancelled)
3. **Metrics recorded** - `supervisor_actions_during_circuit_total` incremented once
4. **Duration measured** - `supervisor_action_post_circuit_duration_seconds` observes ~20s (action completed 20s after circuit opened)

**Test Pattern:**

```go
It("records metrics when circuit opens during action execution", func() {
    // Setup: Start long-running action
    actionStarted := make(chan struct{})
    go func() {
        close(actionStarted)
        time.Sleep(30 * time.Second)
    }()
    <-actionStarted

    // Trigger: Open circuit at T+10s
    time.Sleep(10 * time.Millisecond)
    supervisor.circuitOpen = true

    // Wait for action to complete
    time.Sleep(20 * time.Millisecond)

    // Verify metrics
    counter := testutil.ToFloat64(supervisor.metrics.actionsCircuitCounter)
    Expect(counter).To(Equal(1.0))

    histogram := testutil.CollectAndCompare(supervisor.metrics.actionPostCircuitDuration)
    Expect(histogram).To(HaveObservation(BeNumerically("~", 20, 1)))
})
```

---

## References

- [Prometheus Metric Types](https://prometheus.io/docs/concepts/metric_types/)
- [Histogram Best Practices](https://prometheus.io/docs/practices/histograms/)
- FSMv2 Infrastructure Supervision: `docs/design/fsmv2-infrastructure-supervision-patterns.md`
