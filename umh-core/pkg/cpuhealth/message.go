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
	"strings"
)

// limitedVisibilityNote is the limited-visibility advisory appended to the
// healthy headline when the sample is in the dead-zone (no CPU limit, no PSI,
// not virtualized). It is the caller-facing "blind state" signal: the verdict
// State is binary healthy|degraded, so the blind state cannot be a state — it
// surfaces here as message text instead.
const limitedVisibilityNote = "Limited visibility: no CPU limit or pressure stats set, so starvation cannot be fully measured. Set a CPU limit or enable Linux pressure stats (psi=1) to turn on full monitoring."

// ComposeMessage turns a Verdict and its derived Signals into the C2 two-layer
// Console message: a one-line headline naming the dominant cause, followed by
// the literal "Technical Details:" separator and the curated per-cause copy the
// alert adapter collapses into the expandable panel. When multiple causes fire,
// each cause's detail paragraph is listed in the Technical Details section
// (dominant first, separated by a blank line). A healthy verdict yields the
// "CPU healthy" headline — never an empty string (an empty headline renders
// blank in the Console). When the healthy sample is in the dead-zone
// (signals.LimitedVisibility), the limited-visibility advisory is appended on a
// new line so the operator is told why starvation cannot be fully measured.
func ComposeMessage(verdict Verdict, signals Signals) string {
	if verdict.State != StateDegraded || len(verdict.Causes) == 0 {
		msg := "CPU healthy"
		if signals.LimitedVisibility {
			msg += "\n" + limitedVisibilityNote
		}

		return msg
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
	case CauseKindHostContention:
		return "Host CPU taken by other software"
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
//   - host-contention: cause.Value is contentionCores (cores outside UMH).
//     The host-busy RATIO is not exposed on Signals, so the busy-percentage
//     fallback is derived from the same Cause Value (cores*100 = single-core
//     CPU%, the Linux convention where 300% = 3 fully-busy cores), keeping the
//     percentage and the core count internally consistent.
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
	case CauseKindHostContention:
		cores := int(math.Round(c.Value))
		busyPct := pctOf(c.Value)

		return fmt.Sprintf("This machine ran at %d%% CPU (peak over the last minute); about %d cores are used by software outside UMH. This instance has no reserved CPU here, so it competes for what's left. Check what else runs on this host. Give UMH dedicated CPU — reserve/pin cores for it, or reduce what else runs here. A CPU limit caps UMH; it does not protect it from neighbours.", busyPct, cores)
	case CauseKindSaturation:
		pct := pctOf(signals.AvgUsageFraction)

		return fmt.Sprintf("CPU averaged %d%% over the last minute. This instance has no CPU limit and its OS isn't reporting CPU-pressure stats, so we can't confirm whether work is being starved — but there's little headroom left. Set a CPU limit (simplest, and it lets us measure starvation directly), or enable Linux pressure stats (boot the OS with psi=1). Consider adding CPU capacity.", pct)
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
	case CauseKindHostContention:
		return "Can't add another bridge: other software on this host is using most of the CPU. Give UMH dedicated CPU, or reduce what else runs here, first."
	case CauseKindSaturation:
		return "Can't add another bridge: CPU has been running near full and we can't determine the cause. Add CPU capacity, or set a CPU limit, first."
	default:
		return "Can't add another bridge: CPU is degraded."
	}
}
