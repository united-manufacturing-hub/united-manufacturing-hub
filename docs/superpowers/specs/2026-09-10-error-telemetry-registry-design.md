# Spec: an error-telemetry registry for umh-core

Status: Phase 1 draft, awaiting review. Intensity: **Standard** (`vsdd/status.md`).

## Problem

A umh-core Sentry alert names an event and nothing else. `event_name` arrives as
`persistent_push_failure` or `stuck_action_detected`, and whoever is triaging has to
grep the repo to learn what the event means, who owns it, and whether it is worth
waking someone. Roughly 149 distinct names exist across 165 call sites, and eight are
prose rather than identifiers, among them `cpu: startup cgroup snapshot failed; quota
signals omitted` and `upsert failed, will retry next tick`.

Nothing declares the set. A new event is a string literal at a call site, so nobody
can list what umh-core can report, two call sites can spell the same event two ways,
and the severity is decided per call by whether the author reached for `SentryWarn`
or `SentryError`.

ManagementConsole solved this for itself. `shared/error_telemetry/` holds one YAML
file listing every event with a one-line brief and a severity, generates Go and
TypeScript identifiers from it, and offers one function, `ReportError`, that takes
nothing else. umh-core has no equivalent, and PR 2744 has just added the
repo's first hierarchical names (`cpu::read_failed::cpu_stat::missing`), so the
divergence is about to harden: the next person adding an event copies whichever
neighbour they happen to read.

The reader to keep in mind here is the next maintainer adding an event, and the
on-call engineer reading an alert at an inconvenient hour.

## What We're Shipping

- **A registry**, `umh-core/pkg/telemetry`, whose `telemetry.yaml` lists every event
  umh-core can report, each with a `brief` and a `severity`. Editing that file is how
  an event comes to exist.
- **Generated Go identifiers** from that YAML, reached as
  `telemetry.Transport.Push.PersistentFailure`, so a typo is a compile error rather
  than a Sentry issue nobody recognises.
- **One reporting method** on `deps.FSMLogger`, replacing `SentryWarn` and
  `SentryError`: `Sentry(id, feature, hierarchyPath, cause, fields...)`. The log
  level comes from `id.Severity`, so an event's severity is declared once, where the
  event is declared.
- **The brief on every Sentry event.** When `cause` is nil the method synthesizes one
  from `id.Brief`, which is what ManagementConsole does at
  `shared/error_telemetry/report_error.go:106`. The existing hook then renders
  `Exception.Type` from the message and `Exception.Value` from the error, so the tag
  is the issue title and the brief is the line under it.
- **All ~149 existing names converted** to the hierarchy and registered, including
  the eight prose ones, across all 165 call sites.
- **A test that fails when the generated file is stale**, so an edited YAML with a
  forgotten `make generate-telemetry` cannot ship.

## What We're NOT Shipping

- **No shared module with ManagementConsole.** umh-core is Apache-2.0 and public;
  ManagementConsole is private. `umh-core/Dockerfile:85-91` vendors both `cryptolib`
  and `shared` from the `mc` build context, and MC's taxonomy lives in
  `shared/error_telemetry/`, so that coupling already exists for a
  `CRYPTOLIB=true` build. It does not exist for the default build, which has no MC
  sources and falls back to the noop validator. Sharing the taxonomy would therefore
  make core logging work only in the private-source build, which is why the two YAML
  files stay separate and aligned by convention, as the three `UX_STANDARDS.md` files
  already are.
- **Almost no change to the Sentry hook.** `pkg/fsmv2/sentry/hook.go:179` already
  sets `event.Tags["event_name"]` from the message and fingerprints on it, so
  `id.Tag` needs no hook edit to become searchable. One edit is unavoidable:
  `IsInternalFrame` (`hook.go:496`) filters stack frames whose function name contains
  `SentryWarn` or `SentryError`, so renaming the method makes the logger itself the
  innermost app frame and Sentry reports the logger as the culprit for every event.
  Rung 16 updates that filter.
- **No change to alert routing.** `feature` stays a separate parameter carrying one
  of the ten `deps.Feature` values, so the Sentry ownership rules that map
  `tags.feature:<x>` to an owner keep working untouched. Unlike ManagementConsole,
  which sets `feature` to the identifier tag itself (`report_error.go:149`).
- **No TypeScript output.** Nothing outside umh-core reads these identifiers.
- **No batching.** ManagementConsole's `Details.Batch` debounces per identifier per
  hour. umh-core already has two suppressors: the hook's five-minute fingerprint
  debouncer, and per-instance report gates such as `CPUDeps.reportedReads`. A third
  would make it unknowable which one applied.
- **No `locations` field.** It records frontend versus backend in ManagementConsole.
  umh-core is one binary, so the value would be identical on all ~149 entries.

## Behavioral Contract

### `telemetry.Identifier`

- **Preconditions**: none; it is a value type with no constructor.
- **Postconditions**: a value obtained from the generated tree has a non-empty `Tag`
  matching the format below, a non-empty `Brief`, and a `Severity` that is exactly
  one of the two declared constants.
- **Invariants**: `Tag` and `Severity` are both stable for the life of an entry.
  Changing either re-groups the Sentry issue, because the fingerprint is built from
  level, feature, event name and error types (`hook.go:322`), so an edit to a
  severity starts a new issue and orphans the history under the old one. The YAML says
  so at the top of the file. `Brief` is safe to edit: it is not a fingerprint
  component.

### The generator

- **Preconditions**: `telemetry.yaml` parses as a map of domain to entries, and each
  entry has both `brief` and `severity`.
- **Postconditions**: writes `identifiers.gen.go` containing one `Identifier` per
  entry, reachable through a nested struct tree derived from the `::` segments, the
  repo's Apache 2.0 licence header, and a `DO NOT EDIT` header naming the source
  file. Output is byte-identical for byte-identical input, which requires emitting in
  sorted key order: a YAML mapping decodes into an unordered Go map, so emission
  order has to be imposed rather than inherited. The licence header is emitted by the
  generator because lefthook's `fix-license-header` job would otherwise inject one
  after generation and the regeneration test would fail on the first commit.
- **Segment to Go identifier**: each segment is split on `_` and each part gets an
  upper-case first letter, so `push::persistent_failure` under domain `transport`
  becomes `telemetry.Transport.Push.PersistentFailure`. Generation aborts rather than
  emit something that will not compile, on any of three grounds: a segment that is a
  Go keyword, a segment that starts with a digit once joined, and two entries whose
  Go paths collide. Collisions are not exotic. The tag format admits `push` and
  `push::persistent_failure` in one domain, which need the same field `Push` as both
  an `Identifier` and a struct, and it admits `persistent__failure` beside
  `persistent_failure`, which title-case to the same name.
- **Invariants**: generation reads one file and writes one file. It never consults
  the environment, the clock, or the network, so its output is a pure function of its
  input.
- **Failure**: any malformed entry aborts generation with a message naming the
  offending key. A partial file is never written.

### `FSMLogger.Sentry`

- **Preconditions**: `id` comes from the generated tree. `feature` is one of the ten
  `deps.Feature` constants (`feature.go:62-99`) or a value from
  `FeatureForWorker(workerType)` (`feature.go:106`), which mints one from an
  arbitrary worker-type string and is what most supervisor call sites pass.
  `hierarchyPath` may be empty. `cause` may be nil.
- **Postconditions**: emits exactly one zap entry at the level `id.Severity` maps to,
  with `id.Tag` as the message, and `feature`, `hierarchyPath`, the caller's fields
  and an error attached. When `cause` is nil the error is `errors.New(id.Brief)`, so
  every event carries a readable sentence and no error-level event arrives without an
  exception. Whether it reaches Sentry is then up to the existing `SentryHook`, which
  debounces per fingerprint.
- **Invariants**: never panics, never returns an error, and never emits an event with
  an empty message. Reporting code must not throw into the caller, which is already
  handling a failure.
- **Failure**: an `id` with an empty `Tag` cannot report the event it meant to, so it
  reports the defect instead, under the fixed tag
  `telemetry::unregistered_identifier`, carrying the caller's fields. A `Severity`
  that is neither constant maps to error, never to a level below warn: the hook only
  forwards warn and above (`hook.go:73`), so any other default would silence the
  event and the defect with it.

## Interface Definition

```go
package telemetry

type Severity string

const (
    SeverityWarning Severity = "warning"
    SeverityError   Severity = "error"
)

type Identifier struct {
    Tag      string
    Brief    string
    Severity Severity
}
```

```go
// pkg/fsmv2/deps
type FSMLogger interface {
    // ...
    Sentry(id telemetry.Identifier, feature Feature, hierarchyPath string,
        cause error, fields ...Field)
}
```

YAML entry shape:

```yaml
transport:
  push::persistent_failure:
    brief: Outbound pushes have failed repeatedly; the instance may look offline.
    severity: error
```

Tag format: `^[a-z0-9_]+(::[a-z0-9_]+)+$`. The domain is the first segment; the YAML
key supplies the rest.

## Edge Cases

| # | Input / condition | Expected behaviour |
|---|---|---|
| 1 | Zero-value `Identifier{}` passed to `Sentry` | Emits `telemetry::unregistered_identifier` at error level with the caller's fields. Never an empty message. |
| 2 | `cause` is nil | Normal path. The zap entry carries no error field; the hook's `ExtractErrorFromFields` finds none and sends no exception. |
| 3 | `hierarchyPath` is empty | Field omitted, as `SentryWarn` does today. |
| 4 | YAML entry missing `brief` | Generation fails, naming the key. |
| 5 | YAML entry missing `severity` | Generation fails. No default: a defaulted severity is a silent decision about whether someone gets paged. |
| 6 | Two YAML keys produce the same tag | Generation fails, naming both. |
| 6b | Two distinct tags collide as a Go path (`push` and `push::persistent_failure`; `persistent__failure` and `persistent_failure`) | Generation fails, naming both keys and the colliding Go path. |
| 7 | YAML key with a leading, trailing, or doubled `::` | Generation fails; the tag would not match the format. |
| 8 | YAML key containing a character outside `[a-z0-9_:]` | Generation fails. |
| 9 | Domain with no entries | Generation succeeds and emits no struct for it. An empty domain is a work-in-progress edit, not an error. |
| 10 | `identifiers.gen.go` stale relative to the YAML | The regeneration test fails, naming the make target. |
| 11 | `identifiers.gen.go` hand-edited | Same test fails, because regeneration overwrites the edit. |
| 11b | Generated file committed, then lefthook's `fix-license-header` runs | No diff, because the generator emits the licence header itself. |
| 12 | ~149 entries growing to hundreds | Generated tree is nested structs; compile cost is linear and no lookup happens at runtime. |
| 12b | YAML keys in a different file order, same content | Output unchanged, because emission is sorted by key. |
| 12c | `Severity` neither `warning` nor `error` in the YAML | Generation fails. At runtime, an unrecognised value maps to error rather than below warn, so the event is never silenced. |
| 13 | Two goroutines reporting the same identifier at once | Unchanged from today: the zap logger and the hook's debouncer are already concurrency-safe, and the registry is immutable data. |
| 14 | An event fires during shutdown | Unchanged from today. Callers that must stay silent on a cancelled context, such as `reportFailedReads`, keep their own check. |

## Non-Functional Requirements

- **Performance**: the registry is compile-time data. `Sentry` does one map-free
  struct read plus the zap call it already did, so the change adds nothing to the
  hot path. Generation runs at development time.
- **Memory**: ~149 identifiers of three strings each, in the binary's read-only data.
- **Security**: briefs and tags are authored constants and carry no runtime data. The
  hook's existing sensitive-key denylist continues to govern the caller's fields.
- **Compatibility**: the fingerprint is level, feature, event name and error types
  (`hook.go:322`), and `Tag` supplies the event name, so renaming a tag re-groups its
  issue. The migration renames all ~149, so every saved search and alert rule keyed
  on an old `event_name` needs updating, and every existing issue's history is
  orphaned under its old name. Both consequences belong in the PR description, the
  rename list in full.
- **Behaviour change, declared**: three names are reported today at both warn and
  error level: `collector_restart_failed`, `desired_state_load_failed` and
  `shutdown_request_failed`. One declared severity per entry means one of the two
  levels changes at those call sites. Rung 4 decides which, per name, and the rung
  that converts each one carries a test for the new level rather than being typed as
  a behaviour-preserving refactor.

## Declared Constraints

| Standard | Source | What this spec claims |
|---|---|---|
| comments | `agent-workflows:standards/comments.md` | Every comment in `umh-core/pkg/telemetry`, and every comment this migration adds or edits elsewhere, satisfies every numbered rule in `comments.md`. In particular the `DO NOT EDIT` header on `identifiers.gen.go` is claimed under §12 as a contract with tooling, and the tag-stability note at the top of `telemetry.yaml` under §13 as a constraint the code cannot show. |
| abstractions | `agent-workflows:standards/abstractions.md` | Every numbered rule `§R1`-`§R12` holds for `pkg/telemetry`. Specifically: §R1, the mapping from an event to its severity exists only in `telemetry.yaml`; §R2, `Identifier` is a struct of three strings, so its representable values far exceed the ~149 the generator emits; the spec narrows the reachable set by making the generated tree the only sanctioned source and giving the zero value defined behaviour (edge case 1), rather than claiming the type itself is narrow; §R5, the tag format is enforced by the generator rather than described in prose; §R8, `Severity` is a named type rather than a string. |

**Not claimed:**

- `abstractions.md` §F1-§F5 are the conformance-run framework for a reviewer, not
  rules an artifact satisfies, so there is nothing here to claim against them.
- `comments.md` §6 and §6b rank files by comment ratio to choose what to read. They
  bind a reviewer's reading order, not this spec.

## Verification Strategy

### Provable properties

| # | Property | Method | Priority |
|---|---|---|---|
| 1 | Every YAML entry yields one `Identifier` with a non-empty `Tag` and `Brief` | unit test over the generated registry | critical |
| 2 | Every `Tag` matches the declared format | table test enumerating the whole registry | critical |
| 3 | `identifiers.gen.go` regenerates byte-identically from unchanged YAML | test that runs the generator into a temp dir and compares | critical |
| 4 | A zero-value `Identifier` reports `telemetry::unregistered_identifier` and never an empty message | unit test on the logger | high |
| 5 | Each `Severity` maps to exactly one zap level, and both constants are covered | table test | high |
| 6 | A malformed YAML entry aborts generation and writes no file | unit test per edge case 4-8 | high |
| 7 | Capstone: a real `Sentry` call produces one hook-intercepted event whose `event_name` tag equals `id.Tag` and whose level matches `id.Severity` | integration test reusing the `recordingLogger` + `SentryHook` pattern from `pkg/fsmv2/cpu/cpu_read_report_test.go` | critical |

### Purity boundary

- **Pure core**: the generator's YAML→Go transformation, tag composition and
  validation. Deterministic, input to output, no clock and no network.
- **Effectful shell**: `FSMLogger.Sentry`, which writes a zap entry; the `SentryHook`
  that intercepts it; the Sentry SDK transport. None of this is new.
- **Boundary interface**: `Identifier`, an immutable value. The shell reads it and
  never mutates it, so every property above except 4 and 7 is provable without a
  logger, a hook, or a network.

### Tooling

- Mutation testing: not run, because Standard intensity omits Phase 5.
- Static analysis: `golangci-lint run ./...` and `nilaway`, as the repo configures
  them. Neither is usable on this machine, for different reasons. `golangci-lint`
  2.6.2 reports itself as built with go1.27.0 and does start, but its type-checker
  cannot read this toolchain's export data (`cannot decode "internal/goarch", export
  data version 4 is greater than maximum supported version 2`), so it emits spurious
  `typecheck` errors even on untouched packages: 25 on `pkg/cpuhealth`. `nilaway`
  fails earlier, with `package requires newer Go version go1.27`. CI is the signal
  for both. Per-rung verification is `go build`, `go vet -tags=test` and
  `go test -race -tags=test`.
- Property-based tests: not warranted. Property 2's input space is the finite
  registry, so a table test enumerates it exhaustively rather than sampling it.
- E2E: none. The capstone is an integration test; the feature has no user journey.

### Commit ladder

Each rung is one commit, each demanded by its own failing test, except the rungs
typed `refactor`, which add no test and change no behaviour. Counts are measured on
`staging` by a paren-matching scan of every non-test `SentryWarn`/`SentryError` call:
165 sites, ~149 distinct string literals, 4 messages computed at runtime. PR 2744
adds to the `cpu` count when it lands.

| # | Rung | Type | Sites |
|---|---|---|---|
| 1 | `Identifier` and `Severity` types | feat | n/a |
| 2 | Generator: one domain, one entry, nested tree, sorted emission, licence header | feat | n/a |
| 3 | Generator: rejects malformed entries and Go-path collisions (edge cases 4-8, 6b, 12c) | feat | n/a |
| 4a-4f | `telemetry.yaml`: the ~149 entries, one sub-rung per subsystem | feat | n/a |
| 5 | Regeneration test (property 3, edge cases 10, 11, 11b) | feat | n/a |
| 6 | `Sentry` on `FSMLogger` and every implementor, in one commit | feat | n/a |
| 7 | Unregistered-identifier guard and severity fallback (property 4) | feat | n/a |
| 8 | Convert `pkg/fsmv2/cpu` | refactor | 1 |
| 9 | Convert `pkg/config` | refactor | 12 |
| 10 | Convert `pkg/communicator` | refactor | 22 |
| 11 | Convert `pkg/fsmv2/workers` | refactor | 28 |
| 12 | Convert `supervisor/reconciliation.go` | refactor | 41 |
| 13 | Convert `supervisor/api.go` and `lifecycle.go` | refactor | 26 |
| 14 | Convert `supervisor/internal/{collection,execution,health}` | refactor | 27 |
| 15 | Convert the remaining `pkg/fsmv2` sites and `pkg/cse` | refactor | 8 |
| 16 | Delete `SentryWarn`/`SentryError`, update `IsInternalFrame`, update the tests that assert message strings | refactor | n/a |
| 17 | **Capstone** (property 7) | feat | n/a |

**Rung 4 is the bulk of the work, not a detail.** Authoring ~149 briefs and choosing
~149 severities needs someone who knows each subsystem, so it splits by subsystem
rather than landing as one commit: 4a supervisor top-level (67 sites), 4b
supervisor internals (27), 4c workers (28), 4d communicator (22), 4e config (12), 4f
cpu, cse and the rest (9). Two things inside it need a human rather than a rule. The
eight prose names have to be read to learn what event each one is, among them `cpu:
startup cgroup snapshot failed; quota signals omitted`, whose `:` and `;` the tag
format rejects. And three names are reported today at both warn and error level:
`collector_restart_failed`, `desired_state_load_failed` and
`shutdown_request_failed`, so declaring one severity changes the level at some call
sites. Whichever rung
converts each of those three carries a test for the new level and is typed `feat`,
not `refactor`.

**Rung 6 has to move every implementor at once.** Adding a method to `FSMLogger`
breaks every type that satisfies it the moment it lands, so the same commit updates
`logger_impl.go`, `logger_noop.go`, `pkg/fsmv2/integration/test_logger.go` and the
eight fakes across six test files (`actions_recovery_test.go` holds three of them,
then `edit-protocolconverter_test.go`, `authenticate_test.go`,
`panic_recovery_test.go`, `collector_panic_test.go`,
`collector_logseverity_test.go`). That work cannot wait for rung 16.

**Rung 16 is not only a deletion.** `IsInternalFrame` (`hook.go:496`) filters stack
frames whose function name contains `SentryWarn` or `SentryError`; once those names
are gone, the new method's frame survives filtering and Sentry names the logger as
every event's culprit. The same commit teaches the filter the new name, and updates
the tests that assert old message strings.

The transitional state is deliberate and internal: rungs 6 through 15 have three
reporting methods on the interface, and rung 16 removes two of them. No release sees
that state.
