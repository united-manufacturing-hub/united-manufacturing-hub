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
	// LimitReserveFraction is the fractional reserve of the quota subtracted
	// alongside the container's 60s-avg usage when computing limit-mode
	// HeadroomCores (headroom = quota − containerUsage60s −
	// LimitReserveFraction × quota). The latch fires when headroom < 0, i.e.
	// when sustained usage enters the reserve band. Strawman 0.10 (10% of the
	// quota); same calibrate-later class as cpuReserveCores.
	// TODO: calibrate.
	LimitReserveFraction float64
	// LimitReserveRecoverFraction is the Schmitt clear threshold for the
	// limit-mode headroom latch, as a fraction of the quota. The latch clears
	// when headroom > LimitReserveRecoverFraction × quota (~half the reserve
	// above 0). Strawman 0.05.
	// TODO: calibrate.
	LimitReserveRecoverFraction float64
}

// DefaultThresholds returns the canonical thresholds (HighUsageFraction 0.70,
// SaturationRecover 0.60, ThrottleHigh 0.05, ThrottleRecover 0.03, PressureHigh
// 0.20, PressureRecover 0.12, StealHigh 0.10, StealRecover 0.06, HostBusyHigh
// 0.70, LimitReserveFraction 0.10, LimitReserveRecoverFraction 0.05).
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
		HostBusyHigh:                0.70,
		LimitReserveFraction:        0.10,
		LimitReserveRecoverFraction: 0.05,
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

// cpuReserveCores is the headroom reserve: one core set aside (for Redpanda
// and system overhead) when computing Signals.HeadroomCores so the number
// reflects capacity available to UMH workloads, not raw free capacity. The
// saturation latch fires when HeadroomCores < 0, i.e. when hostBusyMean >
// capacity - cpuReserveCores (less than one core free), so cpuReserveCores is
// the fire-sensitivity knob: raising it fires earlier (a fuller reserve
// demands more spare capacity before the box reads healthy). Calibrate it
// against the measured idle+working distribution of real UMH hosts before
// committing the threshold.
// TODO: calibrate.
const cpuReserveCores = 1.0

// headroomRecoverCores is the Schmitt clear threshold for the headroom-based
// saturation trigger: the latch fires when HeadroomCores < 0, holds in
// [0, headroomRecoverCores], and clears only when HeadroomCores >
// headroomRecoverCores. It prevents a host parked on the line (headroom near
// 0) from dithering the latch on every tick.
const headroomRecoverCores = 0.5

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
	// present but reading 0. When PsiAvailable is false (and Quota is nil or
	// non-positive) the sample is in the dead-zone — no PSI signal and no
	// cgroup limit, so sustained high usage is the last-resort proxy via the
	// saturation backstop latch.
	PsiAvailable bool
	// HostBusyCoresAvailable is the readability flag for /proc/stat's host-busy
	// signal. It distinguishes "we read /proc/stat, the host was idle"
	// (HostBusyCores == 0, true) from "we can't read /proc/stat at all"
	// (HostBusyCores == 0, false). Set true by readProcStat only when the
	// /proc/stat read+parse succeeded.
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
	throttleRing         []throttlePoint
	stealRing            []stealPoint
	usageRing            []usagePoint
	hostBusyRing         []hostBusyPoint
	throttleFired        bool
	pressureFired        bool
	stealFired           bool
	saturationFired      bool
	limitSaturationFired bool
	hostFullFired        bool
	dRowFired            bool
	noLimitHostFired     bool
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
	// of the usage ring — fractions relative to 1 core, typically in [0,1] but
	// may exceed 1 when observed usage exceeds the denominator (oversubscription);
	// callers that surface mCPU multiply by 1000. Computed UNCONDITIONALLY
	// whenever the ring holds >= 2 entries (observability-only, like
	// ThrottleRatio); 0 otherwise. They do NOT change the verdict — the
	// saturation latch still fires on the AVG, not p95. Since R10.1 the ring
	// fills every tick in ALL modes, so these are non-zero in every mode once
	// the window holds >= 2 entries (usage is a health signal in limit mode —
	// the container's own 60s-avg usage vs the quota — and an observability
	// signal in no-limit mode).
	//
	// UsageRingActive is the fetchability flag for the three percentile fields
	// above: true when the usage ring holds >= 2 entries, false otherwise (the
	// first tick of any mode before the ring has 2 entries). Callers that mirror
	// the percentiles onto a wire use this flag to decide whether to emit a 0
	// (fetchable) or omit (un-fetchable), instead of the value-based 0/omitempty
	// discipline that cannot distinguish a real 0 from an absent signal.
	AvgUsageFraction float64
	P95UsageFraction float64
	P99UsageFraction float64
	// AvgUsageCores, P95UsageCores, P99UsageCores are the avg/p95/p99 of the
	// ABSOLUTE per-tick core usage over the usage ring, mirrored to mCPU
	// (*1000) on the wire. AvgUsageCores is the "one blessed average" R10.1
	// reads identically for the limit-mode headroom `used` and the wire's
	// avgMCpu — computed once, reused (no divergence). Separate from the
	// *Fraction fields because on a no-limit box the cgroup-relative fraction
	// collapses to 0 (no denominator) while the box still uses real cores.
	// 0 until the ring holds >= 2 entries (gated by UsageRingActive).
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
	// HostContentionFired is the host-contention latch state. See the inline
	// comment at the latch in Decide for the fire/clear conditions.
	HostContentionFired bool
	// LimitedVisibility is true when the sample is in the dead-zone (Quota nil
	// or non-positive and PsiAvailable false) — a pure no-PSI/no-limit
	// annotation independent of virtualization. It is a signal for the caller's
	// message, NOT a state: the verdict State is binary healthy|degraded
	// (an unsaturated dead-zone sample is healthy). It is false outside the
	// dead-zone.
	LimitedVisibility bool
	// SaturationFired is the saturation latch state. The trigger is mode-aware
	// (R10.1, the two-rule model): in limit mode (Quota non-nil and > 0) the
	// latch fires when HeadroomCores < 0 (container usage inside the fractional
	// reserve band: usage > quota − LimitReserveFraction×quota) and clears when
	// HeadroomCores > LimitReserveRecoverFraction×quota (Schmitt); in no-limit
	// mode (host CPU count known, LogicalCpus > 0) it fires when HeadroomCores
	// < 0 (hostBusyMean > capacity − cpuReserveCores, less than one core free)
	// and clears when HeadroomCores > headroomRecoverCores (Schmitt); the
	// fraction fallback (dead-zone, LogicalCpus <= 0 — the cgroup-known-only
	// sub-case) fires when the 60s-average usage fraction >= HighUsageFraction
	// and clears below SaturationRecover. The limit-mode case is evaluated
	// first so a limit-set sample never hits the no-limit host-headroom branch.
	SaturationFired bool
	// LimitSaturationFired is the limit-mode container-scope saturation latch
	// (container usage inside the fractional reserve band). It is one of the two
	// internal latches whose OR is the emitted SaturationFired in limit mode;
	// exposed so the basis (R10.4) and message (R10.5) can rank the firings.
	// False in no-limit mode.
	LimitSaturationFired bool
	// HostFullFired is the limit-mode host-scope latch: the host itself is full
	// (LogicalCpus − hostBusyMean − cpuReserveCores < 0, readable via
	// HostBusyCoresAvailable). A limit is a ceiling not a reservation, so a full
	// host stacks as a cause on a limited container. When both latches fire,
	// host-full dominates (attribution host, cause Value = HostHeadroomCores).
	// False when host stats are unavailable (scenario C) and in no-limit mode.
	// R10.3: holds on a transient /proc/stat outage (Schmitt latch holds its
	// prior state on a missing reading; clears only on a confirmed-not-full
	// reading or fresh state).
	HostFullFired bool
	// DRowFired is the no-limit no-host-stats saturation latch (scenario D:
	// /proc/stat unreadable, no cgroup limit). R10.3 makes the fraction
	// fallback reachable by gating on HostBusyCoresAvailable (not LogicalCpus,
	// which is always > 0 via runtime.NumCPU) and rewrites the denominator to
	// LogicalCpus (not Quota/CgroupCores, both 0 in no-limit). The latch fires
	// when usageCores60sMean/LogicalCpus >= HighUsageFraction and clears below
	// SaturationRecover (Schmitt). False when host stats are readable (the
	// host-headroom latch handles it) or in limit mode.
	DRowFired bool
	// NoLimitHostFired is the no-limit host-stats-readable saturation latch
	// (scenario A degraded: the host itself is full — LogicalCpus −
	// hostBusyMean − cpuReserveCores < 0). It is the fourth internal sub-latch
	// whose OR (with limitSaturationFired/hostFullFired/dRowFired) is the
	// emitted SaturationFired. Exposed on the wire so MC can rank the firings
	// without inferring from ceiling+hostBusy.available (the wire contract
	// holds: fired == limitSaturationFired || hostFullFired || dRowFired ||
	// noLimitHostFired). False in limit mode and when host stats are unreadable.
	NoLimitHostFired bool
	// DFraction is the D-row fraction (usageCores60sMean/LogicalCpus, 0..1),
	// the saturation cause Value when DRowFired is true. Computed locally from
	// the one blessed average (usageCores60sMean) and LogicalCpus — NOT
	// saturationAvg (which divides by Quota/CgroupCores, both 0 in no-limit,
	// the bug R10.3 fixes). 0 when the D-row is not active.
	DFraction float64
	// HostHeadroomCores is the host-scope headroom: LogicalCpus − hostBusyMean
	// − cpuReserveCores. In no-limit mode it equals HeadroomCores (same formula);
	// in limit mode it is the host-full latch's decision variable and the
	// saturation cause Value when host-full dominates. NOT clamped (negative on
	// a full host). Computed whenever LogicalCpus > 0.
	HostHeadroomCores float64
	// HostBusyCoresAvailable mirrors Sample.HostBusyCoresAvailable (the
	// sampler's /proc/stat readability flag) onto Signals so the message
	// (R10.5) can gate the C-scenario "host stats unavailable" note on the real
	// flag, not a HostBusyCores60sMean==0 proxy (which is unreliable on a
	// readable idle host). It is signal plumbing, not a verdict input.
	HostBusyCoresAvailable bool
	// HostBusyCores60sMean is the 60s arithmetic mean of per-tick HostBusyCores;
	// observability-only (does not change the verdict). The per-tick input is
	// clamped (NaN/negative/+Inf → 0) because a malformed value poisons the
	// running sum until it ages out.
	HostBusyCores60sMean float64
	// HeadroomCores is the free-capacity-in-cores number: capacity minus
	// measured use minus the reserve. The inputs are mode-aware (R10.1):
	// limit mode (Quota non-nil and > 0) — capacity = Quota, measured use =
	// AvgUsageCores (the container's own 60s-avg usage), reserve =
	// LimitReserveFraction×Quota; no-limit mode — capacity = LogicalCpus,
	// measured use = HostBusyCores60sMean, reserve = cpuReserveCores (1.0).
	// It is the decision variable (latch fires at < 0) and is NOT clamped — a
	// full box yields a negative number, not 0.
	HeadroomCores float64
	// CapacityCores is the core budget used as the headroom denominator: the
	// cgroup Quota when set and positive, else Sample.LogicalCpus (the uncapped
	// host). It is the "total cores" the healthy budget message reports, and the
	// value HeadroomCores is derived from.
	CapacityCores float64
	// ReserveCores is the headroom reserve Decide subtracted alongside the
	// measured use to compute HeadroomCores. Mode-aware (R10.1): no-limit mode
	// uses cpuReserveCores (1.0 core); limit mode uses
	// LimitReserveFraction×Quota (strawman 0.10). Stamped into Signals so the
	// wire's verdict-basis block can publish the exact reserve the verdict used
	// (user-visible via the budget message, which raises the stakes on
	// calibrating both with fleet data; TODO calibrate).
	ReserveCores float64
	// LimitApplies is true when a CPU limit is set (Sample.Quota non-nil and
	// positive), i.e. the throttle rule is applicable to this box. The healthy
	// budget message lists the throttle budget only when it applies.
	LimitApplies bool
	// PsiApplies is true when PSI (cpu.pressure) is readable
	// (Sample.PsiAvailable), i.e. the pressure rule is applicable. The healthy
	// budget message lists the pressure budget only when it applies.
	PsiApplies bool
	// StealApplies is true when the box is virtualized (Sample.Virtualized),
	// i.e. the steal rule is applicable. The healthy budget message lists the
	// steal budget only when it applies.
	StealApplies bool
}

// Verdict is the output of Decide.
type Verdict struct {
	State       State
	Attribution Attribution
	Causes      []Cause
}

// Decide computes a CPU-health verdict from a sample using the two-rule model
// (R10.1): the ceiling headroom is measured against follows whether a CPU
// limit is set. Decide mutates st (*WindowState) in place — appending to the
// throttle, steal, usage, and hostBusy rings and updating the flip-latches —
// so the caller must not share st across goroutines without external
// synchronization. In limit mode (Quota non-nil and > 0) the ceiling is the
// quota, measured use is the container's own 60s-avg usage (AvgUsageCores),
// and the reserve is LimitReserveFraction×quota — headroom < 0 (sustained
// usage inside the fractional reserve band) degrades; in no-limit mode the
// ceiling is the host (LogicalCpus), measured use is hostBusyMean, and the
// reserve is cpuReserveCores (1.0) — headroom < 0 (less than one core free)
// degrades. Throttle stacks above limit-headroom exactly as PSI stacks above
// host-headroom (kind-tier cause sort, unchanged). Decide maintains a usage
// ring in WindowState that fills EVERY tick in ALL modes (one blessed 60s
// mean of the ring's cores samples, read identically by the limit-mode
// headroom, AvgUsageCores, and the wire's avgMCpu) and a 60s mean of
// HostBusyCores. The saturation latch trigger has three branches selected in
// order: limit mode (headroom < 0, Schmitt clears above
// LimitReserveRecoverFraction×quota); no-limit mode with the host CPU count
// known (LogicalCpus > 0 — headroom < 0, Schmitt clears above
// headroomRecoverCores), covering the Quota=&0 truly-uncapped case; and the
// fraction fallback (dead-zone, LogicalCpus <= 0 — 60s-avg usage fraction >=
// HighUsageFraction, Schmitt clears below SaturationRecover). Below the fire
// condition in any mode the verdict is healthy.
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
// Exception: when the dominant cause is internal AND SaturationFired is true
// (the dead-zone saturation backstop), attribution repivots to the
// host/container split — AttributionHost when the host (non-UMH) share
// (HostBusyCores - UsageCores) exceeds the UMH share (UsageCores), else
// AttributionUnknown. Outside the dead-zone saturation case, attribution still
// falls back to the dominant cause's external flag as above.
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

	// deadZone is the no-PSI, no-cgroup-limit state (independent of
	// virtualization since R4). It gates the saturation backstop below and
	// drives LimitedVisibility. R5 folded the host-contention cause into
	// saturation + the host/container attribution split: the
	// CauseKindHostContention constant stays defined (no hard wire break) but
	// Decide never emits it. A neighbour filling the box is now expressed as
	// saturation (headroom < 0) + AttributionHost from the host/container split.
	deadZone := (sample.Quota == nil || !(*sample.Quota > 0)) && !sample.PsiAvailable

	signals.HostContentionFired = false

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

	// hb is the clamped per-tick HostBusyCores, used both for the ring insert
	// (when readable) and the attribution host/container split below. R10.3:
	// only append to the hostBusyRing when /proc/stat was actually readable
	// (HostBusyCoresAvailable). A missing reading is NOT a "host was idle"
	// reading — appending HostBusyCores=0 on an outage tick poisons the 60s
	// mean and corrupts the host-headroom computation when stats return (the
	// diluted mean understates busyness → false-healthy flap). Skipping the
	// append keeps the mean over real readings only; the 2-sample floor and 60s
	// prune still hold. The latch hold-on-missing (the noLimitHost latch holds
	// on !HostBusyCoresAvailable) complements this: the mean is not poisoned
	// AND the latch does not clear.
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

	// Usage ring (unconditional, all modes): maintains the 60s-avg usage
	// fraction (saturationAvg) AND the 60s-avg absolute core usage
	// (usageCores60sMean) over a ring in WindowState. The ring fills EVERY
	// tick in ALL modes (the fill-gate moved out of the dead-zone block) so
	// the limit-mode headroom and the wire's AvgUsageCores are available
	// everywhere. Input clamps on fraction and usageCores stay (NaN/+Inf/
	// negative → 0). A 2-sample floor emits 0 for both means when the ring
	// holds < 2 entries.
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

	// Headroom: mode-aware. In limit mode (Quota non-nil and > 0) the ceiling
	// is the quota, the measured use is the container's own 60s-avg usage
	// (usageCores60sMean), and the reserve is a FRACTION of the quota
	// (LimitReserveFraction). In no-limit mode the ceiling is LogicalCpus, the
	// measured use is hostBusyMean, and the reserve is cpuReserveCores (1.0).
	// Not clamped: a full box yields a negative number. Both means are 0 until
	// their respective rings hold >= 2 entries, so headroom reads
	// capacity − 0 − reserve on a fresh state, matching the floor.
	limitMode := sample.Quota != nil && *sample.Quota > 0

	var capacity float64
	if sample.Quota != nil && *sample.Quota > 0 {
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

	// Host-scope headroom (the rule-1 test at host scope): always computed when
	// the host CPU count is known, used by the host-full latch in limit mode and
	// as the saturation cause Value when host-full dominates. In no-limit mode
	// it equals HeadroomCores (same formula).
	hostHeadroomCores := sample.LogicalCpus - hostBusyMean - cpuReserveCores
	signals.HostHeadroomCores = hostHeadroomCores

	signals.CapacityCores = capacity
	signals.LimitApplies = limitMode
	signals.PsiApplies = sample.PsiAvailable
	signals.StealApplies = sample.Virtualized
	signals.HostBusyCoresAvailable = sample.HostBusyCoresAvailable

	signals.LimitedVisibility = deadZone

	// Saturation latch: mode-aware switch. In limit mode the latch fires on
	// limit-headroom < 0 (container usage inside the fractional reserve band)
	// and clears at headroom > LimitReserveRecoverFraction × quota. In
	// no-limit mode the branch is gated on HostBusyCoresAvailable (R10.3):
	// when /proc/stat is readable, the host-headroom latch fires on
	// HeadroomCores < 0 and clears above headroomRecoverCores (unchanged);
	// when /proc/stat is unreadable AND no limit, the D-row fraction fallback
	// fires on usageCores60sMean/LogicalCpus >= HighUsageFraction and clears
	// below SaturationRecover (REVISED from v3's "uncapped → healthy"). The
	// old fraction branch (deadZone && LogicalCpus <= 0) is removed — it was
	// doubly dead (unreachable AND fraction=0 in no-limit). The limitMode case
	// MUST come first so limit-set samples never hit the no-limit branches
	// (which would re-introduce the B false fire). Below the fire condition the
	// verdict is healthy.
	switch {
	case limitMode:
		// Container-scope latch (R10.1's limit latch, now split out).
		recoverCores := thresholds.LimitReserveRecoverFraction * *sample.Quota

		// No-limit sub-latches are inert in limit mode; clear them so a mode
		// transition does not leak a prior no-limit fire (R10.3 Finding 2).
		st.dRowFired = false
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
		// Host-scope latch: the host itself is full (rule-1 test at host scope).
		// Only when /proc/stat is readable (HostBusyCoresAvailable); a limit is
		// a ceiling not a reservation, so a full host stacks on a limited
		// container. NEVER quota-scoped (that was the B false fire). R10.3:
		// hold-on-uncertain-read — a transient /proc/stat outage holds the
		// prior fire (Schmitt latch holds on a missing reading); clears only on
		// a confirmed-not-full reading or fresh state (hostBusyRing < 2).
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
		// No-limit, host stats readable: host-headroom latch. R10.3 splits this
		// into its own st.noLimitHostFired latch (mirroring limit mode's
		// split) so the D-row branch on a /proc/stat outage tick cannot clobber
		// a prior host-headroom fire (Finding 1: the gate alone was not enough
		// — the D-row wrote the shared st.saturationFired and cleared it on low
		// container usage, flapping degraded→healthy). The D-row latch holds
		// its own state independently; the emitted saturationFired is their OR.
		// Hold-on-missing: a transient /proc/stat outage diverts to the D-row
		// branch, which does NOT touch st.noLimitHostFired, so a prior
		// host-headroom fire holds until a successful reading clears it.
		st.limitSaturationFired = false
		st.hostFullFired = false
		st.dRowFired = false

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

		st.saturationFired = st.noLimitHostFired || st.dRowFired
	case !limitMode && !sample.HostBusyCoresAvailable && sample.LogicalCpus > 0:
		// D-row: no host stats, no limit. Fraction fallback with LogicalCpus
		// denominator (REVISED from v3's "uncapped → healthy"). The usage ring
		// fills every tick (R10.1), so usageCores60sMean/LogicalCpus is
		// computable. saturationAvg (Quota/CgroupCores denominator) is NOT used
		// — it's 0 in no-limit mode (the bug). dFraction is computed locally
		// from the one blessed average (usageCores60sMean) and LogicalCpus.
		// R10.3 Finding 1: this branch does NOT touch st.noLimitHostFired, so a
		// prior host-headroom fire (from a readable tick) holds across the
		// outage — the D-row evaluates its own st.dRowFired independently and
		// the emitted saturationFired is their OR (no flap).
		st.limitSaturationFired = false
		st.hostFullFired = false

		if len(st.usageRing) >= 2 {
			dFraction := usageCores60sMean / sample.LogicalCpus

			signals.DFraction = dFraction
			switch {
			case dFraction >= thresholds.HighUsageFraction:
				st.dRowFired = true
			case dFraction < thresholds.SaturationRecover:
				st.dRowFired = false
			}
		} else {
			st.dRowFired = false
		}

		st.saturationFired = st.noLimitHostFired || st.dRowFired
	default:
		st.limitSaturationFired = false
		st.hostFullFired = false
		st.noLimitHostFired = false
		st.dRowFired = false
		st.saturationFired = false
	}

	signals.SaturationFired = st.saturationFired
	signals.LimitSaturationFired = st.limitSaturationFired
	signals.HostFullFired = st.hostFullFired
	signals.DRowFired = st.dRowFired
	signals.NoLimitHostFired = st.noLimitHostFired

	// Percentile block: set UsageRingActive, AvgUsageFraction, AvgUsageCores
	// from the already-computed early values (one blessed average — do NOT
	// recompute the mean). p95/p99 still sort+nearest-rank from the ring.
	// When len < 2, leave all at 0 and UsageRingActive false (as today).
	// Because the ring now fills every tick, these signals become non-zero in
	// ALL modes once 2 samples are in the window — this is intended (unblocks
	// the display's "this container" row outside the dead-zone).
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
		// Cause Value: negative headroom in cores ("0.5 cores over capacity")
		// when the host CPU count is known (LogicalCpus > 0 — the headroom
		// path), so the detail string is meaningful on every box where
		// saturation fires (including full+PSI/full+capped under option B,
		// where saturationAvg is 0 because the usage ring is dead-zone-only).
		// For the cgroup-known-only fraction fallback (LogicalCpus <= 0),
		// headroom can't be computed (no host count), so fall back to the
		// 60s-avg usage fraction as the Value.
		satValue := signals.HeadroomCores
		switch {
		case sample.LogicalCpus <= 0:
			satValue = saturationAvg
		case signals.HostFullFired:
			satValue = signals.HostHeadroomCores
		case signals.DRowFired:
			satValue = signals.DFraction
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
		// Attribution (R5 fold): steal is a host/hypervisor signal — the
		// hypervisor took vCPU time this instance needed, and the fix is always
		// host-side (give the VM real/guaranteed CPU, stop the steal). So
		// whenever steal fires, attribution is Host, regardless of whether our
		// own workload is also the majority of host-busy (the cause list still
		// surfaces the other causes; the label names where to act). When no
		// steal is present, attribution comes from the host/container split:
		// Host when the host (non-UMH) share exceeds the UMH share, else
		// Unknown. Clamp UsageCores the same way HostBusyCores (hb) is clamped
		// at the ring insert (NaN/negative/+Inf -> 0).
		switch {
		case signals.StealFired:
			attr = AttributionHost
		case signals.HostFullFired:
			attr = AttributionHost
		default:
			uc := sample.UsageCores
			if !(uc >= 0) || math.IsInf(uc, 1) {
				uc = 0
			}

			if hb-uc > uc {
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
// firedCause pairs a Cause with its computed severity and whether it is an
// external (host/hypervisor) signal, for dominant-first ordering.
type firedCause struct {
	cause    Cause
	severity float64
	external bool
}

// causeKindTier ranks starvation causes (throttle/pressure/steal) above
// saturation (capacity), per the spec's kind-priority ("throttle/pressure/
// steal — the serious signals — rank above saturation/no-headroom —
// capacity", spec line 126). Lower tier = higher priority (ranks first).
// host-contention is scaffolded (never in `fired` after the R5 fold) and
// defaults to the starvation tier.
func causeKindTier(k CauseKind) int {
	if k == CauseKindSaturation {
		return 1
	}

	return 0
}

// sortFiredCauses orders the fired causes dominant-first: kind-tier first
// (starvation above saturation), severity as a tiebreaker within a tier
// (higher severity first), then ties to the external (host) side. Mutates the
// slice in place. The pre-R4b sort was severity-magnitude only, which
// accidentally satisfied the kind-priority when saturation's severity was 0
// (outside the dead-zone) but deviated in the dead-zone where saturationAvg
// is non-zero: a low-severity starvation cause would rank below a
// high-severity saturation, headlining capacity instead of the actionable
// starvation signal.
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
