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

// The saturation family answers whether the machine — or our own limit — is
// full. It covers the host-full, usage-fraction, and limit arms of one
// question, plus the helpers chooseSaturationCause uses to order and flag
// them; the arms stay together because saturationRank and saturationFlags
// describe both signals.

package cpuhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// saturationSignal asks "is the machine full?".
//
// A signal is one question. An instrument is one way to measure the answer,
// and a signal can hold more than one: whichever instrument has a usable
// reading answers the question, so losing one source does not lose the
// question. This signal has two instruments. host-headroom measures from
// /proc/stat. usage-fraction measures from our own usage, for the case where
// /proc/stat cannot be read. host-headroom is listed first, so it answers
// whenever its window has a value.
//
// Both instruments sit under one signal so they share one latch: the machine
// has not stopped being full because the measurement changed hands.
func saturationSignal(cores float64) diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            sigSaturation,
		Tier:            tierSaturation,
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{
			{
				Measurement: diagnosis.Measurement[Sample]{
					Name: instHostHeadroom,
					// cores − hostBusy − 1.0. This arm exists only on a box whose
					// core count was readable, so cores > 0 here; the scope guard stays
					// because off a host-scoped sample the count means something else
					// and there is no headroom to read.
					Extract: func(s Sample) diagnosis.Reading {
						// Unreachable in production: cpuTable declares no saturation signal when
						// cores <= 0, pinned by host_headroom_guard_test.go. The guard stays so
						// the subtraction below can never run on a non-positive count.
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
					Span:      60 * time.Second,
					Reduction: diagnosis.Mean,
				},
				// Marks are the two thresholds that turn a number into a yes or no: the
				// value at which this instrument starts saying yes, and the value at which
				// it goes back to saying no. They differ on purpose, so a reading sitting
				// on the boundary does not flap.
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
				Measurement: diagnosis.Measurement[Sample]{
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
					Span:      60 * time.Second,
					Reduction: diagnosis.Mean,
				},
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
// is a pair NewEngine rejects.
func limitSaturationSignal(quota float64) diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            sigLimitSaturation,
		Tier:            tierSaturation,
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Measurement: diagnosis.Measurement[Sample]{
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
				Span:      60 * time.Second,
				Reduction: diagnosis.Mean,
			},
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
// came from, so chooseSaturationCause and the latch flags can agree without
// Decide knowing the arms. The signal name alone cannot name the arm:
// sigSaturation carries two instruments, and Marks.Unit is the only thing that
// tells them apart — "cores" is host-headroom, "fraction" is usage-fraction.
// Marks reads the FROZEN mark, so it names the instrument that actually
// fired; the live per-tick winner would disagree with Marks from tick 3
// onward, which is why this frozen word is the arm's source of truth and not
// the tick's selected instrument.
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

// saturationRank orders the saturation family for chooseSaturationCause:
// host-full outranks the limit arm, the limit arm outranks the no-host-stats
// fallback. The arm IS the rank — the constants in verdict.go are declared in
// that order — so chooseSaturationCause compares one int and needs nothing
// about the arms.
func saturationRank(f diagnosis.Fired) int {
	return int(saturationArmOf(f))
}

// saturationFlags raises the Details latch bits one fired saturation arm owns.
// HostFullFired and NoLimitHostFired are ONE instrument (the host-headroom arm)
// under two names, and the mode is what separates them: limit mode reports the
// host-full fallback as HostFullFired, no-limit mode as NoLimitHostFired. The
// limit arm and the no-host-stats fallback each own one bit.
func saturationFlags(f diagnosis.Fired, sig *Details, hasLimit bool) {
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
