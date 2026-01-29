# FSMv2 Dependencies Guide

This guide covers dependency injection patterns for FSMv2 workers.

## Dependency Inventory

| Dependency | Type | Purpose | Availability |
|------------|------|---------|--------------|
| `Logger` | `*zap.SugaredLogger` | Structured logging with worker context | All workers via `BaseDependencies` |
| `StateReader` | `deps.StateReader` | Read-only access to observed state from CSE | All workers via `BaseDependencies` |
| `MetricsRecorder` | `*deps.MetricsRecorder` | Buffer and record per-tick metrics | All workers via `BaseDependencies` |
| `FrameworkMetrics` | `*deps.FrameworkMetrics` | Supervisor-tracked metrics (read-only) | Injected via `deps.GetFrameworkState()` |
| `ChildrenView` | `config.ChildrenView` | Access child worker info (read-only) | Injected via `deps.GetChildrenView()` |
| `WorkerType` | `string` | Worker type identifier | All workers via `BaseDependencies` |
| `WorkerID` | `string` | Worker instance ID | All workers via `BaseDependencies` |

**ActionHistory** follows the same pattern as FrameworkMetrics: Supervisor records → `ActionHistorySetter` injects into deps → Worker reads via `deps.GetActionHistory()` → Worker assigns in CollectObservedState. The collector never modifies ObservedState.

Custom dependencies (e.g., `Transport`, `ConnectionPool`, channels) can be added by embedding `BaseDependencies` in your own struct. See `workers/communicator/dependencies.go` for an example.

## Global Variables Flow

```text
SetGlobalVariables(vars)
        │
        ▼
   Supervisor
        │
        ├── stores in s.globalVars
        │
        ▼
  DeriveDesiredState(spec)
        │
        ├── spec.Variables.Global = s.globalVars
        │
        ▼
     Worker
        │
        └── RenderConfigTemplate(config, variables)
                │
                └── {{ .global.cluster_id }} → "abc-123"
```

Global variables are fleet-wide settings injected by the supervisor before `DeriveDesiredState()` is called. Workers access them via template expansion in their config.

## StateReader Examples

### Read Own Previous State

**Note**: `GetStateReader()` returns nil in the following cases:
- During worker initialization (before supervisor sets up the state reader)
- In unit tests that don't configure a mock store
- When CSE storage is unavailable

Always nil-check before use:

```go
func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    deps := w.GetDependencies()
    stateReader := deps.GetStateReader()

    if stateReader != nil {
        var prev MyObservedState
        err := stateReader.LoadObservedTyped(ctx, deps.GetWorkerType(), deps.GetWorkerID(), &prev)
        if err == nil {
            // Use prev.SomeField for state change detection
        }
    }
    // ...
}
```

### Read Child Worker State

```go
func (w *ParentWorker) checkChildHealth(ctx context.Context, childID string) bool {
    deps := w.GetDependencies()
    stateReader := deps.GetStateReader()

    var childState ChildObservedState
    err := stateReader.LoadObservedTyped(ctx, "child", childID, &childState)
    if err != nil {
        return false
    }
    return childState.IsHealthy
}
```

## Metrics in Actions

### Recording Metrics

Actions use typed constants from `pkg/fsmv2/deps` for compile-time safety:

```go
func (a *SyncAction) Execute(ctx context.Context, depsAny any) error {
    deps := depsAny.(CommunicatorDependencies)

    // Increment counters (cumulative values)
    deps.MetricsRecorder().IncrementCounter(deps.CounterPullOps, 1)
    deps.MetricsRecorder().IncrementCounter(deps.CounterMessagesPulled, int64(len(messages)))

    // Set gauges (point-in-time values)
    deps.MetricsRecorder().SetGauge(deps.GaugeLastPullLatencyMs, float64(latency.Milliseconds()))
    deps.MetricsRecorder().SetGauge(deps.GaugeConsecutiveErrors, float64(errorCount))

    return nil
}
```

### Draining in CollectObservedState

```go
func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    deps := w.GetDependencies()

    // Get previous metrics from store
    var prevMetrics deps.Metrics
    if stateReader := deps.GetStateReader(); stateReader != nil {
        var prev MyObservedState
        if err := stateReader.LoadObservedTyped(ctx, deps.GetWorkerType(), deps.GetWorkerID(), &prev); err == nil {
            prevMetrics = prev.Metrics
        }
    }

    // Initialize from previous
    newMetrics := prevMetrics
    if newMetrics.Counters == nil {
        newMetrics.Counters = make(map[string]int64)
    }
    if newMetrics.Gauges == nil {
        newMetrics.Gauges = make(map[string]float64)
    }

    // Drain this tick's buffered metrics
    tickMetrics := deps.MetricsRecorder().Drain()

    // Accumulate counters (add deltas)
    for name, delta := range tickMetrics.Counters {
        newMetrics.Counters[name] += delta
    }

    // Set gauges (overwrite with latest)
    for name, value := range tickMetrics.Gauges {
        newMetrics.Gauges[name] = value
    }

    return MyObservedState{
        Metrics: newMetrics,
        // ...
    }, nil
}
```

### Defining Custom Metrics

Add typed constants in `pkg/fsmv2/deps/names.go`:

```go
// MyWorker counter names
const (
    CounterItemsProcessed CounterName = "items_processed"
    CounterItemsFailed    CounterName = "items_failed"
)

// MyWorker gauge names
const (
    GaugeQueueDepth    GaugeName = "queue_depth"
    GaugeProcessingMs  GaugeName = "processing_ms"
)
```

## Parent-Child Dependency Sharing

Use `MergeDependencies()` from `pkg/fsmv2/config/dependencies.go` to share resources:

```go
// Parent creates base dependencies
parentDeps := map[string]any{
    "channelProvider": provider,
    "timeout":         30 * time.Second,
}

// Child overrides specific values
childDeps := map[string]any{
    "timeout":    60 * time.Second,  // Override parent's timeout
    "customDep": customValue,        // Add child-specific dependency
}

// Merge: child overrides parent
merged := config.MergeDependencies(parentDeps, childDeps)
// Result: {"channelProvider": provider, "timeout": 60s, "customDep": customValue}
```

Key behaviors:
- Child values override parent values with the same key
- Setting a key to `nil` in child does NOT remove it (merged map has `key=nil`)
- Shallow merge: interface and channel values are shared (intentional for connection pools)

## Type Assertion Best Practices

### Safe Casting in Actions

```go
func (a *MyAction) Execute(ctx context.Context, depsAny any) error {
    deps, ok := depsAny.(*MyWorkerDependencies)
    if !ok {
        return fmt.Errorf("invalid dependencies type: expected *MyWorkerDependencies, got %T", depsAny)
    }

    // Use deps safely
    client := deps.GetClient()
    // ...
}
```

### Safe Casting for Optional Interfaces

```go
// Check if ObservedState has metrics
if holder, ok := observed.(deps.MetricsHolder); ok {
    metrics := holder.GetMetrics()
    // Export to Prometheus
}

// Check if DesiredState supports shutdown
if sr, ok := any(desired).(fsmv2.ShutdownRequestable); ok {
    sr.SetShutdownRequested(true)
}
```

### Common Patterns

```go
// State.Next() - dependencies come from supervisor injection
func (s *MyState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
    snap, err := helpers.ConvertSnapshot[MyObservedState, *MyDesiredState](snapAny)
    if err != nil {
        // Return error state
    }
    // Use snap.Observed, snap.Desired safely
}
```

## Dependency Injection Architecture

### The Action → ObservedState Bridge

FSMv2 dependencies serve as a bridge between action execution and observation collection:

```text
Action.Execute(ctx, deps)
       ↓
deps.Record*() / deps.Set*()     ← Actions write results
       ↓
CollectObservedState()
       ↓
deps.Get*() / deps.Drain()       ← Collector reads results
       ↓
ObservedState                     ← Results included in observation
```

### When to Use Dependencies vs Direct Service Calls

| Pattern | Use When | Examples |
|---------|----------|----------|
| **Dependency Injection** | Actions write data asynchronously | MetricsRecorder |
| **Direct Service Call** | Read-only I/O during collection | S6 status, HTTP health checks |
| **Supervisor Injection** | Framework provides data | FrameworkMetrics, ActionHistory, ChildrenView |

## Naming Conventions

### Metrics Types

The metrics system has three layers with distinct ownership:

| Type | Written By | Read By | Purpose |
|------|-----------|---------|---------|
| `FrameworkMetrics` | Supervisor | Workers | Supervisor-tracked metrics (state transitions, time in state) |
| `MetricsRecorder` | Actions | Collector | Buffer for action-written metrics (counters, gauges) |
| `Metrics` | N/A (data struct) | ObservedState | Accumulated metrics stored in observation |

**Why "FrameworkMetrics" not "SupervisorMetrics"?**
- "Framework" emphasizes these are supervisor-internal metrics exposed to workers
- Workers receive them read-only via `deps.GetFrameworkState()`
- The supervisor populates them automatically during state transitions

**Why "MetricsRecorder" not "ActionMetrics"?**
- Emphasizes the Record/Drain pattern (actions record, collector drains)
- Actions call `deps.MetricsRecorder().IncrementCounter()`
- Collector drains accumulated values into ObservedState

### StateReader vs ActionHistory

These serve fundamentally different purposes:

| Aspect | StateReader | ActionHistory |
|--------|-------------|---------------|
| Pattern | Query interface | Supervisor-managed buffer |
| Written By | N/A (read-only) | Supervisor (ActionExecutor) |
| Read By | Workers | Workers (via ObservedState) |
| Data | Point-in-time observed state | History of action executions |
| Lifetime | Persisted in CSE | Per-tick buffer, injected into ObservedState |

**Do NOT rename StateReader to "StateHistory"** - it reads current state, not accumulated history.

## Framework-Enforced Utilities (BaseDependencies)

All workers automatically receive these via `BaseDependencies`:

| Utility | Purpose | Access Method | Written By |
|---------|---------|---------------|------------|
| Logger | Structured logging | `deps.GetLogger()`, `deps.ActionLogger(name)` | N/A |
| StateReader | Read state from CSE | `deps.GetStateReader()` | N/A (query) |
| MetricsRecorder | Record custom metrics | `deps.MetricsRecorder().IncrementCounter()` | Actions |
| FrameworkMetrics | Supervisor metrics | `deps.GetFrameworkState()` | Supervisor |
| ChildrenView | Access child info | `deps.GetChildrenView()` | Supervisor |
| WorkerType/ID | Worker identity | `deps.GetWorkerType()`, `deps.GetWorkerID()` | N/A |

**ActionHistory** is accessed via `deps.GetActionHistory()` which returns `[]ActionResult` (read-only). The supervisor injects the data via `ActionHistorySetter` before `CollectObservedState` is called. Workers read and assign: `ObservedState.LastActionResults = deps.GetActionHistory()`. This matches the FrameworkMetrics pattern exactly.

## Optional Patterns for Worker-Specific Dependencies

Workers can add their own utilities by embedding `BaseDependencies`:

### Pattern: Error Tracking (from Communicator)

```go
type MyDependencies struct {
    *deps.BaseDependencies

    mu               sync.RWMutex
    consecutiveErrors int
}

func (d *MyDependencies) RecordError() {
    d.mu.Lock()
    d.consecutiveErrors++
    d.mu.Unlock()
}

func (d *MyDependencies) RecordSuccess() {
    d.mu.Lock()
    d.consecutiveErrors = 0
    d.mu.Unlock()
}

func (d *MyDependencies) GetConsecutiveErrors() int {
    d.mu.RLock()
    defer d.mu.RUnlock()
    return d.consecutiveErrors
}
```

### Pattern: Connection State Tracking (from ExampleChild)

```go
type MyDependencies struct {
    *deps.BaseDependencies

    mu          sync.RWMutex
    isConnected bool
}

func (d *MyDependencies) SetConnected(connected bool) {
    d.mu.Lock()
    d.isConnected = connected
    d.mu.Unlock()
}

func (d *MyDependencies) IsConnected() bool {
    d.mu.RLock()
    defer d.mu.RUnlock()
    return d.isConnected
}
```

### Pattern: External Service Access

```go
type MyDependencies struct {
    *deps.BaseDependencies
    httpClient HTTPClient  // Injected at construction, used directly in actions
}

func NewMyDependencies(base *deps.BaseDependencies, client HTTPClient) *MyDependencies {
    return &MyDependencies{
        BaseDependencies: base,
        httpClient:       client,
    }
}
```

## FSMv1 Migration Guidance

When migrating FSMv1 workers (S6, Benthos, Redpanda) to FSMv2:

### I/O in CollectObservedState (Direct Calls)

FSMv1 `UpdateObservedStateOfInstance()` becomes direct calls in `CollectObservedState()`:

```go
// FSMv1 pattern:
func (r *RedpandaInstance) UpdateObservedStateOfInstance() {
    info, _ := r.service.GetServiceStatus(ctx)
    r.PreviousObservedState.ServiceInfo = info
}

// FSMv2 pattern:
func (w *RedpandaWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    info, err := w.service.GetServiceStatus(ctx)  // Direct call, no DI needed
    return RedpandaObservedState{ServiceInfo: info}, err
}
```

### Action Results (Framework-Managed)

Action results are automatically recorded by the supervisor - workers cannot modify this, only read.

```go
// In action - NO manual recording needed:
func (a *StartServiceAction) Execute(ctx context.Context, depsAny any) error {
    deps := depsAny.(*RedpandaDependencies)
    return deps.service.Start(ctx)  // Just do the work, supervisor records the result
}

// In CollectObservedState - worker reads and assigns (like FrameworkMetrics):
func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    deps := w.GetDependencies()
    return MyObservedState{
        FrameworkMetrics:  deps.GetFrameworkState(),   // Supervisor-managed, read-only
        LastActionResults: deps.GetActionHistory(),    // Supervisor-managed, read-only
    }, nil
}

// Data flow (Provider/Setter pattern):
// 1. ActionExecutor records to supervisor's buffer after Execute()
// 2. Before CollectObservedState: ActionHistorySetter injects into deps
// 3. During CollectObservedState: Worker calls deps.GetActionHistory() and assigns
// 4. Collector saves what CollectObservedState returned (no modifications)
```

This mirrors `FrameworkMetrics` exactly - supervisor owns the data, injects into deps, worker reads and assigns in CollectObservedState.

### Key Differences from FSMv1

| Aspect | FSMv1 | FSMv2 |
|--------|-------|-------|
| State storage | In-memory struct | CSE TriangularStore |
| I/O timing | Any time in callbacks | CollectObservedState only |
| Action results | Stored in struct manually | Auto-recorded by supervisor |
| Parent access | Direct struct access | StateReader.LoadObservedTyped() |
| Health checks | String matching | Typed ObservedState fields |

## Related Documentation

- [doc.go](doc.go) - FSMv2 framework overview
- [api.go](api.go) - Core interfaces (`Worker`, `State`, `Action`)
- [config/dependencies.go](config/dependencies.go) - `MergeDependencies()` implementation
- [deps/metrics.go](deps/metrics.go) - Typed metric name constants
- [workers/example/](workers/example/) - Complete working examples
