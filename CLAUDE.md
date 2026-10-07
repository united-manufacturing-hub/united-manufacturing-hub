# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The United Manufacturing Hub (UMH) is an Industrial IoT platform for manufacturing data ingestion and management. It has two main components:

1. **UMH Core** (`umh-core/`) - Modern single-container edge gateway
2. **UMH Classic** (`deployment/united-manufacturing-hub/`) - Full Kubernetes deployment with Helm charts

umh-core runs benthos-umh (stream processing) and Redpanda (Kafka-compatible broker) as S6-supervised processes. The Management Console sends actions to umh-core and receives status from it.

## Terminology

- **Bridge** (UI) = `protocolConverter:` (YAML) = Protocol Converter (legacy)
- **Stand-alone Flow** (UI) = `dataFlow:` (YAML) = Data Flow Component/DFC (legacy)
- **Stream Processor** = `dataFlow:` with `sources:[]` array (aggregates multiple topics)
- **Data Contract** = underscore-prefixed type (`_raw`, `_pump_v1`, `_maintenance_v1`)
- **Virtual Path** = optional organizational segments in topics (e.g., `motor.electrical`)
- **Tag** = single data point/sensor (industrial term)
- **_raw** → **_devicemodel_v1** → **_businessmodel_v1** (data progression)
- **Permission grant** (only in docs.umh.app) = what the Management Console issues so a user or instance can access instances. Never call it a certificate. "Certificate" is reserved for TLS. Describe what actually happens on the user's screen: "permission grant" or "unlocks access to your X"

## Non-Intuitive Patterns

- **Variable flattening**: user `variables.IP` → `{{ .IP }}` (user variables become top-level)
- **S6 logs**: `.s` = clean rotation, `.u` = unfinished (container killed), `current` = active log
- **Empty FSMState**: `''` means S6 returns nothing (directory missing/corrupted)
- **FSM precedence**: Lifecycle states ALWAYS override operational states
- **One tag, one topic**: Never combine sensors in one payload (avoids timing/merge issues)
- **Bridge = Connection + Source Flow + Sink Flow**: Connection only monitors network availability
- **Data validation**: Happens at UNS output plugin, not at source
- **Bridge states**: `starting_failed_dfc_missing` = no data flow configured yet
- **Resource limiting**: `agent.enableResourceLimitBlocking` blocks bridge creation when resources are constrained. Default: ≤70% CPU; ~5 bridges per CPU core after reserving 1 for Redpanda (`umh-core/pkg/constants/container.go`)
- **Config.yaml is the source of truth**: the Management Console never writes benthos config. It changes `config.yaml`, and umh-core generates the benthos config from it.

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
- `umh-core/pkg/fsm/`: FSMv1 state machines (Benthos, Redpanda, S6, …). Legacy.
- `umh-core/pkg/fsmv2/`: FSMv2 framework and workers. **New logic is built as an FSMv2 worker.** Read `umh-core/pkg/fsmv2/CLAUDE.md` before working there.
- `umh-core/pkg/communicator/`: Management Console connection, action handlers (`actions/`), status subscribers.
- `umh-core/pkg/config/`: `config.yaml` parsing, templates and variables.
- `umh-core/docs/`: user-facing GitBook docs (rules in `umh-core/docs/CLAUDE.md`).

### FSMv1 pattern (`pkg/fsm/<component>/`)

`machine.go` (states, transitions), `fsm_callbacks.go` (fail-free callbacks, logging only), `actions.go` (idempotent operations that can fail and retry), `reconcile.go` (single-threaded control loop, the only place that modifies state), `models.go` (types).

- State precedence: Lifecycle (`to_be_created`, `removing`) > Operational (`running`, `stopped`)
- Actions must be idempotent and handle context cancellation. Failed transitions retry with exponential backoff.
- Never change FSM state directly. Always go through reconciliation.

### Data Architecture

**Two-layer model**:
- **Device models** (`_pump_v1`): Equipment internals, sites control
- **Business models** (`_maintenance_v1`): Enterprise KPIs, aggregated views

**UNS principles**:
- **Publish regardless**: Producers don't wait for consumers
- **Entry/exit via bridges**: All data validated at gateway
- **Location path**: WHERE in organization (enterprise.site.area)
- **Device model**: WHAT data exists (temperature, pressure)
- **Virtual path**: HOW to organize within model (motor.electrical)

## Testing

- Ginkgo v2 with Gomega matchers. Integration tests use Testcontainers.
- Unit tests focus on business logic. Do not mock FSM internals.
- Do not commit focused specs (`FIt`, `FDescribe`).
- What to test, including error paths: [Testing](https://engineering.umh.app/engineering/development-process/how-to-build/testing).

## Code Rules

- **Error handling**: Return errors up the stack and handle them in the reconciliation loop. Decide where each error goes with the questions in [Error management](https://engineering.umh.app/engineering/development-process/how-to-build/coding-standards/error-management).
- **Bumping benthos-umh**: automated. Each benthos-umh release opens a PR here (author `umh-cross-repo-automation`) that changes `BENTHOS_UMH_VERSION` in `umh-core/Makefile` and adds the CHANGELOG entry. Review it; do not write the bump by hand.

### Changelog

Every PR with user-visible changes must add an entry to `umh-core/CHANGELOG.md` under the `## Unreleased` section at the top. For format, voice, and what to include/skip, follow the `changelog-writing` skill (use `/changelog-entry` to generate entries). Never create a new version section — only add to `## Unreleased`. The section is renamed to a version number at release time (see `umh-core/RELEASING.md`).

- **CI enforcement**: PRs that change files under `umh-core/` must modify CHANGELOG.md, or CI fails. Add the `skip-changelog-guard` label to bypass (for CI/CD, refactoring, or test-only changes). The check reads labels from the triggering event, so after adding the label, push a commit to re-run it.
- **Automation**: On release published, workflows sync entries to changelog.umh.app and populate GitHub Release notes from CHANGELOG.md.

## More context on demand

- `.claude/rules/config-templates.md`: template expansion and location paths (loads when you open `umh-core/pkg/config/**` or the bridge and stream-processor services).
- `.claude/rules/communicator.md`: how actions and status travel between the Management Console and umh-core (loads in `umh-core/pkg/communicator/**`).
- `.claude/skills/umh-core-troubleshooting/`: support investigations (instance offline, stuck bridges, log patterns, cross-repo tracing).

## Engineering Handbook

Our shared standards live at https://engineering.umh.app. Start with:

- Go: https://engineering.umh.app/engineering/development-process/how-to-build/coding-standards/go
- Error management: https://engineering.umh.app/engineering/development-process/how-to-build/coding-standards/error-management
- FSMv2 workers: https://engineering.umh.app/engineering/development-process/how-to-build/coding-standards/fsmv2-workers
- Product standards: https://engineering.umh.app/product/product-standards ([Opinionated simplicity](https://engineering.umh.app/product/product-standards/opinionated-simplicity) · [Code and UI](https://engineering.umh.app/product/product-standards/code-and-ui) · [Immediate trust](https://engineering.umh.app/product/product-standards/immediate-trust))
- [How to build](https://engineering.umh.app/engineering/development-process/how-to-build) · [Testing](https://engineering.umh.app/engineering/development-process/how-to-build/testing) · [How to ship](https://engineering.umh.app/engineering/development-process/how-to-ship)
- Why we exist: https://engineering.umh.app/company/why-we-exist

Apply these while writing code, without opening the links:

- New logic is an FSMv2 worker. A worker is not healthy until a check proves it healthy. A simple worker starts degraded.
- For every error, ask in order: can a retry fix it (retry, the user does not see it)? Can the user fix the cause (show it, e.g. set the bridge to degraded)? Can only UMH fix it (Sentry, plus logs)? What happens to the failed data (persistent error → dead-letter queue)?
- A user-facing error names the invalid value, says what the user changes, and never blames the user.
- Every change in behaviour comes with a test that goes red without the change.
- Anything that changes behaviour goes behind a feature flag.
