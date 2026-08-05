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

## S3 — judgement

- [x] **S3 R1 — The table, throttle and steal** · tdd-commit · commit `a6bbe36ff` (amended) · 20 specs.
  - Built `cpuTable(cores, quota)` (5 signals / 7 instruments / 2 tracks, all marks per SPEC 2.4 &
    Appendix A), the sig*/inst*/track*/tier* constants, shared stealMarks/throttleMarks, and
    `NewEngine(cores, quota)` as a thin pkg/diagnosis wrapper. No-quota table omits limit-saturation
    entirely (Fire{0}/Clear{0} refuses construction); throttling stays with Requires HasLimit.
    Throttle fires >0.05 and clears <0.03; steal judged on the mean below 20 samples and p95 from 20,
    the below-minimum p95 never selected (F8).
  - **Conductor conformance fix:** the workflow exported `CpuTable`; the skeleton declares unexported
    `cpuTable`. Renamed and moved the R1 specs to a white-box (in-package `package cpuhealth`) test file
    so the unexported builder is reachable, registered into the single Ginkgo suite (a second RunSpecs
    in one test binary is illegal). Black-box S2 specs stay where they are.
  - Mutation-positive-controlled by the workflow agent (throttle clear at 0.03, steal handover at n=20,
    removing the mean arm) — all bite.
- [x] **S3 R1b — The handover at twenty samples** · test-only (binds S1 R7b latch) · commit `c6c3884ba` · 23 specs.
  - The swap from the mean to the p95 at n=20 moves the instrument, not the latch: a fired latch stays
    fired, the published value steps to the p95's own 0.90 (not the mean's 0.18), and on a window whose
    mean sits below the mark nothing fires at the handover.
  - **Conductor decision (SPECT-vs-engine, recorded):** spec 3's "one sample at 0.90, nineteen at 0"
    window, built with the spike at sample 0, FIRES the mean at n=2 (0.45) and then HOLDS through the
    handover, because F4's clear arm is gated on full window coverage (not full at n=20) — "nothing fires"
    would be unobservable. Built the spike at the LAST sample so the mean stays below the fire mark the
    whole run, which is the faithful reading of "a window whose mean sits below the mark". The spec says
    "build the window by hand", which this is.
- [x] **S3 R2 — Pressure [F1]** · tdd-commit · commit `a982c25eb` · 26 specs.
  - Pressure is Last over its window (min 1), fires on tick 0. NaN/Inf refused at the window append →
    the reduction is StateUntrusted and the latch HOLDS (no clamp-to-0). A negative is finite, enters the
    window, is judged as the number it is, and clears a fired latch below 0.12 once coverage is full.
  - **Mutation-positive-controlled:** a clamp-to-0 in the pressure extractor fails both the hold spec and
    the negative spec.
  - **Genuine find (write-up for Jeremy):** the SPEC R2 scenario table says the rebuild is healthy at
    ticks 45-59 (the negative clears at tick 45), but the engine's F4 coverage-gated clear arm cannot
    clear until the window has full 60s coverage, so the rebuild holds degraded through ~tick 60. SPEC
    R2 explicitly says "if a build disagrees on a tick, trust the engine and say which boundary moved" —
    the healthy boundary moves from tick 45 to tick 60. The F1 tag's stated diff (15-44) will differ
    (15-59) at the recording gate; flag for Jeremy, do NOT change engine behavior.
- [x] **S3 R3 — Limit-mode saturation** · tdd-commit · commit `90670a48e` (amended) · 27 specs.
  - limit-headroom = quota - usage - 0.10 x quota, Mean over 60s. Fire at headroom < 0 (usage > 0.90 x
    quota), clear at headroom > 0.05 x quota. saturation/limit/fire: usage 0.2 -> 1.95 at tick 40 fires
    at tick 95 with value -0.0066, settles at -0.15 at tick 100. Asserted value AND state from the right
    tick. Mutation-positive-controlled (0.10 reserve change fails the spec).
- [x] **S3 R4 — Decide, attribution consults its evidence [D1, D2, D3]** · tdd-commit · commit `79b660f97` · 31 specs.
  - Built `decide.go`: State/Attribution/CauseKind/Unit/Cause/Verdict types + the full 38-field Signals
    struct (skeleton shape) + `Decide`. Observe → saturation fold (host-full > no-host-stats > limit) →
    diagnosis.Rank → Verdict + Signals. Attribution from dominant cause and the host/container split
    read back from Engine.Track("host-busy")/Track("usage-cores"): host only when hbm > 2 x oum (strict);
    internal causes (throttling, pressure, limit-saturation) always unknown; host-full by the split.
  - **Conductor find:** the cause VALUE must be the CURRENT windowed reduction read through
    Engine.Reduction, NOT Fired.Value (which is the latch's firing-tick value, constant while held) — the
    recording's cause values move per tick (throttling ratio decays, settled headroom deepens to -0.8).
  - **Equality boundary pinned:** hbm == 2 x oum is unknown (strict >), mutation-positive-controlled
    (changing > to >= fails the equality spec).
- [x] **S3 R5 — The missing guard, and the wrong subtraction [F2, F6]** · tdd-commit · commit `4a02e326b` · 34 specs.
  - host-headroom's Extract carries BOTH guards (Unknown on cores<=0 [F2], Unknown unless CpuScope==
    ScopeHost [F6]) in the EXTRACTOR, not Decide — a Decide-side guard leaves the invalid subtraction in
    the window and re-fires F6. Both mutation-positive-controlled.
  - **Skeleton-conformance gap closed:** Sample was missing `LogicalCpus` (F6's "2"). Added the field and
    filled it from the cpuset read (the CPUs this process may use) — this resolves the S2-flagged design
    fork (cpuset is the source for LogicalCpus). Added a sampler assertion to scope_test.go.
  - Decide fills the three withheld-headroom Signals fields: HostHeadroomAvailable dispatched on the
    scope (withheld, not failed), LogicalCpus, HostCpus.
- [x] **S3 R6 — Hold and demote [F1, F4, F5]** · test-only (binds engine) · commit `b90ee2ca0` · 37 specs.
  - A fired latch holds while its input is stale (window freezes, reduction StateUntrusted); the hold is
    bounded — once BOTH saturation arms go absent past the demote span, AllAbsent releases the latch,
    and the first readable sub-mark tick does not re-fire it; a failed read is not stored as a real zero.
  - **Conductor decision:** spec 2's window must drive BOTH host-headroom and usage-fraction absent, or
    the fallback's clear arm (not the AllAbsent route) releases the latch — the spec-2 release is the
    AllAbsent/demote route.
- [x] **S3 R7 — The fallback metric set** · tdd-commit · commit `be8a478b2` · 39 specs.
  - When host stats are absent, host-headroom's window empties past the demote span, selection walks to
    usage-fraction and JUDGES the saturation question on our own usage against the logical CPU count
    (verified the fallback fires at 0.75 on a no-limit box). NoHostStatsSaturationFraction is
    usage-fraction's OWN reduction, filled independent of latch state. The dead zone — quota nil or
    non-positive AND PSI absent — is an annotation on a healthy verdict carried by Signals.
    LimitedVisibility, never a state. Mutation-positive-controlled on the dead-zone annotation.
  - Note: the track-vs-instrument distinction for NoHostStatsSaturationFraction is unobservable while
    cores is constant (they agree by linearity); the spec acknowledges only a mid-run core-count change
    separates them, which a rebuilt table cannot express. Asserted the value, not the route.
- [x] **S3 R8 — Verdict assembly** · tdd-commit · commit `bf24c044b` · 43 specs.
  - Decide derives Attribution from the dominant cause (steal-dominant → host), orders Causes through
    diagnosis.Rank (the sort grep gate in pkg/cpuhealth is EMPTY; control in pkg/diagnosis shows Rank),
    returns healthy with no causes when nothing fired. From the same pass it fills the observable metrics
    (ThrottleRatio, PressureAvg60Out, StealP95, HostHeadroomCores, AvgUsageCores, HostBusyCores60sMean),
    the two track floors (UsageRingActive, HostBusyRingActive) and the per-signal readiness trio
    (= Ready), independent of latch state — a quiet throttle latch publishes its 0.02, not a confident 0,
    and a bare-metal steal box reports StealSignalReady false. Mutation-positive-controlled on the
    quiet-latch metric fill.
- [x] **S3 R9b — Bind the generated suite to the real CPU table** · subagent+review-equivalent (implemented
  directly + mutant drive) · commit `9797a2465` · 45 specs.
  - RunSuite drives the six-scenario suite from cpuTable itself: 30 scenarios with a positive quota, 24
    without (6×5 and 6×4). cpuFeed's Readable advances the cumulative throttle counters (NrPeriods =
    100×elapsed, NrThrottled = 0.02×NrPeriods, ratio steady under its mark) so DeltaRatio has a
    denominator and CaseLive reaches Ready; Unreadable leaves every Reading absent while holding
    Virtualized/CpuScope fixed. A mutant sixth row whose Extract returns Known(0) on absent reaches Ready
    on the brief outage, long outage and post-outage dip (the suite exposes it) while CaseBelowFloor stays
    green — the "cannot be made green" proof.
- [x] **S3 R10 — Do not flicker** · tdd-commit · commit `cd6d20d65` · 47 specs.
  - The two-mark latch is what stops a signal between its marks from alternating endpoint states.
    throttle/hold-between-marks fires at 0.20, decays and settles at 0.04 (inside the 0.03-0.05 band), and
    the state changes exactly once (healthy at tick 0, degraded at tick 1, held) — asserted as a transition
    count, not the final state. A signal entering the band from below (0 → 0.04) never fires (strict fire
    mark), zero transitions. Mutation-positive-controlled on the throttle fire mark.

---

## S3 COMPLETE — all 11 rungs (R1..R10, R9b) green on `p2/cpuhealth`. 47 cpuhealth specs (16 S2 + 31 S3).
NEXT: pre-S4 R2 F3-tag pass, then S4 (wording).

## Recording-gate passability after S3 (SPEC §8)
- The gate needs ComposeMessage (S4) to render messages, so it is not runnable at the S3 boundary — the
  S3 rungs build Verdict + Signals only. Assessment below is by mechanism, flagged for the S4 end-of-run.
- **F8-tagged `steal/spike-below-minsamples`:** the rebuild's steal mean fires from tick 3 and the p95
  takes over at n=20 with value 0.90 — matches the F8 row's ticks 3-18/19-59 as analyzed in R1/R1b.
  Latch-held value: R1b spec 1 verified the fired latch survives the handover (F7). PASSABLE.
- **F1-tagged `pressure/nan-inf-negative` (+Inf tag `throttle/counter-outage`):** my R2 finding — the SPEC
  R2 scenario table says the rebuild is healthy at ticks 45-59 (negative clears at tick 45), but the
  engine's F4 coverage-gated clear arm cannot clear until the window has full 60s coverage, so the
  rebuild holds degraded through ~tick 60 and the healthy boundary moves 45→60. The F1 row's stated diff
  (15-44) will differ (15-~59) at the recording gate. SPEC R2 explicitly says "if a build disagrees on a
  tick, trust the engine and say which boundary moved" — this is that case. **FLAG for Jeremy: the F1
  tag's expected after needs its 45-59 block reconciled (healthy-at-60). Do NOT change engine behavior.**
- **Untagged F6/F7/D4/D5** (S2 conductor's pre-S3 decision, awaiting Jeremy's sign-off): the S3 rungs
  confirmed none of the 33 recorded scenarios differs under them. R5's LogicalCpus/F6 fields add no
  Decide-output change to a recorded scenario; F7 is the handover; D4/D5 are Sample-shape. Consistent
  with the "no-diff → don't tag" rule.
- **No S3 rung moved a Tier-1 (hard-locked) scenario untagged:** the Tier-1 scenarios (healthy/*,
  throttle/fire-then-clear, steal/fire-after-minsamples, saturation/*, attribution/*, multi/*) are either
  unaffected by S3's judgement (all keyed Decide behavior is covered by an F-tag) or behave as their rows
  require. The only flagged boundary is the tagged Tier-2 `pressure/nan-inf-negative`.

---

## S4 — wording

Five rungs, one commit each, all green under `-tags=test -count=1`. New file `pkg/cpuhealth/message.go`
(ComposeMessage + composeHealthy + causeHeadline + causeDetails + BlockReason); all strings copied
verbatim from STRING_INVENTORY.md by entry number, plus the two §5 sentences (`F3` "CPU: starting up." and
the `F6` host-headroom sentence). 67 cpuhealth specs (S2+S3 47, S4 +20), 106 diagnosis specs.

- [x] **S4 R1 — The healthy headline** · tdd-commit ladder (manual, Workflow tool unavailable) · commit `875388636` · +8 specs.
  - `composeHealthy`: the two-by-two headline dispatch over (LimitApplies && rounded total > 0) x
    (displayed headroom < 0.05); subject follows MODE not column (a tiny quota reads "This instance"
    entry 12/11); total/used/reserve round1'd then headroom = round1(total-used-reserve); total prints
    integer-when-whole; negative zero → "0.0". Zero-capacity (entry 7) is an early return, never a prefix.
    Limited-visibility (entry 5) and the F6 host-headroom sentence sit in the advisory slot.
  - **S3-gap closed here (spec-reconciliation):** `Signals.CapacityCores`, `ReserveCores`,
    `HostBusyCoresAvailable` were DECLARED but never populated by S3's Decide, and `cpuReserveCores` was
    never defined. R1 populates them (capacity = quota-if-positive else LogicalCpus; reserve = 0.10×quota
    limit / 1.0 no-limit; HostBusyCoresAvailable = sample's HostBusy ok bit) and defines
    `cpuReserveCores = 1.0` (D-08 provenance). Pinned by the conformance spec. Not a judgement-change.
- [x] **S4 R2 — The healthy message reports only what it measured [F3]** · manual ladder · commit `8e06c4f5b` · +4 specs.
  - The withholding: limit headline gated on `UsageRingActive`; no-limit on `HostBusyRingActive` OR
    `HostBusyCoresAvailable`. A withheld usage figure renders the WHOLE message as `"CPU: starting up."`
    alone (D-19/D-20, §5 F3), through the same single-line early return as zero-capacity — one tick after
    each start/respawn. The floors are per-track (cross-track assertion in spec 4).
- [x] **S4 R3 — The budget lines [F1, F3]** · manual ladder · commit `8cd2981a0` · +2 specs.
  - Headroom (entry 14) unconditional; throttle/pressure/steal (entries 15-17) gated on
    `ThrottleSignalReady`/`PressureSignalReady`/`StealSignalReady` — NOT the capability flags
    (LimitApplies/PsiApplies/StealApplies), which are F1. Asserted the difference: StealApplies true +
    StealSignalReady false prints no "Steal 0%".
- [x] **S4 R4 — The degraded copy** · manual ladder · commit `939546216` · +4 specs.
  - `ComposeMessage` routes healthy→composeHealthy, degraded→headline (entries 21-25, one per CauseKind,
    entry 25 default) + one detail paragraph per fired cause (entries 26-38), dominant first, joined by a
    blank line. `causeDetails` reads throttling from Signals.ThrottleRatio and pressure/steal from
    Cause.Value; saturation dispatched on which sub-latch arm fired (fold order), appending clauses 34/37
    (leading-space) rather than replacing paragraphs. Arm 6 compound; readable no-limit full host → arm 7.
- [x] **S4 R5 — Block reasons [D1]** · manual ladder · commit `35c399ede` · +2 specs.
  - `BlockReason` per kind (39-47), saturation dispatched on the arm in BlockReason's OWN order
    (host-full, limit, no-host-stats, no-limit-host); entries 42/45 byte-identical (intentional collision).
  - **S3-gap closed here (spec-reconciliation):** `Signals.NoLimitHostFired` was declared but never
    populated by S3's Decide (host-headroom set HostFullFired in BOTH modes). R5 splits the host-full arm
    by mode (SPEC §2.6 arm table): limit→HostFullFired, no-limit→NoLimitHostFired. Pinned by conformance.
- **Conductor note — Workflow tool absent:** this environment exposes no `Workflow` tool (the S2/S3 pattern
  invoked `tdd-commit.js` via it, then the Skill harness). The ladder was run MANUALLY preserving tdd-commit
  discipline per rung: RED (test fails for the missing behavior) → GREEN (minimal prod code, full suite
  green) → verified (gofmt/vet/no-focused/strings-vs-inventory) → one commit each. No workflow COMMIT agent
  existed, so commits are conductor-made with `--no-verify` after manual gofmt/vet/license checks (the
  lefthook gofmt pre-commit hook is broken for conductor-side commits in this worktree per P2 brief §5).
  Each rung's tests assert the FULL exact strings (not substrings where the format is load-bearing), so the
  inventory conformance is pinned by the assertions themselves.

## P2 GATE ASSESSMENT

**Spec totals:** cpuhealth 67 specs green (S2 16 + S3 31 + S4 20); diagnosis 106 green; build/fmt/vet clean,
no focused specs. 5 S4 commits on `p2/cpuhealth` (base `9454a65` via p1/diagnosis): R1 `875388636`,
R2 `8e06c4f5b`, R3 `8cd2981a0`, R4 `939546216`, R5 `35c399ede`.

**Recording gate (SPEC §8) — outcome: PARTIAL, honestly reported.** The shipped recoder harness
(`recorder/recorder.go.txt`) targets the OLD cpuhealth API (`DefaultThresholds`, `WindowState`, old
`Decide(st, sample, th)`, old Sample fields) and cannot be run against the rebuilt signatures without the
§8 re-record protocol, which is a WRITE at `e642457f5` in the `cpu-rerecord` scratch worktree — outside S4's
per-rung scope. I ran a focused throwaway adapter instead (engine + DeriveEnvironment + Decide +
ComposeMessage + BlockReason) over a representative subset and diffed against `RECORDING_behaviour.txt`:

- **VERIFIED (healthy layer):** `healthy/limit/idle`, `healthy/nolimit/idle`, `healthy/deadzone`,
  `healthy/tiny-quota`, `healthy/zero-capacity`, `healthy/close-to-degraded/limit` all reproduce the
  baseline's steady-state (ticks 1+) MESSAGE strings EXACTLY (headline arithmetic, headroom body, budget
  lines, limited-visibility note, zero-capacity = entry 7 alone). Tick 0 renders `"CPU: starting up."`
  (the F3-expected withholding; healthy/zero-capacity is not F3-tagged and correctly stays entry 7 at tick 0).
- **VERIFIED (degraded layer):** the saturation/limit/degraded cases render the correct arm paragraphs and
  block reasons (limit-saturation → entries 33/43; host-full → entry 30/42) matching the inventory; the F6
  byte-identical 42=45 collision survives; Decide attributes `unknown` at steady state when the split says
  our load filled the machine (D1).
- **NOT mechanically re-run:** the full 33-scenario run-length diff (the recorder targets the old API; a
  faithful re-run is the §8 re-record protocol, a separate scratch-worktree write). So the Tier-1/Tier-2
  HALT/REPORT verdicts are NOT certified end-to-end here; that gate remains for the independent verifier
  (or P3), which is the §4a-signed closeout.

**Reconcile-open for Jeremy (this build's write-ups):**
1. 🔴 **D1 spec-2 ("not told to reduce other software" when own instance filled the machine) is NOT
   satisfiable by `BlockReason` as frozen.** `BlockReason(dominantKind, signals)` receives no attribution,
   `Signals` carries none, and the departure set has no blame-aware block-reason string; the authoritative
   R5 arm table ships `NoLimitHostFired` → entry 45 ("reduce other software") for BOTH host-dominant and
   container-dominant. The D1 attribution change (host→unknown) IS delivered by S3 R4's split and was
   verified; the sentence-level clause needs Jeremy's ruling (attribution input to the message functions,
   or a new §5 row — both outside current frozen scope). **P1-worthy: it is a customer-visible wording
   question surface.**
2. 🔴 **D1 attribution transient at host-full-AND-limit:** at the early degraded ticks the host-busy / ours
   means are still window-ramping, so the split transiently reads `host` before the full-60s window settles
   it to `unknown` (matches the S3 scenario table's steady-state ticks 100-149). The scenario is D1-tagged;
   the early-tick transient is a real behavior a full gate must reconcile with the D1 row's expected after.
3. **S3 R2 carryover (flagged in S3 handoff):** the `pressure/nan-inf-negative` healthy boundary moves
   45→60 (F4 coverage-gated clear), so the F1 tag's stated diff (15-44) differs (≈15-59) at the gate. Do not
   change engine behavior; reconcile the F1 row's expected-after.
4. **Untagged F6/F7/D4/D5** (S2 conductor's no-diff decision): still awaiting Jeremy's sign-off; S3 and S4
   confirmed no recorded scenario differs under them.
5. **R1b spec-3 window** (S3 handoff): the "nothing fires" unobservable spike placement made buildable by
   placing the spike at the last sample; recorded in S3 STATUS.

**P1/pkg/diagnosis defects found: none.** The three S3-gaps S4 closed (CapacityCores/ReserveCores/
HostBusyCoresAvailable/NoLimitHostFired population + cpuReserveCores constant) are within `pkg/cpuhealth`
(P2), not P1.

**Next actions:** independent verify re-runs the gates w/ positive controls; PR #2680 (P2 → p1/diagnosis /
staging); P3. Do NOT mark P2 done (per brief §7 — a separate session certifies).
