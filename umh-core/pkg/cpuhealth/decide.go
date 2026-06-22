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

import "time"

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
)

// CauseKind enumerates the reason classes that can degrade CPU health.
type CauseKind string

const (
	CauseKindSaturation CauseKind = "saturation"
	CauseKindThrottling CauseKind = "throttling"
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
}

// DefaultThresholds returns the canonical thresholds (HighUsageFraction 0.70,
// ThrottleHigh 0.05, ThrottleRecover 0.03).
func DefaultThresholds() Thresholds {
	return Thresholds{
		HighUsageFraction: 0.70,
		ThrottleHigh:      0.05,
		ThrottleRecover:   0.03,
	}
}

// throttleWindow is the sliding window length over which the throttle ratio
// is computed.
const throttleWindow = 60 * time.Second

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
	throttleFired bool
}

// Signals holds derived intermediate values computed during Decide.
type Signals struct {
	UsageFraction float64
	ThrottleFired bool
}

// Verdict is the output of Decide.
type Verdict struct {
	State       State
	Attribution Attribution
	Causes      []Cause
}

// Decide computes a CPU-health verdict from a sample. Decide mutates st
// (*WindowState) in place — appending to the throttle ring and updating the
// flip-latch — so the caller must not share st across goroutines without
// external synchronization. High usage alone is not ill health: a capped
// container pinned at its quota with no throttle/pressure/steal/host-contention
// signal is busy, not sick, so Decide currently returns StateHealthy for any
// sample that has no other cause. Saturation is reintroduced later, decided
// from a windowed average of UsageFraction stored in WindowState rather than
// raw usage.
//
// Decide reads thresholds.ThrottleHigh and thresholds.ThrottleRecover for the
// throttle Schmitt flip-latch (the latch fires above ThrottleHigh and clears
// only below ThrottleRecover). HighUsageFraction remains reserved for the
// later windowed-saturation logic and is not read yet; when it is, a NaN
// HighUsageFraction must not silently blind saturation detection
// (NaN >= threshold is always false).
//
// When Quota is non-nil, Decide uses it exclusively: a positive Quota yields
// UsageCores/Quota, and a non-positive Quota (zero/negative/NaN) means
// uncapped (the `> 0` guard rejects all three), so the unlimited-cgroup case
// (parseCPUMax quotaCores==0) is treated as uncapped instead of poisoning the
// fraction with +Inf/NaN — there is no fallback to CgroupCores when Quota is
// non-nil. When Quota is nil, the fraction is derived from CgroupCores when
// positive. When neither is available the sample is uncapped and Decide
// returns StateHealthy regardless of UsageCores (uncapped cannot be
// quota-saturated); host-contention attribution is not computed by Decide.
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
	// Clear the ring on counter regression BEFORE appending, mirroring
	// production container_monitor.updateThrottleWindow: a cgroup recreation /
	// pod reschedule drops nr_periods and/or nr_throttled below the ring's
	// newest entry, so the pre-reset samples are no longer on the same counter
	// baseline. Without this clear, the stale pre-reset oldest point stays in
	// the ring for up to 60s: nr_periods delta = newest - oldest stays <= 0
	// (throttleRatio returns 0 → latch forced CLEAR, a blind spot), and once the
	// fresh counters regrow past the stale oldest the denominator is inflated by
	// pre-reset periods → ratio understated → throttle silently missed during
	// the cgroup cold-start window where throttling is most likely. Clearing
	// turns the 60s blind spot into a ~1-tick blind spot (the next fresh sample
	// pair rebuilds the delta from the new baseline).
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
	switch {
	case ratio > thresholds.ThrottleHigh:
		st.throttleFired = true
	case ratio < thresholds.ThrottleRecover:
		st.throttleFired = false
	}
	signals.ThrottleFired = st.throttleFired

	if signals.ThrottleFired {
		return Verdict{
			State:       StateDegraded,
			Attribution: AttributionUnknown,
			Causes:      []Cause{{Kind: CauseKindThrottling, Value: ratio}},
		}, signals
	}

	return Verdict{State: StateHealthy}, signals
}

// throttleRatio computes the 60s cumulative throttle ratio as the two-point
// counter delta (nr_throttled delta / nr_periods delta, oldest-to-newest). It
// returns 0 when there are fewer than two points or the period delta is
// non-positive. The non-positive-periods branch is now only a safe divide-guard
// for the genuinely-empty / ring-rebuilding case: Decide clears the ring on any
// counter regression (nrPeriods OR nrThrottled dropping below the ring's newest
// entry) before appending, so a partial reset (nrThrottled-only regression with
// a positive period delta) cannot leave a stale oldest point that would yield a
// negative ratio here. A negative nrThrottled delta with a positive period delta
// is therefore unreachable after the clear-on-regression; the guard remains as a
// defensive divide-guard, not the reset path.
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
