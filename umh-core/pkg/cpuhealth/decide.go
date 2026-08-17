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

// Decide, the judgement entry point. It is ONE function —
// Observe, then the saturation fold, then diagnosis.Rank, then the Signals fill,
// all in a single pass over one fired set. A verdict field is not asserted
// without the evidence for it: attribution consults the host/container split
// read back from the engine's two tracks, and an internal cause (throttling,
// pressure, the container's own limit budget) attributes unknown, never host.

package cpuhealth

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// Decide is the frozen entry point: zero I/O, no clock. Observe, then the
// saturation fold, then diagnosis.Rank, then the Signals fill, all in a single
// pass over one fired set.
//
// The saturation family folds into exactly one CauseKindSaturation before
// ranking, in the decided precedence — host-full, then the limit arm, then
// the no-host-stats fallback — so the two entries that would print the same
// paragraph twice reach the customer as one cause. Rank runs AFTER the fold and
// is the only thing that orders Verdict.Causes.
//
// Attribution is derived from the dominant (ranked-first) cause. External
// causes (steal, host-contention) attribute host; internal causes (throttling,
// pressure, the container's own limit budget) attribute unknown; host-full
// saturation attributes by the host/container split, which is host only when
// the host's non-container share exceeds our own sustained usage.
func Decide(engine *diagnosis.Engine[Sample], s Sample, env diagnosis.Environment) (Verdict, Signals) {
	fired, readiness := engine.Observe(s, env, s.Timestamp)

	var sig Signals

	// The attribution split. Both terms are 60s means in cores, read back from
	// the engine's tracks — never from an instrument, because neither instrument
	// that touches the series holds the series. The comparison is STRICTLY
	// greater (hbm - uc > uc), matching the parked branch, so exact equality is
	// unknown. Do not run the split on an untrusted mean: one sample of host
	// busy against one of ours is an attribution on a single instant.
	hostBusyMean, hostBusyState := engine.Track(trackHostBusy).Get()
	ourUsageMean, ourUsageState := engine.Track(trackUsageCores).Get()
	splitHost := hostBusyState == diagnosis.StateValue && ourUsageState == diagnosis.StateValue && hostBusyMean > 2*ourUsageMean

	// Fold the saturation family into one cause and collect the rest. The fired
	// set arrives unranked and in table order; the fold's fixed precedence is
	// the only rule Decide applies before Rank. Decide knows nothing about the
	// saturation arms — the rank and the latch flags are the table's — so the
	// fold keeps the highest-ranked member of the family and records the flags.
	rest := make([]diagnosis.Fired, 0, len(fired))
	var survivor *diagnosis.Fired
	best := 0 // below every saturation-arm rank, so the first member wins a tie
	for i := range fired {
		f := &fired[i]
		switch f.Identity.Signal {
		case sigSaturation, sigLimitSaturation:
			sig.SaturationFired = true
			saturationFlags(*f, &sig, env.Has(HasLimit))
			// The fold keeps the highest-ranked member of the saturation family.
			// Ties cannot occur (the latch is per signal); if one somehow did,
			// the strictly-greater compare keeps the first member.
			if rank := saturationRank(*f); rank > best {
				best = rank
				survivor = f
			}
		default:
			rest = append(rest, *f)
			switch f.Identity.Signal {
			case sigThrottling:
				sig.ThrottleFired = true
			case sigPressure:
				sig.PressureFired = true
			case sigSteal:
				sig.StealFired = true
			}
		}
	}
	if survivor != nil {
		rest = append(rest, *survivor)
	}
	// HostContentionFired is reserved; Decide sets it false unconditionally.
	sig.HostContentionFired = false

	// Rank the folded set; the result IS the order of Verdict.Causes, and there
	// is no local sort anywhere in this package. Then build the verdict
	// and the attribution from the dominant cause.
	ranked := diagnosis.Rank(rest)
	causes := make([]Cause, len(ranked))
	for i, f := range ranked {
		causes[i] = causeOf(engine, f)
	}
	verdict := Verdict{Causes: causes}
	if len(causes) == 0 {
		verdict.State = StateHealthy
	} else {
		verdict.State = StateDegraded
		verdict.Attribution = attributeFor(causes[0], survivor, splitHost)
	}

	// The withheld-headroom facts ride Signals, not Verdict — three
	// fields, none of the parked 31 can stand in for them. HostHeadroomAvailable
	// is dispatched on the sample's scope, NOT on the window's state: an
	// affinity box or an unestablished scope is a withholding ("we read it and
	// it means something else"), while a plain /proc/stat read failure leaves
	// the bit set and the window absent, so a read failure is not rendered as a
	// withholding. The two counts ride the snapshot so the "pinned to 2 of 8
	// CPUs" sentence can name them without Decide doing any I/O.
	sig.HostHeadroomAvailable = s.CpuScope == ScopeHost
	if lc, ok := s.LogicalCpus.Get(); ok {
		sig.LogicalCpus = lc
	}
	if hc, ok := s.HostCpus.Get(); ok {
		sig.HostCpus = hc
	}

	// The average usage fraction is usage-fraction's
	// own reduction — the number the latch was judged on, not the usage track
	// divided by anything, so a mid-run core-count change cannot split them. It
	// is filled on every tick, not only when the no-host-stats arm fires.
	sig.AvgUsageFraction, _ = engine.Reduction(sigSaturation, instUsageFraction).Get()

	// The dead-zone annotation. The dead zone is quota nil or non-positive AND
	// PSI absent, and it is an annotation on a healthy verdict, never a state.
	// LimitedVisibility is where ComposeMessage reads it.
	if q, ok := s.Quota.Get(); !ok || q <= 0 {
		sig.LimitedVisibility = !s.PsiAvailable
	}

	// The observable metrics, the two track floors and each
	// signal's readiness are filled from the same pass, independent of latch
	// state, so a signal sitting below its mark still reaches Signals. This is
	// the route a no-latch tick's numbers take: Observe returns fired latches
	// only, so without these reads a confident 0 would be published on every
	// healthy tick.
	sig.ThrottleRatio, _ = engine.Reduction(sigThrottling, instThrottleRatio).Get()
	sig.PressureAvg60, _ = engine.Reduction(sigPressure, instPressureAvg60).Get()
	sig.StealP95, _ = engine.Reduction(sigSteal, instStealP95).Get()
	sig.HostHeadroomCores, _ = engine.Reduction(sigSaturation, instHostHeadroom).Get()
	sig.AvgUsageCores = ourUsageMean
	sig.AvgHostBusyCores = hostBusyMean
	sig.UsageRingActive = ourUsageState == diagnosis.StateValue
	sig.HostBusyRingActive = hostBusyState == diagnosis.StateValue

	// The headroom ceiling and the reserve the verdict subtracted, stamped so
	// the message's headline and headroom line read exactly the number the
	// verdict used: capacity is the quota when set and positive, else
	// the logical CPU count; the reserve is 10% of the quota in limit mode and
	// cpuReserveCores in no-limit mode.
	if q, ok := s.Quota.Get(); ok && q > 0 {
		sig.CapacityCores = q
		sig.ReserveCores = 0.10 * q
	} else {
		sig.CapacityCores = sig.LogicalCpus
		sig.ReserveCores = cpuReserveCores
	}
	// HostBusyCoresAvailable mirrors the sample's own readability: a raw
	// AvgHostBusyCores == 0 proxy cannot tell an unreadable /proc/stat from
	// an idle host.
	if _, ok := s.HostBusy.Get(); ok {
		sig.HostBusyCoresAvailable = true
	}

	// The per-signal readiness trio, out of the same pass that judged them.
	// Ready and nothing else: NoInstrument on a bare-metal box and NoneReady on
	// a thin window both mean this tick has no usable reading, and printing a
	// confident number for either states a figure that was never measured.
	for _, r := range readiness {
		usable := r.Availability == diagnosis.Ready
		switch r.Signal {
		case sigThrottling:
			sig.ThrottleSignalReady = usable
		case sigPressure:
			sig.PressureSignalReady = usable
		case sigSteal:
			sig.StealSignalReady = usable
		}
	}

	// Capability, not readability: the healthy message's budget lines must not
	// read these as "the reading succeeded". The *SignalReady trio is the
	// readability half.
	sig.LimitApplies = env.Has(HasLimit)
	sig.PressureApplies = s.PsiAvailable
	sig.StealApplies = env.Has(HasVirtualization)

	return verdict, sig
}
