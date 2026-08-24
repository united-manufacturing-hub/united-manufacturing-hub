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

// The two capacity signals: is the machine full, and are we out of our own CPU
// limit. Both are declared here, with the two instruments that measure the
// machine and the two refinements that narrow a full machine to a side.
//
// Both refinements read our usage over the machine's busy time. That number
// needs both of its terms measured over the same CPUs, so a box whose
// /proc/stat is unreadable and a container pinned to a subset of the CPUs both
// come out unknown. A share in the narrow band around one half crosses neither
// fire threshold, so nothing new is narrowed there. A refinement that fired
// earlier holds its answer only while the share stays inside its own clear
// threshold. That clear threshold is nearer the middle still. Where neither has
// ever fired, and where the one that did has released, nothing narrows the full
// machine to a side and the blame is unknown.

package cpuhealth

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// hostCpuFullSignal asks "is the machine full?".
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
//
// A full machine says nothing about whose load filled it, so the signal itself
// blames nobody and the two refinements under it narrow that.
func hostCpuFullSignal(cores float64) diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            signalHostCpuFull,
		Tier:            tierSaturation,
		Attribution:     blameUnknown,
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Refinements:     shareRefinements(),
		Instruments: []diagnosis.Instrument[Sample]{
			{
				Measurement: diagnosis.Measurement[Sample]{
					Name: instrumentHostHeadroom,
					// cores − hostBusy − 1.0. This arm exists only on a box whose
					// core count was readable, so cores > 0 here; the scope guard stays
					// because off a host-scoped sample the count means something else
					// and there is no headroom to read.
					// Why subtracting a machine-wide busy time from this
					// container-scoped count is valid: see host_source.go's header.
					Extract: func(s Sample) diagnosis.Reading {
						// Unreachable in production: cpuTable declares no host-cpu-full signal when
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
					Name: instrumentUsageFraction,
					// Evidence of last resort. Our own usage over the CPUs we may
					// run on reserves 30% of them, where host-headroom reserves one
					// core of the machine, so the two arms disagree about what full
					// means and the gap widens with the core count. HasLimitedVisibility
					// is the box where nothing better exists: no PSI to read the
					// harm off, and no quota to judge our own budget against. Where
					// either does exist, an unreadable /proc/stat leaves this signal
					// with nothing to read at all, and pressure or throttling and
					// container-limit-full carry the box instead.
					Requires: []diagnosis.Capability{HasLimitedVisibility},
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

// containerShare is our own usage over the machine's busy time: the fraction of
// everything running on this box that is us. Both refinements below read this
// one number and differ only in which side of it they blame.
//
// It withholds off ScopeHost for the reason host-headroom does. The machine's
// busy time covers every CPU, while a pinned container's usage covers only the
// CPUs it may run on, so the fraction is small for a reason that is not fault.
func containerShare(s Sample) diagnosis.Reading {
	if s.CpuScope != ScopeHost {
		return diagnosis.Unknown()
	}

	busy, ok := s.HostBusy.Get()
	if !ok || busy <= 0 {
		return diagnosis.Unknown()
	}

	usage, ok := s.UsageCores.Get()
	if !ok {
		return diagnosis.Unknown()
	}

	return diagnosis.Known(usage / busy)
}

// shareRefinements narrow a full machine to a side. host-share says the rest of
// the box accounts for most of the busy time; container-share says we do.
//
// Their bands do not overlap, and both can therefore never be fired at once:
// firing either one is exactly the condition that clears the other. Between
// 0.495 and 0.505 no mark of either is crossed, so whichever fired last holds,
// and a share drifting across the middle does not swap the blame back and
// forth. Outside that pair of clear marks but inside the two fire marks — a
// share of 0.506, say — the fired one releases and the other does not fire, so
// the blame goes back to unknown rather than to the other side.
func shareRefinements() []diagnosis.Signal[Sample] {
	return []diagnosis.Signal[Sample]{
		{
			Name:            refinementHostShare,
			Tier:            tierSaturation,
			Attribution:     blameHost,
			DemoteSpan:      60 * time.Second,
			ReleaseOnAbsent: true,
			Instruments: []diagnosis.Instrument[Sample]{{
				Measurement: diagnosis.Measurement[Sample]{
					Name:      refinementHostShare,
					Extract:   containerShare,
					Span:      60 * time.Second,
					Reduction: diagnosis.Mean,
				},
				Marks: diagnosis.Marks{
					Fire:     diagnosis.Mark{At: 0.49},
					Clear:    diagnosis.Mark{At: 0.505},
					Polarity: diagnosis.LowerIsWorse,
					Unit:     "fraction",
					// Severity 1 where none of the machine's busy time is ours.
					Worst: 0.0,
				},
			}},
		},
		{
			Name:            refinementContainerShare,
			Tier:            tierSaturation,
			Attribution:     blameContainer,
			DemoteSpan:      60 * time.Second,
			ReleaseOnAbsent: true,
			Instruments: []diagnosis.Instrument[Sample]{{
				Measurement: diagnosis.Measurement[Sample]{
					Name:      refinementContainerShare,
					Extract:   containerShare,
					Span:      60 * time.Second,
					Reduction: diagnosis.Mean,
				},
				Marks: diagnosis.Marks{
					Fire:     diagnosis.Mark{At: 0.51},
					Clear:    diagnosis.Mark{At: 0.495},
					Polarity: diagnosis.HigherIsWorse,
					Unit:     "fraction",
					// Severity 1 where all of the machine's busy time is ours.
					Worst: 1.0,
				},
			}},
		},
	}
}

// containerLimitFullSignal is "are we out of our own budget?" It is the row whose
// marks are denominated in the quota, and it is the reason quota is a float64:
// it is the only place in the design where a Reading would have had to reach
// Marks.Worst, which is a float64, so it cannot. cpuTable omits it entirely
// when quota is not positive, because Fire{At: 0} against Clear{At: 0.05 × 0}
// is a pair NewEngine rejects.
func containerLimitFullSignal(quota float64) diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name: signalContainerLimitFull,
		Tier: tierSaturation,
		// Spending OUR OWN budget is inside this container by definition, so
		// this row needs no refinement to place the blame.
		Attribution:     blameContainer,
		DemoteSpan:      60 * time.Second,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Measurement: diagnosis.Measurement[Sample]{
				Name:     instrumentLimitHeadroom,
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
