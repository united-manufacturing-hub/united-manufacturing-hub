# PR3 cascade-rebase record

> Cherry-pick rebase from old PR3 base onto new PR2 cleanup tip.
> Method: Option B (cherry-pick fresh) per architect.
> Recovery anchor preserved: `pr3-pre-rebase-20260429-112746`.

## Inputs

- **Old PR3 head** (pre-rebase): `b955952a9` (PR3 C9: Doc sweep)
- **Pre-rebase tag**: `pr3-pre-rebase-20260429-112746`
- **New PR2 tip** (rebase base): `203f99cda` (PR2 boundary cleanup)

## Outputs

- **New PR3 tip**: `d75938c3a` (review-iter1 fixup commit at tip)
- **New PR3 C9 SHA**: `5bfd8788b` (replaces `b955952a9` in fine plan §3 lines 131 + 416)
- **New cascade/pr3-removals branch**: 13 commits on top of `203f99cda` (10 cherry-picked + 2 cascade-back fixups + 1 review-iter1 fixup)

## Review iteration log

| Iter | Verdict | Findings | Resolution SHA |
|---|---|---|---|
| 0 | (initial submission) | 12 commits cherry-picked + 2 cascade-back fixups | `704b22439` |
| 1 | NEEDS-FIX (6 Important, 0 Critical) | I-1 dead test data, I-2 stale test comment, I-3 godoc inaccuracies, I-4 inverted reconciliation comment, I-5 workerType alias dup, I-6 LoadObservedTyped error swallow | `d75938c3a` |

## Cherry-pick list (chronological order applied)

| # | Old SHA | New SHA | Subject | Conflict count |
|---|---|---|---|---|
| 1 | `3da5f5ce6` | `1706eb202` | refactor(fsmv2): delete WrapStatus and WrapStatusAccumulated | 1 (worker_base.go struct field reordering) |
| 2 | `67e2c2547` | `42e252ee2` | PR3 C1: migrate exampleslow to register.Worker[*ExampleslowDependencies] | 6 (state files + snapshot.go) |
| 3 | `8363c9257` | `d7f43086d` | PR3 C2: migrate examplechild to register.Worker[*ExamplechildDependencies] | 8 (state files + snapshot files + integration test) |
| 4 | `e5ee3ddc9` | `962bd11ca` | PR3 C3: migrate exampleparent to register.Worker[*ParentDependencies] | 9 + structural rewrite (Option A) |
| 5 | `9603408f7` | `795eff736` | PR3 C4: migrate examplefailing to register.Worker[*FailingDependencies] | 8 (state files + snapshot files) |
| 6 | `aa074e2f4` | `2ba9dca54` | PR3 C5: delete examplepanic worker | 6 (modify/delete; delete won) |
| 7 | `c124573b1` | `2b8864f54` | PR3 C6: Delete WrapAction[TDeps] framework compat seam | 0 (auto-merged) |
| 8 | `aa0876521` | `4bc2b4855` | PR3 C7: Delete factory.RegisterWorkerType | 0 (auto-merged) |
| 9 | `b42b06b09` | `8fa0afbfd` | PR3 C8: Delete RegisterWorkerAndSupervisorFactoryByType + extraDeps | 0 (auto-merged) |
| 10 | `b955952a9` | `5bfd8788b` | PR3 C9: Doc sweep — re-anchor godoc/READMEs/validator on post-deletion API | 0 (auto-merged) |
| fixup | (n/a) | `9ced5ec2d` | fixup(examplechild/state): migrate makeChildSnapshot to new lean types | 0 (post-test fix) |
| fixup | (n/a) | `704b22439` | fixup(validator): drop examplepanic from isChildWorker test fixture | 0 (cascade-back) |
| review-iter1 | (n/a) | `d75938c3a` | fixup(rebase-review-iter1): address 6 reviewer findings | 0 (post-review) |

**Total**: 10 commits cherry-picked (38 conflicts resolved across 36 files) + 2 cascade-back fixups + 1 review-iter1 fixup commit.

## Conflict categories

### Mechanical (most common)

- **Type rename**: `IsStopRequired()` → `ShouldStop()` (CHANGE-1 / P1.5b rename); applied to 4 child workers' state files
- **Status accessor**: `snap.Observed.<field>` → `snap.Status.<field>` (Observation[TStatus] doesn't expose Status fields; use deprecated flat aliases on WorkerSnapshot for shutdown/parent state)
- **Transition arity**: 4-arg → 5-arg (`nil` for children); applied to all Transition() call sites in PR3 commits
- **Fat snapshot deletion**: snapshot.go files deleted `ExampleXObservedState` / `ExampleXDesiredState` / `ExampleXSnapshot` types in favor of lean `ExampleXConfig` + `ExampleXStatus` lives at the leaf snapshot package
- **Old-fat-type test deletion**: snapshot_test.go files deleted tests on `ObservedState.ShouldStop()` (the type was deleted)

### Modify/delete

- **C5 (delete examplepanic)**: 6 modify/delete conflicts where PR2 P2.6's F1⊕F7 migration touched examplepanic state files that PR3 C5 deletes wholesale. Delete won (consistent with PR3 C5 intent).

### Structural (architect Option A applied to C3 only)

- **C3 (exampleparent)**: PR3 C3's `worker.go` called `w.SetChildSpecsFactory(buildChildSpecs)` but PR2 P2.5 deleted that method. Architect chose Option A: drop the call + buildChildSpecs function entirely, migrate `RenderChildren` to take `WorkerSnapshot[ExampleparentConfig, ExampleparentStatus]` (replaces the old `*ParentUserSpec` signature). Children flow exclusively via state.Next NextResult.Children → supervisor.
- **State-package mirror**: previously exempt from Test 14 (returned principled-nil); flipped to byte-equivalent — `state/render_children.go` now mirrors children.go body byte-for-byte. Test 14 registry updated: `migrationWindow: true` → removed (defaults to false / strict byte-equivalence enforced).
- **Test #13 layer 2 fixture**: `architecture_p1_8_test.go` updated — exampleparent's render fn now uses `WorkerSnapshot[ExampleparentConfig, ExampleparentStatus]` instead of `*ParentUserSpec`. Added `exampleparentsnapshot` import.
- **worker_test.go**: `DeriveDesiredState` assertions migrated to `*WrappedDesiredState[ExampleparentConfig]` cast; children-count test moved to call `RenderChildren` directly (children no longer come from DDS path).
- **Drop dead code in worker.go**: `buildChildSpecs` function, `defaultChildConfig` const, `DeriveDesiredState` method (option-a custom override), unused `fmt` and `config` imports.

### Cascade-back fixes folded as fixup commits

- **fixup(examplechild/state) `9ced5ec2d`**: PR2 P2.6 added `state_stopped_test.go` (and `state_disconnected_test.go` as consumer of its helper) using legacy `ExamplechildObservedState` / `ExamplechildDesiredState` types. PR3 C2's migration deleted those types but PR3 C2 was authored against an older PR2 base that didn't yet have these test files, so the update was missed. Mechanical fold: `makeChildSnapshot` now constructs `Observation[ExamplechildStatus]` + `*WrappedDesiredState[ExamplechildConfig]`. Belongs in PR3 C2; recommend folding into C2 via interactive rebase before merge.

- **fixup(validator) `704b22439`**: PR3 C5 deletes the examplepanic worker, but `internal/validator/is_child_worker_test.go` still listed its directory path in the positive-cases array (a regression guard for the F9⊕F1 isChildWorker heuristic). Test was failing post-rebase because the directory no longer exists. Mechanical fix: drop examplepanic from the fixture; remaining 5 child workers (examplechild, examplefailing, exampleslow, transport/push, transport/pull) keep the guard active. Belongs in PR3 C5; recommend folding into C5 via interactive rebase before merge.

## Validation

- ✅ `go build ./pkg/fsmv2/...` clean post-C3 (intermediate check)
- ✅ `go build ./pkg/fsmv2/...` clean post-rebase (final)
- ✅ `go test ./pkg/fsmv2/... -short -count=1` PASS for all packages our changes touched (after both fixup commits)
- ⚠ Pre-existing failures (NOT caused by our rebase, verified on cascade/pr2-prove-pattern at `203f99cda`):
  - `pkg/fsmv2/examples` (Communicator Scenario timing-sensitive integration tests fail with "Expected 0 to be >= 1" in 22-27s windows). Failure pattern reproduces on PR2 tip — pre-existing flake/breakage.
- ✅ Recovery anchor `pr3-pre-rebase-20260429-112746` preserved
- ✅ No `<<<<<<<` / `>>>>>>>` markers anywhere in the tree
- ✅ No references to deleted types (`ExampleXObservedState` / `ExampleXDesiredState`) in code (1 stale comment in `supervisor/reconciliation.go:857` — boundary cleanup)
- ✅ Test #14 mirror byte-equivalence: exampleparent flipped from migration-window-exempt to byte-equivalent (passes)
- ✅ `SetChildSpecsFactory` references: 0
- ✅ `buildChildSpecs` references: 0
- ✅ `TestIsChildWorkerDetectsKnownChildWorkers` passes (validator F9⊕F1 guard still active over 5 remaining child workers)

## Notes for architect / boundary review

1. **C3 Option A applied** — see "Structural" section above. Architect approved this re-architecture in DM thread (see `c3_conflict_report.md`).
2. **pr2_issues #5 status**: Architect's brief noted this resolves the P2.2 deferred decision (option-b "refactor exampleparent to canonical WorkerSnapshot" was blocked at type level; PR3 C3 introduces ExampleparentConfig/Status types so the deferral is unblocked). Update pr2_issues #5 to RESOLVED at SHA `962bd11ca` (PR3 C3).
3. **Stale comment in reconciliation.go:857** — references `ExampleparentDesiredState` which no longer exists. One-line cleanup at PR3 boundary.
4. **Stale Skip messages in inheritance_scenario_test.go** (lines 292 + 445) — string literals reference `extractUserSpec` which PR3 C2 deleted. Boundary cleanup.
5. **Fixup commit `9ced5ec2d`** — should be folded into PR3 C2 (`d7f43086d`) via interactive rebase before merge. The cherry-pick boundary makes mid-stack amend complex; appended as fixup with clear linking message.
6. **Architect requested reviewer dispatch** — risk: HIGH (multi-file structural migration in C3 + dead-code deletion + cascade-back fixup). Recommend reviewer focus on C3 cumulative diff (canonical-vs-mirror byte-equivalence + RenderChildren signature change + test fixture migration).

## Pre-rebase vs post-rebase diff scope

```
git diff 203f99cda..9ced5ec2d --stat | tail -1
```

Will include all 10 PR3 commits + fixup; verify against `git diff 203f99cda..b955952a9 --stat` (old PR3 over old PR2 — for sanity). New PR3 should be smaller (no stale-duplicate "old PR2 work" the recon flagged).
