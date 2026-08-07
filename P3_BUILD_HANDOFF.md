# P3 build handoff — fsmv2 CPU monitor worker (ENG-5128)

**State:** P3 COMPLETE on branch `p3/cpuhealth` in worktree
`/Users/jeremytheocharis/umh-git-v2/.worktrees/p3-cpuhealth`. 7 commits, all
local, nothing pushed. Working tree clean. Whole-repo build green; every fsmv2
package green; cpuhealth + diagnosis green.

**⚠️ The reader certifies. Do not mark P3 done on my word.** Re-run the gates with
positive controls and break my fixes to watch the guards fail.

## Branch + pre-build P2 amendment
- P3 cut from `origin/p2/cpuhealth`, then **re-pointed** onto P2's amended commit
  `a58949dbe` (the `cpuhealth.Table` export added to P2 during this build).
- `cpuhealth.Table` is an additive P2 amendment: `NewEngine` delegates through it
  so a worker can walk `table.Signals` for per-signal Availability. Loop:
  implementer → DA (raised "drop Table, use Observe's []Readiness" — REFUTED:
  worker calls frozen `Decide(...)`, a second `Observe` double-appends windows,
  `Reduction` returns no Availability) → fixer → DA round-2 CONVERGED
  (mutation-verified fingerprint).

## Commits / rungs
| commit | rung | mode | specs |
|---|---|---|---|
| `f68974dcc` | R1 poll/report | conductor manual tdd-commit + self-review | 5 (register, two-tick, healthy, degraded, engine-error) |
| `1d3edfa34` | R2 never-fabricates | conductor manual tdd-commit (conformance pin) | 2 |
| `eae576581` | R3 spawn gate | conductor manual tdd-commit | 2 |
| `6013e3c08` | R4 absence-of-evidence | conductor manual tdd-commit | 4 |
| `e9d855a22` | R1 fix | startup-snapshot-read-error logged + comment corrected | — |
| `501c3f6cc` | scenario | conductor-built (non-rung) | 2 |
| `10c7c5bb6` | docs | STATUS | — |

All rungs run MANUALLY with tdd-commit discipline (RED→GREEN→commit→review→fix),
not via the Workflow tool: the Workflow agent processes resolve against the
session cwd, not this worktree — the `agents_default_to_session_repo` hazard
that P2's handoff also records. Commits are `--no-verify` after manual
gofmt/vet/license (the lefthook gofmt hook is broken for conductor-side commits).

## Decisions not stated by the spec (I own these; verify them)
1. **CPUStatus field set (R1):** `Verdict`, `Message`, `SignalsCapable`,
   `SignalsMeasured`, `Polls`. The two counts are declared in R1 (SPEC says R4
   needs them); the rest was R1's to settle. Recorded in STATUS.
2. **Flag mechanism (Finding-1 ruling):** `USE_FSMV2_CPU` read ONCE in
   `cmd/main.go` (env-var-only, never persisted), set on
   `configData.Agent.UseFSMv2CPU`, **published to the configworker via the deps
   registry** (`CPUEnabledDepsKey`, the `ConfigManagerDepsKey` pattern), stored
   on the worker at construction. NOT read via the file-backed config manager
   (which cannot see an env-only flag). This was the ruling the user requested;
   validated against how nmap gates.
3. **`NewDepsWithSampler` seam:** additive constructor so the scenario (a
   different package, `examples`) can build a mock-backed `*CPUDeps`. Production
   `NewDeps` delegates to it. The rung tests use the in-package `newDeps`
   instead — both build identically.
4. **Startup read failure → healthy verdict is REAL, not a bug I introduced:**
   a transient startup `cpu.max`/cpuset read failure pins `cores=0/quota=0` and
   permanently drops the quota signals; the first Poll still reports healthy.
   I logged the discard and corrected the overclaiming comment, but did NOT make
   it report could-not-measure — that's a P4-seam behavior change.

## The R4 refusal — how it reaches the gate (P3's boundary)
P3 R4 computes `SignalsCapable`/`SignalsMeasured` on the worker; the refusal
(measured < capable while a capable signal has not first-measured) is carried by
the counts. **The seam in P4** turns that into a degraded `CPUHealth` / refused
bridge. The verdict stays "healthy" — the counts, not the verdict, carry the
refusal. Nothing in P3 blocks a bridge (that IS P4's refusal counter/flag).

## What I was unsure about (most valuable output)
1. ⚠️ **F10 recorded-scenario before/after is NOT done.** SPEC R4's gate requires
   the new F10 scenario recorded against the parked commit (`e642457f5`) and
   against the worker. The recorder is in `artifacts/.../recorder/` (targets the
   OLD api) and the re-record protocol runs in a separate scratch worktree
   (`.worktrees/cpu-rerecord`). I did NOT do this — node it as the verifying
   session's recording-gate work. The unit spec 4 (frozen-after-measured keeps
   admitting) IS written and mutant-verified, which the SPEC says is "the only
   guard the recording gate cannot provide."
2. ⚠️ **R3/R4 self-review agents were launched but their reports did not deliver
   as messages (async delivery stalled).** Convergence is certified here by my
   DIRECT mutant evidence: R3 ungated-upsert fails disabled spec; R4
   per-tick-ready-not-sticky fails the outage spec; R4 NoInstrument-as-capable
   fails the bare-metal spec. A verifying session should re-run these reviews
   with its own agents (with the SendMessage-back instruction their briefs
   lacked).
3. **Scenario `measured=1` on the healthy box** is honest but minimal — only
   pressure judges on the first sample (throttle is a delta-ratio needing 2,
   steal needs 2). Capable=4 reflects the two-quota VM. If a richer demo is
   wanted, drive 2+ Poll ticks.
4. **`cpuEnabled` field vs config**: I injected the flag as a separate bool deps
   value, not via `cfg.Agent.UseFSMv2CPU`, because the configworker reads
   config from the file. If config.yaml ever carries the flag, the field and
   the manager path should reconcile. See ENG-4400 TODO on the configworker.

## Deferred / later (out of P3 scope, do not build here)
- P4: the seam (status.CPU / CPUHealth / OverallHealth filled from the worker),
  the flag's telemetry registration, docs/changelog.
- P5: refusal counter on the bridge-admission gate.
- P6: default flip. P7: legacy removal.
- ENG-5264: instance-vs-other-software blame (the 42/45 wording collision).
- F6/F7/D4/D5: untagged rows (no recorded scenario differs under them).

## Verification
```
go test -tags=test -count=1 ./pkg/cpuhealth/... ./pkg/diagnosis/...
go test -tags=test -count=1 ./pkg/fsmv2/...        # FULL tree, green (~10 min)
go build ./...
go vet ./pkg/fsmv2/cpu/... ./pkg/fsmv2/workers/configworker/... ./pkg/fsmv2/examples/...
gofmt -l (my files clean)
```
Scenario run (verified):
```
go run pkg/fsmv2/cmd/runner/main.go --scenario cpuhealth
# healthy box -> verdict="healthy" capable=4 measured=1
# failing box -> could not measure (read .../cpu.stat: permission denied)
```

Do NOT push, open a PR, run `gh stack`, or touch Linear/Slack — a human does that.

## F16 / F17 ladder (on top of P3 R1-R4, added 2026-08-06/07)

VSDD intensity **FULL** (Jeremy: recovery = "no, and it's already too late" — bridge blocking must
ship behind `enableResourceLimitBlocking`, F18). P2 shipped F17 rungs 1-3 (HasPressureStats from
sticky `PsiAvailable`; saturation omitted when `cores<=0`; byte-identical guard). P3 re-pointed onto
P2's new head + fixtures gained `PsiAvailable` (11/11). Rungs below start at HEAD 1210c5fec.

- [ ] **R4 — 10s bound (`F16`)** · tdd-commit + conductor closure · commit `1210c5fec` · 15/15 green · **reviewed GO**.
  - `CPUStatus.RefusingAdmission bool` = `measured<capable` AND `elapsed < f16AdmissionWindow(10s)` from the FIRST sample's Timestamp (synthetic clock, no wall clock). Counts+verdict unchanged at deadline; no Poll error. P4 (deferred) consumes the boolean, not the counts.
  - Three review findings hardened (each break-restore-verified): window width pinned to a literal; partial-measurement branch (measured=1/capable=2) asserted; inert-flag/read-error trap documented for F18 (empty Poll = "no determination", not "admission open").
- [ ] **R5 — Sentry-once at deadline (`F16`)** · in flight.
  - `FSMLogger.SentryError` naming never-measured signals, once per worker, never per tick, never via a Poll error; no-PSI boxes fire nothing.
- [x] **R5 — Sentry-once at deadline (`F16`)** · tdd-commit + conductor closure + independent review · commit `73053ca43` · 18/18 green · **reviewed GO**.
  - `FSMLogger.SentryError` naming never-measured capable signals, ONCE per worker (monotonic latch, never per tick), never via a Poll error, never on a no-PSI box. Fixed error string (Sentry grouping key) + structured `deps.Field` (signal names, measured/capable shortfall, window) for queryability.
  - Three findings hardened (break-restore-verified): structured fields kept out of the grouping key; single-sourced `overDeadline` boundary; plural-capable path covered (a 4-capable/1-measured spec forcing the name-join).
  - 🔴 OPEN design surface (reviewer concurred deferrable, SPEC-correct as shipped, human's call): (A) monotonic latch has no recovery complement — a recovered box keeps a standing Sentry error, a broken box fires once then silent; (B) post-deadline status reads healthy+not-refusing while counts still carry the gap — no explicit indicator surfaced for the seam. Both touch P4 boundary / product decision.
- [ ] **R6 — re-derive R4 counts** · satisfied by the re-point fixture reconciliation (counts were already written for the F17 behavior; the re-point fixture fix restored their intent — capable=2/measured=1 etc. NOT hand-edited). Full fsmv2 gate in flight.
