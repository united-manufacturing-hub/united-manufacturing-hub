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
	"fmt"
	"math"
	"strconv"
	"strings"
)

// limitedVisibilityNote is the limited-visibility advisory appended to the
// healthy headline when the sample is in the dead-zone (no CPU limit, no PSI,
// not virtualized). It is the caller-facing "blind state" signal: the verdict
// State is binary healthy|degraded, so the blind state cannot be a state — it
// surfaces here as message text instead.
const limitedVisibilityNote = "Limited visibility: this instance has no CPU limit set and its operating system is not reporting CPU-pressure stats, so UMH cannot fully tell when work is waiting for a free core. Set a CPU limit or enable Linux pressure stats (boot with psi=1) to turn on full monitoring."

// ComposeMessage turns a Verdict and its derived Signals into the C2 two-layer
// Console message: a one-line headline naming the dominant cause, followed by
// the literal "Technical Details:" separator and the curated per-cause copy the
// alert adapter collapses into the expandable panel. When multiple causes fire,
// each cause's detail paragraph is listed in the Technical Details section
// (dominant first, separated by a blank line).
//
// A healthy verdict yields the budget dashboard: a headline stating how many
// cores the instance uses of its total and how much headroom remains before it
// is marked degraded, followed by a Technical Details line-up of ONLY the
// applicable alert-rule budgets (headroom always, then throttle/pressure/steal
// each only when its rule applies). The displayed headroom is computed from the
// already-rounded total/used/reserve so the printed arithmetic is exact by
// construction — the healthy message never renders an empty string (an empty
// headline renders blank in the Console). When the healthy sample is in the
// dead-zone (signals.LimitedVisibility), the limited-visibility advisory is
// inserted between the headline and Technical Details so the operator is told
// why starvation cannot be fully measured.
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

	return headline + "\nTechnical Details: " + details
}

// composeHealthy renders the two-layer healthy budget message. The displayed
// components are rounded first, then headroom is derived as
// total − used − reserve on those ALREADY-ROUNDED values, so the printed
// arithmetic in the Technical Details headroom line is exact by construction
// (never independently rounds signals.HeadroomCores). used/headroom/reserve
// print with one decimal; total prints as an integer when whole. The Technical
// Details dashboard lists only the applicable alert-rule budgets: headroom
// always, then throttle/pressure/steal each only when its rule applies.
func composeHealthy(signals Signals) string {
	// Zero-signals guard: when CapacityCores is 0 (cgroup read failure, Decide
	// never ran, signals zero-valued), do not compose the garbled "0.0 of 0
	// cores, -1.0 headroom" budget dashboard. Return a safe healthy string.
	if signals.CapacityCores == 0 {
		return "CPU status healthy; CPU usage is not available."
	}

	var usedDisp, reserveDisp float64
	if signals.LimitApplies {
		usedDisp = round1(signals.AvgUsageCores)
		reserveDisp = round1(signals.ReserveCores)
	} else {
		usedDisp = round1(signals.HostBusyCores60sMean)
		reserveDisp = round1(cpuReserveCores)
	}

	totalDisp := round1(signals.CapacityCores)
	headroomDisp := totalDisp - usedDisp - reserveDisp
	// Clean up floating-point residue from subtracting rounded values (e.g.
	// 2.0 - 1.8 - 0.2 yields -4.4e-16, not 0). This is not an independent
	// rounding of signals.HeadroomCores; it makes the by-construction exact
	// arithmetic actually exact in float64.
	headroomDisp = round1(headroomDisp)

	usedStr := fmtCores1(usedDisp)
	totalStr := fmtCoresTotal(totalDisp)
	headroomStr := fmtCores1(headroomDisp)
	reserveStr := fmtCores1(reserveDisp)

	var headline string

	// Sub-0.05-core quotas (e.g. a 40m Kubernetes limit = 0.04 cores) pass the
	// CapacityCores==0 guard above but collapse to totalDisp=0.0 after round1,
	// which would make usedDisp/totalDisp = +Inf and pctOf(+Inf) render a
	// garbled integer in the "(Z% of its limit)" suffix. When the rounded
	// total is 0, skip the percentage and render a no-suffix headline variant.
	totalTooSmallToPct := totalDisp <= 0

	if signals.LimitApplies && !totalTooSmallToPct {
		pctOfLimit := pctOf(usedDisp / totalDisp)
		if math.Abs(headroomDisp) < 0.05 {
			headline = fmt.Sprintf("CPU healthy. This instance is using %s of %s cores (%d%% of its limit) and is close to being marked degraded.", usedStr, totalStr, pctOfLimit)
		} else {
			headline = fmt.Sprintf("CPU healthy. This instance is using %s of %s cores (%d%% of its limit) and can use %s more before it is marked degraded.", usedStr, totalStr, pctOfLimit, headroomStr)
		}
	} else {
		// No-percentage variant: used either when no limit applies, or when the
		// limit is so small (sub-0.05 cores) that totalDisp rounds to 0 and the
		// percentage would divide by zero.
		if math.Abs(headroomDisp) < 0.05 {
			headline = fmt.Sprintf("CPU healthy. This instance is using %s of %s cores and is close to being marked degraded.", usedStr, totalStr)
		} else {
			headline = fmt.Sprintf("CPU healthy. This instance is using %s of %s cores and can use %s more before it is marked degraded.", usedStr, totalStr, headroomStr)
		}
	}

	msg := headline
	if signals.LimitedVisibility {
		msg += "\n" + limitedVisibilityNote
	}

	details := []string{
		fmt.Sprintf("Headroom %s cores = %s total - %s used - %s reserved (degraded below 0).", headroomStr, totalStr, usedStr, reserveStr),
	}
	if signals.LimitApplies {
		details = append(details, fmt.Sprintf("Throttling %d%% (degraded above 5%%).", pctOf(signals.ThrottleRatio)))
	}

	if signals.PsiApplies {
		details = append(details, fmt.Sprintf("Pressure %d%% (degraded above 20%%).", pctOf(signals.PressureAvg60Out)))
	}

	if signals.StealApplies {
		details = append(details, fmt.Sprintf("Steal %d%% (degraded above 10%%).", pctOf(signals.StealP95)))
	}

	return msg + "\nTechnical Details: " + strings.Join(details, " ")
}

// round1 rounds a cores value to one decimal place (half away from zero). It is
// the single rounding point for the healthy budget dashboard: total, used, and
// reserve are rounded once here, and headroom is derived from those rounded
// values so the printed arithmetic stays self-consistent.
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

// fmtCoresTotal formats the total-cores value as a whole integer when it has no
// fractional part (so an 8-core box reads "8", not "8.0"), else with one
// decimal. Its input is already round1'd, so it is a multiple of 0.1.
func fmtCoresTotal(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatInt(int64(v), 10)
	}

	return fmt.Sprintf("%.1f", v)
}

// causeHeadline returns the one-line Console headline naming a cause. The
// dominant cause's headline is the first line of ComposeMessage's two-layer
// format.
func causeHeadline(kind CauseKind) string {
	switch kind {
	case CauseKindThrottling:
		return "CPU limited"
	case CauseKindPressure:
		return "CPU contention"
	case CauseKindSteal:
		return "CPU taken by the server"
	case CauseKindSaturation:
		return "CPU running near full"
	default:
		return "CPU degraded"
	}
}

// causeDetails returns the curated Technical-Details copy for a single cause,
// interpolating the live number from the cause's Value or the derived Signals.
//
// Number sources per cause (see the 2026-06-18 supertable):
//   - throttling: signals.ThrottleRatio (60s cumulative ratio, 0..1)
//   - pressure:   cause.Value (PressureAvg60 carried on the Cause; the raw
//     signal lives on Sample, not Signals, so the Cause Value is the source)
//   - steal:      cause.Value (60s-windowed p95 of StealFraction, 0..1)
//   - saturation: signals.AvgUsageFraction (60s-average usage fraction)
func causeDetails(c Cause, signals Signals) string {
	switch c.Kind {
	case CauseKindThrottling:
		pct := pctOf(signals.ThrottleRatio)

		return fmt.Sprintf("This instance hit its CPU limit and was paused until the next cycle, in %d%% of CPU scheduling periods over the last minute. Work is being delayed. Raise this instance's CPU limit, or reduce the load on it.", pct)
	case CauseKindPressure:
		pct := pctOf(c.Value)

		return fmt.Sprintf("Tasks in this instance spent %d%% of the last minute waiting for a free CPU core. Reduce the load on this instance, or give it more CPU. If other workloads share this server they may be competing for it.", pct)
	case CauseKindSteal:
		pct := pctOf(c.Value)

		return fmt.Sprintf("Other virtual machines on the same physical server took CPU this instance needed, up to %d%% at peak over the last minute. This is outside UMH's control. On your virtualization platform, give this VM more guaranteed CPU, or reduce the other VMs sharing the server.", pct)
	case CauseKindSaturation:
		switch {
		case signals.HostFullFired:
			limitStr := fmtCoresTotal(round1(signals.CapacityCores))
			if signals.LimitSaturationFired {
				return fmt.Sprintf("The machine is full and this instance's CPU limit cannot help. Add CPU to the machine, or reduce other software running on it. (This instance is also at its %s-core limit.)", limitStr)
			}

			return "The machine is full. Add CPU to the machine, or reduce other software running on it."
		case signals.DRowFired:
			pct := pctOf(c.Value)

			return fmt.Sprintf("CPU averaged %d%% of the machine over the last minute and this instance has little headroom left. Host contention is not visible here (no CPU limit set, no pressure stats). Set a CPU limit or enable Linux pressure stats (boot with psi=1) for more detail. Consider adding CPU capacity.", pct)
		case signals.LimitSaturationFired:
			pct := pctOf(signals.AvgUsageCores / signals.CapacityCores)
			detail := fmt.Sprintf("CPU averaged %d%% of its limit over the last minute and this instance has little headroom left. Raise its CPU limit, or reduce the load to grow.", pct)
			// C-scenario honesty note: when host stats are unavailable in
			// limit mode (the sampler could not read /proc/stat), host-side
			// contention is invisible. Gate on the real readability flag, not
			// a HostBusyCores60sMean==0 proxy (which is unreliable on a
			// readable idle host).
			if !signals.HostBusyCoresAvailable {
				detail += " Host stats are unavailable, so host-side contention is not visible."
			}

			return detail
		default:
			// No-limit host-headroom (host stats readable, not D-row). The
			// percentage is host-busy vs host cores (LogicalCpus, surfaced as
			// CapacityCores in no-limit mode), NOT AvgUsageFraction (which is 0
			// in no-limit).
			pct := pctOf(signals.HostBusyCores60sMean / signals.CapacityCores)
			detail := fmt.Sprintf("CPU averaged %d%% of the machine over the last minute and this instance has little headroom left. Add CPU capacity, or reduce the load on it.", pct)
			// When PSI is also absent (limitedVisibility, the experiment case),
			// note that richer detail is available by enabling PSI.
			if signals.LimitedVisibility {
				detail += " Pressure stats are unavailable; enable Linux pressure stats (boot with psi=1) for richer detail."
			}

			return detail
		}
	default:
		return "CPU is degraded."
	}
}

// pctOf converts a 0..1 fraction to a rounded integer percentage. Values >1
// (oversubscription / multi-core busy) are preserved as >100 (the Linux CPU%
// convention), so a 3-core contention reads as 300%, not clamped to 100.
func pctOf(fraction float64) int {
	return int(math.Round(fraction * 100))
}

// BlockReason returns the per-cause bridge-block message shown when bridge
// creation is refused because the instance's CPU is degraded. The dominant
// cause kind selects the message; an unknown kind falls back to the generic
// degraded message.
func BlockReason(dominantKind CauseKind) string {
	switch dominantKind {
	case CauseKindThrottling:
		return "Can't add another bridge: this instance is already hitting its CPU limit. Raise the limit or reduce load first."
	case CauseKindPressure:
		return "Can't add another bridge: tasks on this instance are already waiting for a free CPU core. Reduce load, or give this instance more CPU, first."
	case CauseKindSteal:
		return "Can't add another bridge: the server isn't giving this instance enough CPU (other VMs are using it). Free up CPU on the server first."
	case CauseKindSaturation:
		return "Can't add another bridge: CPU has been running near full and we can't determine the cause. Add CPU capacity, or set a CPU limit, first."
	default:
		return "Can't add another bridge: CPU is degraded."
	}
}
