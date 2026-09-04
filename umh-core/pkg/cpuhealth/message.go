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
// Every customer-visible string in this file is written down exactly once. The
// const block below holds the whole sentences; the Technical Details lines are
// assembled from formats at the sites that build them, and the const block
// names the one deliberate duplicate. A second copy of a sentence is where
// drift starts.
//
// No signal threshold is written down here. Every number the Technical Details
// table states is read from the mark pair its own signal_*.go declares, which
// is the pair the engine judges against, so the rule the message states and the
// rule the code applies cannot come apart.
//
// One number here is this file's own: the 0.05 cores at which the healthy
// headline starts calling a box close to degraded. It is a display band, not a
// rule the engine judges, and it does not read hostHeadroomMarks.

package cpuhealth

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// ComposeMessage turns a Verdict and its derived Details into the message an
// operator reads: a one-line headline naming the dominant cause, then the
// curated per-cause copy (dominant first, joined by a blank line), then the
// Technical Details table. A healthy verdict yields the budget headline from
// composeHealthy, followed by the same table.
//
// One paragraph per cause, except that the two capacity causes share one:
// speaksForCapacity says which of the pair writes it.
func ComposeMessage(verdict Verdict, details Details) string {
	if verdict.State != StateDegraded || len(verdict.Causes) == 0 {
		return composeHealthy(details)
	}

	headline := causeHeadline(verdict.Causes[0].Kind)

	parts := make([]string, 0, len(verdict.Causes))
	for _, c := range verdict.Causes {
		if !speaksForCapacity(c, verdict.Causes) {
			continue
		}
		parts = append(parts, causeDetails(c, verdict.Causes, details))
	}
	body := strings.Join(parts, "\n\n")

	return headline + "\n" + body + technicalDetails(verdict.Causes, details)
}

// composeHealthy renders the healthy message: the budget headline, any
// advisory the tick earned, then the Technical Details table. The displayed
// cores figures come from effectiveBudget, so the printed headroom arithmetic is
// exact by construction.
func composeHealthy(details Details) string {
	// Zero-capacity guard: when CapacityCores is 0, do not compose the garbled
	// "0.0 of 0 cores, -1.0 headroom" budget dashboard. Return a safe string
	// that conveys monitoring-unavailability; the State on the wire stays
	// healthy per the binary contract. It is an early return, not a prefix, and
	// technicalDetails withholds the table on the same condition.
	if details.CapacityCores == 0 {
		return cpuMonitoringUnavailable
	}

	// The healthy message reports only what it measured. When the usage figure
	// is withheld the message has no headline sentence left, so render the
	// single "CPU: starting up." line over the table, where a line whose own
	// window has not reduced yet says so rather than stating a figure nothing
	// measured. It lasts two ticks after each start and respawn: the sampler
	// derives no rate from its first read, and the mean over the rates after
	// that needs two of them. It is not the zero-capacity case (a standing
	// state): that one returns bare, this one carries the table.
	if !usageMeasured(details) {
		return cpuStartingUp + technicalDetails(nil, details)
	}

	b := effectiveBudget(details)
	usedStr := fmtCores1(b.used)
	totalStr := fmtCoresTotal(b.total)
	headroomStr := fmtCores1(b.headroom)

	// A sub-0.05-core quota collapses to a displayed total of 0: suppress the
	// percentage (a division by a displayed zero) and use the no-percentage
	// variant. The subject follows the MODE, not the column: a tiny quota is
	// still limit mode, so it reads "This instance".
	totalTooSmallToPct := b.total <= 0

	var headline string
	if details.LimitApplies && !totalTooSmallToPct {
		pctOfLimit := toPercent(b.used / b.total)
		if b.headroom < 0.05 {
			headline = fmt.Sprintf(headlineLimitClose, usedStr, totalStr, pctOfLimit)
		} else {
			headline = fmt.Sprintf(headlineLimitMore, usedStr, totalStr, pctOfLimit, headroomStr)
		}
	} else {
		subject := subjectTheMachine
		if details.LimitApplies {
			subject = subjectThisInstance
		}
		if b.headroom < 0.05 {
			headline = fmt.Sprintf(headlineClose, subject, usedStr, totalStr)
		} else {
			headline = fmt.Sprintf(headlineMore, subject, usedStr, totalStr, headroomStr)
		}
	}

	msg := headline

	if details.LimitedVisibility {
		msg += "\n" + limitedVisibilityNote
	}
	if !details.HostHeadroomAvailable && details.HostCpus > 0 {
		// The host-headroom sentence, with the two core counts substituted. The HostCpus >
		// 0 half is the ScopeUnknown case: on an unknown machine count the
		// bare float64 stays 0 and the sentence is withheld silently.
		msg += "\n" + fmt.Sprintf("host headroom unavailable: this container is pinned to %s of %s CPUs",
			fmtCoresTotal(details.LogicalCpus), fmtCoresTotal(details.HostCpus))
	}

	// nil causes: a healthy tick has fired none, so no line states a clear
	// mark.
	return msg + technicalDetails(nil, details)
}

// budgetCores are the four cores figures a headline and a headroom line share.
type budgetCores struct {
	headroom float64
	total    float64
	used     float64
	reserve  float64
}

// newBudget derives headroom from the already-rounded three, so the arithmetic
// the headroom line prints adds up exactly. No stored field can serve: a
// headroom read from Details would not be the difference of these three.
func newBudget(total, used, reserve float64) budgetCores {
	b := budgetCores{
		total:   round1(total),
		used:    round1(used),
		reserve: round1(reserve),
	}
	b.headroom = round1(b.total - b.used - b.reserve) // clears float residue, not an independent rounding

	return b
}

// machineBudget takes the reserve from the fixed cpuReserveCores, not from
// Details.ReserveCores: that field follows the mode, so under a CPU limit it
// holds the limit's reserve, which says nothing about the machine.
func machineBudget(details Details) budgetCores {
	return newBudget(details.LogicalCpus, details.AvgHostBusyCores, cpuReserveCores)
}

// instanceBudget reads ReserveCores from Details rather than recomputing it:
// buildDetails filled it from the verdict's own reserve, never the constant.
func instanceBudget(details Details) budgetCores {
	return newBudget(details.CapacityCores, details.AvgUsageCores, details.ReserveCores)
}

// effectiveBudget is the customer's ceiling: their own limit where one applies,
// the machine otherwise.
func effectiveBudget(details Details) budgetCores {
	if details.LimitApplies {
		return instanceBudget(details)
	}

	return machineBudget(details)
}

// machineMeasured needs both the window and the reading: a thin window and an
// unreadable /proc/stat both leave the subtraction with no term.
func machineMeasured(details Details) bool {
	return details.HostBusyRingActive && details.HostBusyCoresAvailable
}

func instanceMeasured(details Details) bool {
	return details.UsageRingActive
}

// usageMeasured has one floor per mode, because an outage can leave one window
// thin while the other fills and a limit-mode headline reads only its own usage.
func usageMeasured(details Details) bool {
	if details.LimitApplies {
		return instanceMeasured(details)
	}

	return machineMeasured(details)
}

// cpuRule is one line of the Technical Details table: a rule that can degrade
// this instance's CPU, against the threshold applying to it now.
//
// applies and ready must never be read as each other: applies is whether the box
// can run the rule at all, ready is whether this tick produced a value. An
// absent line says which of the two it is.
type cpuRule struct {
	// label opens the line, and names the rule rather than the instrument: an
	// operator reading "Steal" need not know which of the two steal arms
	// answered.
	label string
	// measured is the reading, already written in the rule's own unit - "18%",
	// or the whole headroom subtraction.
	measured string
	// marks is the pair the engine judged this rule against, carried whole so
	// the line states the live threshold rather than a copy of it.
	marks   diagnosis.Marks
	applies bool
	ready   bool
	// latched is whether the SIGNAL this line belongs to has fired and not yet
	// released. It picks which of the two marks the line states, and it is read
	// per signal, so both lines of a two-instrument signal state the same side.
	latched bool
	// firedHere is whether THIS line's own instrument produced one of this
	// tick's causes, which is proof the rule ran on this box whatever the
	// capability flag says. It is the instrument's own firing and never its
	// signal's: host-cpu-full answering from /proc/stat says nothing about
	// whether our-usage-over-the-machine can be measured here.
	firedHere bool
}

// render writes one line of the table. A rule that has not fired shows the mark
// that would fire it; a rule that has fired shows the mark that would clear it.
// A rule with no reading says which kind of absence it is: "measuring" while its
// window is still filling, "held" once it has fired and not yet cleared. An
// operator is then never told that a fired signal is still warming up.
func (r cpuRule) render() string {
	switch {
	case !r.applies && !r.firedHere:
		return r.label + " not available (not possible)."
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
		r.label, r.measured, verb, markSide(mark, r.marks.Polarity, r.latched), markValue(mark, r.marks.Unit))
}

// markSide names the side of a mark in the reader's words. A fire mark is
// crossed toward the worse side and a clear mark toward the better one, so one
// polarity reads both ways. An inclusive mark counts landing exactly on it and
// reads "at" - "degrades at 70%", where 70% busy is already a full machine.
func markSide(m diagnosis.Mark, p diagnosis.Polarity, clearing bool) string {
	if m.Inclusive {
		return "at"
	}
	if (p == diagnosis.HigherIsWorse) == clearing {
		return "below"
	}

	return "above"
}

// markValue writes a mark in the unit its own pair declares: a ratio or a
// fraction as a percentage, cores as a plain cores figure. A unit no signal
// declares falls through to the plain figure, which states no unit it cannot
// vouch for.
func markValue(m diagnosis.Mark, unit string) string {
	switch unit {
	case unitRatio, unitFraction:
		return strconv.Itoa(toPercent(m.At)) + "%"
	default:
		return fmtCoresTotal(round1(m.At))
	}
}

// technicalDetails renders the labelled table: the label on its own line, then
// one line per rule in cpuRules' fixed order, whether or not this box can run
// the rule. causes is this tick's fired list, and is empty on a healthy tick.
//
// A failed cgroup read (CapacityCores == 0) gets no table at all. On that tick
// we do not know which rules apply, so a table would assert knowledge we do not
// have.
func technicalDetails(causes []Cause, details Details) string {
	if details.CapacityCores == 0 {
		return ""
	}

	rules := cpuRules(causes, details)
	lines := make([]string, 0, len(rules))
	for _, r := range rules {
		lines = append(lines, r.render())
	}

	return technicalDetailsLabel + strings.Join(lines, "\n")
}

// cpuRules lists the rules the table reports, in the order it prints them.
// Every rule the engine is judging appears in every state: a slot saying why it
// is absent is what stops a reader taking a missing line for a rule that is
// fine.
//
// Latchedness is read per SIGNAL and the capability override per INSTRUMENT;
// cpuRule's two fields say why.
func cpuRules(causes []Cause, details Details) []cpuRule {
	machineFull := hasKind(causes, CauseKindHostCpuFull)
	limitFull := hasKind(causes, CauseKindContainerLimitFull)
	_, hostHeadroomFired := firedCause(causes, CauseKindHostCpuFull, instrumentHostHeadroom)
	_, usageFired := firedCause(causes, CauseKindHostCpuFull, instrumentUsageFraction)
	_, limitHeadroomFired := firedCause(causes, CauseKindContainerLimitFull, instrumentLimitHeadroom)

	// A box can be judged against two ceilings at once, the machine's cores and
	// its own CPU limit, so each gets its own line: a machine that filled while
	// the container sits well inside its limit must not print the container's
	// comfortable figure under a headline saying the machine is full.
	cores, quota := tableCeilings(details)

	rules := make([]cpuRule, 0, 6)
	if hostCpuFullDeclared(cores) {
		rules = append(rules, headroomRule(labelMachineHeadroom, machineBudget(details),
			hostHeadroomMarks, machineMeasured(details), machineFull, hostHeadroomFired))
	}
	if containerLimitFullDeclared(quota) {
		rules = append(rules, headroomRule(labelInstanceHeadroom, instanceBudget(details),
			limitHeadroomMarks(quota), instanceMeasured(details), limitFull, limitHeadroomFired))
	}

	// Steal is answered by a percentile arm and a mean arm sharing one pair.
	// Details.StealP95 names the percentile, which reads 0 until the window
	// holds twenty samples, so a latched episode reports the arm it fired on -
	// the number the steal paragraph above the table already prints.
	stealValue := details.StealP95
	steal, stealFired := firedCause(causes, CauseKindSteal, instrumentStealP95, instrumentStealMean)
	if stealFired {
		stealValue = steal.Value
	}

	// The three lines below each carry a whole signal, so their signal's latch
	// and their own instruments' firing are the same fact read twice. Steal has
	// two instruments, but both of them feed this one line.
	throttlingFired := hasKind(causes, CauseKindThrottling)
	pressureFired := hasKind(causes, CauseKindPressure)

	return append(rules,
		cpuRule{
			label:    "Usage",
			measured: fmt.Sprintf("%d%% of capacity", toPercent(details.AvgUsageFraction)),
			marks:    usageFractionMarks,
			// usage-fraction is evidence of last resort: it is declared only
			// where there is no quota to judge our own budget against and no PSI
			// to read the harm off, which is exactly what
			// Details.LimitedVisibility holds. A box with a CPU limit therefore
			// never runs this rule.
			applies: details.LimitedVisibility,
			// usage-fraction reduces the same sample field over the same span as
			// the usage-cores measurement, so one window's state answers for
			// both.
			ready:     details.UsageRingActive,
			latched:   machineFull,
			firedHere: usageFired,
		},
		cpuRule{
			label:     "Throttling",
			measured:  fmt.Sprintf("%d%%", toPercent(details.ThrottleRatio)),
			marks:     throttleMarks,
			applies:   details.LimitApplies,
			ready:     details.ThrottleSignalReady,
			latched:   throttlingFired,
			firedHere: throttlingFired,
		},
		cpuRule{
			label:     "Pressure",
			measured:  fmt.Sprintf("%d%%", toPercent(details.PressureAvg60)),
			marks:     pressureMarks,
			applies:   details.PressureApplies,
			ready:     details.PressureSignalReady,
			latched:   pressureFired,
			firedHere: pressureFired,
		},
		cpuRule{
			label:     "Steal",
			measured:  fmt.Sprintf("%d%%", toPercent(stealValue)),
			marks:     stealMarks,
			applies:   details.StealApplies,
			ready:     details.StealSignalReady,
			latched:   stealFired,
			firedHere: stealFired,
		},
	)
}

// headroomRule builds one headroom line: the subtraction its budget spells out,
// against the mark pair the signal measuring that ceiling declares.
//
// applies is true by construction: the caller appends a line only where the
// declared-ceiling test passed, and a ceiling that fails it gets no slot at
// all. That the engine then judges the rule is the usual case, not a guarantee
// - hostCpuFullDeclared has the exception.
func headroomRule(label string, b budgetCores, marks diagnosis.Marks, ready, latched, firedHere bool) cpuRule {
	return cpuRule{
		label: label,
		measured: fmt.Sprintf("%s cores = %s total - %s used - %s reserved",
			fmtCores1(b.headroom), fmtCoresTotal(b.total), fmtCores1(b.used), fmtCores1(b.reserve)),
		marks:     marks,
		applies:   true,
		ready:     ready,
		latched:   latched,
		firedHere: firedHere,
	}
}

// tableCeilings reads back the two figures the capacity predicates take: the
// machine's core count and this container's CPU quota.
//
// A quota reaches Details only through the mode: CapacityCores is the quota
// where a limit applies and the core count where none does, so zero here means
// no-limit mode.
//
// These are this tick's read; cpuTable was handed the startup one. A cpuset
// that failed at startup and reads later prints a headroom line for a ceiling
// the table never declared (ENG-5752). The opposite order drops a line that had
// no figure to print anyway.
func tableCeilings(details Details) (cores, quota float64) {
	if details.LimitApplies {
		return details.LogicalCpus, details.CapacityCores
	}

	return details.LogicalCpus, 0
}

// firedCause returns this tick's cause for one instrument, matching the kind
// AND one of the instruments named. It answers "did this LINE's own instrument
// fire", which is not the same question as "is the signal latched": hasKind
// answers that one, and a two-instrument signal holds a single latch its two
// lines share.
func firedCause(causes []Cause, kind CauseKind, instruments ...string) (Cause, bool) {
	for _, c := range causes {
		if c.Kind != kind {
			continue
		}
		for _, name := range instruments {
			if c.Instrument == name {
				return c, true
			}
		}
	}

	return Cause{}, false
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
// /proc/stat, rather than estimated from this container's own usage. It reads
// the arm the episode fired on, which a later handover does not move.
func machineWasRead(causes []Cause) bool {
	for _, c := range causes {
		if c.Kind == CauseKindHostCpuFull && c.Instrument == instrumentHostHeadroom {
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
// Where /proc/stat answered, the machine's reading speaks and names the limit
// in a trailing clause. Where the only reading is usage-fraction's estimate
// from our own usage, the container's limit is the measured ceiling and speaks.
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
// interpolating the live number from the cause's Value or from Details.
//
// The two capacity kinds dispatch on the cause's own instrument. A machine read
// full then asks whether the container is at its limit too, because that case
// blends two remedies into one sentence; failing that it asks whose load filled
// the machine, which fullMachineDetail answers.
func causeDetails(c Cause, causes []Cause, details Details) string {
	switch c.Kind {
	case CauseKindThrottling:
		return fmt.Sprintf(detailThrottling, toPercent(details.ThrottleRatio))
	case CauseKindPressure:
		return fmt.Sprintf(detailPressure, toPercent(c.Value))
	case CauseKindSteal:
		return fmt.Sprintf(detailSteal, toPercent(c.Value))
	case CauseKindContainerLimitFull:
		// CapacityCores == 0 replaces the percentage sentence — never
		// prefixes it — here and on the machine arm's readable branch below.
		// The host-unreadable clause still appends afterward, unaffected by
		// which sentence came before it.
		var detail string
		if details.CapacityCores == 0 {
			detail = detailSatCapacityUnavailable
		} else {
			detail = fmt.Sprintf(detailSatLimit, toPercent(details.AvgUsageCores/details.CapacityCores))
		}
		if !details.HostBusyCoresAvailable {
			detail += detailSatHostUnavail
		}

		return detail
	case CauseKindHostCpuFull:
		switch c.Instrument {
		case instrumentUsageFraction:
			// The estimate from our own usage. The no-PSI wording is the one
			// that earns the pressure-stats advice.
			if details.PressureApplies {
				return fmt.Sprintf(detailSatNoStatsPSI, toPercent(c.Value))
			}

			return fmt.Sprintf(detailSatNoStatsNoPSI, toPercent(c.Value))
		case instrumentHostHeadroom:
			if hasKind(causes, CauseKindContainerLimitFull) {
				return fmt.Sprintf(detailSatBothAtLimit, fmtCoresTotal(round1(details.CapacityCores)))
			}
			if details.LimitApplies {
				return fullMachineDetail(c.Attribution)
			}
			if !details.HostBusyCoresAvailable {
				return detailSatNoLimitUnavail
			}
			var detail string
			if details.CapacityCores == 0 {
				detail = detailSatCapacityUnavailable
			} else {
				detail = fmt.Sprintf(detailSatNoLimitRead, toPercent(details.AvgHostBusyCores/details.CapacityCores))
			}
			if details.LimitedVisibility {
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
func fullMachineBlock(attribution Attribution, details Details) string {
	switch attribution {
	case AttributionContainer:
		return blockHostFullContainer
	case AttributionHost:
		if details.LimitApplies {
			return blockHostFull
		}

		return blockNoLimitHost
	default:
		return blockHostFullUnknown
	}
}

// BlockReason returns the refusal message shown when a degraded CPU blocks
// bridge creation. It picks its cause through speakingCause, reads the
// attribution, and asks the same blend question, all as causeDetails does: the
// two surfaces must not hand one customer contradictory remedies. An unknown
// kind falls back to the generic degraded message.
func BlockReason(causes []Cause, details Details) string {
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
		case instrumentHostHeadroom:
			// The order causeDetails takes: a container also at its limit is
			// answered there by one blended sentence, which carries the host
			// remedy, so the refusal shown beside it carries the host remedy
			// too.
			if hasKind(causes, CauseKindContainerLimitFull) {
				cause = blockHostFull
			} else {
				cause = fullMachineBlock(c.Attribution, details)
			}
		case instrumentUsageFraction:
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

// toPercent converts a 0..1 fraction to a rounded integer percentage. Values >1
// (oversubscription / multi-core busy) are preserved as >100 (the Linux CPU%
// convention), so a three-core contention reads as 300%, not clamped to 100.
func toPercent(fraction float64) int {
	return int(math.Round(fraction * 100))
}

const (
	// The line rendered on a tick whose usage figure is withheld (window below
	// its floor, or the host reading absent): no headline and no advisory. The
	// Technical Details table follows it, so an operator still sees which rules
	// will be judged and which windows are still filling.
	//
	// Not cpuMonitoringUnavailable ("cgroup read failed"): on this tick the
	// read succeeded and there is simply one sample, so claiming a read failure
	// tells the customer a specific untrue thing.
	cpuStartingUp = "CPU: starting up."

	// Rendered alone when CapacityCores is 0; an early return, never a prefix.
	cpuMonitoringUnavailable = "CPU monitoring unavailable: cgroup read failed. Defaulting to healthy."

	// The limited-visibility advisory: an annotation on a healthy verdict,
	// never a state.
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

	// The label above the table, byte-identical on the healthy and the degraded
	// path, and on its own line in both. The literal "\nTechnical Details:\n".
	//
	// The table's own lines are not constants: cpuRule.render assembles each one
	// from a label, a reading and a threshold read live from the rule's marks.
	technicalDetailsLabel = "\nTechnical Details:\n"

	// The two headroom lines' labels. A box under a CPU limit is judged against
	// both ceilings at once and prints both lines, so each has to name whose
	// spare cores it is counting. They borrow the two words the headlines above
	// already use for the same pair of subjects: "This instance" and "The
	// machine".
	labelInstanceHeadroom = "Instance headroom"
	labelMachineHeadroom  = "Machine headroom"

	// The degraded headlines, one per CauseKind. headlineGeneric is the default
	// arm, unreachable through today's five kinds but still written so the enum
	// can grow.
	headlineThrottling = "CPU limited"
	headlinePressure   = "CPU contention"
	headlineSteal      = "CPU taken by the server"
	headlineSaturation = "CPU running near full"
	headlineGeneric    = "CPU degraded"

	// The degradation detail paragraphs. Throttling reads the ratio from
	// Details; pressure and steal read Cause.Value, the reduction of the arm
	// the episode fired on — which for steal can be the mean while
	// Details.StealP95 always names the percentile.
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
