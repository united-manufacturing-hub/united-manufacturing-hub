# STATUS — P1 pkg/diagnosis (ENG-5128 S1)

Branch: `p1/diagnosis` (cut from `fsmv2-simple-newdeps` @ 47062a084).
One entry per rung. Conductor = main session; rungs built via `tdd-commit` workflow + setup-seed subagents.

## S1 — pkg/diagnosis

- [x] **R1 — Reading** · tdd-commit · commit `be27222` · 2 specs green.
  - Mode: setup subagent seeded `reading.go` compile-stubs; `tdd-commit` (run `wf_654e2d6b`) wrote RED+GREEN+commit; self-review did NOT converge (6 remaining findings) — conductor triaged: all 6 dismissed/deferred (3 contradict SPEC §0/skeleton, 1 belongs to R2, 2 style). One self-caught deviation (spec's two It texts folded into one) — fixed by splitting test, amended.
  - Decisions not stated by spec: none.
  - Unsure: none.
- [x] **R2 — Window** · tdd-commit · commit `67d488698` (amended) · 16 specs green (14 window + 2 reading).
  - Mode: 3 setup-seed subagents (reading/window/reduction compile-stubs) then `tdd-commit` (run `wf_fcb60308`). Workflow committed but did NOT converge review.
  - Conductor closure (workflow fixer couldn't converge pre-interruption; applied + verified by running):
    1. **CRITICAL + hard gate blocker: `betteralign` failed** (Window struct field order, 96→56 pointer bytes). Fixed via `betteralign -apply` → clean.
    2. **Real defect: GREEN gutted the shared reduction surface.** Restored to skeleton contract: `Point.Against`, `Reduction.ordered+against+fold`, and all SIX reductions (Slope/P95/P99 had been deleted). Needed by R3 fold, R4 Point.Against, R8 ordered. My testIntent explicitly said keep reduction.go's surface; GREEN violated it.
    3. TWO mutation-survivor test gaps positive-controlled (break impl → guard fails → restore): counter restart strict `<` (equal value keeps) ✓; demote strict `>` (exact boundary holds) ✓.
    4. Commit message overclaim (#10) corrected on amend.
  - Review-triage: 12 findings → 2 FIXED (#0 betteralign, #6/#7 tests), 1 FIXED (#10 message), 7 DISMISSED (contradict skeleton/SPEC §0: white-box test package, Get tuple, Reduction exported, nicknames 'the fold'/'the latch' = spec vocabulary), 9 DEFERRED to R8 (Tick-token: Age-before-Reduce/Coverage not enforced; Type split on window; monotonic-at precondition) / R4 (denominator reset, "either counter") / R3 (fold).
  - ⚠️ OPEN (transferred to verifying session, NOT silently dropped): **freeze is one tick late under §2.5's Age-then-Append loop order** — R2 tests call Append-then-Age, which masks that under the real loop the first failed tick prunes before the freeze engages (deferred finding). The engine loop (R7b) and Tick-token (R8) address it. Judge at review.
  - Unsure: the R2/R3 boundary (State logic in R2 vs fold NUMBER in R3) was my conductor call — Reduce State is window logic, `v` is R3's. Verify. 
- [x] **R3 — Reduction** · tdd-commit · commit `a72bf0b28` (amended) · 25 specs green.
  - Mode: `tdd-commit` (run `wf_063149b0`). Run had subagent errors (2 agents errored mid-response, 3 empty) — review did NOT converge; verified each finding against ground truth.
  - Conductor closure (applied + verified by running):
    1. **#0 (unanimous): `NewReduction` promised min<1 + nil-fold refusal but validated neither** → Reduce derefs `w.red.fold` unconditionally → live panic on a nil-fold window. Fixed: NewReduction now validates (min<1→err, nil-fold→err) matching its doc + SPEC's "checked twice" narrative; Reduce added a nil-fold backstop (→StateUntrusted 0, R8 still refuses at NewEngine). Added refusal spec, mutation positive-controlled (removing nil-check → spec fails).
    2. **Surface regression caught: R3 GREEN deleted `Reduction.ordered`** (R8 spec 5 refuses a percentile on a boolean series via `ordered`). Restored field + `ordered:true` on P95/P99 (skeleton contract).
    3. **#1 minimality: O V E R R I D E.** Reviewers wanted the NaN/Inf guard cut, `<=0`→`==0`, and the 8th It removed. Kept all three: (a) `<=0` is MORE correct pre-R4 (Append detects only numerator resets in R3, so a negative denominator delta is R3-reachable and `==0` would publish a wrong negative ratio); (b) cutting the 8th It while keeping the guards leaves them untested (the defensive-additions-worse-than-nothing anti-pattern). Documented override.
  - #2 (monotonic-At) → DEFER R8 (Tick-token). #3 (denominator-reset interior) → DEFER R4 (spec 3). `reduced_access_test.go` kept (SPEC §0 mandates it).
  - Unsure: Slope over a below-min (1-point) window yields div-by-zero → the NaN guard was load-bearing for (0,Untrusted) vs (Inf,Untrusted); kept on that basis.
- [x] **R4 — Counter pairs** · tdd-commit · commit `c103863a8` (amended) · 28 specs green.
  - Mode: `tdd-commit` (run `wf_37f110ad`). Review not converged; findings were test-gaps (reviewer self-corrected its own "critical": wipe is a design-stage risk, no live consumer).
  - Conductor closure (test additions to pin reintroduction sites):
    1. Negative control — NON-counter DeltaRatio window whose denominator falls is KEPT (does not wipe). Pins `w.counter` gate on the denominator arm. **Mutation positive-controlled** (removing `w.counter` → spec fails → restored).
    2. Both-counters-fall same-tick (full cgroup death): wipes once, re-accumulates to StateValue (0.5).
    3. eqDenom extended: equal denominator keeps both edges (no restart) but Reduce voids to StateUntrusted (zero delta) — documents the two-gate layering.
  - KEPT vs minimality: the `aok/paok` presence-guards (called "unreachable") kept as documented defense-in-depth — removing risks a latent ordering bug if the Append gate reorders.
  - Process note (#4): RED not separately committed — inherent to tdd-commit's one-commit-per-rung design; noted for handoff.
- [x] **R5 — The latch** · tdd-commit (specs 1-5,7,8) + spec 6 by hand · [F4, F5, F7] · 46 specs green.
  - Commits: surface+spec6 `3516734bc` (by hand) · behavior `8118795fd` (amended).
  - **Spec 6 by hand** (latch_spec6_test.go): closes F1's reintroduction BY SIGNATURE — Coverage exactly 2 Duration fields, Update func(Reduced,Coverage,Marks,time.Time). Mutation positive-controlled (adding a bool to Coverage fails it).
  - Mode: seeded latch.go (exact 4-method surface) then `tdd-commit` (run `wf_98e25671`) for specs 1-5,7,8. Brief said "specs 1-5,7"; I also routed spec 8 (ReleaseAfter demote clock) through tdd-commit since SPEC lists 8 behavior specs and labels only spec 6 by-hand — noted.
  - **Critical conductor fix: R5 GREEN removed Fired.Value + Fired.Since** — R6's Severity() reads Value. Restored both per skeleton, wired Update (stamp on fire transition) + Fired(). Added pinning spec, mutation positive-controlled (a Fired that drops Value fails it).
  - **Signature note (write-up, not a §5 departure):** kept `Reset(now time.Time)` vs skeleton's `Reset()` — re-fire-after-reset (spec 5) measures from the injected reset time; a no-args Reset() cannot. R7b has `at` to pass. Flagged for verifying session.
  - KEPT: inert defensive guards, single-goroutine concurrency doc (deferred to wiring); noted Polarity bare-int implicit-else as latent hardening.
- [x] **R6 — Ranking** · tdd-commit · commit `1a5b328b6` (amended) · 55 specs green.
  - Mode: `tdd-commit` (run `whbxx4mzn`). Review not converged — CRITICAL finding.
  - **CRITICAL conductor fix:** the workflow's Rank compared only Tier/External/Index and NEVER called Severity(), and its spec-2 red test was REWRITTEN from "rank within a tier by severity" to "by declared table position" with fixtures where severity coincides with Index — the severity branch was never exercised. SPEC §9 R6 spec 2 mandates the severity level. Restored `Tier → Severity() desc → External → Index`, restored the SPEC test text, and added a severity-vs-Index DISAGREEMENT fixture (higher-severity cause at higher Index must rank first). **Mutation positive-controlled** — removing the severity comparison fails the spec.
  - Fixed a mangled commit message.
  - Kept clamp01 NaN→0 (degenerate marks, capacity==fire); marks validation is R8's. #1/#2 (severity denominator invariants, falling-arm capacity doc) noted as latent.
- [x] **R7 — Instrument + Environment** · by hand (subagent + review) · commit `31bcf0525` · 63 specs green.
  - Mode: implement-subagent + separate verifier-subagent (verifier: all 8 checks PASS — skeleton conformance, four specs genuinely exercised incl. spec 4's two-same-capability guard, domain gate, tests register at 63).
  - Capability gate (Signal.Capable) returns EVERY satisfied instrument in table order, not the first (D-04); readiness gate deferred to Engine.Select (R7b). NewEnvironment is the only Environment constructor. Instrument.Read applies both extractors off one snapshot.
  - **Cross-cutting gate fix** (separate commit `89a47c4dd`): the skeleton's CPU/steal example doc comments were being copied verbatim across rungs — stop condition 5's vocabulary gate (recursive, includes comments + _test.go) fails on them. The skeleton explicitly says "transcribe shapes, rewrite examples." Rewrote window.go/reading_test.go/latch_test.go wording + a 'cpu'->'sig' test fixture. Domain-word sweep now CLEAN. Caught via the R7 implement agent surfacing its own "copy verbatim" choice (which was wrong).
- [x] **R7b — Engine runs the tick** · tdd-commit · commit `fc144744c` (amended) · 70 specs green.
  - Mode: `tdd-commit` (run `wukd2fb4f`) + conductor closure. The workflow's one-red-test drive (SPEC 2) under-covered the rung; 10 findings were mostly minimality (wanted LESS) but the rung required MORE.
  - **Conductor closure (the engine is the core; must be complete):**
    1. **RESTORED the track machinery** the GREEN's minimality stripped — Track type, Table.Tracks, Engine.tracks/tracked, NewEngine track windows, Observe track fold, Track readback (R7b spec 7 was entirely missing).
    2. **ADDED tests** for previously-uncovered rungs: spec 7 (tracks), spec 9 (3-signal readiness — D-18: quiet Ready contributes nothing, incapable has a NoInstrument row that EXISTS), spec 6 (release-on-absent resets on AllAbsent, two signals), spec 3 (NoInstrument demote-clock release). Fixed a test that asserted the readiness slice instead of the fired set (engine was correct).
    3. Fix #7: consistent latch deref (dropped dead nil-guard).
  - 70 specs = 63 + 7 R7b (was 3, added 4).
  - Notes → R8: duplicate signal/instrument names (#4), nil Extract (#5) are construction refusals R8 must add. Mixed-Marks arms (#6) is F7's design. Single-goroutine ownership documented on Engine (#8). Flame GoDoc register (#9) left (design rationale).
- [x] **R8 — Unconstructable bad states** · by hand (subagent + review) · commit `07a5c4d90` · 85 specs green.
  - Mode: implement-subagent + verifier-subagent (9/9 checks PASS → "ready to commit").
  - NewEngine's validate() is the one choke point. Refuses: non-positive span, mark pair clear-not-on-holding-side (both polarities, via worse()), ordered-on-boolean, min<1 (re-checked on all six package values — "checked twice"), dividing-with-nil-Against, track-dividing, nil Extract (instrument+track), duplicate signal names, duplicate instrument names. Converse (non-nil Against under non-dividing) NOT refused.
  - **R8 also closed R7b's deferred refusals** (duplicate names #4, nil Extract #5) — they were R8's unconstructable-bad-states job after all.
  - The R9 span-at-interval cadence refusal deferred to R9 (R9 spec 3).
- [x] **R9 — Table + suite generator** · by hand (subagent + review) · commit `d62fbc385` · 91 specs green.
  - Mode: implement-subagent + verifier-subagent (10/10 PASS → "ready to commit").
  - Suite emits 6×len(Signals), never tracks; Run drives OBSERVE (production loop), one Engine per scenario, reports last-tick availability. The 6-case expected availabilities matched EXACTLY (no row moved). F1-mutant feed (Unreadable→Known(0)) regresses Brief/Long/PostOutageDip to Ready, proving the generator catches a row that skips the readability path.
  - NewEngine gained the span-at-interval cadence refusal (R9 spec 3, applied to tracks too).

## P1 GATE — PASS (per D-06, the conductor's build-side run)
- Command 0 (BASE=fsmv2-simple-newdeps): **12** commits · negative control (BASE=origin/staging): **31** (base correct — staging is the ancestor).
- Command 1 (every path under pkg/diagnosis or STATUS.md): **EMPTY**. Command 2 (nothing imports it): **EMPTY**. Command 3 (prefix control): 24 files match.
- vet/build/gofmt/betteralign clean · no focused specs · domain-word gate clean · **91 specs green**.
- 12 local commits on `p1/diagnosis`, nothing pushed, no PR.
- Handoff: `artifacts/2026-07-30_cpu-complexity-refactor-basis/P1_BUILD_HANDOFF_2026-08-04.md`.
- **P1 NOT done — separate verifier session runs every gate with positive controls and breaks fixes.**

## Post-open review (abstraction reviewer + self-review), 2026-08-04
- Opened **draft PR #2679** (base `fsmv2-simple-newdeps`, assigned Aaron99B). gh stack SUBMIT blocked repo-wide ("Stacked PRs not enabled") — confirmed empirically; manual stacking.
- **Abstraction review** (read all 25 files vs skeleton) raised 2 CRITICAL skeleton-contract violations:
  1. `NewWindow` returned `*Window`; skeleton is `(*Window, error)` (refuses zero/negative span). → CONFORMED.
  2. `Latch.Reset(now)`; skeleton is `Reset()`. → CONFORMED to no-arg; re-fire bar after reset re-anchored at lastUpdate; epoch-zero sentinel in the fire arm fixed. Positive-controlled.
  - Committed as `8ab5b86f9` (14 commits from base) and pushed; CI running.
  - Importants: AllAbsent `else ReleaseAfter` kept and acknowledged as a deliberate departure (time-bounds a hold the skeleton left unbounded, serving §8's "no latch outlives its evidence") · `Case` closed-set (3 hand-maintained sites, G7) noted · span-at-interval arithmetic duplicated (suggestion).
  - Self-review workflow still running; results to be consolidated.
- [ ] R3 — Reduction · tdd-commit · [F3, F4, F5]
- [ ] R4 — Counter pairs · tdd-commit
- [ ] R5 — Latch · tdd-commit (specs 1-5,7) + spec 6 by hand · [F4, F5, F7]
- [ ] R6 — Ranking · tdd-commit
- [ ] R7 — Instrument + Environment · by hand
- [ ] R7b — Engine · tdd-commit · [F5, F7]
- [ ] R8 — Unconstructable bad states · by hand
- [ ] R9 — Table + suite generator · by hand

---

# P2 — pkg/cpuhealth (ENG-5128, S2+S3+S4)

Branch: `p2/cpuhealth` (cut from `origin/p1/diagnosis` @ 9454a651). Built by the P2 conductor
session overnight 2026-08-04 → 08-05. P2-build is P1's verify: any P1 bug found here is fixed back
into PR #2679. One continuous build (no section checkpoints, per Jeremy).

## S2 — measurement

- [x] **S2 R1 — Capacity** · tdd-commit · commit `af814c88a` · 1 spec green.
  - Three cpu.max outcomes (positive limit / literal "max" = present 0.0 no-limit / unreadable +
    unparsable = absent no-signal); non-positive limit never a positive capacity/denominator.
  - **Conductor conformance fix (post-workflow):** GREEN built a LOCAL `Reading` type instead of
    importing `pkg/diagnosis`; skeleton declares `Quota diagnosis.Reading`. Fixed to
    `diagnosis.Reading`/`diagnosis.Known`, amended `--no-verify`.
  - Review triage (self-review 'commit' profile, 2 rounds, NOT converged): #1 dead-`Read`-error —
    REJECT for this rung (skeleton: error is the whole-sample cpu.stat failure; cpu.max is
    best-effort; becomes live at S2 R3) · #2 capacity conflation `0 <period>` vs uncapped — shape
    fix would violate skeleton; edge recorded, no recorded scenario reaches it · #3 one-It-block
    test-masking — LEGIT nit, noted · #4 period-guard minimality — deferred · #5-6 unrelated
    prc-benthos-monitor artifact findings — NOISE, ignored.
  - **Cost-control:** switched self-review to `reviewProfile:'minimal'` after this rung (the 'commit'
    profile cost ~4.87M tokens / 50 agents for one rung).
  - Tooling: lefthook pre-commit gofmt hook is broken for conductor-side amends in this worktree
    (`root: umh-core` + repo-relative staged path). Verified gofmt/vet/license manually; used
    `--no-verify`. Workflow's own COMMIT agent commits pass the hook — do not bypass those.
- [x] **S2 R2 — PSI** · tdd-commit (minimal review) · commit `098a536cc` · 5 specs green.
  - `Sample.Pressure diagnosis.Reading` + `Sample.PsiAvailable bool` (sticky); readPSI parses the
    "some" line's avg60 `/100` → present 0..1; never-readable → absent + PsiAvailable false.
  - Cost-control effective: `reviewProfile:'minimal'` cut the cell to ~1.39M tokens / 14 agents (vs
    ~4.87M / 50 for 'commit'). 4 findings after 2 rounds, all triaged:
    - #3 "'seen' = parsed avg60, not file-readable" — RESOLVED by parked code (sampler.go:210 sets
      PsiAvailable only on parse success); rebuild matches. Documented.
    - #2 minimality (bundled 2nd spec; unforced terminal fallback) — ACCEPT (SPEC S2 R2 is one rung
      with both assertions); fallback is defensive.
    - #1 + #4 (no test for literal-zero avg60 → present 0.0; no test for never-readable-from-start) —
      FIXED: conductor added the two complement controls (test-only), suite now 5 specs. Both pin
      D4-reintroduction corners (a `frac <= 0` guard would silently break every healthy cgroup's D4
      pair).
  - Production landed in `capacity.go` (accumulating sampler) rather than a new file — noted; the
    sampler consolidates at S2 R5 (snapshot) if needed.
- [x] **S2 R3 — Throttle counters** · tdd-commit (minimal) · commit `4f897e87` · review CONVERGED (1 round).
  - `Sample.NrPeriods`/`NrThrottled diagnosis.Reading`; `parseCounter` reads one key from cpu.stat;
    `readThrottle` reads cpu.stat once per snapshot (same bytes as usage). D5: absent OR unparsable
    counter → Unknown (never a trusted 0). cpu.stat read failure → whole-sample error — **Read's error
    return becomes live for the first time** (resolving S2 R1's deferred finding #1, which was correct
    to defer to here). Conformance confirmed: diagnosis.Reading only, no local type.
  - 866K tokens / 10 agents (transient final-da server error, panel still converged).
- [x] **S2 R3b — Usage as a rate** · tdd-commit (minimal) · commit `c8277199` · review CONVERGED (1 round).
  - `Sample.UsageCores diagnosis.Reading` (instantaneous rate) + `Sample.UsageUsec diagnosis.Reading`
    (raw counter kept beside the rate). Stateful sampler holds previous usage_usec edge; first read
    and counter-fall → Unknown (never a fabricated Known(0)); rate = delta_usec/elapsed_usec.
    Conformance: diagnosis.Reading only.
  - **DEFERRED (recorded, not fixed): injectable time source wasn't built** — `elapsed` is
    self-referential (test derives it from the snapshots' own time.Now timestamps), so the rate is
    never pinned to an ABSOLUTE value; a cross-cadence rate bug could escape. Verified residual is
    bounded (delta + 1e6 divisor are independent literals → rate arithmetic IS caught). Per bounded
    convergence, deferral accepted; candidate for a later hardening rung / absolute-rate pin.
- [x] **S2 R4 — Host signals** · tdd-commit (minimal) · commit `e4a93581` · review CONVERGED (2 rounds).
  - `Sample.HostBusy` + `Sample.Steal` (diagnosis.Reading) off the first `/proc/stat` aggregate `cpu `
    line; USER_HZ=100; busy excludes iowait/steal/guest/guest_nice; steal denominator sums 0..7 only;
    Steal is a per-read Reading (F1 readability half); first read = baseline (both Unknown). 1.33M
    tokens.
  - DEFERRED (debt-note): `readHost ok=false` branches (unreadable/malformed /proc/stat) unexercised
    by this red test — safe error paths, noted for a later F1-readability rung. Accepted.
- [x] **S2 R4b — CPU scope** · tdd-commit (minimal) · commit `fe9a1fb90` (amended) · 10 specs green.
  - `Sample.CpuScope Scope` (ScopeUnknown/Host/Affinity) + `Sample.HostCpus`; Scope via cpuset
    (allowed set) vs /proc/stat per-CPU line count (machine count); unreadable machine count OR
    unreadable cpuset → ScopeUnknown, never silent Host (F6).
  - **Conductor fixes (review did NOT converge; F6-core rung):**
    1. REAL DEFECT: `readCpuset` only parsed a single "lo-hi" range — the comma-separated /
       non-contiguous shapes Kubernetes static CPU manager emits for pinned pods ("0,2,4",
       "0-1,4-5") collapsed to ScopeUnknown and lost F6's main signal. FIXED (split on ','),
       + comma-list fixtures (0,2→Affinity; 0,1,2,3→Host).
    2. TEST-GAP: known machine count + unreadable cpuset → a regression defaulting to ScopeHost
       would pass the suite. FIXED (+ never-silent-host case, HostCpus=4.0).
  - DEFERRED: #3 sticky-cache CpuScope/HostCpus like psiAvailable (perf/hardening) · #4 comparison
    source (cpuset vs LogicalCpus/runtime.NumCPU) deviates from parked design — **RECORDED as a
    SPEC-sign-off item at the S3 R5 boundary**, not resolved unilaterally (a genuine design fork;
    do not let S3 R5 blindly consume CpuScope without it).
- [x] **S2 R4c — Virtualisation (incl. ARM64)** · tdd-commit (minimal) · commit `0a14b54b2` (amended).
  - `Sample.Virtualized bool` cached sticky; x86 flags-line "hypervisor"; ARM64 (no flags line) uses
    the DMI product_name fallback; unreadable cpuinfo → false; transient DMI read retried (does NOT
    permanently cache false).
  - **Conductor fixes (review NOT converged):** #2 sticky-false-cached DMI branch unexercised — FIXED
    (+ a bare-metal whose DMI succeeds with a non-matching name stays false across reads, cpuinfo
    read once) · #3 hyper-v token asserted-but-untested — FIXED (added to vendor loop).
  - **DEFERRED to R4d [F9]:** DMI product_name token set omits cloud VMs (AWS Graviton "Amazon EC2",
    GCP "Google Compute Engine", QEMU "Standard PC") — inherited from parked, not a regression; F9's
    SPEC trap is that the fix is a SECOND DMI source (sys_vendor), not a longer list. Folded into
    R4d's dispatch.
- [x] **S2 R4d — DMI vendor source (F9)** · tdd-commit (minimal) · commit `436816c8a` (amended) · 15 specs.
  - Added a SECOND DMI source `sys_vendor` (independent read/cache/retry) so cloud ARM64 VMs
    (Graviton "Amazon EC2", GCP, QEMU) resolve Virtualized when product_name can't — per F9, a new
    source, NOT a longer product-name list.
  - **Conductor fixes (review NOT converged):** dropped token `microsoft` — sys_vendor "Microsoft
    Corporation" is ambiguous (Azure ARM vs bare-metal Surface); bare-metal false-positive is the
    worse error, so Azure ARM is a documented known-limitation. Added `qemu` (real hypervisor, never
    bare-metal). + Surface regression test (must stay false) and QEMU vendor case.
  - The flags-line-guard minimality finding was correctly triaged to DISCARD (a pre-existing test
    forces it — verified by running).
- [x] **S2 R5 — One snapshot per tick** · tdd-commit (minimal) · commit `df7348983` (amended) · 16 cpuhealth + 106 diagnosis specs.
  - Owns `Read` (I/O boundary): one Timestamp per snapshot; all Readings via diagnosis Known/Unknown
    (tested from package cpuhealth_test OUTSIDE pkg/diagnosis — pins S1 exports); `DeriveEnvironment`
    = Virtualized + positive-Quota → HasVirtualization/HasLimit (Known(0) uncapped does NOT satisfy
    HasLimit — positivity, not presence); cpu.stat read/parse failure → whole-sample error, any other
    source → that field Unknown.
  - **Conductor fixes (review NOT converged):**
    1. 🔥 **Confirmed internal SPEC contradiction reconciled:** SPEC R3 spec-2 ("unavailable when
       absent OR fails to parse") contradicted R5 spec 4 ("fail whole snapshot when cpu.stat
       unparsable"). R5 (boundary rung, later) is authoritative: absent key → unavailable Reading;
       PRESENT-but-unparsable cpu.stat → whole-sample error. Left unresolved, the R3 conformance
       gate would flag correct code and a future editor could reintroduce corrupt-cpu.stat-as-
       no-signal. Updated artifacts/SPEC.md R3 bullet + note; flagged for verifier confirm.
    2. + corrupt `nr_periods`/`nr_throttled` whole-sample-failure pins (nr_throttled branch was
       untested).
  - ACCEPTED/noted (low severity, repo-consistent): single-~100-line-It packing 4 concerns (test-
    split candidate for a hardening pass) · `make(...,0,2)` preallocation not test-forced (leave).

---

## S2 COMPLETE — all 9 rungs (R1..R5) green on `p2/cpuhealth` (base 9454a65, 9 commits). NEXT: pre-S3 tag+regenerate pass (SPEC §8, writes at e642457f5 in cpu-rerecord scratch) before S3's recording gate becomes meetable.

## PRE-S3 tag+regenerate pass (SPEC §8) — DONE (5 rows), F6/F7/D4/D5 deliberately NOT tagged → SPEC-reconciliation for Jeremy

Ran in `.worktrees/cpu-rerecord` (detached e642457f5) via subagent. Recorder reproduced the untagged
baseline byte-identically (two runs). Tagged baseline saved to
`artifacts/.../RECORDING_behaviour.txt` (backup `RECORDING_behaviour_untagged.bak`); tagged-vs-untagged
diff is tag-lines-only (programmatically confirmed).

**Tagged (5 rows → 8 scenario-tags):** D2 → throttle/fire-then-clear, throttle/hold-between-marks,
throttle/counter-outage, pressure/fire-then-clear, multi/throttle-pressure-saturation (per DEPARTURES —
D2 changes 5 scenarios, all tagged) · D3 → saturation/nolimit/host-outage-held · F8 →
steal/spike-below-minsamples (§8 explicit) · F1 → pressure/nan-inf-negative (§9 S3 R2) · F1 →
throttle/counter-outage alongside F5 (§9 S4 R3).

**NOT tagged — F6, F7, D4, D5 — a genuine SPEC conflict, surfaced for Jeremy (not silently tagged):**
SPEC §8's blanket "all seven must be tagged" conflicts with its own primary rule "a tag on a scenario
that does not differ is a gate failure." Verified by mechanism that **none of the 33 recorded scenarios
differs under them**: F6 (no scenario drives CpuScope/pinning; all are effectively host/unknown) · F7
(swap scenarios post-outage-recovery + first-readable-tick-dip already F4-tagged; no authoritative F7
assignment) · D4/D5 (sampler-level Sample-shape changes; no Decide-output change in a recorded scenario).
DEPARTURES covers only D1-D3,F1-F5; §5 captures F6/F7/D4/D5/F8 with NO scenario name (F8's is the sole
§8 exception). **Decision recorded:** leave F6/F7/D4/D5 untagged (per the no-diff rule) and get Jeremy's
sign-off that §8's "all seven" wording should be reconciled to "the rows that differ"; if a later S3/S4
rung DOES change one of these scenarios, its tag must be added at that point.
