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

// Every customer-visible string in this file is a numbered entry in
// STRING_INVENTORY.md, by entry number (never retyped), plus the two §5
// sentences (F6, F3) that are not inventory entries and are copied from §5.
// The inventory is normative on the text; a second full copy is where drift
// starts.

const (
	// §5 F3 (D-20) — the one line rendered on a tick whose usage figure is
	// withheld (window below its floor, or the host reading absent): no
	// headline, no advisory, no technical details. It is one of the two stack
	// sentences not in the inventory, copied from §5.
	//
	// Not entry 7 ("cgroup read failed"): on this tick the read succeeded and
	// there is simply one sample, so claiming a read failure is F1 in
	// customer-facing prose.
	cpuStartingUp = "CPU: starting up."

	// Entry 7 — rendered alone when CapacityCores is 0; an early return, never
	// a prefix.
	cpuMonitoringUnavailable = "CPU monitoring unavailable: cgroup read failed. Defaulting to healthy."

	// Entry 5 — the limited-visibility advisory, an annotation on a healthy
	// verdict in the dead-zone, never a state.
	limitedVisibilityNote = "Limited visibility: this instance has no CPU limit set and its operating system is not reporting CPU-pressure stats, so UMH cannot fully tell when work is waiting for a free core. Set a CPU limit or enable Linux pressure stats (boot with psi=1) to turn on full monitoring."

	// Entry 11 / Entry 10 — the headline subjects in no-limit mode ("The
	// machine") and limit mode ("This instance").
	subjectThisInstance = "This instance"
	subjectTheMachine   = "The machine"

	// Entries 8 and 9 — the limit-mode headlines.
	headlineLimitClose = "CPU healthy. This instance is using %s of %s cores (%d%% of its limit) and is close to being marked degraded."
	headlineLimitMore  = "CPU healthy. This instance is using %s of %s cores (%d%% of its limit) and can use %s more before it is marked degraded."

	// Entries 12 and 13 — the no-percentage headlines, subject substituted.
	headlineClose = "CPU healthy. %s is using %s of %s cores and is close to being marked degraded."
	headlineMore  = "CPU healthy. %s is using %s of %s cores and can use %s more before it is marked degraded."

	// Entry 14 — the unconditional headroom budget line.
	headroomLine = "Headroom %s cores = %s total - %s used - %s reserved (degraded below 0)."

	// Entries 15–17 — the throttle/pressure/steal budget lines (R3 gates
	// these on per-signal readiness; R1 uses the capability flags).
	throttleLine = "Throttling %d%% (degraded above 5%%)."
	pressureLine = "Pressure %d%% (degraded above 20%%)."
	stealLine    = "Steal %d%% (degraded above 10%%)."

	// Entry 18 — the healthy separator; byte-identical to the degraded one
	// (entry 6). The literal "\nTechnical Details: ".
	technicalDetails = "\nTechnical Details: "
)

// composeHealthy renders the two-layer healthy budget message. The displayed
// components are rounded first, then headroom is derived as
// total - used - reserve on those ALREADY-ROUNDED values, so the printed
// arithmetic in the Technical Details headroom line is exact by construction
// (never independently rounds Signals.HeadroomCores). used/headroom/reserve
// print with one decimal; total prints as an integer when whole. The
// Technical Details dashboard lists only the applicable alert-rule budgets:
// headroom always, then throttle/pressure/steal each only when its rule
// applies. R2 adds the below-floor withholding; R3 flips the budget gates to
// per-signal readiness.
func composeHealthy(signals Signals) string {
	// Zero-capacity guard: when CapacityCores is 0, do not compose the garbled
	// "0.0 of 0 cores, -1.0 headroom" budget dashboard. Return a safe string
	// that conveys monitoring-unavailability; the State on the wire stays
	// healthy per the binary contract. It is an early return, not a prefix.
	if signals.CapacityCores == 0 {
		return cpuMonitoringUnavailable
	}

	// R2: the healthy message reports only what it measured. Two track floors
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

	// The display figures. total/used/reserve are each rounded once; headroom
	// is derived from those rounded values. ReserveCores is read from Signals
	// in both modes (R1) — Decide filled it from the verdict's own reserve —
	// never from the constant.
	var usedDisp, reserveDisp float64
	if signals.LimitApplies {
		usedDisp = round1(signals.AvgUsageCores)
	} else {
		usedDisp = round1(signals.HostBusyCores60sMean)
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

	// The advisory slot, between the headline and Technical Details. The
	// limited-visibility note comes first; the F6 host-headroom sentence
	// second. Both may appear on one tick.
	if signals.LimitedVisibility {
		msg += "\n" + limitedVisibilityNote
	}
	if !signals.HostHeadroomAvailable && signals.HostCpus > 0 {
		// §5 F6 verbatim, with the two core counts substituted. The HostCpus >
		// 0 half is the ScopeUnknown case: on an unknown machine count the
		// bare float64 stays 0 and the sentence is withheld silently.
		msg += "\n" + fmt.Sprintf("host headroom unavailable: this container is pinned to %s of %s CPUs",
			fmtCoresTotal(signals.LogicalCpus), fmtCoresTotal(signals.HostCpus))
	}

	// The budget body. Headroom is unconditional; each of throttle, pressure
	// and steal prints only when THIS TICK'S reading is usable (R3). The gates
	// are the per-signal readiness trio, never the capability flags — a
	// LimitApplies/PsiApplies/StealApplies build prints a confident 0% for a
	// reading that never happened, which is F1.
	details := []string{
		fmt.Sprintf(headroomLine, headroomStr, totalStr, usedStr, reserveStr),
	}
	if signals.ThrottleSignalReady {
		details = append(details, fmt.Sprintf(throttleLine, pctOf(signals.ThrottleRatio)))
	}
	if signals.PressureSignalReady {
		details = append(details, fmt.Sprintf(pressureLine, pctOf(signals.PressureAvg60Out)))
	}
	if signals.StealSignalReady {
		details = append(details, fmt.Sprintf(stealLine, pctOf(signals.StealP95)))
	}

	return msg + technicalDetails + strings.Join(details, " ")
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

// fmtCoresTotal formats a cores value as a whole integer when it has no
// fractional part (so an 8-core box reads "8", not "8.0"), else with one
// decimal. Its input is already round1'd.
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
