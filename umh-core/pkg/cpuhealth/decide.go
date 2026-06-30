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

package cpuhealth

import (
	"math"
	"sort"
	"time"
)

// State is the overall CPU health state.
type State string

const (
	StateHealthy  State = "healthy"
	StateDegraded State = "degraded"
)

// Attribution names the dominant cause class when degraded.
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

// Cause is a single degradation reason with an associated numeric value.
type Cause struct {
	Kind  CauseKind
	Value float64
}

// Thresholds holds the tunable cutoffs for the Decide verdict.
type Thresholds struct {
	HighUsageFraction float64
	SaturationRecover float64
	ThrottleHigh      float64
	ThrottleRecover   float64
	PressureHigh      float64
	PressureRecover   float64
	StealHigh         float64
	StealRecover      float64
	HostBusyHigh      float64
}

// DefaultThresholds returns the canonical thresholds (HighUsageFraction 0.70,
// SaturationRecover 0.60, ThrottleHigh 0.05, ThrottleRecover 0.03, PressureHigh
// 0.20, PressureRecover 0.12, StealHigh 0.10, StealRecover 0.06, HostBusyHigh
// 0.70).
func DefaultThresholds() Thresholds {
	return Thresholds{
		HighUsageFraction: 0.70,
		SaturationRecover: 0.60,
		ThrottleHigh:      0.05,
		ThrottleRecover:   0.03,
		PressureHigh:      0.20,
		PressureRecover:   0.12,
		StealHigh:         0.10,
		StealRecover:      0.06,
		HostBusyHigh:      0.70,
	}
}

// throttleWindow is the sliding window length over which the throttle ratio
// is computed.
const throttleWindow = 60 * time.Second

// stealWindow is the sliding window length over which the steal p95 is
// computed. It matches throttleWindow (60s) so both rings cover the same
// observation horizon; they are kept as separate consts so they can diverge
// if the debounce needs ever call for it.
const stealWindow = 60 * time.Second

// Sample is a single point-in-time CPU usage observation.
//
// Quota is the cgroup CPU quota as a pointer so that nil distinguishes "no
// quota set" from a read-failed / first-call / genuinely-idle zero. When Quota
// is non-nil, Decide uses it exclusively: a positive value derives the usage
// fraction (UsageCores/Quota), and a non-positive value (zero, negative, or
// NaN — the `> 0` guard rejects all three, mirroring the legacy CgroupCores
// guard) means uncapped, so a zero quota — the unlimited-cgroup case returned
// by parseCPUMax for `cpu.max = "max <period>"` — cannot produce a +Inf/NaN
// poison fraction and stays healthy. When Quota is nil, Decide falls back to
// CgroupCores when it is positive, and otherwise treats the sample as uncapped
// (StateHealthy, no fraction computed).
// CgroupCores is the legacy non-pointer representation retained for callers
// that still populate it (it remains the live production path).
type Sample struct {
	Timestamp   time.Time
	Quota       *float64
	UsageCores  float64
	CgroupCores float64
	NrPeriods   int64
	NrThrottled int64
	// PressureAvg60 is the kernel's cpu.pressure "some avg60" running 60s
	// average, thresholded DIRECTLY by Decide (no extra windowing — the kernel
	// already smoothed it over 60s). The value is a FRACTION in [0,1], not the
	// raw kernel percentage: /proc/pressure/cpu reports avg60 in 0..100, so the
	// reader MUST divide by 100 before assigning this field (DefaultThresholds
	// PressureHigh 0.20 / PressureRecover 0.12 are fractions; passing a raw
	// kernel percentage would fire at 0.2%, essentially always-on). A NaN,
	// negative, OR +Inf value (malformed PSI line, div-by-zero, transient read
	// failure) is clamped to 0 before thresholding and before exposure as a
	// Cause Value via the `!(p >= 0) || math.IsInf(p, 1)` guard (catches NaN AND
	// negatives in one test via the `!(p >= 0)` idiom, plus +Inf via the
	// IsInf check since `+Inf >= 0` is true and would otherwise skip the clamp):
	// NaN `> PressureHigh` and NaN `< PressureRecover` are both false, so without
	// the guard a NaN reading would stick the latch at its prior state
	// indefinitely (a fired cause could never self-clear), and a NaN or +Inf
	// Cause Value would break JSON marshalling of the whole Verdict. This clamp
	// is intentionally stricter than the throttle ratio clamp (`ratio < 0`,
	// which only catches negatives): PressureAvg60 is a raw float64 input that
	// can be NaN or +Inf, unlike the integer-derived throttle ratio whose clamp
	// is NaN-safe only because throttleRatio() structurally cannot produce NaN.
	PressureAvg60 float64
	// StealFraction is the fraction of wall-time the hypervisor gave this VM's
	// vCPU to other VMs (0.0-1.0), read from the 8th field of /proc/stat's
	// `cpu ` line. It is only readable on a virtualized box; when Virtualized
	// is false Decide does not process steal (it is not a readable signal on
	// bare metal).
	StealFraction float64
	// HostBusyCores is the count of host-level busy CPU cores, readable
	// independent of virtualization. The sampler computes it from /proc/stat's
	// non-idle fields EXCLUDING the steal, guest, and guest_nice columns, so
	// steal is not double-counted (steal is already its own cause). LogicalCpus
	// is the host's total logical CPU count; their ratio
	// (HostBusyCores/LogicalCpus) is the host busyness fraction thresholded
	// against HostBusyHigh for the host-contention latch. LogicalCpus MUST be
	// positive for the ratio to be meaningful; Decide treats LogicalCpus <= 0
	// as no-signal (the latch is not evaluated), mirroring the non-positive
	// Quota / CgroupCores guards.
	HostBusyCores float64
	LogicalCpus   float64
	// Virtualized is set by the sampler from /proc/cpuinfo's hypervisor flag.
	// When false, steal is not a readable signal (it is structurally 0 on bare
	// metal, so reading 0 there is the absence of a signal, not evidence of a
	// healthy host).
	Virtualized bool
	// PsiAvailable is the readability flag for PSI (cpu.pressure). It
	// distinguishes "PSI compiled in + present" (true) from "PressureAvg60
	// reads 0 because PSI is absent" (false). Without it the dead-zone
	// saturation backstop branch is unreachable dead code: a naive
	// "PressureAvg60 == 0" is true both when PSI is absent AND when PSI is
	// present but reading 0. When PsiAvailable is false (and Quota is nil and
	// Virtualized is false) the sample is in the dead-zone — the one case where
	// no starvation signal exists, so sustained high usage is the last-resort
	// proxy via the saturation backstop latch.
	PsiAvailable bool
}

// throttlePoint is one timestamped throttle-counter observation in the
// WindowState ring.
type throttlePoint struct {
	ts          time.Time
	nrPeriods   int64
	nrThrottled int64
}

// WindowState holds the per-signal 60s sample ring and the per-signal
// flip-latch that Decide mutates in place. The sliding 60s window is the
// debounce; the asymmetric (Schmitt) recover band is the only extra mechanism.
type WindowState struct {
	throttleRing        []throttlePoint
	stealRing           []stealPoint
	usageRing           []usagePoint
	hostBusyRing        []hostBusyPoint
	throttleFired       bool
	pressureFired       bool
	stealFired          bool
	hostContentionFired bool
	saturationFired     bool
}

// stealPoint is one timestamped steal-fraction observation in the WindowState
// ring.
type stealPoint struct {
	ts    time.Time
	steal float64
}

// usagePoint is one timestamped usage-fraction observation in the WindowState
// usage ring, used by the dead-zone saturation backstop latch.
type usagePoint struct {
	ts       time.Time
	fraction float64
}

// hostBusyPoint is one timestamped HostBusyCores observation in the WindowState
// hostBusyRing, used to compute Signals.HostBusyCores60sMean.
type hostBusyPoint struct {
	ts       time.Time
	hostBusy float64
}

// Signals holds derived intermediate values computed during Decide.
type Signals struct {
	UsageFraction float64
	// ThrottleRatio is the computed 60s cumulative throttle ratio
	// (nr_throttled delta / nr_periods delta, oldest-to-newest over the pruned
	// window), populated UNCONDITIONALLY — independent of the flip-latch state —
	// so the numeric metric is observable on the wire even when the latch has
	// not fired. Negatives are clamped to 0 before assignment so a residual
	// negative ratio from any edge case cannot leak to the wire.
	ThrottleRatio float64
	// PressureAvg60Out is the clamped pressure avg60 value populated
	// UNCONDITIONALLY — independent of the PressureFired latch state — so a
	// follow-up consumer can surface the numeric metric even when the latch has
	// not fired (mirroring how ThrottleRatio is populated). NaN/negative/+Inf
	// are clamped to 0.
	PressureAvg60Out float64
	// StealP95 is the 60s-windowed steal p95 populated UNCONDITIONALLY —
	// independent of the StealFired latch state — so a follow-up consumer can
	// surface the numeric metric even when the latch has not fired (mirroring
	// how ThrottleRatio is populated). It is 0 on a non-virtualized box (steal
	// is not a readable signal) and 0 until the ring holds at least 2 samples.
	StealP95 float64
	// AvgUsageFraction, P95UsageFraction, P99UsageFraction are the avg/p95/p99
	// of the dead-zone usage ring — fractions relative to 1 core, typically in
	// [0,1] but may exceed 1 when observed usage exceeds CgroupCores
	// (oversubscription); callers that surface mCPU multiply by 1000. Computed
	// UNCONDITIONALLY whenever the ring holds >= 2 entries (observability-only,
	// like ThrottleRatio); 0 otherwise. They do NOT change the verdict — the
	// saturation latch still fires on the AVG, not p95. Outside the dead-zone
	// the usage ring is cleared, so these are 0 there (usage is not a health
	// signal outside the dead-zone currently).
	//
	// UsageRingActive is the fetchability flag for the three percentile fields
	// above: true when the usage ring holds >= 2 entries (the dead-zone with
	// enough samples to compute percentiles), false otherwise (outside the
	// dead-zone, or the first dead-zone tick before the ring has 2 entries).
	// Callers that mirror the percentiles onto a wire use this flag to decide
	// whether to emit a 0 (fetchable) or omit (un-fetchable), instead of the
	// value-based 0/omitempty discipline that cannot distinguish a real 0 from
	// an absent signal.
	AvgUsageFraction float64
	P95UsageFraction float64
	P99UsageFraction float64
	UsageRingActive  bool
	ThrottleFired    bool
	// PressureFired is the pressure Schmitt latch state (fires above
	// PressureHigh, clears only below PressureRecover), independent of the
	// throttle latch.
	PressureFired bool
	// StealFired is the steal Schmitt latch state (fires above StealHigh,
	// clears below StealRecover; holds between), independent of the
	// throttle/pressure latches.
	StealFired bool
	// HostContentionFired is the host-contention latch state. See the inline
	// comment at the latch in Decide for the fire/clear conditions.
	HostContentionFired bool
	// LimitedVisibility is true when the sample is in the dead-zone (Quota nil
	// or non-positive, PsiAvailable false, Virtualized false) — the blind state
	// where no starvation signal exists. It is a signal for the caller's
	// message, NOT a
	// state: the verdict State is binary healthy|degraded (blind-but-quiet is
	// healthy). It is false outside the dead-zone.
	LimitedVisibility bool
	// SaturationFired is the dead-zone saturation backstop latch state. It
	// fires when the 60s-average usage fraction >= HighUsageFraction and clears
	// when the 60s-average drops below SaturationRecover (Schmitt). It is only
	// evaluated in the dead-zone.
	SaturationFired bool
	// HostBusyCores60sMean is the 60s arithmetic mean of per-tick HostBusyCores;
	// observability-only (does not change the verdict). The per-tick input is
	// clamped (NaN/negative/+Inf → 0) because a malformed value poisons the
	// running sum until it ages out.
	HostBusyCores60sMean float64
}

// Verdict is the output of Decide.
type Verdict struct {
	State       State
	Attribution Attribution
	Causes      []Cause
}

// Decide computes a CPU-health verdict from a sample. Decide mutates st
// (*WindowState) in place — appending to the throttle and steal rings and
// updating the flip-latches — so the caller must not share st across goroutines
// without external synchronization. High usage alone is not ill health: a
// capped container pinned at its quota with no throttle/pressure/steal/host-
// contention signal is busy, not sick, so Decide returns StateHealthy for any
// sample that has no other cause. The ONE exception is the dead-zone
// saturation backstop: when the sample is in the dead-zone (Quota nil or
// non-positive, PSI absent, not virtualized — no starvation signal exists),
// Decide maintains a
// 60s-average usage fraction over a usage ring in WindowState and fires a
// saturation Cause when that average >= HighUsageFraction (Schmitt: fires at
// >= HighUsageFraction, clears below SaturationRecover). Outside the dead-zone
// high usage alone is never ill health, and a prior dead-zone fire is cleared
// on the first non-dead-zone tick (the latch and ring do not leak).
//
// Decide reads thresholds.ThrottleHigh and thresholds.ThrottleRecover for the
// throttle Schmitt flip-latch (the latch fires above ThrottleHigh and clears
// only below ThrottleRecover). Decide also thresholds sample.PressureAvg60
// DIRECTLY (no ring — the kernel already smoothed it over 60s) against
// thresholds.PressureHigh and thresholds.PressureRecover via a second Schmitt
// flip-latch, emitting a pressure Cause when that latch fires; a NaN/negative
// PressureAvg60 is clamped to 0 before thresholding (see PressureAvg60 doc for
// the failure mode). HighUsageFraction is read by the dead-zone saturation
// backstop latch (fires when the 60s-avg usage fraction >= HighUsageFraction,
// clears below SaturationRecover); a NaN HighUsageFraction must not silently
// blind saturation detection (NaN >= threshold is always false).
//
// Decide also maintains a steal ring of per-tick StealFraction samples
// (virtualized-only: when Sample.Virtualized is false, steal is not processed).
// The steal latch fires when the p95 of the ring exceeds thresholds.StealHigh
// and clears only when the p95 drops below thresholds.StealRecover; between the
// two marks it holds (Schmitt), emitting a steal Cause. Steal is an external
// cause (AttributionHost when it is the dominant cause), distinct from the
// separate CauseKindHostContention cause (host-contention the cause) computed
// by the host-contention latch below. The ring is pruned to a 60s window
// (stealWindow) so the p95 is a 60s-windowed p95, not an all-history p95; the
// latch is not evaluated until the ring holds at least 2 samples (a first-tick
// spike cannot fire).
//
// Decide also maintains a demand-gated host-contention latch (see the inline
// comment at the latch for the fire/clear conditions).
//
// When several causes fire, the DOMINANT (highest-severity) one sets
// Attribution: external causes (steal, host-contention) yield AttributionHost,
// internal causes (throttle, pressure) yield AttributionUnknown. Ties go to the
// external/host side. Verdict.Causes is ordered dominant-first.
//
// When Quota is non-nil, Decide uses it exclusively: a positive Quota yields
// UsageCores/Quota, and a non-positive Quota (zero/negative/NaN) means
// uncapped (the `> 0` guard rejects all three), so the unlimited-cgroup case
// (parseCPUMax quotaCores==0) is treated as uncapped instead of poisoning the
// fraction with +Inf/NaN — there is no fallback to CgroupCores when Quota is
// non-nil. When Quota is nil, the fraction is derived from CgroupCores when
// positive. When neither is available the sample is uncapped and Decide
// returns StateHealthy regardless of UsageCores (uncapped cannot be
// quota-saturated).
func Decide(st *WindowState, sample Sample, thresholds Thresholds) (Verdict, Signals) {
	var fraction float64

	if sample.Quota != nil {
		if *sample.Quota > 0 {
			fraction = sample.UsageCores / *sample.Quota
		}
	} else if sample.CgroupCores > 0 {
		fraction = sample.UsageCores / sample.CgroupCores
	}

	signals := Signals{UsageFraction: fraction}

	// Throttle flip-latch: maintain a 60s ring of throttle-counter samples and
	// a Schmitt trigger that fires above ThrottleHigh and clears only below
	// ThrottleRecover. The 60s ratio is the two-point counter delta
	// (nr_throttled delta / nr_periods delta, oldest-to-newest over the pruned
	// window). A sample exactly at the cutoff is kept (Before(cutoff) is false).
	var ratio float64
	// Clear the ring on EITHER counter regression BEFORE appending: a cgroup
	// recreation / pod reschedule drops nr_periods and/or nr_throttled below the
	// ring's newest entry, so the pre-reset samples are no longer on the same
	// counter baseline. Without this clear, the stale pre-reset oldest point
	// stays in the ring for up to 60s: nr_periods delta = newest - oldest stays
	// <= 0 (throttleRatio returns 0 → latch forced CLEAR, a blind spot), and
	// once the fresh counters regrow past the stale oldest the denominator is
	// inflated by pre-reset periods → ratio understated → throttle silently
	// missed during the cgroup cold-start window where throttling is most
	// likely. Clearing turns the 60s blind spot into a ~1-tick blind spot (the
	// next fresh sample pair rebuilds the delta from the new baseline).
	//
	// Either counter regressing triggers the clear. A nrThrottled-only drop
	// with a growing nrPeriods is treated the same as a nrPeriods regression:
	// the counters are no longer on the same baseline, so the ring is rebuilt
	// from the fresh sample.
	if len(st.throttleRing) > 0 {
		newest := st.throttleRing[len(st.throttleRing)-1]
		if sample.NrPeriods < newest.nrPeriods || sample.NrThrottled < newest.nrThrottled {
			st.throttleRing = st.throttleRing[:0]
		}
	}

	st.throttleRing = append(st.throttleRing, throttlePoint{
		ts:          sample.Timestamp,
		nrPeriods:   sample.NrPeriods,
		nrThrottled: sample.NrThrottled,
	})
	cutoff := sample.Timestamp.Add(-throttleWindow)
	ring := st.throttleRing
	n := 0

	for _, p := range ring {
		if !p.ts.Before(cutoff) {
			ring[n] = p
			n++
		}
	}

	ring = ring[:n]
	st.throttleRing = ring

	ratio = throttleRatio(ring)
	// Clamp negatives to 0 before exposing on the wire: a residual negative
	// ratio from any edge case must not leak to callers (the numeric metric is
	// observable independent of latch state).
	if ratio < 0 {
		ratio = 0
	}

	signals.ThrottleRatio = ratio
	switch {
	case ratio > thresholds.ThrottleHigh:
		st.throttleFired = true
	case ratio < thresholds.ThrottleRecover:
		st.throttleFired = false
	}

	signals.ThrottleFired = st.throttleFired

	// Pressure flip-latch: pressure is the kernel's own 60s running average
	// (cpu.pressure "some avg60"), so it is thresholded DIRECTLY — no additional
	// windowing/ring (unlike throttle, which needs the counter-delta ring). The
	// Schmitt flip-latch is the only state: it fires above PressureHigh and
	// clears only below PressureRecover; between the two marks it holds.
	// NaN/negative/+Inf clamped to 0 before thresholding and before exposure as
	// a Cause Value; see PressureAvg60 doc for the failure mode and the
	// stricter-than-throttle clamp rationale.
	p := sample.PressureAvg60
	if !(p >= 0) || math.IsInf(p, 1) {
		p = 0
	}

	signals.PressureAvg60Out = p
	switch {
	case p > thresholds.PressureHigh:
		st.pressureFired = true
	case p < thresholds.PressureRecover:
		st.pressureFired = false
	}

	signals.PressureFired = st.pressureFired

	// Steal flip-latch (virtualized-only): steal is the fraction of wall-time
	// the hypervisor gave this VM's vCPU to other VMs (Sample.StealFraction).
	// Unlike throttle (counter-delta) and pressure (kernel-avg60 direct), steal
	// uses a ring of per-tick StealFraction samples reduced by p95
	// (near-worst-of-window via nearest-rank: a sustained spike fires, a single
	// isolated spike is absorbed). The latch fires when the p95 > StealHigh and
	// clears only when the p95 drops below StealRecover; between the two marks it
	// holds (Schmitt). Steal is only processed on a virtualized box: when
	// Virtualized is false, steal is not a readable signal (it is structurally 0
	// on bare metal, so reading 0 there is the absence of a signal, not evidence
	// of a healthy host), so Decide skips the steal ring entirely.
	var stealP95Val float64

	if sample.Virtualized {
		st.stealRing = append(st.stealRing, stealPoint{
			ts:    sample.Timestamp,
			steal: sample.StealFraction,
		})

		// Prune entries older than the steal window. This mirrors the throttle
		// ring's pruning: a sample exactly at the cutoff is kept (Before(cutoff)
		// is false). stealPoint.ts is read here — without pruning the ring grows
		// unbounded and the p95 becomes an all-history p95, not a 60s-windowed
		// p95.
		stealCutoff := sample.Timestamp.Add(-stealWindow)
		stealRing := st.stealRing
		stealN := 0

		for _, sp := range stealRing {
			if !sp.ts.Before(stealCutoff) {
				stealRing[stealN] = sp
				stealN++
			}
		}

		stealRing = stealRing[:stealN]
		st.stealRing = stealRing

		// Small-N floor: do NOT evaluate the steal latch until the ring holds at
		// least 2 samples. With a single sample the nearest-rank p95 is that
		// sample's value, so a first-tick high-steal reading (0.15) would fire the
		// latch immediately — contradicting "sustained fires, isolated absorbed."
		// A 2-sample floor mirrors throttle's two-point delta floor
		// (throttleRatio returns 0 when len < 2) for consistency.
		if len(st.stealRing) >= 2 {
			stealP95Val = stealP95(st.stealRing)
			// Clamp NaN/negative/+Inf to 0 before thresholding and before
			// exposure as a Cause Value, mirroring the PressureAvg60 clamp: a
			// NaN StealFraction (malformed /proc/stat parse) makes stealP95
			// return NaN, which would hold the steal latch (NaN > StealHigh and
			// NaN < StealRecover both false), break json.Marshal of the whole
			// Verdict (NaN Cause Value), and corrupt the sort comparator.
			if !(stealP95Val >= 0) || math.IsInf(stealP95Val, 1) {
				stealP95Val = 0
			}

			switch {
			case stealP95Val > thresholds.StealHigh:
				st.stealFired = true
			case stealP95Val < thresholds.StealRecover:
				st.stealFired = false
			}
		}
	}

	signals.StealFired = st.stealFired
	signals.StealP95 = stealP95Val

	// Host-contention latch: fires when the demand gate (pressure OR throttle)
	// is open AND host_busy_ratio > HostBusyHigh. Clears when the demand gate
	// drops — it cannot stay fired once every demand signal is gone, even if
	// the host is still busy (the light-workload false-degrade guard). The
	// demand gate is pressure OR throttle: a throttled container is
	// unambiguously demanding CPU, so host-contention can fire under a
	// throttle-only demand too, not just pressure. The host_busy_ratio divide
	// is guarded: LogicalCpus <= 0 means no-signal (the latch is not
	// evaluated), mirroring the non-positive Quota / CgroupCores guards and
	// preventing a +Inf false-fire or NaN silent-disable. The Cause Value is
	// contentionCores (HostBusyCores - UsageCores), clamped to finite >= 0 so
	// a NaN or +Inf reading cannot break JSON marshalling of the whole Verdict
	// (same discipline as the PressureAvg60 clamp).
	var (
		contentionCores float64
		hostBusyRatio   float64
	)

	demandGateOpen := signals.PressureFired || signals.ThrottleFired
	if !demandGateOpen {
		st.hostContentionFired = false
	} else if sample.LogicalCpus > 0 && !math.IsInf(sample.LogicalCpus, 0) {
		hostBusyRatio = sample.HostBusyCores / sample.LogicalCpus
		// Clamp NaN/negative/+Inf to 0 before using hostBusyRatio for the
		// severity call and the latch comparison, mirroring the
		// PressureAvg60/stealP95 clamps: a NaN HostBusyCores would produce a
		// NaN hostBusyRatio that escapes the severity clamps (NaN comparisons
		// are false), corrupting the sort comparator and the latch.
		if !(hostBusyRatio >= 0) || math.IsInf(hostBusyRatio, 1) {
			hostBusyRatio = 0
		}

		contentionCores = sample.HostBusyCores - sample.UsageCores
		if !(contentionCores >= 0) || math.IsInf(contentionCores, 0) {
			contentionCores = 0
		}

		if hostBusyRatio > thresholds.HostBusyHigh {
			st.hostContentionFired = true
		}
	}

	signals.HostContentionFired = st.hostContentionFired

	// Host-busy 60s mean ring: maintain a 60s sliding window of per-tick
	// HostBusyCores samples and expose the arithmetic mean as
	// Signals.HostBusyCores60sMean: append-per-tick, prune entries older than
	// 60s with the cutoff sample KEPT (!ts.Before(cutoff)), a 2-sample floor
	// emitting 0. The input HostBusyCores is clamped BEFORE insert
	// (NaN/negative/+Inf → 0). This is an INPUT clamp, DISTINCT from the steal
	// ring's OUTPUT clamp: steal appends StealFraction raw and clamps the p95
	// AFTER computation, whereas hostBusy clamps the per-tick value BEFORE
	// append. The disciplines differ on purpose — a NaN/negative in a running
	// MEAN poisons the sum until the sample ages out (60s), unlike a p95 which
	// can be clamped post-hoc. hostBusy also runs UNCONDITIONALLY every tick
	// (no virtualization gate, unlike steal which is skipped when
	// sample.Virtualized is false), so the input clamp is the only defense.
	// This rung ONLY computes and exposes the mean — NO verdict change, NO
	// new/changed cause, NO headroom formula.
	var hostBusyMean float64

	hb := sample.HostBusyCores
	if !(hb >= 0) || math.IsInf(hb, 1) {
		hb = 0
	}

	st.hostBusyRing = append(st.hostBusyRing, hostBusyPoint{
		ts:       sample.Timestamp,
		hostBusy: hb,
	})
	hostBusyCutoff := sample.Timestamp.Add(-stealWindow)
	hostBusyRing := st.hostBusyRing
	hostBusyN := 0

	for _, hp := range hostBusyRing {
		if !hp.ts.Before(hostBusyCutoff) {
			hostBusyRing[hostBusyN] = hp
			hostBusyN++
		}
	}

	hostBusyRing = hostBusyRing[:hostBusyN]
	st.hostBusyRing = hostBusyRing

	if len(st.hostBusyRing) >= 2 {
		var sum float64
		for _, hp := range st.hostBusyRing {
			sum += hp.hostBusy
		}

		hostBusyMean = sum / float64(len(st.hostBusyRing))
	}

	signals.HostBusyCores60sMean = hostBusyMean

	// Saturation backstop (dead-zone-only): the dead-zone is the one case where
	// no starvation signal exists — Quota is nil or non-positive (uncapped → no
	// limit), PsiAvailable is false (no PSI → pressure is structurally absent),
	// and Virtualized is false (no steal). There, sustained high usage is the
	// last-resort proxy. Decide computes a 60s-AVERAGE usage fraction (NOT p95 —
	// a sustained-headroom proxy; a brief spike must not trip it) over a usage
	// ring in WindowState. A Schmitt latch fires when the 60s-avg >=
	// HighUsageFraction and clears only when it drops below SaturationRecover;
	// between the marks it holds. The 60s usage ring is pruned (reusing the
	// throttle/steal pruning pattern). Saturation is the only NEW cause the
	// dead-zone introduces (the only cause whose signal is absent elsewhere);
	// throttle can co-fire from cpu.stat counter deltas regardless of Quota, so
	// it is NOT the sole cause that can fire in the dead-zone. The backstop
	// only fires for the Quota==nil + CgroupCores>0 sub-case, where a fraction
	// is computable; Quota=&0 (truly uncapped, cpu.max="max") cannot compute a
	// fraction (the `>0` guard rejects it and there is no CgroupCores fallback
	// when Quota is non-nil), so the proxy is inert there by design —
	// blind-but-quiet is healthy per the model. The guardrail: below the fire
	// mark in the dead-zone the verdict is healthy — blind-but-quiet is
	// healthy, never a distinct unknown state. LimitedVisibility signals the
	// blind state to the caller regardless of the verdict.
	var saturationAvg float64

	deadZone := (sample.Quota == nil || !(*sample.Quota > 0)) && !sample.PsiAvailable && !sample.Virtualized
	signals.LimitedVisibility = deadZone

	if deadZone {
		// Input clamp: a NaN or +Inf UsageCores reading produces a NaN/+Inf
		// fraction that would poison the 60s running sum until the sample ages
		// out (NaN propagates through sum/len, making every subsequent average
		// NaN — the latch could never fire and an existing fire would silently
		// clear, blinding saturation for up to 60s). Clamp the stored fraction
		// before insertion, mirroring the PressureAvg60 input clamp. The emitted
		// signals.UsageFraction is reassigned to the clamped value so the wire
		// field stays consistent with the ring copy.
		if !(fraction >= 0) || math.IsInf(fraction, 1) {
			fraction = 0
		}

		signals.UsageFraction = fraction
		st.usageRing = append(st.usageRing, usagePoint{
			ts:       sample.Timestamp,
			fraction: fraction,
		})

		usageCutoff := sample.Timestamp.Add(-throttleWindow)
		usageRing := st.usageRing
		usageN := 0

		for _, up := range usageRing {
			if !up.ts.Before(usageCutoff) {
				usageRing[usageN] = up
				usageN++
			}
		}

		usageRing = usageRing[:usageN]
		st.usageRing = usageRing

		var sum float64
		for _, up := range st.usageRing {
			sum += up.fraction
		}

		saturationAvg = sum / float64(len(st.usageRing))
		// Small-N floor: do NOT evaluate the saturation latch until the ring
		// holds at least 2 samples. With a single sample the 60s-avg is that
		// sample's value, so a first-tick high-usage reading would fire
		// immediately — contradicting "sustained fires, isolated absorbed."
		// A 2-sample floor mirrors the steal ring's floor and throttle's
		// two-point delta floor for consistency. When the ring holds fewer
		// than 2 samples (e.g. after a >60s sampling gap pruned the ring to a
		// single sample), there is insufficient evidence to sustain a prior
		// fire — clear the latch so a stale fire does not emit a
		// {saturation, Value: <lone-sample>} cause contradicting the
		// documented "clears when the 60s-average drops below
		// SaturationRecover."
		if len(st.usageRing) >= 2 {
			switch {
			case saturationAvg >= thresholds.HighUsageFraction:
				st.saturationFired = true
			case saturationAvg < thresholds.SaturationRecover:
				st.saturationFired = false
			}
		} else {
			st.saturationFired = false
		}
	} else {
		// Outside the dead-zone the saturation backstop is not evaluated.
		// Mirror the host-contention latch's demand-gate-close clear: reset
		// the latch AND the usage ring so a prior dead-zone fire cannot leak a
		// stale {saturation, Value: 0} cause on every subsequent non-dead-zone
		// tick (saturationAvg is only computed inside the dead-zone block, so a
		// stale latch would emit a zero-valued cause). Clearing the ring is
		// required, not optional: a dead-zone→non-dead-zone→dead-zone
		// transition changes the fraction denominator, so stale samples would
		// corrupt the 60s average on re-entry.
		st.saturationFired = false
		st.usageRing = st.usageRing[:0]
	}

	signals.SaturationFired = st.saturationFired

	if n := len(st.usageRing); n >= 2 {
		signals.UsageRingActive = true

		vals := make([]float64, n)
		for i, up := range st.usageRing {
			vals[i] = up.fraction
		}

		sort.Float64s(vals)

		// AvgUsageFraction is the arithmetic mean of the same vals slice used
		// for p95/p99, computed UNCONDITIONALLY when the ring holds >= 2
		// entries (independent of the dead-zone saturation latch). This is
		// the same value as saturationAvg when in the dead-zone, but it does
		// not depend on the dead-zone-only local.
		var sum float64
		for _, v := range vals {
			sum += v
		}

		signals.AvgUsageFraction = sum / float64(n)

		p95Rank := int(math.Ceil(0.95 * float64(n)))
		signals.P95UsageFraction = vals[p95Rank-1]
		p99Rank := int(math.Ceil(0.99 * float64(n)))
		signals.P99UsageFraction = vals[p99Rank-1]
	}

	var causes []Cause

	type firedCause struct {
		cause    Cause
		severity float64
		external bool
	}

	var fired []firedCause
	if signals.ThrottleFired {
		fired = append(fired, firedCause{Cause{Kind: CauseKindThrottling, Value: ratio}, severity(ratio, thresholds.ThrottleHigh), false})
	}

	if signals.PressureFired {
		fired = append(fired, firedCause{Cause{Kind: CauseKindPressure, Value: p}, severity(p, thresholds.PressureHigh), false})
	}

	if signals.StealFired {
		fired = append(fired, firedCause{Cause{Kind: CauseKindSteal, Value: stealP95Val}, severity(stealP95Val, thresholds.StealHigh), true})
	}

	if signals.HostContentionFired {
		fired = append(fired, firedCause{Cause{Kind: CauseKindHostContention, Value: contentionCores}, severity(hostBusyRatio, thresholds.HostBusyHigh), true})
	}

	if signals.SaturationFired {
		fired = append(fired, firedCause{Cause{Kind: CauseKindSaturation, Value: saturationAvg}, severity(saturationAvg, thresholds.HighUsageFraction), false})
	}

	if len(fired) > 0 {
		sort.SliceStable(fired, func(i, j int) bool {
			if fired[i].severity != fired[j].severity {
				return fired[i].severity > fired[j].severity
			}
			// Ties go to the external side (host).
			return fired[i].external && !fired[j].external
		})

		causes = make([]Cause, len(fired))
		for i, fc := range fired {
			causes[i] = fc.cause
		}

		attr := AttributionUnknown
		if fired[0].external {
			attr = AttributionHost
		}

		return Verdict{
			State:       StateDegraded,
			Attribution: attr,
			Causes:      causes,
		}, signals
	}

	return Verdict{State: StateHealthy}, signals
}

// throttleRatio computes the 60s cumulative throttle ratio as the two-point
// counter delta (nr_throttled delta / nr_periods delta, oldest-to-newest). It
// returns 0 when there are fewer than two points or the period delta is
// non-positive. The non-positive-periods branch is a safe divide-guard for the
// genuinely-empty / ring-rebuilding case. Decide clears the ring when either
// counter regresses below the ring's newest entry before appending, so a
// post-reset ring holds only fresh-baseline points. The guard remains as a
// defensive divide-guard for the empty/ring-rebuilding case. Decide clamps any
// residual negative ratio to 0 before exposing it on Signals.ThrottleRatio.
func throttleRatio(ring []throttlePoint) float64 {
	if len(ring) < 2 {
		return 0
	}

	oldest := ring[0]
	newest := ring[len(ring)-1]

	periods := newest.nrPeriods - oldest.nrPeriods
	if periods <= 0 {
		return 0
	}

	return float64(newest.nrThrottled-oldest.nrThrottled) / float64(periods)
}

// stealP95 returns the nearest-rank p95 of the steal fractions in the ring
// (near-worst-of-window via nearest-rank). Nearest-rank: rank = ceil(0.95 * N),
// value at sorted[rank-1] (0-indexed). Returns 0 when the ring is empty. With
// 20 identical samples the p95 is that value; a single spike among 20 zeros
// yields p95 0.0 (the spike is absorbed), so the window absorbs a transient
// isolated spike while a sustained spike fires.
func stealP95(ring []stealPoint) float64 {
	n := len(ring)
	if n == 0 {
		return 0
	}

	vals := make([]float64, n)
	for i, p := range ring {
		vals[i] = p.steal
	}

	sort.Float64s(vals)

	rank := int(math.Ceil(0.95 * float64(n)))
	if rank < 1 {
		rank = 1
	}

	return vals[rank-1]
}

// severity returns the fraction into the danger band toward maximum: (value -
// high) / (1 - high). A value just above high yields ~0; a value at 1.0 yields
// 1.0 when high < 1.0. This is the shared scale that puts every cause on one
// axis for dominance comparison.
//
// Preconditions: high must be in [0, 1) for the scale to be finite and
// meaningful; DefaultThresholds honors this today. severity is NaN-safe to
// keep the dominance sort well-defined when a feed produces a NaN/+Inf reading
// or a caller mis-configures a threshold: any NaN/+Inf input, or a high >= 1.0,
// yields 0 (treat as not-firing, lowest severity) so a malformed reading or
// threshold never dominates a valid one and NaN never reaches the sort
// comparator. host-contention's severity numerator is host_busy_ratio
// (HostBusyCores/LogicalCpus), NOT its wire Cause Value (contentionCores =
// HostBusyCores-UsageCores) — the two are different axes.
//
// A HELD cause (Schmitt latch fired, current reading in the hold band below the
// High mark) yields a negative raw severity. That negative is clamped to 0 so
// held causes tie at 0 and the external tie-break decides deterministically
// (Host whenever an external cause is held), rather than the least-negative
// held cause winning and making Attribution flap tick-to-tick as held readings
// jitter.
func severity(value, high float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.IsNaN(high) || math.IsInf(high, 0) || high >= 1.0 {
		return 0
	}

	sev := (value - high) / (1 - high)
	if sev < 0 {
		return 0
	}

	return sev
}
