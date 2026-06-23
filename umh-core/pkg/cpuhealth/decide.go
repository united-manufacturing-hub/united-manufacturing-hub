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
	CauseKindSaturation CauseKind = "saturation"
	CauseKindThrottling CauseKind = "throttling"
	CauseKindPressure   CauseKind = "pressure"
	CauseKindSteal      CauseKind = "steal"
)

// Cause is a single degradation reason with an associated numeric value.
type Cause struct {
	Kind  CauseKind
	Value float64
}

// Thresholds holds the tunable cutoffs for the Decide verdict.
type Thresholds struct {
	HighUsageFraction float64
	ThrottleHigh      float64
	ThrottleRecover   float64
	PressureHigh      float64
	PressureRecover   float64
	StealHigh         float64
	StealRecover      float64
}

// DefaultThresholds returns the canonical thresholds (HighUsageFraction 0.70,
// ThrottleHigh 0.05, ThrottleRecover 0.03, PressureHigh 0.20, PressureRecover
// 0.12, StealHigh 0.10, StealRecover 0.06).
func DefaultThresholds() Thresholds {
	return Thresholds{
		HighUsageFraction: 0.70,
		ThrottleHigh:      0.05,
		ThrottleRecover:   0.03,
		PressureHigh:      0.20,
		PressureRecover:   0.12,
		StealHigh:         0.10,
		StealRecover:      0.06,
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
	UsageCores  float64
	CgroupCores float64
	Quota       *float64
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
	// Virtualized is set by the sampler from /proc/cpuinfo's hypervisor flag.
	// When false, steal is not a readable signal (it is structurally 0 on bare
	// metal, so reading 0 there is the absence of a signal, not evidence of a
	// healthy host).
	Virtualized bool
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
	throttleRing  []throttlePoint
	stealRing     []stealPoint
	throttleFired bool
	pressureFired bool
	stealFired    bool
}

// stealPoint is one timestamped steal-fraction observation in the WindowState
// ring.
type stealPoint struct {
	ts    time.Time
	steal float64
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
	ThrottleFired bool
	// PressureFired is the pressure Schmitt latch state (fires above
	// PressureHigh, clears only below PressureRecover), independent of the
	// throttle latch.
	PressureFired bool
	// StealFired is the steal Schmitt latch state (fires above StealHigh,
	// clears below StealRecover; holds between), independent of the
	// throttle/pressure latches.
	StealFired bool
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
// contention signal is busy, not sick, so Decide currently returns StateHealthy
// for any sample that has no other cause. Saturation is reintroduced later,
// decided from a windowed average of UsageFraction stored in WindowState
// rather than raw usage.
//
// Decide reads thresholds.ThrottleHigh and thresholds.ThrottleRecover for the
// throttle Schmitt flip-latch (the latch fires above ThrottleHigh and clears
// only below ThrottleRecover). Decide also thresholds sample.PressureAvg60
// DIRECTLY (no ring — the kernel already smoothed it over 60s) against
// thresholds.PressureHigh and thresholds.PressureRecover via a second Schmitt
// flip-latch, emitting a pressure Cause when that latch fires; a NaN/negative
// PressureAvg60 is clamped to 0 before thresholding (see PressureAvg60 doc for
// the failure mode). HighUsageFraction remains reserved for the
// later windowed-saturation logic and is not read yet; when it is, a NaN
// HighUsageFraction must not silently blind saturation detection
// (NaN >= threshold is always false).
//
// Decide also maintains a steal ring of per-tick StealFraction samples
// (virtualized-only: when Sample.Virtualized is false, steal is not processed).
// The steal latch fires when the p95 of the ring exceeds thresholds.StealHigh
// and clears only when the p95 drops below thresholds.StealRecover; between the
// two marks it holds (Schmitt), emitting a steal Cause with AttributionHost
// (host-contention attribution IS computed by Decide when the steal latch
// fires). The ring is pruned to a 60s window (stealWindow) so the p95 is a
// 60s-windowed p95, not an all-history p95; the latch is not evaluated until
// the ring holds at least 2 samples (a first-tick spike cannot fire).
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
			switch {
			case stealP95Val > thresholds.StealHigh:
				st.stealFired = true
			case stealP95Val < thresholds.StealRecover:
				st.stealFired = false
			}
		}
	}

	signals.StealFired = st.stealFired

	var causes []Cause
	if signals.ThrottleFired {
		causes = append(causes, Cause{Kind: CauseKindThrottling, Value: ratio})
	}

	if signals.PressureFired {
		causes = append(causes, Cause{Kind: CauseKindPressure, Value: p})
	}

	if signals.StealFired {
		causes = append(causes, Cause{Kind: CauseKindSteal, Value: stealP95Val})
	}

	if len(causes) > 0 {
		attr := AttributionUnknown
		if signals.StealFired {
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
