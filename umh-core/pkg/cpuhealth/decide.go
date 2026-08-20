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

// Decide, the judgement entry point. Each stage takes the previous stage's
// output — Observe, then chooseSaturationCause, then buildVerdict and
// detailsFor off the same choice — so no verdict field is asserted without
// the evidence for it. The attribution is read off the fired signal that
// ranked first, where the table declared it.

package cpuhealth

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// Decide runs one tick and returns the tick's Verdict and Details. Each stage
// takes the previous stage's output: only Observe touches state, because the
// engine advances its windows as it reads them.
func Decide(engine *diagnosis.Engine[Sample], s Sample, env diagnosis.Environment) (Verdict, Details) {
	fired, readiness := engine.Observe(s, env, s.Timestamp)
	chosen := chooseSaturationCause(fired, env.Has(HasLimit))
	verdict := buildVerdict(engine, chosen)
	details := detailsFor(engine, s, env, readiness, chosen)
	return verdict, details
}

// saturationChoice is the result of picking one "the CPU is full" cause out of
// several. A tick can produce several at once: the host has no headroom, this
// container has spent its own quota, and the fallback used when the host is
// unreadable. They describe one situation, so Decide keeps only the
// highest-ranked one.
type saturationChoice struct {
	// Fired is every signal that fired this tick, with the surplus "CPU is
	// full" causes removed — this is what Rank orders.
	Fired []diagnosis.Fired
	// Winner is the "CPU is full" cause that was kept, nil if none fired.
	// Blame depends on which one it was.
	Winner *diagnosis.Fired
	// Latched holds the Details fields set here and nowhere else: the four
	// family latches, and the four saturation arms saturationFlags sets.
	Latched Details
}

// chooseSaturationCause picks the one "CPU is full" cause to report and
// collects every other fired signal. The fired set arrives unranked and in
// table order; this fixed precedence is the only rule Decide applies before
// Rank, and the rank and latch flags are the table's.
func chooseSaturationCause(fired []diagnosis.Fired, hasLimit bool) saturationChoice {
	rest := make([]diagnosis.Fired, 0, len(fired))
	var latched Details
	var winner *diagnosis.Fired
	best := 0 // below every saturation-arm rank, so the first member wins a tie
	for i := range fired {
		f := &fired[i]
		switch f.Identity.Signal {
		case sigHostCpuFull, sigContainerLimitFull:
			latched.SaturationFired = true
			saturationFlags(*f, &latched, hasLimit)
			// Keep the highest-ranked member of the saturation family.
			// Ties cannot occur (the latch is per signal); if one somehow did,
			// the strictly-greater compare keeps the first member.
			if rank := saturationRank(*f); rank > best {
				best = rank
				winner = f
			}
		default:
			rest = append(rest, *f)
			switch f.Identity.Signal {
			case sigThrottling:
				latched.ThrottleFired = true
			case sigPressure:
				latched.PressureFired = true
			case sigSteal:
				latched.StealFired = true
			}
		}
	}
	if winner != nil {
		rest = append(rest, *winner)
	}
	return saturationChoice{Fired: rest, Winner: winner, Latched: latched}
}

// buildVerdict ranks what chooseSaturationCause left and builds the Verdict.
// Exactly one rule decides State: degraded when at least one signal fired
// this tick, healthy when none did — no severity floor, no second condition.
// It owns no Details fields — it returns the Verdict only. Rank is the only
// thing that orders Verdict.Causes, and the attribution is the blame the table
// declared for the signal Rank put first, or for the refinement narrowing it.
// The Fired that ranked first is kept for that read; the Cause built from it
// carries no blame.
func buildVerdict(engine *diagnosis.Engine[Sample], chosen saturationChoice) Verdict {
	ranked := diagnosis.Rank(chosen.Fired)
	causes := make([]Cause, len(ranked))
	for i, f := range ranked {
		causes[i] = causeOf(engine, f)
	}
	verdict := Verdict{Causes: causes}
	if len(causes) == 0 {
		verdict.State = StateHealthy
	} else {
		verdict.State = StateDegraded
		verdict.Attribution = attributionOf(declaredBlame(ranked[0]))
	}
	return verdict
}

// detailsFor fills every Details field not already set by chooseSaturationCause
// or buildVerdict, starting from the saturation choice's own latched fields.
func detailsFor(engine *diagnosis.Engine[Sample], s Sample, env diagnosis.Environment, readiness []diagnosis.Readiness, chosen saturationChoice) Details {
	d := chosen.Latched

	// The withheld-headroom facts belong on Details, not Verdict — three
	// fields, none of the other 31 can stand in for them. HostHeadroomAvailable
	// is dispatched on the sample's scope, NOT on the window's state: an
	// affinity box or an unestablished scope is a withholding ("we read it and
	// it means something else"), while a plain /proc/stat read failure leaves
	// the bit set and the window absent, so a read failure is not rendered as a
	// withholding. The two counts are on Details so the "pinned to 2 of 8
	// CPUs" sentence can name them without Decide doing any I/O.
	d.HostHeadroomAvailable = s.CpuScope == ScopeHost
	if lc, ok := s.LogicalCpus.Get(); ok {
		d.LogicalCpus = lc
	}
	if hc, ok := s.HostCpus.Get(); ok {
		d.HostCpus = hc
	}

	// The average usage fraction is usage-fraction's
	// own reduction — the number the latch was judged on, not the usage track
	// divided by anything, so a mid-run core-count change cannot split them. It
	// is filled on every tick, not only when the no-host-stats arm fires.
	d.AvgUsageFraction, _ = engine.Reduction(sigHostCpuFull, instUsageFraction).Get()

	// The dead-zone annotation. The dead zone is quota nil or non-positive AND
	// PSI absent, and it is an annotation on a healthy verdict, never a state.
	// LimitedVisibility is where ComposeMessage reads it.
	if q, ok := s.Quota.Get(); !ok || q <= 0 {
		d.LimitedVisibility = !s.PsiAvailable
	}

	// The observable metrics, the two track floors and each
	// signal's readiness are filled from the same pass, independent of latch
	// state, so a signal sitting below its mark still reaches Details. This is
	// the route a no-latch tick's numbers take: Observe returns fired latches
	// only, so without these reads a confident 0 would be published on every
	// healthy tick.
	d.ThrottleRatio, _ = engine.Reduction(sigThrottling, instThrottleRatio).Get()
	d.PressureAvg60, _ = engine.Reduction(sigPressure, instPressureAvg60).Get()
	d.StealP95, _ = engine.Reduction(sigSteal, instStealP95).Get()
	d.HostHeadroomCores, _ = engine.Reduction(sigHostCpuFull, instHostHeadroom).Get()

	// The two measurement tracks, each a plain 60-second average declared in
	// table_cpu.go. The state says whether the window reduced to a value, and
	// the healthy headline gates on it so a thin window is not reported as a
	// confident 0.
	hostBusyMean, hostBusyState := engine.Measurement(trackHostBusy).Get()
	usageMean, usageState := engine.Measurement(trackUsageCores).Get()
	d.AvgUsageCores = usageMean
	d.AvgHostBusyCores = hostBusyMean
	d.UsageRingActive = usageState == diagnosis.StateValue
	d.HostBusyRingActive = hostBusyState == diagnosis.StateValue

	// The headroom ceiling and reserve mirror exactly what the verdict used, so
	// the message's headline and headroom line report the same number.
	if q, ok := s.Quota.Get(); ok && q > 0 {
		d.CapacityCores = q
		d.ReserveCores = limitReserveFraction * q
	} else {
		d.CapacityCores = d.LogicalCpus
		d.ReserveCores = cpuReserveCores
	}
	// HostBusyCoresAvailable mirrors the sample's own readability: a raw
	// AvgHostBusyCores == 0 proxy cannot tell an unreadable /proc/stat from
	// an idle host.
	if _, ok := s.HostBusy.Get(); ok {
		d.HostBusyCoresAvailable = true
	}

	// The per-signal readiness trio, out of the same pass that judged them.
	// Ready and nothing else: NoInstrument on a bare-metal box and NoneReady on
	// a thin window both mean this tick has no usable reading, and printing a
	// confident number for either states a figure that was never measured.
	for _, r := range readiness {
		usable := r.Availability == diagnosis.Ready
		switch r.Signal {
		case sigThrottling:
			d.ThrottleSignalReady = usable
		case sigPressure:
			d.PressureSignalReady = usable
		case sigSteal:
			d.StealSignalReady = usable
		}
	}

	// Capability, not readability: the healthy message's budget lines must not
	// read these as "the reading succeeded". The *SignalReady trio is the
	// readability half.
	d.LimitApplies = env.Has(HasLimit)
	d.PressureApplies = s.PsiAvailable
	d.StealApplies = env.Has(HasVirtualization)

	return d
}
