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
	// LimitReserveFraction is the limit-mode headroom reserve as a fraction
	// of the quota: headroom = quota − containerUsage60s −
	// LimitReserveFraction × quota. The saturation latch fires when headroom
	// drops below 0 (sustained usage inside the reserve band). Initial value
	// 0.10; same calibrate-later class as cpuReserveCores.
	// TODO: calibrate.
	LimitReserveFraction float64
	// LimitReserveRecoverFraction is the Schmitt clear threshold for the
	// limit-mode headroom latch, as a fraction of the quota: the latch clears
	// when headroom > LimitReserveRecoverFraction × quota. Initial value 0.05.
	// TODO: calibrate.
	LimitReserveRecoverFraction float64
}

// DefaultThresholds returns the canonical thresholds (HighUsageFraction 0.70,
// SaturationRecover 0.60, ThrottleHigh 0.05, ThrottleRecover 0.03, PressureHigh
// 0.20, PressureRecover 0.12, StealHigh 0.10, StealRecover 0.06,
// LimitReserveFraction 0.10, LimitReserveRecoverFraction 0.05).
func DefaultThresholds() Thresholds {
	return Thresholds{
		HighUsageFraction:           0.70,
		SaturationRecover:           0.60,
		ThrottleHigh:                0.05,
		ThrottleRecover:             0.03,
		PressureHigh:                0.20,
		PressureRecover:             0.12,
		StealHigh:                   0.10,
		StealRecover:                0.06,
		LimitReserveFraction:        0.10,
		LimitReserveRecoverFraction: 0.05,
	}
}

// throttleWindow is the sliding window length over which the throttle ratio
// is computed.
const throttleWindow = 60 * time.Second

// stealWindow is the sliding window length over which the steal p95 is
// computed. It equals throttleWindow today; the consts stay separate so
// tuning one window cannot silently change the other.
const stealWindow = 60 * time.Second

// hostBusyWindow is the sliding window length over which the host-busy
// 60s mean (HostBusyCores60sMean) is computed. Separate from stealWindow
// so tuning the steal p95's window cannot silently shorten the host-busy
// mean, which feeds HeadroomCores in no-limit mode and drives the
// saturation latch.
const hostBusyWindow = 60 * time.Second

// usageWindow is the sliding window length over which the usage-ring
// 60s means (saturationAvg, usageCores60sMean) are computed. Separate
// from throttleWindow so the two horizons can be tuned independently.
const usageWindow = 60 * time.Second

// cpuReserveCores is the no-limit headroom reserve: one core set aside for
// Redpanda and system overhead, so HeadroomCores reflects capacity available
// to UMH workloads rather than raw free capacity. The saturation latch fires
// when HeadroomCores < 0 (less than one core free), so raising this value
// makes the latch fire earlier.
// TODO: calibrate against the idle+working distribution of real UMH hosts.
const cpuReserveCores = 1.0

// headroomRecoverCores is the Schmitt clear threshold for the headroom-based
// saturation trigger: the latch fires when HeadroomCores < 0, holds in
// [0, headroomRecoverCores], and clears only when HeadroomCores >
// headroomRecoverCores. It prevents a host parked on the line (headroom near
// 0) from dithering the latch on every tick.
const headroomRecoverCores = 0.5

// Sample is a single point-in-time CPU usage observation.
//
// Quota is the cgroup CPU quota in cores, as a pointer so nil ("no quota
// reading") is distinct from zero. When Quota is non-nil, Decide uses it
// exclusively: a positive value is the limit-mode ceiling and the usage
// fraction's denominator; a non-positive value (zero, negative, NaN) means
// uncapped. The zero case matters because readCPUMax returns &0 for
// cpu.max = "max <period>", and dividing by it would poison the fraction
// with +Inf/NaN. When Quota is nil, Decide falls back to a positive
// CgroupCores, else treats the sample as uncapped (healthy, no fraction).
//
// CgroupCores is the legacy non-pointer quota representation. The production
// sampler sets Quota, not CgroupCores; the fallback is kept for callers and
// tests that still populate it.
type Sample struct {
	Timestamp   time.Time
	Quota       *float64
	UsageCores  float64
	CgroupCores float64
	NrPeriods   int64
	NrThrottled int64
	// NrPeriodsAvailable is the readability flag for the throttle counters:
	// it distinguishes "cpu.stat read succeeded, container idle" from
	// "cpu.stat read failed" when NrPeriods is 0. When false, Decide holds
	// the throttle ring and latch instead of appending a zero point, which
	// would read as a counter regression and wipe the 60s window.
	NrPeriodsAvailable bool
	// PressureAvg60 is the kernel's cpu.pressure "some avg60" reading as a
	// FRACTION in [0,1], not the raw kernel percentage: /proc/pressure/cpu
	// reports avg60 in 0..100, so the reader MUST divide by 100 before
	// assigning this field (the pressure thresholds are fractions; a raw
	// percentage would fire at 0.2%, essentially always-on). Decide
	// thresholds it directly, with no extra windowing, because the kernel
	// already smoothed it over 60s. NaN, negative, and +Inf readings are
	// clamped to 0 before thresholding: NaN compares false against both
	// Schmitt marks, so an unclamped NaN would freeze the latch (a fired
	// cause could never self-clear), and a NaN or +Inf Cause Value breaks
	// JSON marshalling of the whole Verdict.
	PressureAvg60 float64
	// StealFraction is the fraction of wall-time (0.0-1.0) the hypervisor
	// gave this VM's vCPU to other VMs, from the 8th field of /proc/stat's
	// `cpu ` line. Only meaningful on a virtualized box; when Virtualized is
	// false Decide skips steal entirely.
	StealFraction float64
	// HostBusyCores is the host-level busy core count. The sampler computes
	// it from /proc/stat's non-idle fields EXCLUDING steal, guest, and
	// guest_nice, so steal (already its own cause) is not double-counted.
	// LogicalCpus is the host's total logical CPU count; the hostFull and
	// noLimitHost latches threshold on host headroom (LogicalCpus minus the
	// 60s hostBusyMean minus cpuReserveCores). Decide treats LogicalCpus <= 0
	// as no-signal and does not evaluate the latch, mirroring the
	// non-positive Quota / CgroupCores guards.
	HostBusyCores float64
	LogicalCpus   float64
	// Virtualized is set by the sampler from /proc/cpuinfo's hypervisor flag.
	// When false, steal is not a readable signal (it is structurally 0 on bare
	// metal, so reading 0 there is the absence of a signal, not evidence of a
	// healthy host).
	Virtualized bool
	// PsiAvailable reports that PSI (cpu.pressure) is present on this host.
	// It distinguishes "PSI present, reading 0" from "PressureAvg60 is 0
	// because PSI is absent". When PsiAvailable is false and no quota is
	// set, the sample is in the dead-zone, which drives LimitedVisibility.
	PsiAvailable bool
	// PsiReadable is set only on a clean cpu.pressure avg60 parse. Distinct
	// from PsiAvailable: a transient read failure leaves PsiReadable=false
	// even though PSI is known to exist on the host. When PsiReadable is
	// false, Decide holds the pressure latch instead of clearing it on
	// PressureAvg60=0, mirroring the hold-on-missing discipline on the
	// hostBusy and throttle rings.
	PsiReadable bool
	// HostBusyCoresAvailable is the readability flag for /proc/stat's
	// host-busy signal: it distinguishes "read succeeded, host idle" from
	// "/proc/stat unreadable" when HostBusyCores is 0. Set true by
	// readProcStat only on a successful read and parse.
	HostBusyCoresAvailable bool
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
	throttleRing               []throttlePoint
	stealRing                  []stealPoint
	usageRing                  []usagePoint
	hostBusyRing               []hostBusyPoint
	throttleFired              bool
	pressureFired              bool
	stealFired                 bool
	saturationFired            bool
	limitSaturationFired       bool
	hostFullFired              bool
	noHostStatsSaturationFired bool
	noLimitHostFired           bool
}

// stealPoint is one timestamped steal-fraction observation in the WindowState
// ring.
type stealPoint struct {
	ts    time.Time
	steal float64
}

// usagePoint is one timestamped usage-fraction observation in the WindowState
// usage ring, used by the no-host-stats saturation latch (NoHostStatsSaturationFired).
type usagePoint struct {
	ts       time.Time
	fraction float64
	// cores is the absolute UsageCores at this tick (clamped like fraction).
	// The percentile wire signals (AvgUsageFraction/P95/P99, mirrored to mCPU)
	// are computed from cores, not fraction: on a no-limit dead-zone box the
	// fraction has no denominator (CgroupCores 0) and collapses to 0, but the
	// box still uses real cores. fraction stays the saturation-latch input
	// (60s-avg fraction vs HighUsageFraction) in the cgroup-known sub-case.
	cores float64
}

// hostBusyPoint is one timestamped HostBusyCores observation in the WindowState
// hostBusyRing, used to compute Signals.HostBusyCores60sMean.
type hostBusyPoint struct {
	ts       time.Time
	hostBusy float64
}

// Signals holds derived intermediate values computed during Decide and
// returned alongside the Verdict: numeric metrics for the wire and the
// message, plus the per-latch Fired flags mirrored from WindowState.
// Signals is a read-only output; it carries no state between ticks.
type Signals struct {
	UsageFraction float64
	// ThrottleRatio is the 60s cumulative throttle ratio (nr_throttled delta
	// / nr_periods delta, oldest-to-newest over the pruned window). Populated
	// independent of the latch state so the metric stays observable on the
	// wire even when the latch has not fired; negatives are clamped to 0.
	ThrottleRatio float64
	// PressureAvg60Out is the clamped pressure avg60 value
	// (NaN/negative/+Inf clamped to 0). Like ThrottleRatio, populated
	// independent of the latch state.
	PressureAvg60Out float64
	// StealP95 is the 60s-windowed steal p95. Like ThrottleRatio, populated
	// independent of the latch state. It is 0 on a non-virtualized box and 0
	// until the ring holds at least 2 samples.
	StealP95 float64
	// AvgUsageFraction, P95UsageFraction, P99UsageFraction are the avg/p95/
	// p99 of the usage ring's fractions (relative to the quota; may exceed 1
	// under oversubscription). Observability-only: they do not change the
	// verdict, and the saturation latch fires on the avg, not the p95. 0
	// until the ring holds >= 2 entries.
	//
	// UsageRingActive is the fetchability flag for the percentile fields:
	// true once the usage ring holds >= 2 entries. Callers that mirror the
	// percentiles onto the wire use it to emit a real 0 versus omit an
	// absent signal, which a value-based 0/omitempty check cannot
	// distinguish.
	AvgUsageFraction float64
	P95UsageFraction float64
	P99UsageFraction float64
	// AvgUsageCores, P95UsageCores, P99UsageCores are the avg/p95/p99 of the
	// ABSOLUTE per-tick core usage over the usage ring, mirrored to mCPU
	// (*1000) on the wire. AvgUsageCores is computed once and read by both
	// the limit-mode headroom and the wire's avgMCpu, so the two cannot
	// diverge. Separate from the *Fraction fields because on a no-limit box
	// the quota-relative fraction collapses to 0 (no denominator) while the
	// box still uses real cores. 0 until the ring holds >= 2 entries.
	AvgUsageCores   float64
	P95UsageCores   float64
	P99UsageCores   float64
	UsageRingActive bool
	ThrottleFired   bool
	// PressureFired is the pressure Schmitt latch state (fires above
	// PressureHigh, clears only below PressureRecover), independent of the
	// throttle latch.
	PressureFired bool
	// StealFired is the steal Schmitt latch state (fires above StealHigh,
	// clears below StealRecover; holds between), independent of the
	// throttle/pressure latches.
	StealFired bool
	// HostContentionFired is reserved for a future host-contention cause.
	// Currently unused: Decide sets it to false unconditionally. A neighbor
	// filling the box is expressed as saturation (headroom < 0) plus
	// AttributionHost from the host/container split, so no separate
	// host-contention cause is emitted.
	HostContentionFired bool
	// LimitedVisibility is true when the sample is in the dead-zone (Quota nil
	// or non-positive and PsiAvailable false), a pure no-PSI/no-limit
	// annotation independent of virtualization. It is a signal for the caller's
	// message, NOT a state: the verdict State is binary healthy|degraded
	// (an unsaturated dead-zone sample is healthy). It is false outside the
	// dead-zone.
	LimitedVisibility bool
	// SaturationFired is the emitted saturation latch state: the OR of the
	// four internal sub-latches below (LimitSaturationFired, HostFullFired,
	// NoHostStatsSaturationFired, NoLimitHostFired). Each sub-latch is a
	// Schmitt trigger; see the Decide doc and the package doc for the
	// mode-aware trigger arms.
	SaturationFired bool
	// LimitSaturationFired is the limit-mode container-scope saturation
	// latch: the container's sustained usage is inside the quota's reserve
	// band. Exposed so the verdict-basis block and the message can rank the
	// firings. False in no-limit mode.
	LimitSaturationFired bool
	// HostFullFired is the limit-mode host-scope latch: the host itself is
	// full (LogicalCpus − hostBusyMean − cpuReserveCores < 0). A limit is a
	// ceiling, not a reservation, so a full host stacks as a cause on a
	// limited container. When both limit-mode latches fire, host-full
	// dominates (attribution host, cause Value = HostHeadroomCores). False
	// in no-limit mode; holds its prior state across a transient /proc/stat
	// outage and clears only on a confirmed-not-full reading or fresh state.
	HostFullFired bool
	// NoHostStatsSaturationFired is the no-limit fallback latch for when
	// /proc/stat is unreadable: it fires when usageCores60sMean/LogicalCpus
	// >= HighUsageFraction and clears below SaturationRecover. The
	// denominator is LogicalCpus, not Quota/CgroupCores (both 0 in no-limit
	// mode). False when host stats are readable (the host-headroom latch
	// covers that case) and in limit mode.
	NoHostStatsSaturationFired bool
	// NoLimitHostFired is the no-limit host-scope latch: host stats
	// readable and the host is full (LogicalCpus − hostBusyMean −
	// cpuReserveCores < 0). Exposed on the wire so MC can rank the firings
	// without inferring from ceiling+hostBusy.available. False in limit
	// mode and when host stats are unreadable.
	NoLimitHostFired bool
	// NoHostStatsSaturationFraction is usageCores60sMean/LogicalCpus, the
	// saturation cause Value when NoHostStatsSaturationFired is true; 0
	// otherwise. It deliberately does not use saturationAvg, whose
	// Quota/CgroupCores denominator is 0 in no-limit mode.
	NoHostStatsSaturationFraction float64
	// HostHeadroomCores is the host-scope headroom: LogicalCpus −
	// hostBusyMean − cpuReserveCores. In no-limit mode it equals
	// HeadroomCores; in limit mode it is the host-full latch's decision
	// variable and the saturation cause Value when host-full dominates. NOT
	// clamped: negative on a full host. Computed whenever LogicalCpus > 0.
	HostHeadroomCores float64
	// HostBusyCoresAvailable mirrors Sample.HostBusyCoresAvailable so the
	// message can gate its "host stats unavailable" note on the real
	// readability flag rather than a HostBusyCores60sMean==0 proxy, which
	// cannot tell an unreadable /proc/stat from a readable idle host.
	HostBusyCoresAvailable bool
	// HostBusyCores60sMean is the 60s arithmetic mean of per-tick
	// HostBusyCores; observability-only. The per-tick input is clamped
	// (NaN/negative/+Inf to 0) because a malformed value poisons the running
	// sum until it ages out.
	HostBusyCores60sMean float64
	// HeadroomCores is free capacity in cores: capacity minus measured use
	// minus reserve. Limit mode: quota − AvgUsageCores −
	// LimitReserveFraction×quota. No-limit mode: LogicalCpus −
	// HostBusyCores60sMean − cpuReserveCores. It is the saturation decision
	// variable (the latch fires below 0) and is NOT clamped: a full box
	// yields a negative number, not 0.
	HeadroomCores float64
	// CapacityCores is the headroom ceiling: the cgroup quota when set and
	// positive, else LogicalCpus. It is the "total cores" the healthy budget
	// message reports.
	CapacityCores float64
	// ReserveCores is the reserve Decide subtracted when computing
	// HeadroomCores: cpuReserveCores in no-limit mode,
	// LimitReserveFraction×quota in limit mode. Stamped into Signals so the
	// wire's verdict-basis block publishes the exact reserve the verdict
	// used.
	ReserveCores float64
	// LimitApplies is true when a CPU limit is set (Sample.Quota non-nil and
	// positive). The healthy budget message lists the throttle budget only
	// when it applies.
	LimitApplies bool
	// PsiApplies is true when PSI is present (Sample.PsiAvailable). The
	// healthy budget message lists the pressure budget only when it applies.
	PsiApplies bool
	// StealApplies is true when the box is virtualized
	// (Sample.Virtualized). The healthy budget message lists the steal
	// budget only when it applies.
	StealApplies bool
}

// Verdict is the output of Decide.
type Verdict struct {
	State       State
	Attribution Attribution
	Causes      []Cause
}

// Decide computes a CPU-health verdict from one sample and returns it with
// the derived Signals. It is the only mutator of st: it appends to the
// throttle, steal, usage, and hostBusy rings and updates the flip-latches in
// place, so callers must not share st across goroutines without external
// synchronization.
//
// The ceiling follows the CPU limit (see the package doc for the model).
// Limit mode (Quota non-nil and > 0) is evaluated first, so a limit-set
// sample never hits the no-limit branches: headroom is the quota minus the
// container's 60s-avg usage minus LimitReserveFraction×quota. No-limit mode
// measures the host instead: LogicalCpus minus the 60s host-busy mean minus
// cpuReserveCores. In either mode the saturation latch fires when headroom
// drops below 0; when no host stats are readable, the no-limit fallback
// fires on sustained container usage at or above HighUsageFraction of
// LogicalCpus. Below the fire condition the verdict is healthy.
//
// Every latch is a Schmitt trigger: it fires at its High mark, clears only
// below its Recover mark, and holds between them. Throttle latches on the
// 60s counter-delta ratio. Pressure thresholds sample.PressureAvg60 directly
// (the kernel already smoothed it over 60s), clamping NaN/negative/+Inf to 0
// first. Steal latches on the 60s nearest-rank p95 of StealFraction; it is
// processed only when sample.Virtualized is true, and only once the ring
// holds at least 2 samples, so a first-tick spike cannot fire.
//
// When several causes fire, Verdict.Causes is ordered dominant-first (see
// sortFiredCauses) and the dominant cause sets Attribution. Steal and a full
// host attribute to the host. Otherwise the 60s host/container split
// decides: AttributionHost when the non-container share of host-busy
// (HostBusyCores60sMean − AvgUsageCores) exceeds the container's own share,
// else AttributionUnknown. On a /proc/stat outage tick with a held
// NoLimitHostFired latch the split cannot run, so Attribution is forced to
// Host for that tick.
//
// Quota semantics (nil versus &0 versus positive) are documented on
// [Sample]. HostContentionFired is reserved and always false: a neighbor
// filling the box is expressed as saturation plus AttributionHost.
func Decide(st *WindowState, sample Sample, thresholds Thresholds) (Verdict, Signals) {
	var fraction float64

	if sample.Quota != nil {
		if *sample.Quota > 0 {
			fraction = sample.UsageCores / *sample.Quota
		}
	} else if sample.CgroupCores > 0 {
		// Retained: the production cgroup sampler sets Quota, not
		// CgroupCores, so this branch is unreachable from the sampler
		// path. Kept because tests pin the legacy CgroupCores fallback
		// contract (Sample{CgroupCores:X, Quota:nil} computes a fraction).
		fraction = sample.UsageCores / sample.CgroupCores
	}

	signals := Signals{UsageFraction: fraction}

	// Throttle: a 60s ring of counter samples reduced to a two-point delta
	// ratio, latched by a Schmitt trigger. A sample exactly at the prune
	// cutoff is kept (Before(cutoff) is false).
	var ratio float64
	// Clear the ring before appending when either counter regresses below
	// the ring's newest entry: a cgroup recreation (pod reschedule) resets
	// the counters, so pre-reset points are on a different baseline. Kept in
	// the ring, they would first force the ratio to 0 (non-positive period
	// delta) and then understate it (denominator inflated by pre-reset
	// periods), silently missing throttle for up to 60s during the
	// cold-start window where throttling is most likely. Clearing shrinks
	// that blind spot to about one tick.
	//
	// Both the clear and the append are gated on NrPeriodsAvailable: a
	// missing cpu.stat reading is a transient read failure, not a counter
	// regression. Appending its zero point would look like a regression and
	// wipe the window, clearing the latch for ~60s. Skipping it holds the
	// ring and the latch; the prune below still runs, so entries age out on
	// a sustained outage and the latch eventually clears.
	if sample.NrPeriodsAvailable {
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
	}

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

	// Pressure: the kernel already smooths avg60 over 60s, so it is
	// thresholded directly; the Schmitt latch is the only state.
	// NaN/negative/+Inf are clamped to 0 before thresholding and before
	// exposure as a Cause Value (see the PressureAvg60 doc for the failure
	// mode). The `!(p >= 0)` idiom catches NaN and negatives in one test;
	// IsInf catches +Inf, which `>= 0` would let through.
	p := sample.PressureAvg60
	if !(p >= 0) || math.IsInf(p, 1) {
		p = 0
	}

	signals.PressureAvg60Out = p
	// Hold-on-missing: when PsiReadable is false (transient cpu.pressure
	// read failure), skip the latch evaluation. The missing reading yields
	// PressureAvg60=0, which would fall below PressureRecover and flap a
	// previously-fired latch to healthy for the failure tick.
	if sample.PsiReadable {
		switch {
		case p > thresholds.PressureHigh:
			st.pressureFired = true
		case p < thresholds.PressureRecover:
			st.pressureFired = false
		}
	}

	signals.PressureFired = st.pressureFired

	// Steal (virtualized only): a 60s ring of per-tick StealFraction samples
	// reduced by nearest-rank p95, so a sustained spike fires while an
	// isolated one is absorbed, then Schmitt-latched. Skipped entirely on a
	// non-virtualized box: steal is structurally 0 on bare metal, so a 0
	// there is the absence of a signal, not evidence of a healthy host.
	var stealP95Val float64

	if sample.Virtualized {
		st.stealRing = append(st.stealRing, stealPoint{
			ts:    sample.Timestamp,
			steal: sample.StealFraction,
		})

		// Prune entries older than the steal window (a sample exactly at the
		// cutoff is kept): without pruning the ring grows unbounded and the
		// p95 becomes an all-history p95, not a 60s-windowed one.
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

		// Do not evaluate the steal latch until the ring holds 2 samples:
		// with one sample the nearest-rank p95 is that sample, so a
		// first-tick spike would fire immediately, contradicting "sustained
		// fires, isolated absorbed."
		if len(st.stealRing) >= 2 {
			stealP95Val = stealP95(st.stealRing)
			// Clamp NaN/negative/+Inf to 0 before thresholding and before
			// exposure as a Cause Value: a NaN p95 (malformed /proc/stat
			// parse) would freeze the latch (NaN compares false against both
			// marks), break json.Marshal of the whole Verdict, and corrupt
			// the severity sort comparator.
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

	// deadZone is the no-PSI, no-cgroup-limit state (independent of
	// virtualization); it drives LimitedVisibility. CauseKindHostContention
	// stays defined so the wire contract does not break, but Decide never
	// emits it: a neighbor filling the box is expressed as saturation plus
	// AttributionHost.
	deadZone := (sample.Quota == nil || !(*sample.Quota > 0)) && !sample.PsiAvailable

	signals.HostContentionFired = false

	// Host-busy 60s mean: a sliding ring of per-tick HostBusyCores exposed
	// as Signals.HostBusyCores60sMean, with a 2-sample floor emitting 0. The
	// input is clamped BEFORE insert (NaN/negative/+Inf to 0), unlike the
	// steal ring, which clamps its p95 after computation: a bad value in a
	// running mean poisons the sum until it ages out, while a p95 can be
	// clamped post-hoc.
	var hostBusyMean float64

	// Append only when /proc/stat was actually readable: a missing reading
	// is not a "host was idle" reading, and appending a 0 on an outage tick
	// dilutes the 60s mean, understating busyness once stats return (a
	// false-healthy flap). The attribution split below reads the 60s mean,
	// not this per-tick value, for label stability. The noLimitHost latch's
	// hold-on-missing complements this gate: the mean is not poisoned AND
	// the latch does not clear.
	hb := sample.HostBusyCores
	if !(hb >= 0) || math.IsInf(hb, 1) {
		hb = 0
	}

	if sample.HostBusyCoresAvailable {
		st.hostBusyRing = append(st.hostBusyRing, hostBusyPoint{
			ts:       sample.Timestamp,
			hostBusy: hb,
		})
	}

	hostBusyCutoff := sample.Timestamp.Add(-hostBusyWindow)
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

	// Usage ring (fills every tick, all modes): maintains the 60s-avg usage
	// fraction (saturationAvg) and the 60s-avg absolute core usage
	// (usageCores60sMean), so the limit-mode headroom and the wire's
	// AvgUsageCores are available everywhere. Inputs are clamped
	// (NaN/negative/+Inf to 0); a 2-sample floor emits 0 for both means.
	if !(fraction >= 0) || math.IsInf(fraction, 1) {
		fraction = 0
	}

	usageCores := sample.UsageCores
	if !(usageCores >= 0) || math.IsInf(usageCores, 1) {
		usageCores = 0
	}

	signals.UsageFraction = fraction
	st.usageRing = append(st.usageRing, usagePoint{
		ts:       sample.Timestamp,
		fraction: fraction,
		cores:    usageCores,
	})

	usageCutoff := sample.Timestamp.Add(-usageWindow)
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

	var saturationAvg float64

	var usageCores60sMean float64

	if len(st.usageRing) >= 2 {
		var fracSum float64

		var coreSum float64

		for _, up := range st.usageRing {
			fracSum += up.fraction
			coreSum += up.cores
		}

		saturationAvg = fracSum / float64(len(st.usageRing))
		usageCores60sMean = coreSum / float64(len(st.usageRing))
	}

	// Headroom (see Signals.HeadroomCores for the mode-aware formula). Both
	// means are 0 until their rings hold >= 2 entries, so a fresh state
	// reads capacity − 0 − reserve.
	limitMode := sample.Quota != nil && *sample.Quota > 0

	var capacity float64
	if limitMode {
		capacity = *sample.Quota
	} else {
		capacity = sample.LogicalCpus
	}

	if limitMode {
		quota := *sample.Quota
		reserve := thresholds.LimitReserveFraction * quota
		signals.HeadroomCores = capacity - usageCores60sMean - reserve
		signals.ReserveCores = reserve
	} else {
		signals.HeadroomCores = capacity - hostBusyMean - cpuReserveCores
		signals.ReserveCores = cpuReserveCores
	}

	// Host-scope headroom: computed whenever the host CPU count is known.
	// The host-full latch's decision variable in limit mode, and the
	// saturation cause Value when host-full dominates.
	hostHeadroomCores := sample.LogicalCpus - hostBusyMean - cpuReserveCores
	signals.HostHeadroomCores = hostHeadroomCores

	signals.CapacityCores = capacity
	signals.LimitApplies = limitMode
	signals.PsiApplies = sample.PsiAvailable
	signals.StealApplies = sample.Virtualized
	signals.HostBusyCoresAvailable = sample.HostBusyCoresAvailable

	signals.LimitedVisibility = deadZone

	// Saturation: mode-aware switch over the four sub-latches (see the
	// Signals docs for each latch's fire and clear marks). The limitMode
	// case MUST come first: a limit-set sample falling into the no-limit
	// branches would measure host busyness against the quota-sized capacity
	// and fire falsely. Below the fire condition the verdict is healthy.
	switch {
	case limitMode:
		// Container-scope latch.
		recoverCores := thresholds.LimitReserveRecoverFraction * *sample.Quota

		// No-limit sub-latches are inert in limit mode; clear them so a mode
		// transition does not leak a prior no-limit fire.
		st.noHostStatsSaturationFired = false
		st.noLimitHostFired = false

		if len(st.usageRing) >= 2 {
			switch {
			case signals.HeadroomCores < 0:
				st.limitSaturationFired = true
			case signals.HeadroomCores > recoverCores:
				st.limitSaturationFired = false
			}
		} else {
			st.limitSaturationFired = false
		}
		// Host-scope latch: the host itself is full. Evaluated only when
		// /proc/stat is readable; a limit is a ceiling, not a reservation,
		// so a full host stacks as a cause on a limited container. It
		// thresholds host headroom, never the quota. A transient /proc/stat
		// outage holds the prior fire; the latch clears only on a
		// confirmed-not-full reading or fresh state (hostBusyRing < 2).
		switch {
		case sample.HostBusyCoresAvailable && len(st.hostBusyRing) >= 2:
			switch {
			case hostHeadroomCores < 0:
				st.hostFullFired = true
			case hostHeadroomCores > headroomRecoverCores:
				st.hostFullFired = false
			}
		case sample.HostBusyCoresAvailable:
			st.hostFullFired = false
		default: // !HostBusyCoresAvailable: hold prior fire (no clear)
		}

		st.saturationFired = st.limitSaturationFired || st.hostFullFired
	case !limitMode && sample.HostBusyCoresAvailable && sample.LogicalCpus > 0:
		// No-limit, host stats readable: host-headroom latch, kept as its
		// own st.noLimitHostFired rather than a shared saturation flag. A
		// /proc/stat outage tick diverts to the no-host-stats branch, which
		// does not touch st.noLimitHostFired, so a prior host-headroom fire
		// holds until a successful reading clears it; with a single shared
		// latch, low container usage on the outage tick cleared it and
		// flapped degraded to healthy. The emitted saturationFired is the OR
		// of the sub-latches.
		st.limitSaturationFired = false
		st.hostFullFired = false
		st.noHostStatsSaturationFired = false

		if len(st.hostBusyRing) >= 2 {
			switch {
			case signals.HeadroomCores < 0:
				st.noLimitHostFired = true
			case signals.HeadroomCores > headroomRecoverCores:
				st.noLimitHostFired = false
			}
		} else {
			st.noLimitHostFired = false
		}

		st.saturationFired = st.noLimitHostFired || st.noHostStatsSaturationFired
	case !limitMode && !sample.HostBusyCoresAvailable && sample.LogicalCpus > 0:
		// No host stats, no limit: fraction fallback over LogicalCpus.
		// saturationAvg is not usable here (its Quota/CgroupCores denominator
		// is 0 in no-limit mode), so the fraction comes from
		// usageCores60sMean and LogicalCpus. This branch does not touch
		// st.noLimitHostFired, so a prior host-headroom fire from a readable
		// tick holds across the outage.
		st.limitSaturationFired = false
		st.hostFullFired = false

		if len(st.usageRing) >= 2 {
			noHostStatsSaturationFraction := usageCores60sMean / sample.LogicalCpus

			signals.NoHostStatsSaturationFraction = noHostStatsSaturationFraction
			switch {
			case noHostStatsSaturationFraction >= thresholds.HighUsageFraction:
				st.noHostStatsSaturationFired = true
			case noHostStatsSaturationFraction < thresholds.SaturationRecover:
				st.noHostStatsSaturationFired = false
			}
		} else {
			st.noHostStatsSaturationFired = false
		}

		st.saturationFired = st.noLimitHostFired || st.noHostStatsSaturationFired
	default:
		st.limitSaturationFired = false
		st.hostFullFired = false
		st.noLimitHostFired = false
		st.noHostStatsSaturationFired = false
		st.saturationFired = false
	}

	signals.SaturationFired = st.saturationFired
	signals.LimitSaturationFired = st.limitSaturationFired
	signals.HostFullFired = st.hostFullFired
	signals.NoHostStatsSaturationFired = st.noHostStatsSaturationFired
	signals.NoLimitHostFired = st.noLimitHostFired

	// Percentiles: reuse the already-computed means (do NOT recompute; the
	// limit-mode headroom and the wire must read the same average). p95/p99
	// are nearest-rank over the sorted ring. When the ring holds < 2
	// entries, everything stays 0 and UsageRingActive stays false.
	if n := len(st.usageRing); n >= 2 {
		signals.UsageRingActive = true
		signals.AvgUsageFraction = saturationAvg
		signals.AvgUsageCores = usageCores60sMean

		// Fraction-based p95/p99 (cgroup-relative, 0-1): nearest-rank from the
		// sorted ring. The saturation latch input and the "CPU averaged N%"
		// message number use the avg (above), not p95.
		vals := make([]float64, n)
		for i, up := range st.usageRing {
			vals[i] = up.fraction
		}

		sort.Float64s(vals)

		p95Rank := int(math.Ceil(0.95 * float64(n)))
		signals.P95UsageFraction = vals[p95Rank-1]
		p99Rank := int(math.Ceil(0.99 * float64(n)))
		signals.P99UsageFraction = vals[p99Rank-1]

		// Cores-based p95/p99 (absolute per-tick core usage): the wire's
		// p95/p99 mCPU rows. Mirrored to mCPU (*1000) by the caller.
		coreVals := make([]float64, n)
		for i, up := range st.usageRing {
			coreVals[i] = up.cores
		}

		sort.Float64s(coreVals)

		signals.P95UsageCores = coreVals[p95Rank-1]
		signals.P99UsageCores = coreVals[p99Rank-1]
	}

	var causes []Cause

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

	if signals.SaturationFired {
		// Cause Value: the negative headroom in cores ("0.5 cores over
		// capacity") whenever the host CPU count is known, so the detail
		// string is meaningful on every box where saturation fires. The
		// LogicalCpus <= 0 arm is defensive-dead (runtime.NumCPU is always
		// >= 1).
		satValue := signals.HeadroomCores
		switch {
		case sample.LogicalCpus <= 0:
			satValue = saturationAvg
		// HostFullFired is limit-mode-only and NoHostStatsSaturationFired is
		// no-limit-mode-only (each is cleared in the other mode's branch),
		// so the outage sub-case below cannot shadow the
		// NoHostStatsSaturationFired arm. The HostBusyCoresAvailable gate
		// keeps a held HostFullFired from selecting HostHeadroomCores during
		// a sustained /proc/stat outage, where the hostBusyRing ages out,
		// hostBusyMean drops to 0, and HostHeadroomCores reads positive.
		case signals.HostFullFired && sample.HostBusyCoresAvailable:
			satValue = signals.HostHeadroomCores
		case signals.HostFullFired && !sample.HostBusyCoresAvailable:
			satValue = 0
		case signals.NoHostStatsSaturationFired:
			satValue = signals.NoHostStatsSaturationFraction
		case signals.NoLimitHostFired && !sample.HostBusyCoresAvailable:
			satValue = 0
		}

		fired = append(fired, firedCause{Cause{Kind: CauseKindSaturation, Value: satValue}, severity(saturationAvg, thresholds.HighUsageFraction), false})
	}

	if len(fired) > 0 {
		sortFiredCauses(fired)

		causes = make([]Cause, len(fired))
		for i, fc := range fired {
			causes[i] = fc.cause
		}

		attr := AttributionUnknown
		// Steal always attributes to the host: the hypervisor took vCPU time
		// this instance needed, and the fix is host-side (guaranteed CPU for
		// the VM), even when the container's own workload is the majority of
		// host-busy. The cause list still surfaces the other causes; the
		// label names where to act. Without steal, the host/container split
		// in the default case decides.
		switch {
		case signals.StealFired:
			attr = AttributionHost
		case signals.HostFullFired:
			attr = AttributionHost
		case signals.NoLimitHostFired && !sample.HostBusyCoresAvailable:
			// Held NoLimitHostFired on a /proc/stat outage tick: the
			// host/container split cannot run (host-busy reads 0) and would
			// flip attribution to Unknown, so force Host for the outage
			// tick; the latched verdict is a host-side problem regardless of
			// a momentary outage. On readable ticks NoLimitHostFired falls
			// through to the default split, which can still yield Unknown
			// when the container itself is the dominant cause.
			attr = AttributionHost
		default:
			// Host/container split over the 60s means, NOT the per-tick
			// values: the label must be as stable as the Schmitt-latched
			// verdict it annotates. A per-tick split flips Host/Unknown on
			// every host-busy spike, and the MC renders a toggling
			// attribution badge over a stable degraded verdict. The means
			// are clamped like the ring inserts (NaN/negative/+Inf to 0).
			hbm := signals.HostBusyCores60sMean
			if !(hbm >= 0) || math.IsInf(hbm, 1) {
				hbm = 0
			}

			uc := signals.AvgUsageCores
			if !(uc >= 0) || math.IsInf(uc, 1) {
				uc = 0
			}

			if hbm-uc > uc {
				attr = AttributionHost
			}
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
// non-positive; the latter is a divide-guard for the ring-rebuilding case,
// since Decide already clears the ring on a counter regression. Decide clamps
// any residual negative ratio to 0 before exposing it on
// Signals.ThrottleRatio.
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

// stealP95 returns the nearest-rank p95 of the steal fractions in the ring:
// rank = ceil(0.95 * N), value at sorted[rank-1]. Returns 0 when the ring is
// empty. A single spike among 20 zeros yields 0 (absorbed); 20 identical
// samples yield that value, so a sustained spike fires.
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

// firedCause pairs a Cause with its computed severity and whether it is an
// external (host/hypervisor) signal, for dominant-first ordering.
type firedCause struct {
	cause    Cause
	severity float64
	external bool
}

// causeKindTier ranks the starvation causes (throttle, pressure, steal) above
// saturation: starvation means work is being delayed right now, while
// saturation is a capacity warning. Lower tier ranks first. host-contention
// is reserved (never emitted) and defaults to the starvation tier.
func causeKindTier(k CauseKind) int {
	if k == CauseKindSaturation {
		return 1
	}

	return 0
}

// sortFiredCauses orders the fired causes dominant-first: kind-tier first
// (starvation above saturation), then severity within a tier (higher first),
// then ties to the external (host) side. Mutates the slice in place. The
// kind-tier must come before severity: sorting on severity alone lets a
// high-severity saturation outrank a low-severity starvation cause,
// headlining capacity instead of the actionable starvation signal.
func sortFiredCauses(fired []firedCause) {
	sort.SliceStable(fired, func(i, j int) bool {
		ti, tj := causeKindTier(fired[i].cause.Kind), causeKindTier(fired[j].cause.Kind)
		if ti != tj {
			return ti < tj
		}

		if fired[i].severity != fired[j].severity {
			return fired[i].severity > fired[j].severity
		}

		// Ties go to the external side (host).
		return fired[i].external && !fired[j].external
	})
}

// severity returns the fraction into the danger band toward maximum: (value −
// high) / (1 − high). A value just above high yields ~0; a value at 1.0
// yields 1.0. This shared scale puts every cause on one axis for dominance
// comparison; high must be in [0, 1) for it to be finite, which
// DefaultThresholds honors.
//
// NaN-safety: any NaN/+Inf input, or a high >= 1.0, yields 0 (lowest
// severity), so a malformed reading or threshold never dominates a valid one
// and NaN never reaches the sort comparator.
//
// A HELD cause (latch fired, current reading below the High mark) yields a
// negative raw severity, clamped to 0 so held causes tie and the external
// tie-break decides deterministically; letting the least-negative held cause
// win would flap Attribution tick-to-tick as held readings jitter.
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
