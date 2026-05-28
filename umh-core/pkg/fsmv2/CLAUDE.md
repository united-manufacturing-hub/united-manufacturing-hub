# FSMv2 Development Guide

This file captures patterns and insights for working with the FSMv2 framework.

## Architecture Test Compliance

The `architecture_test.go` validates ALL workers via file-system scanning (not runtime reflection). Compliance must be satisfied from day one - tests will fail if any invariant is violated.

**Key invariants to follow:**

| Rule | Description |
|------|-------------|
| Empty State Structs | States have no fields (except embedded base) |
| Shutdown Check First | Check `IsShutdownRequested()` as FIRST conditional in `Next()` |
| State XOR Action | Return state OR action, never both |
| Single Type Assertion | `Next()` has exactly one type assertion at entry |
| Pure DeriveDesiredState | No dependency access - only use `spec` parameter |
| Context Cancellation | Handle `ctx.Done()` at entry in `CollectObservedState` |
| Pointer Receivers | Use `*WorkerType` for all Worker methods |

Run architecture tests after every change:
```bash
go test ./pkg/fsmv2/... -run "Architecture" -v
```

## Parent-Child Worker Pattern

Parent workers orchestrate children via `DeriveDesiredState` - they return `ChildrenSpecs` and the supervisor handles child spawning automatically. Parents don't manually create child workers.

```go
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
    return &config.DesiredState{
        BaseDesiredState: config.BaseDesiredState{State: "running"},
        ChildrenSpecs: []config.ChildSpec{
            {Name: "child1", WorkerType: "childworker", ...},
        },
    }, nil
}
```

Children aggregation (health counts) is handled by the supervisor, not in `CollectObservedState`. The supervisor calls `SetChildrenCounts()` after collection.

## Channel Singleton Pattern

For workers that share channels (like TransportWorker with Push/Pull children), use a singleton `ChannelProvider`:

```go
// Set before creating workers
transport.SetChannelProvider(provider)

// Dependencies get channels from singleton
func NewDependencies(...) *Dependencies {
    provider := GetChannelProvider()
    inbound, outbound := provider.GetChannels(identity.ID)
    // ...
}
```

This enables parent-child channel sharing without tight coupling.

## State Machine States

Each state file follows this pattern:

```go
type RunningState struct {
    helpers.RunningHealthyBase  // Embed exactly one base type
}

func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
    snap := fsmv2.ConvertWorkerSnapshot[MyConfig, MyStatus](snapAny)

    // Shutdown check FIRST. Build the reason from snapshot fields so operators
    // can see why the worker stopped without reading code.
    if snap.ShouldStop() {
        reason := fmt.Sprintf("stop required: shutdown=%t, parentState=%q",
            snap.IsShutdownRequested(), snap.Observed.ParentMappedState)
        return fsmv2.Transition(&ShuttingDownState{}, fsmv2.SignalNone, nil, reason, nil)
    }

    // Business logic.
    if needsWork {
        reason := fmt.Sprintf("dispatching %s (queue=%d)", MyActionName, snap.Status.QueueDepth)
        return fsmv2.Transition(s, fsmv2.SignalNone, &MyAction{}, reason, nil)
    }

    // Catch-all return: include the conditions that kept the worker here so
    // operators can identify which precondition is still missing.
    reason := fmt.Sprintf("running: queue=%d, hasWork=%t", snap.Status.QueueDepth, needsWork)
    return fsmv2.Transition(s, fsmv2.SignalNone, nil, reason, nil)
}
```

`fsmv2.Transition()` is the canonical return shape for state files. Never use the generic
`fsmv2.Result[any, any](...)` form — the architecture gate rejects it.

## Reason Strings in State Transitions

The `Reason` parameter in `fsmv2.Transition()` is visible in structured JSON logs (every state transition), supervisor heartbeats (every ~100 ticks), and parent supervisor's `ChildInfo.StateReason`. Write reasons that help operators troubleshoot without reading code.

**Rules:**
- Include dynamic snapshot values via `fmt.Sprintf` — never hardcode values that exist in the snapshot
- For "stop required" transitions: `fmt.Sprintf("stop required: shutdown=%t, parentState=%s", ...)`
- For "waiting" catch-alls: include which preconditions are missing (`hasTransport=%t, hasValidToken=%t`)
- For degraded/error states: include the consecutive error count
- For child lifecycle: include the parent mapped state (`parentState=%q`)

**Gold standard** — communicator worker's `buildRecoveringReason()` in `state_recovering.go`:
```go
fmt.Sprintf("sync recovering: %d consecutive errors (%s), backoff %s",
    consecutiveErrors, mapErrorTypeToReason(errorType), backoffDelay.Round(time.Second))
```

**Bad**: `"Stop required"`, `"Waiting for transport or token"`, `"Degraded, still pushing"`
**Good**: `"stop required: shutdown=false, parentState=stopped"`, `"waiting: hasTransport=true, hasValidToken=false"`, `"degraded (5 consecutive errors), still pushing"`

## Children-in-Next Pattern

Parent state files can express child sets directly in `Next()` via the fifth argument of
`fsmv2.Transition()`. This is the path toward moving child lifecycle decisions out of
`DeriveDesiredState` and into the state machine where they are visible to operators.

**Nil = no opinion** (supervisor uses the `ChildrenSpecs` from `DeriveDesiredState`):

```go
return fsmv2.Transition(s, fsmv2.SignalNone, nil, "no change to children", nil)
```

**Empty slice = zero children** (supervisor tears down all children):

```go
return fsmv2.Transition(&TryingToStopState{}, fsmv2.SignalNone, nil, "stopping, no children needed",
    []config.ChildSpec{})
```

**Populated slice = exact desired child set** (supervisor reconciles to match):

```go
return fsmv2.Transition(s, fsmv2.SignalNone, nil, "running with children",
    []config.ChildSpec{
        {Name: "child1", WorkerType: "examplechild", UserSpec: spec1},
        {Name: "child2", WorkerType: "examplechild", UserSpec: spec2},
    })
```

Until a parent state returns a non-nil `Children` value, the supervisor continues using
the legacy `ChildrenSpecs` path from `DeriveDesiredState`. Both paths coexist safely
during migration.

## Declarative Children Pattern (RenderChildren + NewChildSpec[T])

Migrated parent workers (transport, communicator, exampleparent) declare children via
a package-level `RenderChildren` function in `children.go` instead of `DeriveDesiredState`.
States call it from their `Next()` method.

```go
// children.go — the single declaration site for this parent's child set.
func RenderChildren(cfg MyParentConfig, enabled bool) ([]config.ChildSpec, error) {
    spec, err := config.NewChildSpec("child-name", "childworker", MyChildConfig{
        Field: cfg.Field,
    }, enabled)
    if err != nil {
        return nil, err
    }
    return []config.ChildSpec{spec}, nil
}
```

**`NewChildSpec[T]`** marshals the typed config to `UserSpec.Config` and sets `Enabled`:
```go
spec, err := config.NewChildSpec[MyChildConfig]("name", "workertype", cfg, enabled)
```

**`enabled` controls per-tick run intent:**
- Alive-trajectory states pass `true` (children run).
- Stop states pass `false` (children stay resident via `IsDisabled=true`).
- Stateless/cheap children (e.g., exampleparent's examplechild) emit `[]config.ChildSpec{}`
  in stop states to despawn entirely.

**`IsDisabled` flow:** the CHANGE-19 reducer in `reconcileChildren` translates
`ChildSpec.Enabled` to the child's `IsDisabled` bit every tick. A child with
`IsDisabled=true` stays resident in `StoppedState` and does not resume until the
parent sets `Enabled=true` again. `IsShutdownRequested` (terminal removal) always
wins over `IsDisabled` in `StoppedState.Next`. A child whose spec has `Enabled=false`
from first creation is still spawned and driven to `StoppedState` — it is never skipped.

**Architecture validator:** any worker directory whose name contains `"parent"` must
declare a top-level `RenderChildren` function (in any file under the directory, including
`children.go`) or have child-building in `DeriveDesiredState`. This is enforced by
`MISSING_CHILDSPEC_VALIDATION` in `internal/validator/worker.go`.

## Worker Struct Pattern (WorkerBase embed)

Every worker embeds `fsmv2.WorkerBase[TConfig, TStatus, TDeps]`.
`TDeps` is the concrete dependencies pointer (e.g., `*MyDependencies`); use `struct{}`
for workers that have no custom dependencies.

```go
type MyWorker struct {
    fsmv2.WorkerBase[MyConfig, MyStatus, *MyDependencies]
    // Extra non-deps fields (e.g., a pre-acquired connection) go here.
}
```

**Constructor ritual** — call InitBase, then build deps from its returned `*BaseDependencies`, then BindDeps:

```go
func NewMyWorker(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, pool ConnectionPool) (*MyWorker, error) {
    w := &MyWorker{}
    bd := w.InitBase(id, logger, sr)
    d := NewMyDependencies(pool, bd)
    w.BindDeps(d)
    return w, nil
}
```

**Typed dependency accessor** — add one per concrete worker (WorkerBase only exposes `any`). Use comma-ok and a nil guard so the panic message survives any future TDeps change:

```go
func (w *MyWorker) GetDependencies() *MyDependencies {
    raw := w.GetDependenciesAny()

    d, ok := raw.(*MyDependencies)
    if !ok || d == nil {
        panic("MyWorker: GetDependencies called before BindDeps")
    }

    return d
}
```

**What WorkerBase provides without override:**
- `DeriveDesiredState`: parses `config.UserSpec`, duck-types `GetState() string` for state propagation
- `GetInitialState`: registry lookup via `fsmv2.LookupInitialState(workerType)` — register in `state/` init()
- `Identity()`, `Logger()`, `GetDependenciesAny()`
- ~~`Config()`, `ConfigReady()`~~ — deprecated, slated for deletion in L3. Read config via `fsmv2.ExtractConfig[T](desired)` in `CollectObservedState`.

**What to override:**

| Override | When |
|----------|------|
| `CollectObservedState` | Always — worker-specific snapshot |
| `DeriveDesiredState` | Custom children specs (application, communicator) or non-standard config parsing |
| `GetInitialState` | Worker does NOT register via fsmv2.RegisterInitialState (e.g., push, pull) |
| `GetDependenciesAny() any { return nil }` | No-deps worker. `WorkerBase[..., struct{}]` boxes `struct{}{}` into `any`, which is non-nil and silently skips framework metrics injection. The override returns a true nil to prevent that. |

## Worker Registration

Workers register in `init()` with one `register.Worker` line. The framework
wires the worker factory, supervisor factory, and CSE TypeRegistry from the
same call:

```go
func init() {
    register.Worker[MyConfig, MyStatus, *MyDependencies]("myworker",
        func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
            return NewMyWorker(id, logger, sr)
        })
}
```

`TConfig` and `TStatus` are the developer-defined config/status types from the
`WorkerBase[TConfig, TStatus, TDeps]` embed. `TDeps` is the worker's typed
dependency payload — use `register.NoDeps` for workers without per-instance
dependencies. The constructor returns `(fsmv2.Worker, error)`; a non-nil error
or nil worker at instantiation time panics with a contextualised message.

The folder name must match the worker type (e.g., `transport/` for type
`"transport"`).

### Parent-Child Typed Deps

Parents publish their dependencies via `register.SetDeps[T]`; children retrieve
them via `register.GetDeps[T]` inside a `SetDepsBuilder` closure. The closure
runs at child instantiation time (not at `init()` time), so the publisher
always wins the race when parent and child are wired in the same supervisor.

Transport / push canonical example:

```go
// transport/worker.go
func init() {
    register.Worker[snapshot.TransportDesiredState, snapshot.TransportStatus, *TransportDependencies]("transport",
        func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
            w, err := NewTransportWorker(id, logger, sr)
            if err != nil {
                return nil, err
            }
            register.SetDeps[*TransportDependencies]("transport", w.GetDependencies())
            return w, nil
        })
}

// transport/push/worker.go
func init() {
    register.Worker[snapshot.PushDesiredState, snapshot.PushStatus, *PushDependencies]("push",
        func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
            builder, ok := register.GetDepsBuilder("push")
            if !ok {
                return nil, errors.New("push deps builder missing")
            }
            pdeps := builder(id, logger, sr).(*PushDependencies)
            return NewPushWorker(id, logger, sr, pdeps)
        })

    register.SetDepsBuilder[*PushDependencies]("push",
        func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) *PushDependencies {
            parent := register.GetDeps[*transport_pkg.TransportDependencies]("transport")
            if parent == nil {
                return nil
            }
            d, err := NewPushDependencies(parent, deps.NewBaseDependencies(logger, sr, id))
            if err != nil {
                logger.SentryError(deps.FeatureForWorker("push"), id.HierarchyPath, err, "push_dependencies_creation_failed")
                return nil
            }
            return d
        })
}
```

`TDeps` is a phantom type parameter at registration — the compiler does not
verify it matches the `TDeps` in the worker's `WorkerBase[…]` embed. By
convention, the same `TDeps` appears in both places.

## FSMv2 Framework Types

Key types used when building workers:

- **`NewObservation[TStatus](status)`**: preferred constructor for `CollectObservedState` return values; the collector fills CollectedAt, framework metrics, action history, and accumulated worker metrics automatically after COS returns
- **`Observation[TStatus]`**: flat JSON serialization with framework fields (state, shutdown, children counts)
- **`WrappedDesiredState[TConfig]`**: promotes `BaseDesiredState` fields alongside TConfig
- **`WorkerSnapshot[TConfig, TStatus]`**: typed snapshot for state `Next()` methods
- **`ConvertWorkerSnapshot[TConfig, TStatus]`**: entry-point type assertion in states
- **`ExtractConfig[TConfig](desired)`**: typed config access in `CollectObservedState`
**Architecture validators** require `ConvertWorkerSnapshot` as the single entry-point in `Next()` methods; `snap.ShouldStop()` (WorkerSnapshot) is the canonical shutdown check.

**Capability interfaces** (optional, detected via type assertion on first instantiation):
- `ActionProvider`: `Actions() map[string]Action[any]`
- `MetricsProvider`: `Metrics() []prometheus.Collector`
- `GracefulShutdowner`: `Shutdown(ctx context.Context) error`
- `ChildrenViewConsumer`: `SetChildrenView(view config.ChildrenView) ObservedState`

**L5 invariant**: Optional capability interfaces must NEVER be implemented on embedded base types. Only the concrete worker struct should implement them.

## Metrics Patterns

Workers record metrics via `deps.MetricsRecorder()`. There are two observation return paths with different metric handling:

| Return Path | CollectedAt | Metrics Handling | When to Use |
|-------------|-------------|------------------|-------------|
| `fsmv2.NewObservation(status)` | Zero (collector sets it) | Collector loads previous from CSE, drains recorder, merges (counters additive, gauges replace) | New workers (preferred) |
| `w.WrapStatus(status)` | Set by caller | Worker drains recorder in COS, current-tick only | Legacy workers (deprecated) |
| `w.WrapStatusAccumulated(ctx, status)` | Set by caller | Worker loads CSE + drains + merges in COS | Legacy workers needing cross-tick accumulation (deprecated) |

**The zero-time gate**: The collector checks `observed.GetTimestamp().IsZero()`. If true (NewObservation), it runs post-COS wrapping. If false (WrapStatus/WrapStatusAccumulated), it skips wrapping since the worker already handled it. Both paths coexist safely during migration.

**Drain is destructive**: `MetricsRecorder().Drain()` empties the buffer. Only one path should drain per tick. NewObservation workers must not call Drain() in COS — the collector does it. WrapStatus workers drain in COS — the collector skips it.

## Graceful Shutdown Cascading

Each supervisor level has a `DefaultGracefulShutdownTimeout` of 5 seconds. For nested supervisors (parent-child workers), timeouts cascade:

| Nesting Level | Total Timeout |
|---------------|---------------|
| 1 (single worker) | 5s |
| 2 (parent + child) | 10s |
| 3 (grandparent + parent + child) | 15s |

**Test implications**: When testing shutdown scenarios with parent-child workers, allow sufficient time:

```go
// For parent with child supervisor (2 levels)
Eventually(result.Done, 15*time.Second).Should(BeClosed())
```

The shutdown flow:
1. Phase 1: Request children to stop (waits for graceful timeout)
2. Phase 2: Stop own workers (waits for graceful timeout)
3. Phase 3: Cancel context

## Testing Patterns

- Use Ginkgo/Gomega for tests
- Test files use `package foo_test` (external black-box testing)
- Mock dependencies with interfaces
- Test context cancellation explicitly
- Verify architecture compliance continuously

```bash
# Run all tests for a worker
go test ./pkg/fsmv2/workers/transport/... -v

# Check coverage
go test ./pkg/fsmv2/workers/transport/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

## Ginkgo Test Suite Organization

**One `RunSpecs()` per package** - multiple test files can have `var _ = Describe()` blocks, but only ONE file should call `RunSpecs()`:

```go
// examples_suite_test.go - THE ONLY file with RunSpecs
package examples_test

import (
    "testing"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestExamples(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Examples Suite")
}

// transport_scenario_test.go - NO RunSpecs, just Describe blocks
package examples_test

var _ = Describe("Transport Scenario", func() {
    // tests...
})
```

**Common mistake**: Having multiple `RunSpecs()` calls causes "Rerunning Suite" errors in CI.

## Destructive Channel Drain Safety

`<-chan` reads are destructive — once a message is read from a channel, it's gone. Pre-check all preconditions (token valid, transport exists) BEFORE draining. Use a `pendingMessages` buffer to store failed messages for retry on the next tick.

## Error Classification Pattern

9 ErrorTypes exist in `communicator/transport/http`. `ShouldStopRetrying` always returns false — classify locally as infrastructure (retry forever) vs non-infrastructure (drop). No retry cap: at 100ms tick rate, even `maxRetries=3` = 300ms, meaning a 1-second network outage drops messages.

## Parent-Level Transport Reset (resetGeneration Pattern)

`t.Reset()` belongs at the parent worker, not child actions. Parent `DegradedState` checks `backoff.ShouldResetTransport()` → dispatches `ResetTransportAction` → increments `resetGeneration`. Children compare generations via `CheckAndClearOnReset()` and clear their pending buffers. Always advance the retry counter after reset to break the modulo trigger in `ShouldResetTransport`.

## Per-Child Error Tracking in Parent-Child Workers

Each child (push/pull) has its own `RetryTracker` and `failurerate.Tracker` for independent health decisions:

- **Errors** flow to BOTH the child's tracker AND the parent's tracker (`RecordTypedError`, `RecordError` delegate up)
- **Successes** flow to the child's tracker ONLY (`RecordSuccess` does NOT propagate to parent; only auth success resets the parent tracker)
- **Reads** (`GetConsecutiveErrors`, `GetDegradedEnteredAt`, `GetLastErrorAt`) come from the child's own tracker
- Each child independently decides Running vs Degraded based on its own consecutive error count
- The parent tracker accumulates ALL child errors for `ShouldResetTransport` decisions

## Record* Methods: Only Call After Real Transport Operations

`RecordSuccess()` and `RecordTypedError()` both feed the `failurerate.Tracker` rolling window. They must ONLY be called after a real HTTP request completed (success or failure). Never call them when nothing happened (e.g., empty channel, backpressure skip, precondition failure).

**The rule**: if no HTTP round-trip occurred this tick, the action should return without calling any `Record*` method. "Nothing happened" is not a success and not a failure — it is the absence of an outcome.

This prevents failure rate dilution: if idle ticks feed phantom "successes" into the rolling window, a 100% broken transport can appear healthy because idle ticks vastly outnumber real operations in bursty workloads.

## State Transition Traps

### StoppingState must always progress

- MUST transition to StoppedState (or self-return with a cleanup action)
- Self-return with `nil` action is **forbidden** — creates a permanent deadlock (worker stuck in PhaseStopping)
- Self-return WITH an action (e.g., `&FlushAction{}`) is allowed (active cleanup)
- CI enforced: `ValidateStoppingStateNoCatchAllSelfReturn` in `internal/validator/state.go`
- See any `state_stopping.go` for the pattern

### Observed vs Desired ParentMappedState

`ParentMappedState` is only populated on the **observed** state (via `SetParentMappedState()`). The **desired** state copy is always empty. Use `snap.Observed.ParentMappedState` in reason strings, never `snap.Desired.ParentMappedState`.
