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

package memoryhealth

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

const (
	bytesPerGiB = 1 << 30

	closeToDegradedShareOfTotal = 0.05
)

// ComposeMessage renders a verdict as the memory health message: a headline,
// then one paragraph per cause, then the Technical Details table.
func ComposeMessage(verdict Verdict, details Details) string {
	if verdict.State != StateDegraded || len(verdict.Causes) == 0 {
		return composeHealthy(details)
	}

	paragraphs := make([]string, 0, len(verdict.Causes))
	for _, cause := range verdict.Causes {
		if !speaksForFullness(cause, verdict.Causes) {
			continue
		}

		paragraphs = append(paragraphs, causeDetails(cause, verdict.Causes, details))
	}

	return causeHeadline(verdict.Causes[0].Kind) + "\n" + strings.Join(paragraphs, "\n\n") + technicalDetails(verdict.Causes, details)
}

func composeHealthy(details Details) string {
	if !usageMeasured(details) {
		return memoryStartingUp + technicalDetails(nil, details)
	}

	return healthyHeadline(details) + technicalDetails(nil, details)
}

func healthyHeadline(details Details) string {
	budget := effectiveBudget(details)
	used := fmtGiBNumber(budget.used)
	total := fmtGiB(budget.total)
	nearDegraded := budget.headroom < closeToDegradedShareOfTotal*budget.total

	if !details.LimitApplies {
		if nearDegraded {
			return fmt.Sprintf(headlineHealthyMachineClose, used, total)
		}

		return fmt.Sprintf(headlineHealthyMachine, used, total, fmtGiB(budget.headroom))
	}

	if nearDegraded {
		return fmt.Sprintf(headlineHealthyLimitClose, used, total, toPercent(details.AvgLimitUsedFraction))
	}

	return fmt.Sprintf(headlineHealthyLimit, used, total, toPercent(details.AvgLimitUsedFraction), fmtGiB(budget.headroom))
}

type memoryBudget struct {
	used     float64
	total    float64
	headroom float64
}

func newBudget(usedBytes, totalBytes float64) memoryBudget {
	used := round1(usedBytes / bytesPerGiB)
	degradedAt := round1(fullMarks.Fire.At * totalBytes / bytesPerGiB)

	return memoryBudget{
		used:     used,
		total:    round1(totalBytes / bytesPerGiB),
		headroom: round1(degradedAt - used),
	}
}

func effectiveBudget(details Details) memoryBudget {
	if details.LimitApplies {
		return newBudget(details.AvgUsedBytes, details.TotalBytes)
	}

	return newBudget(details.AvgHostUsedFraction*details.HostTotalBytes, details.HostTotalBytes)
}

func usageMeasured(details Details) bool {
	if details.LimitApplies {
		return details.UsedRingActive && details.LimitRingActive
	}

	return details.HostRingActive && details.HostTotalBytes > 0
}

type memoryRule struct {
	label                string
	measured             string
	marks                diagnosis.Marks
	notApplicableBecause string
	ready                bool
	latched              bool
}

func (r memoryRule) render() string {
	switch {
	case r.notApplicableBecause != "" && !r.latched:
		return fmt.Sprintf("%s not measured (%s).", r.label, r.notApplicableBecause)
	case !r.ready && !r.latched:
		return r.label + " not available (measuring)."
	case !r.ready:
		return r.label + " not available (held)."
	}

	mark, verb := r.marks.Fire, "degrades"
	if r.latched {
		mark, verb = r.marks.Clear, "recovers"
	}

	return fmt.Sprintf("%s %s (%s %s %s).",
		r.label, r.measured, verb, diagnosis.MarkSide(mark, r.marks.Polarity, r.latched), markValue(mark, r.marks.Unit))
}

func markValue(mark diagnosis.Mark, unit string) string {
	if unit == unitFraction {
		return strconv.Itoa(toPercent(mark.At)) + "%"
	}

	return strconv.Itoa(int(mark.At))
}

func technicalDetails(causes []Cause, details Details) string {
	rules := memoryRules(causes, details)

	lines := make([]string, 0, len(rules))
	for _, rule := range rules {
		lines = append(lines, rule.render())
	}

	return technicalDetailsLabel + strings.Join(lines, "\n")
}

func memoryRules(causes []Cause, details Details) []memoryRule {
	return []memoryRule{
		{
			label:                labelInstanceMemory,
			measured:             fmt.Sprintf("%d%% of %s limit", toPercent(details.AvgLimitUsedFraction), fmtGiB(details.TotalBytes/bytesPerGiB)),
			marks:                fullMarks,
			notApplicableBecause: absentUnless(details.LimitApplies, reasonNoMemoryLimit),
			ready:                details.LimitRingActive,
			latched:              hasKind(causes, CauseKindContainerLimitFull),
		},
		{
			label:                labelMachineMemory,
			measured:             fmt.Sprintf("%d%% of %s", toPercent(details.AvgHostUsedFraction), fmtGiB(details.HostTotalBytes/bytesPerGiB)),
			marks:                fullMarks,
			notApplicableBecause: absentUnless(details.HostTotalBytes > 0, reasonNoMachineReading),
			ready:                details.HostRingActive,
			latched:              hasKind(causes, CauseKindHostMemoryFull),
		},
		{
			label:                labelPressure,
			measured:             fmt.Sprintf("%d%%", toPercent(details.PressureAvg60)),
			marks:                pressureMarks,
			notApplicableBecause: absentUnless(details.PressureApplies, reasonNoPressureStats),
			ready:                details.PressureSignalReady,
			latched:              hasKind(causes, CauseKindPressure),
		},
		{
			label:                labelOomKills,
			measured:             fmt.Sprintf("%d in the last minute", int(details.OomKillsLast60s)),
			marks:                oomKillMarks,
			notApplicableBecause: absentUnless(details.OomKillsApply, reasonNoOomCounter),
			ready:                details.OomSignalReady,
			latched:              hasKind(causes, CauseKindOomKill),
		},
	}
}

func absentUnless(applies bool, because string) string {
	if applies {
		return ""
	}

	return because
}

func causeHeadline(kind CauseKind) string {
	switch kind {
	case CauseKindOomKill:
		return headlineOomKill
	case CauseKindPressure:
		return headlinePressure
	case CauseKindContainerLimitFull:
		return headlineLimitFull
	case CauseKindHostMemoryFull:
		return headlineHostFull
	default:
		return headlineGeneric
	}
}

func isFullnessKind(kind CauseKind) bool {
	return kind == CauseKindContainerLimitFull || kind == CauseKindHostMemoryFull
}

func hasKind(causes []Cause, kind CauseKind) bool {
	for _, cause := range causes {
		if cause.Kind == kind {
			return true
		}
	}

	return false
}

func speaksForFullness(cause Cause, causes []Cause) bool {
	if !isFullnessKind(cause.Kind) {
		return true
	}

	if !hasKind(causes, CauseKindContainerLimitFull) || !hasKind(causes, CauseKindHostMemoryFull) {
		return true
	}

	return cause.Kind == CauseKindHostMemoryFull
}

func causeDetails(cause Cause, causes []Cause, details Details) string {
	switch cause.Kind {
	case CauseKindOomKill:
		return fmt.Sprintf(detailOomKill, processCount(cause.Value))
	case CauseKindPressure:
		return fmt.Sprintf(detailPressure, toPercent(cause.Value))
	case CauseKindContainerLimitFull:
		return fmt.Sprintf(detailLimitFull, toPercent(cause.Value), fmtGiB(details.TotalBytes/bytesPerGiB))
	case CauseKindHostMemoryFull:
		if hasKind(causes, CauseKindContainerLimitFull) {
			return fmt.Sprintf(detailBothFull, toPercent(details.AvgLimitUsedFraction), fmtGiB(details.TotalBytes/bytesPerGiB))
		}

		return fmt.Sprintf(detailHostFull, toPercent(cause.Value), fmtGiB(details.HostTotalBytes/bytesPerGiB))
	default:
		return detailGeneric
	}
}

func processCount(kills float64) string {
	count := int(kills)
	if count == 1 {
		return "1 process"
	}

	return strconv.Itoa(count) + " processes"
}

func round1(value float64) float64 {
	return math.Round(value*10) / 10
}

func fmtGiBNumber(gib float64) string {
	return fmt.Sprintf("%.1f", round1(gib))
}

func fmtGiB(gib float64) string {
	return fmtGiBNumber(gib) + " GiB"
}

func toPercent(fraction float64) int {
	return int(math.Round(fraction * 100))
}

const (
	memoryStartingUp = "Memory: starting up."

	headlineHealthyLimit        = "Memory healthy. This instance is using %s of %s (%d%% of its limit) and can use %s more before it is marked degraded."
	headlineHealthyLimitClose   = "Memory healthy. This instance is using %s of %s (%d%% of its limit) and is close to being marked degraded."
	headlineHealthyMachine      = "Memory healthy. The machine is using %s of %s and can use %s more before it is marked degraded."
	headlineHealthyMachineClose = "Memory healthy. The machine is using %s of %s and is close to being marked degraded."

	headlineOomKill   = "Memory ran out"
	headlinePressure  = "Memory contention"
	headlineLimitFull = "Memory running near its limit"
	headlineHostFull  = "Machine memory running near full"
	headlineGeneric   = "Memory degraded"

	detailOomKill   = "The kernel stopped %s in this instance in the last minute because memory ran out. Give this instance more memory, or reduce the load on it."
	detailPressure  = "Tasks in this instance spent %d%% of the last minute waiting for memory. Reduce the load on this instance, or give it more memory. If other workloads share this machine they may be competing for it."
	detailLimitFull = "This instance averaged %d%% of its %s memory limit over the last minute and has little headroom left. Raise its memory limit, or reduce the load on it."
	detailHostFull  = "The machine's memory averaged %d%% of %s over the last minute. Add memory to the machine, or reduce what is running on it."
	detailBothFull  = "The machine's memory is nearly full, so raising this instance's memory limit cannot help. Add memory to the machine, or reduce other software running on it. (This instance is also at %d%% of its %s memory limit.)"
	detailGeneric   = "Memory is degraded."

	technicalDetailsLabel = "\nTechnical Details:\n"

	labelInstanceMemory = "Instance memory"
	labelMachineMemory  = "Machine memory"
	labelPressure       = "Pressure"
	labelOomKills       = "OOM kills"

	reasonNoMemoryLimit    = "this instance has no memory limit set, so the machine's memory is its limit"
	reasonNoMachineReading = "the machine's memory is not readable right now"
	reasonNoPressureStats  = "this operating system does not report memory pressure stats"
	reasonNoOomCounter     = "the OOM kill counter is not readable"
)
