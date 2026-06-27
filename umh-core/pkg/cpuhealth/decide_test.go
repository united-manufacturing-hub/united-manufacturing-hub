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
// Steal/pressure/host-contention/saturation are not exercised here; the
// Sample carries only throttle counters.
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

// TestDecide_PressureCause_Avg60DirectSchmitt pins the pressure starvation
// cause in Decide. Pressure is the kernel's own 60s running average
// (cpu.pressure "some avg60"), so it is thresholded DIRECTLY — no additional
// windowing/ring/p95 (unlike throttle, which needs the counter-delta ring).
// The Schmitt flip-latch is the ONLY state.
//
// Pinned behaviors:
//  1. A pressure reading above PressureHigh (0.20) FIRES a pressure cause:
//     {degraded, unknown, [pressure]} with Cause Value = PressureAvg60.
//  2. A pressure reading below PressureRecover (0.12) CLEARS the latch.
//  3. The latch HOLDS (stays degraded) for a reading in the Schmitt band
//     (0.12 <= reading <= 0.20) after firing — the band is the debounce.
//  4. When throttle is ALSO firing, BOTH causes appear in the list (throttle
//     and pressure can co-fire).
//  5. A transient pressure spike at 0.21 that is immediately followed by a
//     band reading fires and then holds (the latch requires crossing
//     PressureHigh to fire; once fired it holds until < PressureRecover) —
//     the Schmitt band IS the debounce, since the kernel already smoothed
//     over 60s.
func TestDecide_PressureCause_Avg60DirectSchmitt(t *testing.T) {
	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()
	st := &WindowState{}

	decide := func(dt time.Duration, pressureAvg60 float64) (Verdict, Signals) {
		return Decide(st, Sample{
			Timestamp:     base.Add(dt),
			PressureAvg60: pressureAvg60,
		}, thresholds)
	}

	// (1) FIRE — a single pressure reading at 0.21 (> PressureHigh 0.20) fires
	// the pressure cause: degraded, unknown attribution, one pressure cause
	// whose Value is the raw avg60 (NOT a re-percentiled value).
	v1, sig1 := decide(10*time.Second, 0.21)
	if v1.State != StateDegraded {
		t.Fatalf("fire State: got %q, want %q (pressure 0.21 > PressureHigh 0.20 → degraded)", v1.State, StateDegraded)
	}
	if v1.Attribution != AttributionUnknown {
		t.Fatalf("fire Attribution: got %q, want %q (pressure is internal → unknown)", v1.Attribution, AttributionUnknown)
	}
	if !sig1.PressureFired {
		t.Fatalf("fire PressureFired: got false, want true (latch fires above PressureHigh)")
	}
	if len(v1.Causes) != 1 {
		t.Fatalf("fire Causes length: got %d, want 1 (single pressure cause)", len(v1.Causes))
	}
	if v1.Causes[0].Kind != CauseKindPressure {
		t.Fatalf("fire Cause Kind: got %q, want %q", v1.Causes[0].Kind, CauseKindPressure)
	}
	if !floatEq(v1.Causes[0].Value, 0.21) {
		t.Fatalf("fire Cause Value: got %v, want 0.21 (the raw avg60, thresholded directly — no p95)", v1.Causes[0].Value)
	}

	// (2) HOLD — a reading at 0.15 (in the Schmitt band 0.12..0.20) must NOT
	// clear the latch: the latch holds fired. (The kernel already smoothed over
	// 60s; the band is the only debounce.)
	v2, sig2 := decide(20*time.Second, 0.15)
	if v2.State != StateDegraded {
		t.Fatalf("hold State: got %q, want %q (pressure 0.15 is in the Schmitt band; latch holds fired)", v2.State, StateDegraded)
	}
	if !sig2.PressureFired {
		t.Fatalf("hold PressureFired: got false, want true (latch holds in the band; only drops below PressureRecover clears)")
	}
	if len(v2.Causes) != 1 || v2.Causes[0].Kind != CauseKindPressure {
		t.Fatalf("hold Causes: got %+v, want one pressure cause", v2.Causes)
	}

	// (3) CLEAR — a reading at 0.10 (< PressureRecover 0.12) clears the latch:
	// healthy, PressureFired false, no causes.
	v3, sig3 := decide(30*time.Second, 0.10)
	if v3.State != StateHealthy {
		t.Fatalf("clear State: got %q, want %q (pressure 0.10 < PressureRecover 0.12 → latch clears)", v3.State, StateHealthy)
	}
	if sig3.PressureFired {
		t.Fatalf("clear PressureFired: got true, want false (latch clears below PressureRecover)")
	}
	if len(v3.Causes) != 0 {
		t.Fatalf("clear Causes length: got %d, want 0", len(v3.Causes))
	}

	// (4) CO-FIRE with throttle — when throttle is ALSO firing, BOTH causes
	// appear in the list (throttle and pressure can co-fire). Feed a sample
	// with both a sustained throttle ratio above ThrottleHigh and a pressure
	// reading above PressureHigh. Two ticks are needed to build the throttle
	// ring delta; the first tick establishes the baseline.
	st2 := &WindowState{}
	tNow := base
	// Baseline throttle counters.
	Decide(st2, Sample{
		Timestamp:     tNow,
		NrPeriods:     1000,
		NrThrottled:   10,
		PressureAvg60: 0.0,
	}, thresholds)
	tNow = tNow.Add(10 * time.Second)
	// Second tick: +1000 periods, +100 throttled → 60s ratio 0.10 > 0.05
	// (throttle fires), AND pressure 0.25 > 0.20 (pressure fires).
	v4, sig4 := Decide(st2, Sample{
		Timestamp:     tNow,
		NrPeriods:     2000,
		NrThrottled:   110,
		PressureAvg60: 0.25,
	}, thresholds)
	if v4.State != StateDegraded {
		t.Fatalf("cofire State: got %q, want %q (both throttle and pressure fire)", v4.State, StateDegraded)
	}
	if !sig4.ThrottleFired {
		t.Fatalf("cofire ThrottleFired: got false, want true (throttle ratio 0.10 > 0.05)")
	}
	if !sig4.PressureFired {
		t.Fatalf("cofire PressureFired: got false, want true (pressure 0.25 > 0.20)")
	}
	if len(v4.Causes) != 2 {
		t.Fatalf("cofire Causes length: got %d, want 2 (both throttle and pressure causes present)", len(v4.Causes))
	}
	kinds := map[CauseKind]bool{}
	for _, c := range v4.Causes {
		kinds[c.Kind] = true
	}
	if !kinds[CauseKindThrottling] || !kinds[CauseKindPressure] {
		t.Fatalf("cofire Causes: got %+v, want both throttling and pressure present", v4.Causes)
	}

	// (5) BOUNDARY — the latch uses strict `>` (fire) and strict `<` (clear), so
	// a reading of exactly PressureHigh (0.20) must NOT fire from a healthy
	// latch, and a reading of exactly PressureRecover (0.12) must NOT clear from
	// a fired latch. The hold band is [0.12, 0.20] inclusive on both ends.
	// Pinning the strict-comparison contract prevents a future refactor flipping
	// `>` to `>=` (or `<` to `<=`) from silently shifting the band.
	stB := &WindowState{}
	// Exactly PressureHigh (0.20) from healthy: must NOT fire.
	vb1, sigb1 := Decide(stB, Sample{
		Timestamp:     base.Add(40 * time.Second),
		PressureAvg60: thresholds.PressureHigh, // 0.20
	}, thresholds)
	if vb1.State != StateHealthy {
		t.Fatalf("boundary-high State: got %q, want %q (exactly PressureHigh 0.20, strict `>` must NOT fire)", vb1.State, StateHealthy)
	}
	if sigb1.PressureFired {
		t.Fatalf("boundary-high PressureFired: got true, want false (strict `>`: 0.20 is not > 0.20)")
	}
	if len(vb1.Causes) != 0 {
		t.Fatalf("boundary-high Causes: got %+v, want none (0.20 does not fire)", vb1.Causes)
	}
	// Fire the latch for the next boundary check.
	Decide(stB, Sample{
		Timestamp:     base.Add(50 * time.Second),
		PressureAvg60: 0.21,
	}, thresholds)
	if !stB.pressureFired {
		t.Fatalf("boundary setup: latch should have fired at 0.21")
	}
	// Exactly PressureRecover (0.12) from fired: must NOT clear (strict `<`).
	vb2, sigb2 := Decide(stB, Sample{
		Timestamp:     base.Add(60 * time.Second),
		PressureAvg60: thresholds.PressureRecover, // 0.12
	}, thresholds)
	if vb2.State != StateDegraded {
		t.Fatalf("boundary-recover State: got %q, want %q (exactly PressureRecover 0.12, strict `<` must NOT clear)", vb2.State, StateDegraded)
	}
	if !sigb2.PressureFired {
		t.Fatalf("boundary-recover PressureFired: got false, want true (strict `<`: 0.12 is not < 0.12, latch holds fired)")
	}

	// (6) NaN GUARD — a NaN PressureAvg60 is clamped to 0 before thresholding.
	// On a FRESH (healthy) latch, NaN must NOT fire (treated as 0, which is <
	// PressureRecover → latch stays/clears). On an already-FIRED latch, NaN
	// must CLEAR (treated as 0 < PressureRecover) — the pre-clamp bug stuck the
	// latch fired indefinitely. NaN must also never leak as a Cause Value.
	nan := math.NaN()
	stN := &WindowState{}
	// Fresh latch + NaN: stays healthy, no cause, no NaN Value.
	vn1, sign1 := Decide(stN, Sample{
		Timestamp:     base.Add(70 * time.Second),
		PressureAvg60: nan,
	}, thresholds)
	if vn1.State != StateHealthy {
		t.Fatalf("nan-fresh State: got %q, want %q (NaN clamped to 0, does not fire)", vn1.State, StateHealthy)
	}
	if sign1.PressureFired {
		t.Fatalf("nan-fresh PressureFired: got true, want false (NaN→0 does not fire)")
	}
	if len(vn1.Causes) != 0 {
		t.Fatalf("nan-fresh Causes: got %+v, want none", vn1.Causes)
	}
	// Fire the latch, then feed NaN: must CLEAR (NaN→0 < PressureRecover).
	Decide(stN, Sample{
		Timestamp:     base.Add(80 * time.Second),
		PressureAvg60: 0.21,
	}, thresholds)
	if !stN.pressureFired {
		t.Fatalf("nan setup: latch should have fired at 0.21")
	}
	vn2, sign2 := Decide(stN, Sample{
		Timestamp:     base.Add(90 * time.Second),
		PressureAvg60: nan,
	}, thresholds)
	if vn2.State != StateHealthy {
		t.Fatalf("nan-fired State: got %q, want %q (NaN clamped to 0 < PressureRecover → latch clears)", vn2.State, StateHealthy)
	}
	if sign2.PressureFired {
		t.Fatalf("nan-fired PressureFired: got true, want false (NaN→0 clears the latch, does not stick fired)")
	}
	if len(vn2.Causes) != 0 {
		t.Fatalf("nan-fired Causes: got %+v, want none (NaN does not leak as a Cause Value)", vn2.Causes)
	}

	// (7) +INF GUARD — a +Inf PressureAvg60 is clamped to 0 before thresholding.
	// The `!(p >= 0)` idiom catches NaN and negatives but NOT +Inf (`+Inf >= 0`
	// is true), so without the math.IsInf(p, 1) guard a +Inf would leak into
	// Cause.Value and break json.Marshal of the whole Verdict
	// (`json: unsupported value: +Inf`). On a FRESH (healthy) latch, +Inf must
	// NOT fire (clamped to 0, which is < PressureRecover). On an already-FIRED
	// latch, +Inf must CLEAR (clamped to 0 < PressureRecover). +Inf must also
	// never leak as a Cause Value.
	inf := math.Inf(1)
	stI := &WindowState{}
	// Fresh latch + +Inf: stays healthy, no cause, no +Inf Value.
	vi1, sigi1 := Decide(stI, Sample{
		Timestamp:     base.Add(110 * time.Second),
		PressureAvg60: inf,
	}, thresholds)
	if vi1.State != StateHealthy {
		t.Fatalf("inf-fresh State: got %q, want %q (+Inf clamped to 0, does not fire)", vi1.State, StateHealthy)
	}
	if sigi1.PressureFired {
		t.Fatalf("inf-fresh PressureFired: got true, want false (+Inf→0 does not fire)")
	}
	if len(vi1.Causes) != 0 {
		t.Fatalf("inf-fresh Causes: got %+v, want none", vi1.Causes)
	}
	// Fire the latch, then feed +Inf: must CLEAR (+Inf→0 < PressureRecover).
	Decide(stI, Sample{
		Timestamp:     base.Add(120 * time.Second),
		PressureAvg60: 0.21,
	}, thresholds)
	if !stI.pressureFired {
		t.Fatalf("inf setup: latch should have fired at 0.21")
	}
	vi2, sigi2 := Decide(stI, Sample{
		Timestamp:     base.Add(130 * time.Second),
		PressureAvg60: inf,
	}, thresholds)
	if vi2.State != StateHealthy {
		t.Fatalf("inf-fired State: got %q, want %q (+Inf clamped to 0 < PressureRecover → latch clears)", vi2.State, StateHealthy)
	}
	if sigi2.PressureFired {
		t.Fatalf("inf-fired PressureFired: got true, want false (+Inf→0 clears the latch)")
	}
	if len(vi2.Causes) != 0 {
		t.Fatalf("inf-fired Causes: got %+v, want none (+Inf does not leak as a Cause Value)", vi2.Causes)
	}

	// (8) DEFAULT-ZERO — a zero PressureAvg60 (the unset/zero-value case) never
	// fires: 0 < PressureRecover so the latch is forced CLEAR on every tick.
	stZ := &WindowState{}
	vz, sigz := Decide(stZ, Sample{
		Timestamp:     base.Add(100 * time.Second),
		PressureAvg60: 0.0,
	}, thresholds)
	if vz.State != StateHealthy {
		t.Fatalf("zero State: got %q, want %q (0 < PressureRecover, never fires)", vz.State, StateHealthy)
	}
	if sigz.PressureFired {
		t.Fatalf("zero PressureFired: got true, want false (0 never fires)")
	}
	if len(vz.Causes) != 0 {
		t.Fatalf("zero Causes: got %+v, want none", vz.Causes)
	}
}

// TestDecide_StealCause_VirtualizedRingP95Schmitt pins the steal starvation
// cause in Decide — an EXTERNAL/host-attributed cause. Steal is the
// fraction of wall-time the hypervisor gave this VM's vCPU to other VMs
// (Sample.StealFraction, 0.0-1.0), read from the 8th field of /proc/stat's
// `cpu ` line. Unlike throttle (counter-delta) and pressure (kernel-avg60
// direct), steal uses a 60s RING of per-tick StealFraction samples reduced by
// p95 (near-worst-of-window via nearest-rank: a sustained spike fires, a
// single isolated spike is absorbed). A THIRD Schmitt flip-latch fires when
// the p95 > StealHigh (0.10) and clears only when it drops below StealRecover
// (0.06); between the marks it holds.
//
// Steal is only readable on a virtualized box. Sample.Virtualized is set by
// the sampler from /proc/cpuinfo's hypervisor flag. When Virtualized is
// FALSE, Decide skips the steal ring entirely — it does not append, does not
// evaluate the latch, and does not reset the ring or clear the latch (steal
// is structurally 0 on bare metal, so reading 0 there is the absence of a
// signal, not evidence of a healthy host). When Virtualized is TRUE, steal is
// processed even when it reads 0 (0 on a VM = the host is serving us fine →
// contributes to healthy, not a dead-zone). Readable iff virtualized, not
// "/proc/stat parses."
//
// When steal fires the verdict is {degraded, HOST, [steal]} with the Cause
// Value = the steal p95. HOST is the external attribution. When steal and an
// internal cause (throttle/pressure) BOTH fire, attribution is HOST
// (external-steal currently wins attribution when it fires; the full
// dominance ordering is not yet implemented) and causes lists both.
//
// Pinned behaviors:
//
//  1. FIRE — virtualized, 20 sustained ticks at 0.15 (> StealHigh 0.10) →
//     degraded, HOST, one steal cause whose Value is the p95 (0.15: all
//     samples identical → p95 is 0.15 under any reduction method).
//  2. BARE-METAL PREDICATE — Virtualized=false with a nonzero StealFraction
//     (0.15) on every tick: steal is NOT processed → healthy, no steal cause,
//     StealFired false, AND the steal ring is reset (remains empty).
//     This is the spec's "readable iff virtualized, not /proc/stat parses."
//  3. VM ZERO → HEALTHY — Virtualized=true, StealFraction=0.0 for 20 ticks:
//     steal IS processed (ring appended, non-empty) but 0 < StealRecover so
//     the latch stays clear and the verdict is healthy. A quiescent VM is
//     healthy, not a dead-zone — the contrast with case 2 is the predicate.
//  4. TRANSIENT SPIKE ABSORBED — Virtualized=true, 20 ticks at 0.0 then ONE
//     tick at 0.15: with 21 samples the p95 (near-worst-of-window via
//     nearest-rank) stays below StealHigh 0.10 (nearest-rank p95 of twenty 0s
//     + one 0.15 = 0.0), so the latch does NOT fire and the verdict is
//     healthy. The p95 window absorbs a single isolated spike; a sustained
//     spike (case 1) fires.
//  5. CO-FIRE WITH THROTTLE → HOST — steal firing AND throttle firing on the
//     same sample: both causes appear, attribution is HOST (external wins
//     attribution when it fires; the full dominance ordering is not yet
//     implemented).
func TestDecide_StealCause_VirtualizedRingP95Schmitt(t *testing.T) {
	base := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// feedSteal pushes n ticks of the given StealFraction onto st's steal ring
	// at 1s spacing (well inside the 60s window), Virtualized=virt, returning
	// the final verdict+signals. NrPeriods/NrThrottled are left zero so the
	// throttle ring (which needs a 2-point delta) stays quiet unless a
	// throttle baseline is established by the caller.
	feedSteal := func(st *WindowState, n int, steal float64, virt bool, startOff time.Duration) (Verdict, Signals) {
		var v Verdict
		var s Signals
		for i := 0; i < n; i++ {
			v, s = Decide(st, Sample{
				Timestamp:     base.Add(startOff).Add(time.Duration(i) * time.Second),
				StealFraction: steal,
				Virtualized:   virt,
			}, thresholds)
		}
		return v, s
	}

	// (1) FIRE — 20 sustained ticks at 0.15 on a virtualized box: p95 = 0.15
	// (> StealHigh 0.10) → latch fires, degraded, HOST, one steal cause with
	// Value = the p95.
	st1 := &WindowState{}
	v1, sig1 := feedSteal(st1, 20, 0.15, true, 0)
	if v1.State != StateDegraded {
		t.Fatalf("fire State: got %q, want %q (steal p95 0.15 > StealHigh 0.10 → degraded)", v1.State, StateDegraded)
	}
	if v1.Attribution != AttributionHost {
		t.Fatalf("fire Attribution: got %q, want %q (steal is external → host, the first non-unknown attribution)", v1.Attribution, AttributionHost)
	}
	if !sig1.StealFired {
		t.Fatalf("fire StealFired: got false, want true (p95 0.15 > StealHigh 0.10 fires the latch)")
	}
	if len(v1.Causes) != 1 {
		t.Fatalf("fire Causes length: got %d, want 1 (single steal cause)", len(v1.Causes))
	}
	if v1.Causes[0].Kind != CauseKindSteal {
		t.Fatalf("fire Cause Kind: got %q, want %q", v1.Causes[0].Kind, CauseKindSteal)
	}
	if !floatEq(v1.Causes[0].Value, 0.15) {
		t.Fatalf("fire Cause Value: got %v, want 0.15 (the steal p95; 20 identical samples → p95 is 0.15 under any reduction)", v1.Causes[0].Value)
	}

	// (2) BARE-METAL PREDICATE — Virtualized=false with a nonzero
	// StealFraction (0.15) on every tick: steal is NOT processed. The latch
	// stays unfired, no steal cause, verdict healthy, AND the steal ring is
	// NOT appended (remains empty — the structural-0-on-bare-metal case is
	// the absence of a signal, not evidence of health).
	st2 := &WindowState{}
	v2, sig2 := feedSteal(st2, 20, 0.15, false, 100*time.Second)
	if v2.State != StateHealthy {
		t.Fatalf("baremetal State: got %q, want %q (Virtualized=false → steal not processed → healthy)", v2.State, StateHealthy)
	}
	if sig2.StealFired {
		t.Fatalf("baremetal StealFired: got true, want false (steal not processed on bare metal)")
	}
	if len(v2.Causes) != 0 {
		t.Fatalf("baremetal Causes: got %+v, want none (no steal cause when Virtualized=false)", v2.Causes)
	}
	if len(st2.stealRing) != 0 {
		t.Fatalf("baremetal stealRing length: got %d, want 0 (ring must NOT be appended when Virtualized=false — steal is not a readable signal on bare metal)", len(st2.stealRing))
	}

	// (3) VM ZERO → HEALTHY — Virtualized=true, StealFraction=0.0 for 20
	// ticks: steal IS processed (ring appended, non-empty) but 0 <
	// StealRecover so the latch stays clear and the verdict is healthy. A
	// quiescent VM is healthy, NOT a dead-zone. The contrast with case 2
	// (same zeros, but Virtualized=false → ring empty) is the readability
	// predicate: on a VM the 0 is a real "host is serving us" signal.
	st3 := &WindowState{}
	v3, sig3 := feedSteal(st3, 20, 0.0, true, 200*time.Second)
	if v3.State != StateHealthy {
		t.Fatalf("vm-zero State: got %q, want %q (steal 0 on a VM is processed but does not fire → healthy)", v3.State, StateHealthy)
	}
	if sig3.StealFired {
		t.Fatalf("vm-zero StealFired: got true, want false (0 < StealRecover 0.06, latch stays clear)")
	}
	if len(v3.Causes) != 0 {
		t.Fatalf("vm-zero Causes: got %+v, want none", v3.Causes)
	}
	if len(st3.stealRing) == 0 {
		t.Fatalf("vm-zero stealRing length: got 0, want non-zero (steal IS processed on a VM even at 0 — 0 is a real healthy signal, not a dead-zone)")
	}

	// (4) TRANSIENT SPIKE ABSORBED — 20 ticks at 0.0 then ONE tick at 0.15
	// (Virtualized=true). With 21 samples the p95 stays below StealHigh 0.10
	// (nearest-rank p95 of twenty 0s + one 0.15 = 0.0), so the latch does NOT
	// fire and the verdict is healthy. The p95 window absorbs a single
	// isolated spike; a sustained spike (case 1) fires.
	st4 := &WindowState{}
	v4a, _ := feedSteal(st4, 20, 0.0, true, 300*time.Second)
	if v4a.State != StateHealthy {
		t.Fatalf("spike-pre State: got %q, want %q (20 zero ticks → healthy)", v4a.State, StateHealthy)
	}
	v4, sig4 := Decide(st4, Sample{
		Timestamp:     base.Add(300 * time.Second).Add(20 * time.Second),
		StealFraction: 0.15,
		Virtualized:   true,
	}, thresholds)
	if v4.State != StateHealthy {
		t.Fatalf("spike State: got %q, want %q (one 0.15 spike among 20 zeros: p95 < 0.10 → does not fire)", v4.State, StateHealthy)
	}
	if sig4.StealFired {
		t.Fatalf("spike StealFired: got true, want false (single isolated spike: p95 of mostly-0 + one 0.15 stays below StealHigh)")
	}
	if len(v4.Causes) != 0 {
		t.Fatalf("spike Causes: got %+v, want none (transient spike absorbed by the p95 window)", v4.Causes)
	}

	// (5) CO-FIRE WITH THROTTLE → HOST — steal firing AND throttle firing on
	// the same sample: both causes appear, attribution is HOST (external-steal
	// wins attribution when it fires). Two ticks build the throttle ring
	// delta; the steal ring is filled with sustained-high steal so its p95
	// fires too.
	st5 := &WindowState{}
	// Baseline throttle counters at t0.
	Decide(st5, Sample{
		Timestamp:     base.Add(400 * time.Second),
		NrPeriods:     1000,
		NrThrottled:   10,
		StealFraction: 0.0,
		Virtualized:   true,
	}, thresholds)
	// 18 more sustained-high-steal ticks to build the steal p95 (all 0.50).
	// The intermediate ticks carry forward NrPeriods/NrThrottled (1000+i, 10+i)
	// so they do NOT regress below the t0 baseline (1000, 10) and trigger the
	// counter-regression ring-clear; with zero-value counters the ring would be
	// rebuilt from (0,0) and the actual throttle ratio would be 0.055, not the
	// intended 0.10. Steal is 0.50 (well above StealHigh 0.10) so its severity
	// (0.50-0.10)/0.90 = 0.44 robustly dominates throttle sev 0.053 — the
	// attribution is HOST by a wide margin, not a near-tie.
	for i := 1; i < 19; i++ {
		Decide(st5, Sample{
			Timestamp:     base.Add(400 * time.Second).Add(time.Duration(i) * time.Second),
			NrPeriods:     1000 + int64(i),
			NrThrottled:   10 + int64(i),
			StealFraction: 0.50,
			Virtualized:   true,
		}, thresholds)
	}
	// Final tick: +1000 periods, +100 throttled → 60s throttle ratio
	// (110-10)/(2000-1000) = 0.10 > ThrottleHigh 0.05 (throttle fires), AND
	// steal 0.50 continues (p95 > StealHigh → steal fires).
	v5, sig5 := Decide(st5, Sample{
		Timestamp:     base.Add(400 * time.Second).Add(19 * time.Second),
		NrPeriods:     2000,
		NrThrottled:   110,
		StealFraction: 0.50,
		Virtualized:   true,
	}, thresholds)
	if v5.State != StateDegraded {
		t.Fatalf("cofire State: got %q, want %q (both steal and throttle fire)", v5.State, StateDegraded)
	}
	if v5.Attribution != AttributionHost {
		t.Fatalf("cofire Attribution: got %q, want %q (external-steal is dominant: steal sev %v > throttle sev %v → host)", v5.Attribution, AttributionHost, severity(0.50, thresholds.StealHigh), severity(0.10, thresholds.ThrottleHigh))
	}
	if !sig5.StealFired {
		t.Fatalf("cofire StealFired: got false, want true (sustained steal 0.50 → p95 > 0.10)")
	}
	if !sig5.ThrottleFired {
		t.Fatalf("cofire ThrottleFired: got false, want true (throttle ratio 0.10 > 0.05)")
	}
	if len(v5.Causes) != 2 {
		t.Fatalf("cofire Causes length: got %d, want 2 (both steal and throttling present)", len(v5.Causes))
	}
	kinds := map[CauseKind]bool{}
	var throttleVal float64
	for _, c := range v5.Causes {
		kinds[c.Kind] = true
		if c.Kind == CauseKindThrottling {
			throttleVal = c.Value
		}
	}
	if !kinds[CauseKindSteal] || !kinds[CauseKindThrottling] {
		t.Fatalf("cofire Causes: got %+v, want both steal and throttling present", v5.Causes)
	}
	wantThrottle := 100.0 / 1000.0
	if !floatEq(throttleVal, wantThrottle) {
		t.Fatalf("cofire throttle Cause Value: got %v, want %v (clean two-point delta 0.10, not the 0.055 a zero-intermediate ring-clear produces)", throttleVal, wantThrottle)
	}
}

// TestDecide_StealCause_FireThenRecover is the round-trip test for the steal
// Schmitt latch's CLEAR branch (StealRecover). Without a recover branch, once
// the steal latch fires it is permanently stuck at {degraded, host} until
// restart — the latch only ever sets stealFired=true and never clears it. This
// test feeds sustained-high steal to FIRE the latch, then sustained-low steal
// for long enough that the 60s window's p95 drops below StealRecover (0.06),
// and asserts the latch CLEARS (healthy, no steal cause). This test MUST fail
// when FIX 1 (the StealRecover clear branch) is reverted.
func TestDecide_StealCause_FireThenRecover(t *testing.T) {
	base := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()
	st := &WindowState{}

	// FIRE — 20 sustained ticks at 0.15 (> StealHigh 0.10) at 1s spacing
	// (t=0..t=19, all within the 60s window). p95 = 0.15 → latch fires.
	for i := 0; i < 20; i++ {
		Decide(st, Sample{
			Timestamp:     base.Add(time.Duration(i) * time.Second),
			StealFraction: 0.15,
			Virtualized:   true,
		}, thresholds)
	}
	vFire, sigFire := Decide(st, Sample{
		Timestamp:     base.Add(20 * time.Second),
		StealFraction: 0.15,
		Virtualized:   true,
	}, thresholds)
	if vFire.State != StateDegraded {
		t.Fatalf("fire State: got %q, want %q (sustained steal 0.15 → p95 > StealHigh 0.10)", vFire.State, StateDegraded)
	}
	if vFire.Attribution != AttributionHost {
		t.Fatalf("fire Attribution: got %q, want %q", vFire.Attribution, AttributionHost)
	}
	if !sigFire.StealFired {
		t.Fatalf("fire StealFired: got false, want true (latch must fire above StealHigh)")
	}

	// RECOVER — feed sustained-low steal (0.0) at 10s spacing so that after
	// enough ticks the 60s window contains only low samples. At 10s spacing:
	// the high-steal samples (t=0..t=20) age out once they exceed 60s from the
	// newest timestamp. By t=130s, cutoff = t=70s, so all high-steal samples
	// (t<=20) are pruned; the window holds only 0.0 samples from t=70 onward.
	// p95 of all-zeros = 0.0 < StealRecover 0.06 → latch CLEARS.
	for i := 0; i < 12; i++ {
		Decide(st, Sample{
			Timestamp:     base.Add(time.Duration(30+i*10) * time.Second),
			StealFraction: 0.0,
			Virtualized:   true,
		}, thresholds)
	}
	// Final tick at t=160s: cutoff = t=100s; window holds only 0.0 samples.
	vClear, sigClear := Decide(st, Sample{
		Timestamp:     base.Add(160 * time.Second),
		StealFraction: 0.0,
		Virtualized:   true,
	}, thresholds)
	if vClear.State != StateHealthy {
		t.Fatalf("recover State: got %q, want %q (p95 dropped below StealRecover 0.06 → latch clears)", vClear.State, StateHealthy)
	}
	if sigClear.StealFired {
		t.Fatalf("recover StealFired: got true, want false (latch must clear below StealRecover; without FIX 1 the latch never clears)")
	}
	if len(vClear.Causes) != 0 {
		t.Fatalf("recover Causes length: got %d, want 0 (no steal cause once latch clears)", len(vClear.Causes))
	}
}

// TestDecide_StealCause_RingPruning pins the 60s pruning of the steal ring
// (FIX 2). Without pruning, stealPoint.ts is dead data — the ring is appended
// every virtualized tick but never trimmed, so the p95 becomes an all-history
// p95 (unbounded growth, old high-steal entries never drop). This test feeds
// enough virtualized samples spanning > 60s of timestamps that old entries
// should be pruned, asserts the ring length stays bounded, AND asserts that a
// stale high-steal entry from > 60s ago does NOT keep the p95 elevated after
// it ages out. This test MUST fail when FIX 2 (pruning) is reverted.
func TestDecide_StealCause_RingPruning(t *testing.T) {
	base := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// Scenario A: ring length is bounded by pruning. Feed 10s-spaced
	// virtualized samples for 130s (14 samples). With a 60s window, the ring
	// should hold at most ~7 entries (t=70..t=130). Without pruning it grows to
	// 14.
	stA := &WindowState{}
	for i := 0; i < 14; i++ {
		Decide(stA, Sample{
			Timestamp:     base.Add(time.Duration(i*10) * time.Second),
			StealFraction: 0.0,
			Virtualized:   true,
		}, thresholds)
	}
	if len(stA.stealRing) > 8 {
		t.Fatalf("ring length after pruning: got %d, want <= 8 (60s window at 10s spacing = ~7 entries; without FIX 2 the ring grows to 14)", len(stA.stealRing))
	}

	// Scenario B: a stale high-steal entry from > 60s ago does NOT keep the p95
	// elevated after it ages out. Feed a burst of high-steal samples early
	// (t=0..t=10), then enough low-steal samples that the high entries age out
	// of the 60s window AND the ring holds >= 2 low samples (so the latch is
	// actually evaluated and clears). At t=80s, cutoff = t=20s, so the high-steal
	// samples (t=0, t=10) are pruned. Feed low samples at t=70 and t=80 so the
	// window holds 2 zero entries → p95 = 0.0 < StealRecover → latch clears.
	stB := &WindowState{}
	// High-steal burst at t=0 and t=10 (2 samples, 0.15 each). Ring size 2 >=
	// floor → latch fires.
	Decide(stB, Sample{
		Timestamp:     base,
		StealFraction: 0.15,
		Virtualized:   true,
	}, thresholds)
	Decide(stB, Sample{
		Timestamp:     base.Add(10 * time.Second),
		StealFraction: 0.15,
		Virtualized:   true,
	}, thresholds)
	if !stB.stealFired {
		t.Fatalf("setup: latch should have fired after 2 high-steal samples")
	}
	// Low-steal samples at t=70 and t=80 (10s spacing). At t=80 cutoff=t=20,
	// so the two 0.15 entries (t=0, t=10) are pruned. Window = [t=70, t=80],
	// two 0.0 samples. p95 = 0.0 < StealRecover → latch CLEARS.
	Decide(stB, Sample{
		Timestamp:     base.Add(70 * time.Second),
		StealFraction: 0.0,
		Virtualized:   true,
	}, thresholds)
	vB, sigB := Decide(stB, Sample{
		Timestamp:     base.Add(80 * time.Second),
		StealFraction: 0.0,
		Virtualized:   true,
	}, thresholds)
	if sigB.StealFired {
		t.Fatalf("aged-out high-steal StealFired: got true, want false (stale 0.15 entries from t=0/t=10 pruned at t=80; p95 of 2 zeros = 0.0 < StealRecover; without FIX 2 they linger and keep the p95 elevated)")
	}
	if vB.State != StateHealthy {
		t.Fatalf("aged-out high-steal State: got %q, want %q (stale entries pruned → p95 low → healthy)", vB.State, StateHealthy)
	}
}

// TestDecide_StealCause_SmallNFloor pins the small-N degeneracy floor (FIX 3).
// The nearest-rank p95 with N=1 returns the single sample's value, so without
// a floor a first-tick high-steal sample (0.15) fires the latch immediately —
// contradicting "sustained fires, isolated absorbed." A 2-sample floor (matching
// throttle's two-point delta floor) prevents a single first-tick spike from
// firing. This test MUST fail when FIX 3 (the floor) is reverted.
func TestDecide_StealCause_SmallNFloor(t *testing.T) {
	base := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()
	st := &WindowState{}

	// A single virtualized first-tick high-steal sample (0.15 > StealHigh 0.10).
	// Ring size = 1, below the 2-sample floor, so the latch must NOT evaluate
	// and must NOT fire.
	v, sig := Decide(st, Sample{
		Timestamp:     base,
		StealFraction: 0.15,
		Virtualized:   true,
	}, thresholds)
	if sig.StealFired {
		t.Fatalf("small-N StealFired: got true, want false (ring size 1 < floor 2; a single first-tick spike must not fire; without FIX 3 the N=1 p95=0.15 fires immediately)")
	}
	if v.State != StateHealthy {
		t.Fatalf("small-N State: got %q, want %q (ring size 1 < floor 2 → latch not evaluated → healthy)", v.State, StateHealthy)
	}
	if len(v.Causes) != 0 {
		t.Fatalf("small-N Causes length: got %d, want 0", len(v.Causes))
	}
	if len(st.stealRing) != 1 {
		t.Fatalf("small-N ring length: got %d, want 1 (the sample IS appended; only the latch evaluation is skipped)", len(st.stealRing))
	}
}

// TestDecide_HostContentionCause_DemandGateFireThenClear pins the invariant
// that the host-contention latch clears when the co-firing demand signal
// (pressure) drops, even while the host stays busy. Without the
// demand-gate-drop clear branch, a busy host plus a light UMH workload would
// be indistinguishable from "we have a light workload" and the latch could
// stick fired after a transient pressure spike, permanently pinning
// {degraded, host}. The demand-gate-drop clear is the load-bearing guard
// against that false-degrade; do not remove it.
func TestDecide_HostContentionCause_DemandGateFireThenClear(t *testing.T) {
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	const hostBusyCores = 3.2
	const ourUsageCores = 0.5
	const wantContentionCores = 2.7

	feedHost := func(st *WindowState, n int, pressure float64, startOff time.Duration) (Verdict, Signals) {
		var v Verdict
		var s Signals
		for i := 0; i < n; i++ {
			v, s = Decide(st, Sample{
				Timestamp:     base.Add(startOff).Add(time.Duration(i) * time.Second),
				UsageCores:    ourUsageCores,
				HostBusyCores: hostBusyCores,
				LogicalCpus:   4.0,
				PressureAvg60: pressure,
			}, thresholds)
		}
		return v, s
	}

	// (A) NEGATIVE — demand gate closed: busy host but no pressure firing
	// → host-contention must NOT fire → healthy.
	st := &WindowState{}
	vA, sigA := feedHost(st, 5, 0.0, 0)
	if vA.State != StateHealthy {
		t.Fatalf("negative State: got %q, want %q (busy host + low usage but NO demand signal → light workload → healthy; host-contention must NOT fire without a demand signal)", vA.State, StateHealthy)
	}
	if sigA.HostContentionFired {
		t.Fatalf("negative HostContentionFired: got true, want false (demand gate closed → host-contention must not fire)")
	}
	for _, c := range vA.Causes {
		if c.Kind == CauseKindHostContention {
			t.Fatalf("negative Causes: host-contention must NOT appear without a demand signal (got %+v)", vA.Causes)
		}
	}

	// (B) FIRE — demand gate opens: same busy host, pressure now firing →
	// host-contention fires → degraded, HOST, causes include host-contention
	// (value = contention cores) AND pressure.
	vB, sigB := feedHost(st, 5, 0.30, 5*time.Second)
	if vB.State != StateDegraded {
		t.Fatalf("fire State: got %q, want %q (busy host + pressure firing → host-contention fires → degraded)", vB.State, StateDegraded)
	}
	if vB.Attribution != AttributionHost {
		t.Fatalf("fire Attribution: got %q, want %q (host-contention is external → host)", vB.Attribution, AttributionHost)
	}
	if !sigB.HostContentionFired {
		t.Fatalf("fire HostContentionFired: got false, want true (host_busy 0.80 > 0.70 AND pressure firing → host-contention latch fires)")
	}
	if !sigB.PressureFired {
		t.Fatalf("fire PressureFired: got false, want true (PressureAvg60 0.30 > PressureHigh 0.20 → pressure co-fires)")
	}
	var hostContentionVal float64
	var hasHostContention, hasPressure bool
	for _, c := range vB.Causes {
		if c.Kind == CauseKindHostContention {
			hasHostContention = true
			hostContentionVal = c.Value
		}
		if c.Kind == CauseKindPressure {
			hasPressure = true
		}
	}
	if !hasHostContention {
		t.Fatalf("fire Causes: host-contention MISSING from %+v (must appear when it fires)", vB.Causes)
	}
	if !hasPressure {
		t.Fatalf("fire Causes: pressure MISSING from %+v (the co-firing demand-signal cause must also appear)", vB.Causes)
	}
	if !floatEq(hostContentionVal, wantContentionCores) {
		t.Fatalf("fire host-contention Cause Value: got %v, want %v (contention_cores = max(0, HostBusyCores - UsageCores) = 3.2 - 0.5)", hostContentionVal, wantContentionCores)
	}

	// (C) CLEAR — demand gate drops, host stays busy. host-contention must
	// clear because the demand signal is gone, even though the host is still
	// busy. Without the demand-gate-drop clear branch the latch would stick
	// fired here. After clear: healthy.
	vC, sigC := feedHost(st, 5, 0.0, 10*time.Second)
	if vC.State != StateHealthy {
		t.Fatalf("clear State: got %q, want %q (demand gate dropped → host-contention clears → healthy; the host staying busy must NOT keep it fired)", vC.State, StateHealthy)
	}
	if sigC.HostContentionFired {
		t.Fatalf("clear HostContentionFired: got true, want false (demand gate dropped → latch MUST clear even though host is still busy; a fire-only latch without the demand-gate-drop clear branch would stay fired here)")
	}
	if sigC.PressureFired {
		t.Fatalf("clear PressureFired: got true, want false (PressureAvg60 0.0 < PressureRecover 0.12 → pressure clears → demand gate closes)")
	}
	for _, c := range vC.Causes {
		if c.Kind == CauseKindHostContention {
			t.Fatalf("clear Causes: host-contention must NOT remain after the demand gate drops (got %+v)", vC.Causes)
		}
	}
}

// TestDecide_HostContentionCause_ThrottleOnlyDemandGate pins the invariant that
// the host-contention demand gate opens under a THROTTLE-ONLY demand — i.e.,
// pressure is NOT firing (pressure latch clear) but throttle IS firing (throttle
// latch fired), and the host is busy with a non-UMH majority. A throttled
// container is unambiguously demanding CPU, so host-contention must be able to
// fire under a throttle-only demand, not just pressure. The engine narrowed the
// gate to pressure-only; this test MUST fail under that narrowing (the
// throttle-only demand leaves the gate closed, so host-contention does not
// fire) and pass once the gate is `pressure OR throttle`.
//
// Sequence (10s ticks, 60s throttle window):
//
//  1. BASELINE — first throttle sample establishes the ring baseline; the
//     throttle ring holds a single point so throttleRatio returns 0 (len < 2)
//     and the throttle latch is NOT firing. Pressure is held at 0 (clear).
//     The demand gate is closed → host-contention must NOT fire even though
//     the host is busy with a non-UMH majority.
//  2. FIRE — a second throttle sample pushes the 60s ratio above ThrottleHigh
//     (0.10 > 0.05) → throttle latch fires. Pressure is still 0 (clear), so
//     the demand gate is open via THROTTLE ONLY. The host is busy with a
//     non-UMH majority (HostBusyCores 3.2 > 0.70 * LogicalCpus 4.0 = 2.8, and
//     contentionCores = 3.2 - 0.5 = 2.7 > 0). host-contention MUST fire:
//     degraded, HOST attribution, causes include BOTH host-contention (value =
//     contention cores 2.7) AND throttling.
//
// Under the narrowed pressure-only gate (`demandGateOpen := signals.PressureFired`),
// step 2 fails: the throttle-only demand leaves the gate closed, so
// host-contention does not fire, attribution is unknown (throttle is internal),
// and no host-contention cause appears.
func TestDecide_HostContentionCause_ThrottleOnlyDemandGate(t *testing.T) {
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	const hostBusyCores = 3.2
	const ourUsageCores = 0.5
	const wantContentionCores = 2.7

	// (1) BASELINE — first throttle sample establishes the ring baseline. The
	// throttle ring holds a single point → throttleRatio returns 0 (len < 2) →
	// throttle latch NOT firing. Pressure held at 0 (clear). The demand gate is
	// closed, so host-contention must NOT fire even though the host is busy
	// with a non-UMH majority. This pins that the gate is not spuriously open
	// before the throttle latch has fired.
	st := &WindowState{}
	v1, sig1 := Decide(st, Sample{
		Timestamp:     base,
		UsageCores:    ourUsageCores,
		HostBusyCores: hostBusyCores,
		LogicalCpus:   4.0,
		NrPeriods:     1000,
		NrThrottled:   10,
		PressureAvg60: 0.0,
	}, thresholds)
	if sig1.ThrottleFired {
		t.Fatalf("baseline ThrottleFired: got true, want false (ring size 1 → ratio 0 → latch not firing)")
	}
	if sig1.PressureFired {
		t.Fatalf("baseline PressureFired: got true, want false (PressureAvg60 0.0 < PressureRecover → latch clear)")
	}
	if sig1.HostContentionFired {
		t.Fatalf("baseline HostContentionFired: got true, want false (demand gate closed: neither pressure nor throttle firing)")
	}
	if v1.State != StateHealthy {
		t.Fatalf("baseline State: got %q, want %q (demand gate closed → no host-contention → healthy)", v1.State, StateHealthy)
	}

	// (2) FIRE — second throttle sample: +1000 periods, +100 throttled → 60s
	// ratio (110-10)/(2000-1000) = 0.10 > ThrottleHigh 0.05 → throttle latch
	// fires. Pressure is STILL 0 (clear), so the demand gate is open via
	// THROTTLE ONLY. The host is busy with a non-UMH majority (host_busy_ratio
	// 3.2/4.0 = 0.80 > HostBusyHigh 0.70, contentionCores 2.7 > 0), so
	// host-contention MUST fire: degraded, HOST attribution, causes include
	// BOTH host-contention (value = contention cores 2.7) AND throttling.
	v2, sig2 := Decide(st, Sample{
		Timestamp:     base.Add(10 * time.Second),
		UsageCores:    ourUsageCores,
		HostBusyCores: hostBusyCores,
		LogicalCpus:   4.0,
		NrPeriods:     2000,
		NrThrottled:   110,
		PressureAvg60: 0.0,
	}, thresholds)
	if !sig2.ThrottleFired {
		t.Fatalf("fire ThrottleFired: got false, want true (60s ratio 0.10 > ThrottleHigh 0.05)")
	}
	if sig2.PressureFired {
		t.Fatalf("fire PressureFired: got true, want false (PressureAvg60 0.0 → pressure must NOT fire; this is the THROTTLE-ONLY demand path)")
	}
	if !sig2.HostContentionFired {
		t.Fatalf("fire HostContentionFired: got false, want true (demand gate open via THROTTLE ONLY: host_busy 0.80 > 0.70 AND throttle firing → host-contention fires; under the pressure-only narrowing the gate stays closed and this fails)")
	}
	if v2.State != StateDegraded {
		t.Fatalf("fire State: got %q, want %q (host-contention fires → degraded)", v2.State, StateDegraded)
	}
	if v2.Attribution != AttributionHost {
		t.Fatalf("fire Attribution: got %q, want %q (host-contention is external → host; under the pressure-only narrowing only throttle fires and attribution is unknown)", v2.Attribution, AttributionHost)
	}
	var hostContentionVal float64
	var hasHostContention, hasThrottling bool
	for _, c := range v2.Causes {
		if c.Kind == CauseKindHostContention {
			hasHostContention = true
			hostContentionVal = c.Value
		}
		if c.Kind == CauseKindThrottling {
			hasThrottling = true
		}
	}
	if !hasHostContention {
		t.Fatalf("fire Causes: host-contention MISSING from %+v (must fire alongside throttle under the throttle-only demand path)", v2.Causes)
	}
	if !hasThrottling {
		t.Fatalf("fire Causes: throttling MISSING from %+v (the throttle latch is firing and must appear as a cause)", v2.Causes)
	}
	if !floatEq(hostContentionVal, wantContentionCores) {
		t.Fatalf("fire host-contention Cause Value: got %v, want %v (contention_cores = HostBusyCores - UsageCores = 3.2 - 0.5)", hostContentionVal, wantContentionCores)
	}
}

// TestDecide_SaturationBackstop_DeadZoneFireThenClearGuardrail pins the
// saturation backstop cause and the guardrail — the whole rebuild's reason.
// Rung 2 deleted the raw-usage degrade (busy is not sick). This test
// re-introduces a usage-based degrade BUT ONLY in the dead-zone — the one
// case where no starvation signal exists (no CPU limit → no throttle; no PSI
// → no pressure; not virtualized → no steal; and host-contention can't fire
// without a demand signal). There, sustained high usage is the last-resort
// proxy.
//
// The dead-zone predicate: Quota is nil (no limit) AND PsiAvailable is false
// (no PSI) AND Virtualized is false (not virtualized). Sample.PsiAvailable is
// the readability flag that distinguishes "PSI compiled in + present" from
// "PressureAvg60 reads 0 because PSI is absent" — without it the dead-zone
// branch is unreachable dead code (a naive "PressureAvg60 == 0" is true both
// when PSI is absent AND when PSI is present but reading 0).
//
// In the dead-zone, Decide computes a 60s-AVERAGE usage fraction (NOT p95 — a
// sustained-headroom proxy; a brief spike must not trip it) over a usage ring
// in WindowState. When the 60s-avg >= HighUsageFraction (0.70) the saturation
// cause fires: {degraded, unknown, [saturation]} with Cause Value = the 60s-avg
// usage fraction. A Schmitt latch (fire >= 0.70, clear < SaturationRecover 0.60)
// prevents boundary dither. The 60s usage ring is pruned (reusing the
// throttle/steal pruning pattern) — no unbounded growth.
//
// Pinned behaviors (the (b) discipline: fire-then-clear REQUIRED, avg-not-p95,
// no-false-fire):
//
//  1. FIRE-THEN-CLEAR (REQUIRED round-trip): dead-zone, sustained high usage
//     (60s-avg >= 0.70) → saturation fires (degraded/unknown/[saturation]).
//     Then sustained low usage (60s-avg < 0.60 for enough ticks the 60s window
//     drops) → saturation clears (healthy). This is the critical round-trip.
//  2. THE GUARDRAIL: below 70% avg in the dead-zone → healthy. NEVER a
//     distinct unknown state. Blind-but-quiet = healthy. Do NOT manufacture
//     degraded from a monitoring gap.
//  3. LIMITED VISIBILITY: when in the dead-zone, Signals.LimitedVisibility is
//     true (a signal for the caller's message, NOT a state). It is false
//     outside the dead-zone.
//  4. AVG-NOT-P95: a brief spike to 0.95 that recovers (60s-avg stays < 0.70)
//     must NOT fire saturation (avg-not-p95 is the explicit decision — p95
//     would re-create the flicker).
//  5. NON-DEAD-ZONE: a limited container (Quota set) at 95% usage with no
//     throttle → healthy (saturation not evaluated; busy is not sick). This
//     re-confirms rung 2's thesis holds outside the dead-zone.
//  6. Saturation is the SOLE cause when it fires (no other cause can fire in
//     the dead-zone by definition).
func TestDecide_SaturationBackstop_DeadZoneFireThenClearGuardrail(t *testing.T) {
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// deadZoneSample constructs a dead-zone sample: Quota nil (no limit),
	// PsiAvailable false (no PSI), Virtualized false (not virtualized).
	// CgroupCores 4.0 provides the denominator for the usage fraction
	// (UsageCores/CgroupCores) — it is NOT a limit (Quota is the limit; nil
	// here means uncapped). The dead-zone is the ONLY place saturation is
	// evaluated.
	deadZoneSample := func(dt time.Duration, usageCores float64) Sample {
		return Sample{
			Timestamp:    base.Add(dt),
			UsageCores:   usageCores,
			CgroupCores:  4.0,
			PsiAvailable: false,
			Virtualized:  false,
		}
	}

	// (1) FIRE-THEN-CLEAR — the REQUIRED round-trip. Feed sustained high usage
	// (0.80 fraction = 3.2/4.0) at 10s spacing for 80s so the 60s-avg is
	// representative, then assert saturation fires. Then feed sustained low
	// usage (0.40 fraction = 1.6/4.0) for enough ticks that the 60s window
	// holds only low samples (avg < SaturationRecover 0.60), and assert
	// saturation clears (healthy).
	st := &WindowState{}

	// FIRE — 8 ticks at 0.80 fraction (t=0..t=70), then the 9th at t=80. At
	// t=80 the 60s window (cutoff=t=20) holds 7 samples all at 0.80 → avg
	// 0.80 >= HighUsageFraction 0.70 → saturation fires.
	for i := 0; i < 8; i++ {
		Decide(st, deadZoneSample(time.Duration(i*10)*time.Second, 3.2), thresholds)
	}
	vFire, sigFire := Decide(st, deadZoneSample(80*time.Second, 3.2), thresholds)
	if vFire.State != StateDegraded {
		t.Fatalf("fire State: got %q, want %q (dead-zone, 60s-avg usage 0.80 >= HighUsageFraction 0.70 → saturation fires)", vFire.State, StateDegraded)
	}
	if vFire.Attribution != AttributionUnknown {
		t.Fatalf("fire Attribution: got %q, want %q (saturation is internal → unknown)", vFire.Attribution, AttributionUnknown)
	}
	// (6) Saturation is the SOLE cause — no other cause can fire in the
	// dead-zone by definition (no limit → no throttle; no PSI → no pressure;
	// not virtualized → no steal; no demand signal → no host-contention).
	// NOTE: "no limit → no throttle" describes the production invariant
	// (uncapped cgroups report nr_throttled=0), NOT a code-level Quota gate
	// on the throttle latch — throttle is evaluated unconditionally from
	// cpu.stat counter deltas regardless of Quota and CAN co-fire in the
	// dead-zone when throttle counters are non-zero (see test 13).
	if len(vFire.Causes) != 1 {
		t.Fatalf("fire Causes length: got %d, want 1 (saturation is the sole cause in the dead-zone — no other cause can fire there by definition)", len(vFire.Causes))
	}
	if vFire.Causes[0].Kind != CauseKindSaturation {
		t.Fatalf("fire Cause Kind: got %q, want %q", vFire.Causes[0].Kind, CauseKindSaturation)
	}
	// Cause Value = the 60s-avg usage fraction (0.80).
	if !floatEq(vFire.Causes[0].Value, 0.80) {
		t.Fatalf("fire Cause Value: got %v, want 0.80 (the 60s-avg usage fraction)", vFire.Causes[0].Value)
	}
	// (3) LimitedVisibility is true in the dead-zone (a signal, NOT a state).
	if !sigFire.LimitedVisibility {
		t.Fatalf("fire LimitedVisibility: got false, want true (dead-zone = blind state; signal the caller to note limited visibility)")
	}

	// CLEAR — feed sustained low usage (0.40 fraction) at 10s spacing past the
	// last high-usage tick so the 60s window eventually holds only low samples.
	// By t=170s, cutoff=t=110s, ring holds only 0.40 samples → avg 0.40 <
	// SaturationRecover 0.60 → latch clears.
	for i := 0; i < 8; i++ {
		Decide(st, deadZoneSample(time.Duration(90+i*10)*time.Second, 1.6), thresholds)
	}
	vClear, sigClear := Decide(st, deadZoneSample(170*time.Second, 1.6), thresholds)
	// (2) THE GUARDRAIL — below 70% avg in the dead-zone → healthy. NEVER a
	// distinct unknown state. State is binary healthy|degraded; blind-but-quiet
	// is healthy. Do NOT manufacture degraded from a monitoring gap.
	if vClear.State != StateHealthy {
		t.Fatalf("clear State: got %q, want %q (60s-avg usage 0.40 < SaturationRecover 0.60 → latch clears → healthy; the guardrail: blind-but-quiet = healthy, never a distinct unknown state)", vClear.State, StateHealthy)
	}
	if len(vClear.Causes) != 0 {
		t.Fatalf("clear Causes length: got %d, want 0 (no causes when healthy)", len(vClear.Causes))
	}
	// (3) LimitedVisibility is STILL true in the dead-zone when healthy — the
	// blind state persists regardless of the verdict.
	if !sigClear.LimitedVisibility {
		t.Fatalf("clear LimitedVisibility: got false, want true (still in the dead-zone; blind-but-quiet signals limited visibility even when healthy)")
	}

	// (4) AVG-NOT-P95 — a brief spike to 0.95 that recovers (60s-avg stays <
	// 0.70) must NOT fire saturation. 6 ticks at 0.40, 1 tick at 0.95, then 1
	// more 0.40 tick. The 60s-avg never reaches 0.70; a p95 would have
	// returned 0.95 (nearest-rank of mostly-0.40 + one 0.95) and fired — the
	// average is what prevents the flicker.
	st2 := &WindowState{}
	for i := 0; i < 6; i++ {
		Decide(st2, deadZoneSample(time.Duration(i*10)*time.Second, 1.6), thresholds)
	}
	Decide(st2, deadZoneSample(60*time.Second, 3.8), thresholds) // 3.8/4.0 = 0.95 spike
	vSpike, sigSpike := Decide(st2, deadZoneSample(70*time.Second, 1.6), thresholds)
	// At t=70: cutoff=t=10, ring = t=10..t=70 (7 samples: 6 at 0.40, 1 at
	// 0.95) → avg = (6*0.40 + 0.95)/7 = 3.35/7 = 0.479 < 0.70. Does NOT fire.
	if vSpike.State != StateHealthy {
		t.Fatalf("spike State: got %q, want %q (60s-avg 0.479 < 0.70; a brief spike is absorbed by the average — avg-not-p95: p95 would have been 0.95 and fired)", vSpike.State, StateHealthy)
	}
	if len(vSpike.Causes) != 0 {
		t.Fatalf("spike Causes length: got %d, want 0 (spike absorbed; no false fire)", len(vSpike.Causes))
	}
	if !sigSpike.LimitedVisibility {
		t.Fatalf("spike LimitedVisibility: got false, want true (dead-zone)")
	}

	// (5) NON-DEAD-ZONE — a limited container (Quota set) at 95% usage with no
	// throttle → healthy (saturation not evaluated; busy is not sick). This
	// re-confirms rung 2's thesis holds outside the dead-zone. Quota non-nil
	// means a limit is set → NOT the dead-zone → saturation is NOT evaluated.
	// PsiAvailable true (PSI present) also disqualifies the dead-zone.
	quota := 2.0
	st3 := &WindowState{}
	for i := 0; i < 8; i++ {
		Decide(st3, Sample{
			Timestamp:    base.Add(time.Duration(i*10) * time.Second),
			UsageCores:   1.9, // 0.95 of 2.0 quota
			Quota:        &quota,
			PsiAvailable: true,
			Virtualized:  false,
		}, thresholds)
	}
	vNonDz, sigNonDz := Decide(st3, Sample{
		Timestamp:    base.Add(80 * time.Second),
		UsageCores:   1.9,
		Quota:        &quota,
		PsiAvailable: true,
		Virtualized:  false,
	}, thresholds)
	if vNonDz.State != StateHealthy {
		t.Fatalf("non-dead-zone State: got %q, want %q (Quota set + PSI available → NOT the dead-zone → saturation NOT evaluated; 0.95 usage with no throttle is busy, not sick)", vNonDz.State, StateHealthy)
	}
	if len(vNonDz.Causes) != 0 {
		t.Fatalf("non-dead-zone Causes length: got %d, want 0 (saturation does not fire outside the dead-zone)", len(vNonDz.Causes))
	}
	// LimitedVisibility is false outside the dead-zone (PSI is available → not
	// a blind state).
	if sigNonDz.LimitedVisibility {
		t.Fatalf("non-dead-zone LimitedVisibility: got true, want false (PSI available → not a blind state → no limited-visibility signal)")
	}

	// (7) TRANSITION-OUT-OF-DEAD-ZONE — fire saturation in the dead-zone on a
	// WindowState, then feed a non-dead-zone sample (Quota set, PsiAvailable
	// true) on the SAME WindowState and assert the stale latch is cleared on
	// the very next tick: StateHealthy + 0 causes + SaturationFired false.
	// This is the guard against a latch leak: without a reset on dead-zone
	// exit, a prior fire would emit {saturation, Value: 0} on every subsequent
	// healthy tick (saturationAvg is only recomputed inside the dead-zone
	// block). blind-but-quiet is healthy, never a stuck false-degrade.
	st4 := &WindowState{}
	for i := 0; i < 8; i++ {
		Decide(st4, deadZoneSample(time.Duration(i*10)*time.Second, 3.2), thresholds)
	}
	vFire4, _ := Decide(st4, deadZoneSample(80*time.Second, 3.2), thresholds)
	if vFire4.State != StateDegraded {
		t.Fatalf("transition fire State: got %q, want %q (dead-zone 60s-avg 0.80 >= 0.70 → saturation fires before transition)", vFire4.State, StateDegraded)
	}
	// Flip out of the dead-zone on the SAME st4: set Quota, PsiAvailable true.
	quota4 := 2.0
	vTrans, sigTrans := Decide(st4, Sample{
		Timestamp:    base.Add(90 * time.Second),
		UsageCores:   1.9, // 0.95 of 2.0 quota — busy, not sick
		Quota:        &quota4,
		PsiAvailable: true,
		Virtualized:  false,
	}, thresholds)
	if vTrans.State != StateHealthy {
		t.Fatalf("transition State: got %q, want %q (dead-zone→non-dead-zone: latch MUST clear on the very next tick; a stale fire must not leak a false-degrade)", vTrans.State, StateHealthy)
	}
	if len(vTrans.Causes) != 0 {
		t.Fatalf("transition Causes length: got %d, want 0 (stale saturation latch must not append a zero-valued cause outside the dead-zone)", len(vTrans.Causes))
	}
	if sigTrans.SaturationFired {
		t.Fatalf("transition SaturationFired: got true, want false (latch cleared on dead-zone exit)")
	}
	if sigTrans.LimitedVisibility {
		t.Fatalf("transition LimitedVisibility: got true, want false (Quota set + PSI available → not the dead-zone)")
	}

	// (8) HOLD (Schmitt) — fire saturation at 0.80, then cool the 60s-avg into
	// the hold band (0.60..0.70) and assert the latch STAYS fired. This is the
	// core Schmitt guarantee: a regression that collapses SaturationRecover to
	// HighUsageFraction (clearing as soon as avg drops below 0.70) would pass
	// the fire-then-clear test but fail here. Mirrors the throttle HOLD assertion
	// at the throttle flip-latch test.
	st5 := &WindowState{}
	for i := 0; i < 8; i++ {
		Decide(st5, deadZoneSample(time.Duration(i*10)*time.Second, 3.2), thresholds) // 0.80
	}
	vFire5, _ := Decide(st5, deadZoneSample(80*time.Second, 3.2), thresholds)
	if vFire5.State != StateDegraded {
		t.Fatalf("hold fire State: got %q, want %q (dead-zone 60s-avg 0.80 >= 0.70 → saturation fires)", vFire5.State, StateDegraded)
	}
	// Cool to 0.65 (2.6/4.0) — inside the hold band (SaturationRecover 0.60 ..
	// HighUsageFraction 0.70). Feed enough ticks that all 0.80 samples age out
	// and the 60s window holds only 0.65 samples.
	for i := 0; i < 7; i++ {
		Decide(st5, deadZoneSample(time.Duration(90+i*10)*time.Second, 2.6), thresholds) // 0.65
	}
	vHold, sigHold := Decide(st5, deadZoneSample(160*time.Second, 2.6), thresholds)
	// At t=160: cutoff=t=100; ring = t=100..t=160 (7 samples at 0.65) → avg
	// 0.65, inside the Schmitt band. Latch holds fired.
	if vHold.State != StateDegraded {
		t.Fatalf("hold State: got %q, want %q (60s-avg 0.65 is in the Schmitt band 0.60..0.70; latch holds fired)", vHold.State, StateDegraded)
	}
	if !sigHold.SaturationFired {
		t.Fatalf("hold SaturationFired: got false, want true (latch holds in the band; only drops below SaturationRecover 0.60 clears)")
	}
	if len(vHold.Causes) != 1 || vHold.Causes[0].Kind != CauseKindSaturation {
		t.Fatalf("hold Causes: got %+v, want one saturation cause (latch held)", vHold.Causes)
	}

	// (9) GAP-CLEAR — fire saturation, then a >60s sampling gap prunes the ring
	// to a single low sample. The latch MUST clear: a single sample is
	// insufficient evidence to sustain a prior fire. Without the inner else
	// (clear on len < 2) the latch would hold indefinitely and emit a
	// {saturation, Value: 0.40} cause while HighUsageFraction is 0.70 —
	// contradicting the documented "clears when the 60s-average drops below
	// SaturationRecover."
	st6 := &WindowState{}
	for i := 0; i < 8; i++ {
		Decide(st6, deadZoneSample(time.Duration(i*10)*time.Second, 3.2), thresholds) // 0.80
	}
	vFire6, _ := Decide(st6, deadZoneSample(80*time.Second, 3.2), thresholds)
	if vFire6.State != StateDegraded {
		t.Fatalf("gap-clear fire State: got %q, want %q (saturation fires before gap)", vFire6.State, StateDegraded)
	}
	// >60s gap: next tick at t=150 (70s after t=80). cutoff=90; all 0.80
	// samples (t=0..80) are pruned. Ring holds only the t=150 sample.
	vGap, sigGap := Decide(st6, deadZoneSample(150*time.Second, 1.6), thresholds) // 0.40
	if vGap.State != StateHealthy {
		t.Fatalf("gap-clear State: got %q, want %q (single-sample ring after >60s gap: insufficient evidence to sustain fire → latch clears → healthy)", vGap.State, StateHealthy)
	}
	if sigGap.SaturationFired {
		t.Fatalf("gap-clear SaturationFired: got true, want false (latch cleared: single sample cannot sustain a prior fire)")
	}
	if len(vGap.Causes) != 0 {
		t.Fatalf("gap-clear Causes length: got %d, want 0 (latch cleared, no cause emitted)", len(vGap.Causes))
	}

	// (10) NON-POSITIVE QUOTA DEAD-ZONE — a non-nil but non-positive Quota
	// (Quota=&0, the unlimited-cgroup case from parseCPUMax for cpu.max="max")
	// on bare metal without PSI is uncapped AND blind. The dead-zone predicate
	// must treat non-positive Quota as nil-equivalent so LimitedVisibility is
	// true (the caller is told it is blind). A naive `Quota == nil` check would
	// exclude this case: LimitedVisibility false (caller not told it is blind)
	// AND the saturation backstop skipped. NOTE: Quota=&0 is signal-only here
	// (LimitedVisibility=true, no backstop fire) — the backstop only fires for
	// the Quota==nil + CgroupCores>0 sub-case where a fraction is computable;
	// Quota=&0 cannot compute a fraction (the `>0` guard rejects it and there
	// is no CgroupCores fallback when Quota is non-nil), so the proxy is inert
	// by design (blind-but-quiet=healthy per the model).
	zeroQuota := 0.0
	st7 := &WindowState{}
	vZeroQ, sigZeroQ := Decide(st7, Sample{
		Timestamp:    base,
		UsageCores:   1.5,
		Quota:        &zeroQuota,
		PsiAvailable: false,
		Virtualized:  false,
	}, thresholds)
	if !sigZeroQ.LimitedVisibility {
		t.Fatalf("non-positive Quota LimitedVisibility: got false, want true (Quota=&0 is uncapped: non-positive Quota is nil-equivalent for the dead-zone predicate; the caller must be told it is blind)")
	}
	if vZeroQ.State != StateHealthy {
		t.Fatalf("non-positive Quota State: got %q, want %q (uncapped: no fraction computed, no cause)", vZeroQ.State, StateHealthy)
	}
	if len(vZeroQ.Causes) != 0 {
		t.Fatalf("non-positive Quota Causes length: got %d, want 0 (uncapped, no fire)", len(vZeroQ.Causes))
	}

	// (11) NAN USAGE INPUT CLAMP — a NaN UsageCores reading produces a NaN
	// fraction that poisons the 60s running sum until the sample ages out.
	// The input clamp (mirroring the PressureAvg60 clamp) stores 0 instead, so
	// subsequent averages are not corrupted. Without the input clamp the ring
	// stays NaN-corrupted: sum is NaN → saturationAvg NaN → NaN >= 0.70 is
	// false → the latch cannot fire AND an existing fire holds/clears, blinding
	// saturation for up to 60s. This test forces the input clamp by feeding
	// NaN then enough normal high-usage samples that the avg reaches 0.70 ONLY
	// if the NaN was clamped to 0 (if it stayed NaN, the sum would be NaN → no
	// fire).
	st8 := &WindowState{}
	// NaN sample at t=0 (5s spacing keeps it in the 60s window through t=35).
	Decide(st8, Sample{
		Timestamp:   base,
		UsageCores:  math.NaN(),
		CgroupCores: 4.0,
	}, thresholds)
	// 7 normal high-usage samples at 5s spacing (0.80 fraction = 3.2/4.0).
	for i := 1; i <= 7; i++ {
		Decide(st8, deadZoneSample(time.Duration(i*5)*time.Second, 3.2), thresholds)
	}
	// At t=35s: cutoff = t-60s = before t=0, so all 8 samples survive. Ring =
	// [0 (clamped NaN), 0.80, 0.80, 0.80, 0.80, 0.80, 0.80, 0.80] → avg =
	// (0 + 7*0.80)/8 = 0.70 >= HighUsageFraction → fires. Without the input
	// clamp the ring holds NaN → sum NaN → saturationAvg NaN → NaN >= 0.70 is
	// false → does NOT fire.
	vNaN, sigNaN := Decide(st8, deadZoneSample(35*time.Second, 3.2), thresholds)
	if vNaN.State != StateDegraded {
		t.Fatalf("NaN-clamp State: got %q, want %q (NaN was input-clamped to 0; avg = (0+7*0.80)/8 = 0.70 >= 0.70 → fires; without the input clamp the NaN would poison the sum and prevent firing)", vNaN.State, StateDegraded)
	}
	if !sigNaN.SaturationFired {
		t.Fatalf("NaN-clamp SaturationFired: got false, want true (input clamp prevented ring corruption; saturation can fire)")
	}

	// (12) RE-ENTRY RING CLEAR — fire saturation in the dead-zone, exit to a
	// non-dead-zone sample, then RE-ENTER the dead-zone with low usage. The
	// usage ring MUST have been cleared on exit (the dead-zone else-branch):
	// without the clear, stale 0.80 samples from the first dead-zone period
	// would corrupt the 60s average on re-entry and false-fire saturation. This
	// pins the ring clear that the transition-out test (7) does not exercise
	// (it checks only the single non-dead-zone tick after exit).
	//
	// The false fire occurs at the FIRST re-entry tick (t=100s), NOT at the
	// steady state: at t=100, cutoff=40s, so stale t=40..80 samples (5×0.80)
	// survive and avg = (5*0.80+0.40)/6 = 0.733 >= 0.70. By t=170 the stale
	// samples have aged out naturally (cutoff=110), so asserting only at t=170
	// is a no-op pin — a regression that deletes the ring-clear leaves t=170
	// PASSING. Assert at t=100 instead (and continue to t=170 for the steady
	// state).
	st9 := &WindowState{}
	for i := 0; i < 8; i++ {
		Decide(st9, deadZoneSample(time.Duration(i*10)*time.Second, 3.2), thresholds) // 0.80
	}
	vFire9, _ := Decide(st9, deadZoneSample(80*time.Second, 3.2), thresholds)
	if vFire9.State != StateDegraded {
		t.Fatalf("re-entry fire State: got %q, want %q (saturation fires before exit)", vFire9.State, StateDegraded)
	}
	// Exit the dead-zone (Quota set, PSI available).
	quota9 := 2.0
	Decide(st9, Sample{
		Timestamp:    base.Add(90 * time.Second),
		UsageCores:   1.9,
		Quota:        &quota9,
		PsiAvailable: true,
		Virtualized:  false,
	}, thresholds)
	// FIRST re-entry tick (t=100s): the ring-clear pin. With the clear, the
	// ring holds 1 sample (0.40) → len<2 → latch cleared → healthy. WITHOUT the
	// clear, stale t=40..80 samples survive (cutoff=40s) and avg =
	// (5*0.80+0.40)/6 = 0.733 >= 0.70 → false fire → degraded.
	vReentry, sigReentry := Decide(st9, deadZoneSample(100*time.Second, 1.6), thresholds)
	if vReentry.State != StateHealthy {
		t.Fatalf("re-entry State (first tick t=100): got %q, want %q (usage ring was cleared on dead-zone exit; first re-entry tick holds 1 sample → len<2 → healthy; without the clear, stale 0.80 samples would false-fire at avg 0.733)", vReentry.State, StateHealthy)
	}
	if sigReentry.SaturationFired {
		t.Fatalf("re-entry SaturationFired (first tick t=100): got true, want false (ring cleared on exit; no stale samples to sustain a fire)")
	}
	// Continue re-entry to the steady state and confirm it stays healthy.
	for i := 1; i < 8; i++ {
		Decide(st9, deadZoneSample(time.Duration(100+i*10)*time.Second, 1.6), thresholds) // 0.40
	}
	vSteady, sigSteady := Decide(st9, deadZoneSample(170*time.Second, 1.6), thresholds)
	if vSteady.State != StateHealthy {
		t.Fatalf("re-entry steady State (t=170): got %q, want %q (ring holds only 0.40 samples → avg 0.40 < 0.60 → healthy)", vSteady.State, StateHealthy)
	}
	if sigSteady.SaturationFired {
		t.Fatalf("re-entry steady SaturationFired (t=170): got true, want false (no stale samples to sustain a fire)")
	}

	// (13) CO-FIRE SEVERITY ORDERING — in the dead-zone, throttle and
	// saturation can co-fire (throttle is evaluated regardless of Quota). When
	// both fire, the dominance sort orders by severity. This pins the
	// severity(saturationAvg, ...) call and the sort comparator for saturation
	// (the fire-then-clear test always has saturation as the sole cause, so the
	// sort is a no-op there). Throttle sev (0.80-0.05)/0.95 = 0.789 >
	// saturation sev (0.80-0.70)/0.30 = 0.333 → throttle dominant →
	// causes[0]=throttling, causes[1]=saturation, attribution=unknown (both
	// internal).
	st10 := &WindowState{}
	// Dead-zone samples with high throttle (ratio 0.80) AND high usage (0.80).
	deadZoneThrottleSample := func(dt time.Duration, nrPeriods, nrThrottled int64) Sample {
		return Sample{
			Timestamp:    base.Add(dt),
			UsageCores:   3.2, // 0.80 of 4.0
			CgroupCores:  4.0,
			NrPeriods:    nrPeriods,
			NrThrottled:  nrThrottled,
			PsiAvailable: false,
			Virtualized:  false,
		}
	}
	for i := 0; i < 8; i++ {
		Decide(st10, deadZoneThrottleSample(time.Duration(i*10)*time.Second, int64(i+1)*1000, int64(i+1)*800), thresholds)
	}
	vCo, sigCo := Decide(st10, deadZoneThrottleSample(80*time.Second, 9000, 7200), thresholds)
	if vCo.State != StateDegraded {
		t.Fatalf("co-fire State: got %q, want %q (both throttle and saturation fire)", vCo.State, StateDegraded)
	}
	if len(vCo.Causes) != 2 {
		t.Fatalf("co-fire Causes length: got %d, want 2 (throttle + saturation)", len(vCo.Causes))
	}
	if vCo.Causes[0].Kind != CauseKindThrottling {
		t.Fatalf("co-fire causes[0]: got %q, want %q (throttle sev 0.789 > saturation sev 0.333 → throttle dominant)", vCo.Causes[0].Kind, CauseKindThrottling)
	}
	if vCo.Causes[1].Kind != CauseKindSaturation {
		t.Fatalf("co-fire causes[1]: got %q, want %q (saturation is lower severity → second)", vCo.Causes[1].Kind, CauseKindSaturation)
	}
	if vCo.Attribution != AttributionUnknown {
		t.Fatalf("co-fire Attribution: got %q, want %q (both throttle and saturation are internal → unknown)", vCo.Attribution, AttributionUnknown)
	}
	if !sigCo.ThrottleFired || !sigCo.SaturationFired {
		t.Fatalf("co-fire latches: ThrottleFired=%v SaturationFired=%v, want both true", sigCo.ThrottleFired, sigCo.SaturationFired)
	}
}

// TestDecide_SaturationBackstop_TwoSampleFloor pins the 2-sample floor on the
// saturation backstop latch: the latch is NOT evaluated when the usage ring
// holds fewer than 2 samples. A lone first-tick dead-zone high-usage reading
// (60s-avg would be 0.95, well above HighUsageFraction 0.70) must NOT fire —
// "sustained fires, isolated absorbed." Without the floor a single 0.95 sample
// would fire immediately on the first tick. This test is the (b)-discipline pin
// for the floor guard: it MUST fail if the `len(st.usageRing) >= 2` guard is
// removed (the lone 0.95 sample would fire). The existing fire-then-clear test
// only fires after 8 ticks, so a regression deleting the floor would pass there
// — this test closes that gap.
func TestDecide_SaturationBackstop_TwoSampleFloor(t *testing.T) {
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// A SINGLE dead-zone high-usage sample: 3.8/4.0 = 0.95 fraction, well into
	// the fire band (>= HighUsageFraction 0.70). Quota nil, PsiAvailable false,
	// Virtualized false → dead-zone. The ring holds 1 entry after this tick →
	// below the 2-sample floor → latch NOT evaluated → healthy.
	st := &WindowState{}
	v, sig := Decide(st, Sample{
		Timestamp:    base,
		UsageCores:   3.8, // 0.95 of 4.0
		CgroupCores:  4.0,
		PsiAvailable: false,
		Virtualized:  false,
	}, thresholds)
	if v.State != StateHealthy {
		t.Fatalf("State: got %q, want %q (single dead-zone high-usage sample: ring holds 1 entry, below the 2-sample floor → latch not evaluated → healthy; a lone first-tick reading must not fire — sustained fires, isolated absorbed)", v.State, StateHealthy)
	}
	if len(v.Causes) != 0 {
		t.Fatalf("Causes length: got %d, want 0 (floor guard prevents the lone 0.95 sample from firing saturation)", len(v.Causes))
	}
	if sig.SaturationFired {
		t.Fatalf("SaturationFired: got true, want false (2-sample floor: a single sample is insufficient evidence to fire)")
	}
	// The dead-zone is still signalled to the caller regardless of the floor.
	if !sig.LimitedVisibility {
		t.Fatalf("LimitedVisibility: got false, want true (dead-zone blind state is signalled independent of the latch)")
	}
}

// TestDecide_DominanceBySeverity pins the dominance rule: when several causes
// fire, the highest-severity one sets attribution. Severity =
// (value - high_threshold) / (1 - high_threshold) — the fraction into the
// danger band toward maximum (0 = just barely firing, 1 = maxed). This is the
// one scale that puts every cause on a shared axis. Ties go to the EXTERNAL
// side (host). Causes[] is ordered dominant-first.
//
// Each cause's severity numerator is its own reduced value, and its
// high_threshold is its own High mark: throttle uses the throttle ratio +
// ThrottleHigh; pressure uses PressureAvg60 + PressureHigh; steal uses the
// steal p95 + StealHigh; host-contention uses host_busy_ratio (NOT its wire
// value of contention cores) + HostBusyHigh.
//
// The dominant cause sets attribution: external (steal, host-contention) →
// HOST; internal (throttle, pressure) → UNKNOWN.
//
// Pinned behaviors:
//
//	(a) throttle 0.80 (sev 0.79) + host-contention host_busy 0.71 (sev 0.033)
//	    → throttle dominant → UNKNOWN; causes[0]=throttling.
//	(b) throttle 0.10 (sev 0.053) + host-contention host_busy 1.0 (sev 1.0)
//	    → host-contention dominant → HOST; causes[0]=host-contention.
//	(c) steal 0.50 (sev 0.44) + pressure 0.25 (sev 0.0625)
//	    → steal dominant → HOST; causes[0]=steal.
//	(d) TIE — throttle 1.0 (sev 1.0) + host-contention host_busy 1.0 (sev 1.0)
//	    → external wins → HOST; causes[0]=host-contention.
//	(e) CROSSOVER — a sequence where the dominant cause CHANGES as severity
//	    shifts: throttle dominant early (host barely over), then host-contention
//	    becomes more severe (host rises to full) → attribution flips UNKNOWN →
//	    HOST at the crossover.
func TestDecide_DominanceBySeverity(t *testing.T) {
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// (a) throttle dominant over host-contention (host barely over 0.70).
	// throttle sev (0.80-0.05)/0.95 = 0.789 >> host-contention sev
	// (0.71-0.70)/0.30 = 0.033 → throttle dominant → UNKNOWN. host-contention's
	// severity uses host_busy_ratio (0.71), NOT contention cores (2.34) — if it
	// used contention cores the severity would be ~5.5 (unclamped) and
	// host-contention would wrongly dominate.
	t.Run("throttle_dominant_over_host_contention_host_barely_over", func(t *testing.T) {
		st := &WindowState{}
		Decide(st, Sample{
			Timestamp: base, NrPeriods: 1000, NrThrottled: 10,
			HostBusyCores: 2.84, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		// +1000 periods, +800 throttled → 60s ratio 0.80 > 0.05 (throttle fires).
		// host_busy 2.84/4.0 = 0.71 > 0.70 (host-contention fires, demand gate
		// open via throttle).
		v, sig := Decide(st, Sample{
			Timestamp: base.Add(10 * time.Second),
			NrPeriods: 2000, NrThrottled: 810,
			HostBusyCores: 2.84, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		if !sig.ThrottleFired || !sig.HostContentionFired {
			t.Fatalf("setup: ThrottleFired=%v HostContentionFired=%v, want both true", sig.ThrottleFired, sig.HostContentionFired)
		}
		if v.Attribution != AttributionUnknown {
			t.Fatalf("Attribution: got %q, want %q (throttle sev 0.79 > host-contention sev 0.033 → throttle dominant → unknown)", v.Attribution, AttributionUnknown)
		}
		if len(v.Causes) < 2 || v.Causes[0].Kind != CauseKindThrottling {
			t.Fatalf("Causes ordering: got %+v, want throttling first (dominant)", v.Causes)
		}
		if v.Causes[1].Kind != CauseKindHostContention {
			t.Fatalf("Causes ordering[1]: got %q, want %q (host-contention second, lower severity)", v.Causes[1].Kind, CauseKindHostContention)
		}
	})

	// (b) host-contention dominant over throttle (host maxed).
	// throttle sev (0.10-0.05)/0.95 = 0.053 << host-contention sev
	// (1.0-0.70)/0.30 = 1.0 → host-contention dominant → HOST.
	t.Run("host_contention_dominant_over_throttle_host_maxed", func(t *testing.T) {
		st := &WindowState{}
		Decide(st, Sample{
			Timestamp: base, NrPeriods: 1000, NrThrottled: 10,
			HostBusyCores: 4.0, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		// +1000 periods, +100 throttled → 60s ratio 0.10 > 0.05 (throttle fires).
		// host_busy 4.0/4.0 = 1.0 > 0.70 (host-contention fires).
		v, sig := Decide(st, Sample{
			Timestamp: base.Add(10 * time.Second),
			NrPeriods: 2000, NrThrottled: 110,
			HostBusyCores: 4.0, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		if !sig.ThrottleFired || !sig.HostContentionFired {
			t.Fatalf("setup: ThrottleFired=%v HostContentionFired=%v, want both true", sig.ThrottleFired, sig.HostContentionFired)
		}
		if v.Attribution != AttributionHost {
			t.Fatalf("Attribution: got %q, want %q (host-contention sev 1.0 > throttle sev 0.053 → host-contention dominant → host)", v.Attribution, AttributionHost)
		}
		if len(v.Causes) < 2 || v.Causes[0].Kind != CauseKindHostContention {
			t.Fatalf("Causes ordering: got %+v, want host-contention first (dominant)", v.Causes)
		}
		if v.Causes[1].Kind != CauseKindThrottling {
			t.Fatalf("Causes ordering[1]: got %q, want %q (throttling second, lower severity)", v.Causes[1].Kind, CauseKindThrottling)
		}
	})

	// (c) steal dominant over pressure.
	// steal sev (0.50-0.10)/0.90 = 0.44 >> pressure sev (0.25-0.20)/0.80 =
	// 0.0625 → steal dominant → HOST.
	t.Run("steal_dominant_over_pressure", func(t *testing.T) {
		st := &WindowState{}
		// First tick: steal ring gets 1 point (below 2-sample floor, latch not
		// evaluated). Pressure 0.25 > 0.20 fires.
		Decide(st, Sample{
			Timestamp:     base,
			StealFraction: 0.50, Virtualized: true,
			PressureAvg60: 0.25,
		}, thresholds)
		// Second tick: steal ring has 2 points, p95 = 0.50 > 0.10 (steal fires).
		// Pressure still 0.25 (fires).
		v, sig := Decide(st, Sample{
			Timestamp:     base.Add(1 * time.Second),
			StealFraction: 0.50, Virtualized: true,
			PressureAvg60: 0.25,
		}, thresholds)
		if !sig.StealFired || !sig.PressureFired {
			t.Fatalf("setup: StealFired=%v PressureFired=%v, want both true", sig.StealFired, sig.PressureFired)
		}
		if v.Attribution != AttributionHost {
			t.Fatalf("Attribution: got %q, want %q (steal sev 0.44 > pressure sev 0.0625 → steal dominant → host)", v.Attribution, AttributionHost)
		}
		if len(v.Causes) < 2 || v.Causes[0].Kind != CauseKindSteal {
			t.Fatalf("Causes ordering: got %+v, want steal first (dominant)", v.Causes)
		}
		if v.Causes[1].Kind != CauseKindPressure {
			t.Fatalf("Causes ordering[1]: got %q, want %q (pressure second, lower severity)", v.Causes[1].Kind, CauseKindPressure)
		}
	})

	// (d) TIE — two causes with equal severity, one external one internal.
	// throttle 1.0 (sev (1.0-0.05)/(1-0.05) = 1.0) + host-contention
	// host_busy 1.0 (sev (1.0-0.70)/(1-0.70) = 1.0) → tie at 1.0 → external
	// wins → HOST; causes[0]=host-contention (external). The severities are
	// exactly equal because (v-high)/(1-high) with v=1.0 yields (1.0-high)/
	// (1.0-high) = 1.0 for both causes regardless of high.
	t.Run("tie_external_wins", func(t *testing.T) {
		st := &WindowState{}
		Decide(st, Sample{
			Timestamp: base, NrPeriods: 1000, NrThrottled: 10,
			HostBusyCores: 4.0, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		// throttle ratio = (1010-10)/(2000-1000) = 1000/1000 = 1.0, host_busy =
		// 4.0/4.0 = 1.0. Both sev = 1.0 → tie → external wins.
		v, sig := Decide(st, Sample{
			Timestamp: base.Add(10 * time.Second),
			NrPeriods: 2000, NrThrottled: 1010,
			HostBusyCores: 4.0, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		if !sig.ThrottleFired || !sig.HostContentionFired {
			t.Fatalf("setup: ThrottleFired=%v HostContentionFired=%v, want both true", sig.ThrottleFired, sig.HostContentionFired)
		}
		if v.Attribution != AttributionHost {
			t.Fatalf("Attribution: got %q, want %q (tie at sev 1.0 → external (host-contention) wins → host)", v.Attribution, AttributionHost)
		}
		if len(v.Causes) < 2 || v.Causes[0].Kind != CauseKindHostContention {
			t.Fatalf("Causes ordering: got %+v, want host-contention first (external wins tie)", v.Causes)
		}
		if v.Causes[1].Kind != CauseKindThrottling {
			t.Fatalf("Causes ordering[1]: got %q, want %q (internal throttling second after tie-break)", v.Causes[1].Kind, CauseKindThrottling)
		}
	})

	// (e) CROSSOVER — the dominant cause CHANGES as severity shifts. Same
	// WindowState across the sequence. Early: throttle 0.80 (sev 0.79) +
	// host_busy 0.71 (sev 0.033) → throttle dominant → UNKNOWN. Late: throttle
	// ratio drops to 0.06 (sev 0.011, latch holds fired above 0.05) + host_busy
	// rises to 1.0 (sev 1.0) → host-contention dominant → HOST. Assert
	// attribution flips UNKNOWN → HOST at the crossover.
	t.Run("crossover_dominance_shift", func(t *testing.T) {
		st := &WindowState{}
		// Tick 1 (t=0): baseline throttle counters. Host busy at 0.71.
		Decide(st, Sample{
			Timestamp: base, NrPeriods: 1000, NrThrottled: 10,
			HostBusyCores: 2.84, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		// Tick 2 (t=10s): throttle ratio 0.80 (sev 0.79), host_busy 0.71
		// (sev 0.033) → throttle dominant → UNKNOWN.
		v1, sig1 := Decide(st, Sample{
			Timestamp: base.Add(10 * time.Second),
			NrPeriods: 2000, NrThrottled: 810,
			HostBusyCores: 2.84, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		if !sig1.ThrottleFired || !sig1.HostContentionFired {
			t.Fatalf("phase1 setup: ThrottleFired=%v HostContentionFired=%v, want both true", sig1.ThrottleFired, sig1.HostContentionFired)
		}
		if v1.Attribution != AttributionUnknown {
			t.Fatalf("phase1 Attribution: got %q, want %q (throttle sev 0.79 > host-contention sev 0.033 → throttle dominant → unknown)", v1.Attribution, AttributionUnknown)
		}
		// Tick 3 (t=20s): throttle ratio drops to 0.06 (still > 0.05, latch
		// holds fired, sev 0.011), host_busy rises to 1.0 (sev 1.0) →
		// host-contention dominant → HOST. Attribution flips UNKNOWN → HOST.
		// Counters are monotonically increasing (nrThrottled 850 > 810 from
		// tick 2) to avoid the counter-regression ring-clear. 60s ratio =
		// (850-10)/(15000-1000) = 840/14000 = 0.06.
		v2, sig2 := Decide(st, Sample{
			Timestamp: base.Add(20 * time.Second),
			NrPeriods: 15000, NrThrottled: 850,
			HostBusyCores: 4.0, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		if !sig2.ThrottleFired || !sig2.HostContentionFired {
			t.Fatalf("phase2 setup: ThrottleFired=%v HostContentionFired=%v, want both true", sig2.ThrottleFired, sig2.HostContentionFired)
		}
		if v2.Attribution != AttributionHost {
			t.Fatalf("phase2 Attribution: got %q, want %q (host-contention sev 1.0 > throttle sev 0.011 → host-contention dominant → host; attribution must flip from unknown to host at the crossover)", v2.Attribution, AttributionHost)
		}
		if len(v2.Causes) < 2 || v2.Causes[0].Kind != CauseKindHostContention {
			t.Fatalf("phase2 Causes ordering: got %+v, want host-contention first (dominant after crossover)", v2.Causes)
		}
		if v2.Causes[1].Kind != CauseKindThrottling {
			t.Fatalf("phase2 Causes ordering[1]: got %q, want %q (throttling second, lower severity after crossover)", v2.Causes[1].Kind, CauseKindThrottling)
		}
	})

	// (f) HELD-BAND NO-FLAP — both an external cause (host-contention) and an
	// internal cause (pressure) are HELD (latch fired, current reading in the
	// hold band below the High mark). A held cause's current reading is below
	// its High mark, so the raw severity (value-high)/(1-high) is NEGATIVE.
	// Without the < 0 → 0 clamp, the least-negative held cause wins dominance
	// and Attribution flaps as held readings jitter; with the clamp, all held
	// causes tie at 0 and the external tie-break decides deterministically →
	// HOST whenever an external cause is held.
	//
	// The test data makes the clamp LOAD-BEARING: the two hold ticks are chosen
	// so that the RAW-severity ordering SWAPS across them. Without the clamp,
	// tick 1 → HOST (host-contention less negative) and tick 2 → UNKNOWN
	// (pressure less negative) → Attribution flaps. With the clamp, both ticks
	// tie at 0 → external wins → stable HOST. If you remove the `if sev < 0 {
	// return 0 }` clamp in severity(), this test FAILS at tick 2.
	//
	// Pressure hold band is [PressureRecover 0.12, PressureHigh 0.20). The
	// host-contention latch has no recover threshold of its own: once fired it
	// stays fired as long as the demand gate (pressure OR throttle) is open,
	// even when hostBusyRatio drops below HostBusyHigh — that is its "hold"
	// behavior. Both latches are fired first (pressure > 0.20, hostBusyRatio >
	// 0.70 with demand gate open via pressure), then driven into their hold
	// bands. Both causes use DIRECT readings (no ring), so the severity
	// numerators are fully controllable per-tick.
	t.Run("held_band_no_flap_external_wins", func(t *testing.T) {
		st := &WindowState{}
		// FIRE — pressure 0.21 > PressureHigh 0.20 fires the pressure latch
		// (internal). With pressure fired the demand gate opens; hostBusyRatio
		// 3.2/4.0 = 0.80 > HostBusyHigh 0.70 fires host-contention (external).
		Decide(st, Sample{
			Timestamp:     base,
			PressureAvg60: 0.21,
			HostBusyCores: 3.2, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		// HOLD tick 1 — pressure 0.13 (hold band [0.12, 0.20), latch stays
		// fired) and hostBusyRatio 2.76/4.0 = 0.69 (<= 0.70, but latch stays
		// fired: demand gate open via pressure). Both held → raw severities
		// negative.
		//   pressure sev  = (0.13 - 0.20) / 0.80 = -0.0875
		//   host-cont sev = (0.69 - 0.70) / 0.30 = -0.0333
		// Without the < 0 → 0 clamp: host-contention (-0.0333) > pressure
		// (-0.0875) → host-contention dominant → HOST.
		// With the clamp: both → 0 → tie → external wins → HOST.
		v1, sig1 := Decide(st, Sample{
			Timestamp:     base.Add(1 * time.Second),
			PressureAvg60: 0.13,
			HostBusyCores: 2.76, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		if !sig1.PressureFired || !sig1.HostContentionFired {
			t.Fatalf("hold1 setup: PressureFired=%v HostContentionFired=%v, want both held true", sig1.PressureFired, sig1.HostContentionFired)
		}
		if v1.Attribution != AttributionHost {
			t.Fatalf("hold1 Attribution: got %q, want %q (both held → severity clamped to 0 → tie → external (host-contention) wins → host)", v1.Attribution, AttributionHost)
		}
		// HOLD tick 2 — pressure 0.19 (hold band) and hostBusyRatio 2.0/4.0 =
		// 0.50 (still <= 0.70, latch held via demand gate). The RAW-severity
		// ordering SWAPS: pressure is now less negative than host-contention.
		//   pressure sev  = (0.19 - 0.20) / 0.80 = -0.0125
		//   host-cont sev = (0.50 - 0.70) / 0.30 = -0.6667
		// Without the clamp: pressure (-0.0125) > host-contention (-0.6667)
		// → pressure dominant → UNKNOWN → Attribution FLAPS from HOST.
		// With the clamp: both → 0 → tie → external wins → HOST (stable).
		v2, sig2 := Decide(st, Sample{
			Timestamp:     base.Add(2 * time.Second),
			PressureAvg60: 0.19,
			HostBusyCores: 2.0, LogicalCpus: 4.0, UsageCores: 0.5,
		}, thresholds)
		if !sig2.PressureFired || !sig2.HostContentionFired {
			t.Fatalf("hold2 setup: PressureFired=%v HostContentionFired=%v, want both held true", sig2.PressureFired, sig2.HostContentionFired)
		}
		if v2.Attribution != AttributionHost {
			t.Fatalf("hold2 Attribution: got %q, want %q (held-band jitter must NOT flap attribution; both clamped to 0 → external wins → host)", v2.Attribution, AttributionHost)
		}
	})
}

// TestDecide_UsagePercentiles_FromSaturationRing pins the avg/p95/p99
// usage-fraction fields derived from WindowState.usageRing (the dead-zone
// saturation backstop's 60s usage ring) and asserts they are observability-only
// (do not change the verdict). The caller multiplies each fraction by 1000 for
// mCPU relative to 1 core.
//
// Pinned behaviors:
//
//  1. Signals gains AvgUsageFraction, P95UsageFraction, P99UsageFraction float64
//     (fractions relative to 1 core; typically in [0,1] but may exceed 1 when
//     observed usage exceeds CgroupCores — oversubscription).
//  2. avg = mean of the ring entries; p95/p99 = nearest-rank
//     (sort, rank = ceil(p*N), value at sorted[rank-1]).
//  3. Computed UNCONDITIONALLY whenever the ring holds >= 2 entries — observable
//     regardless of latch state (the ThrottleRatio observability discipline).
//  4. When the ring holds < 2 entries (first tick, or post-reset empty) → all 0.
//  5. These are observability-only: they do NOT change the verdict. The
//     saturation latch still fires on the AVG (not p95) per the spec.
//  6. Computed from the SAME ring the saturation backstop uses (no separate ring).
//     Outside the dead-zone the usage ring is cleared, so the percentiles are 0
//     there (usage is not a health signal outside the dead-zone currently).
//
// The ramp: feed 5 dead-zone ticks at fractions 0.1, 0.5, 0.9, 0.3, 0.7 (all
// within the 60s window) and then a 6th tick (0.1 at t=50s), also within the
// window, so the ring holds all 6 entries [0.1, 0.5, 0.9, 0.3, 0.7, 0.1].
// Sorted = [0.1, 0.1, 0.3, 0.5, 0.7, 0.9]; avg = 2.6/6 = 0.4333;
// p95 rank = ceil(0.95*6) = ceil(5.7) = 6 → sorted[5] = 0.9;
// p99 rank = ceil(0.99*6) = ceil(5.94) = 6 → sorted[5] = 0.9. avg 0.4333 < 0.70 →
// the saturation latch does NOT fire, so the verdict stays StateHealthy — the
// percentiles are surfaced on a healthy verdict (observability-only).
func TestDecide_UsagePercentiles_FromSaturationRing(t *testing.T) {
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// deadZoneSample constructs a dead-zone sample (Quota nil, PsiAvailable
	// false, Virtualized false) with a usage fraction = usageCores/4.0. The
	// dead-zone is the ONLY place the usage ring is populated currently.
	deadZoneSample := func(dt time.Duration, fraction float64) Sample {
		return Sample{
			Timestamp:    base.Add(dt),
			UsageCores:   fraction * 4.0,
			CgroupCores:  4.0,
			PsiAvailable: false,
			Virtualized:  false,
		}
	}

	// (1) FIRST-TICK floor — a single dead-zone sample leaves the ring with 1
	// entry (< 2), so avg/p95/p99 MUST all be 0. A lone first-tick reading
	// carries no percentile information.
	st1 := &WindowState{}
	_, sig1 := Decide(st1, deadZoneSample(0, 0.9), thresholds)
	if sig1.AvgUsageFraction != 0 {
		t.Fatalf("first-tick AvgUsageFraction: got %v, want 0 (ring holds < 2 entries → no percentile)", sig1.AvgUsageFraction)
	}
	if sig1.P95UsageFraction != 0 {
		t.Fatalf("first-tick P95UsageFraction: got %v, want 0 (ring holds < 2 entries → no percentile)", sig1.P95UsageFraction)
	}
	if sig1.P99UsageFraction != 0 {
		t.Fatalf("first-tick P99UsageFraction: got %v, want 0 (ring holds < 2 entries → no percentile)", sig1.P99UsageFraction)
	}

	// (2) RAMP — feed the 5-fraction ramp. All 5 ticks land within the 60s
	// window (10s spacing → cutoff at t=40s keeps t=40..t=80, but the ramp is
	// t=0..t=40 so all 5 are retained). After the final tick the ring holds
	// [0.1, 0.5, 0.9, 0.3, 0.7] → avg 0.5, p95 0.9, p99 0.9.
	st := &WindowState{}
	ramp := []float64{0.1, 0.5, 0.9, 0.3, 0.7}
	for i, frac := range ramp {
		Decide(st, deadZoneSample(time.Duration(i*10)*time.Second, frac), thresholds)
	}
	v, sig := Decide(st, deadZoneSample(50*time.Second, 0.1), thresholds)

	// The 6th tick (0.1 at t=50s) is within the 60s window, so the ring now
	// holds 6 entries: [0.1, 0.5, 0.9, 0.3, 0.7, 0.1]. Recompute the expected
	// values over the 6-entry ring so the assertion is exact, not approximate.
	// Sorted = [0.1, 0.1, 0.3, 0.5, 0.7, 0.9]; avg = 2.6/6 = 0.4333...;
	// p95 rank = ceil(0.95*6) = ceil(5.7) = 6 → sorted[5] = 0.9;
	// p99 rank = ceil(0.99*6) = ceil(5.94) = 6 → sorted[5] = 0.9.
	wantAvg := 0.0
	for _, frac := range []float64{0.1, 0.5, 0.9, 0.3, 0.7, 0.1} {
		wantAvg += frac
	}
	wantAvg /= 6.0
	if !floatEq(sig.AvgUsageFraction, wantAvg) {
		t.Fatalf("AvgUsageFraction: got %v, want %v (mean of the 6-entry usage ring)", sig.AvgUsageFraction, wantAvg)
	}
	if !floatEq(sig.P95UsageFraction, 0.9) {
		t.Fatalf("P95UsageFraction: got %v, want 0.9 (nearest-rank p95: rank=ceil(0.95*6)=6 → sorted[5]=0.9)", sig.P95UsageFraction)
	}
	if !floatEq(sig.P99UsageFraction, 0.9) {
		t.Fatalf("P99UsageFraction: got %v, want 0.9 (nearest-rank p99: rank=ceil(0.99*6)=6 → sorted[5]=0.9)", sig.P99UsageFraction)
	}

	// (3) FRACTIONS, not mCPU — non-negative fractions relative to 1 core
	// (the caller multiplies by 1000 for the wire). They are typically in
	// [0,1] but may exceed 1 when observed usage exceeds CgroupCores
	// (oversubscription); only the lower bound is asserted here, and the
	// oversubscription case below pins the >1 behavior.
	if sig.AvgUsageFraction < 0 {
		t.Fatalf("AvgUsageFraction negative: got %v (fractions are non-negative)", sig.AvgUsageFraction)
	}
	if sig.P95UsageFraction < 0 {
		t.Fatalf("P95UsageFraction negative: got %v (fractions are non-negative)", sig.P95UsageFraction)
	}
	if sig.P99UsageFraction < 0 {
		t.Fatalf("P99UsageFraction negative: got %v (fractions are non-negative)", sig.P99UsageFraction)
	}

	// (3b) OVERSUBSCRIPTION — a dead-zone sample whose UsageCores exceeds
	// CgroupCores yields a fraction > 1.0 (multi-core mCPU after *1000). The
	// input clamp rejects only NaN/+Inf/negative, so a finite >1.0 value
	// passes through into the ring and surfaces on the wire; the caller must
	// NOT add an upper clamp (the >1.0 value is meaningful). With
	// UsageCores=2.0 and CgroupCores=0.5 the fraction is 4.0; feed two such
	// samples so the ring holds >= 2 entries and all three percentiles pin to
	// 4.0.
	stOvs := &WindowState{}
	ovsSample := func(dt time.Duration) Sample {
		return Sample{
			Timestamp:    base.Add(120 * time.Second).Add(dt),
			UsageCores:   2.0,
			CgroupCores:  0.5,
			PsiAvailable: false,
			Virtualized:  false,
		}
	}
	Decide(stOvs, ovsSample(0), thresholds)
	vOvs, sigOvs := Decide(stOvs, ovsSample(10*time.Second), thresholds)
	if !floatEq(sigOvs.AvgUsageFraction, 4.0) {
		t.Fatalf("oversubscription AvgUsageFraction: got %v, want 4.0 (UsageCores=2.0 / CgroupCores=0.5 — >1.0 is legitimate, no upper clamp)", sigOvs.AvgUsageFraction)
	}
	if !floatEq(sigOvs.P95UsageFraction, 4.0) {
		t.Fatalf("oversubscription P95UsageFraction: got %v, want 4.0", sigOvs.P95UsageFraction)
	}
	if !floatEq(sigOvs.P99UsageFraction, 4.0) {
		t.Fatalf("oversubscription P99UsageFraction: got %v, want 4.0", sigOvs.P99UsageFraction)
	}
	// The oversubscription ring (4.0, 4.0) has avg 4.0 >= HighUsageFraction
	// 0.70, so the saturation latch fires and the verdict degrades — pin the
	// interaction so a regression that suppresses percentiles when
	// SaturationFired=true, or drops the saturation cause when percentiles are
	// populated, cannot pass undetected.
	if !sigOvs.SaturationFired {
		t.Fatalf("oversubscription SaturationFired: got false, want true (avg 4.0 >= HighUsageFraction 0.70)")
	}
	if vOvs.State != StateDegraded {
		t.Fatalf("oversubscription State: got %q, want %q (saturation latch fired)", vOvs.State, StateDegraded)
	}
	if len(vOvs.Causes) < 1 {
		t.Fatalf("oversubscription Causes length: got %d, want >= 1 (saturation cause must be present)", len(vOvs.Causes))
	}

	// (4) OBSERVABILITY-ONLY — the percentiles do NOT change the verdict. With
	// avg 0.4333 < HighUsageFraction 0.70 the saturation latch does not fire, so
	// the verdict is StateHealthy with zero causes. The percentiles are surfaced
	// on a healthy verdict (they are a numeric metric, like ThrottleRatio).
	if v.State != StateHealthy {
		t.Fatalf("State: got %q, want %q (percentiles are observability-only; the saturation latch fires on the AVG (0.4333 < 0.70) not p95, so the verdict stays healthy)", v.State, StateHealthy)
	}
	if len(v.Causes) != 0 {
		t.Fatalf("Causes length: got %d, want 0 (percentiles do not introduce a cause; observability-only)", len(v.Causes))
	}
	if sig.SaturationFired {
		t.Fatalf("SaturationFired: got true, want false (the percentiles do NOT fire the saturation latch; avg 0.4333 < 0.70)")
	}

	// (5) p99 >= p95 — the real nearest-rank invariant (both pick high-rank
	// indices, so p99 is never below p95). p95 >= avg is NOT a universal
	// invariant of nearest-rank at larger N (e.g. 19 zeros + one spike: mean
	// 50 but p95 rank resolves to a zero), so it is deliberately not asserted
	// here; p99 >= p95 holds by nearest-rank construction (both index the same
	// sorted slice at high ranks), not by a runtime check — the test pins the
	// invariant so a formula change that breaks it fails here.
	if sig.P99UsageFraction < sig.P95UsageFraction {
		t.Fatalf("p99 < p95: p99=%v p95=%v (p99 must be >= p95 by nearest-rank construction)", sig.P99UsageFraction, sig.P95UsageFraction)
	}

	// (6) OUTSIDE THE DEAD-ZONE — the usage ring is cleared on a non-dead-zone
	// tick, so the percentiles MUST be 0 there (usage is not a health signal
	// outside the dead-zone currently). Re-use the populated st and flip to a
	// capped, PSI-available sample on the SAME WindowState.
	quota := 2.0
	_, sigNdz := Decide(st, Sample{
		Timestamp:    base.Add(60 * time.Second),
		UsageCores:   1.9,
		Quota:        &quota,
		PsiAvailable: true,
		Virtualized:  false,
	}, thresholds)
	if sigNdz.AvgUsageFraction != 0 {
		t.Fatalf("non-dead-zone AvgUsageFraction: got %v, want 0 (usage ring cleared outside the dead-zone → no percentile)", sigNdz.AvgUsageFraction)
	}
	if sigNdz.P95UsageFraction != 0 {
		t.Fatalf("non-dead-zone P95UsageFraction: got %v, want 0 (usage ring cleared outside the dead-zone → no percentile)", sigNdz.P95UsageFraction)
	}
	if sigNdz.P99UsageFraction != 0 {
		t.Fatalf("non-dead-zone P99UsageFraction: got %v, want 0 (usage ring cleared outside the dead-zone → no percentile)", sigNdz.P99UsageFraction)
	}

	// (7) p95 vs p99 DISTINGUISHED — at N=6 (and N=5) both ranks collapse to
	// the same sorted index, so a p99-formula regression (e.g. reusing the
	// 0.95 multiplier) would pass undetected. A larger ring (N=100) separates
	// the ranks: p95 rank = ceil(0.95*100) = 95 → sorted[94]; p99 rank =
	// ceil(0.99*100) = 99 → sorted[98]. Feed 100 distinct fractions
	// 0.01..1.00 so the two percentiles land on different values and are
	// independently validated.
	stBig := &WindowState{}
	bigFracs := make([]float64, 100)
	var sigBig Signals
	for i := 1; i <= 100; i++ {
		frac := float64(i) / 100.0
		bigFracs[i-1] = frac
		_, sigBig = Decide(stBig, Sample{
			Timestamp:    base.Add(180 * time.Second).Add(time.Duration(i-1) * 500 * time.Millisecond),
			UsageCores:   frac,
			CgroupCores:  1.0,
			PsiAvailable: false,
			Virtualized:  false,
		}, thresholds)
	}
	wantBigAvg := 0.0
	for _, f := range bigFracs {
		wantBigAvg += f
	}
	wantBigAvg /= 100.0
	if !floatEq(sigBig.AvgUsageFraction, wantBigAvg) {
		t.Fatalf("large-N AvgUsageFraction: got %v, want %v (mean of the 100-entry ring)", sigBig.AvgUsageFraction, wantBigAvg)
	}
	if !floatEq(sigBig.P95UsageFraction, 0.95) {
		t.Fatalf("large-N P95UsageFraction: got %v, want 0.95 (rank=ceil(0.95*100)=95 → sorted[94]=0.95)", sigBig.P95UsageFraction)
	}
	if !floatEq(sigBig.P99UsageFraction, 0.99) {
		t.Fatalf("large-N P99UsageFraction: got %v, want 0.99 (rank=ceil(0.99*100)=99 → sorted[98]=0.99 — distinct from p95, guards a p99-formula regression)", sigBig.P99UsageFraction)
	}
}

// TestDecide_StealAndPressure_UnconditionalSignals pins the PR1 observability
// widening: the steal p95 (the existing stealP95Val local) and the pressure
// value (sample.PressureAvg60) are surfaced as UNCONDITIONAL Signals fields
// (Signals.StealP95, Signals.PressureAvg60Out), populated regardless of the
// StealFired/PressureFired latch state — exactly the pattern Signals.ThrottleRatio
// already follows (read off signals even when ThrottleFired is false). This is
// observability-only: the verdict (State/Attribution/Causes) is NOT changed by
// these fields, so each sub-case also pins that the verdict stays healthy with
// no causes when only sub-threshold steal/pressure are present.
//
//  1. STEAL BELOW FIRE → STILL REPORTED. On a virtualized box, a sustained
//     steal of 0.08 keeps the steal latch UNFIRED (p95 0.08 is not > StealHigh
//     0.10), yet Signals.StealP95 must carry the p95 0.08. (The latch state and
//     the numeric metric are decoupled, like ThrottleRatio vs ThrottleFired.)
//  2. PRESSURE BELOW FIRE → STILL REPORTED. A PressureAvg60 of 0.10 keeps the
//     pressure latch UNFIRED (0.10 is not > PressureHigh 0.20), yet
//     Signals.PressureAvg60Out must carry 0.10.
//  3. SOURCE ABSENT → 0. A bare-metal sample (Virtualized=false) reports
//     StealP95 0 (steal is not a readable signal), and a sample with no PSI
//     pressure (PressureAvg60 0) reports PressureAvg60Out 0.
//  4. NEGATIVE CLAMP → 0. A negative PressureAvg60 is clamped to 0 on the
//     PressureAvg60Out field (mirroring the ThrottleRatio negative clamp).
func TestDecide_StealAndPressure_UnconditionalSignals(t *testing.T) {
	base := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// (1) STEAL BELOW FIRE → STILL REPORTED. 20 sustained ticks at 0.08 on a
	// virtualized box: p95 = 0.08, below StealHigh 0.10, so the latch never
	// fires — but the numeric p95 must still be surfaced on the wire.
	st1 := &WindowState{}
	var sig1 Signals
	var v1 Verdict
	for i := 0; i < 20; i++ {
		v1, sig1 = Decide(st1, Sample{
			Timestamp:     base.Add(time.Duration(i) * time.Second),
			StealFraction: 0.08,
			Virtualized:   true,
		}, thresholds)
	}
	if sig1.StealFired {
		t.Fatalf("steal-below StealFired: got true, want false (p95 0.08 is not > StealHigh 0.10)")
	}
	if !floatEq(sig1.StealP95, 0.08) {
		t.Fatalf("steal-below StealP95: got %v, want 0.08 (the p95 must be surfaced UNCONDITIONALLY even when the latch has not fired, mirroring ThrottleRatio)", sig1.StealP95)
	}
	if v1.State != StateHealthy {
		t.Fatalf("steal-below State: got %q, want %q (observability field must not change the verdict)", v1.State, StateHealthy)
	}
	if len(v1.Causes) != 0 {
		t.Fatalf("steal-below Causes: got %+v, want none (sub-threshold steal is not a cause)", v1.Causes)
	}

	// (2) PRESSURE BELOW FIRE → STILL REPORTED. PressureAvg60 0.10 is below
	// PressureHigh 0.20 (and below PressureRecover 0.12), so the latch never
	// fires — but the value must still be surfaced.
	st2 := &WindowState{}
	v2, sig2 := Decide(st2, Sample{
		Timestamp:     base.Add(200 * time.Second),
		PressureAvg60: 0.10,
	}, thresholds)
	if sig2.PressureFired {
		t.Fatalf("pressure-below PressureFired: got true, want false (0.10 is not > PressureHigh 0.20)")
	}
	if !floatEq(sig2.PressureAvg60Out, 0.10) {
		t.Fatalf("pressure-below PressureAvg60Out: got %v, want 0.10 (the pressure value must be surfaced UNCONDITIONALLY even when the latch has not fired)", sig2.PressureAvg60Out)
	}
	if v2.State != StateHealthy {
		t.Fatalf("pressure-below State: got %q, want %q (observability field must not change the verdict)", v2.State, StateHealthy)
	}
	if len(v2.Causes) != 0 {
		t.Fatalf("pressure-below Causes: got %+v, want none (sub-threshold pressure is not a cause)", v2.Causes)
	}

	// (3) SOURCE ABSENT → 0. Bare metal (Virtualized=false) → steal not a
	// readable signal → StealP95 0. No PSI pressure (PressureAvg60 0) →
	// PressureAvg60Out 0.
	st3 := &WindowState{}
	_, sig3 := Decide(st3, Sample{
		Timestamp:     base.Add(400 * time.Second),
		StealFraction: 0.50, // nonzero, but Virtualized=false so it is not processed
		Virtualized:   false,
		PressureAvg60: 0.0,
	}, thresholds)
	if !floatEq(sig3.StealP95, 0) {
		t.Fatalf("absent StealP95: got %v, want 0 (bare metal → steal is not a readable signal → 0)", sig3.StealP95)
	}
	if !floatEq(sig3.PressureAvg60Out, 0) {
		t.Fatalf("absent PressureAvg60Out: got %v, want 0 (no PSI pressure → 0)", sig3.PressureAvg60Out)
	}

	// (4) NEGATIVE CLAMP → 0. A negative PressureAvg60 is clamped to 0 before
	// exposure, mirroring the ThrottleRatio negative clamp.
	st4 := &WindowState{}
	_, sig4 := Decide(st4, Sample{
		Timestamp:     base.Add(600 * time.Second),
		PressureAvg60: -0.5,
	}, thresholds)
	if !floatEq(sig4.PressureAvg60Out, 0) {
		t.Fatalf("negative-clamp PressureAvg60Out: got %v, want 0 (negatives clamp to 0, mirroring the ThrottleRatio clamp)", sig4.PressureAvg60Out)
	}
}
