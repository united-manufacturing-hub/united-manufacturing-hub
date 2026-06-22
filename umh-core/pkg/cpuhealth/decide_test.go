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
// pkg/cpuhealth library. It pins the four behaviors the saturation-only
// verdict contracts. The usage fraction's denominator is the cgroup quota
// (Sample.CgroupCores), not host cores; the test pins that.
//
//  1. A high-usage sample (UsageCores >= 0.70 * CgroupCores, CgroupCores>0)
//     yields State==degraded, Attribution==unknown, and exactly one Cause
//     whose Kind==saturation and whose Value is the usage fraction
//     (UsageCores/CgroupCores).
//  2. An idle/low-usage sample (UsageCores < 0.70 * CgroupCores) yields
//     State==healthy, Attribution empty/zero, Causes nil-or-empty.
//  3. When thresholds.HighUsageFraction <= 0, Decide falls back to
//     DefaultThresholds() (HighUsageFraction 0.70).
//  4. Signals.UsageFraction == UsageCores/CgroupCores when CgroupCores>0,
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

	// (1) High-usage sample crosses the 0.70 quota-relative threshold ->
	// degraded / unknown / exactly one saturation cause carrying the fraction.
	t.Run("high usage degrades with single saturation cause", func(t *testing.T) {
		st := &WindowState{}
		// 1.5 of 2.0 cores = 0.75 fraction, >= 0.70.
		sample := Sample{Timestamp: ts, UsageCores: 1.5, CgroupCores: 2.0}
		verdict, signals := Decide(st, sample, defaultThresholds)

		if verdict.State != StateDegraded {
			t.Fatalf("State: got %q, want %q", verdict.State, StateDegraded)
		}
		if verdict.Attribution != AttributionUnknown {
			t.Fatalf("Attribution: got %q, want %q", verdict.Attribution, AttributionUnknown)
		}
		if len(verdict.Causes) != 1 {
			t.Fatalf("Causes length: got %d, want exactly 1", len(verdict.Causes))
		}
		c := verdict.Causes[0]
		if c.Kind != CauseKindSaturation {
			t.Fatalf("Cause.Kind: got %q, want %q", c.Kind, CauseKindSaturation)
		}
		if got, want := c.Value, signals.UsageFraction; !floatEq(got, want) {
			t.Fatalf("Cause.Value: got %v, want usage fraction %v", got, want)
		}
		// The fraction is the quota-relative one (1.5/2.0 = 0.75): the
		// denominator is the cgroup quota, not host cores.
		if got, want := c.Value, 0.75; !floatEq(got, want) {
			t.Fatalf("Cause.Value (quota-relative): got %v, want 0.75", got)
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

	// (3) When thresholds.HighUsageFraction <= 0, Decide falls back to
	// DefaultThresholds() (HighUsageFraction 0.70).
	t.Run("invalid thresholds fall back to DefaultThresholds", func(t *testing.T) {
		// Distinguish fallback-to-default from an invalid <=0 threshold: the
		// 0.69 just-below sample is the distinguishing assertion (stays
		// healthy only under the 0.70 fallback, degrades under any <=0
		// threshold); the 0.70 boundary is a complementary sanity check that
		// degrades under both paths.
		st := &WindowState{}
		justBelow := Sample{Timestamp: ts, UsageCores: 1.38, CgroupCores: 2.0} // 0.69
		verdict, _ := Decide(st, justBelow, Thresholds{HighUsageFraction: 0})
		if verdict.State != StateHealthy {
			t.Fatalf("just-below-default (0.69) under invalid threshold: State got %q, want %q (fallback to default 0.70)", verdict.State, StateHealthy)
		}
		st2 := &WindowState{}
		atBoundary := Sample{Timestamp: ts, UsageCores: 1.4, CgroupCores: 2.0} // 0.70
		verdictBoundary, _ := Decide(st2, atBoundary, Thresholds{HighUsageFraction: -1})
		if verdictBoundary.State != StateDegraded {
			t.Fatalf("boundary (0.70) under invalid threshold: State got %q, want %q (fallback to default 0.70)", verdictBoundary.State, StateDegraded)
		}

		// A NaN threshold must also fall back to DefaultThresholds, not blind
		// the monitor. `NaN <= 0` is false, so a bare `<= 0` guard would keep
		// the NaN and `fraction >= NaN` (always false) would force healthy
		// even on a high-usage sample. The just-below-default sample stays
		// healthy (distinguishing fallback-to-0.70 from a kept-NaN that would
		// also read healthy — the high-usage sample below is the real pin).
		st3 := &WindowState{}
		nanThreshold := Thresholds{HighUsageFraction: math.NaN()}
		justBelowDefault := Sample{Timestamp: ts, UsageCores: 1.38, CgroupCores: 2.0} // 0.69
		verdictNanBelow, _ := Decide(st3, justBelowDefault, nanThreshold)
		if verdictNanBelow.State != StateHealthy {
			t.Fatalf("just-below-default (0.69) under NaN threshold: State got %q, want %q (fallback to default 0.70)", verdictNanBelow.State, StateHealthy)
		}
		st4 := &WindowState{}
		highUsage := Sample{Timestamp: ts, UsageCores: 1.5, CgroupCores: 2.0} // 0.75
		verdictNanHigh, _ := Decide(st4, highUsage, nanThreshold)
		if verdictNanHigh.State != StateDegraded {
			t.Fatalf("high-usage (0.75) under NaN threshold: State got %q, want %q (NaN must fall back to default, not blind)", verdictNanHigh.State, StateDegraded)
		}
	})
}

// floatEq compares two floats with a tolerance tight enough for these
// arithmetic-only assertions.
func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
