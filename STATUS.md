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
- [ ] R3 — Reduction · tdd-commit · [F3, F4, F5]
- [ ] R4 — Counter pairs · tdd-commit
- [ ] R5 — Latch · tdd-commit (specs 1-5,7) + spec 6 by hand · [F4, F5, F7]
- [ ] R6 — Ranking · tdd-commit
- [ ] R7 — Instrument + Environment · by hand
- [ ] R7b — Engine · tdd-commit · [F5, F7]
- [ ] R8 — Unconstructable bad states · by hand
- [ ] R9 — Table + suite generator · by hand
