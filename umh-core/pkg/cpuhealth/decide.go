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
// output — Observe, then buildVerdict and detailsFor off the same fired set —
// so no verdict field is asserted without the evidence for it. The attribution
// is read off the fired signal that ranked first, where the table declared it.

package cpuhealth

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// Decide runs one tick and returns the tick's Verdict and Details. Each stage
// takes the previous stage's output: only Observe touches state, because the
// engine advances its windows as it reads them.
func Decide(engine *diagnosis.Engine[Sample], s Sample, env diagnosis.Environment) (Verdict, Details) {
	fired, readiness := engine.Observe(s, env, s.Timestamp)
	verdict := buildVerdict(engine, fired)
	details := detailsFor(engine, s, env, readiness, fired)
	return verdict, details
}

// buildVerdict ranks the fired set and builds the Verdict. Nothing runs before
// the ranking: every fired signal reaches it, and Rank is the only thing that
// orders Verdict.Causes.
//
// Exactly one rule decides State: degraded when at least one signal fired this
// tick, healthy when none did — no severity floor, no second condition. It owns
// no Details fields — it returns the Verdict only. The attribution is the blame
// the table declared for the signal Rank put first, or for the refinement
// narrowing it. The Fired that ranked first is kept for that read; the Cause
// built from it carries no blame.
func buildVerdict(engine *diagnosis.Engine[Sample], fired []diagnosis.Fired) Verdict {
	ranked := diagnosis.Rank(fired)
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

// detailsFor fills every Details field. buildVerdict returns the Verdict alone,
// so this is the sole producer of the struct.
func detailsFor(engine *diagnosis.Engine[Sample], s Sample, env diagnosis.Environment, readiness []diagnosis.Readiness, fired []diagnosis.Fired) Details {
	var d Details

	// The three starvation latches, for a reader of Details that wants to know
	// a signal fired without walking the cause list. The two capacity signals
	// have no counterpart here: their causes carry the kind and the instrument,
	// which is everything a flag could have said.
	for _, f := range fired {
		switch f.Identity.Signal {
		case sigThrottling:
			d.ThrottleFired = true
		case sigPressure:
			d.PressureFired = true
		case sigSteal:
			d.StealFired = true
		}
	}

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
	// is filled on every tick, not only when usage-fraction is the instrument
	// that fired.
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

	// The two measurements, each a plain 60-second average declared in
	// table_cpu.go. The state says whether the window reduced to a value, and
	// the healthy headline gates on it so a thin window is not reported as a
	// confident 0.
	hostBusyMean, hostBusyState := engine.Measurement(measHostBusy).Get()
	usageMean, usageState := engine.Measurement(measUsageCores).Get()
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
