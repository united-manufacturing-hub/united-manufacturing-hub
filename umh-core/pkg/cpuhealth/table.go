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

// The CPU declaration: which signals exist, what each one measures, where its
// thresholds sit, and the constructor that builds an engine from it.

package cpuhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// The signal names. Every window CPU reads back is named by one of these, in
// the table that declares it and at the call site that reads it. An unnamed
// pair reduces to StateAbsent forever, so a string literal at either end is a
// typo that reads as a permanent absence — a constant makes it a compile error
// instead.
const (
	sigSaturation      = "saturation"
	sigLimitSaturation = "limit-saturation"
	sigThrottling      = "throttling"
	sigSteal           = "steal"
	sigPressure        = "pressure"

	instHostHeadroom  = "host-headroom"
	instUsageFraction = "usage-fraction"
	instLimitHeadroom = "limit-headroom"
	instThrottleRatio = "throttle-ratio"
	instStealP95      = "steal-p95"
	instStealMean     = "steal-mean"
	instPressureAvg60 = "pressure-avg60"

	trackHostBusy   = "host-busy"
	trackUsageCores = "usage-cores"
)

// The tiers. Starvation outranks saturation regardless of severity, because
// "you are being throttled" is actionable and "you are busy" is not. The words
// are CPU's; diagnosis only ever sees the number.
const (
	tierStarvation = 0
	tierSaturation = 1
)

// The two shared mark pairs. Both are ratios on a 0..1 scale with a capacity
// of 1.0; steal's pair is shared by the p95 arm and the mean fallback, which is
// what lets the mean stand in for a percentile at two samples without a second
// threshold.
var (
	stealMarks = diagnosis.Marks{
		Fire: diagnosis.Mark{At: 0.10}, Clear: diagnosis.Mark{At: 0.06},
		Polarity: diagnosis.HigherIsWorse, Unit: "ratio", Worst: 1.0,
	}
	throttleMarks = diagnosis.Marks{
		Fire: diagnosis.Mark{At: 0.05}, Clear: diagnosis.Mark{At: 0.03},
		Polarity: diagnosis.HigherIsWorse, Unit: "ratio", Worst: 1.0,
	}
)

// NewEngine builds the engine once, at construction. Both arguments are startup
// facts, cached across ticks, exactly as a Capability is; a quota that changes
// at runtime needs a rebuilt table, and rebuilding drops every window.
func NewEngine(cores, quota float64) (*diagnosis.Engine[Sample], error) {
	return diagnosis.NewEngine(Table(cores, quota))
}

// Table builds the CPU signal table for a caller that needs the Signal values
// themselves: a worker outside this package walks them to ask Engine.Select for
// per-signal Availability, and cpuTable is unexported.
//
// What this guarantees is narrow and exact. NewEngine delegates through Table,
// so the table a caller walks and the table that caller's engine was built from
// come from the same call — a signal the worker polls is a signal the engine
// keyed windows under. It is not a package-wide guarantee: RunSuite still builds
// its own table from cpuTable directly, so "nothing in cpuhealth can drift" is
// not a claim this function is in a position to make.
func Table(cores, quota float64) diagnosis.Table[Sample] {
	return cpuTable(cores, quota)
}

// cpuTable is the CPU declaration, built by a function because two marks and
// one capacity are denominated in quantities that vary per box: the quota and
// the logical CPU count. Both arguments are startup facts, cached across ticks,
// exactly as a Capability is. host-headroom's capacity is the fixed reserve
// rather than the core count, so only the limit arm's capacity scales.
//
// 🔥 quota is a float64 and not a Reading. Marks.Worst and Mark.At are both
// float64, so a table cannot be built from an absence — and it does not need to
// be: HasLimit is present exactly when cpu.max names a positive quota, which is
// the same read that supplies the number.
//
// When there is no positive quota, cpuTable omits the limit-saturation signal
// entirely. It is not enough to leave the row unreachable through Requires: at
// quota = 0 the pair is Fire{At: 0} against Clear{At: 0.05 × 0}, which
// NewEngine refuses under LowerIsWorse — so a box with no limit could not build
// a table at all. Omitting the row is the only arrangement that constructs.
// throttling stays, Requires: HasLimit and all, because its marks are ratios
// and do not scale with the quota.
//
// The same conditional omission applies to the core count: a box whose core
// count was never readable (cores <= 0) declares no saturation row at all —
// there is no capacity to be full, only a count that was never taken.
func cpuTable(cores, quota float64) diagnosis.Table[Sample] {
	t := diagnosis.Table[Sample]{
		Interval: time.Second,
		// The two folds no instrument produces. Both are declared on every box,
		// with no Requires and no quota in sight, which is the whole point:
		// attribution needs both 60s means everywhere, and the instruments that
		// touch these series hold something else.
		//
		// host-busy: host-headroom's window holds cores − hostBusy − reserve AND
		// is Unknown() off ScopeHost, so inverting it loses the term on exactly
		// the affinity boxes whose host/container split is still valid.
		//
		// usage-cores: limit-headroom's window holds quota − usage − 0.10 × quota
		// and does not exist at all when cpuTable omits limit-saturation, which
		// is every box with no positive quota.
		Tracks: []diagnosis.Track[Sample]{
			{
				Name:    trackHostBusy,
				Extract: func(s Sample) diagnosis.Reading { return s.HostBusy },
				Span:    60 * time.Second,
				Red:     diagnosis.Mean, // minimum 2 — the parked 2-sample ring floor
			},
			{
				Name:    trackUsageCores,
				Extract: func(s Sample) diagnosis.Reading { return s.UsageCores },
				Span:    60 * time.Second,
				Red:     diagnosis.Mean,
			},
		},
		Signals: []diagnosis.Signal[Sample]{
			throttlingSignal(),
			pressureSignal(),
			stealSignal(),
		},
	}
	if cores > 0 {
		t.Signals = append(t.Signals, saturationSignal(cores))
	}
	if quota > 0 {
		t.Signals = append(t.Signals, limitSaturationSignal(quota))
	}
	return t
}

// throttlingSignal is "is the kernel capping us against our own quota?" It is
// the one instrument reading running totals, hence the only Counter in the
// table.
func throttlingSignal() diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            sigThrottling,
		Tier:            tierStarvation,
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Name:     instThrottleRatio,
			Requires: []diagnosis.Capability{HasLimit},
			Extract:  func(s Sample) diagnosis.Reading { return s.NrThrottled },
			Against:  func(s Sample) diagnosis.Reading { return s.NrPeriods },
			Span:     60 * time.Second,
			Red:      diagnosis.DeltaRatio,
			// Both cpu.stat counters are running totals: a fall is a cgroup
			// reset, and a delta across it is arithmetic on two origins. This
			// is the only CPU instrument that declares it.
			Counter: true,
			Marks:   throttleMarks,
		}},
	}
}

// pressureSignal is "are our tasks waiting for a core?" PSI's avg60 is already
// a 60-second average, so the reduction is Last and the instrument can fire on
// tick 0.
func pressureSignal() diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            sigPressure,
		Tier:            tierStarvation,
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Name:     instPressureAvg60,
			Requires: []diagnosis.Capability{HasPressureStats},
			Extract:  func(s Sample) diagnosis.Reading { return s.Pressure },
			Span:     60 * time.Second,
			Red:      diagnosis.Last,
			Marks: diagnosis.Marks{
				Fire:     diagnosis.Mark{At: 0.20},
				Clear:    diagnosis.Mark{At: 0.12},
				Polarity: diagnosis.HigherIsWorse,
				Unit:     "ratio",
				Worst:    1.0,
			},
		}},
	}
}

// stealSignal is "is something outside this box taking our CPU?" It carries two
// instruments that answer the same question — a percentile once the ring holds
// twenty entries, and a mean before that — so steal is judgeable two seconds
// after a start instead of twenty.
func stealSignal() diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            sigSteal,
		Tier:            tierStarvation,
		External:        true, // Rank's third tie-break; not what sets attribution
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{
			{
				Name:     instStealP95,
				Requires: []diagnosis.Capability{HasVirtualization},
				Extract:  func(s Sample) diagnosis.Reading { return s.Steal },
				Span:     60 * time.Second,
				Red:      diagnosis.P95, // the reduction declares its own minimum: 20
				Marks:    stealMarks,
			},
			{
				Name:     instStealMean,
				Requires: []diagnosis.Capability{HasVirtualization},
				Extract:  func(s Sample) diagnosis.Reading { return s.Steal },
				Span:     60 * time.Second,
				Red:      diagnosis.Mean, // minimum 2
				Marks:    stealMarks,     // the mean fallback shares the p95 bar:
				// the question and its unit are the same, only the minimum
				// differs. Do not "fix" this arm back to p95, and do not add a
				// second threshold: one 0.9 spike firing the mean at n=2 is the
				// accepted design, not the defect.
				//
				// Counter stays false, on both steal arms. A steal fraction that
				// falls has fallen. Declare it a counter and the window restarts
				// on the first dip, never reaches p95's twenty samples, and the
				// handover, the mean fallback and the two-second readiness all die silently
				// green — steal simply never fires.
			},
		},
	}
}

// saturationSignal is "is the machine full?" It holds the question twice:
// host-headroom answers it from /proc/stat, usage-fraction from our own usage
// when /proc/stat is unreadable. Both sit under one signal so the latch
// survives the swap, and host-headroom is listed first so selection prefers it
// whenever its window can supply a value.
func saturationSignal(cores float64) diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            sigSaturation,
		Tier:            tierSaturation,
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{
			{
				Name: instHostHeadroom,
				// cores − hostBusy − 1.0. This arm exists only on a box whose
				// core count was readable, so cores > 0 here; the scope guard stays
				// because off a host-scoped sample the count means something else
				// and there is no headroom to read.
				Extract: func(s Sample) diagnosis.Reading {
					// Defense-in-depth, not the gate. The real gate is the
					// omission: cpuTable appends no saturation signal when cores
					// <= 0, so this Extract is unreachable through production, and
					// the absence is pinned by the RED test's hasSignal assertion.
					// This guard only matters if that append gate is re-removed or
					// saturationSignal is called directly with a non-positive
					// count — the subtraction below must never run on such a
					// count, so the arm withholds here too.
					if cores <= 0 {
						return diagnosis.Unknown()
					}
					if s.CpuScope != ScopeHost {
						return diagnosis.Unknown()
					}
					hb, ok := s.HostBusy.Get()
					if !ok {
						return diagnosis.Unknown()
					}
					return diagnosis.Known(cores - hb - 1.0)
				},
				Span: 60 * time.Second,
				Red:  diagnosis.Mean,
				Marks: diagnosis.Marks{
					Fire:     diagnosis.Mark{At: 0},
					Clear:    diagnosis.Mark{At: 0.5},
					Polarity: diagnosis.LowerIsWorse,
					Unit:     "cores",
					// Severity 1 at −Reserve, so Worst is the reserve, not
					// the core count. Headroom is cores − hostBusy − reserve and
					// hostBusy cannot exceed cores, so the quantity bottoms out
					// at −cpuReserveCores: a wholly consumed box. Worst is
					// negative because it lives on the worse (lower) side of
					// Fire, the same side as the value that reaches it.
					Worst: -cpuReserveCores,
				},
			},
			{
				Name: instUsageFraction,
				Extract: func(s Sample) diagnosis.Reading {
					// Same defense-in-depth for the division below: unreachable
					// through production while the append gate holds, but must
					// never divide by a non-positive count if that gate is
					// re-removed — so the arm withholds here too.
					if cores <= 0 {
						return diagnosis.Unknown()
					}
					u, ok := s.UsageCores.Get()
					if !ok {
						return diagnosis.Unknown()
					}
					return diagnosis.Known(u / cores)
				},
				Span: 60 * time.Second,
				Red:  diagnosis.Mean,
				Marks: diagnosis.Marks{
					// 0.70 fires AT the mark: exactly 70% of the machine busy is
					// a full machine, not a 69%-and-waiting one.
					Fire:     diagnosis.Mark{At: 0.70, Inclusive: true},
					Clear:    diagnosis.Mark{At: 0.60},
					Polarity: diagnosis.HigherIsWorse,
					Unit:     "fraction",
					Worst:    1.0,
				},
			},
		},
	}
}

// limitSaturationSignal is "are we out of our own budget?" It is the row whose
// marks are denominated in the quota, and it is the reason quota is a float64:
// it is the only place in the design where a Reading would have had to reach
// Marks.Worst, which is a float64, so it cannot. cpuTable omits it entirely
// when quota is not positive, because Fire{At: 0} against Clear{At: 0.05 × 0}
// is a pair NewEngine refuses.
func limitSaturationSignal(quota float64) diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            sigLimitSaturation,
		Tier:            tierSaturation,
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Name:     instLimitHeadroom,
			Requires: []diagnosis.Capability{HasLimit},
			// quota − usage − 0.10 × quota, in cores. The usage term is the
			// SAMPLER's rate, never the cumulative counter beside it:
			// nothing can subtract a counter from a quota.
			Extract: func(s Sample) diagnosis.Reading {
				u, ok := s.UsageCores.Get()
				if !ok {
					return diagnosis.Unknown()
				}
				return diagnosis.Known(quota - u - 0.10*quota)
			},
			Span: 60 * time.Second,
			Red:  diagnosis.Mean,
			Marks: diagnosis.Marks{
				Fire:     diagnosis.Mark{At: 0},
				Clear:    diagnosis.Mark{At: 0.05 * quota},
				Polarity: diagnosis.LowerIsWorse,
				Unit:     "cores",
				// Same reasoning as host-headroom: usage cannot exceed the quota
				// the kernel throttles it to, so quota − usage − 0.10 × quota
				// bottoms out at −0.10 × quota. Worst is negative because it
				// lives on the worse (lower) side of Fire, the same side as the
				// value that reaches it; a container wholly out of its budget
				// scores 1.0.
				Worst: -0.10 * quota,
			},
		}},
	}
}

// saturationArmOf identifies which arm of the saturation family a fired signal
// came from, so the fold and the latch flags can agree without Decide knowing
// the arms. The signal name alone cannot name the arm: sigSaturation carries
// two instruments, and Marks.Unit is the only thing that tells them apart —
// "cores" is host-headroom, "fraction" is usage-fraction. Marks reads the
// FROZEN mark, so it names the instrument that actually fired; the live
// per-tick winner would disagree with Marks from tick 3 onward, which is why
// this frozen word is the arm's source of truth and not the tick's selected
// instrument.
func saturationArmOf(f diagnosis.Fired) saturationArm {
	switch f.Identity.Signal {
	case sigLimitSaturation:
		return limitArm
	case sigSaturation:
		if f.Marks.Unit == "fraction" {
			return noHostStatsArm
		}
		return hostFullArm
	default:
		return noSaturationArm
	}
}

// saturationRank orders the saturation family for the fold: host-full outranks
// the limit arm, the limit arm outranks the no-host-stats fallback. The arm IS
// the rank — the constants in verdict.go are declared in that order — so the
// fold compares one int and knows nothing about the arms. Ties cannot occur
// (the latch is per signal, and this function is only called on a fired
// saturation-family signal); if one somehow did, the fold's strictly-greater
// compare keeps the first member.
func saturationRank(f diagnosis.Fired) int {
	return int(saturationArmOf(f))
}

// saturationFlags raises the Signals latch bits one fired saturation arm owns.
// HostFullFired and NoLimitHostFired are ONE instrument (the host-headroom arm)
// under two names, and the mode is what separates them: limit mode reports the
// host-full fallback as HostFullFired, no-limit mode as NoLimitHostFired. The
// limit arm and the no-host-stats fallback each own one bit.
func saturationFlags(f diagnosis.Fired, sig *Signals, hasLimit bool) {
	switch saturationArmOf(f) {
	case hostFullArm:
		if hasLimit {
			sig.HostFullFired = true
		} else {
			sig.NoLimitHostFired = true
		}
	case limitArm:
		sig.LimitSaturationFired = true
	case noHostStatsArm:
		sig.NoHostStatsSaturationFired = true
	}
}
