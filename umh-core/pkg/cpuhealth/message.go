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
// the constants below, plus the host-headroom sentence composeHealthy formats
// inline, and one deliberate duplicate the const block names. A second copy of
// a sentence is where drift starts.
//
// No threshold is written down here at all. Every number the Technical Details
// table states is read from the mark pair its own signal_*.go declares, which
// is the pair the engine judges against, so the rule the message states and the
// rule the code applies cannot come apart.

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
// cores figures come from coresBudget, so the printed headroom arithmetic is
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
	// measured. It lasts two ticks after each start and
	// respawn: the sampler derives no rate from its first read, and the mean
	// over the rates after that needs two of them. It is not the zero-capacity
	// case (a standing state) - they share a rendering path and nothing else.
	if !usageMeasured(details) {
		return cpuStartingUp + technicalDetails(nil, details)
	}

	b := coresBudget(details)
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

	// A healthy tick has no fired cause, so every rule shows the mark that
	// would fire it.
	return msg + technicalDetails(nil, details)
}

// budgetCores are the four cores figures a headline and a headroom line share:
// a ceiling, what is being used against it, what is held back, and the spare
// that leaves.
type budgetCores struct {
	headroom float64
	total    float64
	used     float64
	reserve  float64
}

// newBudget assembles the four figures from a ceiling, a usage and a reserve.
// Each of the three is rounded to one decimal first, and headroom is then
// derived as total - used - reserve from the already-rounded three, so the
// arithmetic the headroom line prints adds up exactly; it never independently
// rounds Details.HeadroomCores.
func newBudget(total, used, reserve float64) budgetCores {
	b := budgetCores{
		total:   round1(total),
		used:    round1(used),
		reserve: round1(reserve),
	}
	b.headroom = round1(b.total - b.used - b.reserve) // clears float residue, not an independent rounding

	return b
}

// machineBudget is the machine's cores against the machine's busy time, the
// pair hostCpuFullSignal's host-headroom arm subtracts. Its reserve is the
// fixed cpuReserveCores rather than Details.ReserveCores, because Details
// carries one reserve and it follows the mode: on a box under a CPU limit that
// field holds the limit's own reserve, which says nothing about the machine.
func machineBudget(details Details) budgetCores {
	return newBudget(details.LogicalCpus, details.AvgHostBusyCores, cpuReserveCores)
}

// instanceBudget is this container's own CPU limit against its own usage, the
// pair containerLimitFullSignal's arm subtracts. ReserveCores is read from
// Details rather than recomputed - buildDetails filled it from the verdict's
// own reserve, never from the constant.
func instanceBudget(details Details) budgetCores {
	return newBudget(details.CapacityCores, details.AvgUsageCores, details.ReserveCores)
}

// coresBudget is the budget the healthy headline speaks in, which is the one
// the mode makes the customer's ceiling: their own limit where one applies, the
// machine otherwise.
func coresBudget(details Details) budgetCores {
	if details.LimitApplies {
		return instanceBudget(details)
	}

	return machineBudget(details)
}

// machineMeasured reports whether this tick produced the machine's busy figure
// machineBudget subtracts. It needs both the window and the reading: a thin
// window and an unreadable /proc/stat both leave the subtraction with no term.
func machineMeasured(details Details) bool {
	return details.HostBusyRingActive && details.HostBusyCoresAvailable
}

// instanceMeasured reports whether this tick produced the container's own usage
// figure instanceBudget subtracts.
func instanceMeasured(details Details) bool {
	return details.UsageRingActive
}

// usageMeasured reports whether this tick produced the usage figure the healthy
// headline is built on. Two measurement floors, one per mode: an outage can
// leave one window thin while the other fills, and a limit-mode headline reads
// the container's own usage, so a thin host-busy window must not withhold it.
func usageMeasured(details Details) bool {
	if details.LimitApplies {
		return instanceMeasured(details)
	}

	return machineMeasured(details)
}

// cpuRule is one line of the Technical Details table: one rule that can degrade
// this instance's CPU, written as what it measured against the threshold that
// applies to it right now.
//
// applies and ready are the two halves Details keeps apart and forbids reading
// as each other. applies is whether the box can run the rule at all; ready is
// whether this tick's window produced a value. An absent line says which of the
// two it is, because "this kernel reports no pressure statistics" and "the
// window is still filling" send a reader to different places.
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
	// released. It picks which of the two marks the line states. A signal holds
	// one latch however many instruments it has, so the two lines a
	// two-instrument signal prints state the same side of their own pairs: one
	// question cannot have been answered yes for one of its lines and no for
	// the other.
	latched bool
	// firedHere is whether THIS line's own instrument produced one of this
	// tick's causes, which is proof the rule ran on this box whatever the
	// capability flag says. It is the instrument's own firing and never its
	// signal's: host-cpu-full answering from /proc/stat says nothing about
	// whether our-usage-over-the-machine can be measured here.
	firedHere bool
}

// render writes one line of the table.
//
// A rule that has not fired shows the mark that would fire it; a latched rule
// shows the mark that clears it, because for a latched rule the clear mark is
// the one that decides what happens next. It also removes an apparent
// contradiction: a rule still latched while its reading has fallen back under
// its own fire mark would otherwise be shown a threshold it is already inside.
func (r cpuRule) render() string {
	// A fired instrument is proof the rule ran on this box, whatever the
	// capability flag says, so a fired rule is never reported as one the box
	// cannot run.
	switch {
	case !r.applies && !r.firedHere:
		return r.label + " not available (not possible)."
	case !r.ready:
		return r.label + " not available (measuring)."
	}

	mark, verb := r.marks.Fire, "degrades"
	if r.latched {
		mark, verb = r.marks.Clear, "recovers"
	}

	return fmt.Sprintf("%s %s (%s %s %s).",
		r.label, r.measured, verb, markSide(mark, r.marks.Polarity, r.latched), markValue(mark, r.marks.Unit))
}

// markSide names the side of a mark that crosses it, in the reader's words. A
// fire mark is crossed toward the worse side and a clear mark toward the better
// one, so a single polarity reads both ways.
//
// An inclusive mark counts landing exactly on it, and reads "at": "degrades at
// 70%" for the usage rule, where 70% of the machine busy is already a full
// machine.
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
// one line per rule the engine is judging, in a fixed order. causes is this
// tick's fired list, and is empty on a healthy tick.
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
// Each rule carries the mark pair its own signal_*.go declares, never a copy of
// the numbers in it.
//
// Latchedness is read per SIGNAL and the capability override per INSTRUMENT;
// cpuRule's two fields say why.
func cpuRules(causes []Cause, details Details) []cpuRule {
	machineFull := hasKind(causes, CauseKindHostCpuFull)
	limitFull := hasKind(causes, CauseKindContainerLimitFull)
	_, hostHeadroomFired := firedCause(causes, CauseKindHostCpuFull, instrumentHostHeadroom)
	_, usageFired := firedCause(causes, CauseKindHostCpuFull, instrumentUsageFraction)
	_, limitHeadroomFired := firedCause(causes, CauseKindContainerLimitFull, instrumentLimitHeadroom)

	// Headroom is one subtraction against a ceiling, and a box can be judged
	// against two of them at once: the machine's cores, and this container's
	// own CPU limit. One line per ceiling the engine is judging, so a machine
	// that filled while the container sits well inside its limit does not print
	// the container's comfortable figure under a headline saying the machine is
	// full.
	//
	// Which ceilings those are is decided in exactly one place. table_cpu.go
	// declares a capacity signal under hostCpuFullDeclared and
	// containerLimitFullDeclared, and the two lines below are appended under
	// the same two predicates, read off the figures the table was built from.
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
// applies is true by construction. The caller appends this line only for a
// ceiling cpuTable declared a signal for, so the box does run the rule; a
// ceiling it declared nothing for gets no line at all rather than a slot.
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

// tableCeilings returns the two figures cpuTable was built from - the machine's
// core count and this container's CPU quota - read back off Details, so the
// table's headroom lines and the engine's capacity signals answer to the same
// two predicates.
//
// A quota is on Details only through the mode: CapacityCores is the quota where
// a limit applies and the machine's core count where none does, so a zero quota
// is what no-limit mode means here.
//
// Both figures are this tick's read, while cpuTable was handed the startup one.
// They differ only where a cpuset that read at startup stops reading later, and
// on that tick the line would have no figure to print anyway.
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

// BlockReason returns the per-cause bridge-refusal message shown when bridge
// creation is refused because the instance's CPU is degraded. It speaks with
// the cause the Technical Details speak with, both through speakingCause, so
// the two surfaces cannot hand one customer two contradictory remedies at once.
// It reads the attribution for the same reason, and asks the ranked causes the
// same blend question first. An unknown kind falls back to the generic degraded
// message.
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
