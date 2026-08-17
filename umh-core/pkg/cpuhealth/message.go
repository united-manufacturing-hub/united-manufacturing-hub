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

// The customer-facing text: every sentence this package can print, and the
// rules that pick which one.
//
// Every customer-visible string in this file is written down exactly once —
// the constants below, plus the host-headroom sentence composeHealthy
// formats inline. A second copy of a sentence is where drift starts.

package cpuhealth

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ComposeMessage turns a Verdict and its derived Signals into the two-layer
// message: a one-line headline naming the dominant cause, then the literal
// Technical Details separator and the curated per-cause copy (dominant first,
// joined by a blank line). A healthy verdict yields the budget dashboard from
// composeHealthy.
func ComposeMessage(verdict Verdict, signals Signals) string {
	if verdict.State != StateDegraded || len(verdict.Causes) == 0 {
		return composeHealthy(signals)
	}

	dominant := verdict.Causes[0]
	headline := causeHeadline(dominant.Kind)

	parts := make([]string, 0, len(verdict.Causes))
	for _, c := range verdict.Causes {
		parts = append(parts, causeDetails(c, signals))
	}
	details := strings.Join(parts, "\n\n")

	return headline + technicalDetails + details
}

// composeHealthy renders the two-layer healthy budget message. The displayed
// components are rounded first, then headroom is derived as
// total - used - reserve on those ALREADY-ROUNDED values, so the printed
// arithmetic in the Technical Details headroom line is exact by construction
// (never independently rounds Signals.HeadroomCores). used/headroom/reserve
// print with one decimal; total prints as an integer when whole. The
// Technical Details dashboard lists only the applicable alert-rule budgets:
// headroom always, then throttle/pressure/steal each only when its rule
// applies.
func composeHealthy(signals Signals) string {
	// Zero-capacity guard: when CapacityCores is 0, do not compose the garbled
	// "0.0 of 0 cores, -1.0 headroom" budget dashboard. Return a safe string
	// that conveys monitoring-unavailability; the State on the wire stays
	// healthy per the binary contract. It is an early return, not a prefix.
	if signals.CapacityCores == 0 {
		return cpuMonitoringUnavailable
	}

	// The healthy message reports only what it measured. Two track floors
	// (one per mode) and one readability gate; when any withholds the usage
	// figure the message has no headline sentence left, so render the single
	// "CPU: starting up." line alone. It lasts one tick after each start and
	// respawn, and it is not the zero-capacity case (a standing state) — they
	// share a rendering path and nothing else.
	if signals.LimitApplies {
		if !signals.UsageRingActive {
			return cpuStartingUp
		}
	} else {
		if !signals.HostBusyRingActive || !signals.HostBusyCoresAvailable {
			return cpuStartingUp
		}
	}

	// The display figures. ReserveCores is read from Signals in both modes —
	// Decide filled it from the verdict's own reserve — never from the constant.
	var usedDisp, reserveDisp float64
	if signals.LimitApplies {
		usedDisp = round1(signals.AvgUsageCores)
	} else {
		usedDisp = round1(signals.AvgHostBusyCores)
	}
	reserveDisp = round1(signals.ReserveCores)
	totalDisp := round1(signals.CapacityCores)
	headroomDisp := round1(totalDisp - usedDisp - reserveDisp) // clears float residue, not an independent rounding

	usedStr := fmtCores1(usedDisp)
	totalStr := fmtCoresTotal(totalDisp)
	headroomStr := fmtCores1(headroomDisp)
	reserveStr := fmtCores1(reserveDisp)

	// A sub-0.05-core quota collapses to totalDisp == 0: suppress the
	// percentage (a division by a displayed zero) and use the no-percentage
	// variant. The subject follows the MODE, not the column: a tiny quota is
	// still limit mode, so it reads "This instance".
	totalTooSmallToPct := totalDisp <= 0

	var headline string
	if signals.LimitApplies && !totalTooSmallToPct {
		pctOfLimit := pctOf(usedDisp / totalDisp)
		if headroomDisp < 0.05 {
			headline = fmt.Sprintf(headlineLimitClose, usedStr, totalStr, pctOfLimit)
		} else {
			headline = fmt.Sprintf(headlineLimitMore, usedStr, totalStr, pctOfLimit, headroomStr)
		}
	} else {
		subject := subjectTheMachine
		if signals.LimitApplies {
			subject = subjectThisInstance
		}
		if headroomDisp < 0.05 {
			headline = fmt.Sprintf(headlineClose, subject, usedStr, totalStr)
		} else {
			headline = fmt.Sprintf(headlineMore, subject, usedStr, totalStr, headroomStr)
		}
	}

	msg := headline

	if signals.LimitedVisibility {
		msg += "\n" + limitedVisibilityNote
	}
	if !signals.HostHeadroomAvailable && signals.HostCpus > 0 {
		// The host-headroom sentence, with the two core counts substituted. The HostCpus >
		// 0 half is the ScopeUnknown case: on an unknown machine count the
		// bare float64 stays 0 and the sentence is withheld silently.
		msg += "\n" + fmt.Sprintf("host headroom unavailable: this container is pinned to %s of %s CPUs",
			fmtCoresTotal(signals.LogicalCpus), fmtCoresTotal(signals.HostCpus))
	}

	// The budget body. Headroom is unconditional; each of throttle, pressure
	// and steal prints only when THIS TICK'S reading is usable. The gates
	// are the per-signal readiness trio, never the capability flags — a
	// LimitApplies/PressureApplies/StealApplies build prints a confident 0% for a
	// reading that never happened.
	details := []string{
		fmt.Sprintf(headroomLine, headroomStr, totalStr, usedStr, reserveStr),
	}
	if signals.ThrottleSignalReady {
		details = append(details, fmt.Sprintf(throttleLine, pctOf(signals.ThrottleRatio)))
	}
	if signals.PressureSignalReady {
		details = append(details, fmt.Sprintf(pressureLine, pctOf(signals.PressureAvg60)))
	}
	if signals.StealSignalReady {
		details = append(details, fmt.Sprintf(stealLine, pctOf(signals.StealP95)))
	}

	return msg + technicalDetails + strings.Join(details, " ")
}

// causeHeadline returns the one-line headline naming a cause kind.
func causeHeadline(kind CauseKind) string {
	switch kind {
	case CauseKindThrottling:
		return headlineThrottling
	case CauseKindPressure:
		return headlinePressure
	case CauseKindSteal:
		return headlineSteal
	case CauseKindSaturation:
		return headlineSaturation
	default:
		return headlineGeneric
	}
}

// causeDetails returns the curated Technical-Details copy for one cause,
// interpolating the live number from the cause's Value or the derived Signals.
// The saturation switch reads the sub-latch flags directly. Its compound
// host-full-and-limit case comes first and has no counterpart in BlockReason;
// the four single-arm cases under it run in the precedence the two functions
// share. The compound NoLimitHostFired-with-host-busy-unreadable case runs
// before the default branch, so a readable no-limit full host falls to that
// default.
func causeDetails(c Cause, signals Signals) string {
	switch c.Kind {
	case CauseKindThrottling:
		return fmt.Sprintf(detailThrottling, pctOf(signals.ThrottleRatio))
	case CauseKindPressure:
		return fmt.Sprintf(detailPressure, pctOf(c.Value))
	case CauseKindSteal:
		return fmt.Sprintf(detailSteal, pctOf(c.Value))
	case CauseKindSaturation:
		switch {
		case signals.HostFullFired && signals.LimitSaturationFired:
			limitStr := fmtCoresTotal(round1(signals.CapacityCores))
			return fmt.Sprintf(detailSatBothAtLimit, limitStr)
		case signals.HostFullFired:
			return detailSatHostFull
		case signals.LimitSaturationFired:
			var detail string
			if signals.CapacityCores == 0 {
				detail = detailSatCapacityUnavailable
			} else {
				pct := pctOf(signals.AvgUsageCores / signals.CapacityCores)
				detail = fmt.Sprintf(detailSatLimit, pct)
			}
			if !signals.HostBusyCoresAvailable {
				detail += detailSatHostUnavail
			}
			return detail
		case signals.NoHostStatsSaturationFired:
			pct := pctOf(c.Value)
			if signals.PressureApplies {
				return fmt.Sprintf(detailSatNoStatsPSI, pct)
			}
			return fmt.Sprintf(detailSatNoStatsNoPSI, pct)
		case signals.NoLimitHostFired && !signals.HostBusyCoresAvailable:
			return detailSatNoLimitUnavail
		default:
			var detail string
			if signals.CapacityCores == 0 {
				detail = detailSatCapacityUnavailable
			} else {
				pct := pctOf(signals.AvgHostBusyCores / signals.CapacityCores)
				detail = fmt.Sprintf(detailSatNoLimitRead, pct)
			}
			if signals.LimitedVisibility {
				detail += detailSatNoLimitClause
			}
			return detail
		}
	default:
		return detailGeneric
	}
}

// BlockReason returns the per-cause bridge-refusal message shown when bridge
// creation is refused because the instance's CPU is degraded. The dominant
// cause kind selects the message; the saturation kind further dispatches on
// which sub-latch arm survived the fold, in the precedence this function shares
// with causeDetails: host-full, then limit, then no-host-stats, then
// no-limit-host. The two are one tick's two customer-facing views and must stay
// in step — the limit arm and the no-host-stats arm do co-fire, and an order
// that differs between them hands one customer two contradictory remedies at
// once. An unknown kind falls back to the generic degraded message.
func BlockReason(dominantKind CauseKind, signals Signals) string {
	var cause string
	switch dominantKind {
	case CauseKindThrottling:
		cause = blockThrottling
	case CauseKindPressure:
		cause = blockPressure
	case CauseKindSteal:
		cause = blockSteal
	case CauseKindSaturation:
		switch {
		case signals.HostFullFired:
			cause = blockHostFull
		case signals.LimitSaturationFired:
			cause = blockLimitSaturation
		case signals.NoHostStatsSaturationFired:
			cause = blockNoHostStats
		case signals.NoLimitHostFired:
			cause = blockNoLimitHost
		default:
			cause = blockSaturationOther
		}
	default:
		cause = blockGeneric
	}
	return blockPrefix + cause
}

// round1 rounds a cores value to one decimal place (half away from zero).
func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// fmtCores1 formats a cores value with one decimal place, normalizing a
// negative zero to a plain "0.0".
func fmtCores1(v float64) string {
	if v == 0 {
		v = 0
	}
	return fmt.Sprintf("%.1f", v)
}

// fmtCoresTotal formats an already-round1'd value: a whole number prints as
// an integer, else with one decimal.
func fmtCoresTotal(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatInt(int64(v), 10)
	}
	return fmt.Sprintf("%.1f", v)
}

// pctOf converts a 0..1 fraction to a rounded integer percentage. Values >1
// (oversubscription / multi-core busy) are preserved as >100 (the Linux CPU%
// convention), so a three-core contention reads as 300%, not clamped to 100.
func pctOf(fraction float64) int {
	return int(math.Round(fraction * 100))
}

const (
	// The one line rendered on a tick whose usage figure is withheld (window
	// below its floor, or the host reading absent): no headline, no advisory,
	// no technical details.
	//
	// Not cpuMonitoringUnavailable ("cgroup read failed"): on this tick the
	// read succeeded and there is simply one sample, so claiming a read failure
	// tells the customer a specific untrue thing.
	cpuStartingUp = "CPU: starting up."

	// Rendered alone when CapacityCores is 0; an early return, never a prefix.
	cpuMonitoringUnavailable = "CPU monitoring unavailable: cgroup read failed. Defaulting to healthy."

	// The limited-visibility advisory, an annotation on a healthy verdict in
	// the dead-zone, never a state.
	limitedVisibilityNote = "Limited visibility: this instance has no CPU limit set and its operating system is not reporting CPU-pressure stats, so UMH cannot fully tell when work is waiting for a free core. Set a CPU limit or enable Linux pressure stats (boot with psi=1) to turn on full monitoring."

	// The headline subjects in no-limit mode ("The machine") and limit mode
	// ("This instance").
	subjectThisInstance = "This instance"
	subjectTheMachine   = "The machine"

	// The limit-mode headlines.
	headlineLimitClose = "CPU healthy. This instance is using %s of %s cores (%d%% of its limit) and is close to being marked degraded."
	headlineLimitMore  = "CPU healthy. This instance is using %s of %s cores (%d%% of its limit) and can use %s more before it is marked degraded."

	// The no-percentage headlines, subject substituted.
	headlineClose = "CPU healthy. %s is using %s of %s cores and is close to being marked degraded."
	headlineMore  = "CPU healthy. %s is using %s of %s cores and can use %s more before it is marked degraded."

	// The unconditional headroom budget line.
	headroomLine = "Headroom %s cores = %s total - %s used - %s reserved (degraded below 0)."

	// The throttle/pressure/steal budget lines, each gated on that signal's
	// per-tick readiness.
	throttleLine = "Throttling %d%% (degraded above 5%%)."
	pressureLine = "Pressure %d%% (degraded above 20%%)."
	stealLine    = "Steal %d%% (degraded above 10%%)."

	// The separator, byte-identical on the healthy and the degraded path. The
	// literal "\nTechnical Details: ".
	technicalDetails = "\nTechnical Details: "

	// The degraded headlines, one per CauseKind. headlineGeneric is the default
	// arm, unreachable through today's five kinds but still written so the enum
	// can grow.
	headlineThrottling = "CPU limited"
	headlinePressure   = "CPU contention"
	headlineSteal      = "CPU taken by the server"
	headlineSaturation = "CPU running near full"
	headlineGeneric    = "CPU degraded"

	// The degradation detail paragraphs. Throttling reads the
	// ratio from Signals; pressure and steal read Cause.Value (the raw PSI /
	// steal figure never reaches Signals).
	detailThrottling = "This instance hit its CPU limit and was paused until the next cycle, in %d%% of CPU scheduling periods over the last minute. Work is being delayed. Raise this instance's CPU limit, or reduce the load on it."
	detailPressure   = "Tasks in this instance spent %d%% of the last minute waiting for a free CPU core. Reduce the load on this instance, or give it more CPU. If other workloads share this server they may be competing for it."
	detailSteal      = "Other virtual machines on the same physical server took CPU this instance needed, up to %d%% at peak over the last minute. This is outside UMH's control. On your virtualization platform, give this VM more guaranteed CPU, or reduce the other VMs sharing the server."

	// The saturation-family paragraphs. detailSatHostUnavail and
	// detailSatNoLimitClause are clauses, not paragraphs: each has a leading
	// space and is appended to the paragraph above it, never a replacement.
	detailSatBothAtLimit    = "The machine is full and this instance's CPU limit cannot help. Add CPU to the machine, or reduce other software running on it. (This instance is also at its %s-core limit.)"
	detailSatHostFull       = "The machine is full. Add CPU to the machine, or reduce other software running on it."
	detailSatNoStatsPSI     = "CPU averaged %d%% of the machine over the last minute and this instance has little headroom left. Host contention is not visible here (host CPU usage is not readable). Consider adding CPU capacity."
	detailSatNoStatsNoPSI   = "CPU averaged %d%% of the machine over the last minute and this instance has little headroom left. Host contention is not visible here (host CPU usage is not readable). Enable Linux pressure stats (boot with psi=1) for richer detail. Consider adding CPU capacity."
	detailSatLimit          = "CPU averaged %d%% of its limit over the last minute and this instance has little headroom left. Raise its CPU limit, or reduce the load on it."
	detailSatHostUnavail    = " Host stats are unavailable, so host-side contention is not visible."
	detailSatNoLimitUnavail = "CPU is degraded. Host CPU usage is not readable right now (host stats temporarily unavailable), so the host-busy percentage cannot be shown. Add CPU capacity, or reduce the load on it."
	detailSatNoLimitRead    = "CPU averaged %d%% of the machine over the last minute and this instance has little headroom left. Add CPU capacity, or reduce the load on it."
	detailSatNoLimitClause  = " Pressure stats are unavailable; enable Linux pressure stats (boot with psi=1) for richer detail."

	// detailSatCapacityUnavailable replaces the percentage sentence — never
	// prefixes it — on the limit arm and the default (readable no-limit) arm
	// when CapacityCores itself reads 0, the one input both arms' percentage
	// divides by. Shared byte-for-byte between the two arms deliberately: a
	// customer cannot use a wrong percentage either way, and the remedy is the
	// same. Its own clause (host-unavailable or no-pressure-stats) still
	// appends afterward, unaffected by which sentence came before it.
	detailSatCapacityUnavailable = "CPU is degraded, but its usage percentage cannot be shown right now because this instance's CPU capacity is not currently readable. Add CPU capacity, or reduce the load on it."

	// The generic degraded paragraph.
	detailGeneric = "CPU is degraded."

	// The shared refusal-prefix, composed once at the point BlockReason returns.
	// Each block constant below carries only its own per-cause remainder.
	blockPrefix = "Can't add another bridge: "

	// The bridge-refusal (block) reasons, one per cause kind, saturation
	// dispatched on the sub-latch arm in BlockReason's own order.
	// blockHostFull and blockNoLimitHost are byte-identical and the collision is
	// deliberate: the remediation for a full machine is the same with or without
	// a limit, and giving each arm its own wording is a behaviour change.
	// blockNoLimitHost does NOT distinguish "this instance itself filled the machine" from
	// "other software on the host did": that finer attribution needs per-process
	// (/proc/[pid]/stat) reads and is deferred to ENG-5264. Do not split its
	// wording now — it would pre-empt ENG-5264's clean divorce.
	blockThrottling      = "this instance is already hitting its CPU limit. Raise the limit or reduce load first."
	blockPressure        = "tasks on this instance are already waiting for a free CPU core. Reduce load, or give this instance more CPU, first."
	blockSteal           = "the server isn't giving this instance enough CPU (other VMs are using it). Free up CPU on the server first."
	blockHostFull        = "the machine is full. Add CPU to the machine, or reduce other software running on it, first."
	blockLimitSaturation = "this instance is at its CPU limit. Raise the limit, or reduce the load, first."
	blockNoHostStats     = "CPU is running near full and host stats are unavailable. Add CPU capacity, or set a CPU limit, first."
	blockNoLimitHost     = "the machine is full. Add CPU to the machine, or reduce other software running on it, first."
	blockSaturationOther = "CPU is running near full. Add CPU capacity, or set a CPU limit, first."
	blockGeneric         = "CPU is degraded."
)
