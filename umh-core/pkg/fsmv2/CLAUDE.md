# FSMv2 Development Guide

This file captures patterns and insights for working with the FSMv2 framework.

## Architecture Test Compliance

The `architecture_test.go` validates a subset of worker invariants via file-system scanning (not runtime reflection). The CI-enforced rules will fail builds; the conventions are reviewer-enforced — code authors and reviewers should hold the line.

**Key invariants to follow:**

| Rule | Description | Enforcement |
|------|-------------|-------------|
| Empty State Structs | States have no fields (except embedded base) | Convention |
| Stop Check First | Check `IsBeingRemoved` (or `ShouldStop`) as FIRST conditional in `Next()` | Convention |
| State XOR Action | Return state OR action, never both | CI (`ValidateStateXORAction`) |
| No Nil State Returns | `Transition`'s state argument is non-nil | CI (`ValidateNoNilStateReturns`) |
| Signal/State Mutual Exclusion | A `Next()` does not return both `SignalNeedsRemoval` and a non-self state | CI (`ValidateSignalStateMutualExclusion`) |
| StoppingState Progress | StoppingState catch-all must not self-return without an action | CI (`ValidateStoppingStateNoCatchAllSelfReturn`) |
| Single Type Assertion | `Next()` has exactly one type assertion at entry | Convention |
| Pure DeriveDesiredState | No dependency access - only use `spec` parameter | Convention |

Run CI-enforced architecture tests after every change:
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

## Stopping a worker — six cases, three signals

A worker can be wanted-stopped for six semantically distinct reasons. PR5
disambiguated these into three primitive signals on `WrappedDesiredState`,
plus the umbrella check `WorkerSnapshot.ShouldStop()`.

### The three primitive signals

| Signal | Permanence | Source | Meaning |
|--------|------------|--------|---------|
| `Desired.IsBeingRemoved()` | Permanent (this instance gone) | Supervisor (`SetBeingRemoved`) | Tear down: emit `SignalNeedsRemoval` from `Stopped`, lose retained state |
| `Desired.IsDisabled()` | Transient (parent says pause) | CHANGE-19 reducer (parent's `ChildSpec.Enabled=false`) | Stop and stay resident; flipping `Enabled=true` resumes |
| `Desired.Config.GetState()=="stopped"` | Transient (user says pause) | User YAML (`state: stopped`) | Stop and stay resident; flipping back to `running` resumes |

`WorkerSnapshot.ShouldStop()` is the umbrella OR of all three. State files
that don't care about the source ("just stop") use it directly. The Stopped
state needs to discriminate (permanent → emit removal; transient → stay
resident); see `helpers.StoppedNext` below for the canonical pattern.

### The six cases

| # | Case | Source | Permanence | Mapped signal |
|---|------|--------|------------|---------------|
| A | User `state: stopped` (YAML) | user config | Transient | `Config.GetState()=="stopped"` |
| B | Parent disable (`ChildSpec.Enabled=false`) | parent's `renderChildren` | Transient | `Desired.IsDisabled()` |
| C | Parent remove (omit from spec list) | parent returns shorter `[]ChildSpec` | Permanent | `Desired.IsBeingRemoved()` |
| D | Self-driven restart (`SignalNeedsRestart`) | worker self | Permanent (this instance) | `Desired.IsBeingRemoved()` |
| E | Graceful process shutdown | process exit | Permanent (this process) | `Desired.IsBeingRemoved()` |
| F | Supervisor self-protection (collector unresponsive) | Layer-3 escalation | Permanent (this instance) | `Desired.IsBeingRemoved()` |

Cases C, D, E, F all converge on `IsBeingRemoved=true` — they are different
upstream causes for the same "this instance must be torn down" outcome.
Cases A and B are the genuinely transient ones: the worker stops but stays
resident in `s.children`, ready to resume when the signal clears.

### Canonical Stopped state — `helpers.StoppedNext`

Most workers' `StoppedState.Next()` is a three-branch decision over the
three signals. The framework provides `internal/helpers.StoppedNext` so
state files don't have to reimplement it:

```go
func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
    snap := fsmv2.ConvertWorkerSnapshot[Config, Status](snapAny)
    return helpers.StoppedNext(s, snap, &TryingToConnectState{}, "attempting to connect")
}
```

Three branches:

1. **`Desired.IsBeingRemoved()`** → `SignalNeedsRemoval` (permanent: cases C, D, E, F)
2. **`!ShouldStop()`** → advance to the next state (signal cleared; resume)
3. **else** → stay in `Stopped` (transient: cases A and B; preserves in-memory state)

Workers with custom Stopped semantics (children rendering, additional
preconditions before advancing, custom signals) hand-roll the three branches
and keep `helpers.StoppedNext` as the reference. `transport/state_stopped.go`
is one such case — it emits children even while stopped.

### When to use which signal

- **`Enabled=false` (case B)**: per-child transient disable. The parent wants finer control than "all-or-nothing" (e.g., one of N children paused by config). Worker stays resident; flipping back to `Enabled=true` resumes.
- **Omit child from spec list (case C)**: parent's "this child should not exist anymore." Drives the supervisor's Phase-1 absent-from-specs teardown via `RequestRemoval` → `IsBeingRemoved=true` → `NeedsRemoval` → removal from `s.children`.
- **`SetBeingRemoved` directly (cases D, E, F)**: framework-driven removal — `SignalNeedsRestart` from a state, graceful process shutdown, or supervisor self-protection. State files don't write this flag; they emit signals and let the supervisor set it.
- **Config `state: stopped` (case A)**: user-facing stop. The worker stays resident; the user can resume by editing config back to `state: running`.

### Future: state-preservation across respawn

Designed but not yet implemented. See `003_klaus/artifacts/2026-04-09_fsmv2_state_recovery_design.md`. Two layers:

1. **`cse:"retain"` struct tag**: workers annotate `Status` fields that should survive restart. The collector merges retained values from `prevObserved` into the fresh COS-returned struct, only when the fresh value is zero. Workers don't change code — just struct tags.
2. **`DependencyHydrator` interface**: optional `HydrateDependencies(ctx, prevObs)` called once before first COS, for workers whose Dependencies must be warm before any state observation (e.g., CertFetcher's `CertHandler` map for TLS).

This design lands on top of the disambiguated signals: `IsDisabled` (case B,
transient) keeps the worker resident, so retained state on `Status` survives
the disable cycle automatically once `cse:"retain"` is implemented.
`IsBeingRemoved` (cases C, D, E, F) tears the worker down, so retained state
is lost — that is the intended boundary.

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

    // Stop check FIRST — IsBeingRemoved (permanent) routes to StoppingState.
    // Non-Stopped states that don't care about source can use snap.ShouldStop()
    // instead (umbrella over IsBeingRemoved + IsDisabled + Config "stopped").
    if snap.Desired.IsBeingRemoved() {
        return fsmv2.Transition(&StoppingState{}, fsmv2.SignalNone, nil, "removal requested")
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

`fsmv2.Transition` is the canonical return shape for state files. It is a non-generic alias for `fsmv2.Result[any, any]` that also accepts typed `Action[TDeps]` values directly.

> **Compat seam — migration window only**
>
> `fsmv2.Result[any, any](...)` and the `fsmv2.WrapAction[TDeps](&MyAction{})` wrapper remain as a compat seam for any code still using the old shape. Delete once all callsites use `fsmv2.Transition` + direct `&MyAction{}` construction. New state files must NOT use `Result`/`WrapAction`.

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

Workers register in `init()` with both worker and supervisor factories:

```go
func init() {
    if err := factory.RegisterWorkerType[ObservedState, *DesiredState](
        workerFactory,
        supervisorFactory,
    ); err != nil {
        panic(err)
    }
}
```

The folder name must match the worker type (e.g., `transport/` for type `"transport"`).

## Worker API v2 (WorkerBase)

New workers should use `WorkerBase[TConfig, TStatus, TDeps]` instead of the legacy 7-file pattern. Key types:

- **`WorkerBase[TConfig, TStatus, TDeps]`** — embed in your worker struct; provides `InitBase`, `Config()`, `DeriveDesiredState`. Use `register.NoDeps` as `TDeps` for workers with no custom deps.
- **`NewObservation[TStatus](status)`** — preferred constructor for `CollectObservedState` return values; the collector fills CollectedAt, framework metrics, action history, and accumulated worker metrics automatically after COS returns
- **`Observation[TStatus]`** — flat JSON serialization with framework fields (state, isBeingRemoved, children counts)
- **`WrappedDesiredState[TConfig]`** — promotes `BaseDesiredState` fields alongside TConfig
- **`WorkerSnapshot[TConfig, TStatus]`** — typed snapshot for state `Next()` methods
- **`ConvertWorkerSnapshot[TConfig, TStatus]`** — entry-point type assertion in states
- **`ExtractConfig[TConfig](desired)`** — typed config access in `CollectObservedState`
- **`WrapAction[TDeps]`** — *(deprecated, removed in PR3)* adapts typed actions to `Action[any]`. Prefer passing `&MyAction{}` directly to `fsmv2.Transition` — the framework auto-wraps via reflection.
- **`register.Worker[TConfig, TStatus, TDeps]`** — one-line registration (factory + supervisor + CSE types). Use `register.NoDeps` for zero-dep workers.

**Architecture validators** accept both APIs: `ConvertWorkerSnapshot` and `ConvertSnapshot` are valid entry points. The stop check uses `snap.ShouldStop()` (umbrella over `IsBeingRemoved`, `IsDisabled`, and `Config.GetState()=="stopped"`) for non-Stopped states that just need "should I stop?". Stopped states discriminate further: `snap.Desired.IsBeingRemoved()` for permanent removal (emit `SignalNeedsRemoval`) versus the transient signals (stay resident). `helpers.StoppedNext` encodes the canonical pattern. The deprecated flat `snap.IsBeingRemoved` field is gone — read it via `snap.Desired.IsBeingRemoved()`.

**Capability interfaces** (optional, detected via type assertion on first instantiation):
- `ActionProvider` — `Actions() map[string]Action[any]`
- `ChildSpecProvider` — `ChildSpecs() []config.ChildSpec`
- `MetricsProvider` — `Metrics() []prometheus.Collector`
- `GracefulShutdowner` — `Shutdown(ctx context.Context) error`
- `ChildrenViewConsumer` — `SetChildrenView(view config.ChildrenView) ObservedState`

**L5 invariant**: `WorkerBase` must NEVER implement optional capability interfaces. Only the concrete worker struct should implement them.

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

### Parent-driven stop flows through IsDisabled

There is no longer a separate `ParentMappedState` signal on `Observation[T]`. When a parent wants a child paused it sets `ChildSpec.Enabled=false`; the CHANGE-19 reducer (`supervisor/reconciliation.go`) translates that into `IsDisabled=true` on the child synchronously, before the child's tick. The child stays resident in `s.children` and resumes when the parent flips `Enabled=true` again.

When a parent wants a child gone permanently it omits the spec from its `[]ChildSpec` return; the supervisor's Phase-1 absent-from-specs path drives `RequestRemoval` → `IsBeingRemoved=true` → `SignalNeedsRemoval` from Stopped → removal from `s.children`. This is one-way: a removed child does not come back without a fresh spec.

State files therefore use `snap.ShouldStop()` (umbrella over `IsBeingRemoved`, `IsDisabled`, and Config "stopped") for non-Stopped states that don't care about the source, and discriminate in `Stopped` via `helpers.StoppedNext`. There is no parent-mapped-state path for child workers using `Observation[T]` to consult.
