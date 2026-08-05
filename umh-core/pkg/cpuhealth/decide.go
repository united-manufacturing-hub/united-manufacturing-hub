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

// S3 R4 (D1, D2, D3): Decide, the judgement entry point. It is ONE function —
// Observe, then the saturation fold, then diagnosis.Rank, then the Signals fill,
// all in a single pass over one fired set. A verdict field is not asserted
// without the evidence for it: attribution consults the host/container split
// read back from the engine's two tracks, and an internal cause (throttling,
// pressure, the container's own limit budget) attributes unknown, never host.
package cpuhealth

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// State is the customer-visible health state of the CPU verdict. Two values and
// no third: the dead zone is an annotation on a healthy verdict, never a state.
type State string

const (
	StateHealthy  State = "healthy"
	StateDegraded State = "degraded"
)

// Attribution names the dominant cause class when degraded. Two members:
// unknown is the complement of host, and there is deliberately no internal
// member — adding one is the answer ENG-3323 exists to measure.
type Attribution string

const (
	AttributionUnknown Attribution = "unknown"
	AttributionHost    Attribution = "host"
)

// CauseKind enumerates the reason classes that can degrade CPU health.
type CauseKind string

const (
	CauseKindSaturation     CauseKind = "saturation"
	CauseKindThrottling     CauseKind = "throttling"
	CauseKindPressure       CauseKind = "pressure"
	CauseKindSteal          CauseKind = "steal"
	CauseKindHostContention CauseKind = "host-contention"
)

// Unit is the unit a cause's value is denominated in, copied from the mark pair
// that judged it so the message layer can render "cores" vs "ratio".
type Unit string

// cpuReserveCores is the no-limit headroom reserve: one core set aside for
// Redpanda. It is Redpanda's default maxCores (--smp), not a calibration guess
// (DECISIONS.md D-08); Decide stamps it onto Signals.ReserveCores so the
// message reads the same number the verdict subtracted.
const cpuReserveCores = 1.0

// Cause is one entry in a degraded verdict, ordered by diagnosis.Rank.
type Cause struct {
	Kind  CauseKind
	Value float64
	Unit  Unit
}

// Verdict is what Decide returns: the state, the attribution of the dominant
// cause, and the ranked cause list. The message is NOT a field on it — the
// message layer composes it from Verdict and Signals.
type Verdict struct {
	State       State
	Attribution Attribution
	Causes      []Cause
}

// saturationArm identifies which instrument of the saturation family a folded
// cause came from. The latch is per signal, so at most one of host-full,
// no-host-stats and the limit arm survives the fold; attribution differs by
// arm, so Decide keeps the identity beside the survivor.
type saturationArm int

const (
	noSaturationArm saturationArm = iota
	hostFullArm
	noHostStatsArm
	limitArm
)

// Decide is the frozen entry point: zero I/O, no clock. Observe, then the
// saturation fold, then diagnosis.Rank, then the Signals fill, all in a single
// pass over one fired set.
//
// The saturation family folds into exactly one CauseKindSaturation before
// ranking, in today's fixed precedence — host-full, then the no-host-stats
// fallback, then the limit arm — so the two entries that would print the same
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
	// the only rule Decide applies before Rank.
	rest := make([]diagnosis.Fired, 0, len(fired))
	var survivor *diagnosis.Fired
	for i := range fired {
		f := &fired[i]
		switch f.Identity.Signal {
		case sigSaturation:
			// host-headroom has Unit "cores"; usage-fraction has Unit "fraction".
			arm := hostFullArm
			if f.Marks.Unit == "fraction" {
				arm = noHostStatsArm
			}
			sig.SaturationFired = true
			if arm == hostFullArm {
				// HostFullFired and NoLimitHostFired are ONE instrument (the
				// host-headroom arm) under two names, and the mode is what
				// separates them (SPEC 2.6's arm table): limit mode reports the
				// host-full fallback as HostFullFired, no-limit as
				// NoLimitHostFired.
				if env.Has(HasLimit) {
					sig.HostFullFired = true
				} else {
					sig.NoLimitHostFired = true
				}
			} else {
				sig.NoHostStatsSaturationFired = true
			}
			if arm == hostFullArm || survivor == nil {
				survivor = f
			}
		case sigLimitSaturation:
			sig.SaturationFired = true
			sig.LimitSaturationFired = true
			if survivor == nil {
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
	// is no local sort anywhere in this package (S3 R8). Then build the verdict
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

	// S3 R5 (F6): the withheld-headroom facts ride Signals, not Verdict — three
	// fields, none of the parked 31 can stand in for them. HostHeadroomAvailable
	// is dispatched on the sample's scope, NOT on the window's state: an
	// affinity box or an unestablished scope is a withholding ("we read it and
	// it means something else"), while a plain /proc/stat read failure leaves
	// the bit set and the window absent, so a read failure is not rendered as a
	// withholding. The two counts ride the snapshot so the F6 sentence can name
	// them without Decide doing any I/O.
	sig.HostHeadroomAvailable = s.CpuScope == ScopeHost
	if lc, ok := s.LogicalCpus.Get(); ok {
		sig.LogicalCpus = lc
	}
	if hc, ok := s.HostCpus.Get(); ok {
		sig.HostCpus = hc
	}

	// S3 R7 spec 1: the no-host-stats saturation fraction is usage-fraction's
	// own reduction — the number the latch was judged on, not the usage track
	// divided by anything, so a mid-run core-count change cannot split them.
	sig.NoHostStatsSaturationFraction, _ = engine.Reduction(sigSaturation, instUsageFraction).Get()

	// S3 R7 spec 2: the dead-zone annotation. Appendix A defines the dead zone
	// as quota nil or non-positive AND PSI absent, and it is an annotation on a
	// healthy verdict, never a state. LimitedVisibility is where ComposeMessage
	// reads it.
	if q, ok := s.Quota.Get(); !ok || q <= 0 {
		sig.LimitedVisibility = !s.PsiAvailable
	}

	// S3 R8 spec 4: the observable metrics, the two track floors and each
	// signal's readiness are filled from the same pass, independent of latch
	// state, so a signal sitting below its mark still reaches Signals. This is
	// the route a no-latch tick's numbers take: Observe returns fired latches
	// only, so without these reads a confident 0 would be published on every
	// healthy tick.
	sig.ThrottleRatio, _ = engine.Reduction(sigThrottling, instThrottleRatio).Get()
	sig.PressureAvg60Out, _ = engine.Reduction(sigPressure, instPressureAvg60).Get()
	sig.StealP95, _ = engine.Reduction(sigSteal, instStealP95).Get()
	sig.HostHeadroomCores, _ = engine.Reduction(sigSaturation, instHostHeadroom).Get()
	sig.AvgUsageCores = ourUsageMean
	sig.HostBusyCores60sMean = hostBusyMean
	sig.UsageRingActive = ourUsageState == diagnosis.StateValue
	sig.HostBusyRingActive = hostBusyState == diagnosis.StateValue

	// The headroom ceiling and the reserve the verdict subtracted, stamped so
	// the message's headline and headroom line read exactly the number the
	// verdict used (S4 R1): capacity is the quota when set and positive, else
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
	// HostBusyCores60sMean == 0 proxy cannot tell an unreadable /proc/stat from
	// an idle host (S4 R2's readability gate).
	if _, ok := s.HostBusy.Get(); ok {
		sig.HostBusyCoresAvailable = true
	}

	// The per-signal readiness trio, out of the same pass that judged them.
	// Ready and nothing else: NoInstrument on a bare-metal box and NoneReady on
	// a thin window both mean this tick has no usable reading, and printing a
	// confident number for either is F1.
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

	// Capability, not readability: F1 is that distinction, and S4 R3 must not
	// read these as "the reading succeeded". The *SignalReady trio is the
	// readability half.
	sig.LimitApplies = env.Has(HasLimit)
	sig.PsiApplies = s.PsiAvailable
	sig.StealApplies = env.Has(HasVirtualization)

	return verdict, sig
}

// causeOf maps one folded Fired to a Cause. The saturation family always maps
// to CauseKindSaturation. The value is the CURRENT reduction of the arm that
// produced the latch, read back through Engine.Reduction — not Fired.Value,
// which is stamped at the firing tick and stays constant while the latch holds;
// the recording's cause values are the current windowed number, which moves
// while the latch is held (the throttling ratio decays, the settled headroom
// deepens). The unit comes from the mark pair that judged the arm.
func causeOf(engine *diagnosis.Engine[Sample], f diagnosis.Fired) Cause {
	switch f.Identity.Signal {
	case sigThrottling:
		v, _ := engine.Reduction(sigThrottling, instThrottleRatio).Get()
		return Cause{Kind: CauseKindThrottling, Value: v, Unit: Unit(f.Marks.Unit)}
	case sigPressure:
		v, _ := engine.Reduction(sigPressure, instPressureAvg60).Get()
		return Cause{Kind: CauseKindPressure, Value: v, Unit: Unit(f.Marks.Unit)}
	case sigSteal:
		// Selection prefers the p95 whenever its window can supply a value, so
		// the cause value follows the same preference: p95 when it is
		// StateValue, the mean before that (F8).
		v, st := engine.Reduction(sigSteal, instStealP95).Get()
		if st != diagnosis.StateValue {
			v, _ = engine.Reduction(sigSteal, instStealMean).Get()
		}
		return Cause{Kind: CauseKindSteal, Value: v, Unit: Unit(f.Marks.Unit)}
	case sigSaturation:
		if f.Marks.Unit == "fraction" {
			v, _ := engine.Reduction(sigSaturation, instUsageFraction).Get()
			return Cause{Kind: CauseKindSaturation, Value: v, Unit: Unit(f.Marks.Unit)}
		}
		v, _ := engine.Reduction(sigSaturation, instHostHeadroom).Get()
		return Cause{Kind: CauseKindSaturation, Value: v, Unit: Unit(f.Marks.Unit)}
	default: // sigLimitSaturation
		v, _ := engine.Reduction(sigLimitSaturation, instLimitHeadroom).Get()
		return Cause{Kind: CauseKindSaturation, Value: v, Unit: Unit(f.Marks.Unit)}
	}
}

// attributeFor derives the attribution from the dominant cause. The saturation
// kind is ambiguous — it is the fold's one cause — so Decide resolves it with
// the survivor's arm: host-full attributes by the split, the limit arm and the
// no-host-stats fallback are internal (the split cannot run for them).
func attributeFor(dominant Cause, survivor *diagnosis.Fired, splitHost bool) Attribution {
	switch dominant.Kind {
	case CauseKindSteal, CauseKindHostContention:
		return AttributionHost
	case CauseKindThrottling, CauseKindPressure:
		return AttributionUnknown
	case CauseKindSaturation:
		// host-full is the survivor from the "saturation" signal whose unit is
		// "cores"; the limit arm and the no-host-stats fallback are internal.
		hostFull := survivor != nil && survivor.Identity.Signal == sigSaturation && survivor.Marks.Unit == "cores"
		if hostFull && splitHost {
			return AttributionHost
		}
		return AttributionUnknown
	}
	return AttributionUnknown
}

// Signals is the per-tick fact bag ComposeMessage and BlockReason read to render
// a sentence the ranked cause list alone cannot carry. The first 31 fields are
// the parked shape in declaration order; everything after them is appended. Two
// of the three frozen signatures take a Signals, so Decide is its sole producer.
//
// ⚠️ C1 scores this struct at exactly 2 (*Available fields: HostBusyCoresAvailable
// and HostHeadroomAvailable) against a cap of 2 — a third fails the gate, which
// is why the readiness trio below is named for what it holds instead.
type Signals struct {
	// The metrics. Each is populated independent of its latch state, so the
	// number stays observable when the latch has not fired.
	UsageFraction    float64 // quota-relative; collapses to 0 in no-limit mode
	ThrottleRatio    float64 // 60s nr_throttled/nr_periods delta; negatives clamped to 0
	PressureAvg60Out float64 // PSI avg60; NaN/negative/+Inf clamped to 0
	StealP95         float64 // 60s p95; 0 on bare metal and below 2 samples

	// Observability only: none of the six changes a verdict.
	AvgUsageFraction float64
	P95UsageFraction float64
	P99UsageFraction float64
	AvgUsageCores    float64 // ABSOLUTE cores; limit-mode headroom and the wire's avgMCpu read this one value
	P95UsageCores    float64
	P99UsageCores    float64
	UsageRingActive  bool // usage-cores reduced to StateValue — S4 R2's LIMIT-mode floor gate
	// HostBusyRingActive: host-busy reduced to StateValue — S4 R2's NO-LIMIT
	// floor gate. Two tracks, two floors: an outage can leave one thin while
	// the other fills.
	HostBusyRingActive bool

	// The latches.
	ThrottleFired       bool
	PressureFired       bool
	StealFired          bool
	HostContentionFired bool // reserved; Decide sets it false unconditionally
	LimitedVisibility   bool // the dead-zone annotation, never a State

	// The saturation family. SaturationFired is the OR of the arms; the four
	// are what S4 R5 dispatches on. Fired.Identity carries the signal name only,
	// so Decide recovers the instrument from Fired.Marks.Unit: "cores" is
	// host-headroom, "fraction" is usage-fraction.
	SaturationFired               bool
	LimitSaturationFired          bool
	HostFullFired                 bool
	NoHostStatsSaturationFired    bool
	NoLimitHostFired              bool
	NoHostStatsSaturationFraction float64

	// The headroom arithmetic. Neither headroom is clamped: a full box yields a
	// negative number, not a 0.
	HostHeadroomCores      float64
	HostBusyCoresAvailable bool // the sample's own readability flag; a ==0 proxy cannot tell unreadable from idle
	HostBusyCores60sMean   float64
	HeadroomCores          float64 // the saturation decision variable
	CapacityCores          float64 // the quota when set and positive, else LogicalCpus
	ReserveCores           float64

	// Capability, NOT readability. F1 is that distinction, and S4 R3 is the rung
	// forbidden from reading these three as "the reading succeeded". The three
	// *SignalReady fields below are the readability half, and the two families
	// are never interchangeable.
	LimitApplies bool
	PsiApplies   bool
	StealApplies bool

	// ---- past the parked 31. S3 R5 fills all three. ----
	//
	// HostHeadroomAvailable is the SECOND field matching C1's pattern and it
	// puts this struct AT the cap of two.
	HostHeadroomAvailable bool    // false when the scope is not ScopeHost: withheld, not failed
	LogicalCpus           float64 // the "2" — the CPUs this process may use
	HostCpus              float64 // the "8" — the machine's count, from S2 R4b

	// ---- per-signal readiness. S3 R8 spec 4 fills all three from Observe's
	// second return. Each is Availability == Ready for that signal. ----
	ThrottleSignalReady bool
	PressureSignalReady bool
	StealSignalReady    bool
}
