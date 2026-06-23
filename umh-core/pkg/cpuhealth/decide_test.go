// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build test

package cpuhealth

import (
	"math"
	"testing"
	"time"
)

// TestDecide_SaturationOnly is the characterization test for the pure
// pkg/cpuhealth library. It pins the behaviors the verdict contracts. The
// usage fraction's denominator is the cgroup quota (Sample.CgroupCores),
// not host cores; the test pins that.
//
// Decide no longer degrades on raw usage: high usage alone is healthy
// (busy is not sick). The earlier degraded-on-high-usage assertions were
// reshaped accordingly; the fraction computation, uncapped guardrail, and
// low-usage healthy pins below are unchanged.
//
//  1. A high-usage sample (UsageCores >= 0.70 * CgroupCores, CgroupCores>0)
//     yields State==healthy with no causes — high usage alone is not ill
//     health. Signals.UsageFraction is still the quota-relative fraction.
//  2. An idle/low-usage sample (UsageCores < 0.70 * CgroupCores) yields
//     State==healthy, Attribution empty/zero, Causes nil-or-empty.
//  3. Signals.UsageFraction == UsageCores/CgroupCores when CgroupCores>0,
//     else 0.
//
// Saturation-only: no throttle/pressure/steal/host-contention/windowing/
// dominance. The library is pure (no I/O, no time.Now) and imports neither
// models.* nor fsm/*.
func TestDecide_SaturationOnly(t *testing.T) {
	ts := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	defaultThresholds := DefaultThresholds()

	// (4) Signals.UsageFraction is the quota-relative fraction, computed
	// purely from the Sample (no time.Now, no I/O). With CgroupCores>0 it is
	// UsageCores/CgroupCores; with CgroupCores==0 it is 0 (guard against
	// divide-by-zero).
	t.Run("Signals.UsageFraction is quota-relative and divide-safe", func(t *testing.T) {
		st := &WindowState{}
		sample := Sample{Timestamp: ts, UsageCores: 1.4, CgroupCores: 2.0}
		_, signals := Decide(st, sample, defaultThresholds)
		if got, want := signals.UsageFraction, 1.4/2.0; !floatEq(got, want) {
			t.Fatalf("UsageFraction with CgroupCores>0: got %v, want %v", got, want)
		}

		// Zero-denominator guard: an uncapped (CgroupCores<=0) sample stays
		// healthy regardless of UsageCores, pinning the documented contract
		// that host-contention attribution is not computed by Decide.
		stZero := &WindowState{}
		uncapped := Sample{Timestamp: ts, UsageCores: 100, CgroupCores: 0}
		verdictZero, signalsZero := Decide(stZero, uncapped, defaultThresholds)
		if got := signalsZero.UsageFraction; got != 0 {
			t.Fatalf("UsageFraction with CgroupCores==0: got %v, want 0 (divide-safe)", got)
		}
		if verdictZero.State != StateHealthy {
			t.Fatalf("uncapped high-usage State: got %q, want %q", verdictZero.State, StateHealthy)
		}
		if verdictZero.Attribution != Attribution("") {
			t.Fatalf("uncapped high-usage Attribution: got %q, want empty", verdictZero.Attribution)
		}
		if len(verdictZero.Causes) != 0 {
			t.Fatalf("uncapped high-usage Causes length: got %d, want 0", len(verdictZero.Causes))
		}
	})

	// (1) High-usage sample crosses the 0.70 quota-relative threshold but
	// high usage alone is healthy — busy is not sick. The fraction is still
	// computed and exposed on Signals (quota-relative denominator).
	t.Run("high usage stays healthy with fraction computed", func(t *testing.T) {
		st := &WindowState{}
		// 1.5 of 2.0 cores = 0.75 fraction, >= 0.70.
		sample := Sample{Timestamp: ts, UsageCores: 1.5, CgroupCores: 2.0}
		verdict, signals := Decide(st, sample, defaultThresholds)

		if verdict.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (high usage alone is healthy)", verdict.State, StateHealthy)
		}
		if verdict.Attribution != Attribution("") {
			t.Fatalf("Attribution: got %q, want empty (healthy carries no attribution)", verdict.Attribution)
		}
		if len(verdict.Causes) != 0 {
			t.Fatalf("Causes length: got %d, want 0 (no saturation cause on raw usage)", len(verdict.Causes))
		}
		// The fraction is the quota-relative one (1.5/2.0 = 0.75): the
		// denominator is the cgroup quota, not host cores.
		if got, want := signals.UsageFraction, 0.75; !floatEq(got, want) {
			t.Fatalf("Signals.UsageFraction (quota-relative): got %v, want 0.75", got)
		}
	})

	// (2) Idle/low-usage sample stays healthy with no causes: below
	// HighUsageFraction the verdict has no Causes and State stays StateHealthy
	// regardless of Sample.UsageCores.
	t.Run("low usage stays healthy with no causes", func(t *testing.T) {
		st := &WindowState{}
		// 0.5 of 2.0 cores = 0.25 fraction, < 0.70.
		sample := Sample{Timestamp: ts, UsageCores: 0.5, CgroupCores: 2.0}
		verdict, _ := Decide(st, sample, defaultThresholds)

		if verdict.State != StateHealthy {
			t.Fatalf("State: got %q, want %q", verdict.State, StateHealthy)
		}
		if verdict.Attribution != Attribution("") {
			t.Fatalf("Attribution: got %q, want empty (healthy carries no attribution)", verdict.Attribution)
		}
		if len(verdict.Causes) != 0 {
			t.Fatalf("Causes length: got %d, want 0 (healthy has no causes)", len(verdict.Causes))
		}
	})
}

// TestDecide_QuotaPointerUncapped pins the Sample.Quota *float64 contract:
// a non-nil positive Quota derives the usage fraction from it; a non-nil
// non-positive Quota means uncapped (no fallback to CgroupCores); a nil Quota
// falls back to CgroupCores (which remains the live production path for
// callers that still populate it). The pointer kills the zero-sentinel that
// conflated read-failed / uncapped / first-call / genuinely-idle and let
// Decide silently return healthy on any of them via the `CgroupCores > 0`
// guard. With the pointer, uncapped is explicit and the healthy verdict it
// yields is deliberate (the guardrail: uncapped cannot be quota-saturated),
// not the accidental fraction=0 → healthy of the sentinel.
//
// NOTE: Quota is not yet populated by the production caller (container_monitor
// does not currently call Decide; the cgroupSampler and windowState fields are
// retained for the saturation wiring in WindowState). This test exercises the
// Quota branch in isolation so the type-level seam is correct and safe to
// wire; it is not a production-vetted path yet.
//
// CgroupCores is retained on the Sample struct as a legacy fallback; this test
// exercises Quota only (TestDecide_SaturationOnly covers CgroupCores). Pinned
// behaviors:
//
//  1. Capped sample (Quota → 2.0), UsageCores 1.5 → fraction 0.75 → healthy
//     (high usage alone is not ill health); fraction still computed.
//  2. Capped sample (Quota → 2.0), UsageCores 0.5 → fraction 0.25 → healthy.
//  3. Uncapped sample (Quota nil), high UsageCores → DELIBERATE healthy: no
//     fraction is computed (Signals.UsageFraction == 0), the same uncapped
//     sample at a higher UsageCores also stays healthy, AND a capped sample at
//     the same UsageCores computes a non-zero fraction — so the uncapped
//     fraction-0 is the guardrail, not the fraction-zero accident.
//  4. A non-nil non-positive Quota (zero, negative, NaN) is treated as
//     uncapped — Decide does NOT divide by it (no +Inf/NaN poison fraction) and
//     does NOT fall back to CgroupCores, so a zero quota (the unlimited-cgroup
//     case) stays healthy with no fraction even when CgroupCores is positive.
//  5. When both Quota (non-nil positive) and CgroupCores are set, Quota wins
//     (the precedence contract): the fraction is UsageCores/Quota, not
//     UsageCores/CgroupCores.
func TestDecide_QuotaPointerUncapped(t *testing.T) {
	ts := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	defaultThresholds := DefaultThresholds()

	// (1) Capped sample, Quota 2.0, UsageCores 1.5 → fraction 0.75. High
	// usage alone is healthy, so the verdict is healthy with no causes; the
	// quota-relative fraction is still computed and exposed on Signals.
	t.Run("capped high usage stays healthy with fraction computed", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		sample := Sample{Timestamp: ts, UsageCores: 1.5, Quota: &quota}
		verdict, signals := Decide(st, sample, defaultThresholds)

		if verdict.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (high usage alone is healthy)", verdict.State, StateHealthy)
		}
		if verdict.Attribution != Attribution("") {
			t.Fatalf("Attribution: got %q, want empty (healthy carries no attribution)", verdict.Attribution)
		}
		if len(verdict.Causes) != 0 {
			t.Fatalf("Causes length: got %d, want 0 (no saturation cause on raw usage)", len(verdict.Causes))
		}
		if got, want := signals.UsageFraction, 0.75; !floatEq(got, want) {
			t.Fatalf("Signals.UsageFraction: got %v, want %v", got, want)
		}
	})

	// (2) Capped sample, Quota 2.0, UsageCores 0.5 → fraction 0.25 → healthy.
	t.Run("capped low usage stays healthy", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		sample := Sample{Timestamp: ts, UsageCores: 0.5, Quota: &quota}
		verdict, _ := Decide(st, sample, defaultThresholds)

		if verdict.State != StateHealthy {
			t.Fatalf("State: got %q, want %q", verdict.State, StateHealthy)
		}
		if verdict.Attribution != Attribution("") {
			t.Fatalf("Attribution: got %q, want empty (healthy carries no attribution)", verdict.Attribution)
		}
		if len(verdict.Causes) != 0 {
			t.Fatalf("Causes length: got %d, want 0 (healthy has no causes)", len(verdict.Causes))
		}
	})

	// (3) Uncapped sample (Quota nil), high UsageCores → DELIBERATE healthy.
	// Deliberateness proof has two prongs: (a) raising UsageCores on the same
	// uncapped sample keeps it healthy with no fraction computed (uncapped
	// cannot be quota-saturated, so no fraction is derived); (b) a capped
	// sample at the same UsageCores degrades — proving Decide would degrade
	// given a quota, so the uncapped healthy is the guardrail, not the
	// fraction-zero accident of the old CgroupCores sentinel.
	t.Run("uncapped high usage is deliberately healthy", func(t *testing.T) {
		st := &WindowState{}
		uncapped := Sample{Timestamp: ts, UsageCores: 5.0, Quota: nil}
		verdict, signals := Decide(st, uncapped, defaultThresholds)

		if verdict.State != StateHealthy {
			t.Fatalf("uncapped high-usage State: got %q, want %q (guardrail: uncapped cannot be quota-saturated)", verdict.State, StateHealthy)
		}
		if verdict.Attribution != Attribution("") {
			t.Fatalf("uncapped high-usage Attribution: got %q, want empty", verdict.Attribution)
		}
		if len(verdict.Causes) != 0 {
			t.Fatalf("uncapped high-usage Causes length: got %d, want 0", len(verdict.Causes))
		}
		if got := signals.UsageFraction; got != 0 {
			t.Fatalf("uncapped Signals.UsageFraction: got %v, want 0 (no fraction computed when Quota nil)", got)
		}

		// (a) Higher UsageCores on the same uncapped sample stays healthy — no
		// fraction is computed, so the verdict cannot flip on usage magnitude.
		stHigher := &WindowState{}
		uncappedHigher := Sample{Timestamp: ts, UsageCores: 50.0, Quota: nil}
		verdictHigher, signalsHigher := Decide(stHigher, uncappedHigher, defaultThresholds)
		if verdictHigher.State != StateHealthy {
			t.Fatalf("uncapped higher-usage State: got %q, want %q", verdictHigher.State, StateHealthy)
		}
		if got := signalsHigher.UsageFraction; got != 0 {
			t.Fatalf("uncapped higher-usage Signals.UsageFraction: got %v, want 0", got)
		}

		// (b) A capped sample at the same UsageCores (5.0) also stays
		// healthy (high usage alone is not ill health), but computes a
		// non-zero UsageFraction where the uncapped sample computes none.
		// That fraction difference is what the future saturation logic in
		// WindowState will read.
		quotaTwo := 2.0
		stCapped := &WindowState{}
		capped := Sample{Timestamp: ts, UsageCores: 5.0, Quota: &quotaTwo}
		verdictCapped, signalsCapped := Decide(stCapped, capped, defaultThresholds)
		if verdictCapped.State != StateHealthy {
			t.Fatalf("capped same-usage State: got %q, want %q (high usage is healthy)", verdictCapped.State, StateHealthy)
		}
		if got, want := signalsCapped.UsageFraction, 5.0/2.0; !floatEq(got, want) {
			t.Fatalf("capped same-usage UsageFraction: got %v, want %v (non-zero where uncapped computes none)", got, want)
		}
	})

	// (4) A non-nil non-positive Quota (zero, negative, NaN) is treated as
	// uncapped: the `> 0` guard rejects all three so Decide does NOT divide by
	// it, and because Quota is non-nil Decide does NOT fall back to CgroupCores
	// either. Without the guard, Quota=&0 with UsageCores>0 yields +Inf
	// (spurious degraded with a +Inf poison Value), Quota=&0 with
	// UsageCores==0 yields 0/0=NaN (silent healthy on a fully-throttled
	// condition), and NaN/negative Quota yields a NaN/negative fraction
	// (silent healthy). The guard makes all of these deliberate uncapped-healthy
	// with no fraction computed, mirroring the legacy CgroupCores>0 guard. This
	// is the case that matters when Quota is wired from the real cpu.max:
	// parseCPUMax returns quotaCores==0 for `cpu.max = "max <period>"` (the
	// default unlimited cgroup), so a Quota=&0 must not poison the verdict even
	// if CgroupCores is still populated.
	t.Run("non-positive Quota is treated as uncapped not divided", func(t *testing.T) {
		// (4a) Quota=&0.0, UsageCores>0 → uncapped healthy, fraction 0 (no
		// +Inf poison, no spurious degraded).
		zero := 0.0
		stA := &WindowState{}
		sampleA := Sample{Timestamp: ts, UsageCores: 1.5, Quota: &zero}
		verdictA, signalsA := Decide(stA, sampleA, defaultThresholds)
		if verdictA.State != StateHealthy {
			t.Fatalf("Quota=&0 UsageCores>0 State: got %q, want %q (zero quota is uncapped, not +Inf degraded)", verdictA.State, StateHealthy)
		}
		if math.IsInf(signalsA.UsageFraction, 0) || math.IsNaN(signalsA.UsageFraction) {
			t.Fatalf("Quota=&0 UsageCores>0 Signals.UsageFraction: got %v, want finite 0 (no +Inf/NaN poison)", signalsA.UsageFraction)
		}
		if signalsA.UsageFraction != 0 {
			t.Fatalf("Quota=&0 UsageCores>0 Signals.UsageFraction: got %v, want 0 (no fraction computed for non-positive Quota)", signalsA.UsageFraction)
		}
		if len(verdictA.Causes) != 0 {
			t.Fatalf("Quota=&0 UsageCores>0 Causes length: got %d, want 0", len(verdictA.Causes))
		}

		// (4b) Quota=&0.0, UsageCores==0 → uncapped healthy, fraction 0 (not
		// the 0/0=NaN silent-healthy accident).
		stB := &WindowState{}
		sampleB := Sample{Timestamp: ts, UsageCores: 0, Quota: &zero}
		verdictB, signalsB := Decide(stB, sampleB, defaultThresholds)
		if verdictB.State != StateHealthy {
			t.Fatalf("Quota=&0 UsageCores==0 State: got %q, want %q", verdictB.State, StateHealthy)
		}
		if math.IsNaN(signalsB.UsageFraction) {
			t.Fatalf("Quota=&0 UsageCores==0 Signals.UsageFraction: got NaN, want 0 (no 0/0 NaN)")
		}
		if signalsB.UsageFraction != 0 {
			t.Fatalf("Quota=&0 UsageCores==0 Signals.UsageFraction: got %v, want 0", signalsB.UsageFraction)
		}

		// (4c) Quota=&NaN → uncapped healthy, fraction 0 (not a NaN fraction
		// that would silently force healthy via `NaN >= 0.70` false).
		nan := math.NaN()
		stC := &WindowState{}
		sampleC := Sample{Timestamp: ts, UsageCores: 1.5, Quota: &nan}
		verdictC, signalsC := Decide(stC, sampleC, defaultThresholds)
		if verdictC.State != StateHealthy {
			t.Fatalf("Quota=&NaN State: got %q, want %q (NaN quota is uncapped)", verdictC.State, StateHealthy)
		}
		if math.IsNaN(signalsC.UsageFraction) {
			t.Fatalf("Quota=&NaN Signals.UsageFraction: got NaN, want 0 (no NaN poison)")
		}
		if signalsC.UsageFraction != 0 {
			t.Fatalf("Quota=&NaN Signals.UsageFraction: got %v, want 0", signalsC.UsageFraction)
		}

		// (4d) Quota=&-1.0 → uncapped healthy, fraction 0 (not a negative
		// fraction that would silently force healthy).
		neg := -1.0
		stD := &WindowState{}
		sampleD := Sample{Timestamp: ts, UsageCores: 1.5, Quota: &neg}
		verdictD, signalsD := Decide(stD, sampleD, defaultThresholds)
		if verdictD.State != StateHealthy {
			t.Fatalf("Quota=&-1.0 State: got %q, want %q (negative quota is uncapped)", verdictD.State, StateHealthy)
		}
		if signalsD.UsageFraction != 0 {
			t.Fatalf("Quota=&-1.0 Signals.UsageFraction: got %v, want 0 (no negative poison)", signalsD.UsageFraction)
		}

		// (4e) The load-bearing pin: a non-positive Quota WITH a positive
		// CgroupCores stays uncapped. Because Quota is non-nil, Decide must NOT
		// fall back to CgroupCores — otherwise a wired Quota=&0 (unlimited
		// cgroup) combined with a still-populated CgroupCores would compute
		// UsageCores/CgroupCores and degrade, the exact blindness the seam
		// exists to prevent. Quota=&0, CgroupCores=2.0, UsageCores=1.5 would
		// yield fraction 0.75 → degraded under a fall-through; under the
		// uncapped rule it stays healthy with fraction 0.
		zeroE := 0.0
		stE := &WindowState{}
		sampleE := Sample{Timestamp: ts, UsageCores: 1.5, Quota: &zeroE, CgroupCores: 2.0}
		verdictE, signalsE := Decide(stE, sampleE, defaultThresholds)
		if verdictE.State != StateHealthy {
			t.Fatalf("Quota=&0 CgroupCores=2.0 State: got %q, want %q (non-nil non-positive Quota is uncapped even when CgroupCores>0; must not fall through to CgroupCores)", verdictE.State, StateHealthy)
		}
		if signalsE.UsageFraction != 0 {
			t.Fatalf("Quota=&0 CgroupCores=2.0 Signals.UsageFraction: got %v, want 0 (no fallback to CgroupCores when Quota is non-nil)", signalsE.UsageFraction)
		}
		if len(verdictE.Causes) != 0 {
			t.Fatalf("Quota=&0 CgroupCores=2.0 Causes length: got %d, want 0 (uncapped has no causes)", len(verdictE.Causes))
		}
	})

	// (5) Precedence: when both Quota (non-nil positive) and CgroupCores are
	// set to differing values, Quota wins — the fraction is UsageCores/Quota,
	// not UsageCores/CgroupCores. Pins the doc contract at decide.go: a future
	// refactor flipping the branch order would otherwise pass CI. The verdict
	// is healthy either way (high usage alone is not ill health), so
	// precedence is pinned solely by the fraction value.
	t.Run("both set: Quota wins over CgroupCores", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		// Quota=2.0, CgroupCores=4.0, UsageCores=1.5 → Quota-derived 0.75,
		// NOT CgroupCores-derived 0.375.
		sample := Sample{Timestamp: ts, UsageCores: 1.5, Quota: &quota, CgroupCores: 4.0}
		verdict, signals := Decide(st, sample, defaultThresholds)

		if verdict.State != StateHealthy {
			t.Fatalf("both-set State: got %q, want %q (high usage is healthy)", verdict.State, StateHealthy)
		}
		if got, want := signals.UsageFraction, 0.75; !floatEq(got, want) {
			t.Fatalf("both-set Signals.UsageFraction: got %v, want %v (Quota-derived 1.5/2.0, not CgroupCores-derived 1.5/4.0=0.375)", got, want)
		}
		if len(verdict.Causes) != 0 {
			t.Fatalf("both-set Causes length: got %d, want 0 (no saturation cause on raw usage)", len(verdict.Causes))
		}
	})
}

// TestDecide_HighUsageAloneIsHealthy pins the Decide library thesis: high
// CPU utilization is NOT ill health. A capped container pinned at 0.95 of
// its quota with no throttle/pressure/steal/host-contention signal is
// working, not sick — Decide must return StateHealthy with no causes.
// Decide no longer degrades on raw usage >= HighUsageFraction; saturation
// will be decided from a windowed average of UsageFraction in WindowState,
// never from a single sample's raw usage. Throttling is a separate
// starvation signal handled outside Decide.
//
// This test pins the pure cpuhealth.Decide contract, not the end-to-end
// container monitor behavior: GetStatus still degrades on high raw
// cpuPercent until saturation logic in WindowState replaces it.
//
// Pinned behaviors:
//
//  1. A capped sample (Quota 2.0) at very high usage (UsageCores 1.9 of 2.0
//     = 0.95 fraction) and no other signals returns StateHealthy with no
//     causes and no attribution — NOT degraded.
//  2. A capped sample at low usage (0.5 of 2.0) still returns healthy
//     (unchanged).
func TestDecide_HighUsageAloneIsHealthy(t *testing.T) {
	ts := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	defaultThresholds := DefaultThresholds()

	// (1) Capped at 0.95 quota-relative fraction: busy is not sick. Decide
	// must not degrade on raw usage alone.
	t.Run("capped very high usage is healthy with no causes", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		sample := Sample{Timestamp: ts, UsageCores: 1.9, Quota: &quota}
		verdict, _ := Decide(st, sample, defaultThresholds)

		if verdict.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (high usage alone is not ill health; saturation cause removed from Decide)", verdict.State, StateHealthy)
		}
		if verdict.Attribution != Attribution("") {
			t.Fatalf("Attribution: got %q, want empty (healthy carries no attribution)", verdict.Attribution)
		}
		if len(verdict.Causes) != 0 {
			t.Fatalf("Causes length: got %d, want 0 (busy is not sick; no saturation cause on raw usage)", len(verdict.Causes))
		}
	})

	// (2) Capped low-usage sample stays healthy: low usage was and remains
	// healthy.
	t.Run("capped low usage stays healthy", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		sample := Sample{Timestamp: ts, UsageCores: 0.5, Quota: &quota}
		verdict, _ := Decide(st, sample, defaultThresholds)

		if verdict.State != StateHealthy {
			t.Fatalf("State: got %q, want %q", verdict.State, StateHealthy)
		}
		if verdict.Attribution != Attribution("") {
			t.Fatalf("Attribution: got %q, want empty", verdict.Attribution)
		}
		if len(verdict.Causes) != 0 {
			t.Fatalf("Causes length: got %d, want 0", len(verdict.Causes))
		}
	})
}

// floatEq compares two floats with a tolerance tight enough for these
// arithmetic-only assertions.
func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// TestDecide_ThrottleFlipLatch_WindowedSchmitt pins the windowed Schmitt
// flip-latch throttle verdict in Decide. WindowState holds a 60s sample ring
// and a dual-threshold flip-latch per signal. The sliding 60s window IS the
// debounce (consecutive evaluations share ~60s of data, so the reduced value
// moves slowly regardless of tick rate). The only extra mechanism is the
// asymmetric (Schmitt) recover band: the throttle cause FIRES when the 60s
// ratio rises above ThrottleHigh (0.05) and CLEARS only when it falls below
// ThrottleRecover (0.03); between the two marks the latch holds its prior
// state.
//
// The 60s ratio is the two-point counter delta (nr_throttled delta /
// nr_periods delta, oldest-to-newest over the windowed ring). The window
// prunes samples older than 60s; a sample exactly at the cutoff is kept
// (Before(cutoff) is false).
//
// One linear scenario walks the flip-latch through every state:
//
//  1. TRANSIENT — a single spiked sample whose 60s cumulative ratio stays
//     below 0.05 does NOT fire the latch (the window absorbs it): healthy,
//     Signals.ThrottleFired false, no causes.
//  2. FIRE — a second spike pushes the 60s ratio above 0.05 → latch fires:
//     {degraded, unknown, [throttling]} with Cause Value = the 60s ratio,
//     Signals.ThrottleFired true.
//  3. HOLD (Schmitt) — calm samples bring the 60s ratio down to 0.045
//     (between the 0.03 recover and 0.05 high marks) → latch stays fired:
//     still degraded.
//  4. CLEAR — further calm samples drop the 60s ratio below 0.03 → latch
//     clears: healthy, Signals.ThrottleFired false, no causes.
//
// Steal/pressure/host-contention/saturation are later rungs and are not
// exercised here; the Sample carries only throttle counters at this rung.
func TestDecide_ThrottleFlipLatch_WindowedSchmitt(t *testing.T) {
	// 10s tick; 60s window holds ~7 samples. cutoff = now - 60s; a sample
	// exactly at cutoff is kept (Before(cutoff) is false).
	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()
	st := &WindowState{}

	type tick struct {
		dt  time.Duration
		nrP int64
		nrT int64
	}

	// decide feeds one timestamped throttle sample into WindowState via Decide
	// and returns the resulting verdict + signals. Decide mutates st in place
	// (appends to the throttle ring, updates the flip-latch), so the sequence
	// is stateful across calls — exactly the caller-held-state contract.
	decide := func(tk tick) (Verdict, Signals) {
		return Decide(st, Sample{
			Timestamp:   base.Add(tk.dt),
			NrPeriods:   tk.nrP,
			NrThrottled: tk.nrT,
		}, thresholds)
	}

	// (1) TRANSIENT — six calm ticks at ~0.01 instantaneous ratio, then one
	// spike (+200 throttled in a single tick). The 60s cumulative ratio at
	// t=60s is 250/6000 = 0.0417, below the 0.05 high mark, so the latch must
	// NOT fire: the window absorbs the transient spike.
	for _, tk := range []tick{
		{0, 0, 0},
		{10 * time.Second, 1000, 10},
		{20 * time.Second, 2000, 20},
		{30 * time.Second, 3000, 30},
		{40 * time.Second, 4000, 40},
		{50 * time.Second, 5000, 50},
	} {
		decide(tk)
	}
	v1, sig1 := decide(tick{60 * time.Second, 6000, 250})
	if v1.State != StateHealthy {
		t.Fatalf("transient State: got %q, want %q (60s ratio 0.0417 < 0.05; window absorbs the spike)", v1.State, StateHealthy)
	}
	if sig1.ThrottleFired {
		t.Fatalf("transient ThrottleFired: got true, want false (latch must not fire on a transient breach the window absorbs)")
	}
	if len(v1.Causes) != 0 {
		t.Fatalf("transient Causes length: got %d, want 0", len(v1.Causes))
	}

	// (2) FIRE — a second spike (+220 throttled this tick). The 60s cumulative
	// ratio at t=70s is (470-10)/(7000-1000) = 460/6000 = 0.0767, above the
	// 0.05 high mark, so the latch fires: degraded, unknown attribution, a
	// single throttling cause whose Value is the 60s ratio.
	v2, sig2 := decide(tick{70 * time.Second, 7000, 470})
	if v2.State != StateDegraded {
		t.Fatalf("fire State: got %q, want %q (60s ratio 0.0767 > 0.05 high mark)", v2.State, StateDegraded)
	}
	if v2.Attribution != AttributionUnknown {
		t.Fatalf("fire Attribution: got %q, want %q", v2.Attribution, AttributionUnknown)
	}
	if !sig2.ThrottleFired {
		t.Fatalf("fire ThrottleFired: got false, want true (latch fires above the high mark)")
	}
	if len(v2.Causes) != 1 {
		t.Fatalf("fire Causes length: got %d, want 1 (single throttling cause)", len(v2.Causes))
	}
	if v2.Causes[0].Kind != CauseKindThrottling {
		t.Fatalf("fire Cause Kind: got %q, want %q", v2.Causes[0].Kind, CauseKindThrottling)
	}
	wantRatio := 460.0 / 6000.0
	if !floatEq(v2.Causes[0].Value, wantRatio) {
		t.Fatalf("fire Cause Value: got %v, want %v (the 60s windowed ratio)", v2.Causes[0].Value, wantRatio)
	}

	// (3) HOLD (Schmitt) — calm ticks (+10 throttled each) until the spiked
	// samples age partway out and the 60s cumulative ratio drops to 0.045,
	// between the 0.03 recover and 0.05 high marks. The latch must hold its
	// fired state: a ratio in the band does NOT clear it.
	for _, tk := range []tick{
		{80 * time.Second, 8000, 480},
		{90 * time.Second, 9000, 490},
		{100 * time.Second, 10000, 500},
		{110 * time.Second, 11000, 510},
	} {
		decide(tk)
	}
	// At t=120s: cutoff=t=60s; window = t=60..t=120. Oldest=t=60 (6000,250),
	// newest=t=120 (12000,520). Delta = (520-250)/(12000-6000) = 270/6000 =
	// 0.045 — in the Schmitt band.
	v3, sig3 := decide(tick{120 * time.Second, 12000, 520})
	if v3.State != StateDegraded {
		t.Fatalf("hold State: got %q, want %q (60s ratio 0.045 is in the Schmitt band; latch holds fired)", v3.State, StateDegraded)
	}
	if !sig3.ThrottleFired {
		t.Fatalf("hold ThrottleFired: got false, want true (latch holds in the band; only drops below ThrottleRecover clears)")
	}
	if len(v3.Causes) != 1 || v3.Causes[0].Kind != CauseKindThrottling {
		t.Fatalf("hold Causes: got %+v, want one throttling cause", v3.Causes)
	}

	// (4) CLEAR — one more calm tick. At t=130s: cutoff=t=70s; window =
	// t=70..t=130. Oldest=t=70 (7000,470), newest=t=130 (13000,530). Delta =
	// (530-470)/(13000-7000) = 60/6000 = 0.01, below the 0.03 recover mark, so
	// the latch clears: healthy, ThrottleFired false, no causes.
	v4, sig4 := decide(tick{130 * time.Second, 13000, 530})
	if v4.State != StateHealthy {
		t.Fatalf("clear State: got %q, want %q (60s ratio 0.01 < 0.03 recover mark; latch clears)", v4.State, StateHealthy)
	}
	if sig4.ThrottleFired {
		t.Fatalf("clear ThrottleFired: got true, want false (latch clears below the recover mark)")
	}
	if len(v4.Causes) != 0 {
		t.Fatalf("clear Causes length: got %d, want 0", len(v4.Causes))
	}
}

// TestDecide_ThrottleFlipLatch_CounterReset pins the counter-reset behavior.
// When a cgroup is recreated mid-incident (container restart), the
// nr_throttled and/or nr_periods counters drop. Decide clears the ring before
// appending when either counter regresses below the ring's newest entry, so
// the reset sample becomes a fresh baseline: the ring holds a single point,
// throttleRatio returns 0 (len < 2), and 0 < ThrottleRecover clears the latch.
// In both scenarios no negative Cause Value is emitted.
func TestDecide_ThrottleFlipLatch_CounterReset(t *testing.T) {
	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// Scenario A: both-counter reset (periods <= 0). The ring's oldest point
	// has high counters; the newest has lower counters on both fields (cgroup
	// recreated). The periods guard catches this: periods = newest.nrPeriods -
	// oldest.nrPeriods < 0 → return 0. Without the guard, the double-negative
	// (negative nrThrottled delta / negative period delta) would yield a
	// spurious positive ratio that keeps the latch fired.
	t.Run("both-counter reset clears latch with no negative Cause Value", func(t *testing.T) {
		st := &WindowState{}
		// Fire the latch: t=0 (1000, 100), t=10s (2000, 200) → ratio 0.1 > 0.05.
		Decide(st, Sample{Timestamp: base, NrPeriods: 1000, NrThrottled: 100}, thresholds)
		v1, sig1 := Decide(st, Sample{Timestamp: base.Add(10 * time.Second), NrPeriods: 2000, NrThrottled: 200}, thresholds)
		if v1.State != StateDegraded || !sig1.ThrottleFired {
			t.Fatalf("fire: state=%q fired=%v, want degraded/fired", v1.State, sig1.ThrottleFired)
		}
		// Both-counter reset: nrPeriods 2000→50, nrThrottled 200→5. The ring is
		// cleared (50 < 2000 nrPeriods regresses), the reset sample becomes the
		// sole ring entry, throttleRatio returns 0 (len < 2), and 0 <
		// ThrottleRecover clears the latch. No stale pre-reset oldest remains.
		v2, sig2 := Decide(st, Sample{Timestamp: base.Add(20 * time.Second), NrPeriods: 50, NrThrottled: 5}, thresholds)
		if sig2.ThrottleFired {
			t.Fatalf("both-counter reset ThrottleFired: got true, want false (periods < 0 → ratio 0 < ThrottleRecover → latch clears)")
		}
		if v2.State != StateHealthy {
			t.Fatalf("both-counter reset State: got %q, want %q", v2.State, StateHealthy)
		}
		if len(v2.Causes) != 0 {
			t.Fatalf("both-counter reset Causes length: got %d, want 0 (no negative Cause Value)", len(v2.Causes))
		}
	})

	// Scenario B: nrThrottled-only reset (periods > 0, nrThrottled regresses).
	// nrPeriods keeps growing but nrThrottled drops below the ring's newest
	// value. Decide's clear-on-regression catches the nrThrottled regression
	// (10 < 200) and clears the ring, so the reset sample becomes the sole ring
	// entry, throttleRatio returns 0 (len < 2), and the latch clears with no
	// negative Cause Value. Without the clear, the stale oldest (1000, 100)
	// would remain and yield a negative ratio (10-100)/1100 = -0.0818.
	t.Run("nrThrottled-only reset clears latch with no negative Cause Value", func(t *testing.T) {
		st := &WindowState{}
		// Fire the latch: t=0 (1000, 100), t=10s (2000, 200) → ratio 0.1 > 0.05.
		Decide(st, Sample{Timestamp: base, NrPeriods: 1000, NrThrottled: 100}, thresholds)
		v1, sig1 := Decide(st, Sample{Timestamp: base.Add(10 * time.Second), NrPeriods: 2000, NrThrottled: 200}, thresholds)
		if v1.State != StateDegraded || !sig1.ThrottleFired {
			t.Fatalf("fire: state=%q fired=%v, want degraded/fired", v1.State, sig1.ThrottleFired)
		}
		// nrThrottled-only reset: nrPeriods grows (2000→2100, periods > 0) but
		// nrThrottled drops below the ring's newest 200 (2100, 10). The
		// clear-on-regression fires (10 < 200), the ring is reset to the single
		// new sample, throttleRatio returns 0, and the latch clears.
		v2, sig2 := Decide(st, Sample{Timestamp: base.Add(20 * time.Second), NrPeriods: 2100, NrThrottled: 10}, thresholds)
		if sig2.ThrottleFired {
			t.Fatalf("nrThrottled-only reset ThrottleFired: got true, want false (negative ratio < ThrottleRecover → latch clears)")
		}
		if v2.State != StateHealthy {
			t.Fatalf("nrThrottled-only reset State: got %q, want %q", v2.State, StateHealthy)
		}
		if len(v2.Causes) != 0 {
			t.Fatalf("nrThrottled-only reset Causes length: got %d, want 0 (no negative Cause Value)", len(v2.Causes))
		}
	})
}

// TestDecide_ThrottleFlipLatch_CounterResetClearsRing is the RED test for the
// clear-on-regression-before-append fix. The bug: Decide appended to the ring
// before checking for counter regression and did NOT clear the ring when
// counters regressed (a cgroup recreation / pod reschedule). After a counter reset,
// the stale pre-reset oldest point stayed in the ring: nr_periods delta =
// newest - oldest stayed <= 0 → throttleRatio returned 0 → latch forced CLEAR
// (a blind spot), and once fresh counters regrew past the stale oldest the
// denominator was inflated by pre-reset periods → ratio understated → throttle
// silently missed during the cgroup cold-start window where throttling is most
// likely. The fix clears the ring BEFORE appending when either counter
// regresses below the ring's newest entry, turning the 60s blind spot into a
// ~1-tick blind spot (the next fresh sample pair rebuilds the delta).
//
// Sequence (10s ticks, 60s window):
//
//  1. BUILD + FIRE — two large-counter samples whose 60s ratio is 0.1 > 0.05
//     → latch fires.
//  2. RESET — a sample whose NrPeriods and NrThrottled drop far below the
//     ring's newest (cgroup recreated). The ring is cleared; the reset sample
//     is the sole entry → ratio 0 → latch clears. The stale pre-reset oldest
//     (nrPeriods 100000) does NOT stay in the ring.
//  3. FRESH THROTTLE — one fresh post-reset sample pair at a 0.1 instantaneous
//     ratio. With the fix, the ring holds only post-reset points, so the 60s
//     ratio is 0.1 > 0.05 and the latch FIRES promptly. Without the fix, the
//     stale pre-reset oldest (nrPeriods 100000) remains, nr_periods delta is
//     -98990 <= 0, throttleRatio returns 0, and the latch STAYS CLEAR —
//     throttle silently missed.
//
// Without the clear-on-regression fix, step 3 fails: the latch is clear where
// it should be fired, and the ring still contains the stale pre-reset entry.
func TestDecide_ThrottleFlipLatch_CounterResetClearsRing(t *testing.T) {
	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()
	st := &WindowState{}

	type tick struct {
		dt  time.Duration
		nrP int64
		nrT int64
	}

	decide := func(tk tick) (Verdict, Signals) {
		return Decide(st, Sample{
			Timestamp:   base.Add(tk.dt),
			NrPeriods:   tk.nrP,
			NrThrottled: tk.nrT,
		}, thresholds)
	}

	// (1) BUILD + FIRE — large pre-reset counters. t=0 (100000, 5000),
	// t=10s (110000, 6000). 60s ratio = (6000-5000)/(110000-100000) =
	// 1000/10000 = 0.1 > 0.05 → latch fires.
	decide(tick{0, 100000, 5000})
	v1, sig1 := decide(tick{10 * time.Second, 110000, 6000})
	if v1.State != StateDegraded || !sig1.ThrottleFired {
		t.Fatalf("fire: state=%q fired=%v, want degraded/fired (60s ratio 0.1 > 0.05)", v1.State, sig1.ThrottleFired)
	}

	// (2) RESET — cgroup recreated: NrPeriods 110000→10, NrThrottled 6000→1.
	// The clear-on-regression fires (10 < 110000), the ring is reset to the
	// sole new sample, throttleRatio returns 0 (len < 2), 0 < ThrottleRecover
	// clears the latch. The stale pre-reset oldest (nrPeriods 100000) must NOT
	// remain in the ring.
	v2, sig2 := decide(tick{20 * time.Second, 10, 1})
	if sig2.ThrottleFired {
		t.Fatalf("reset ThrottleFired: got true, want false (ring cleared → ratio 0 < ThrottleRecover → latch clears)")
	}
	if v2.State != StateHealthy {
		t.Fatalf("reset State: got %q, want %q", v2.State, StateHealthy)
	}
	if len(st.throttleRing) != 1 {
		t.Fatalf("reset ring length: got %d, want 1 (stale pre-reset entries cleared; only the fresh reset sample remains)", len(st.throttleRing))
	}
	if got := st.throttleRing[0].nrPeriods; got != 10 {
		t.Fatalf("reset ring oldest nrPeriods: got %d, want 10 (the fresh reset value, NOT the stale pre-reset 100000)", got)
	}

	// (3) FRESH THROTTLE — one fresh post-reset sample. t=30s (1010, 101):
	// +1000 periods, +100 throttled over the reset baseline (10, 1). With the
	// fix the ring holds only post-reset points: 60s ratio = (101-1)/(1010-10)
	// = 100/1000 = 0.1 > 0.05 → latch FIRES promptly (one tick after reset).
	// Without the fix the stale pre-reset oldest (nrPeriods 100000) remains,
	// periods delta = 1010-100000 = -98990 <= 0, throttleRatio returns 0, and
	// the latch STAYS CLEAR — throttle silently missed during the cold-start
	// window.
	v3, sig3 := decide(tick{30 * time.Second, 1010, 101})
	if !sig3.ThrottleFired {
		t.Fatalf("fresh-throttle ThrottleFired: got false, want true (ring cleared on reset → fresh 60s ratio 0.1 > 0.05 → latch fires promptly; without the fix the stale oldest keeps periods delta <= 0 and the latch stays clear)")
	}
	if v3.State != StateDegraded {
		t.Fatalf("fresh-throttle State: got %q, want %q (fresh 60s ratio 0.1 > 0.05)", v3.State, StateDegraded)
	}
	if len(v3.Causes) != 1 || v3.Causes[0].Kind != CauseKindThrottling {
		t.Fatalf("fresh-throttle Causes: got %+v, want one throttling cause", v3.Causes)
	}
	wantRatio := 100.0 / 1000.0
	if !floatEq(v3.Causes[0].Value, wantRatio) {
		t.Fatalf("fresh-throttle Cause Value: got %v, want %v (fresh post-reset 60s ratio, not an understated ratio inflated by pre-reset periods)", v3.Causes[0].Value, wantRatio)
	}
}
