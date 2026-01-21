# FSMv2 Metrics Guide

How metrics flow through FSMv2 and into the Management Console.

## Overview

FSMv2 provides a standardized metrics infrastructure for workers. Metrics flow from workers
through the CSE (Control Sync Engine) to the frontend for visualization.

```
Worker.CollectObservedState()
        ↓
ObservedState.Metrics (fsmv2.Metrics struct)
        ↓
CSE TriangularStore (persisted, delta-synced)
        ↓
Management Console Frontend
```

## Metrics Architecture

### MetricsRecorder (Collection)

Workers use `MetricsRecorder` to record metrics during action execution:

```go
// In action Execute():
deps.Metrics().IncrementCounter(metrics.CounterMessagesReceived, int64(count))
deps.Metrics().SetGauge(metrics.GaugeLastPullLatencyMs, float64(duration.Milliseconds()))
```

Note: Use typed `metrics.CounterName` and `metrics.GaugeName` constants for compile-time safety.

### MetricsHolder Interface (Export)

ObservedState types implement `MetricsHolder` to expose collected metrics:

```go
type MetricsHolder interface {
    GetMetrics() *fsmv2.Metrics
}
```

### Metrics Struct

The `fsmv2.Metrics` struct contains two metric categories:

```go
type Metrics struct {
    // Counters are cumulative values that only increase.
    // Examples: "pull_ops", "messages_pulled", "errors_total"
    Counters map[string]int64 `json:"counters,omitempty"`

    // Gauges are point-in-time values that can increase or decrease.
    // Examples: "consecutive_errors", "queue_depth", "last_pull_latency_ms"
    Gauges map[string]float64 `json:"gauges,omitempty"`
}

func NewMetrics() Metrics {
    return Metrics{
        Counters: make(map[string]int64),
        Gauges:   make(map[string]float64),
    }
}
```

## Adding Metrics to a Worker

### Step 1: Add Metrics Field to ObservedState

```go
type MyObservedState struct {
    // ... other fields ...

    // Metrics for observability
    Metrics fsmv2.Metrics `json:"metrics,omitempty"`
}
```

### Step 2: Implement MetricsHolder

```go
// GetMetrics implements fsmv2.MetricsHolder
// IMPORTANT: Use value receiver for compatibility with SetState() pattern
func (o MyObservedState) GetMetrics() *fsmv2.Metrics {
    return &o.Metrics
}
```

### Step 3: Merge Metrics in CollectObservedState

For workers with a MetricsRecorder, drain and merge into cumulative metrics:

```go
func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // Drain per-tick metrics from recorder (resets buffer)
    drained := w.deps.Metrics().Drain()

    // Merge into cumulative metrics
    // Note: In production, you'd maintain cumulative state and add drained deltas
    metrics := fsmv2.NewMetrics()
    for name, delta := range drained.Counters {
        metrics.Counters[name] += delta
    }
    for name, value := range drained.Gauges {
        metrics.Gauges[name] = value
    }

    return MyObservedState{
        // ... other fields ...
        Metrics: metrics,
    }, nil
}
```

For workers without a MetricsRecorder (like ApplicationWorker):

```go
func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    return MyObservedState{
        // ... other fields ...
        Metrics: fsmv2.NewMetrics(), // Empty metrics for interface consistency
    }, nil
}
```

### Step 4: Record Metrics in Actions

```go
func (a *ConnectAction) Execute(ctx context.Context, depsAny any) error {
    deps := depsAny.(MyDependencies)

    startTime := time.Now()
    err := performConnection(ctx)

    // Record metrics using typed metric names
    deps.Metrics().SetGauge(metrics.GaugeLastConnectLatencyMs, float64(time.Since(startTime).Milliseconds()))
    if err != nil {
        deps.Metrics().IncrementCounter(metrics.CounterConnectErrors, 1)
    } else {
        deps.Metrics().IncrementCounter(metrics.CounterConnectSuccess, 1)
    }

    return err
}
```

Note: Define your metric names in `pkg/fsmv2/metrics/names.go` as typed constants for compile-time safety.

## Worker-Specific Metrics

Each worker type should define meaningful metrics for its domain:

### CommunicatorWorker Metrics

| Metric | Type | Description |
|--------|------|-------------|
| messages_sent | Counter | Total messages pushed to backend |
| messages_received | Counter | Total messages pulled from backend |
| push_latency_ms | Histogram | Push operation latency |
| pull_latency_ms | Histogram | Pull operation latency |
| connection_errors | Counter | Network/auth failures |
| queue_depth | Gauge | Pending outbound messages |

### ApplicationWorker Metrics

The ApplicationWorker initializes empty metrics for interface consistency. It serves as
a supervisor/orchestrator and doesn't directly interact with external systems, so it has
no operational metrics to record.

### ContainerWorker Metrics (Future)

The ContainerWorker (when implemented) will be responsible for container-level resource metrics:

| Metric | Type | Description |
|--------|------|-------------|
| cpu_usage_percent | Gauge | Container CPU utilization |
| memory_usage_bytes | Gauge | Container memory consumption |
| memory_limit_bytes | Gauge | Container memory limit |
| goroutine_count | Gauge | Number of active goroutines |
| gc_pause_ns | Histogram | GC pause durations |

**Rationale**: CPU/memory metrics belong to the ContainerWorker (not other workers) because:
1. **Single source of truth**: One worker owns resource metrics, avoiding duplicates
2. **Process-level scope**: CPU/memory are process-wide, not per-FSM-worker
3. **Separation of concerns**: Business workers focus on domain metrics

## Metrics Flow to Frontend

### CSE Integration

When a worker's ObservedState is persisted to the TriangularStore, the Metrics field
is included in the JSON serialization. The CSE delta-sync mechanism propagates changes
to subscribers (including the frontend).

```
CollectObservedState() returns ObservedState with Metrics
    ↓
Supervisor calls store.SetObserved(ctx, workerType, workerID, observedState)
    ↓
TriangularStore persists observedState (including Metrics)
    ↓
Delta sync broadcasts change (sync_id increments)
    ↓
Frontend receives update via subscription
```

### Query Metrics

```go
// Backend can query current metrics
observed, _ := store.GetLatestObserved(ctx, workerType, workerID)
if holder, ok := observed.(fsmv2.MetricsHolder); ok {
    metrics := holder.GetMetrics()
    cpuUsage := metrics.Gauges["cpu_usage_percent"]
}
```

## Best Practices

### Metric Naming

Use snake_case with units where applicable:

```go
// Good
"connect_duration_ms"
"messages_sent"
"queue_depth"
"memory_usage_bytes"

// Avoid
"ConnectDuration"     // Not snake_case
"duration"            // No unit suffix
"num_msgs"            // Unclear abbreviation
```

### Counter vs Gauge

- **Counter**: Monotonically increasing (resets on restart)
  - `messages_sent`, `errors_total`, `retries`

- **Gauge**: Point-in-time value (can increase or decrease)
  - `queue_depth`, `cpu_percent`, `memory_bytes`

### Recording Latency

Use gauges for latency (since each measurement replaces the previous):

```go
start := time.Now()
// ... operation ...
deps.Metrics().SetGauge(metrics.GaugeOperationLatencyMs, float64(time.Since(start).Milliseconds()))
```

### Error Counting

Record both success and error counts for ratio calculation:

```go
if err != nil {
    deps.Metrics().IncrementCounter(metrics.CounterOperationErrors, 1)
    return err
}
deps.Metrics().IncrementCounter(metrics.CounterOperationSuccess, 1)
```

## Prometheus Export (Future)

The fsmv2.Metrics struct is designed to be exportable to Prometheus format.
A future enhancement will add automatic Prometheus exposition:

```go
// Planned API
prometheus.RegisterMetricsFrom(supervisor)

// Auto-generates:
// fsmv2_communicator_messages_sent_total{worker_id="comm-1"} 12345
// fsmv2_communicator_push_latency_ms{worker_id="comm-1",quantile="0.99"} 45
```

## Testing Metrics

### Verify Metrics Flow

```go
It("should expose metrics via MetricsHolder", func() {
    observed, err := worker.CollectObservedState(ctx)
    Expect(err).NotTo(HaveOccurred())

    holder, ok := observed.(fsmv2.MetricsHolder)
    Expect(ok).To(BeTrue(), "ObservedState should implement MetricsHolder")

    metrics := holder.GetMetrics()
    Expect(metrics).NotTo(BeNil())
    Expect(metrics.Counters).NotTo(BeNil())
})
```

### Verify Action Records Metrics

```go
It("should record latency metrics", func() {
    recorder := fsmv2.NewMetricsRecorder()
    deps := NewMyDependencies(logger, nil, identity, recorder)

    action := &ConnectAction{}
    err := action.Execute(ctx, deps)
    Expect(err).NotTo(HaveOccurred())

    metrics := recorder.Export()
    Expect(metrics.Counters["connect_success"]).To(Equal(int64(1)))
})
```
