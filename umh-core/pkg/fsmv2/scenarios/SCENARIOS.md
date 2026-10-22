# Writing a scenario

A scenario runs the real supervisor and real workers against mocks, and checks
what the store shows. It is a `Scenario` registered in `Registry`
(`scenarios/registry.go`), and `scenarios/registry_run_test.go` runs every
registered one. The example to copy is `HelloworldScenario` in
`scenarios/helloworld.go`.

## Mocks

`Dependencies` builds the mocks and returns the dependency map (and optionally
a cleanup). `Run` changes a mock, never the map (see `Env.Dependencies`).

**Where to mock.** Mock the lowest interface the production code already uses
to reach outside the process: the network, a database, the filesystem.
Everything above that interface then runs for real in the scenario.

| Worker | Production uses | The scenario passes |
|---|---|---|
| nmap | `*net.Dialer`, through `fsmv2nmap.Dialer` | `mockDialer` (`scenarios/nmap.go`) |
| historian | `*pgxpool.Pool`, through `timescalemetrics.Database` | `timescalemetrics.FakeDatabase` |

**The production change.** The worker declares a typed key for that interface
(`fsmv2nmap.DialerKey`) and reads it in its constructor with
`config.LookupDependency`. When the key is absent, the worker uses the
production default, such as the dialer or the shared pool.

The interface may be new if it is small, declared where it is used, and the
real type already satisfies it, as `*net.Dialer` satisfies `fsmv2nmap.Dialer`.
Do not wrap production code in a new layer, or move it, only so that a
scenario can replace it.

**Which mock.** Use a standard mock when one exists. That is either a mock the
package ships beside the real code, such as `filesystem.NewMockFileSystem`, or
one from the Go standard library or the dependency's own library, such as
`httptest.Server`, `fstest.MapFS` or `net.Pipe`. Otherwise, write a fake next
to the code it imitates, so the fake changes when that code changes. A fake
answers every call for the whole run. `Run` changes its answers through a
setter. Return errors the code classifies the same way as the real ones, for
example a `*pgconn.PgError` with code `28P01` for a rejected password.

## Run

`Run` creates workers with `env.Client.Upsert`, the way a user's config does.
Before each change, call `env.Step` with a short description of the change.
After the change, call `env.WaitFor` with a check that reads the store through
the client and returns what it saw. A check that has not seen the worker yet
reports that it is not done. Each wait fails after `waitForTimeout`
(`scenariorunner/env.go`). A check that ignores its context can hold the wait
past that timeout.

## What fails a run

A logged error is unexpected unless its message contains an entry of
`ExpectedErrors` or of `alwaysAllowedMessages`, or its value carries a cause
listed in `ExpectedErrorCauses` (`scenariorunner/env.go`). A
logged warning is checked the same way against `ExpectedWarnings`.

An entry of `ExpectedErrors`, `ExpectedErrorCauses` or `ExpectedWarnings`
both allows that message and requires it: every listed entry must appear
in a logged error or warning of its kind, or `RunResult.Err` names it.
Everything logged through the run's logger counts, which covers the
scenario's workers, their supervisors and its `Run`. The runner's own
messages and the store's messages do not count.

`scenarios.Run` returns an error when `Run` returns one. It also returns an
error when the run logs an unexpected error before `Run` returns.

Five other failures do not make `scenarios.Run` return an error. After the
run ends, `RunResult.Err` holds the first of these that applies:

1. an unexpected error logged after `Run` returns;
2. an unexpected warning;
3. a listed entry that never appeared, named per missing entry;
4. a stored state that its worker type may not report;
5. a store read that fails during that state check.

The CLI exits 1 on a set `Err`, and `scenarios/registry_run_test.go` fails on it.
Valid states per worker type are in `validWorkerStates`
(`scenarios/valid_worker_states.go`). Add a new worker type there, or its states go
unchecked.

## Running it

```bash
go run ./pkg/fsmv2/cmd/runner --scenario=helloworld
go test -tags=test -count=1 -v ./pkg/fsmv2/scenarios/ -ginkgo.focus="helloworld"
```

A scenario's runner stops one second after the scenario ends;
`--duration=0` keeps it running until Ctrl+C.

## Rules

- No global setters for mocks: `register.SetGlobalDeps` is not for scenarios.
