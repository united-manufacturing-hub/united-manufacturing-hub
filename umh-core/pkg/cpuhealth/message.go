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

// ComposeMessage turns a Verdict and its derived Details into the two-layer
// message: a one-line headline naming the dominant cause, then the literal
// Technical Details separator and the curated per-cause copy (dominant first,
// joined by a blank line). A healthy verdict yields the budget dashboard from
// composeHealthy.
//
// One paragraph per cause, except that the two capacity causes share one:
// speaksForCapacity says which of the pair writes it.
func ComposeMessage(verdict Verdict, signals Details) string {
	if verdict.State != StateDegraded || len(verdict.Causes) == 0 {
		return composeHealthy(signals)
	}

	headline := causeHeadline(verdict.Causes[0].Kind)

	parts := make([]string, 0, len(verdict.Causes))
	for _, c := range verdict.Causes {
		if !speaksForCapacity(c, verdict.Causes) {
			continue
		}
		parts = append(parts, causeDetails(c, verdict.Causes, verdict.Attribution, signals))
	}
	details := strings.Join(parts, "\n\n")

	return headline + technicalDetails + details
}

// composeHealthy renders the two-layer healthy budget message. The displayed
// components are rounded first, then headroom is derived as
// total - used - reserve on those ALREADY-ROUNDED values, so the printed
// arithmetic in the Technical Details headroom line is exact by construction
// (never independently rounds Details.HeadroomCores). used/headroom/reserve
// print with one decimal; total prints as an integer when whole. The
// Technical Details dashboard lists only the applicable alert-rule budgets:
// headroom always, then throttle/pressure/steal each only when its rule
// applies.
func composeHealthy(signals Details) string {
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

	// The display figures. ReserveCores is read from Details in both modes —
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
	case CauseKindHostCpuFull, CauseKindContainerLimitFull:
		return headlineSaturation
	default:
		return headlineGeneric
	}
}

// isCapacityKind reports whether a cause kind is one of the two the capacity
// signals produce: the machine is full, or this container is out of its own
// CPU limit.
func isCapacityKind(k CauseKind) bool {
	return k == CauseKindHostCpuFull || k == CauseKindContainerLimitFull
}

// hasKind reports whether the ranked causes hold one of this kind.
func hasKind(causes []Cause, kind CauseKind) bool {
	for _, c := range causes {
		if c.Kind == kind {
			return true
		}
	}

	return false
}

// machineWasRead reports whether the machine's free cores were read from
// /proc/stat this tick, rather than estimated from this container's own usage.
func machineWasRead(causes []Cause) bool {
	for _, c := range causes {
		if c.Kind == CauseKindHostCpuFull && c.Instrument == instHostHeadroom {
			return true
		}
	}

	return false
}

// speaksForCapacity reports whether this cause is the one the message speaks
// with. Every non-capacity cause speaks for itself; the two capacity causes
// share a single paragraph, because their remedies contradict each other and a
// customer given both is told to buy CPU for a machine and also to raise a
// limit that cannot help on it.
//
// Where /proc/stat answered, the machine's own reading speaks, and its
// paragraph names the container's limit in a trailing clause. Where the only
// reading of the machine is usage-fraction's estimate from our own usage, the
// container's own limit is the measured ceiling and speaks instead.
func speaksForCapacity(c Cause, causes []Cause) bool {
	if !isCapacityKind(c.Kind) {
		return true
	}
	if !hasKind(causes, CauseKindHostCpuFull) || !hasKind(causes, CauseKindContainerLimitFull) {
		return true
	}
	if machineWasRead(causes) {
		return c.Kind == CauseKindHostCpuFull
	}

	return c.Kind == CauseKindContainerLimitFull
}

// speakingCause returns the cause a single-sentence surface renders. It is the
// dominant one, unless the dominant one is the capacity cause that stays quiet,
// in which case it is its pair.
func speakingCause(causes []Cause) Cause {
	dominant := causes[0]
	if speaksForCapacity(dominant, causes) {
		return dominant
	}
	for _, c := range causes {
		if isCapacityKind(c.Kind) && speaksForCapacity(c, causes) {
			return c
		}
	}

	return dominant
}

// causeDetails returns the curated Technical-Details copy for one cause,
// interpolating the live number from the cause's Value or the derived Details.
// The two capacity kinds dispatch on the cause's own instrument, and a machine
// read full asks two further questions: whether the container is also at its
// limit, which is the case whose two remedies have to be blended into one
// sentence, and failing that whose load filled the machine, which
// fullMachineDetail answers.
func causeDetails(c Cause, causes []Cause, attribution Attribution, signals Details) string {
	switch c.Kind {
	case CauseKindThrottling:
		return fmt.Sprintf(detailThrottling, pctOf(signals.ThrottleRatio))
	case CauseKindPressure:
		return fmt.Sprintf(detailPressure, pctOf(c.Value))
	case CauseKindSteal:
		return fmt.Sprintf(detailSteal, pctOf(c.Value))
	case CauseKindContainerLimitFull:
		// CapacityCores == 0 replaces the percentage sentence — never
		// prefixes it — here and on the machine arm's readable branch below.
		// The host-unreadable clause still appends afterward, unaffected by
		// which sentence came before it.
		var detail string
		if signals.CapacityCores == 0 {
			detail = detailSatCapacityUnavailable
		} else {
			detail = fmt.Sprintf(detailSatLimit, pctOf(signals.AvgUsageCores/signals.CapacityCores))
		}
		if !signals.HostBusyCoresAvailable {
			detail += detailSatHostUnavail
		}

		return detail
	case CauseKindHostCpuFull:
		switch c.Instrument {
		case instUsageFraction:
			// The estimate from our own usage. The no-PSI wording is the one
			// that earns the pressure-stats advice.
			if signals.PressureApplies {
				return fmt.Sprintf(detailSatNoStatsPSI, pctOf(c.Value))
			}

			return fmt.Sprintf(detailSatNoStatsNoPSI, pctOf(c.Value))
		case instHostHeadroom:
			if hasKind(causes, CauseKindContainerLimitFull) {
				return fmt.Sprintf(detailSatBothAtLimit, fmtCoresTotal(round1(signals.CapacityCores)))
			}
			if signals.LimitApplies {
				return fullMachineDetail(attribution)
			}
			if !signals.HostBusyCoresAvailable {
				return detailSatNoLimitUnavail
			}
			var detail string
			if signals.CapacityCores == 0 {
				detail = detailSatCapacityUnavailable
			} else {
				detail = fmt.Sprintf(detailSatNoLimitRead, pctOf(signals.AvgHostBusyCores/signals.CapacityCores))
			}
			if signals.LimitedVisibility {
				detail += detailSatNoLimitClause
			}

			return detail
		}
	}

	return detailGeneric
}

// fullMachineDetail returns the machine-full paragraph for one attribution. The
// three differ in their advice, not only in their wording: "reduce other
// software running on it" sends the customer after somebody else's load, and it
// is wrong when the load filling the machine is this instance's own.
//
// fullMachineBlock answers the same question for the refusal line, and the two
// must move together.
func fullMachineDetail(attribution Attribution) string {
	switch attribution {
	case AttributionHost:
		return detailSatHostFull
	case AttributionContainer:
		return detailSatHostFullContainer
	default:
		return detailSatHostFullUnknown
	}
}

// fullMachineBlock returns the machine-full refusal line for one attribution,
// fullMachineDetail's pair.
func fullMachineBlock(attribution Attribution, signals Details) string {
	switch attribution {
	case AttributionContainer:
		return blockHostFullContainer
	case AttributionHost:
		if signals.LimitApplies {
			return blockHostFull
		}

		return blockNoLimitHost
	default:
		return blockHostFullUnknown
	}
}

// BlockReason returns the per-cause bridge-refusal message shown when bridge
// creation is refused because the instance's CPU is degraded. It speaks with
// the cause the Technical Details speak with, both through speakingCause, so
// the two surfaces cannot hand one customer two contradictory remedies at once.
// It reads the attribution for the same reason, and asks the ranked causes the
// same blend question first. An unknown kind falls back to the generic degraded
// message.
func BlockReason(causes []Cause, attribution Attribution, signals Details) string {
	if len(causes) == 0 {
		return blockPrefix + blockGeneric
	}

	c := speakingCause(causes)

	cause := blockGeneric
	switch c.Kind {
	case CauseKindThrottling:
		cause = blockThrottling
	case CauseKindPressure:
		cause = blockPressure
	case CauseKindSteal:
		cause = blockSteal
	case CauseKindContainerLimitFull:
		cause = blockLimitSaturation
	case CauseKindHostCpuFull:
		switch c.Instrument {
		case instHostHeadroom:
			// The order causeDetails takes: a container also at its limit is
			// answered there by one blended sentence, which carries the host
			// remedy, so the refusal shown beside it carries the host remedy
			// too.
			if hasKind(causes, CauseKindContainerLimitFull) {
				cause = blockHostFull
			} else {
				cause = fullMachineBlock(attribution, signals)
			}
		case instUsageFraction:
			cause = blockNoHostStats
		default:
			cause = blockSaturationOther
		}
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
	// ratio from Details; pressure and steal read Cause.Value (the raw PSI /
	// steal figure never reaches Details).
	detailThrottling = "This instance hit its CPU limit and was paused until the next cycle, in %d%% of CPU scheduling periods over the last minute. Work is being delayed. Raise this instance's CPU limit, or reduce the load on it."
	detailPressure   = "Tasks in this instance spent %d%% of the last minute waiting for a free CPU core. Reduce the load on this instance, or give it more CPU. If other workloads share this server they may be competing for it."
	detailSteal      = "Other virtual machines on the same physical server took %d%% of the CPU this instance needed over the last minute. This is outside UMH's control. On your virtualization platform, give this VM more guaranteed CPU, or reduce the other VMs sharing the server."

	// The capacity paragraphs. detailSatHostUnavail and
	// detailSatNoLimitClause are clauses, not paragraphs: each has a leading
	// space and is appended to the paragraph above it, never a replacement.
	//
	// The three detailSatHostFull* paragraphs are one arm read three ways, by
	// whose load filled the machine. They state the same fact and differ in the
	// remedy: only a machine filled from outside earns "reduce other software
	// running on it", and the container reading leads with the load the reader
	// controls.
	detailSatBothAtLimit       = "The machine is full and this instance's CPU limit cannot help. Add CPU to the machine, or reduce other software running on it. (This instance is also at its %s-core limit.)"
	detailSatHostFull          = "The machine is full. Add CPU to the machine, or reduce other software running on it."
	detailSatHostFullContainer = "The machine is full, and this instance is using most of it. Reduce the load on this instance, or add CPU to the machine."
	detailSatHostFullUnknown   = "The machine is full. Add CPU to the machine, or reduce what is running on it."
	detailSatNoStatsPSI        = "CPU averaged %d%% of the machine over the last minute and this instance has little headroom left. Host contention is not visible here (host CPU usage is not readable). Consider adding CPU capacity."
	detailSatNoStatsNoPSI      = "CPU averaged %d%% of the machine over the last minute and this instance has little headroom left. Host contention is not visible here (host CPU usage is not readable). Enable Linux pressure stats (boot with psi=1) for richer detail. Consider adding CPU capacity."
	detailSatLimit             = "CPU averaged %d%% of its limit over the last minute and this instance has little headroom left. Raise its CPU limit, or reduce the load on it."
	detailSatHostUnavail       = " Host stats are unavailable, so host-side contention is not visible."
	detailSatNoLimitUnavail    = "CPU is degraded. Host CPU usage is not readable right now (host stats temporarily unavailable), so the host-busy percentage cannot be shown. Add CPU capacity, or reduce the load on it."
	detailSatNoLimitRead       = "CPU averaged %d%% of the machine over the last minute and this instance has little headroom left. Add CPU capacity, or reduce the load on it."
	detailSatNoLimitClause     = " Pressure stats are unavailable; enable Linux pressure stats (boot with psi=1) for richer detail."

	// detailSatCapacityUnavailable is shared byte-for-byte between the limit
	// arm and the no-limit arm deliberately: a customer cannot use a wrong
	// percentage either way, and the remedy is the same.
	detailSatCapacityUnavailable = "CPU is degraded, but its usage percentage cannot be shown right now because this instance's CPU capacity is not currently readable. Add CPU capacity, or reduce the load on it."

	// The generic degraded paragraph.
	detailGeneric = "CPU is degraded."

	// The shared refusal-prefix, composed once at the point BlockReason returns.
	// Each block constant below carries only its own per-cause remainder.
	blockPrefix = "Can't add another bridge: "

	// The bridge-refusal (block) reasons, one per cause kind. The machine-full
	// kind is dispatched on the instrument that measured it, and its
	// host-headroom arm again on whose load filled the machine, so each line
	// carries the remedy of the paragraph it is shown beside.
	// blockHostFull and blockNoLimitHost are byte-identical and the collision is
	// deliberate: the remediation for a machine filled from outside is the same
	// with or without a limit, and giving each arm its own wording is a
	// behaviour change.
	blockThrottling        = "this instance is already hitting its CPU limit. Raise the limit or reduce load first."
	blockPressure          = "tasks on this instance are already waiting for a free CPU core. Reduce load, or give this instance more CPU, first."
	blockSteal             = "the server isn't giving this instance enough CPU (other VMs are using it). Free up CPU on the server first."
	blockHostFull          = "the machine is full. Add CPU to the machine, or reduce other software running on it, first."
	blockHostFullContainer = "the machine is full, and this instance is using most of it. Reduce the load on this instance, or add CPU to the machine, first."
	blockHostFullUnknown   = "the machine is full. Add CPU to the machine, or reduce what is running on it, first."
	blockLimitSaturation   = "this instance is at its CPU limit. Raise the limit, or reduce the load, first."
	blockNoHostStats       = "CPU is running near full and host stats are unavailable. Add CPU capacity, or set a CPU limit, first."
	blockNoLimitHost       = "the machine is full. Add CPU to the machine, or reduce other software running on it, first."
	blockSaturationOther   = "CPU is running near full. Add CPU capacity, or set a CPU limit, first."
	blockGeneric           = "CPU is degraded."
)
