# PR1→PR2 Cascade Rebase Record (v2 — post-rebase test fix session)

**Date (rebase):** 2026-04-28
**Date (post-rebase fixes):** 2026-05-04
**Working dir:** `/Users/jeremytheocharis/umh-git-v2/cascade-pr2/`
**Branch:** `cascade/pr2-prove-pattern`

## Boundary points

| Point | SHA | Description |
|-------|-----|-------------|
| Old merge-base | `42a265873` | Pre-cascade PR1 base |
| PR1 tip (new base) | `c165fc506` | Post-cascade PR1 (PR1-c7 WorkerBase 3-param + register.NoDeps) |
| Rebase mechanical tip | `a0e636f71` | After rebase + API fixup (original v1 record tip) |
| Post-rebase fix commit | `f9ac813c2` | Test failures introduced by rebase (see §Post-rebase fixes) |
| Artifact cleanup | `264504402` | Typos in .execution/P2.1/build_test.txt + comment-analyzer.md |

v1 record (mechanical rebase only): `.execution/PR1-to-PR2-rebase/rebase_record.md`

## What changed between v1 tip and current tip

The v1 record covered the mechanical rebase through `a0e636f71`. This v2 addendum covers the two post-rebase fix commits.

### Post-rebase fixes (f9ac813c2)

Two independent test failures discovered during the post-rebase verification session.

#### Fix 1 — `deps_registry_test.go`: WorkerBase 3rd type parameter

**Root cause:** PR1-c7 (`974208e30` + `01ae6c05e`) extended `WorkerBase` from 2 to 3 type parameters
(`WorkerBase[TConfig, TStatus, TDeps]`) and changed `register.Worker` factory signature from a 4-arg
closure `(id, logger, sr, deps)` to a 3-arg closure `(id, logger, sr)` with internal `GetDeps[T]()`.
The test file `deps_registry_test.go` was created in a PR2 commit and predated PR1-c7, so it compiled
against the old 2-param shape.

**Files changed:**
- `umh-core/pkg/fsmv2/register/deps_registry_test.go`
  - `fsmv2.WorkerBase[depsRegistryConfig, depsRegistryStatus]` → `fsmv2.WorkerBase[depsRegistryConfig, depsRegistryStatus, register.NoDeps]`
  - Factory closures: 4-param form → 3-param form with `register.GetDeps[depsRegistryDeps]("test-worker")`
  - `register.SetDeps` call moved **before** `register.Worker` registration (the factory closure is
    captured at registration time; deps must be published first so `GetDeps` returns the correct value
    when the factory runs)

#### Fix 2 — `communicator/children.go`: snapshotUserSpec reading from empty ChildrenSpecs

**Root cause:** P2.5 commit `8e3eb53d3` retired `SetChildSpecsFactory` and removed the two
`populateChildrenSpecs()` calls from `WorkerBase.DeriveDesiredState`. This made
`snap.Desired.ChildrenSpecs` permanently empty for all workers using the factory pattern.
`snapshotUserSpec` (in `communicator/children.go`) read the transport child's credentials from
`snap.Desired.ChildrenSpecs[0].UserSpec` — which became empty after P2.5. Result: the transport
child was spawned with a zero-valued `UserSpec`, so it received no `relayURL`/`authToken`, and
authentication never occurred (`AuthCallCount == 0` in all communicator scenario tests).

**Fix:** Rewrote `snapshotUserSpec` to derive the transport's `UserSpec` directly from the
communicator's parsed `CommunicatorConfig` fields via `yaml.Marshal`, bypassing the old factory
path entirely. The `timeout` field is intentionally omitted — the transport's `PostParseHook`
applies a default of 10 s when `Timeout == 0`, avoiding `time.Duration` YAML nanosecond noise.

**Files changed:**
- `umh-core/pkg/fsmv2/workers/communicator/children.go`
  - Added `"gopkg.in/yaml.v3"` import
  - `snapshotUserSpec`: reads `snap.Desired.Config.{RelayURL,InstanceUUID,AuthToken}` instead
    of `snap.Desired.ChildrenSpecs[0].UserSpec`
- `umh-core/pkg/fsmv2/workers/communicator/worker_test.go`
  - `RenderChildren` call in the "valid UserSpec" test: `Desired.Config: desired.Config`
    (was using old ChildrenSpecs approach)
  - `Expect(specs[0].UserSpec.Config).To(Equal(spec.Config))` → three `ContainSubstring` checks
    (re-serialized YAML keys differ from original raw YAML)
- `umh-core/pkg/fsmv2/workers/communicator/worker.go`
  - Removed `SetChildSpecsFactory` call added incorrectly in a prior fix attempt (was no-op
    since `populateChildrenSpecs` is never called post-P2.5)

### Artifact cleanup (264504402)

Two typos in execution artifact files flagged by the post-commit CodeRabbit hook:

| File | Location | Before | After |
|------|----------|--------|-------|
| `.execution/P2.1/build_test.txt` | Line 39 | `mv2/workers/transport/pull/state\t1.224s` (truncated) | `ok  \tgithub.com/…/pkg/fsmv2/workers/transport/pull/state\t1.224s` |
| `.execution/P2.1/build_test.txt` | Lines 44–45 | Duplicate of lines 35–36 | Removed |
| `.self-review/P2.1_20260429_002702/comment-analyzer.md` | Line 69 | `architecture_p1_8_test.go:228-117` | `architecture_p1_8_test.go:117-228` |

## Test status after all fixes

| Package | Result |
|---------|--------|
| `pkg/fsmv2/register/` | PASS (deps_registry_test compilation + runtime) |
| `pkg/fsmv2/workers/communicator/` | PASS (AuthCallCount ≥ 1 in all scenario tests) |
| `pkg/fsmv2/workers/communicator/` unit | PASS (RenderChildren + DeriveDesiredState assertions) |

The communicator scenario tests now complete in ~26–27 s each (1–2 s active + ~25 s graceful
shutdown cascade through 4 supervisor levels).

## Cascade-back flags

**None.** Both fixes are confined to PR2 files. The `snapshotUserSpec` fix is a consequence of
P2.5 retiring the factory pattern — the consumer simply needed updating. The `deps_registry_test.go`
fix is a straightforward rebase conflict that was missed during the mechanical pass because the
file compiled cleanly against an intermediate state.

## Push policy

**Local-only.** No `git push` performed. Implementer handles push after final validation.
