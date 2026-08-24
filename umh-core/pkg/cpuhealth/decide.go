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

// Decide, the judgement entry point. Observe runs first and buildVerdict ranks
// the set it returned, so no verdict field is asserted without the evidence for
// it. detailsFor reads this tick's numbers back out of the same engine. The
// attribution is read off the fired signal that ranked first, where the table
// declared it.

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
	details := detailsFor(engine, s, env, readiness)
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
func detailsFor(engine *diagnosis.Engine[Sample], s Sample, env diagnosis.Environment, readiness []diagnosis.Readiness) Details {
	var d Details

	// The withheld-headroom facts belong on Details, not Verdict — no other
	// field on the struct can stand in for them. HostHeadroomAvailable is
	// dispatched on the sample's scope, NOT on the window's state: an affinity
	// box or an unestablished scope is a withholding ("we read it and it means
	// something else"), while a plain /proc/stat read failure leaves the bit set
	// and the window absent, so a read failure is not rendered as a withholding.
	// The two counts are on Details so the "pinned to 2 of 8 CPUs" sentence can
	// name them without Decide doing any I/O.
	d.HostHeadroomAvailable = s.CpuScope == ScopeHost
	if lc, ok := s.LogicalCpus.Get(); ok {
		d.LogicalCpus = lc
	}
	if hc, ok := s.HostCpus.Get(); ok {
		d.HostCpus = hc
	}

	// The average usage fraction is usage-fraction's own reduction — the number
	// the latch was judged on, not the usage-cores measurement divided by
	// anything, so a mid-run core-count change cannot split them. It is filled
	// on every tick, not only when usage-fraction is the instrument that
	// fired.
	d.AvgUsageFraction, _ = engine.Reduction(signalHostCpuFull, instrumentUsageFraction).Get()

	// Limited visibility is quota nil or non-positive AND PSI absent. It is an
	// annotation on a healthy verdict, never a state, and LimitedVisibility is
	// where ComposeMessage reads it.
	if q, ok := s.Quota.Get(); !ok || q <= 0 {
		d.LimitedVisibility = !s.PsiAvailable
	}

	// The instrument readings come out of the same Observe pass that judged
	// them, and they are read whatever the latch did, so a signal sitting below
	// its mark still reaches Details. Observe returns fired latches only, so
	// without these reads a confident 0 would be published on every healthy
	// tick.
	d.ThrottleRatio, _ = engine.Reduction(signalThrottling, instrumentThrottleRatio).Get()
	d.PressureAvg60, _ = engine.Reduction(signalPressure, instrumentPressureAvg60).Get()
	d.StealP95, _ = engine.Reduction(signalSteal, instrumentStealP95).Get()
	d.HostHeadroomCores, _ = engine.Reduction(signalHostCpuFull, instrumentHostHeadroom).Get()

	// The two measurements, each a plain 60-second average declared in
	// table_cpu.go. The state says whether the window reduced to a value, and
	// the healthy headline gates on it so a thin window is not reported as a
	// confident 0.
	hostBusyMean, hostBusyState := engine.Measurement(measurementHostBusy).Get()
	usageMean, usageState := engine.Measurement(measurementUsageCores).Get()
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
		case signalThrottling:
			d.ThrottleSignalReady = usable
		case signalPressure:
			d.PressureSignalReady = usable
		case signalSteal:
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
