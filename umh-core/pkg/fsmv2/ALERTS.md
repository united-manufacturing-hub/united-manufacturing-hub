# FSMv2 Alerting Guide

This document provides guidance on setting up alerts for FSMv2 metrics in Prometheus/Alertmanager.

## Critical Alerts (P0)

### Circuit Breaker Open

```yaml
- alert: FSMv2CircuitBreakerOpen
  expr: umh_fsmv2_circuit_open == 1
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "FSMv2 circuit breaker is open"
    description: "Circuit breaker for {{ $labels.worker_type }} has been open for over 1 minute. Infrastructure dependencies may be unavailable."
    runbook_url: "https://docs.umh.app/runbooks/fsmv2-circuit-breaker"
```

### Action Execution Timeouts

```yaml
- alert: FSMv2ActionTimeoutsHigh
  expr: rate(umh_fsmv2_action_timeout_total[5m]) > 0.1
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "High rate of FSMv2 action timeouts"
    description: "Worker {{ $labels.worker_type }} action {{ $labels.action_type }} timing out at {{ printf \"%.2f\" $value }} per second."
```

### Dropped Messages

```yaml
- alert: FSMv2MessagesDropped
  expr: rate(umh_fsmv2_adapter_dropped_messages_total[5m]) > 0
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "FSMv2 adapter dropping messages"
    description: "Messages being dropped in {{ $labels.direction }} direction. Check channel capacity and processing speed."
```

## Warning Alerts (P1)

### Action Queue Backlog

```yaml
- alert: FSMv2ActionQueueBacklog
  expr: umh_fsmv2_action_queue_size > 10
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "FSMv2 action queue building up"
    description: "Action queue for {{ $labels.worker_type }} has {{ $value }} pending actions. May indicate slow action execution."
```

### Worker Pool Saturation

```yaml
- alert: FSMv2WorkerPoolSaturated
  expr: umh_fsmv2_worker_pool_utilization > 0.9
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "FSMv2 worker pool near capacity"
    description: "Worker pool {{ $labels.pool_name }} at {{ printf \"%.0f\" (mul $value 100) }}% utilization."
```

### Reconciliation Errors

```yaml
- alert: FSMv2ReconciliationErrors
  expr: rate(umh_fsmv2_reconciliation_total{result="error"}[5m]) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "FSMv2 reconciliation errors"
    description: "Worker {{ $labels.worker_type }} experiencing reconciliation errors at {{ printf \"%.2f\" $value }} per second."
```

### Slow Reconciliation

```yaml
- alert: FSMv2SlowReconciliation
  expr: histogram_quantile(0.95, rate(umh_fsmv2_reconciliation_duration_seconds_bucket[5m])) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "FSMv2 reconciliation taking too long"
    description: "P95 reconciliation time for {{ $labels.worker_type }} is {{ printf \"%.3f\" $value }}s. Target is <100ms."
```

## Informational Alerts

### Infrastructure Recovery

```yaml
- alert: FSMv2InfrastructureRecovery
  expr: increase(umh_fsmv2_infrastructure_recovery_total[5m]) > 0
  labels:
    severity: info
  annotations:
    summary: "FSMv2 infrastructure recovery occurred"
    description: "Worker {{ $labels.worker_type }} recovered from infrastructure failure."
```

### Template Rendering Errors

```yaml
- alert: FSMv2TemplateRenderingErrors
  expr: rate(umh_fsmv2_template_rendering_errors_total[5m]) > 0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "FSMv2 template rendering errors"
    description: "Template rendering errors for {{ $labels.worker_type }}: {{ $labels.error_type }}."
```

## Communicator-Specific Alerts

### Authentication Failures

```yaml
- alert: CommunicatorAuthFailures
  expr: rate(umh_fsmv2_worker_auth_failures_total[5m]) > 0.1
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "Communicator authentication failing"
    description: "Communicator experiencing auth failures. Check credentials and backend availability."
```

### Backend Rate Limiting

```yaml
- alert: CommunicatorRateLimited
  expr: rate(umh_fsmv2_worker_backend_rate_limit_errors_total[5m]) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Communicator being rate limited"
    description: "Backend is rate limiting the communicator. Consider reducing request frequency."
```

### Network Errors

```yaml
- alert: CommunicatorNetworkErrors
  expr: rate(umh_fsmv2_worker_network_errors_total[5m]) > 0.5
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Communicator network errors"
    description: "High rate of network errors. Check connectivity to management console backend."
```

## Recording Rules

Add these recording rules to pre-compute expensive queries:

```yaml
groups:
  - name: fsmv2_recording_rules
    rules:
      - record: fsmv2:reconciliation_error_rate:5m
        expr: rate(umh_fsmv2_reconciliation_total{result="error"}[5m])

      - record: fsmv2:action_timeout_rate:5m
        expr: rate(umh_fsmv2_action_timeout_total[5m])

      - record: fsmv2:reconciliation_p95_seconds:5m
        expr: histogram_quantile(0.95, rate(umh_fsmv2_reconciliation_duration_seconds_bucket[5m]))
```

## Cardinality Considerations

The `state_duration_seconds` metric uses `worker_id` labels which can lead to high cardinality with many workers. Monitor:

```yaml
- alert: FSMv2HighCardinality
  expr: count(umh_fsmv2_state_duration_seconds) > 1000
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "High cardinality in FSMv2 metrics"
    description: "{{ $value }} time series for state_duration_seconds. Consider aggregation."
```

## Dashboard Recommendations

Key panels for an FSMv2 dashboard:

1. **Circuit Breaker Status** - Gauge showing open/closed state
2. **Reconciliation Rate** - Graph of reconciliations per second by result
3. **Reconciliation Latency** - P50/P95/P99 heatmap
4. **Action Queue Depth** - Time series of pending actions
5. **Worker Pool Utilization** - Percentage utilization over time
6. **State Distribution** - Table of workers by current state
7. **Error Rate** - Combined view of all error counters

## Runbook References

For each alert, create runbooks covering:

1. **What it means** - Business impact explanation
2. **Investigation steps** - Commands and queries to run
3. **Common causes** - Known failure modes
4. **Resolution steps** - How to fix common issues
5. **Escalation path** - Who to contact if unresolved
