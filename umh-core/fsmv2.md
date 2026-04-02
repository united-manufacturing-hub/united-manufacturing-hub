# FSMv2 Worker Migration Notes

## CertFetcher: Old Worker Pattern → WorkerBase API

The certfetcher was the first gatekeeper worker migrated to the WorkerBase API.

### What changed

| Old pattern | New pattern |
|-------------|-------------|
| `*helpers.BaseWorker[*Deps]` | `fsmv2.WorkerBase[Config, Status]` |
| Separate `logger` + `identity` fields | Accessed via WorkerBase |
| Manual `CollectedAt`, `Metrics.Framework`, `LastActionResults` | `fsmv2.NewObservation(status)` handles all |
| Custom `DeriveDesiredState` (~20 lines) | Deleted, WorkerBase provides it |
| Custom `GetInitialState()` | `fsmv2.RegisterInitialState` in state init() |
| `helpers.ConvertSnapshot[Obs, *Des]` in states | `fsmv2.ConvertWorkerSnapshot[Config, Status]` |
| `snap.Observed.HasSubHandler` | `snap.Status.HasSubHandler` |
| `snap.Desired.IsShutdownRequested()` | `snap.IsShutdownRequested` (field) |
| `snap.Observed.CollectedAt` | `snap.CollectedAt` (promoted) |
| `factory.RegisterWorkerType[Obs, *Des]` | `factory.RegisterWorkerAndSupervisorFactoryByType` + CSE type registry |
| Full `CertFetcherObservedState` struct | `CertFetcherStatus` (business only) + `Observation[Status]` wrapper |
| `CertFetcherDesiredState` | `WrappedDesiredState[Config]` |

### What was deleted

- `DeriveDesiredState` — WorkerBase handles YAML parsing + defaults. Certfetcher has no user config, so nil spec = always running works out of the box.
- `GetInitialState` — replaced by `fsmv2.RegisterInitialState("certfetcher", &StoppedState{})` in the state package's `init()`.
- `CertFetcherObservedState` boilerplate fields (CollectedAt, Metrics, LastActionResults, State, ShutdownRequested) — all provided by `Observation[T]`.
- `CertFetcherDesiredState` struct — replaced by generic `WrappedDesiredState[Config]`.

## Why the Cert Cache is NOT in FSMv2

The certificate cache lives in `certificatehandler.Handler` (in-memory map), not in FSMv2 infrastructure.

**Performance**: The gatekeeper reads `certHandler.Certificate(email)` on every inbound message. CSE storage round-trips would be too slow.

**Serialization**: `x509.Certificate` objects are not JSON-serializable. CSE uses JSON.

**Cross-worker access**: The gatekeeper (not an FSMv2 worker) reads certs, the certfetcher (FSMv2 worker) writes them. CSE observed state is per-worker.

**Lifetime**: Certs persist across ticks until refreshed. CSE observed state is overwritten every tick.

The certfetcher's `CertFetcherStatus` carries only counts (subscriber count, cached cert count) for state machine decisions. The actual cert objects stay in the handler's memory.

## What Was Confusing

### GetDependenciesAny override is required but not obvious

Without overriding `GetDependenciesAny()`, the supervisor passes `*BaseDependencies` to actions. The action's type assertion then fails silently. If your worker has custom dependencies, you MUST add:

```go
func (w *MyWorker) GetDependenciesAny() any {
    return w.deps
}
```

### RegisterInitialState in a different package

The initial state registration moved from `GetInitialState()` on the worker to `init()` in the state package. The worker needs a blank import to trigger it:

```go
_ "...workers/certfetcher/state"
```

Without it, the worker panics at startup with "no initial state registered".

### CSE type registry is separate from factory registration

`factory.RegisterWorkerAndSupervisorFactoryByType` registers worker/supervisor factories. CSE storage needs its own registration via `storage.GlobalRegistry().RegisterWorkerType()`. These are two separate init calls. Forgetting the CSE one causes silent failures in state persistence.

### CertFetcherDeps interface in snapshot package

Actions call methods on dependencies, but importing the certfetcher package from the action package creates a cycle. The solution is a `CertFetcherDeps` interface in the snapshot package. All FSMv2 workers follow this pattern (PullDependencies, PushDependencies, etc.). It's not new-API-specific but was initially confusing.

### FeatureForWorker availability depends on branch

`FeatureForWorker(workerType)` exists on staging (ENG-4718) but not on all feature branches. The certfetcher uses `FeatureGatekeeper` as fallback. After rebasing onto staging, switch to `FeatureForWorker(d.GetWorkerType())` for consistency.

## What Worked Well

- `NewObservation` eliminated ~15 lines of boilerplate from CollectObservedState
- Deleting `DeriveDesiredState` removed ~20 lines of config parsing the certfetcher didn't need
- `snap.Status.HasSubHandler` reads better than `snap.Observed.HasSubHandler`
- The `lazySubHandler` pattern (extraDeps provider callback) works unchanged with the new API
- The 3-state machine (Stopped/Running/Degraded) and error tracking needed zero changes
