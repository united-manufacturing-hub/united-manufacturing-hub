# CLAUDE.md

This repository holds two products:

1. **UMH Core** (`umh-core/`): the single-container edge gateway. Most work happens here.
2. **UMH Classic** (`deployment/united-manufacturing-hub/`): the Kubernetes deployment with Helm charts.

How we work is in the Engineering Handbook (links below). What umh-core does for users is in `umh-core/docs/`, published at docs.umh.app. This file holds what you need to work in this repository.

## Terminology

The YAML and the code still use older names than the UI:

- **Bridge** (UI) = `protocolConverter:` (YAML) = protocol converter (code)
- **Stand-alone Flow** (UI) = `dataFlow:` (YAML) = data flow component, DFC (code)

## Essential Commands

Run these from `umh-core/` (targets are in `umh-core/Makefile`).

**Build**: `make build` (standard), `make build-debug` (debug), `make build-pprof` (profiling)

**Test**: `make test` (all), `make unit-test`, `make integration-test`, `make benchmark`. One package: `go test -tags=test -count=1 ./pkg/<pkg>/...`

**Dev**: `make test-graphql` (port 8090), `make pod-shell`, `make test-no-copy` (use current config), `make test-debug`

**Clean** (destructive): `make stop-all-pods`, `make cleanup-all`

**MUST run before completing tasks**: `make vet`, `make nilaway` (both required in CI), `golangci-lint run ./...`

**Git**: Default branch is `staging`. Lefthook runs `make vet`, gofmt and license headers on commit; nilaway, golangci-lint and a protobuf sync check (`make build-protobuf`) on push.

## Architecture

- `umh-core/cmd/main.go`: entry point. Reads `/data/config.yaml` and starts the control loop.
- `umh-core/pkg/fsm/`: FSMv1 state machines (Benthos, Redpanda, S6, …). Legacy. The package doc describes the files every component has.
- `umh-core/pkg/fsmv2/`: FSMv2 framework and workers. Read `umh-core/pkg/fsmv2/CLAUDE.md` before working there.
- `umh-core/pkg/communicator/`: Management Console connection, action handlers (`actions/`), status subscribers. The package doc of `router/` describes how actions and status travel.
- `umh-core/pkg/config/`: `config.yaml` parsing, templates and variables. `config.yaml` is the source of truth: umh-core renders every benthos config from it.
- `umh-core/docs/`: user-facing GitBook docs (rules in `umh-core/docs/CLAUDE.md`).

## Testing

- Ginkgo v2 with Gomega matchers. Integration tests use Testcontainers.
- Unit tests focus on business logic. Do not mock FSM internals.

## Changelog

Every PR with user-visible changes adds an entry to `umh-core/CHANGELOG.md` under `## Unreleased` at the top. Never create a version section: `umh-core/RELEASING.md` describes the rename at release time. For format and voice, follow the `changelog-writing` skill (`/changelog-entry` drafts an entry).

PRs that change files under `umh-core/` must modify `CHANGELOG.md`, or CI fails. The `skip-changelog-guard` label bypasses the check (for CI/CD, refactoring, or test-only changes). The check reads labels from the event that started the run, so after adding the label, push a commit to re-run it.

## Reading logs in a running instance

umh-core is one container. S6 supervises every process in it: the agent, Redpanda and one benthos-umh process per flow. Each process logs to `/data/logs/<service>/`: `current` is the live file, `@<timestamp>.s` an archive rotated cleanly, and `@<timestamp>.u` the file that was `current` when the container was killed. The UI can show stale status while data flows, so check the logs and the Kafka topics too (`rpk topic consume`). Service names, rotation and S6 directories: `umh-core/docs/reference/container-layout.md`. A benthos-umh service directory holds the rendered config at `config/benthos.yaml`. `umh-core/tools/s6-analyzer` reads a service directory's S6 state: PID, uptime and exit codes.

## Engineering Handbook

Our shared standards live at https://engineering.umh.app. Look up the pages your task needs before you write code. Each page has a Markdown version: append `.md` to its URL. https://engineering.umh.app/llms.txt lists every page. Start with:

- Go: https://engineering.umh.app/engineering/development-process/how-to-build/coding-standards/go
- Error management: https://engineering.umh.app/engineering/development-process/how-to-build/coding-standards/error-management
- FSMv2 workers: https://engineering.umh.app/engineering/development-process/how-to-build/coding-standards/fsmv2-workers
- Product standards: https://engineering.umh.app/product/product-standards ([Opinionated simplicity](https://engineering.umh.app/product/product-standards/opinionated-simplicity) · [Code and UI](https://engineering.umh.app/product/product-standards/code-and-ui) · [Immediate trust](https://engineering.umh.app/product/product-standards/immediate-trust))
- [How to build](https://engineering.umh.app/engineering/development-process/how-to-build) · [Testing](https://engineering.umh.app/engineering/development-process/how-to-build/testing) · [How to ship](https://engineering.umh.app/engineering/development-process/how-to-ship)
- Why we exist: https://engineering.umh.app/company/why-we-exist
