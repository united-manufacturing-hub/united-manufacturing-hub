## Silent Failure Hunter Review

### Critical (must fix before merge)

- `umh-core/pkg/fsmv2/supervisor/internal/collection/collector.go:459-462` — ~~**Shutdown injection is silently dead for all new concrete ObservedState types.**~~ **FIXED in iter-6 P1-1.** Duck-type probe renamed from `SetBeingRemoved` to `SetShutdownRequested` (collector.go lines 460-463). Verified: `grep -rn "SetBeingRemoved" --include="*.go" pkg/fsmv2/ | grep -v "_test.go"` returns zero matches.

- `umh-core/pkg/fsmv2/workers/example/exampleslow/worker.go:114,124` — ~~**`renderedConfig` result is computed and then unconditionally discarded.**~~ **FIXED in iter-6 P1-2.** `yaml.Unmarshal([]byte(renderedConfig), &parsed)` is now used (worker.go lines 122-124). Verified: `grep -n "renderedConfig\|yaml.Unmarshal" pkg/fsmv2/workers/example/exampleslow/worker.go` shows lines 116, 122, 123.

### Important (should fix)

- `umh-core/pkg/fsmv2/supervisor/api.go:82-88` — ~~**First `DeriveDesiredState` error is silently swallowed without logging.**~~ **FIXED in iter-6 P1-3.** `SentryWarn` with `deps.Err(err)` added before the nil-spec fallback (api.go lines 82-88).

- `umh-core/pkg/fsmv2/supervisor/reconciliation.go:790-796` — ~~**Both JSON marshal and unmarshal errors in the `originalUserSpec` storage path are silently swallowed.**~~ **FIXED in iter-6 P1-5.** `SentryWarn` added on both marshal and unmarshal failure paths (reconciliation.go).

- `umh-core/pkg/fsmv2/workers/transport/pull/worker.go:1765-1771` (and the symmetric `push/worker.go` section) — **`LoadObservedTyped` error is only logged at Debug level before previous metrics are used as the accumulated base.** If the state reader fails for any reason other than ErrNotFound (e.g., a deserialization error after a schema change, a CSE store corruption), the worker silently falls back to zero-valued `prevWorkerMetrics`, meaning all cumulative worker counters reset to zero on the next tick. The only signal is a `Debug` log (`observed_state_load_failed`) which is typically invisible in production log levels. This causes silent metric loss without the operator being informed.

  **Fix:** For errors that are not `ErrNotFound`, log at `Warn` level (or use `SentryWarn`) so the reset is observable. ErrNotFound is expected on first tick and can remain Debug-only.

- `umh-core/pkg/fsmv2/config/childspec.go:289-299` (`ChildrenViewSnapshot.Counts()`) — **`unhealthy` count excludes stopped children, but the `IsHealthy=false, IsStopped=false` combination for children in `PhaseStarting`, `PhaseRunningDegraded`, or `PhaseStopping` is correctly classified as unhealthy. However, the `ChildrenViewSnapshot` struct populates `IsStopped` from `ChildInfo.IsStopped` which is set in `buildChildInfo()`. If a future caller constructs a `ChildrenViewSnapshot` directly (e.g., in a test or via JSON deserialization of old data) without setting `IsStopped`, the `Counts()` method will count ALL non-healthy children as unhealthy, including stopped ones.** This is a latent correctness issue: the `IsStopped` field on `ChildInfo` is new in this PR and existing serialized snapshots in CSE storage will have `is_stopped: false` (zero value), causing `Counts()` to over-count unhealthy children after a rolling deploy until all snapshots are refreshed. This can trigger spurious Degraded→Running transitions.

  **Mitigation:** Document the zero-value behavior in the `IsStopped` field comment. Consider deriving `IsStopped` from `StateName` as a fallback when the field is zero.

- `umh-core/pkg/fsmv2/workers/example/helloworld/worker.go:85-88` — ~~**Context cancellation at entry no longer returns an error.**~~ **FIXED in iter-6 P0-B.** `return nil, ctx.Err()` restored in the ctx.Done branch (worker.go line 87). Companion test reverted to assert error returned (worker_test.go).

### Suggestions (nice to have)

- `umh-core/pkg/fsmv2/workers/transport/worker.go` and `pull/worker.go` and `push/worker.go` (COS implementations) — The `CollectObservedState` methods in the new concrete-type workers now perform `LoadObservedTyped` (a storage read) on every tick to accumulate worker metrics. This is a performance regression versus the previous `NewObservation` approach where the collector handled accumulation. The storage read happens in the collector goroutine at 100ms cadence for every worker; for a transport tree with 3 workers (transport, push, pull) this is 3 additional CSE reads per tick = ~30 reads/second. Consider documenting this tradeoff or moving accumulation back to the collector post-COS hook.

- `umh-core/pkg/fsmv2/config/childspec.go:289` — `ChildrenViewSnapshot.Counts()` is a new method that diverges from the interface docs for `ChildrenView.Counts()`. The interface comment says "healthy = PhaseRunningHealthy, unhealthy = everything except healthy and stopped" (matching `ChildrenManager.Counts()`), but the snapshot implementation relies on the `IsStopped` field being correctly populated. Add a comment explicitly stating the invariant dependency on `ChildInfo.IsStopped` being accurate.

- `umh-core/pkg/fsmv2/supervisor/api.go:192` — `workerLogger.Info("identity_created")` is a new log line without a `SentryWarn`/`SentryError` level. This is fine as an informational log, but it fires on every worker add (including reconciliation re-adds). In a system with many workers reconciling frequently, this could generate significant log volume. Consider `Debug` level or gating it on first creation only.
