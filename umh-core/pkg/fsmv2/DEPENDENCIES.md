# FSMv2 Dependencies Guide

This guide covers dependency injection patterns for FSMv2 workers.

## Dependency Inventory

| Dependency | Type | Purpose | Availability |
|------------|------|---------|--------------|
| `Logger` | `*zap.SugaredLogger` | Structured logging with worker context | All workers via `BaseDependencies` |
| `StateReader` | `fsmv2.StateReader` | Read-only access to worker state store | All workers via `BaseDependencies` |
| `MetricsRecorder` | `*fsmv2.MetricsRecorder` | Buffer and record per-tick metrics | All workers via `BaseDependencies` |
| `WorkerType` | `string` | Worker type identifier | All workers via `BaseDependencies` |
| `WorkerID` | `string` | Worker instance ID | All workers via `BaseDependencies` |
| `Transport` | `transport.Transport` | HTTP transport for push/pull | Communicator only |
| `ConnectionPool` | `ConnectionPool` | Connection management interface | Example child (customizable) |
| `InboundChan` | `chan<- *UMHMessage` | Write channel for received messages | Communicator only |
| `OutboundChan` | `<-chan *UMHMessage` | Read channel for outgoing messages | Communicator only |

// TODO: logger, statereader and metrics recorder i get, maybe even workerypoe and id, but the rest is custom, so not need to document it? jsut that customs are possible?

## Creating Custom Dependencies

TODO: really necessary this tutorial?

### Step 1: Embed BaseDependencies

```go
type MyWorkerDependencies struct {
    *fsmv2.BaseDependencies           // Provides logger, stateReader, metrics
    customClient  CustomClientInterface
    mu            sync.RWMutex         // For thread-safe state
    isConnected   bool
}
```

### Step 2: Create Constructor

```go
func NewMyWorkerDependencies(
    client CustomClientInterface,
    logger *zap.SugaredLogger,
    stateReader fsmv2.StateReader,
    identity fsmv2.Identity,
) *MyWorkerDependencies {
    return &MyWorkerDependencies{
        BaseDependencies: fsmv2.NewBaseDependencies(logger, stateReader, identity),
        customClient:     client,
    }
}
```

### Step 3: Add Accessor Methods

```go
func (d *MyWorkerDependencies) GetClient() CustomClientInterface {
    return d.customClient
}

func (d *MyWorkerDependencies) SetConnected(connected bool) {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.isConnected = connected
}

func (d *MyWorkerDependencies) IsConnected() bool {
    d.mu.RLock()
    defer d.mu.RUnlock()
    return d.isConnected
}
```

### Step 4: Register with Factory

```go
func init() {
    factory.RegisterWorkerType[MyObservedState, *MyDesiredState](
        func(id fsmv2.Identity, logger *zap.SugaredLogger, stateReader fsmv2.StateReader, deps map[string]any) fsmv2.Worker {
            // Create dependencies with injected resources
            myDeps := NewMyWorkerDependencies(nil, logger, stateReader, id)
            return NewMyWorker(id, myDeps)
        },
        func(cfg interface{}) interface{} {
            return supervisor.NewSupervisor[MyObservedState, *MyDesiredState](cfg.(supervisor.Config))
        },
    )
}
```

## Global Variables Flow

```
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

TODO: needs tor be updarted based on the last implementation

Actions use typed constants from `pkg/fsmv2/metrics` for compile-time safety:

```go
func (a *SyncAction) Execute(ctx context.Context, depsAny any) error {
    deps := depsAny.(CommunicatorDependencies)

    // Increment counters (cumulative values)
    deps.Metrics().IncrementCounter(metrics.CounterPullOps, 1)
    deps.Metrics().IncrementCounter(metrics.CounterMessagesPulled, int64(len(messages)))

    // Set gauges (point-in-time values)
    deps.Metrics().SetGauge(metrics.GaugeLastPullLatencyMs, float64(latency.Milliseconds()))
    deps.Metrics().SetGauge(metrics.GaugeConsecutiveErrors, float64(errorCount))

    return nil
}
```

### Draining in CollectObservedState

// TODO: doesn't this happen automatically? or should this not work automatically and provided by the framework here fo fsmv2, e.g. supervisor?

```go
func (w *MyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    deps := w.GetDependencies()

    // Get previous metrics from store
    var prevMetrics fsmv2.Metrics
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
    tickMetrics := deps.Metrics().Drain()

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

Add typed constants in `pkg/fsmv2/metrics/names.go`:

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
if holder, ok := observed.(fsmv2.MetricsHolder); ok {
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

## Related Documentation

- [doc.go](doc.go) - FSMv2 framework overview
- [api.go](api.go) - Core interfaces (`Worker`, `State`, `Action`)
- [config/dependencies.go](config/dependencies.go) - `MergeDependencies()` implementation
- [metrics/names.go](metrics/names.go) - Typed metric name constants
- [workers/example/](workers/example/) - Complete working examples
