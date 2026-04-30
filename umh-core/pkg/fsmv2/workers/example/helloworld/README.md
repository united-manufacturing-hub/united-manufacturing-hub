# Hello World Worker — canonical mini-worker reference

This is the reference implementation for the FSMv2 **mini-worker pattern**: the
smallest useful worker you can build on top of `WorkerBase[TConfig, TStatus]`,
registered with a single `register.Worker` call. Copy this folder as a starting
point for new workers and peel features on or off as you need them.

Helloworld itself is slightly richer than the strict minimum — it has mutable
per-worker dependencies, an action, and four states — so the sections below
call out which files are optional.

## The mini-worker skeleton

The absolute minimum worker is around 60 lines of Go and spans two files.
Nothing else is required to be a legal FSMv2 worker:

```go
// package minimal — the smallest legal FSMv2 worker.
package minimal

import (
    "context"

    "github.com/.../umh-core/pkg/fsmv2"
    "github.com/.../umh-core/pkg/fsmv2/config"
    "github.com/.../umh-core/pkg/fsmv2/deps"
    "github.com/.../umh-core/pkg/fsmv2/register"
)

const workerType = "minimal"

// TConfig: what the user writes in YAML.
type MinimalConfig struct {
    config.BaseUserSpec `yaml:",inline"` // provides GetState() for DeriveDesiredState
}

// TStatus: what CollectObservedState reports each tick.
type MinimalStatus struct{}

// Worker: embed WorkerBase so DeriveDesiredState, GetInitialState, Config() come for free.
type MinimalWorker struct {
    fsmv2.WorkerBase[MinimalConfig, MinimalStatus]
}

// Constructor signature is fixed by register.Worker.
func NewMinimalWorker(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, _ register.NoDeps) (fsmv2.Worker, error) {
    w := &MinimalWorker{}
    w.InitBase(id, logger, sr)
    return w, nil
}

// CollectObservedState is the only method you are required to implement.
func (w *MinimalWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
    return fsmv2.NewObservation(MinimalStatus{}), nil
}

func init() {
    register.Worker[MinimalConfig, MinimalStatus, register.NoDeps](workerType, NewMinimalWorker)
}
```

You still need at least one state (`stopped`) for the supervisor to drive the
FSM. Anything beyond this skeleton — actions, custom deps, child workers,
metrics — is purely additive. The rest of this document walks the helloworld
files and shows how to add each capability.

## State diagram

```text
                          !shutdown              HelloSaid       mood="sad"   mood!="sad"
 ┌─────────┐          ┌──────────────────┐          ┌─────────┐ ────────▶ ┌──────────┐
 │ Stopped │────────▶│ TryingToStart    │────────▶│ Running │ ◀──────── │ Degraded │
 └─────────┘          └──────────────────┘          └─────────┘           └──────────┘
      ▲                       │                          │                    │
      └───────────────────────┴──────────────────────────┴────────────────────┘
                                   shutdown
```

Every state checks `snap.Desired.IsShutdownRequested()` first and returns to `Stopped`
when it is true; the catch-all returns to the current state with no action.

## Control-loop mapping

| Phase | Implementation |
|-------|----------------|
| **Observe** (`CollectObservedState`) | Reads `deps.HasSaidHello()` and the mood file at `cfg.MoodFilePath` |
| **Derive** (`DeriveDesiredState`) | Provided by `WorkerBase` — parses YAML into `HelloworldConfig` via `ParseUserSpec` |
| **Evaluate** (`State.Next()`) | Checks shutdown, `HelloSaid`, and `Mood`; returns state or action |
| **Execute** (`Actions`) | `SayHello` logs a greeting and flips `deps.SetHelloSaid(true)` |

See [`pkg/fsmv2/README.md`](../../../README.md#the-control-loop) for the
framework-level explanation.

## File-by-file guide

```text
helloworld/
├── worker.go         # Worker struct, COS, Actions(), init registration
├── action.go         # SayHello action function + SayHelloActionName const
├── config.go         # HelloworldConfig (TConfig) + HelloworldStatus (TStatus)
├── dependencies.go   # HelloworldDependencies (mutable per-worker state for actions)
└── state/
    ├── stopped.go          # Initial state — waits for !shutdown
    ├── trying_to_start.go  # Emits SayHello action
    ├── running.go          # Steady state — checks mood
    └── degraded.go         # mood="sad" — returns to Running when mood clears
```

### `worker.go` (required)

Holds the worker struct embedding `fsmv2.WorkerBase[TConfig, TStatus]`, the
constructor matching `register.Worker`'s signature, and `CollectObservedState`.
Optional capability methods (here: `Actions`, `GetDependenciesAny`) live on the
same struct — the framework discovers them via type assertion.

Change it when: you add a capability interface (`ActionProvider`,
`ChildSpecProvider`, `MetricsProvider`, `GracefulShutdowner`,
`ChildrenViewConsumer`) or change what you observe.

### `config.go` (required)

`HelloworldConfig` embeds `config.BaseUserSpec` so `WorkerBase.DeriveDesiredState`
can read the `state` field from YAML. `HelloworldStatus` holds the fields the
worker reports each tick — it is your free-form observation payload.

Change it when: you add YAML fields (TConfig) or runtime-observed fields
(TStatus). Add `Validate()` on TConfig to opt into custom validation.

### `action.go` (optional — enables the `ActionProvider` capability)

A single pure function `SayHello(ctx, deps)` wrapped via `fsmv2.SimpleAction`
in `worker.Actions()`. The action is idempotent: it early-returns if hello was
already said.

Omit it when: the worker observes without mutating anything.

### `dependencies.go` (optional — only needed if actions mutate per-worker state)

`HelloworldDependencies` extends `*deps.BaseDependencies` with a `helloSaid`
flag guarded by `sync.RWMutex`. `worker.GetDependenciesAny()` returns it so
the framework passes it to actions.

Omit it when: actions talk only to external systems via `BaseDependencies`,
or when there are no actions.

### `state/*.go` (required — at least one state)

Each state file defines a state struct embedding the appropriate helper base
(e.g. `helpers.StoppedBase`, `helpers.RunningHealthyBase`), implements
`Next(snapAny)`, and returns either a transition or an action via
`fsmv2.Transition(...)`. The first conditional is always
`snap.Desired.IsShutdownRequested()`.

Change it when: you add states, transitions, or per-state business logic.
`StoppingState` must always progress (never self-return with `nil` — CI
enforces this).

## Step-by-step feature adoption

All options are additive — opt in when you need them, skip otherwise:

| Capability | How to opt in |
|------------|---------------|
| **Custom deps** | Define a typed deps struct, pass it as the third type parameter to `register.Worker[TConfig, TStatus, *MyDeps]`, and accept it in the constructor's final argument. Publish instances at parent wiring time via `register.SetDeps[TDeps](workerType, deps)`. |
| **Custom config validation** | Implement `Validate() error` on your `TConfig`. `WorkerBase.DeriveDesiredState` calls it before returning. |
| **Parent/child orchestration** | Implement `ChildSpecs() []config.ChildSpec` on your worker struct (`ChildSpecProvider` capability). The supervisor spawns/manages the declared children. |
| **Custom metrics** | Implement `Metrics() []prometheus.Collector` (`MetricsProvider`). Record inside actions via `deps.MetricsRecorder()`; the collector drains and merges. |
| **Custom actions** | Return them from `Actions() map[string]fsmv2.Action[any]` (`ActionProvider`). Wrap pure functions with `fsmv2.SimpleAction[TDeps](name, fn)`. |
| **Observing children health** | Implement `SetChildrenView(view any)` on your TStatus (`ChildrenViewConsumer`). |
| **Graceful shutdown hooks** | Implement `Shutdown(ctx context.Context) error` (`GracefulShutdowner`). |
| **Late-bound deps (parent publishes before child init)** | Use the `SetDeps`/`GetDeps` pattern in `register/` — parent calls `register.SetDeps` during its own init; child receives the published deps at construction time. |

## Config example

```yaml
config: |
  state: running
  moodFilePath: /tmp/helloworld-mood
```

```bash
echo "happy" > /tmp/helloworld-mood   # stays Running
echo "sad"   > /tmp/helloworld-mood   # → Degraded
rm /tmp/helloworld-mood               # → Running (no file = fine)
```

## Usage

Blank-import the package (and its `state/` sub-package) to register the worker
type and its initial state at init time:

```go
import (
    _ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld"
    _ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)
```

## Common mistakes

1. **Non-idempotent actions** — `SayHello` checks `HasSaidHello()` before acting; actions may run more than once because the supervisor retries failed actions tick-by-tick
2. **Missing registration** — forgot `register.Worker[...]()` in `init()`; worker type is unknown at runtime
3. **Missing blank import** — forgot to import the package in the scenario/main entry point; `init()` never runs
4. **Hyphenated folder name** — Go types cannot contain hyphens, so `example-hello` breaks the folder-equals-workerType invariant enforced by architecture tests
5. **Type prefix mismatch** — folder `helloworld` must pair with `HelloworldConfig` / `HelloworldStatus` (one capital, no camel-case seam on "world")
6. **Returning state AND action** — a state's `Next()` must return one or the other, never both; the supervisor panics if both are set

## See also

- [`pkg/fsmv2/doc.go`](../../../doc.go) — API contracts, patterns, best practices
- [`pkg/fsmv2/README.md`](../../../README.md) — Triangle model, tick loop, quick start
- [`pkg/fsmv2/factory/README.md`](../../../factory/README.md) — folder/worker-type invariants, registration internals
- [`pkg/fsmv2/register/register.go`](../../../register/register.go) — `register.Worker` contract, `SetDeps`/`GetDeps` handoff
- [`pkg/fsmv2/workers/example/examplechild/`](../examplechild/) — fuller example with states, actions, custom deps
- [`pkg/fsmv2/workers/example/exampleparent/`](../exampleparent/) — parent worker with child management
