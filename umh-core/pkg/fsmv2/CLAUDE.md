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
    snap := helpers.ConvertSnapshot[...](snapAny)  // Single type assertion

    // Shutdown check FIRST
    if snap.Desired.IsShutdownRequested() {
        return fsmv2.Transition(&StoppingState{}, fsmv2.SignalNone, nil, "Shutdown requested")
    }

    // Business logic — pass typed actions directly, the framework
    // auto-wraps them into Action[any] via reflection.
    if needsWork {
        return fsmv2.Transition(s, fsmv2.SignalNone, &MyAction{}, "Doing work")
    }

    // Catch-all return at end
    return fsmv2.Transition(s, fsmv2.SignalNone, nil, "Staying in running")
}
```

`fsmv2.Transition` is the canonical return shape for state files. It is a non-generic alias for `fsmv2.Result[any, any]` that also accepts typed `Action[TDeps]` values directly. State files use `fsmv2.Transition` + direct `&MyAction{}` construction — the framework auto-wraps typed actions via reflection.

## Reason Strings in State Transitions

The `Reason` parameter in `fsmv2.Result()` is visible in structured JSON logs (every state transition), supervisor heartbeats (every ~100 ticks), and parent supervisor's `ChildInfo.StateReason`. Write reasons that help operators troubleshoot without reading code.

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

## Factory Registration

Workers register in `init()` via `register.Worker[TConfig, TStatus, TDeps]`, which wires the factory, supervisor, and CSE type registry in one call. Use `register.NoDeps` for zero-dep workers:

```go
func init() {
    register.Worker[Config, Status, register.NoDeps](
        "myworker",
        NewMyWorker,
    )
}
```

The folder name must match the worker type string (e.g., `transport/` for `"transport"`).

## Worker API v2 (WorkerBase)

New workers should use `WorkerBase[TConfig, TStatus]` instead of the legacy 7-file pattern. Key types:

- **`WorkerBase[TConfig, TStatus]`** — embed in your worker struct; provides `InitBase`, `Config()`, `DeriveDesiredState`
- **`NewObservation[TStatus](status)`** — preferred constructor for `CollectObservedState` return values; the collector fills CollectedAt, framework metrics, action history, and accumulated worker metrics automatically after COS returns
- **`Observation[TStatus]`** — flat JSON serialization with framework fields (state, shutdown, children counts)
- **`WrappedDesiredState[TConfig]`** — promotes `BaseDesiredState` fields alongside TConfig
- **`WorkerSnapshot[TConfig, TStatus]`** — typed snapshot for state `Next()` methods
- **`ConvertWorkerSnapshot[TConfig, TStatus]`** — entry-point type assertion in states
- **`ExtractConfig[TConfig](desired)`** — typed config access in `CollectObservedState`
- **`register.Worker[TConfig, TStatus, TDeps]`** — one-line registration (factory + supervisor + CSE types). Use `register.NoDeps` for zero-dep workers.

Pass typed actions (`&MyAction{}`) directly to `fsmv2.Transition` — the framework auto-wraps them into `Action[any]` via reflection. No caller-visible adapter is needed.

**Architecture validators** accept both APIs: `ConvertWorkerSnapshot` and `ConvertSnapshot` are valid entry points; `snap.IsShutdownRequested` (field) and `snap.Desired.IsShutdownRequested()` (method) are valid shutdown checks.

**Capability interfaces** (optional, detected via type assertion on first instantiation):
- `ActionProvider` — `Actions() map[string]Action[any]`
- `ChildSpecProvider` — `ChildSpecs() []config.ChildSpec`
- `MetricsProvider` — `Metrics() []prometheus.Collector`
- `GracefulShutdowner` — `Shutdown(ctx context.Context) error`
- `ChildrenViewConsumer` — `SetChildrenView(view any)`

**L5 invariant**: `WorkerBase` must NEVER implement optional capability interfaces. Only the concrete worker struct should implement them.

## Metrics Patterns

Workers record metrics via `deps.MetricsRecorder()`. Return `fsmv2.NewObservation(status)` from `CollectObservedState` — the collector fills CollectedAt, framework metrics, action history, and accumulated worker metrics automatically after COS returns.

The collector loads the previous observed state from CSE, drains the recorder, and merges (counters additive, gauges replace). Workers must NOT call `MetricsRecorder().Drain()` in COS — the collector does it.

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
