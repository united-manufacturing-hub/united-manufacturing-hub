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

// Run with -tags=test: this file is excluded from plain go test, which
// reports green with 0 specs. CI passes -tags=test for this package.
package cpuhealth

import (
	"math"
	"strings"
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
// (no PSI). Sample.PsiAvailable is
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
	// evaluated. R10.3: sets LogicalCpus=4.0 and HostBusyCoresAvailable=false
	// so the sample routes to the no-host-stats saturation fraction fallback
	// (usageCores60sMean/LogicalCpus), which replaces the old
	// CgroupCores-denominator fraction branch (removed in R10.3 — it was doubly
	// dead: unreachable AND fraction=0 in no-limit). LogicalCpus=4.0 matches
	// CgroupCores=4.0 so the no-host-stats saturation fraction equals the old saturationAvg
	// (UsageCores/4.0), preserving the test's intent.
	deadZoneSample := func(dt time.Duration, usageCores float64) Sample {
		return Sample{
			Timestamp:              base.Add(dt),
			UsageCores:             usageCores,
			CgroupCores:            4.0,
			LogicalCpus:            4.0,
			HostBusyCoresAvailable: false,
			PsiAvailable:           false,
			Virtualized:            false,
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

	// (5) NON-DEAD-ZONE (limit mode) — a limited container (Quota=2.0) at 95%
	// usage (UsageCores=1.9) with no throttle. Under the two-rule model (R10.1),
	// limit-mode headroom = 2−1.9−0.2=−0.1 < 0 → degraded + saturation cause.
	// The old test expected healthy ("busy, not sick"), which was correct under
	// the old model but is wrong under the two-rule model: sustained usage inside
	// the fractional reserve band IS degraded. PsiAvailable true → NOT the
	// dead-zone → LimitedVisibility false.
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
	if vNonDz.State != StateDegraded {
		t.Fatalf("non-dead-zone State: got %q, want %q (limit mode: headroom=2−1.9−0.2=−0.1 < 0 → degraded; sustained usage inside the fractional reserve band IS degraded under the two-rule model)", vNonDz.State, StateDegraded)
	}
	hasSat := false
	for _, c := range vNonDz.Causes {
		if c.Kind == CauseKindSaturation {
			hasSat = true
		}
	}
	if !hasSat {
		t.Fatalf("non-dead-zone Causes: no saturation in %v (limit-mode headroom < 0 must emit saturation)", vNonDz.Causes)
	}
	// LimitedVisibility is false outside the dead-zone (PSI is available → not
	// a blind state).
	if sigNonDz.LimitedVisibility {
		t.Fatalf("non-dead-zone LimitedVisibility: got true, want false (PSI available → not a blind state → no limited-visibility signal)")
	}

	// (7) TRANSITION-OUT-OF-DEAD-ZONE — fire saturation in the dead-zone, then
	// transition to limit mode (Quota=2.0, PsiAvailable=true, UsageCores=1.9).
	// Under the two-rule model (R10.1), the ring fills every tick in all modes
	// (not cleared on dead-zone exit), so the old dead-zone samples (cores=3.2)
	// persist. The limit-mode headroom = 2 − usageCores60sMean(~3.0) − 0.2 =
	// ~−1.2 < 0 → saturation STILL fires. This is CORRECT: the container's
	// 60s-avg usage (3.0) exceeds its new 2.0 limit, so it IS over its limit.
	// The old test expected the latch to clear on dead-zone exit (ring was
	// cleared), but the ring now fills every tick, and the limit-mode headroom
	// correctly fires. See the spec-vs-code conflict surfaced in the R10.1
	// return: the ring-fill move changes transition behavior.
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
		UsageCores:   1.9, // 0.95 of 2.0 quota
		Quota:        &quota4,
		PsiAvailable: true,
		Virtualized:  false,
	}, thresholds)
	// The ring retains old dead-zone samples (cores=3.2); usageCores60sMean
	// = (6*3.2+1.9)/7 ≈ 3.0 > 1.8 (quota − reserve), so headroom < 0 →
	// saturation fires. The container's 60s-avg usage exceeds its new limit.
	if vTrans.State != StateDegraded {
		t.Fatalf("transition State: got %q, want %q (limit-mode headroom = 2 − usageCores60sMean(~3.0) − 0.2 < 0 → degraded; the container's 60s-avg usage exceeds its new 2.0 limit)", vTrans.State, StateDegraded)
	}
	if !sigTrans.SaturationFired {
		t.Fatalf("transition SaturationFired: got false, want true (limit-mode headroom < 0; old dead-zone usage samples persist in the ring)")
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
		Timestamp:              base,
		UsageCores:             math.NaN(),
		CgroupCores:            4.0,
		LogicalCpus:            4.0,
		HostBusyCoresAvailable: false,
		PsiAvailable:           false,
		Virtualized:            false,
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

	// (12) RE-ENTRY RING (latch reset across a mode transition) — fire
	// saturation in the dead-zone, exit to a limit-mode sample, then RE-ENTER
	// the dead-zone with low usage. This pins two design facts:
	//
	// (a) The limit-mode branch (decide.go:925-928) intentionally clears the
	// no-limit sub-latches (noHostStatsSaturationFired, noLimitHostFired) so a
	// prior no-limit fire does not leak into limit mode (Finding: prevent
	// leaking a prior no-limit fire into limit mode). So on dead-zone re-entry
	// the latch starts false, NOT held over from the dead-zone fire.
	//
	// (b) The Schmitt hold band [SaturationRecover 0.60, HighUsageFraction
	// 0.70) preserves the CURRENT latch value: it fires only at >= 0.70 and
	// clears only at < 0.60. Within a single mode session this is what makes a
	// fired latch hold through the band. Across a mode transition the latch
	// resets (see (a)), so on re-entry the hold band preserves the reset value
	// (false) -> Healthy. The Schmitt hold does NOT cross mode transitions.
	//
	// Note on noHostStatsSaturationFraction: usageCores60sMean/LogicalCpus is denominator-
	// independent (LogicalCpus is constant across transitions), so the prior
	// skip rationale ("mixed-denominator fractions") was stale. The real reason
	// the re-entry verdict is Healthy is the latch clearing at the t=90
	// limit-mode tick, not the denominator.
	//
	// The t=90 limit-mode tick itself fires a TRANSIENT FALSE FIRE: stale
	// dead-zone samples (3.2 cores) inflate usageCores60sMean to ~3.0, making
	// limit-mode headroom (quota 2.0 - 3.0 - reserve 0.2) negative, so
	// limitSaturationFired fires even though instantaneous usage (1.9) is
	// within quota. This self-corrects as the stale samples age out (<=60s). It
	// is a known wart, acceptable for the rare dead-zone->limit-mode
	// transition; flagged as a follow-up if mode transitions become common.
	st9 := &WindowState{}
	for i := 0; i < 8; i++ {
		Decide(st9, deadZoneSample(time.Duration(i*10)*time.Second, 3.2), thresholds) // 0.80
	}
	vFire9, _ := Decide(st9, deadZoneSample(80*time.Second, 3.2), thresholds)
	if vFire9.State != StateDegraded {
		t.Fatalf("re-entry fire State: got %q, want %q (saturation fires before exit)", vFire9.State, StateDegraded)
	}
	// Exit the dead-zone to a limit-mode tick (Quota set, PSI available). The
	// limit-mode branch clears the no-limit sub-latches
	// (noHostStatsSaturationFired, noLimitHostFired) at decide.go:925-928. The
	// tick itself fires a transient false fire (stale dead-zone samples inflate
	// usageCores60sMean -> negative limit headroom); pin the verdict to codify
	// the actual behavior.
	quota9 := 2.0
	vExit, sigExit := Decide(st9, Sample{
		Timestamp:    base.Add(90 * time.Second),
		UsageCores:   1.9,
		Quota:        &quota9,
		PsiAvailable: true,
		Virtualized:  false,
	}, thresholds)
	if vExit.State != StateDegraded {
		t.Fatalf("re-entry exit State (t=90): got %q, want %q (transient false fire: stale dead-zone samples inflate limit-mode headroom negative; self-corrects <=60s)", vExit.State, StateDegraded)
	}
	if !sigExit.LimitSaturationFired {
		t.Fatalf("re-entry exit LimitSaturationFired (t=90): got false, want true (transient false fire: stale 3.2-core samples -> usageCores60sMean ~3.0 -> headroom 2.0-3.0-0.2 < 0)")
	}
	// Re-enter the dead-zone with low usage (1.6 cores -> 0.40 fraction). The
	// latch was cleared at t=90, so it starts false. noHostStatsSaturationFraction (~0.6964 from
	// stale+fresh samples) lands in the Schmitt hold band [0.60, 0.70) which
	// preserves the current value (false) -> Healthy. This pins that the
	// Schmitt hold does NOT cross mode transitions: the latch reset, not the
	// denominator, is why the re-entry verdict is Healthy.
	vRe, sigRe := Decide(st9, deadZoneSample(100*time.Second, 1.6), thresholds)
	if vRe.State != StateHealthy {
		t.Fatalf("re-entry State (t=100): got %q, want %q (latch cleared at t=90; noHostStatsSaturationFraction in hold band [0.60,0.70) preserves false -> Healthy)", vRe.State, StateHealthy)
	}
	if sigRe.SaturationFired {
		t.Fatalf("re-entry SaturationFired (t=100): got true, want false (latch was cleared at t=90 limit-mode tick; hold band preserves the reset value)")
	}
	// Steady state: stale samples age out naturally (cutoff=110), the ring
	// holds only 0.40-fraction samples, noHostStatsSaturationFraction 0.40 < SaturationRecover
	// 0.60 -> latch stays false -> healthy.
	for i := 1; i < 8; i++ {
		Decide(st9, deadZoneSample(time.Duration(100+i*10)*time.Second, 1.6), thresholds) // 0.40
	}
	vSteady, sigSteady := Decide(st9, deadZoneSample(170*time.Second, 1.6), thresholds)
	if vSteady.State != StateHealthy {
		t.Fatalf("re-entry steady State (t=170): got %q, want %q (stale samples aged out; ring holds only 0.40 samples -> avg 0.40 < 0.60 -> healthy)", vSteady.State, StateHealthy)
	}
	if sigSteady.SaturationFired {
		t.Fatalf("re-entry steady SaturationFired (t=170): got true, want false (stale samples aged out; avg 0.40 < SaturationRecover -> latch clears)")
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
			Timestamp:              base.Add(dt),
			UsageCores:             3.2, // 0.80 of 4.0
			CgroupCores:            4.0,
			LogicalCpus:            4.0,
			HostBusyCoresAvailable: false,
			NrPeriods:              nrPeriods,
			NrThrottled:            nrThrottled,
			PsiAvailable:           false,
			Virtualized:            false,
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
		t.Fatalf("co-fire Attribution: got %q, want %q (throttle and saturation both internal; HostBusyCores=0 makes the host share 0−3.2 negative, so the split resolves to Unknown)", vCo.Attribution, AttributionUnknown)
	}
	if !sigCo.ThrottleFired || !sigCo.SaturationFired {
		t.Fatalf("co-fire latches: ThrottleFired=%v SaturationFired=%v, want both true", sigCo.ThrottleFired, sigCo.SaturationFired)
	}

	// (14) CO-FIRE ATTRIBUTION REPIVOT — same throttle-dominant co-fire as
	// (13), but HostBusyCores=8.0 (>2*UsageCores=6.4) so the host/container
	// split resolves to Host (host share 8.0−3.2=4.8 > our share 3.2). This
	// pins that the split applies to a throttle-dominant internal cause too
	// (not only pure saturation): a refactor that gates the split on
	// fired[0].Kind==CauseKindSaturation, or drops the repivot for
	// throttle-dominant, would leave attribution Unknown and fail here.
	// LogicalCpus stays 0 so the demand-gated host-contention latch is skipped
	// (LogicalCpus<=0 guard) and cannot fire to make the cause external.
	highHostSample := func(dt time.Duration, nrPeriods, nrThrottled int64) Sample {
		s := deadZoneThrottleSample(dt, nrPeriods, nrThrottled)
		s.HostBusyCores = 8.0
		s.HostBusyCoresAvailable = true // ring fills with 8.0 → 60s mean = 8.0 (the smoothed split uses the mean, not the instantaneous value)
		return s
	}
	st10b := &WindowState{}
	for i := 0; i < 8; i++ {
		Decide(st10b, highHostSample(time.Duration(i*10)*time.Second, int64(i+1)*1000, int64(i+1)*800), thresholds)
	}
	vCoB, sigCoB := Decide(st10b, highHostSample(80*time.Second, 9000, 7200), thresholds)
	if vCoB.State != StateDegraded {
		t.Fatalf("co-fire repivot State: got %q, want %q", vCoB.State, StateDegraded)
	}
	if vCoB.Causes[0].Kind != CauseKindThrottling {
		t.Fatalf("co-fire repivot causes[0]: got %q, want %q (throttle stays dominant)", vCoB.Causes[0].Kind, CauseKindThrottling)
	}
	if vCoB.Attribution != AttributionHost {
		t.Fatalf("co-fire repivot Attribution: got %q, want %q (throttle-dominant internal + SaturationFired; host share 4.8 > our share 3.2 → Host via the split)", vCoB.Attribution, AttributionHost)
	}
	hasHostContentionCoB := false
	for _, c := range vCoB.Causes {
		if c.Kind == CauseKindHostContention {
			hasHostContentionCoB = true
		}
	}
	if hasHostContentionCoB {
		t.Fatalf("co-fire repivot Causes: CauseKindHostContention present, want absent (LogicalCpus=0 skips the host-contention latch; got %v)", vCoB.Causes)
	}
	if !sigCoB.ThrottleFired || !sigCoB.SaturationFired {
		t.Fatalf("co-fire repivot latches: ThrottleFired=%v SaturationFired=%v, want both true", sigCoB.ThrottleFired, sigCoB.SaturationFired)
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
	// false, Virtualized false) with a usage fraction = usageCores/4.0. R10.3:
	// sets LogicalCpus=4.0 and HostBusyCoresAvailable=false so the sample
	// routes to the no-host-stats saturation (usageCores60sMean/LogicalCpus), preserving the
	// test's fraction semantics (LogicalCpus matches CgroupCores=4.0).
	deadZoneSample := func(dt time.Duration, fraction float64) Sample {
		return Sample{
			Timestamp:              base.Add(dt),
			UsageCores:             fraction * 4.0,
			CgroupCores:            4.0,
			LogicalCpus:            4.0,
			HostBusyCoresAvailable: false,
			PsiAvailable:           false,
			Virtualized:            false,
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
			Timestamp:              base.Add(120 * time.Second).Add(dt),
			UsageCores:             2.0,
			CgroupCores:            0.5,
			LogicalCpus:            0.5,
			HostBusyCoresAvailable: false,
			PsiAvailable:           false,
			Virtualized:            false,
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

	// (6) OUTSIDE THE DEAD-ZONE — the usage ring now fills EVERY tick in ALL
	// modes (R10.1 ring-fill move), so the percentiles are NON-ZERO outside
	// the dead-zone (intended enrichment: unblocks the display's "this
	// container" row outside the dead-zone). Re-use the populated st and flip
	// to a capped, PSI-available sample on the SAME WindowState. The ring
	// retains the 6 dead-zone samples [0.1, 0.5, 0.9, 0.3, 0.7, 0.1] plus the
	// new limit-mode sample (fraction=1.9/2.0=0.95) → 7 entries. Sorted =
	// [0.1, 0.1, 0.3, 0.5, 0.7, 0.9, 0.95]; avg = 3.55/7 ≈ 0.507; p95 rank =
	// ceil(0.95*7) = 7 → sorted[6] = 0.95; p99 rank = ceil(0.99*7) = 7 →
	// sorted[6] = 0.95.
	quota := 2.0
	_, sigNdz := Decide(st, Sample{
		Timestamp:    base.Add(60 * time.Second),
		UsageCores:   1.9,
		Quota:        &quota,
		PsiAvailable: true,
		Virtualized:  false,
	}, thresholds)
	if !sigNdz.UsageRingActive {
		t.Fatalf("non-dead-zone UsageRingActive: got false, want true (ring fills every tick in all modes; >= 2 samples present)")
	}
	wantNdzAvg := (0.1 + 0.5 + 0.9 + 0.3 + 0.7 + 0.1 + 0.95) / 7.0
	if !floatEq(sigNdz.AvgUsageFraction, wantNdzAvg) {
		t.Fatalf("non-dead-zone AvgUsageFraction: got %v, want %v (ring NOT cleared outside the dead-zone; 7 samples: 6 dead-zone + 1 limit-mode)", sigNdz.AvgUsageFraction, wantNdzAvg)
	}
	if !floatEq(sigNdz.P95UsageFraction, 0.95) {
		t.Fatalf("non-dead-zone P95UsageFraction: got %v, want 0.95 (rank=ceil(0.95*7)=7 → sorted[6]=0.95)", sigNdz.P95UsageFraction)
	}
	if !floatEq(sigNdz.P99UsageFraction, 0.95) {
		t.Fatalf("non-dead-zone P99UsageFraction: got %v, want 0.95 (rank=ceil(0.99*7)=7 → sorted[6]=0.95)", sigNdz.P99UsageFraction)
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

// TestDecide_StealAndPressure_UnconditionalSignals pins the new unconditional
// Signals fields: the steal p95 (the existing stealP95Val local) and the
// pressure value (sample.PressureAvg60) populate Signals.StealP95 and
// Signals.PressureAvg60Out regardless of the StealFired/PressureFired latch
// state, the same way Signals.ThrottleRatio populates regardless of
// ThrottleFired. The verdict (State/Attribution/Causes) does not change, so
// each sub-case also pins that the verdict stays healthy with no causes when
// only sub-threshold steal/pressure are present.
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

// TestDecide_HostBusy60sMean_RingDiscipline pins the 60s sliding-window MEAN
// of per-tick Sample.HostBusyCores that Decide maintains on a hostBusyRing in
// WindowState, exposed as Signals.HostBusyCores60sMean. It mirrors the steal
// ring discipline (decide.go:459-491): append-per-tick, prune entries older
// than 60s with the cutoff sample KEPT (Before(cutoff) is false), a 2-sample
// floor emitting 0, and NaN/negative/+Inf clamped to 0 BEFORE insert (an
// INPUT clamp, distinct from the steal ring's OUTPUT clamp on the p95).
func TestDecide_HostBusy60sMean_RingDiscipline(t *testing.T) {
	base := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// feedHostBusy pushes n ticks of the given HostBusyCores onto st's
	// hostBusyRing at 1s spacing (well inside the 60s window), with
	// LogicalCpus=8.0, returning the final signals. The demand gate
	// (PressureFired||ThrottleFired) is closed in these samples (PressureAvg60=0,
	// throttle counters unset → ratio=0), so no latch is evaluated; the
	// assertion target is solely sig.HostBusyCores60sMean.
	feedHostBusy := func(st *WindowState, n int, startOff time.Duration, vals ...float64) Signals {
		var s Signals
		for i := 0; i < n; i++ {
			v := 0.0
			if i < len(vals) {
				v = vals[i]
			}
			_, s = Decide(st, Sample{
				Timestamp:              base.Add(startOff).Add(time.Duration(i) * time.Second),
				HostBusyCores:          v,
				HostBusyCoresAvailable: true, // R10.3: ring appends only when /proc/stat is readable
				LogicalCpus:            8.0,
			}, thresholds)
		}
		return s
	}

	// (1) WINDOW MEAN — 3 ticks [2.0, 4.0, 6.0] within 60s on a fresh
	// &WindowState{} → arithmetic mean 4.0. Forcing assertion: a stub
	// returning 0 or the last value (6.0) fails (!floatEq 4.0).
	t.Run("WindowMean", func(t *testing.T) {
		st := &WindowState{}
		sig := feedHostBusy(st, 3, 0, 2.0, 4.0, 6.0)
		if !floatEq(sig.HostBusyCores60sMean, 4.0) {
			t.Fatalf("HostBusyCores60sMean: got %v, want 4.0 (mean of [2,4,6] over the 60s window; a stub returning 0 or the last value 6.0 fails)", sig.HostBusyCores60sMean)
		}
	})

	// (2) PRUNE-TO-ONE — A=2.0@t0, B=8.0@t0+61s → 0.0. A is older than 60s
	// (pruned), only B survives, 1 sample < 2-sample floor → 0. Forcing
	// assertion: a non-pruning mean yields (2+8)/2=5.0 (!floatEq 0.0).
	t.Run("PruneToOne", func(t *testing.T) {
		st := &WindowState{}
		_, _ = Decide(st, Sample{
			Timestamp:     base,
			HostBusyCores: 2.0,
			LogicalCpus:   8.0,
		}, thresholds)
		_, sig := Decide(st, Sample{
			Timestamp:     base.Add(61 * time.Second),
			HostBusyCores: 8.0,
			LogicalCpus:   8.0,
		}, thresholds)
		if !floatEq(sig.HostBusyCores60sMean, 0.0) {
			t.Fatalf("HostBusyCores60sMean: got %v, want 0.0 (A@t0 pruned >60s, only B@t0+61s survives, 1 sample < 2-sample floor; a non-pruning mean yields (2+8)/2=5.0)", sig.HostBusyCores60sMean)
		}
	})

	// (3) CUTOFF-KEPT — A=4.0@t0, B=4.0@exactly t0+60s → 4.0. A sits on the
	// cutoff edge and is KEPT per !ts.Before(cutoff), so 2 samples survive →
	// mean 4.0. Forcing assertion: an off-by-one Before/After prune drops A
	// → 1 sample → 0 (!floatEq 4.0).
	t.Run("CutoffKept", func(t *testing.T) {
		st := &WindowState{}
		_, _ = Decide(st, Sample{
			Timestamp:              base,
			HostBusyCores:          4.0,
			HostBusyCoresAvailable: true, // R10.3: ring appends only when /proc/stat is readable
			LogicalCpus:            8.0,
		}, thresholds)
		_, sig := Decide(st, Sample{
			Timestamp:              base.Add(60 * time.Second),
			HostBusyCores:          4.0,
			HostBusyCoresAvailable: true,
			LogicalCpus:            8.0,
		}, thresholds)
		if !floatEq(sig.HostBusyCores60sMean, 4.0) {
			t.Fatalf("HostBusyCores60sMean: got %v, want 4.0 (A@t0 on cutoff edge KEPT per !ts.Before(cutoff), 2 samples survive; an off-by-one Before/After prune drops A → 1 sample → 0)", sig.HostBusyCores60sMean)
		}
	})

	// (4) SINGLE-SAMPLE FLOOR — 1 tick [6.0] → 0.0 (ring size 1 < floor 2).
	// Forcing assertion: without the floor a 1-sample mean is 6.0
	// (sig.HostBusyCores60sMean != 0.0 → Fatalf).
	t.Run("SingleSampleFloor", func(t *testing.T) {
		st := &WindowState{}
		sig := feedHostBusy(st, 1, 0, 6.0)
		if !floatEq(sig.HostBusyCores60sMean, 0.0) {
			t.Fatalf("HostBusyCores60sMean: got %v, want 0.0 (ring size 1 < 2-sample floor; without the floor a 1-sample mean is 6.0)", sig.HostBusyCores60sMean)
		}
	})

	// (5) NAN CLAMP — A=4.0, B=NaN, C=4.0 within 60s → finite AND 8.0/3.0
	// (NaN clamps to 0 before insert → (4+0+4)/3). TWO forcing assertions:
	// math.IsNaN(...) → Fatalf (NaN inserted unclamped poisons the mean for
	// 60s); then !floatEq(..., 8.0/3.0) → Fatalf.
	t.Run("NaNClamp", func(t *testing.T) {
		st := &WindowState{}
		sig := feedHostBusy(st, 3, 0, 4.0, math.NaN(), 4.0)
		if math.IsNaN(sig.HostBusyCores60sMean) {
			t.Fatalf("HostBusyCores60sMean: got NaN, want finite (NaN must clamp to 0 BEFORE insert; an unclamped NaN poisons the 60s running mean)")
		}
		if !floatEq(sig.HostBusyCores60sMean, 8.0/3.0) {
			t.Fatalf("HostBusyCores60sMean: got %v, want 8.0/3.0 (NaN clamps to 0 before insert → (4+0+4)/3)", sig.HostBusyCores60sMean)
		}
	})

	// (5b) NEGATIVE CLAMP — A=1.0, B=-10.0, C=1.0 within 60s → 2.0/3.0. A
	// negative HostBusyCores (e.g. from a malformed /proc/stat parse) clamps
	// to 0 BEFORE insert → (1+0+1)/3. The inputs are chosen so an UNCLAMPED
	// mean is (1-10+1)/3 = -8/3 < 0: TWO forcing assertions each pin a
	// distinct regression — the mean is non-negative (!(mean >= 0) → Fatalf;
	// removing the clamp makes the unclamped mean -8/3 which fires this);
	// then !floatEq(..., 2.0/3.0) → Fatalf (pins the clamped value). Pins the
	// !(hb >= 0) branch so a maintainer who narrows the clamp to math.IsNaN
	// only lets a negative busy-core count poison the mean uncaught by CI.
	t.Run("NegativeClamp", func(t *testing.T) {
		st := &WindowState{}
		sig := feedHostBusy(st, 3, 0, 1.0, -10.0, 1.0)
		if !(sig.HostBusyCores60sMean >= 0) {
			t.Fatalf("HostBusyCores60sMean: got %v, want >= 0 (a negative HostBusyCores must clamp to 0 BEFORE insert; an unclamped negative drives the mean to -8/3)", sig.HostBusyCores60sMean)
		}
		if !floatEq(sig.HostBusyCores60sMean, 2.0/3.0) {
			t.Fatalf("HostBusyCores60sMean: got %v, want 2.0/3.0 (negative -10.0 clamps to 0 before insert → (1+0+1)/3)", sig.HostBusyCores60sMean)
		}
	})

	// (5c) +INF CLAMP — A=4.0, B=+Inf, C=4.0 within 60s → finite AND 8.0/3.0.
	// +Inf clamps to 0 BEFORE insert → (4+0+4)/3. TWO forcing assertions:
	// math.IsInf(..., 1) → Fatalf (+Inf inserted unclamped makes the mean
	// +Inf for 60s); then !floatEq(..., 8.0/3.0) → Fatalf. Pins the
	// math.IsInf(hb, 1) branch of the clamp.
	t.Run("PosInfClamp", func(t *testing.T) {
		st := &WindowState{}
		sig := feedHostBusy(st, 3, 0, 4.0, math.Inf(1), 4.0)
		if math.IsInf(sig.HostBusyCores60sMean, 1) {
			t.Fatalf("HostBusyCores60sMean: got +Inf, want finite (+Inf must clamp to 0 BEFORE insert; an unclamped +Inf makes the 60s running mean +Inf)")
		}
		if !floatEq(sig.HostBusyCores60sMean, 8.0/3.0) {
			t.Fatalf("HostBusyCores60sMean: got %v, want 8.0/3.0 (+Inf clamps to 0 before insert → (4+0+4)/3)", sig.HostBusyCores60sMean)
		}
	})

	// (6) GAP-PRUNE-TO-<2 — 3 samples [4,4,4]→4.0 (setup assert), then one
	// tick @t0+120s (>60s gap prunes the ring to 1 sample → floor re-met) →
	// 0.0. Forcing assertion: a stale-cached mean keeps the old 4.0
	// (sigGap.HostBusyCores60sMean != 0.0 → Fatalf).
	t.Run("GapPruneToUnderTwo", func(t *testing.T) {
		st := &WindowState{}
		sigSetup := feedHostBusy(st, 3, 0, 4.0, 4.0, 4.0)
		if !floatEq(sigSetup.HostBusyCores60sMean, 4.0) {
			t.Fatalf("setup HostBusyCores60sMean: got %v, want 4.0 (mean of [4,4,4] within 60s before the gap)", sigSetup.HostBusyCores60sMean)
		}
		_, sigGap := Decide(st, Sample{
			Timestamp:     base.Add(120 * time.Second),
			HostBusyCores: 4.0,
			LogicalCpus:   8.0,
		}, thresholds)
		if !floatEq(sigGap.HostBusyCores60sMean, 0.0) {
			t.Fatalf("gap HostBusyCores60sMean: got %v, want 0.0 (>60s gap prunes the ring to 1 sample → 2-sample floor re-met; a stale-cached mean keeps the old 4.0)", sigGap.HostBusyCores60sMean)
		}
	})
}

// TestDecide_Headroom_Computation pins the headroom-in-cores number
// Signals.HeadroomCores = capacity − measured_use − reserve, exposed
// alongside HostBusyCores60sMean with no verdict change. capacity =
// Sample.Quota when non-nil and >0, else Sample.LogicalCpus (the uncapped
// box on the host). measured_use = Signals.HostBusyCores60sMean (the 60s
// mean). reserve = 1.0 core (the cpuReserveCores package const). This
// computation does not change the verdict — headroom < 0 means "box is
// full" but it does not act on the number yet.
func TestDecide_Headroom_Computation(t *testing.T) {
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// feedHeadroom pushes n ticks of the given HostBusyCores onto st's
	// hostBusyRing at 1s spacing (well inside the 60s window) with the given
	// Quota/LogicalCpus, returning the final signals. The demand gate
	// (PressureFired||ThrottleFired) is closed (PressureAvg60=0, throttle
	// counters unset → ratio=0), so no latch fires; the assertion target is
	// solely sig.HeadroomCores.
	// feedHeadroom returns the final Verdict too so sub-blocks can pin the
	// no-verdict-change contract (State==StateHealthy, no Causes): the
	// headroom computation in Decide() writes only signals.HeadroomCores and
	// must not touch any latch or Verdict field.
	// NOTE: a demand-gated headroom latch is NOT caught here because the
	// demand gate is closed; cover such a latch separately if added.
	feedHeadroom := func(st *WindowState, n int, quota *float64, logical float64, busy ...float64) (Verdict, Signals) {
		var (
			v Verdict
			s Signals
		)
		for i := 0; i < n; i++ {
			b := 0.0
			if i < len(busy) {
				b = busy[i]
			}
			v, s = Decide(st, Sample{
				Timestamp:              base.Add(time.Duration(i) * time.Second),
				HostBusyCores:          b,
				HostBusyCoresAvailable: true, // R10.3: ring appends only when /proc/stat is readable
				LogicalCpus:            logical,
				Quota:                  quota,
			}, thresholds)
		}
		return v, s
	}

	// feedLimit is the limit-mode counterpart of feedHeadroom: it sets
	// UsageCores (the limit-mode headroom numerator) instead of HostBusyCores,
	// at 1s spacing with the given Quota/LogicalCpus. HostBusyCores is left 0
	// (limit-mode headroom reads container usage, not host busyness).
	feedLimit := func(st *WindowState, n int, quota *float64, logical float64, usage ...float64) (Verdict, Signals) {
		var (
			v Verdict
			s Signals
		)
		for i := 0; i < n; i++ {
			u := 0.0
			if i < len(usage) {
				u = usage[i]
			}
			v, s = Decide(st, Sample{
				Timestamp:     base.Add(time.Duration(i) * time.Second),
				UsageCores:    u,
				HostBusyCores: 0,
				LogicalCpus:   logical,
				Quota:         quota,
			}, thresholds)
		}
		return v, s
	}

	// (1) QUOTA-SET (limit mode) — 2 ticks UsageCores=1.8, Quota=&4.0,
	// LogicalCpus=8.0 → usageMean=1.8, reserve=LimitReserveFraction×4.0=0.4,
	// headroom = 4 − 1.8 − 0.4 = 1.8. Forcing: a stub using hostBusyMean (0)
	// yields 4−0−0.4=3.6 (!floatEq 1.8); a stub using the flat 1.0 reserve
	// yields 4−1.8−1.0=1.2 (!floatEq 1.8).
	t.Run("QuotaSet", func(t *testing.T) {
		quota := 4.0
		st := &WindowState{}
		_, sig := feedLimit(st, 2, &quota, 8.0, 1.8, 1.8)
		if !floatEq(sig.HeadroomCores, 1.8) {
			t.Fatalf("HeadroomCores: got %v, want 1.8 (capacity=Quota 4.0 − usageMean 1.8 − LimitReserveFraction×4.0=0.4; a stub using hostBusyMean (0) yields 3.6, a stub using flat 1.0 reserve yields 1.2)", sig.HeadroomCores)
		}
	})

	// (2) QUOTA-NIL — 2 ticks mean=6.0, Quota=nil, Logical=8 → headroom =
	// 8 − 6 − 1 = 1.0. Forcing: without the nil-fallback to LogicalCpus,
	// capacity is 0 → headroom −7.0 (!floatEq 1.0).
	t.Run("QuotaNil", func(t *testing.T) {
		st := &WindowState{}
		v, sig := feedHeadroom(st, 2, nil, 8.0, 6.0, 6.0)
		if !floatEq(sig.HeadroomCores, 1.0) {
			t.Fatalf("HeadroomCores: got %v, want 1.0 (capacity=LogicalCpus 8.0 (Quota nil fallback) − mean 6.0 − reserve 1.0; without nil-fallback capacity is 0 → −7.0)", sig.HeadroomCores)
		}
		// Quota=nil + PsiAvailable=false + Virtualized=false → dead-zone active
		// (decide.go deadZone predicate). Pin the no-verdict-change contract
		// in the dead-zone case, where the saturation backstop branch runs: a
		// future change wiring headroom negativity or the dead-zone saturation
		// path into a verdict/Cause would otherwise slip past this sub-block
		// silently.
		if v.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (adds no verdict in the dead-zone; headroom is observability-only)", v.State, StateHealthy)
		}
		if len(v.Causes) != 0 {
			t.Fatalf("Causes length: got %d, want 0 (adds no cause in the dead-zone; headroom is observability-only)", len(v.Causes))
		}
	})

	// (3) QUOTA-ZERO — 2 ticks mean=2.0, Quota=&0.0, Logical=4 → headroom =
	// 4 − 2 − 1 = 1.0 (quota<=0 falls back to logical). Forcing: using quota 0
	// as capacity yields 0 − 2 − 1 = −3.0 (!floatEq 1.0).
	t.Run("QuotaZero", func(t *testing.T) {
		zero := 0.0
		st := &WindowState{}
		v, sig := feedHeadroom(st, 2, &zero, 4.0, 2.0, 2.0)
		if !floatEq(sig.HeadroomCores, 1.0) {
			t.Fatalf("HeadroomCores: got %v, want 1.0 (Quota=&0.0 <= 0 → fallback to LogicalCpus 4.0 − mean 2.0 − reserve 1.0; using quota 0 as capacity yields −3.0)", sig.HeadroomCores)
		}
		// Quota=&0.0 (non-positive) + PsiAvailable=false + Virtualized=false →
		// dead-zone active. Pin the no-verdict-change contract here too
		// (mirrors QuotaNil): the dead-zone saturation backstop branch runs.
		if v.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (adds no verdict in the dead-zone; headroom is observability-only)", v.State, StateHealthy)
		}
		if len(v.Causes) != 0 {
			t.Fatalf("Causes length: got %d, want 0 (adds no cause in the dead-zone; headroom is observability-only)", len(v.Causes))
		}
	})

	// (4) MEAN-NOT-LAST-TICK (limit mode) — ticks [1.0, 3.0] (mean=2.0),
	// Quota=8 → headroom = 8 − 2.0 − 0.8 = 5.2. Forcing: using the last tick
	// 3.0 yields 8 − 3.0 − 0.8 = 4.2 (!floatEq 5.2).
	t.Run("MeanNotLastTick", func(t *testing.T) {
		quota := 8.0
		st := &WindowState{}
		_, sig := feedLimit(st, 2, &quota, 8.0, 1.0, 3.0)
		if !floatEq(sig.HeadroomCores, 5.2) {
			t.Fatalf("HeadroomCores: got %v, want 5.2 (capacity 8.0 − mean(1.0,3.0)=2.0 − LimitReserveFraction×8.0=0.8; using the last tick 3.0 yields 4.2)", sig.HeadroomCores)
		}
	})

	// (5) RESERVE-IS-FRACTION — 2 ticks UsageCores=3.6, Quota=4 → headroom =
	// 4 − 3.6 − 0.4 = 0.0 (boundary). Pins LimitReserveFraction=0.10: a stub
	// using the flat 1.0 reserve yields 4−3.6−1.0=−0.6 (!floatEq 0.0).
	t.Run("ReserveIsFraction", func(t *testing.T) {
		quota := 4.0
		st := &WindowState{}
		_, sig := feedLimit(st, 2, &quota, 8.0, 3.6, 3.6)
		if !floatEq(sig.HeadroomCores, 0.0) {
			t.Fatalf("HeadroomCores: got %v, want 0.0 (capacity 4.0 − usageMean 3.6 − LimitReserveFraction×4.0=0.4; a stub using flat 1.0 reserve yields −0.6 — pins LimitReserveFraction=0.10)", sig.HeadroomCores)
		}
		// The exact-boundary State is not asserted (mirrors LimitMode_Headroom/
		// QuotaSet): 4.0 − 3.6 − 0.4 is a tiny negative (~−8.9e-17) in float64,
		// so the latch fires on the < 0 side. The boundary is pinned by
		// NearLimitFires (−0.1 → degraded) and FreshStateMeanZero (1.8 → healthy).
	})

	// (6) FULL-BOX (limit-saturation fire) — 2 ticks UsageCores=3.9, Quota=4 →
	// headroom = 4 − 3.9 − 0.4 = −0.3 < 0 → degraded + saturation cause. The
	// old FullBox used hostBusy=4 to fire via the host-headroom branch; that
	// path is now the B-false-fire regression (covered by
	// TestDecide_LimitMode_BFalseFireKilled). This subtest now pins the
	// limit-mode saturation fire: sustained container usage inside the
	// fractional reserve band.
	t.Run("FullBox", func(t *testing.T) {
		quota := 4.0
		st := &WindowState{}
		v, sig := feedLimit(st, 2, &quota, 4.0, 3.9, 3.9)
		if !(sig.HeadroomCores < 0) {
			t.Fatalf("HeadroomCores: got %v, want < 0 (limit-mode saturation: 4−3.9−0.4=−0.3)", sig.HeadroomCores)
		}
		if !floatEq(sig.HeadroomCores, -0.3) {
			t.Fatalf("HeadroomCores: got %v, want -0.3 (capacity 4.0 − usageMean 3.9 − LimitReserveFraction×4.0=0.4)", sig.HeadroomCores)
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (limit-mode headroom < 0 → degraded)", v.State, StateDegraded)
		}
		hasSat := false
		for _, c := range v.Causes {
			if c.Kind == CauseKindSaturation {
				hasSat = true
			}
		}
		if !hasSat {
			t.Fatalf("Causes: no saturation in %v (limit-mode headroom < 0 must emit saturation)", v.Causes)
		}
	})

	// (7) FRESH-STATE-MEAN-ZERO (limit mode) — 1 tick UsageCores=3.9 (ring<2
	// → mean 0 via the 2-sample floor), Quota=4 → headroom = 4 − 0 − 0.4 =
	// 3.6. Forcing: using the raw 1-tick 3.9 yields 4 − 3.9 − 0.4 = −0.3
	// (!floatEq 3.6). Also pins the no-verdict-change contract on a fresh
	// state: State stays StateHealthy with no Causes.
	t.Run("FreshStateMeanZero", func(t *testing.T) {
		quota := 4.0
		st := &WindowState{}
		v, sig := feedLimit(st, 1, &quota, 4.0, 3.9)
		if !floatEq(sig.HeadroomCores, 3.6) {
			t.Fatalf("HeadroomCores: got %v, want 3.6 (capacity 4.0 − floored mean 0.0 (ring<2) − LimitReserveFraction×4.0=0.4; using the raw 1-tick 3.9 yields −0.3)", sig.HeadroomCores)
		}
		if v.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (only computes the number; on a fresh state the verdict must stay healthy)", v.State, StateHealthy)
		}
		if len(v.Causes) != 0 {
			t.Fatalf("Causes length: got %d, want 0 (adds no cause; on a fresh state there must be no saturation cause)", len(v.Causes))
		}
	})
}

// TestDecide_Saturation_HeadroomTrigger pins the dead-zone saturation
// backstop's headroom trigger: in the dead-zone the latch fires when
// HeadroomCores < 0 (hostBusyMean > capacity − cpuReserveCores, i.e. less than
// one core free), holds in [0, headroomRecoverCores], and clears when
// HeadroomCores > headroomRecoverCores, instead of the 60s-avg usage
// fraction (>= HighUsageFraction). The cause stays CauseKindSaturation —
// only the trigger changes. All sub-blocks run in the dead-zone (Quota nil,
// PsiAvailable false, Virtualized false = zero-value Sample), where
// capacity=LogicalCpus and headroom=LogicalCpus − 60s-mean(HostBusyCores) −
// cpuReserveCores.
func TestDecide_Saturation_HeadroomTrigger(t *testing.T) {
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// dzSample builds a dead-zone sample (Quota nil, PsiAvailable false,
	// Virtualized false — the zero-value Sample) so capacity=LogicalCpus and
	// headroom=LogicalCpus − 60s-mean(HostBusyCores) − cpuReserveCores. The
	// usageCores/cgroupCores pair populates the 60s-avg usage-fraction path
	// (UsageCores/CgroupCores); passing 0,0 yields fraction 0 (clamped from
	// 0/0 NaN), which never trips the fraction trigger. R10.3: sets
	// HostBusyCoresAvailable=true so the sample routes to the host-headroom
	// branch (these tests exercise the host-headroom path, which requires
	// readable host stats).
	dzSample := func(dt time.Duration, logical, hostBusy, usageCores, cgroupCores float64) Sample {
		return Sample{
			Timestamp:              base.Add(dt),
			HostBusyCores:          hostBusy,
			HostBusyCoresAvailable: true,
			LogicalCpus:            logical,
			UsageCores:             usageCores,
			CgroupCores:            cgroupCores,
		}
	}

	// (1) FULL_BOX_FIRES — 2 ticks HostBusyCores=8.0, LogicalCpus=8.0 →
	// mean=8.0, headroom=8−8−1=−1.0 < 0 (hostBusyMean 8.0 > capacity−reserve
	// 7.0). The fraction trigger reads saturationAvg=0 (zero usage) and stays
	// healthy; the headroom trigger must fire.
	t.Run("FULL_BOX_FIRES", func(t *testing.T) {
		st := &WindowState{}
		var (
			v   Verdict
			sig Signals
		)
		for i := 0; i < 2; i++ {
			v, sig = Decide(st, dzSample(time.Duration(i)*time.Second, 8.0, 8.0, 0, 0), thresholds)
		}
		if !sig.SaturationFired {
			t.Fatalf("SaturationFired: got false, want true (headroom=8−8−1=−1.0 < 0 → hostBusyMean 8.0 > capacity−reserve 7.0 → must fire)")
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (a full box degrades)", v.State, StateDegraded)
		}
		hasSat := false
		for _, c := range v.Causes {
			if c.Kind == CauseKindSaturation {
				hasSat = true
			}
		}
		if !hasSat {
			t.Fatalf("Causes: no CauseKindSaturation present (headroom<=−cpuReserveCores must emit the saturation cause; got %v)", v.Causes)
		}
	})

	// (2) SPARE_CORE_DOESNT_FIRE — 2 ticks HostBusyCores=6.0, LogicalCpus=8.0
	// → headroom=8−6−1=1.0 > 0. The trigger must NOT fire when there is a spare
	// core.
	t.Run("SPARE_CORE_DOESNT_FIRE", func(t *testing.T) {
		st := &WindowState{}
		var (
			v   Verdict
			sig Signals
		)
		for i := 0; i < 2; i++ {
			v, sig = Decide(st, dzSample(time.Duration(i)*time.Second, 8.0, 6.0, 0, 0), thresholds)
		}
		if sig.SaturationFired {
			t.Fatalf("SaturationFired: got true, want false (headroom=8−6−1=1.0 > 0 → must not fire)")
		}
		if v.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (spare core → healthy)", v.State, StateHealthy)
		}
		if len(v.Causes) != 0 {
			t.Fatalf("Causes length: got %d, want 0 (spare core → no causes)", len(v.Causes))
		}
	})

	// (3) FRACTION_TRIGGER_REPLACED — 2 ticks with UsageCores=5.6,
	// CgroupCores=8.0 (→ fraction=5.6/8.0=0.70=HighUsageFraction, the fraction
	// fire condition) AND HostBusyCores=5.6, LogicalCpus=8.0 (→ headroom=8−5.6−1=1.4
	// > 0). The fraction trigger would fire here (0.70) but the headroom
	// trigger must NOT (headroom 1.4 > 0): the fraction trigger is replaced,
	// not kept, when the host CPU count is known.
	t.Run("FRACTION_TRIGGER_REPLACED", func(t *testing.T) {
		st := &WindowState{}
		var (
			v   Verdict
			sig Signals
		)
		for i := 0; i < 2; i++ {
			v, sig = Decide(st, dzSample(time.Duration(i)*time.Second, 8.0, 5.6, 5.6, 8.0), thresholds)
		}
		if sig.SaturationFired {
			t.Fatalf("SaturationFired: got true, want false (fraction=5.6/8.0=0.70 hits the fraction fire condition, but headroom=8−5.6−1=1.4 > 0 → the headroom trigger must NOT fire)")
		}
		if v.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (headroom 1.4 > 0 → healthy)", v.State, StateHealthy)
		}
	})

	// (4) PARKED_ON_LINE_FRESH — 2 ticks HostBusyCores=3.0, LogicalCpus=4.0 →
	// headroom=4−3−1=0.0, the bottom of the hold interval. A host at headroom
	// 0.0 is exactly at the reserve boundary (one core free == the reserve);
	// fire is strictly headroom < 0, so 0.0 must NOT fire (it holds if already
	// fired, and stays healthy if fresh).
	t.Run("PARKED_ON_LINE_FRESH", func(t *testing.T) {
		st := &WindowState{}
		var (
			v   Verdict
			sig Signals
		)
		for i := 0; i < 2; i++ {
			v, sig = Decide(st, dzSample(time.Duration(i)*time.Second, 4.0, 3.0, 0, 0), thresholds)
		}
		if sig.SaturationFired {
			t.Fatalf("SaturationFired: got true, want false (headroom=4−3−1=0.0; fire is strictly < 0, not <= 0 → must not fire)")
		}
		if v.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (headroom=0.0 is not < 0 → healthy)", v.State, StateHealthy)
		}
	})

	// (5) SCHMITT_HOLD_THEN_CLEAR — (a) fire, (b) hold at a strictly-positive
	// headroom inside the hold interval, (c) clear just past headroomRecoverCores.
	// (b) holds at headroom=0.25 (in (0, headroomRecoverCores]): a no-hysteresis
	// stub clearing at >0 would drop the latch here, so the assertion pins
	// headroomRecoverCores. (c) clears at headroom=0.6 (just > 0.5): a
	// never-clear stub keeps the latch fired here.
	t.Run("SCHMITT_HOLD_THEN_CLEAR", func(t *testing.T) {
		st := &WindowState{}

		// (a) fire: 2 ticks HostBusyCores=4.0, LogicalCpus=4.0 → headroom=−1.0.
		for i := 0; i < 2; i++ {
			Decide(st, dzSample(time.Duration(i)*time.Second, 4.0, 4.0, 0, 0), thresholds)
		}
		_, sig1 := Decide(st, dzSample(2*time.Second, 4.0, 4.0, 0, 0), thresholds)
		if !sig1.SaturationFired {
			t.Fatalf("(a) fire SaturationFired: got false, want true (headroom=4−4−1=−1.0 < 0 → must fire)")
		}

		// (b) hold: 70 ticks HostBusyCores=2.75 (i=3..72) → mean settles to
		// 2.75, headroom=4−2.75−1=0.25 (in (0, headroomRecoverCores]) → MUST
		// stay fired (hold interval). A no-hysteresis stub clearing at >0
		// would clear here (0.25 > 0).
		for i := 3; i <= 72; i++ {
			Decide(st, dzSample(time.Duration(i)*time.Second, 4.0, 2.75, 0, 0), thresholds)
		}
		_, sig2 := Decide(st, dzSample(73*time.Second, 4.0, 2.75, 0, 0), thresholds)
		if !sig2.SaturationFired {
			t.Fatalf("(b) hold SaturationFired: got false, want true (headroom=4−2.75−1=0.25 is in (0, headroomRecoverCores] → hold interval → must NOT clear)")
		}
		if sig2.HeadroomCores > 0.5 || sig2.HeadroomCores <= 0 {
			t.Fatalf("(b) hold HeadroomCores: got %v, want in (0, 0.5] (setup check: headroom must be inside the hold interval)", sig2.HeadroomCores)
		}

		// (c) clear: 70 ticks HostBusyCores=2.4 (i=74..144) → mean=2.4,
		// headroom=4−2.4−1=0.6 > headroomRecoverCores → MUST clear.
		for i := 74; i <= 144; i++ {
			Decide(st, dzSample(time.Duration(i)*time.Second, 4.0, 2.4, 0, 0), thresholds)
		}
		_, sig3 := Decide(st, dzSample(145*time.Second, 4.0, 2.4, 0, 0), thresholds)
		if sig3.SaturationFired {
			t.Fatalf("(c) clear SaturationFired: got true, want false (headroom=4−2.4−1=0.6 > headroomRecoverCores → latch must clear)")
		}
		if sig3.HeadroomCores <= 0.5 {
			t.Fatalf("(c) clear HeadroomCores: got %v, want > 0.5 (setup check: headroom must exceed headroomRecoverCores to force clear)", sig3.HeadroomCores)
		}
	})

	// (6) FRACTION_FALLBACK_LOGICALCPUS_UNKNOWN — the host CPU count is
	// unknown (LogicalCpus == 0, the cgroup-known-only sub-case). R10.3 removed
	// the old fraction fallback branch (case deadZone && len(usageRing)>=2,
	// which divided by Quota/CgroupCores — both 0 in no-limit, the bug). The
	// no-host-stats saturation requires LogicalCpus > 0, so LogicalCpus=0 falls through to the
	// `default` branch → healthy (the unreachable-in-prod safety net, since
	// runtime.NumCPU() always returns > 0). This sub-test now pins that
	// unreachable behavior: a sample with LogicalCpus=0 stays healthy
	// regardless of usage, and the saturation latch is cleared.
	t.Run("FRACTION_FALLBACK_LOGICALCPUS_UNKNOWN", func(t *testing.T) {
		st := &WindowState{}
		var (
			v   Verdict
			sig Signals
		)
		for i := 0; i < 2; i++ {
			v, sig = Decide(st, dzSample(time.Duration(i)*time.Second, 0, 0, 5.6, 8.0), thresholds)
		}
		if sig.SaturationFired {
			t.Fatalf("(6) SaturationFired: got true, want false (LogicalCpus==0 → default branch → latch cleared; the old CgroupCores-denominator fraction fallback is removed in R10.3)")
		}
		if v.State != StateHealthy {
			t.Fatalf("(6) State: got %q, want %q (LogicalCpus==0 → default branch → healthy; unreachable in prod via runtime.NumCPU())", v.State, StateHealthy)
		}
	})

	// (7) NEAR_FULL_FIRES — a box with less than one core free (but not
	// literally full) fires under headroom<0. 2 ticks HostBusyCores=7.5,
	// LogicalCpus=8.0 → mean=7.5, headroom=8−7.5−1=−0.5 < 0 (hostBusyMean 7.5
	// > capacity−reserve 7.0). A fire-at-hostBusyMean>=capacity stub (i.e.
	// HeadroomCores <= −cpuReserveCores) would NOT fire here (7.5 < 8); the
	// spec's headroom<0 MUST fire (less than one core free). This pins the
	// fire threshold at < 0, not <= −reserve, and makes cpuReserveCores a
	// fire-sensitivity knob (raising it fires earlier).
	t.Run("NEAR_FULL_FIRES", func(t *testing.T) {
		st := &WindowState{}
		var (
			v   Verdict
			sig Signals
		)
		for i := 0; i < 2; i++ {
			v, sig = Decide(st, dzSample(time.Duration(i)*time.Second, 8.0, 7.5, 0, 0), thresholds)
		}
		if !sig.SaturationFired {
			t.Fatalf("SaturationFired: got false, want true (headroom=%v < 0 → less than one core free → must fire; a fire-at-hostBusyMean>=capacity stub fails here)", sig.HeadroomCores)
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (near-full box with <1 core free degrades)", v.State, StateDegraded)
		}
	})
}

// TestDecide_Saturation_FiresOnVirtualizedBox pins the dead-zone admission of
// a virtualized box: dropping the !Virtualized clause from the dead-zone
// predicate at decide.go so the headroom-based saturation backstop fires
// on a virtualized box with no PSI and no cgroup limit (the common UMH VM
// deployment). The predicate is `(Quota==nil || !(*Quota>0)) && !PsiAvailable`;
// a virtualized dead-zone sample now enters the dead-zone, headroom is
// evaluated, and saturation fires when the box has no spare core.
// `signals.LimitedVisibility = deadZone` is a pure no-PSI/no-limit annotation,
// independent of virtualization.
//
// All sub-blocks use PsiAvailable=false + Quota=nil + CgroupCores=0 (so the
// LogicalCpus<=0 fraction sub-case is inert) + StealFraction=0 (so the steal
// latch at decide.go does not co-fire) + PressureAvg60=0 (so the demand
// gate at decide.go stays closed and host-contention does not co-fire).
// Only Virtualized and the headroom inputs (HostBusyCores, LogicalCpus) vary.
func TestDecide_Saturation_FiresOnVirtualizedBox(t *testing.T) {
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// virtSample builds a dead-zone sample: Quota nil (no limit), PsiAvailable
	// false (no PSI), CgroupCores 0 (so the fraction sub-case is inert),
	// StealFraction 0 (so the steal latch does not co-fire), PressureAvg60 0
	// (so the demand gate stays closed and host-contention does not co-fire).
	// capacity=LogicalCpus and headroom=LogicalCpus − 60s-mean(HostBusyCores) −
	// cpuReserveCores. virt selects Sample.Virtualized. R10.3: sets
	// HostBusyCoresAvailable=true so the sample routes to the host-headroom
	// branch (these tests exercise the host-headroom path on virtualized
	// boxes, which requires readable host stats).
	virtSample := func(dt time.Duration, logical, hostBusy float64, virt bool) Sample {
		return Sample{
			Timestamp:              base.Add(dt),
			HostBusyCores:          hostBusy,
			HostBusyCoresAvailable: true,
			LogicalCpus:            logical,
			Virtualized:            virt,
		}
	}

	// (1) A virtualized full box degrades. 2 ticks HostBusyCores=8.0,
	// LogicalCpus=8.0, Virtualized=true → mean=8.0, headroom=8−8−1=−1.0 < 0.
	// With the !Virtualized clause dropped, the virtualized box enters the
	// dead-zone, saturation is evaluated, and headroom fires. A stub that keeps
	// `!Virtualized` fails here (healthy + no saturation + no limitedVisibility).
	t.Run("VIRTUALIZED_FULL_BOX_DEGRADES", func(t *testing.T) {
		st := &WindowState{}
		var (
			v   Verdict
			sig Signals
		)
		for i := 0; i < 2; i++ {
			v, sig = Decide(st, virtSample(time.Duration(i)*time.Second, 8.0, 8.0, true), thresholds)
		}
		if !sig.SaturationFired {
			t.Fatalf("SaturationFired: got false, want true (Virtualized=true, headroom=%v < 0 → the dead-zone must admit a virtualized box with no PSI and no limit so headroom fires)", sig.HeadroomCores)
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (a virtualized full box with no spare core degrades)", v.State, StateDegraded)
		}
		if !sig.LimitedVisibility {
			t.Fatalf("LimitedVisibility: got false, want true (no PSI AND no limit → blind state; the annotation is now independent of virtualization)")
		}
		hasSat := false
		for _, c := range v.Causes {
			if c.Kind == CauseKindSaturation {
				hasSat = true
			}
		}
		if !hasSat {
			t.Fatalf("Causes: no CauseKindSaturation present (headroom < 0 must emit the saturation cause; got %v)", v.Causes)
		}
	})

	// (2) LimitedVisibility is an annotation INDEPENDENT of the verdict: it
	// shows on a virtualized dead-zone sample EVEN WHEN HEALTHY. 2 ticks
	// HostBusyCores=4.0, LogicalCpus=8.0, Virtualized=true → headroom=8−4−1=3.0
	// > 0 → healthy, saturation does NOT fire. LimitedVisibility is true (no PSI
	// and no limit, regardless of virtualization). A stub gating
	// limitedVisibility on `!Virtualized` OR on the degraded verdict fails here.
	t.Run("LIMITED_VISIBILITY_WHEN_HEALTHY", func(t *testing.T) {
		st := &WindowState{}
		var (
			v   Verdict
			sig Signals
		)
		for i := 0; i < 2; i++ {
			v, sig = Decide(st, virtSample(time.Duration(i)*time.Second, 8.0, 4.0, true), thresholds)
		}
		if sig.SaturationFired {
			t.Fatalf("SaturationFired: got true, want false (headroom=%v > 0 → spare core → must not fire)", sig.HeadroomCores)
		}
		if v.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (spare core → healthy)", v.State, StateHealthy)
		}
		if !sig.LimitedVisibility {
			t.Fatalf("LimitedVisibility: got false, want true (virtualized dead-zone sample with no PSI and no limit — the annotation shows even when healthy)")
		}
	})

	// (3) The non-virtualized dead-zone path is preserved. 2 ticks
	// HostBusyCores=8.0, LogicalCpus=8.0, Virtualized=false → headroom=−1.0 < 0.
	// The dead-zone admits non-virtualized boxes; it must continue to. This is a
	// regression guard. A stub that lifts `!Virtualized` by ALSO dropping the
	// `!PsiAvailable` clause (over-lifting) would still pass here, so this block
	// alone does not guard the PSI clause; it guards the non-virtualized path.
	t.Run("NON_VIRTUALIZED_REGRESSION", func(t *testing.T) {
		st := &WindowState{}
		var (
			v   Verdict
			sig Signals
		)
		for i := 0; i < 2; i++ {
			v, sig = Decide(st, virtSample(time.Duration(i)*time.Second, 8.0, 8.0, false), thresholds)
		}
		if !sig.SaturationFired {
			t.Fatalf("SaturationFired: got false, want true (Virtualized=false, headroom=%v < 0 → the non-virtualized dead-zone path must still fire)", sig.HeadroomCores)
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (non-virtualized full box degrades — non-virtualized path preserved)", v.State, StateDegraded)
		}
		if !sig.LimitedVisibility {
			t.Fatalf("LimitedVisibility: got false, want true (non-virtualized dead-zone sample — non-virtualized path preserved)")
		}
	})

	// (4) FULL_PLUS_PSI — R4 option B: a full box WITH PSI degrades on BOTH
	// pressure and saturation (headroom fires always, not just dead-zone; PSI
	// stacks on top as an additional cause). 2 ticks HostBusyCores=8.0,
	// LogicalCpus=8.0, PsiAvailable=true, PressureAvg60=0.30 (>PressureHigh
	// 0.20 → pressure fires), Quota=nil → headroom=8−8−1=−1.0<0 → saturation
	// fires. NOT dead-zone (PsiAvailable true) → LimitedVisibility=false. Under
	// option A (pre-option-B) this was [pressure] only; option B adds
	// saturation. A stub keeping headroom dead-zone-only fails here (no
	// saturation).
	t.Run("FULL_PLUS_PSI", func(t *testing.T) {
		st := &WindowState{}
		s := Sample{Timestamp: base, HostBusyCores: 8.0, HostBusyCoresAvailable: true, LogicalCpus: 8.0, PsiAvailable: true, PressureAvg60: 0.30}
		Decide(st, s, thresholds)
		s.Timestamp = base.Add(1 * time.Second)
		v, sig := Decide(st, s, thresholds)
		if !sig.PressureFired {
			t.Fatalf("PressureFired: got false, want true (PressureAvg60 0.30 > 0.20)")
		}
		if !sig.SaturationFired {
			t.Fatalf("SaturationFired: got false, want true (R4 option B: headroom=%v < 0 fires always, not just dead-zone; a full+PSI box yields [pressure, saturation])", sig.HeadroomCores)
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (full + PSI → degraded)", v.State, StateDegraded)
		}
		if sig.LimitedVisibility {
			t.Fatalf("LimitedVisibility: got true, want false (PSI present → not blind → no limited visibility)")
		}
		hasPressure, hasSat := false, false
		var satValue float64
		for _, c := range v.Causes {
			if c.Kind == CauseKindPressure {
				hasPressure = true
			}
			if c.Kind == CauseKindSaturation {
				hasSat = true
				satValue = c.Value
			}
		}
		if !hasPressure || !hasSat {
			t.Fatalf("Causes: got %v, want both pressure AND saturation (option B: full+PSI → [pressure, saturation])", v.Causes)
		}
		// The saturation cause Value is the negative headroom in cores (not 0):
		// saturationAvg is 0 outside the dead-zone (usage ring is dead-zone-only),
		// so the Value must come from HeadroomCores to read "N cores over capacity."
		if !floatEq(satValue, sig.HeadroomCores) {
			t.Fatalf("saturation Value: got %v, want %v (HeadroomCores — 'cores over capacity'; saturationAvg is 0 outside the dead-zone)", satValue, sig.HeadroomCores)
		}
	})
}

// TestDecide_HostContentionFold_Full pins that Decide does not emit
// CauseKindHostContention outside the dead-zone when per-tick headroom >= 0,
// and that the host/container attribution split is computed for every degraded
// verdict where host-contention is not firing. The host share is (clamped
// HostBusyCores - clamped UsageCores); attribution is Host when the host share
// exceeds the UMH share (UsageCores), else Unknown. Inside the dead-zone and
// when per-tick headroom < 0, the demand-gated host-contention latch is
// retained (covered by sub-case (5) and the dead-zone characterization tests).
//
// Sub-cases (1)-(4) are NON-dead-zone (Quota set + PsiAvailable true) with
// headroom = 0 (HostBusyCores=7.0, Quota=8.0, reserve=1.0), so SaturationFired
// is false and host-contention does not fire — this isolates the attribution
// split from the dead-zone saturation split. Sub-case (5) exercises the
// retained non-dead-zone headroom<0 fire path.
func TestDecide_HostContentionFold_Full(t *testing.T) {
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// feedPressureTicks runs n Decide ticks at 1s spacing on st with the given
	// sample template (PressureAvg60, HostBusyCores, UsageCores, LogicalCpus),
	// returning the final (Verdict, Signals). Pressure is thresholded directly
	// (no ring), so 2 ticks is enough to hold the latch fired.
	feedPressureTicks := func(st *WindowState, n int, tpl Sample) (Verdict, Signals) {
		var (
			v   Verdict
			sig Signals
		)
		for i := 0; i < n; i++ {
			s := tpl
			s.Timestamp = base.Add(time.Duration(i) * time.Second)
			v, sig = Decide(st, s, thresholds)
		}
		return v, sig
	}

	// hasCause reports whether a Cause with the given Kind appears in v.Causes.
	hasCause := func(v Verdict, kind CauseKind) bool {
		for _, c := range v.Causes {
			if c.Kind == kind {
				return true
			}
		}
		return false
	}

	// onlyCause reports whether v.Causes contains exactly one Cause of the
	// given Kind (the fold must leave exactly the non-host-contention cause).
	onlyCause := func(v Verdict, kind CauseKind) bool {
		if len(v.Causes) != 1 {
			return false
		}
		return v.Causes[0].Kind == kind
	}

	// (1) PRESSURE_PLUS_HOST_BUSY_HOST_DOMINANT — NON-dead-zone (Quota=&8.0,
	// PsiAvailable true), 2 ticks PressureAvg60=0.30 (>PressureHigh 0.20 →
	// pressure fires), HostBusyCores=7.0, UsageCores=1.0, LogicalCpus=8.0.
	// headroom=8−7−1=0 → not < 0 → SaturationFired false (the headroom branch
	// fires on < 0, and 0 is the hold/clear boundary, not a fire), and
	// host-contention does not fire (headroom >= 0). The host (non-UMH) share
	// is 7.0−1.0=6.0 > our share 1.0 → AttributionHost via the split. Expected
	// output: Causes=[pressure], Attribution=Host.
	t.Run("PRESSURE_PLUS_HOST_BUSY_HOST_DOMINANT", func(t *testing.T) {
		quota := 8.0
		st := &WindowState{}
		v, sig := feedPressureTicks(st, 2, Sample{
			Quota:                  &quota,
			PsiAvailable:           true,
			PressureAvg60:          0.30,
			UsageCores:             1.0,
			HostBusyCores:          7.0,
			HostBusyCoresAvailable: true, // ring fills with 7.0 → 60s mean = 7.0 (the smoothed split uses the mean)
			LogicalCpus:            8.0,
		})
		if !sig.PressureFired {
			t.Fatalf("PressureFired: got false, want true (PressureAvg60 0.30 > PressureHigh 0.20)")
		}
		if sig.HostContentionFired {
			t.Fatalf("HostContentionFired: got true, want false (non-dead-zone headroom=0 >= 0 → host-contention must not fire)")
		}
		if sig.SaturationFired {
			t.Fatalf("SaturationFired: got true, want false (headroom=8−7−1=0, not < 0 → non-dead-zone saturation must not fire; isolates the attribution split from the dead-zone split)")
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (pressure fires → degraded)", v.State, StateDegraded)
		}
		if hasCause(v, CauseKindHostContention) {
			t.Fatalf("Causes: CauseKindHostContention present, want absent (non-dead-zone headroom>=0 → host-contention must not be emitted; got %v)", v.Causes)
		}
		if !onlyCause(v, CauseKindPressure) {
			t.Fatalf("Causes: got %v, want exactly [pressure] (host-contention must not appear)", v.Causes)
		}
		if v.Attribution != AttributionHost {
			t.Fatalf("Attribution: got %q, want %q (host share 6.0 > our share 1.0 → Host via the split)", v.Attribution, AttributionHost)
		}
	})

	// (2) PRESSURE_PLUS_HOST_BUSY_WE_DOMINANT — same non-dead-zone pressure
	// fire, but UsageCores=6.5 so the UMH share (6.5) exceeds the host share
	// (7.0−6.5=0.5) → AttributionUnknown. Expected output: Causes=[pressure],
	// Attribution=Unknown (the host/container split yields Unknown when our
	// share dominates).
	t.Run("PRESSURE_PLUS_HOST_BUSY_WE_DOMINANT", func(t *testing.T) {
		quota := 8.0
		st := &WindowState{}
		v, sig := feedPressureTicks(st, 2, Sample{
			Quota:                  &quota,
			PsiAvailable:           true,
			PressureAvg60:          0.30,
			UsageCores:             6.5,
			HostBusyCores:          7.0,
			HostBusyCoresAvailable: true, // ring fills with 7.0 → 60s mean = 7.0 (the smoothed split uses the mean)
			LogicalCpus:            8.0,
		})
		if !sig.PressureFired {
			t.Fatalf("PressureFired: got false, want true (PressureAvg60 0.30 > PressureHigh 0.20)")
		}
		if sig.HostContentionFired {
			t.Fatalf("HostContentionFired: got true, want false (non-dead-zone headroom=0 >= 0 → host-contention must not fire)")
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (pressure fires → degraded)", v.State, StateDegraded)
		}
		if hasCause(v, CauseKindHostContention) {
			t.Fatalf("Causes: CauseKindHostContention present, want absent (non-dead-zone headroom>=0; got %v)", v.Causes)
		}
		if !onlyCause(v, CauseKindPressure) {
			t.Fatalf("Causes: got %v, want exactly [pressure] (host-contention must not appear)", v.Causes)
		}
		if v.Attribution != AttributionUnknown {
			t.Fatalf("Attribution: got %q, want %q (host share 0.5 < our share 6.5 → Unknown via the split)", v.Attribution, AttributionUnknown)
		}
	})

	// (3) THROTTLE_PLUS_HOST_BUSY_HOST_DOMINANT — NON-dead-zone, throttle fires
	// (60s ratio 0.10 > ThrottleHigh 0.05), HostBusyCores=7.0, UsageCores=1.0,
	// LogicalCpus=8.0. headroom=0 → host-contention does not fire. host share
	// 6.0 > our share 1.0 → Host. Expected output: Causes=[throttling],
	// Attribution=Host.
	t.Run("THROTTLE_PLUS_HOST_BUSY_HOST_DOMINANT", func(t *testing.T) {
		quota := 8.0
		st := &WindowState{}
		// Tick 1: establish the throttle ring baseline (single point → ratio 0
		// → latch not firing).
		Decide(st, Sample{
			Timestamp:              base,
			Quota:                  &quota,
			PsiAvailable:           true,
			UsageCores:             1.0,
			HostBusyCores:          7.0,
			HostBusyCoresAvailable: true, // ring fills with 7.0 → 60s mean = 7.0 (the smoothed split uses the mean)
			LogicalCpus:            8.0,
			NrPeriods:              1000,
			NrThrottled:            10,
			PressureAvg60:          0.0,
		}, thresholds)
		// Tick 2: +1000 periods, +100 throttled → ratio 0.10 > 0.05 → throttle
		// fires.
		v, sig := Decide(st, Sample{
			Timestamp:              base.Add(10 * time.Second),
			Quota:                  &quota,
			PsiAvailable:           true,
			UsageCores:             1.0,
			HostBusyCores:          7.0,
			HostBusyCoresAvailable: true, // ring fills with 7.0 → 60s mean = 7.0 (the smoothed split uses the mean)
			LogicalCpus:            8.0,
			NrPeriods:              2000,
			NrThrottled:            110,
			PressureAvg60:          0.0,
		}, thresholds)
		if !sig.ThrottleFired {
			t.Fatalf("ThrottleFired: got false, want true (60s ratio (110-10)/(2000-1000)=0.10 > ThrottleHigh 0.05)")
		}
		if sig.PressureFired {
			t.Fatalf("PressureFired: got true, want false (PressureAvg60 0.0 → this is the THROTTLE-only path)")
		}
		if sig.HostContentionFired {
			t.Fatalf("HostContentionFired: got true, want false (non-dead-zone headroom=0 >= 0 → host-contention must not fire on the throttle path either)")
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (throttle fires → degraded)", v.State, StateDegraded)
		}
		if hasCause(v, CauseKindHostContention) {
			t.Fatalf("Causes: CauseKindHostContention present, want absent (non-dead-zone headroom>=0; got %v)", v.Causes)
		}
		if !onlyCause(v, CauseKindThrottling) {
			t.Fatalf("Causes: got %v, want exactly [throttling] (host-contention must not appear)", v.Causes)
		}
		if v.Attribution != AttributionHost {
			t.Fatalf("Attribution: got %q, want %q (host share 6.0 > our share 1.0 → Host via the split)", v.Attribution, AttributionHost)
		}
	})

	// (4) THROTTLE_PLUS_HOST_BUSY_WE_DOMINANT — same throttle fire, but
	// UsageCores=6.5 so our share (6.5) > host share (0.5) → Unknown. Expected
	// output: Causes=[throttling], Attribution=Unknown.
	t.Run("THROTTLE_PLUS_HOST_BUSY_WE_DOMINANT", func(t *testing.T) {
		quota := 8.0
		st := &WindowState{}
		Decide(st, Sample{
			Timestamp:              base,
			Quota:                  &quota,
			PsiAvailable:           true,
			UsageCores:             6.5,
			HostBusyCores:          7.0,
			HostBusyCoresAvailable: true, // ring fills with 7.0 → 60s mean = 7.0 (the smoothed split uses the mean)
			LogicalCpus:            8.0,
			NrPeriods:              1000,
			NrThrottled:            10,
			PressureAvg60:          0.0,
		}, thresholds)
		v, sig := Decide(st, Sample{
			Timestamp:              base.Add(10 * time.Second),
			Quota:                  &quota,
			PsiAvailable:           true,
			UsageCores:             6.5,
			HostBusyCores:          7.0,
			HostBusyCoresAvailable: true, // ring fills with 7.0 → 60s mean = 7.0 (the smoothed split uses the mean)
			LogicalCpus:            8.0,
			NrPeriods:              2000,
			NrThrottled:            110,
			PressureAvg60:          0.0,
		}, thresholds)
		if !sig.ThrottleFired {
			t.Fatalf("ThrottleFired: got false, want true (60s ratio 0.10 > ThrottleHigh 0.05)")
		}
		if sig.HostContentionFired {
			t.Fatalf("HostContentionFired: got true, want false (non-dead-zone headroom=0 >= 0 → host-contention must not fire)")
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (throttle fires → degraded)", v.State, StateDegraded)
		}
		if hasCause(v, CauseKindHostContention) {
			t.Fatalf("Causes: CauseKindHostContention present, want absent (non-dead-zone headroom>=0; got %v)", v.Causes)
		}
		if !onlyCause(v, CauseKindThrottling) {
			t.Fatalf("Causes: got %v, want exactly [throttling] (host-contention must not appear)", v.Causes)
		}
		if v.Attribution != AttributionUnknown {
			t.Fatalf("Attribution: got %q, want %q (host share 0.5 < our share 6.5 → Unknown via the split)", v.Attribution, AttributionUnknown)
		}
	})

	// (5) STEAL_PLUS_OUR_MAJORITY — the steal-present→host edge case: a full
	// box where OUR workload is the majority (UsageCores=6.0, HostBusyCores=8.0
	// → host share 2 < our share 6) AND the hypervisor is stealing
	// (StealFraction=0.20 > StealHigh 0.10). The split alone would yield
	// Unknown (our share wins), but steal firing → Host, because the fix is
	// host-side (give the VM real CPU, stop the steal), not "reduce your load."
	// Pins the steal-present→host rule over the dominance-based one.
	t.Run("STEAL_PLUS_OUR_MAJORITY", func(t *testing.T) {
		st := &WindowState{}
		s := Sample{Timestamp: base, HostBusyCores: 8.0, UsageCores: 6.0, LogicalCpus: 8.0, Virtualized: true, StealFraction: 0.20}
		Decide(st, s, thresholds)
		s.Timestamp = base.Add(1 * time.Second)
		v, sig := Decide(st, s, thresholds)
		if !sig.StealFired {
			t.Fatalf("StealFired: got false, want true (StealFraction 0.20 > StealHigh 0.10)")
		}
		if !sig.SaturationFired {
			t.Fatalf("SaturationFired: got false, want true (headroom=%v < 0)", sig.HeadroomCores)
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (steal + saturation → degraded)", v.State, StateDegraded)
		}
		if v.Attribution != AttributionHost {
			t.Fatalf("Attribution: got %q, want %q (steal present → Host: the hypervisor is the root cause; the fix is host-side, not 'reduce your load' — even though our workload is the majority of host-busy)", v.Attribution, AttributionHost)
		}
	})

}

// TestDecide_HealthyMessageApplicabilitySignals pins the applicability data the
// healthy message's budget dashboard reads: CapacityCores (the headroom
// denominator) and the three per-rule applicability booleans (LimitApplies,
// PsiApplies, StealApplies). The dashboard lists a rule's budget only when its
// applicability flag is set, so Decide must populate them from the sample's
// Quota / PsiAvailable / Virtualized fields.
func TestDecide_HealthyMessageApplicabilitySignals(t *testing.T) {
	ts := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// (a) Limited box: a positive Quota → LimitApplies true, CapacityCores is
	// the Quota (not LogicalCpus). PSI/steal do not apply.
	t.Run("LimitedBox", func(t *testing.T) {
		quota := 4.0
		st := &WindowState{}
		_, sig := Decide(st, Sample{Timestamp: ts, Quota: &quota, LogicalCpus: 8.0}, thresholds)
		if !floatEq(sig.CapacityCores, 4.0) {
			t.Fatalf("CapacityCores: got %v, want 4.0 (Quota set → capacity is Quota, not LogicalCpus 8.0)", sig.CapacityCores)
		}
		if !sig.LimitApplies {
			t.Fatalf("LimitApplies: got false, want true (a positive Quota means the throttle rule applies)")
		}
		if sig.PsiApplies {
			t.Fatalf("PsiApplies: got true, want false (PsiAvailable is false)")
		}
		if sig.StealApplies {
			t.Fatalf("StealApplies: got true, want false (not virtualized)")
		}
	})

	// (b) PSI box: PsiAvailable → PsiApplies true. No Quota → LimitApplies
	// false and CapacityCores is LogicalCpus.
	t.Run("PsiBox", func(t *testing.T) {
		st := &WindowState{}
		_, sig := Decide(st, Sample{Timestamp: ts, LogicalCpus: 8.0, PsiAvailable: true}, thresholds)
		if !sig.PsiApplies {
			t.Fatalf("PsiApplies: got false, want true (PsiAvailable true means the pressure rule applies)")
		}
		if sig.LimitApplies {
			t.Fatalf("LimitApplies: got true, want false (no Quota set)")
		}
		if sig.StealApplies {
			t.Fatalf("StealApplies: got true, want false (not virtualized)")
		}
		if !floatEq(sig.CapacityCores, 8.0) {
			t.Fatalf("CapacityCores: got %v, want 8.0 (no Quota → capacity is LogicalCpus)", sig.CapacityCores)
		}
	})

	// (c) Virtualized box: Virtualized → StealApplies true.
	t.Run("VirtualizedBox", func(t *testing.T) {
		st := &WindowState{}
		_, sig := Decide(st, Sample{Timestamp: ts, LogicalCpus: 8.0, Virtualized: true}, thresholds)
		if !sig.StealApplies {
			t.Fatalf("StealApplies: got false, want true (virtualized means the steal rule applies)")
		}
		if sig.LimitApplies {
			t.Fatalf("LimitApplies: got true, want false (no Quota set)")
		}
		if sig.PsiApplies {
			t.Fatalf("PsiApplies: got true, want false (PsiAvailable false)")
		}
	})

	// (d) Bare dead-zone box: no Quota, no PSI, not virtualized → all three
	// false and CapacityCores == LogicalCpus.
	t.Run("BareDeadZoneBox", func(t *testing.T) {
		st := &WindowState{}
		_, sig := Decide(st, Sample{Timestamp: ts, LogicalCpus: 8.0}, thresholds)
		if sig.LimitApplies || sig.PsiApplies || sig.StealApplies {
			t.Fatalf("bare dead-zone box: LimitApplies=%v PsiApplies=%v StealApplies=%v, want all false", sig.LimitApplies, sig.PsiApplies, sig.StealApplies)
		}
		if !floatEq(sig.CapacityCores, 8.0) {
			t.Fatalf("CapacityCores: got %v, want 8.0 (dead-zone → capacity is LogicalCpus)", sig.CapacityCores)
		}
	})
}

// TestDecide_UsagePercentilesAreAbsoluteCores pins R8: the avg/p95/p99 usage
// signals are absolute core counts (mirrored to mCPU by the wire via *1000),
// NOT the cgroup-relative usage fraction. On a no-limit dead-zone box
// (CgroupCores == 0, no Quota), the fraction is 0 (no denominator), so a
// fraction-based percentile would collapse to 0 even though the box is using
// real cores. The percentile must reflect the actual core usage.
func TestDecide_UsagePercentilesAreAbsoluteCores(t *testing.T) {
	base := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// No-limit dead-zone box (mirrors the live VM): no Quota, CgroupCores 0,
	// no PSI, 8 logical host cores. Feed a steady 2.75 cores of usage.
	noLimitDeadZone := func(dt time.Duration, usageCores float64) Sample {
		return Sample{
			Timestamp:    base.Add(dt),
			UsageCores:   usageCores,
			CgroupCores:  0,
			LogicalCpus:  8.0,
			PsiAvailable: false,
			Virtualized:  false,
		}
	}

	st := &WindowState{}
	var sig Signals
	for i := 0; i < 6; i++ {
		_, sig = Decide(st, noLimitDeadZone(time.Duration(i*10)*time.Second, 2.75), thresholds)
	}

	if !sig.UsageRingActive {
		t.Fatalf("UsageRingActive: got false, want true (dead-zone ring holds >= 2 samples)")
	}
	// The box uses 2.75 cores every tick. avg/p95/p99 CORES must reflect that
	// (→ 2750 mCPU on the wire), NOT 0 (which is what UsageCores/CgroupCores
	// gives when CgroupCores is 0). The *Fraction fields stay 0 here (no cgroup
	// denominator) — correct for the latch/message, and exactly why the wire
	// needs the separate *Cores fields.
	if !floatEq(sig.AvgUsageCores, 2.75) {
		t.Fatalf("AvgUsageCores: got %v, want 2.75 (absolute cores, not the collapsed cgroup fraction 0)", sig.AvgUsageCores)
	}
	if !floatEq(sig.P95UsageCores, 2.75) {
		t.Fatalf("P95UsageCores: got %v, want 2.75 (absolute cores)", sig.P95UsageCores)
	}
	if !floatEq(sig.P99UsageCores, 2.75) {
		t.Fatalf("P99UsageCores: got %v, want 2.75 (absolute cores)", sig.P99UsageCores)
	}
	if !floatEq(sig.AvgUsageFraction, 0) {
		t.Fatalf("AvgUsageFraction: got %v, want 0 (no cgroup denominator on a no-limit box)", sig.AvgUsageFraction)
	}
}

// TestDecide_LimitMode_Headroom pins the limit-mode headroom formula:
// headroom = quota − containerUsage60s − LimitReserveFraction×quota. In limit
// mode (Quota non-nil and > 0) the ceiling is the quota, the measured use is
// the container's own 60s-avg usage (the ring's cores mean), and the reserve
// is a FRACTION of the quota (strawman 0.10), NOT the flat 1.0-core host
// reserve. headroom < 0 IS the fire boundary; Schmitt recover.
func TestDecide_LimitMode_Headroom(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// feedLimit pushes n ticks of the given UsageCores onto st's usage ring at
	// 1s spacing with the given Quota/LogicalCpus, returning the final
	// verdict+signals. HostBusyCores is 0 (limit-mode headroom reads container
	// usage, not host busyness).
	feedLimit := func(st *WindowState, n int, quota *float64, logical float64, usage ...float64) (Verdict, Signals) {
		var (
			v Verdict
			s Signals
		)
		for i := 0; i < n; i++ {
			u := 0.0
			if i < len(usage) {
				u = usage[i]
			}
			v, s = Decide(st, Sample{
				Timestamp:     base.Add(time.Duration(i) * time.Second),
				UsageCores:    u,
				HostBusyCores: 0,
				LogicalCpus:   logical,
				Quota:         quota,
			}, thresholds)
		}
		return v, s
	}

	// (1) QuotaSet — 2 ticks UsageCores=1.8, Quota=2.0, LogicalCpus=8.0 →
	// usageMean=1.8, reserve=0.10×2=0.2, headroom=2−1.8−0.2=0.0. Pin
	// HeadroomCores == 0.0 (the boundary — exactly at reserve, not < 0).
	// Forcing: a stub using hostBusyMean (0 here) yields 2−0−0.2=1.8
	// (!floatEq 0.0); a stub using the flat 1.0 reserve yields 2−1.8−1.0=−0.8
	// (!floatEq 0.0).
	t.Run("QuotaSet", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		_, sig := feedLimit(st, 2, &quota, 8.0, 1.8, 1.8)
		if !floatEq(sig.HeadroomCores, 0.0) {
			t.Fatalf("HeadroomCores: got %v, want 0.0 (capacity=Quota 2.0 − usageMean 1.8 − LimitReserveFraction×2.0=0.2; a stub using hostBusyMean (0) yields 1.8, a stub using flat 1.0 reserve yields −0.8)", sig.HeadroomCores)
		}
		// The exact-boundary State (headroom=0.0 → healthy) is NOT asserted
		// here: 2.0 − 1.8 − 0.2 is a tiny negative (~−5.6e-17) in float64 (1.8
		// and 0.2 are not exactly representable), so the latch correctly fires
		// on the < 0 side. The < 0 vs <= 0 boundary is pinned by NearLimitFires
		// (headroom −0.1 → degraded) and FreshStateMeanZero (headroom 1.8 →
		// healthy) below, and by the no-limit PARKED_ON_LINE_FRESH (4−3−1=0.0
		// exact → healthy).
	})

	// (2) NearLimitFires — 2 ticks UsageCores=1.9, Quota=2.0 → headroom =
	// 2−1.9−0.2=−0.1 < 0 → degraded + saturation cause. Pins the fire
	// boundary: sustained usage inside the fractional reserve band.
	t.Run("NearLimitFires", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		v, sig := feedLimit(st, 2, &quota, 8.0, 1.9, 1.9)
		if !(sig.HeadroomCores < 0) {
			t.Fatalf("HeadroomCores: got %v, want < 0 (2−1.9−0.2=−0.1)", sig.HeadroomCores)
		}
		if !floatEq(sig.HeadroomCores, -0.1) {
			t.Fatalf("HeadroomCores: got %v, want -0.1 (2−1.9−0.2)", sig.HeadroomCores)
		}
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (headroom < 0 → degraded)", v.State, StateDegraded)
		}
		hasSat := false
		for _, c := range v.Causes {
			if c.Kind == CauseKindSaturation {
				hasSat = true
			}
		}
		if !hasSat {
			t.Fatalf("Causes: no saturation in %v (headroom < 0 must emit saturation)", v.Causes)
		}
	})

	// (3) FreshStateMeanZero — 1 tick UsageCores=1.9, Quota=2.0 (ring<2 →
	// mean 0) → headroom=2−0−0.2=1.8 > 0, healthy, no causes. Pins the
	// 2-sample floor in limit mode.
	t.Run("FreshStateMeanZero", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		v, sig := feedLimit(st, 1, &quota, 8.0, 1.9)
		if !floatEq(sig.HeadroomCores, 1.8) {
			t.Fatalf("HeadroomCores: got %v, want 1.8 (capacity 2.0 − floored mean 0.0 (ring<2) − reserve 0.2; using the raw 1-tick 1.9 yields −0.1)", sig.HeadroomCores)
		}
		if v.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (headroom > 0 → healthy)", v.State, StateHealthy)
		}
		if len(v.Causes) != 0 {
			t.Fatalf("Causes length: got %d, want 0", len(v.Causes))
		}
	})

	// (4) MeanNotLastTick — ticks [1.0, 1.8], Quota=2.0 → mean=1.4, headroom =
	// 2−1.4−0.2=0.4. Forcing: last tick 1.8 yields 2−1.8−0.2=0.0 (!floatEq 0.4).
	t.Run("MeanNotLastTick", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		_, sig := feedLimit(st, 2, &quota, 8.0, 1.0, 1.8)
		if !floatEq(sig.HeadroomCores, 0.4) {
			t.Fatalf("HeadroomCores: got %v, want 0.4 (capacity 2.0 − mean(1.0,1.8)=1.4 − reserve 0.2; using the last tick 1.8 yields 0.0)", sig.HeadroomCores)
		}
	})
}

// TestDecide_LimitMode_BFalseFireKilled is the scenario-B regression test: a
// limited container (Quota=2.0) on an 8-core host that is 62.5% busy
// (HostBusyCores=5.0) with the container idle (UsageCores=0.0).
// HostBusyCoresAvailable is set true so the hostBusyRing fills with 5.0 and
// hostBusyMean=5.0 (not 0). Old code paired hostBusyMean with the quota
// ceiling: headroom = 2−5−1 = −4 < 0 → degraded (BUG — fired at 12.5% host
// busy on a 2-core quota). New code pairs the container's own 60s-avg usage
// (usageCores60sMean) with the quota: headroom = 2−0−0.2 = 1.8 > 0 → HEALTHY.
// This is THE key regression test for the two-rule model: a revert to the old
// formula fires Degraded (the bug), the current code stays Healthy.
func TestDecide_LimitMode_BFalseFireKilled(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	quota := 2.0
	st := &WindowState{}
	var (
		v   Verdict
		sig Signals
	)
	for i := 0; i < 2; i++ {
		v, sig = Decide(st, Sample{
			Timestamp:              base.Add(time.Duration(i) * time.Second),
			Quota:                  &quota,
			LogicalCpus:            8.0,
			HostBusyCores:          5.0,  // host 62.5% busy
			HostBusyCoresAvailable: true, // ring fills → hostBusyMean=5.0
			UsageCores:             0.0,  // container idle
		}, thresholds)
	}
	// Forcing: with HostBusyCoresAvailable=true the ring fills, so
	// hostBusyMean=5.0. A stub reverting to the old formula (pairing
	// hostBusyMean with the quota ceiling) yields headroom 2−5−1=−4 < 0 and
	// fires Degraded (the old bug). The current two-rule formula pairs
	// usageCores60sMean (=0.0) with the quota: 2−0−0.2=1.8 > 0 → Healthy.
	if v.State != StateHealthy {
		t.Fatalf("State: got %q, want %q (limit mode: headroom=2−0−0.2=1.8 > 0 → healthy; old code paired hostBusyMean with quota → 2−5−1=−4 < 0 → false fire)", v.State, StateHealthy)
	}
	if sig.SaturationFired {
		t.Fatalf("SaturationFired: got true, want false (limit mode: container idle → headroom > 0 → no saturation)")
	}
	if len(v.Causes) != 0 {
		t.Fatalf("Causes length: got %d, want 0 (healthy)", len(v.Causes))
	}
	if !floatEq(sig.HeadroomCores, 1.8) {
		t.Fatalf("HeadroomCores: got %v, want 1.8 (2−0−0.2; old code: 2−5−1=−4)", sig.HeadroomCores)
	}
}

// TestDecide_LimitMode_ContainerIdleHostFull_NoSaturation pins the spec
// acceptance row "limited container, host has room → [pressure] only, no
// saturation": a limited container (Quota=2.0) on an 8-core host with
// HostBusyCores=5.0, container usage 0.3, and pressure firing
// (PressureAvg60=0.25 > PressureHigh 0.20). The verdict degrades on pressure
// ALONE — container usage 0.3 → headroom 2−0.3−0.2=1.5 > 0 → saturation does
// NOT co-fire. This pins that host busyness alone no longer fires saturation
// in limit mode (the R10.1 core fix).
func TestDecide_LimitMode_ContainerIdleHostFull_NoSaturation(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	quota := 2.0
	st := &WindowState{}
	var (
		v   Verdict
		sig Signals
	)
	for i := 0; i < 2; i++ {
		v, sig = Decide(st, Sample{
			Timestamp:              base.Add(time.Duration(i) * time.Second),
			Quota:                  &quota,
			LogicalCpus:            8.0,
			HostBusyCores:          5.0,
			HostBusyCoresAvailable: true, // ring fills → hostBusyMean=5.0
			UsageCores:             0.3,
			PressureAvg60:          0.25, // > PressureHigh 0.20 → pressure fires
			PsiAvailable:           true,
		}, thresholds)
	}
	if v.State != StateDegraded {
		t.Fatalf("State: got %q, want %q (pressure fired → degraded)", v.State, StateDegraded)
	}
	// Pressure is the ONLY cause — no saturation co-fire (container usage 0.3
	// → headroom 2−0.3−0.2=1.5 > 0).
	if len(v.Causes) != 1 {
		t.Fatalf("Causes length: got %d, want 1 (pressure only; container usage 0.3 → headroom 1.5 > 0 → no saturation)", len(v.Causes))
	}
	if v.Causes[0].Kind != CauseKindPressure {
		t.Fatalf("Cause Kind: got %q, want %q (pressure only, no saturation)", v.Causes[0].Kind, CauseKindPressure)
	}
	if sig.SaturationFired {
		t.Fatalf("SaturationFired: got true, want false (container usage 0.3 → headroom 1.5 > 0 → no saturation)")
	}
}

// TestDecide_LimitMode_HostFullStacks pins R10.2: in limit mode, when the HOST
// itself is full (the rule-1 test at host scope: LogicalCpus − hostBusyMean −
// cpuReserveCores < 0, readable via HostBusyCoresAvailable), that stacks as a
// second saturation condition on the limited container. A limit is a ceiling,
// not a reservation, so a full host starves a container below its limit. Two
// internal latches (limitSaturationFired container-scope + hostFullFired
// host-scope) emit ONE saturation cause; when both fire, HOST-FULL DOMINATES
// (attribution host, cause Value = the host-scope negative headroom). The
// host-full test is the TRUE host-scope test — NEVER quota-scoped (that was
// the B false fire R10.1 killed).
func TestDecide_LimitMode_HostFullStacks(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	// feedLimitHost pushes n ticks at 1s spacing with the given Quota,
	// LogicalCpus, HostBusyCores, HostBusyCoresAvailable, and UsageCores,
	// returning the final (Verdict, Signals).
	feedLimitHost := func(st *WindowState, n int, quota, logical, hostBusy, usage float64, hostStats bool) (Verdict, Signals) {
		var (
			v Verdict
			s Signals
		)
		for i := 0; i < n; i++ {
			v, s = Decide(st, Sample{
				Timestamp:              base.Add(time.Duration(i) * time.Second),
				Quota:                  &quota,
				LogicalCpus:            logical,
				HostBusyCores:          hostBusy,
				HostBusyCoresAvailable: hostStats,
				UsageCores:             usage,
			}, thresholds)
		}
		return v, s
	}

	// (1) HostFullFires — 2 ticks Quota=2.0, LogicalCpus=8.0, HostBusyCores=7.5
	// (host full: 8−7.5−1=−0.5 < 0), HostBusyCoresAvailable=true, UsageCores=0.0
	// (container idle → limit-sat does NOT fire: 2−0−0.2=1.8 > 0). The host-full
	// latch fires alone → degraded, attribution host, ONE saturation cause whose
	// Value is the host-scope negative headroom (−0.5, the dominant). Forcing: a
	// stub that doesn't add the host-full latch leaves the container healthy (the
	// B-regression resurfaces); a stub that quota-scopes the host-full test
	// (2−7.5−1 < 0) would also fire here but is the WRONG test — pinned via the
	// next sub-test.
	t.Run("HostFullFires", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		v, sig := feedLimitHost(st, 2, quota, 8.0, 7.5, 0.0, true)
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (host full: 8−7.5−1=−0.5 < 0 stacks on the limited container → degraded)", v.State, StateDegraded)
		}
		if !sig.SaturationFired {
			t.Fatalf("SaturationFired: got false, want true (host-full latch fires → emitted saturation)")
		}
		if !sig.HostFullFired {
			t.Fatalf("HostFullFired: got false, want true (host 8−7.5−1=−0.5 < 0 with HostBusyCoresAvailable → host-full latch fires)")
		}
		if sig.LimitSaturationFired {
			t.Fatalf("LimitSaturationFired: got true, want false (container idle: 2−0−0.2=1.8 > 0 → limit-sat does NOT fire)")
		}
		if v.Attribution != AttributionHost {
			t.Fatalf("Attribution: got %q, want %q (host-full fires → host-scope condition → attribution host)", v.Attribution, AttributionHost)
		}
		if len(v.Causes) != 1 {
			t.Fatalf("Causes length: got %d, want 1 (ONE saturation cause, not two)", len(v.Causes))
		}
		if v.Causes[0].Kind != CauseKindSaturation {
			t.Fatalf("Cause Kind: got %q, want %q", v.Causes[0].Kind, CauseKindSaturation)
		}
		if !floatEq(v.Causes[0].Value, -0.5) {
			t.Fatalf("Cause Value: got %v, want -0.5 (the host-scope negative headroom, the dominant ceiling; NOT the limit-scope 1.8)", v.Causes[0].Value)
		}
	})

	// (2) HostNotFullDoesntFire — 2 ticks Quota=2.0, LogicalCpus=8.0,
	// HostBusyCores=5.0 (host 5/8 → 8−5−1=2 > 0 → host-full does NOT fire),
	// UsageCores=0.0 (limit-sat 1.8 > 0 → doesn't fire). HEALTHY. The B-regression
	// stays killed: host busyness alone does not fire saturation in limit mode.
	t.Run("HostNotFullDoesntFire", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		v, sig := feedLimitHost(st, 2, quota, 8.0, 5.0, 0.0, true)
		if v.State != StateHealthy {
			t.Fatalf("State: got %q, want %q (host 8−5−1=2 > 0 → host-full does NOT fire; limit-sat 1.8 > 0 → does NOT fire → healthy)", v.State, StateHealthy)
		}
		if sig.HostFullFired {
			t.Fatalf("HostFullFired: got true, want false (host 8−5−1=2 > 0 → host not full)")
		}
		if sig.SaturationFired {
			t.Fatalf("SaturationFired: got true, want false (neither latch fires)")
		}
	})

	// (3) BothFireHostFullDominates — 2 ticks Quota=2.0, LogicalCpus=8.0,
	// HostBusyCores=7.5, UsageCores=1.9 (limit-sat fires: 2−1.9−0.2=−0.1 < 0;
	// host-full fires: 8−7.5−1=−0.5 < 0). Both latches fire → ONE saturation
	// cause, attribution host, cause Value = the host-scope −0.5 (the dominant —
	// NOT the limit-scope −0.1). Forcing: a stub that emits two saturation
	// causes fails the "ONE cause" pin; a stub that uses the limit-scope headroom
	// as the Value when both fire yields −0.1 (!floatEq −0.5).
	t.Run("BothFireHostFullDominates", func(t *testing.T) {
		quota := 2.0
		st := &WindowState{}
		v, sig := feedLimitHost(st, 2, quota, 8.0, 7.5, 1.9, true)
		if v.State != StateDegraded {
			t.Fatalf("State: got %q, want %q (both latches fire → degraded)", v.State, StateDegraded)
		}
		if len(v.Causes) != 1 {
			t.Fatalf("Causes length: got %d, want 1 (ONE saturation cause, not two — host-full dominates but does not add a second cause)", len(v.Causes))
		}
		if v.Causes[0].Kind != CauseKindSaturation {
			t.Fatalf("Cause Kind: got %q, want %q", v.Causes[0].Kind, CauseKindSaturation)
		}
		if v.Attribution != AttributionHost {
			t.Fatalf("Attribution: got %q, want %q (host-full fires → host-scope condition → attribution host)", v.Attribution, AttributionHost)
		}
		if !floatEq(v.Causes[0].Value, -0.5) {
			t.Fatalf("Cause Value: got %v, want -0.5 (host-scope negative headroom, the dominant ceiling — NOT the limit-scope −0.1)", v.Causes[0].Value)
		}
		if !sig.HostFullFired {
			t.Fatalf("HostFullFired: got false, want true (host 8−7.5−1=−0.5 < 0)")
		}
		if !sig.LimitSaturationFired {
			t.Fatalf("LimitSaturationFired: got false, want true (limit-sat 2−1.9−0.2=−0.1 < 0)")
		}
		if !sig.SaturationFired {
			t.Fatalf("SaturationFired: got false, want true (OR of both latches)")
		}
	})

	// (4) HostFullNoHostStats_Cleared — scenario C: /proc/stat unreadable
	// (HostBusyCoresAvailable=false, HostBusyCores=0). (4a) container idle →
	// HEALTHY (host-full can't fire without host stats; limit-sat 1.8 > 0 →
	// doesn't fire). (4b) container near limit (UsageCores=1.9) → DEGRADED via
	// limit-sat ONLY (HostFullFired==false, LimitSaturationFired==true),
	// attribution from the split (hb=0, uc=1.9 → 0−1.9 < 1.9 → not host →
	// unknown).
	t.Run("HostFullNoHostStats_Cleared", func(t *testing.T) {
		quota := 2.0

		// (4a) container idle, no host stats → healthy.
		stA := &WindowState{}
		vA, sigA := feedLimitHost(stA, 2, quota, 8.0, 0.0, 0.0, false)
		if vA.State != StateHealthy {
			t.Fatalf("(4a) State: got %q, want %q (no host stats → host-full can't fire; limit-sat 1.8 > 0 → doesn't fire → healthy)", vA.State, StateHealthy)
		}
		if sigA.HostFullFired {
			t.Fatalf("(4a) HostFullFired: got true, want false (HostBusyCoresAvailable=false → host-full latch cleared/not evaluated)")
		}
		if sigA.SaturationFired {
			t.Fatalf("(4a) SaturationFired: got true, want false (neither latch fires without host stats and container idle)")
		}

		// (4b) container near limit, no host stats → degraded via limit-sat ONLY.
		stB := &WindowState{}
		vB, sigB := feedLimitHost(stB, 2, quota, 8.0, 0.0, 1.9, false)
		if vB.State != StateDegraded {
			t.Fatalf("(4b) State: got %q, want %q (limit-sat 2−1.9−0.2=−0.1 < 0 → degraded)", vB.State, StateDegraded)
		}
		if !sigB.LimitSaturationFired {
			t.Fatalf("(4b) LimitSaturationFired: got false, want true (limit-sat fires without host stats)")
		}
		if sigB.HostFullFired {
			t.Fatalf("(4b) HostFullFired: got true, want false (HostBusyCoresAvailable=false → host-full can't fire)")
		}
		if vB.Attribution != AttributionUnknown {
			t.Fatalf("(4b) Attribution: got %q, want %q (no host stats → hb=0, uc=1.9 → 0−1.9 < 1.9 → not host → unknown)", vB.Attribution, AttributionUnknown)
		}
	})
}

// TestDecide_Saturation_HeadroomTrigger_NoLimitByteIdentity re-confirms the
// no-limit path is byte-identical after R10.2: the existing FULL_BOX_FIRES
// (Quota nil, host 8/8) still yields StateDegraded, attribution host (via the
// split: hb 8 − uc 0 > uc 0 → host), ONE saturation cause. The no-limit branch
// body is unchanged; only the two latch-clears (limitSaturationFired=false,
// hostFullFired=false) were added, which don't affect no-limit's
// st.saturationFired.
func TestDecide_Saturation_HeadroomTrigger_NoLimitByteIdentity(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	st := &WindowState{}
	var (
		v   Verdict
		sig Signals
	)
	for i := 0; i < 2; i++ {
		v, sig = Decide(st, Sample{
			Timestamp:              base.Add(time.Duration(i) * time.Second),
			HostBusyCores:          8.0,
			HostBusyCoresAvailable: true,
			LogicalCpus:            8.0,
		}, thresholds)
	}
	if v.State != StateDegraded {
		t.Fatalf("State: got %q, want %q (no-limit full box: headroom=8−8−1=−1.0 < 0 → degraded; byte-identical to R10.1)", v.State, StateDegraded)
	}
	if !sig.SaturationFired {
		t.Fatalf("SaturationFired: got false, want true (no-limit headroom < 0 → saturation fires; byte-identical)")
	}
	if v.Attribution != AttributionHost {
		t.Fatalf("Attribution: got %q, want %q (no-limit split: hb 8 − uc 0 = 8 > 0 → host; byte-identical — HostFullFired is false in no-limit mode, so the host-full attribution rule does not engage)", v.Attribution, AttributionHost)
	}
	if sig.HostFullFired {
		t.Fatalf("HostFullFired: got true, want false (no-limit mode → host-full latch cleared, not evaluated)")
	}
	if sig.LimitSaturationFired {
		t.Fatalf("LimitSaturationFired: got true, want false (no-limit mode → limit-sat latch cleared, not evaluated)")
	}
	if len(v.Causes) != 1 {
		t.Fatalf("Causes length: got %d, want 1 (ONE saturation cause; byte-identical)", len(v.Causes))
	}
	if v.Causes[0].Kind != CauseKindSaturation {
		t.Fatalf("Cause Kind: got %q, want %q", v.Causes[0].Kind, CauseKindSaturation)
	}
}

// TestDecide_NoHostStatsSaturation_ReachableAndFires is THE key R10.3 test: the no-PSI/no-limit
// fallback (scenario D) is reachable AND correct. /proc/stat unreadable
// (HostBusyCoresAvailable=false, HostBusyCores=0), no limit (Quota nil),
// PsiAvailable false, LogicalCpus 8.0, UsageCores 6.0 sustained (6.0/8.0=0.75
// >= HighUsageFraction 0.70). 2+ ticks. Today the no-limit branch fires on
// HeadroomCores (LogicalCpus − hostBusyMean − reserve = 8−0−1 = 7 > 0) and
// reads healthy forever — the blind spot. R10.3 gates on
// HostBusyCoresAvailable so the unreadable case falls through to the no-host-stats saturation
// fraction fallback (usageCores60sMean/LogicalCpus >= 0.70).
func TestDecide_NoHostStatsSaturation_ReachableAndFires(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	st := &WindowState{}
	var (
		v   Verdict
		sig Signals
	)
	for i := 0; i < 3; i++ {
		v, sig = Decide(st, Sample{
			Timestamp:              base.Add(time.Duration(i) * time.Second),
			HostBusyCores:          0,
			HostBusyCoresAvailable: false,
			Quota:                  nil,
			PsiAvailable:           false,
			LogicalCpus:            8.0,
			UsageCores:             6.0,
		}, thresholds)
	}
	if v.State != StateDegraded {
		t.Fatalf("State: got %q, want %q (no-host-stats saturation: usageCores60sMean 6.0 / LogicalCpus 8.0 = 0.75 >= 0.70 → degraded; today healthy — the blind spot)", v.State, StateDegraded)
	}
	if !sig.SaturationFired {
		t.Fatalf("SaturationFired: got false, want true (no-host-stats saturation fraction >= 0.70)")
	}
	if !sig.NoHostStatsSaturationFired {
		t.Fatalf("NoHostStatsSaturationFired: got false, want true (no-host-stats saturation branch fired)")
	}
	if !floatEq(sig.NoHostStatsSaturationFraction, 0.75) {
		t.Fatalf("NoHostStatsSaturationFraction: got %v, want 0.75 (6.0/8.0)", sig.NoHostStatsSaturationFraction)
	}
	if len(v.Causes) != 1 || v.Causes[0].Kind != CauseKindSaturation {
		t.Fatalf("Causes: got %+v, want one saturation cause", v.Causes)
	}
	if !floatEq(v.Causes[0].Value, 0.75) {
		t.Fatalf("Cause Value: got %v, want 0.75 (the no-host-stats saturation fraction)", v.Causes[0].Value)
	}
	if sig.HostFullFired {
		t.Fatalf("HostFullFired: got true, want false (no host stats → host-full can't fire; holds false since it never fired)")
	}
}

// TestDecide_NoHostStatsSaturation_BelowThresholdHealthy pins the guardrail: blind-but-quiet =
// healthy (not a third state). Same as ReachableAndFires but UsageCores 3.0
// (3.0/8.0 = 0.375 < 0.70) → HEALTHY, SaturationFired false, NoHostStatsSaturationFired false.
func TestDecide_NoHostStatsSaturation_BelowThresholdHealthy(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	st := &WindowState{}
	var (
		v   Verdict
		sig Signals
	)
	for i := 0; i < 3; i++ {
		v, sig = Decide(st, Sample{
			Timestamp:              base.Add(time.Duration(i) * time.Second),
			HostBusyCores:          0,
			HostBusyCoresAvailable: false,
			Quota:                  nil,
			PsiAvailable:           false,
			LogicalCpus:            8.0,
			UsageCores:             3.0,
		}, thresholds)
	}
	if v.State != StateHealthy {
		t.Fatalf("State: got %q, want %q (no-host-stats saturation: 3.0/8.0 = 0.375 < 0.70 → healthy; blind-but-quiet = healthy)", v.State, StateHealthy)
	}
	if sig.SaturationFired {
		t.Fatalf("SaturationFired: got true, want false (below threshold)")
	}
	if sig.NoHostStatsSaturationFired {
		t.Fatalf("NoHostStatsSaturationFired: got true, want false (below threshold)")
	}
}

// TestDecide_NoHostStatsSaturation_SchmittRecover pins the Schmitt recover band on the no-host-stats saturation
// latch: fire at 0.75, drop to 0.40 (< SaturationRecover 0.60) → latch clears
// → healthy. Then 0.65 (in the hold band 0.60..0.70) → latch stays fired
// (Schmitt hold).
func TestDecide_NoHostStatsSaturation_SchmittRecover(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	dSample := func(dt time.Duration, usage float64) Sample {
		return Sample{
			Timestamp:              base.Add(dt),
			HostBusyCores:          0,
			HostBusyCoresAvailable: false,
			Quota:                  nil,
			PsiAvailable:           false,
			LogicalCpus:            8.0,
			UsageCores:             usage,
		}
	}

	// (a) fire at 0.75 (6.0/8.0)
	st := &WindowState{}
	for i := 0; i < 3; i++ {
		Decide(st, dSample(time.Duration(i)*time.Second, 6.0), thresholds)
	}
	vFire, sigFire := Decide(st, dSample(3*time.Second, 6.0), thresholds)
	if vFire.State != StateDegraded {
		t.Fatalf("(a) fire State: got %q, want %q (0.75 >= 0.70 → degraded)", vFire.State, StateDegraded)
	}
	if !sigFire.NoHostStatsSaturationFired {
		t.Fatalf("(a) fire NoHostStatsSaturationFired: got false, want true")
	}

	// (b) drop to 0.40 (3.2/8.0) for enough ticks that the ring holds only 0.40
	// → noHostStatsSaturationFraction 0.40 < SaturationRecover 0.60 → latch clears.
	for i := 0; i < 70; i++ {
		Decide(st, dSample(time.Duration(4+i)*time.Second, 3.2), thresholds)
	}
	vClear, sigClear := Decide(st, dSample(73*time.Second, 3.2), thresholds)
	if vClear.State != StateHealthy {
		t.Fatalf("(b) clear State: got %q, want %q (0.40 < 0.60 SaturationRecover → latch clears)", vClear.State, StateHealthy)
	}
	if sigClear.NoHostStatsSaturationFired {
		t.Fatalf("(b) clear NoHostStatsSaturationFired: got true, want false (latch cleared)")
	}

	// (c) re-fire at 0.75, then 0.65 (5.2/8.0, in the hold band 0.60..0.70)
	// → latch stays fired (Schmitt hold).
	st2 := &WindowState{}
	for i := 0; i < 3; i++ {
		Decide(st2, dSample(time.Duration(i)*time.Second, 6.0), thresholds)
	}
	for i := 0; i < 70; i++ {
		Decide(st2, dSample(time.Duration(3+i)*time.Second, 5.2), thresholds)
	}
	vHold, sigHold := Decide(st2, dSample(72*time.Second, 5.2), thresholds)
	if vHold.State != StateDegraded {
		t.Fatalf("(c) hold State: got %q, want %q (0.65 in Schmitt band 0.60..0.70 → latch holds fired)", vHold.State, StateDegraded)
	}
	if !sigHold.NoHostStatsSaturationFired {
		t.Fatalf("(c) hold NoHostStatsSaturationFired: got false, want true (Schmitt hold in the band)")
	}
}

// TestDecide_NoLimit_HostStatsReadable_Unchanged (byte-identity): Quota nil,
// HostBusyCoresAvailable true, HostBusyCores 7.5, LogicalCpus 8.0 → headroom
// 8−7.5−1 = −0.5 < 0 → degraded via the host-headroom latch (unchanged). The
// gate diverts only the unreadable case; the readable path is byte-identical.
func TestDecide_NoLimit_HostStatsReadable_Unchanged(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	st := &WindowState{}
	var (
		v   Verdict
		sig Signals
	)
	for i := 0; i < 3; i++ {
		v, sig = Decide(st, Sample{
			Timestamp:              base.Add(time.Duration(i) * time.Second),
			HostBusyCores:          7.5,
			HostBusyCoresAvailable: true,
			Quota:                  nil,
			PsiAvailable:           false,
			LogicalCpus:            8.0,
			UsageCores:             0,
		}, thresholds)
	}
	if v.State != StateDegraded {
		t.Fatalf("State: got %q, want %q (host-headroom: 8−7.5−1=−0.5 < 0 → degraded; gate didn't break the readable path)", v.State, StateDegraded)
	}
	if !sig.SaturationFired {
		t.Fatalf("SaturationFired: got false, want true (host-headroom latch fires)")
	}
	if sig.NoHostStatsSaturationFired {
		t.Fatalf("NoHostStatsSaturationFired: got true, want false (host stats readable → host-headroom latch, not the no-host-stats saturation)")
	}
	if sig.HostFullFired {
		t.Fatalf("HostFullFired: got true, want false (no-limit mode → host-full latch not evaluated)")
	}
}

// TestDecide_ScenarioC_LimitNoHostStats verifies scenario C end-to-end: limit
// set (Quota=2.0), /proc/stat unreadable, UsageCores 1.9 → limit-saturation
// fires (2−1.9−0.2=−0.1 < 0), host-full can't fire (no host stats) → degraded
// via limit-sat ONLY. Limit mode needs no host stats.
func TestDecide_ScenarioC_LimitNoHostStats(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	quota := 2.0
	st := &WindowState{}
	var (
		v   Verdict
		sig Signals
	)
	for i := 0; i < 3; i++ {
		v, sig = Decide(st, Sample{
			Timestamp:              base.Add(time.Duration(i) * time.Second),
			Quota:                  &quota,
			HostBusyCores:          0,
			HostBusyCoresAvailable: false,
			PsiAvailable:           false,
			LogicalCpus:            8.0,
			UsageCores:             1.9,
		}, thresholds)
	}
	if v.State != StateDegraded {
		t.Fatalf("State: got %q, want %q (limit-sat: 2−1.9−0.2=−0.1 < 0 → degraded; limit mode needs no host stats)", v.State, StateDegraded)
	}
	if !sig.LimitSaturationFired {
		t.Fatalf("LimitSaturationFired: got false, want true (limit-sat fires without host stats)")
	}
	if sig.HostFullFired {
		t.Fatalf("HostFullFired: got true, want false (no host stats → host-full can't fire)")
	}
	if sig.NoHostStatsSaturationFired {
		t.Fatalf("NoHostStatsSaturationFired: got true, want false (limit mode, not the no-host-stats saturation)")
	}
}

// TestDecide_HostFull_HoldOnMissing (self-review hunch): limit 2, host 7.5/8
// full (HostBusyCoresAvailable=true), container idle → host-full fires
// (degraded). Then a tick with HostBusyCoresAvailable=false (transient outage)
// → latch HOLDS (stays degraded), does NOT clear. Then a tick with
// HostBusyCoresAvailable=true, host 2.5/8 (not full, headroom=8−2.5−1=4.5 >
// 0.5) → latch clears → healthy. Forcing: the old `else { clear }` would clear
// on the outage tick (flap).
func TestDecide_HostFull_HoldOnMissing(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	quota := 2.0

	// (a) fire: host full, container idle → host-full fires.
	st := &WindowState{}
	for i := 0; i < 3; i++ {
		Decide(st, Sample{
			Timestamp:              base.Add(time.Duration(i) * time.Second),
			Quota:                  &quota,
			HostBusyCores:          7.5,
			HostBusyCoresAvailable: true,
			PsiAvailable:           false,
			LogicalCpus:            8.0,
			UsageCores:             0,
		}, thresholds)
	}
	vFire, sigFire := Decide(st, Sample{
		Timestamp:              base.Add(3 * time.Second),
		Quota:                  &quota,
		HostBusyCores:          7.5,
		HostBusyCoresAvailable: true,
		PsiAvailable:           false,
		LogicalCpus:            8.0,
		UsageCores:             0,
	}, thresholds)
	if vFire.State != StateDegraded {
		t.Fatalf("(a) fire State: got %q, want %q (host full: 8−7.5−1=−0.5 < 0)", vFire.State, StateDegraded)
	}
	if !sigFire.HostFullFired {
		t.Fatalf("(a) fire HostFullFired: got false, want true")
	}

	// (b) transient outage: HostBusyCoresAvailable=false → latch HOLDS.
	vHold, sigHold := Decide(st, Sample{
		Timestamp:              base.Add(4 * time.Second),
		Quota:                  &quota,
		HostBusyCores:          0,
		HostBusyCoresAvailable: false,
		PsiAvailable:           false,
		LogicalCpus:            8.0,
		UsageCores:             0,
	}, thresholds)
	if !sigHold.HostFullFired {
		t.Fatalf("(b) hold HostFullFired: got false, want true (transient /proc/stat outage → latch HOLDS, does not clear → no flap)")
	}
	if vHold.State != StateDegraded {
		t.Fatalf("(b) hold State: got %q, want %q (latch held → still degraded)", vHold.State, StateDegraded)
	}

	// (c) recovery: HostBusyCoresAvailable=true, host not full (2.5/8) →
	// headroom=8−2.5−1=4.5 > headroomRecoverCores 0.5 → latch clears → healthy.
	for i := 0; i < 70; i++ {
		Decide(st, Sample{
			Timestamp:              base.Add(time.Duration(5+i) * time.Second),
			Quota:                  &quota,
			HostBusyCores:          2.5,
			HostBusyCoresAvailable: true,
			PsiAvailable:           false,
			LogicalCpus:            8.0,
			UsageCores:             0,
		}, thresholds)
	}
	vClear, sigClear := Decide(st, Sample{
		Timestamp:              base.Add(74 * time.Second),
		Quota:                  &quota,
		HostBusyCores:          2.5,
		HostBusyCoresAvailable: true,
		PsiAvailable:           false,
		LogicalCpus:            8.0,
		UsageCores:             0,
	}, thresholds)
	if sigClear.HostFullFired {
		t.Fatalf("(c) clear HostFullFired: got true, want false (host not full, headroom 4.5 > 0.5 → latch clears)")
	}
	if vClear.State != StateHealthy {
		t.Fatalf("(c) clear State: got %q, want %q (host-full cleared, container idle → healthy)", vClear.State, StateHealthy)
	}
}

// TestDecide_NoLimit_HostHeadroom_HoldOnMissing pins R10.3 Finding 1: a no-limit
// host-saturated box (host full, container idle) that suffers a transient
// /proc/stat outage must NOT flap degraded→healthy→degraded. The host-headroom
// fire holds across the outage tick (the no-host-stats saturation branch does not touch
// st.noLimitHostFired), and the emitted saturationFired is the OR of the two
// no-limit sub-latches.
func TestDecide_NoLimit_HostHeadroom_HoldOnMissing(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	th := DefaultThresholds()

	// (a) fire: no limit, host 7.5/8 full, container idle → host-headroom fires.
	st := &WindowState{}
	for i := 0; i < 3; i++ {
		Decide(st, Sample{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
			HostBusyCoresAvailable: true, UsageCores: 0,
		}, th)
	}
	vFire, _ := Decide(st, Sample{
		Timestamp: base.Add(3 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
		HostBusyCoresAvailable: true, UsageCores: 0,
	}, th)
	if vFire.State != StateDegraded {
		t.Fatalf("(a) fire State: got %q, want %q (host full: 8−7.5−1=−0.5 < 0)", vFire.State, StateDegraded)
	}

	// (b) transient outage: /proc/stat unreadable, container idle. The no-host-stats saturation
	// branch runs (noHostStatsSaturationFraction=0/8=0 < 0.60 → no-host-stats saturation clears) BUT the prior
	// host-headroom fire (st.noLimitHostFired) HOLDS — the no-host-stats saturation does not touch
	// it. Emitted saturationFired = noLimitHostFired || noHostStatsSaturationFired = true.
	vHold, sigHold := Decide(st, Sample{
		Timestamp: base.Add(4 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 0,
		HostBusyCoresAvailable: false, UsageCores: 0,
	}, th)
	if vHold.State != StateDegraded {
		t.Fatalf("(b) hold State: got %q, want %q (prior host-headroom fire holds across the /proc/stat outage → no flap)", vHold.State, StateDegraded)
	}
	if sigHold.SaturationFired {
		// SaturationFired should still be true (held). This asserts the hold.
	} else {
		t.Fatalf("(b) hold SaturationFired: got false, want true (st.noLimitHostFired held; no-host-stats saturation clear must not clobber it)")
	}
	if sigHold.NoHostStatsSaturationFired {
		t.Fatalf("(b) hold NoHostStatsSaturationFired: got true, want false (container idle → noHostStatsSaturationFraction 0 < 0.60 → no-host-stats saturation does not fire)")
	}

	// (c) outage resolves, host still full → still degraded (host-headroom re-fires).
	vBack, _ := Decide(st, Sample{
		Timestamp: base.Add(5 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
		HostBusyCoresAvailable: true, UsageCores: 0,
	}, th)
	if vBack.State != StateDegraded {
		t.Fatalf("(c) back State: got %q, want %q (host full again → degraded)", vBack.State, StateDegraded)
	}
}

// TestDecide_NoLimitHostFull_OutageHoldsAttributionHost pins the attribution
// complement to R10.3 Finding 1: on a no-limit host-full box, the verdict STATE
// holds across a transient /proc/stat outage (st.noLimitHostFired is not
// cleared by the no-host-stats saturation branch), but the attribution switch had no
// case signals.NoLimitHostFired — so the default branch ran with hb=0 (no
// host-busy reading) and hb-uc=0-0=0, not > 0, flipping attribution to
// AttributionUnknown. The MC would render a toggling Host→Unknown→Host badge
// while the verdict stays degraded. This test pins attribution to stay Host.
func TestDecide_NoLimitHostFull_OutageHoldsAttributionHost(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	th := DefaultThresholds()

	// (a) fire: no limit, host 7.5/8 full, container idle → host-headroom fires,
	// attribution is Host (host share 7.5 > container share 0).
	st := &WindowState{}
	for i := 0; i < 3; i++ {
		Decide(st, Sample{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
			HostBusyCoresAvailable: true, UsageCores: 0,
		}, th)
	}
	vFire, sigFire := Decide(st, Sample{
		Timestamp: base.Add(3 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
		HostBusyCoresAvailable: true, UsageCores: 0,
	}, th)
	if vFire.State != StateDegraded {
		t.Fatalf("(a) fire State: got %q, want %q (host full: 8−7.5−1=−0.5 < 0)", vFire.State, StateDegraded)
	}
	if vFire.Attribution != AttributionHost {
		t.Fatalf("(a) fire Attribution: got %q, want %q (host share 7.5 > container share 0)", vFire.Attribution, AttributionHost)
	}
	if !sigFire.NoLimitHostFired {
		t.Fatalf("(a) fire NoLimitHostFired: got false, want true (no-limit host-headroom fire)")
	}

	// (b) transient /proc/stat outage: HostBusyCoresAvailable=false, hb=0,
	// container idle. st.noLimitHostFired HOLDS (no-host-stats saturation branch does not touch
	// it) → State stays Degraded. Attribution must ALSO stay Host: the latched
	// no-limit host-full verdict is a host-side problem regardless of whether
	// /proc/stat is momentarily readable. Without a case for NoLimitHostFired
	// in the attribution switch, the default branch runs with hb=0 → hb-uc=0,
	// not > 0 → AttributionUnknown (the flap this test pins).
	vHold, _ := Decide(st, Sample{
		Timestamp: base.Add(4 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 0,
		HostBusyCoresAvailable: false, UsageCores: 0,
	}, th)
	if vHold.State != StateDegraded {
		t.Fatalf("(b) hold State: got %q, want %q (noLimitHostFired latch holds across outage)", vHold.State, StateDegraded)
	}
	if vHold.Attribution != AttributionHost {
		t.Fatalf("(b) hold Attribution: got %q, want %q (latched no-limit host-full verdict must keep AttributionHost through the /proc/stat outage — no flap)",
			vHold.Attribution, AttributionHost)
	}

	// (c) outage resolves, host still full → still degraded, attribution Host.
	vBack, _ := Decide(st, Sample{
		Timestamp: base.Add(5 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
		HostBusyCoresAvailable: true, UsageCores: 0,
	}, th)
	if vBack.State != StateDegraded {
		t.Fatalf("(c) back State: got %q, want %q (host full again → degraded)", vBack.State, StateDegraded)
	}
	if vBack.Attribution != AttributionHost {
		t.Fatalf("(c) back Attribution: got %q, want %q (host share 7.5 > container share 0)", vBack.Attribution, AttributionHost)
	}
}

// TestDecide_NoLimitHostOutage_SatValueNotPositiveHeadroom pins the Cause
// Value on a sustained /proc/stat outage in no-limit mode: NoLimitHostFired
// latches (verdict stays Degraded), but the ring ages out past the 60s window
// so hostBusyMean=0 and HeadroomCores=LogicalCpus-0-1=7 (large positive). The
// satValue initial assignment (satValue := signals.HeadroomCores) carries this
// large positive Value for a degraded saturation cause, which is semantically
// backwards (Value is documented as negative headroom / over-capacity). The
// Value must not be a large positive headroom on an outage tick.
func TestDecide_NoLimitHostOutage_SatValueNotPositiveHeadroom(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	th := DefaultThresholds()

	// (a) fire: no limit, host 7.5/8 full, container idle. 4 ticks fill the
	// hostBusyRing so the 2-sample floor passes and host-headroom fires.
	st := &WindowState{}
	for i := 0; i < 4; i++ {
		Decide(st, Sample{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
			HostBusyCoresAvailable: true, UsageCores: 0,
		}, th)
	}

	// (b) sustained outage: jump past the 60s hostBusyWindow so all readable
	// readings age out. HostBusyCoresAvailable=false so the ring is not
	// appended. hostBusyMean=0 (empty ring, 2-sample floor). HeadroomCores =
	// 8 - 0 - 1 = 7 (large positive). noLimitHostFired holds (the no-host-stats
	// saturation branch does not touch it). State stays Degraded.
	vHold, sigHold := Decide(st, Sample{
		Timestamp: base.Add(65 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 0,
		HostBusyCoresAvailable: false, UsageCores: 0,
	}, th)

	if vHold.State != StateDegraded {
		t.Fatalf("State: got %q, want %q (noLimitHostFired latch holds across the sustained outage)", vHold.State, StateDegraded)
	}
	if !sigHold.NoLimitHostFired {
		t.Fatalf("NoLimitHostFired: got false, want true (latch holds)")
	}
	if sigHold.HostBusyCoresAvailable {
		t.Fatalf("HostBusyCoresAvailable: got true, want false (outage tick)")
	}
	if len(vHold.Causes) == 0 {
		t.Fatalf("expected at least one cause, got none")
	}
	satVal := vHold.Causes[0].Value
	if satVal > 0 {
		t.Fatalf("Cause Value: got %v, want <= 0 (a degraded saturation cause must not carry a large positive headroom Value on an outage tick)", satVal)
	}
}

// TestDecide_NoLimitHostOutage_OverlapWithNoHostStats pins the satValue switch
// ordering fix: in the overlap NoLimitHostFired=true &&
// NoHostStatsSaturationFired=true && !HostBusyCoresAvailable (reachable when a
// host-headroom saturation latches noLimitHostFired on a readable tick, then
// /proc/stat drops while the container keeps high cgroup usage so the
// no-host-stats branch fills the usage ring and fires noHostStatsSaturationFired),
// the NoLimitHostFired && !HostBusyCoresAvailable satValue gate must NOT shadow
// the NoHostStatsSaturationFired arm. The overlap must fall through to
// satValue = NoHostStatsSaturationFraction (matching the NoHostStatsSaturationFired message arm),
// and the rendered message must show the NoHostStatsSaturationFraction percentage, not "0%".
func TestDecide_NoLimitHostOutage_OverlapWithNoHostStats(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	th := DefaultThresholds()

	// (a) fire: no limit, host 7.5/8 full, container idle. 4 ticks fill the
	// hostBusyRing so the 2-sample floor passes and host-headroom fires
	// (HeadroomCores = 8 - 7.5 - 1 = -0.5 < 0).
	st := &WindowState{}
	for i := 0; i < 4; i++ {
		Decide(st, Sample{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
			HostBusyCoresAvailable: true, UsageCores: 0,
		}, th)
	}

	// (b) jump past the 60s window so the readable hostBusyRing and the
	// zero-usage entries age out. Then outage ticks with high container usage
	// (6.0/8 = 0.75 >= HighUsageFraction 0.70) fill the usage ring and fire
	// noHostStatsSaturationFired. noLimitHostFired holds (the no-host-stats
	// branch does not touch it). After 2 outage ticks the overlap is reached:
	// NoLimitHostFired=true && NoHostStatsSaturationFired=true &&
	// !HostBusyCoresAvailable, NoHostStatsSaturationFraction=0.75.
	Decide(st, Sample{
		Timestamp: base.Add(65 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 0,
		HostBusyCoresAvailable: false, UsageCores: 6.0,
	}, th)
	vHold, sigHold := Decide(st, Sample{
		Timestamp: base.Add(66 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 0,
		HostBusyCoresAvailable: false, UsageCores: 6.0,
	}, th)

	if vHold.State != StateDegraded {
		t.Fatalf("State: got %q, want %q (overlap must stay Degraded)", vHold.State, StateDegraded)
	}
	if !sigHold.NoLimitHostFired {
		t.Fatalf("NoLimitHostFired: got false, want true (latch must hold across the outage)")
	}
	if !sigHold.NoHostStatsSaturationFired {
		t.Fatalf("NoHostStatsSaturationFired: got false, want true (high container usage on outage tick must fire)")
	}
	if sigHold.HostBusyCoresAvailable {
		t.Fatalf("HostBusyCoresAvailable: got true, want false (outage tick)")
	}
	if len(vHold.Causes) == 0 {
		t.Fatalf("expected at least one cause, got none")
	}

	// Regression pin: satValue must be NoHostStatsSaturationFraction (0.75), NOT 0. The new
	// NoLimitHostFired && !HostBusyCoresAvailable gate sits AFTER the
	// NoHostStatsSaturationFired arm, so the overlap falls through to
	// satValue = NoHostStatsSaturationFraction.
	satVal := vHold.Causes[0].Value
	if satVal != 0.75 {
		t.Fatalf("Cause Value: got %v, want 0.75 (NoHostStatsSaturationFraction; the new NoLimitHostFired gate must not shadow NoHostStatsSaturationFired in the overlap)", satVal)
	}

	// Message pin: the NoHostStatsSaturationFired arm renders the NoHostStatsSaturationFraction
	// percentage (75%), not "0%" (which the pre-fix satValue=0 produced).
	msg := ComposeMessage(vHold, sigHold)
	_, details, _ := strings.Cut(msg, "Technical Details: ")
	details = strings.TrimSpace(details)
	if !strings.Contains(details, "75%") {
		t.Fatalf("overlap detail must render the NoHostStatsSaturationFraction percentage 75%%: %q", details)
	}
	if strings.Contains(details, "0%") {
		t.Fatalf("overlap detail must NOT render 0%% (the pre-fix satValue=0 shadowed NoHostStatsSaturationFraction): %q", details)
	}
}

// TestDecide_NoLimitHostFull_ContainerIsCause_AttributionUnknown pins the
// DA-found regression in the attribution fix: on a no-limit host-full box
// where the CONTAINER ITSELF is the dominant cause (host-busy = container +
// small neighbour), the default heuristic must run and yield
// AttributionUnknown. NoLimitHostFired fires (headroom < 0), but on a READABLE
// tick (HostBusyCoresAvailable=true) the host/container split is computable
// (hb−uc = 0.5, not > uc=7.0) → AttributionUnknown (the container IS the
// cause, not a host/neighbour problem). A fix that unconditionally forces
// AttributionHost on every NoLimitHostFired tick breaks this case.
func TestDecide_NoLimitHostFull_ContainerIsCause_AttributionUnknown(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	th := DefaultThresholds()

	// no limit, host 7.5/8 full, container using 7.0 (7.0 container + 0.5
	// neighbour). HeadroomCores = 8−7.5−1 = −0.5 < 0 → NoLimitHostFired.
	// Default heuristic: hb−uc = 7.5−7.0 = 0.5, not > uc=7.0 →
	// AttributionUnknown (container is the cause).
	st := &WindowState{}
	for i := 0; i < 3; i++ {
		Decide(st, Sample{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
			HostBusyCoresAvailable: true, UsageCores: 7.0,
		}, th)
	}
	v, sig := Decide(st, Sample{
		Timestamp: base.Add(3 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
		HostBusyCoresAvailable: true, UsageCores: 7.0,
	}, th)
	if v.State != StateDegraded {
		t.Fatalf("State: got %q, want %q (host full: 8−7.5−1=−0.5 < 0)", v.State, StateDegraded)
	}
	if !sig.NoLimitHostFired {
		t.Fatalf("NoLimitHostFired: got false, want true (headroom −0.5 < 0)")
	}
	if v.Attribution != AttributionUnknown {
		t.Fatalf("Attribution: got %q, want %q (container is the dominant cause: hb−uc=0.5 not > uc=7.0 → default heuristic → Unknown)",
			v.Attribution, AttributionUnknown)
	}
}

// TestDecide_NoHostStatsSaturationToHostHeadroom_Transition pins R10.3 Finding 2: on a no-host-stats saturation →
// host-headroom transition (/proc/stat becomes readable mid-fire), st.noHostStatsSaturationFired
// is cleared by the host-headroom branch, so the saturation cause Value is
// signals.HeadroomCores (the host-scope headroom), NOT a stale
// signals.NoHostStatsSaturationFraction=0 routed through a stale NoHostStatsSaturationFired=true.
func TestDecide_NoHostStatsSaturationToHostHeadroom_Transition(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	th := DefaultThresholds()

	// (a) no-host-stats saturation fires: no host stats, no limit, usage 6/8 = 0.75 >= 0.70.
	st := &WindowState{}
	for i := 0; i < 3; i++ {
		Decide(st, Sample{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 0,
			HostBusyCoresAvailable: false, UsageCores: 6.0,
		}, th)
	}
	vFire, sigFire := Decide(st, Sample{
		Timestamp: base.Add(3 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 0,
		HostBusyCoresAvailable: false, UsageCores: 6.0,
	}, th)
	if vFire.State != StateDegraded {
		t.Fatalf("(a) fire State: got %q, want %q (no-host-stats saturation: 6/8=0.75 >= 0.70)", vFire.State, StateDegraded)
	}
	if !sigFire.NoHostStatsSaturationFired {
		t.Fatalf("(a) fire NoHostStatsSaturationFired: got false, want true")
	}
	// The cause Value is the no-host-stats saturation fraction (NoHostStatsSaturationFraction, 0.75).
	noHostStatsVal := vFire.Causes[0].Value
	if noHostStatsVal < 0.74 || noHostStatsVal > 0.76 {
		t.Fatalf("(a) fire cause Value: got %v, want ~0.75 (the no-host-stats saturation fraction)", noHostStatsVal)
	}

	// (b) /proc/stat becomes readable, host IS full (7.5/8). Routes to the
	// host-headroom branch. Feed 2 readable ticks (the 2-sample floor on
	// hostBusyRing — a first-tick reading cannot fire). st.noHostStatsSaturationFired MUST be
	// cleared (Finding 2) so the cause Value is signals.HeadroomCores (−0.5),
	// not a stale NoHostStatsSaturationFraction=0.
	Decide(st, Sample{
		Timestamp: base.Add(4 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
		HostBusyCoresAvailable: true, UsageCores: 6.0,
	}, th)
	vTrans, sigTrans := Decide(st, Sample{
		Timestamp: base.Add(5 * time.Second),
		Quota:     nil, LogicalCpus: 8.0, HostBusyCores: 7.5,
		HostBusyCoresAvailable: true, UsageCores: 6.0,
	}, th)
	if vTrans.State != StateDegraded {
		t.Fatalf("(b) transition State: got %q, want %q (host full: 8−7.5−1=−0.5 < 0; 2 readable ticks satisfy the floor)", vTrans.State, StateDegraded)
	}
	if sigTrans.NoHostStatsSaturationFired {
		t.Fatalf("(b) transition NoHostStatsSaturationFired: got true, want false (host-headroom branch must clear st.noHostStatsSaturationFired on entry — Finding 2)")
	}
	// The cause Value is the host-scope headroom (−0.5), NOT 0.
	transVal := vTrans.Causes[0].Value
	if transVal > -0.49 || transVal < -0.51 {
		t.Fatalf("(b) transition cause Value: got %v, want ~−0.5 (signals.HeadroomCores; a stale NoHostStatsSaturationFired would route to NoHostStatsSaturationFraction=0)", transVal)
	}
}

// TestDecide_Attribution_Uses60sMean_NotInstantaneous pins that the default
// attribution host/container split uses the 60s MEAN host-busy and usage
// (signals.HostBusyCores60sMean, signals.AvgUsageCores), NOT the per-tick
// instantaneous values. The verdict State is Schmitt-latched (stable across a
// transient spike), so the attribution label that annotates it must be equally
// stable — otherwise a single reconcile tick where host-busy spikes flips
// Attribution Host→Unknown→Host on a stable degraded verdict, and the MC would
// render a toggling attribution badge.
//
// Setup: no-limit box, 8 logical cores. Feed 5 readable ticks at 1s spacing
// with HostBusyCores=2.0 (mean 2.0) and UsageCores=1.0 (mean 1.0), plus a
// firing throttle (internal cause → attribution comes from the default split,
// not steal/host-full/no-limit-host). On the split: mean hb 2.0 − uc 1.0 = 1.0,
// NOT > uc 1.0 → AttributionUnknown.
//
// Then a SPIKE tick: HostBusyCores=7.5 (instantaneous) but
// HostBusyCoresAvailable=false so the hostBusyRing does NOT append it — the 60s
// mean stays 2.0. The instantaneous-only (OLD) split uses hb=7.5 → 7.5−1.0=6.5
// > 1.0 → AttributionHost (flap!). The 60s-mean (NEW) split uses hb=2.0 →
// 2.0−1.0=1.0, not > 1.0 → AttributionUnknown (stable). Throttle keeps firing
// on the spike tick so the verdict stays degraded and the split runs.
func TestDecide_Attribution_Uses60sMean_NotInstantaneous(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	thresholds := DefaultThresholds()

	st := &WindowState{}

	// tick returns a no-limit Sample at base+dt with the given HostBusyCores
	// (readable), UsageCores, and monotonic throttle counters. The throttle
	// counters advance by 1000 periods / 100 throttled per tick so the 60s
	// ratio is 0.10 (> ThrottleHigh 0.05) from tick 1 onward.
	tick := func(dt time.Duration, hostBusy, usage float64, periods, throttled int64) Sample {
		return Sample{
			Timestamp:              base.Add(dt),
			Quota:                  nil,
			LogicalCpus:            8.0,
			HostBusyCores:          hostBusy,
			HostBusyCoresAvailable: true,
			UsageCores:             usage,
			PsiAvailable:           false,
			NrPeriods:              periods,
			NrThrottled:            throttled,
		}
	}

	// (1) Establish the 60s means: 5 readable ticks at hb=2.0, uc=1.0, with a
	// firing throttle. After tick 1 the throttle ratio is 0.10 > 0.05 → latch
	// fires; the verdict is degraded (throttle, internal) and the default
	// attribution split runs. mean hb=2.0, mean uc=1.0 → 2.0−1.0=1.0 not > 1.0
	// → AttributionUnknown (the stable label).
	var v Verdict
	for i := 0; i < 5; i++ {
		v, _ = Decide(st, tick(time.Duration(i)*time.Second, 2.0, 1.0,
			int64(1000+i*1000), int64(i*100)), thresholds)
	}
	if v.State != StateDegraded {
		t.Fatalf("steady State: got %q, want %q (throttle fires → degraded)", v.State, StateDegraded)
	}
	if v.Attribution != AttributionUnknown {
		t.Fatalf("steady Attribution: got %q, want %q (mean hb 2.0 − uc 1.0 = 1.0, not > uc 1.0 → Unknown)", v.Attribution, AttributionUnknown)
	}

	// (2) SPIKE tick: instantaneous HostBusyCores=7.5 but
	// HostBusyCoresAvailable=false so the hostBusyRing does NOT append it — the
	// 60s mean stays 2.0. Throttle keeps firing (verdict stays degraded, split
	// runs). The OLD split uses instantaneous hb=7.5 → 7.5−1.0=6.5 > 1.0 → Host
	// (flap). The NEW split uses mean hb=2.0 → 2.0−1.0=1.0, not > 1.0 → Unknown
	// (stable, matching the verdict).
	spike := Sample{
		Timestamp:              base.Add(5 * time.Second),
		Quota:                  nil,
		LogicalCpus:            8.0,
		HostBusyCores:          7.5,   // instantaneous spike
		HostBusyCoresAvailable: false, // ring does NOT append → mean stays 2.0
		UsageCores:             1.0,
		PsiAvailable:           false,
		NrPeriods:              6000,
		NrThrottled:            500, // ratio (500-0)/(6000-1000)=0.10 > 0.05 → still firing
	}
	vSpike, sigSpike := Decide(st, spike, thresholds)
	if vSpike.State != StateDegraded {
		t.Fatalf("spike State: got %q, want %q (throttle still firing → still degraded; the verdict is stable)", vSpike.State, StateDegraded)
	}
	if vSpike.Attribution != AttributionUnknown {
		t.Fatalf("spike Attribution: got %q, want %q (the 60s mean hb stayed 2.0 — HostBusyCoresAvailable=false so the spike was NOT appended; 2.0−1.0=1.0 not > 1.0 → Unknown. The OLD instantaneous split uses hb=7.5 → 6.5 > 1.0 → Host (flap). Attribution must be as stable as the verdict it annotates.)", vSpike.Attribution, AttributionUnknown)
	}
	// Sanity: the mean really did stay 2.0 (the spike was not appended).
	if !floatEq(sigSpike.HostBusyCores60sMean, 2.0) {
		t.Fatalf("spike HostBusyCores60sMean: got %v, want 2.0 (HostBusyCoresAvailable=false → ring did not append the 7.5 spike; a mean that moved to include 7.5 would make the test non-forcing)", sigSpike.HostBusyCores60sMean)
	}
}
