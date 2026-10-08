# FSMv2

The rules every FSMv2 worker follows are on the handbook page [FSMv2 workers](https://engineering.umh.app/engineering/development-process/how-to-build/coding-standards/fsmv2-workers). They say when a worker may report healthy and when a parent goes degraded. Errors follow [Error management](https://engineering.umh.app/engineering/development-process/how-to-build/coding-standards/error-management).

This file says where the FSMv2 rules for this repository live in the code.

## Architecture test

`architecture_test.go` checks the source of every worker against the patterns in `internal/validator/`. A failure names the pattern, why it exists and the correct code (`internal/validator/registry.go`). Run it from `umh-core/` after every change to a worker:

```bash
go test -tags=test -count=1 ./pkg/fsmv2/ -ginkgo.focus=Architecture -v
```

Do not filter with `-run Architecture`. The whole Ginkgo suite is one Go test, `TestFsmv2`, so that filter runs no specs and still passes.

## Where to read

| Topic | Where |
|---|---|
| Framework overview, states, actions, parent and child workers | `doc.go`, `README.md` |
| A worker to copy | `workers/example/examplechild/`, `workers/example/exampleparent/` |
| Returning from `Next()`, reason strings, the children a parent wants | `Transition` and `NextResult` in `api.go`. The architecture test rejects `fsmv2.Result[...]` in state files. |
| `WorkerBase`, `BindDeps` and the typed deps accessor | `worker_base.go` |
| Registration and dependencies a parent passes to its children | `register/` |
| Child specs and `Enabled` | `config/childspec.go` |
| Observations and worker metrics | `NewObservation` in `observation.go`, `wrapNewObservation` in `supervisor/internal/collection/collector.go` |
| Supervisor, tick loop, graceful shutdown budget | `supervisor/doc.go` |
| Dependencies | `DEPENDENCIES.md` |
| Porting an FSMv1 component | `MIGRATION.md` |
