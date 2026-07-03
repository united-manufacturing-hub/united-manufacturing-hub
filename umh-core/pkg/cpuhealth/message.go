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
const limitedVisibilityNote = "Limited visibility: this instance has no CPU limit set and its operating system is not reporting CPU-pressure stats, so we cannot fully tell when work is waiting for a free core. Set a CPU limit or enable Linux pressure stats (boot with psi=1) to turn on full monitoring."

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
	usedDisp := round1(signals.HostBusyCores60sMean)
	totalDisp := round1(signals.CapacityCores)
	reserveDisp := round1(cpuReserveCores)
	headroomDisp := totalDisp - usedDisp - reserveDisp

	usedStr := fmtCores1(usedDisp)
	totalStr := fmtCoresTotal(totalDisp)
	headroomStr := fmtCores1(headroomDisp)
	reserveStr := fmtCores1(reserveDisp)

	var headline string
	if math.Abs(headroomDisp) < 0.05 {
		headline = fmt.Sprintf("CPU healthy. This instance is using %s of %s cores and is close to being marked degraded.", usedStr, totalStr)
	} else {
		headline = fmt.Sprintf("CPU healthy. This instance is using %s of %s cores and can use %s more before it is marked degraded.", usedStr, totalStr, headroomStr)
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

		return fmt.Sprintf("Other virtual machines on the same physical server took CPU this instance needed — %d%% of the last minute. This is outside UMH's control. On your virtualization platform, give this VM more guaranteed CPU, or reduce the other VMs sharing the server.", pct)
	case CauseKindSaturation:
		pct := pctOf(signals.AvgUsageFraction)
		if signals.LimitedVisibility {
			return fmt.Sprintf("CPU averaged %d%% over the last minute, with little headroom left. This instance has no CPU limit set and its operating system is not reporting CPU-pressure stats, so we cannot confirm whether work is waiting for a free core. Set a CPU limit, which also lets us measure that wait directly, or enable Linux pressure stats (boot with psi=1). Consider adding CPU capacity.", pct)
		}

		return fmt.Sprintf("CPU averaged %d%% over the last minute and this instance has little headroom left. Add CPU capacity, or raise its CPU limit if one is set and this load is expected.", pct)
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
