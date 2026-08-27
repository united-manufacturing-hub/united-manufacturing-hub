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

// The healthy headline. composeHealthy renders the two-layer healthy
// budget dashboard. The headline is a two-by-two dispatch over
// (LimitApplies && rounded total > 0) and (displayed headroom < 0.05); the
// subject follows the mode and not the column; the displayed headroom is
// derived from the already-rounded total/used/reserve. The withholding
// ("CPU: starting up.") and the readiness-gated budget lines are covered
// separately, so their fields (UsageRingActive/HostBusyRingActive/
// proc-readability) are set true here to keep these assertions on the headline.
package cpuhealth

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// tableLines returns the Technical Details table's lines, in the order the
// message printed them, and fails when the message carries no table.
func tableLines(msg string) []string {
	_, table, found := strings.Cut(msg, technicalDetailsLabel)
	Expect(found).To(BeTrue(), "the message carries no Technical Details table: %q", msg)

	return strings.Split(table, "\n")
}

// tableRules returns the rule each table line reports, in order: the word the
// line opens with.
func tableRules(msg string) []string {
	rules := make([]string, 0, 5)
	for _, line := range tableLines(msg) {
		rules = append(rules, strings.SplitN(line, " ", 2)[0])
	}

	return rules
}

// allRules is the fixed order the table prints, every slot present in every
// state that has a table.
var allRules = []string{"Headroom", "Usage", "Throttling", "Pressure", "Steal"}

// healthyDetails builds a Details bag with every healthy-message input in a
// usable state, so individual fields can be overridden per assertion.
func healthyDetails() Details {
	return Details{
		LimitApplies:           true,
		CapacityCores:          2.0,
		AvgUsageCores:          0.0,
		ReserveCores:           0.2,
		UsageRingActive:        true,
		HostBusyRingActive:     true,
		HostBusyCoresAvailable: true,
		ThrottleSignalReady:    true,
		PressureSignalReady:    true,
		StealSignalReady:       true,
		HostHeadroomAvailable:  true,
		HostCpus:               8,
		LogicalCpus:            2,
	}
}

var _ = Describe("the healthy headline", func() {
	It("should render the headline in limit mode with a percentage, and in no-limit mode with 'The machine' as subject", func() {
		// Limit mode: entry 9 "CPU healthy. This instance is using %s of %s cores (%d%% of its limit) and can use %s more before it is marked degraded."
		limit := healthyDetails()
		Expect(composeHealthy(limit)).To(ContainSubstring(
			"CPU healthy. This instance is using 0.0 of 2 cores (0% of its limit) and can use 1.8 more before it is marked degraded."))

		// No-limit mode: entry 13 with subject entry 10 "The machine"; used is host-busy.
		nolimit := healthyDetails()
		nolimit.LimitApplies = false
		nolimit.CapacityCores = 8
		nolimit.AvgUsageCores = 0
		nolimit.AvgHostBusyCores = 0.0
		nolimit.ReserveCores = 1.0
		Expect(composeHealthy(nolimit)).To(ContainSubstring(
			"CPU healthy. The machine is using 0.0 of 8 cores and can use 7.0 more before it is marked degraded."))
	})

	It("should derive the displayed headroom from the already-rounded total, used and reserve so the printed arithmetic is exact", func() {
		details := healthyDetails()
		details.AvgUsageCores = 0.3
		msg := composeHealthy(details)
		// 2 total - 0.3 used - 0.2 reserved = 1.5, printed exactly.
		Expect(msg).To(ContainSubstring("Headroom 1.5 cores = 2 total - 0.3 used - 0.2 reserved (degrades below %s).",
			fmtCoresTotal(limitHeadroomMarks(details.CapacityCores).Fire.At)))
		Expect(msg).To(ContainSubstring("can use 1.5 more before it is marked degraded."))
	})

	It("should omit the percentage suffix when the rounded total is zero, as a sub-0.05-core quota produces", func() {
		details := healthyDetails()
		details.CapacityCores = 0.04 // rounds to 0.0 total
		details.AvgUsageCores = 0.01
		details.ReserveCores = 0.0
		msg := composeHealthy(details)
		// The no-percentage close headline with the limit-mode subject: no
		// "(N% of its limit)" suffix, "This instance".
		Expect(msg).To(ContainSubstring("CPU healthy. This instance is using 0.0 of 0 cores and is close to being marked degraded."))
		Expect(msg).NotTo(ContainSubstring("% of its limit"))
	})

	It("should say the instance is close to being marked degraded once the displayed headroom falls below 0.05 cores, instead of offering more", func() {
		details := healthyDetails()
		details.AvgUsageCores = 1.8 // headroom 2.0-1.8-0.2 = 0.0 < 0.05
		Expect(composeHealthy(details)).To(ContainSubstring("and is close to being marked degraded."))
		Expect(composeHealthy(details)).NotTo(ContainSubstring("and can use"))

		above := healthyDetails()
		above.AvgUsageCores = 0.0 // headroom 1.8 >= 0.05
		Expect(composeHealthy(above)).To(ContainSubstring("and can use 1.8 more before it is marked degraded."))
	})

	It("should put the limited-visibility advisory between the headline and the technical details when limited visibility is set", func() {
		details := healthyDetails()
		details.LimitedVisibility = true
		msg := composeHealthy(details)
		Expect(msg).To(ContainSubstring(
			"Limited visibility: this instance has no CPU limit set and its operating system is not reporting CPU-pressure stats, so UMH cannot fully tell when work is waiting for a free core. Set a CPU limit or enable Linux pressure stats (boot with psi=1) to turn on full monitoring."))
		// The advisory sits between the headline and the Technical Details separator.
		head := strings.Index(msg, "CPU healthy.")
		adv := strings.Index(msg, "Limited visibility:")
		sep := strings.Index(msg, "Technical Details:")
		Expect(head).To(BeNumerically(">=", 0))
		Expect(adv).To(BeNumerically(">", head))
		Expect(sep).To(BeNumerically(">", adv))
	})

	It("should render the monitoring-unavailable line alone when capacity is zero, with no headline, no advisory and no technical details", func() {
		details := healthyDetails()
		details.CapacityCores = 0
		Expect(composeHealthy(details)).To(Equal(
			"CPU monitoring unavailable: cgroup read failed. Defaulting to healthy."))
	})

	It("should say host headroom is unavailable, naming both core counts, when the container's count describes only the CPUs it may run on", func() {
		details := healthyDetails()
		details.HostHeadroomAvailable = false
		details.HostCpus = 8
		details.LogicalCpus = 2
		msg := composeHealthy(details)
		Expect(msg).To(ContainSubstring("host headroom unavailable: this container is pinned to 2 of 8 CPUs"))
		// It is an advisory-slot line, not the whole message.
		Expect(msg).To(ContainSubstring("CPU healthy."))
		Expect(msg).To(ContainSubstring("Technical Details:"))
		// On ScopeUnknown HostCpus is 0 (bare float64, unknown Get leaves it 0),
		// so the sentence is not rendered for an unknown machine count.
		unknown := healthyDetails()
		unknown.HostHeadroomAvailable = false
		unknown.HostCpus = 0
		unknown.LogicalCpus = 2
		Expect(composeHealthy(unknown)).NotTo(ContainSubstring("pinned to"))
	})

	It("conformance: Decide populates CapacityCores, ReserveCores, and HostBusyCoresAvailable for the message to read", func() {
		engine, err := NewEngine(8, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()
		smp := Sample{
			Timestamp:   base,
			CpuScope:    ScopeHost,
			Quota:       diagnosis.Known(2.0),
			UsageCores:  diagnosis.Known(0.3),
			HostBusy:    diagnosis.Known(1.0),
			LogicalCpus: diagnosis.Known(8),
			HostCpus:    diagnosis.Known(8),
			// PsiAvailable false => limited visibility; supply pressure to keep
			// it healthy.
			Pressure: diagnosis.Known(0.0),
		}
		_, details := Decide(engine, smp, env)
		Expect(details.CapacityCores).To(Equal(2.0), "limit mode: capacity is the quota")
		Expect(details.ReserveCores).To(Equal(0.2), "limit mode: reserve is 0.10 x quota")
		Expect(details.HostBusyCoresAvailable).To(BeTrue(), "the sample's HostBusy ok bit rides the readability flag")

		// No-limit mode uses a sample whose quota is not positive (the cpu.max
		// "max" case reads Known(0)) — a no-limit box never carries a positive
		// quota on its sample.
		engineNL, err := NewEngine(8, 0)
		Expect(err).NotTo(HaveOccurred())
		envNL := diagnosis.NewEnvironment()
		smpNL := smp
		smpNL.Quota = diagnosis.Known(0)
		_, sigNL := Decide(engineNL, smpNL, envNL)
		Expect(sigNL.CapacityCores).To(Equal(8.0), "no-limit mode: capacity is the logical CPU count")
		Expect(sigNL.ReserveCores).To(Equal(1.0), "no-limit mode: reserve is cpuReserveCores")
	})
})

var _ = Describe("the healthy message reports only what it measured", func() {
	It("should not report host usage or headroom when the host reading is absent", func() {
		details := healthyDetails()
		details.LimitApplies = false
		details.CapacityCores = 8
		details.AvgHostBusyCores = 0.0
		details.ReserveCores = 1.0
		details.HostBusyCoresAvailable = false // read failed, window still full
		msg := composeHealthy(details)
		Expect(msg).To(HavePrefix(cpuStartingUp + technicalDetailsLabel))
		Expect(msg).To(ContainSubstring("Headroom not available (measuring)."),
			"the headroom line states no figure either, and says which kind of absence it is")
	})

	It("should not report the limit-mode usage figure when the container's own window holds too few samples to reduce, even though the reading succeeded", func() {
		details := healthyDetails()
		details.UsageRingActive = false
		msg := composeHealthy(details)
		Expect(msg).To(HavePrefix(cpuStartingUp + technicalDetailsLabel))
		Expect(msg).To(ContainSubstring("Headroom not available (measuring)."))
	})

	It("should not report the no-limit usage figure when the host window holds too few samples to reduce, even though the reading succeeded", func() {
		details := healthyDetails()
		details.LimitApplies = false
		details.CapacityCores = 8
		details.AvgHostBusyCores = 0.0
		details.ReserveCores = 1.0
		details.HostBusyCoresAvailable = true
		details.HostBusyRingActive = false
		msg := composeHealthy(details)
		Expect(msg).To(HavePrefix(cpuStartingUp + technicalDetailsLabel))
		Expect(msg).To(ContainSubstring("Headroom not available (measuring)."))
	})

	It("should render no headline at all on a tick whose usage figure is withheld, returning through the same single-line path the zero-capacity guard uses rather than a headline with a hole in it", func() {
		details := healthyDetails()
		details.UsageRingActive = false
		msg := composeHealthy(details)
		Expect(msg).To(HavePrefix("CPU: starting up." + technicalDetailsLabel))
		Expect(msg).NotTo(ContainSubstring("CPU healthy."))

		// The table comes with it, and every line says which kind of absence it
		// is rather than stating a figure nothing measured. On the first ticks
		// that is what an operator wants: the rules that will be judged, and how
		// far off each reading is.
		startup := healthyDetails()
		startup.UsageRingActive = false
		startup.HostBusyRingActive = false
		startup.ThrottleSignalReady = false
		startup.PressureSignalReady = false
		startup.StealSignalReady = false
		lines := tableLines(composeHealthy(startup))
		Expect(tableRules(composeHealthy(startup))).To(Equal(allRules))
		for _, line := range lines {
			Expect(line).To(SatisfyAny(
				HaveSuffix("not available (measuring)."),
				HaveSuffix("not available (not possible)."),
			), "a box that has measured nothing yet may state no figure, and this line does: %q", line)
		}

		// The floors are per measurement, not one flag: a limit-mode headline uses
		// the container's usage-cores, so a thin host-busy window must NOT
		// withhold it.
		limitOK := healthyDetails()
		limitOK.HostBusyRingActive = false
		Expect(composeHealthy(limitOK)).To(ContainSubstring("CPU healthy."))
	})
})

// limitedVisibilityDetails builds the healthy bag of the one box the usage rule
// runs on: no CPU limit to judge our own budget against, and no pressure
// statistics to read the harm off, so our own usage over the machine's cores is
// the last evidence left that the machine is full.
func limitedVisibilityDetails() Details {
	d := healthyDetails()
	d.LimitApplies = false
	d.LimitedVisibility = true
	d.CapacityCores = 4
	d.ReserveCores = 1.0
	d.AvgHostBusyCores = 1.2
	d.AvgUsageFraction = 0.30
	d.ThrottleSignalReady = false
	d.PressureApplies = false
	d.PressureSignalReady = false

	return d
}

// The Technical Details table. Every threshold asserted below is computed from
// the mark pair the signal declares, never written as a literal. A spec that
// pinned the literal would stay green after someone moved the mark and the
// message started stating a rule the code no longer applies, which is the
// defect this table exists to close: it would pin the duplication rather than
// the constant.
var _ = Describe("the Technical Details table", func() {
	It("should state each rule's own fire mark, read from the signal that owns it, while the rule has not fired", func() {
		all := healthyDetails()
		all.ThrottleRatio = 0.02
		all.PressureAvg60 = 0.05
		all.StealP95 = 0.30
		all.PressureApplies = true
		all.StealApplies = true
		msg := composeHealthy(all)

		Expect(msg).To(ContainSubstring("Headroom 1.8 cores = 2 total - 0.0 used - 0.2 reserved (degrades below %s).",
			fmtCoresTotal(limitHeadroomMarks(all.CapacityCores).Fire.At)))
		Expect(msg).To(ContainSubstring("Throttling 2%% (degrades above %d%%).", toPercent(throttleMarks.Fire.At)))
		Expect(msg).To(ContainSubstring("Pressure 5%% (degrades above %d%%).", toPercent(pressureMarks.Fire.At)))
		Expect(msg).To(ContainSubstring("Steal 30%% (degrades above %d%%).", toPercent(stealMarks.Fire.At)))
	})

	It("should state the mark that clears a rule once it has latched, because that is the number deciding what happens next", func() {
		latched := degradedSig()
		latched.PressureAvg60 = 0.15
		msg := ComposeMessage(degradedVerdict(CauseKindPressure, instrumentPressureAvg60, 0.15), latched)

		Expect(msg).To(ContainSubstring("Pressure 15%% (recovers below %d%%).", toPercent(pressureMarks.Clear.At)))
		Expect(msg).NotTo(ContainSubstring("Pressure 15% (degrades"),
			"the reading has fallen back under the fire mark while the latch holds, so naming the fire mark would read as a contradiction")
	})

	It("should report the usage rule, the second way a machine can be judged full, so a box degraded by it is not shown a table in which every listed rule looks fine", func() {
		// The rule fires on our own usage over the machine's cores, and it is
		// the half the table used to omit: a box degraded by it saw a headroom
		// line sitting comfortably above its mark and nothing else.
		quiet := limitedVisibilityDetails()
		Expect(composeHealthy(quiet)).To(ContainSubstring("Usage 30%% of capacity (degrades at %d%%).",
			toPercent(usageFractionMarks.Fire.At)),
			"the mark is inclusive, so 70%% of the machine busy is already a full machine and the line reads \"at\"")

		fired := limitedVisibilityDetails()
		fired.AvgUsageFraction = 0.75
		msg := ComposeMessage(degradedVerdict(CauseKindHostCpuFull, instrumentUsageFraction, 0.75), fired)
		Expect(msg).To(ContainSubstring("Usage 75%% of capacity (recovers below %d%%).",
			toPercent(usageFractionMarks.Clear.At)))

		// A latch is proof the rule ran on this box, so a fired rule is never
		// reported as one the box cannot run.
		Expect(ComposeMessage(degradedVerdict(CauseKindHostCpuFull, instrumentUsageFraction, 0.8), degradedSig())).
			NotTo(ContainSubstring("Usage not available (not possible)."))
	})

	It("should carry all five rules, in one order, in every state that has a table", func() {
		Expect(tableRules(composeHealthy(healthyDetails()))).To(Equal(allRules))
		Expect(tableRules(ComposeMessage(degradedVerdict(CauseKindSteal, instrumentStealMean, 0.18), degradedSig()))).
			To(Equal(allRules))

		thin := healthyDetails()
		thin.UsageRingActive = false
		Expect(tableRules(composeHealthy(thin))).To(Equal(allRules),
			"a rule dropped from the table is a rule the reader takes for fine")
	})

	It("should say which kind of absence a missing reading is, because a kernel that reports no pressure statistics and a window still filling send a reader to different places", func() {
		bare := healthyDetails()
		bare.PressureApplies = false
		bare.PressureSignalReady = false
		Expect(composeHealthy(bare)).To(ContainSubstring("Pressure not available (not possible)."))

		thin := healthyDetails()
		thin.PressureApplies = true
		thin.PressureSignalReady = false
		Expect(composeHealthy(thin)).To(ContainSubstring("Pressure not available (measuring)."))
	})

	It("should read each rule's figure from its signal's readiness, and print no figure at all for a rule the box cannot run", func() {
		// A virtualized box (StealApplies true) whose steal window has no usable
		// value this tick must not print a confident 0% steal line.
		vm := healthyDetails()
		vm.StealApplies = true
		vm.StealSignalReady = false
		msg := composeHealthy(vm)
		Expect(msg).To(ContainSubstring("Steal not available (measuring)."))
		Expect(msg).NotTo(ContainSubstring("Steal 0%"))
		Expect(msg).To(ContainSubstring("Headroom"))

		// The throttle gate is readiness, not LimitApplies.
		noThrottle := healthyDetails()
		noThrottle.LimitApplies = true
		noThrottle.ThrottleSignalReady = false
		Expect(composeHealthy(noThrottle)).To(ContainSubstring("Throttling not available (measuring)."))

		// The two halves cannot stand in for each other in the other direction
		// either: bare metal declares no steal instrument, so a steal figure
		// there would state a rule that can never fire. The readiness flag is
		// set here anyway, which no real box does, to pin the capability as the
		// thing withholding the figure.
		bareMetal := healthyDetails()
		bareMetal.StealApplies = false
		bareMetal.StealSignalReady = true
		bareMetal.StealP95 = 0.30
		Expect(composeHealthy(bareMetal)).To(ContainSubstring("Steal not available (not possible)."))
		Expect(composeHealthy(bareMetal)).NotTo(ContainSubstring("Steal 30%"))
	})

	It("should carry no table at all when the cgroup read failed, because nothing on that tick says which rules apply", func() {
		healthy := healthyDetails()
		healthy.CapacityCores = 0
		Expect(composeHealthy(healthy)).To(Equal(cpuMonitoringUnavailable))

		degraded := degradedSig()
		degraded.CapacityCores = 0
		Expect(ComposeMessage(degradedVerdict(CauseKindContainerLimitFull, instrumentLimitHeadroom, 0.5), degraded)).
			NotTo(ContainSubstring("Technical Details:"))
	})
})

// degradedSig returns a Details with the headroom family populated, so each
// assertion varies one field against a fixed background. Which capacity cause
// fired is on the Verdict, not here.
func degradedSig() Details {
	return Details{
		CapacityCores:          4.0,
		AvgUsageCores:          0.5,
		AvgHostBusyCores:       1.0,
		HostBusyCoresAvailable: true,
		ReserveCores:           1.0,
		LimitApplies:           true,
		PressureApplies:        true,
		// A box that has run long enough to degrade has filled its windows, so
		// the Technical Details table reads figures rather than a column of
		// "measuring".
		UsageRingActive:     true,
		HostBusyRingActive:  true,
		ThrottleSignalReady: true,
		PressureSignalReady: true,
		StealApplies:        true,
		StealSignalReady:    true,
	}
}

// degradedVerdict builds a one-cause degraded verdict. The instrument is what
// the two capacity kinds dispatch on, so it is named at every call.
func degradedVerdict(kind CauseKind, instrument string, value float64) Verdict {
	return Verdict{State: StateDegraded, Attribution: AttributionHost, Causes: []Cause{{Kind: kind, Instrument: instrument, Attribution: AttributionHost, Value: value}}}
}

// bothCapacityVerdict builds the degraded verdict of a tick on which the
// machine is full and this container is also out of its own CPU limit.
func bothCapacityVerdict(value float64) Verdict {
	return Verdict{State: StateDegraded, Attribution: AttributionHost, Causes: []Cause{
		{Kind: CauseKindHostCpuFull, Instrument: instrumentHostHeadroom, Attribution: AttributionHost, Value: value},
		{Kind: CauseKindContainerLimitFull, Instrument: instrumentLimitHeadroom, Attribution: AttributionHost, Value: value},
	}}
}

var _ = Describe("degraded copy", func() {
	It("should render one headline per cause kind", func() {
		Expect(causeHeadline(CauseKindThrottling)).To(Equal("CPU limited"))
		Expect(causeHeadline(CauseKindPressure)).To(Equal("CPU contention"))
		Expect(causeHeadline(CauseKindSteal)).To(Equal("CPU taken by the server"))
		Expect(causeHeadline(CauseKindHostCpuFull)).To(Equal("CPU running near full"))
		// headlineGeneric: the default arm, unreachable through today's five kinds but
		// still written so the enum can grow.
		Expect(causeHeadline(CauseKind("future-kind"))).To(Equal("CPU degraded"))
	})

	It("should state the steal figure as a plain share of the last minute, claiming no peak", func() {
		// A steal cause can carry the mean of the last minute, not only the
		// percentile: describeCause reports whichever arm the episode fired on, and
		// the mean is the arm that fires in the first twenty seconds. "At peak"
		// would be a claim about a percentile, so the sentence makes none.
		// Asserted whole, because the wording is customer-visible copy.
		//
		// The steal line of the table reports the same 18% as the paragraph
		// above it. Details.StealP95 names the percentile, which reads 0 until
		// the window holds twenty samples, so a latched episode reports the arm
		// it fired on instead; the two would otherwise contradict each other
		// three lines apart.
		msg := ComposeMessage(degradedVerdict(CauseKindSteal, instrumentStealMean, 0.18), degradedSig())
		Expect(msg).To(Equal("CPU taken by the server\n" +
			"Other virtual machines on the same physical server took 18% of the CPU this instance needed over the last minute. This is outside UMH's control. On your virtualization platform, give this VM more guaranteed CPU, or reduce the other VMs sharing the server." +
			"\nTechnical Details:\n" +
			"Headroom 2.5 cores = 4 total - 0.5 used - 1.0 reserved (degrades below 0).\n" +
			"Usage not available (not possible).\n" +
			"Throttling 0% (degrades above 5%).\n" +
			"Pressure 0% (degrades above 20%).\n" +
			"Steal 18% (recovers below 6%)."))
	})

	It("should render the curated detail paragraph for each fired cause, dominant first", func() {
		verdict := Verdict{State: StateDegraded, Attribution: AttributionHost, Causes: []Cause{
			{Kind: CauseKindPressure, Value: 0.40},
			{Kind: CauseKindThrottling},
		}}
		msg := ComposeMessage(verdict, degradedSig())
		head := strings.Index(msg, "CPU contention")
		p1 := strings.Index(msg, "Tasks in this instance spent 40% of the last minute waiting for a free CPU core.")
		Expect(head).To(BeNumerically(">=", 0))
		Expect(p1).To(BeNumerically(">", head), "the dominant (pressure) paragraph comes first")
		Expect(msg).To(ContainSubstring("This instance hit its CPU limit and was paused until the next cycle"))
		// Two detail paragraphs are joined by a blank line, not a space.
		Expect(msg).To(ContainSubstring("\n\n"))
	})

	It("should dispatch the capacity paragraph on the cause it is handed and append the two clauses rather than replacing their paragraphs", func() {
		// The machine read full, with a limit in force and the container not
		// at it, entry 30.
		msg := ComposeMessage(degradedVerdict(CauseKindHostCpuFull, instrumentHostHeadroom, 0.5), degradedSig())
		Expect(msg).To(ContainSubstring("The machine is full. Add CPU to the machine, or reduce other software running on it."))

		// The machine read full AND the container at its limit, entry 29 with
		// the limit: one blended sentence, not two paragraphs.
		hfl := degradedSig()
		hfl.CapacityCores = 2
		msg = ComposeMessage(bothCapacityVerdict(0.5), hfl)
		Expect(msg).To(ContainSubstring("The machine is full and this instance's CPU limit cannot help. Add CPU to the machine, or reduce other software running on it. (This instance is also at its 2-core limit.)"))
		Expect(msg).NotTo(ContainSubstring("\n\n"), "the blend replaces both paragraphs; it does not precede one")

		// The container at its own limit, entry 33, with entry 34 appended when
		// host stats are unreadable (the clause is appended, not a replacement).
		ls := degradedSig()
		ls.AvgUsageCores = 1.9
		ls.CapacityCores = 2.0
		ls.HostBusyCoresAvailable = false
		msg = ComposeMessage(degradedVerdict(CauseKindContainerLimitFull, instrumentLimitHeadroom, 0.5), ls)
		Expect(msg).To(ContainSubstring("CPU averaged 95% of its limit over the last minute and this instance has little headroom left. Raise its CPU limit, or reduce the load on it."))
		Expect(msg).To(ContainSubstring(" Host stats are unavailable, so host-side contention is not visible."))

		// The machine read full with no limit in force and the host reading
		// gone, entry 35.
		nl6 := degradedSig()
		nl6.LimitApplies = false
		nl6.HostBusyCoresAvailable = false
		msg = ComposeMessage(degradedVerdict(CauseKindHostCpuFull, instrumentHostHeadroom, 0.5), nl6)
		Expect(msg).To(ContainSubstring("CPU is degraded. Host CPU usage is not readable right now (host stats temporarily unavailable), so the host-busy percentage cannot be shown. Add CPU capacity, or reduce the load on it."))

		// The same with the host reading present, entry 36, with entry 37
		// appended when LimitedVisibility.
		arm7 := degradedSig()
		arm7.LimitApplies = false
		arm7.AvgHostBusyCores = 3.8
		arm7.CapacityCores = 4.0
		arm7.LimitedVisibility = true
		msg = ComposeMessage(degradedVerdict(CauseKindHostCpuFull, instrumentHostHeadroom, 0.5), arm7)
		Expect(msg).To(ContainSubstring("CPU averaged 95% of the machine over the last minute and this instance has little headroom left. Add CPU capacity, or reduce the load on it."))
		Expect(msg).To(ContainSubstring(" Pressure stats are unavailable; enable Linux pressure stats (boot with psi=1) for richer detail."))

		// The machine estimated full from our own usage, with and without PSI.
		// The PressureApplies-false arm is the one that EARNS the psi advice.
		on := degradedSig()
		msg = ComposeMessage(degradedVerdict(CauseKindHostCpuFull, instrumentUsageFraction, 0.8), on)
		Expect(msg).To(ContainSubstring("CPU averaged 80% of the machine over the last minute and this instance has little headroom left. Host contention is not visible here (host CPU usage is not readable). Consider adding CPU capacity."))
		Expect(msg).NotTo(ContainSubstring("Enable Linux pressure stats"))

		off := degradedSig()
		off.PressureApplies = false
		msg = ComposeMessage(degradedVerdict(CauseKindHostCpuFull, instrumentUsageFraction, 0.8), off)
		Expect(msg).To(ContainSubstring("Enable Linux pressure stats (boot with psi=1) for richer detail. Consider adding CPU capacity."))
	})

	It("should not render an unbounded or silently-wrong percentage when a capacity cause's capacity reads zero", func() {
		// The limit arm: AvgUsageCores/CapacityCores with CapacityCores == 0 is
		// +Inf, and toPercent's int(math.Round(...)) conversion of +Inf is
		// implementation-defined — on this platform it comes out as
		// math.MaxInt64, a 19-digit percentage. Reachable when the limit-mode
		// quota-based signal fires from its own frozen quota while the sample's
		// own Quota reads unknown/zero on the same tick and LogicalCpus is also
		// unset, so buildDetails's fallback leaves CapacityCores at 0.
		limitArm := degradedSig()
		limitArm.CapacityCores = 0
		limitArm.AvgUsageCores = 2.0
		msg := ComposeMessage(degradedVerdict(CauseKindContainerLimitFull, instrumentLimitHeadroom, 0.5), limitArm)
		Expect(msg).NotTo(ContainSubstring("9223372036854775807"))
		Expect(msg).NotTo(ContainSubstring("%"))
		Expect(msg).To(ContainSubstring("not currently readable"))

		// The default (readable no-limit) arm: AvgHostBusyCores/CapacityCores
		// with CapacityCores == 0 is 0/0 = NaN, and toPercent(NaN) happens to come
		// out as 0 on this platform — a silently wrong "0%" that looks
		// plausible but was never measured.
		defaultArm := degradedSig()
		defaultArm.LimitApplies = false
		defaultArm.HostBusyCoresAvailable = true
		defaultArm.CapacityCores = 0
		defaultArm.AvgHostBusyCores = 0.0
		msg = ComposeMessage(degradedVerdict(CauseKindHostCpuFull, instrumentHostHeadroom, 0.5), defaultArm)
		Expect(msg).NotTo(ContainSubstring("CPU averaged 0% of the machine"))
		Expect(msg).To(ContainSubstring("not currently readable"))
	})

	It("should render the generic degraded paragraph for an unknown cause kind", func() {
		msg := ComposeMessage(degradedVerdict(CauseKind("future-kind"), "", 0.5), degradedSig())
		Expect(msg).To(ContainSubstring("CPU is degraded."))
	})
})

// oneCause wraps a single cause as the ranked list BlockReason reads.
func oneCause(kind CauseKind, instrument string, attribution Attribution) []Cause {
	return []Cause{{Kind: kind, Instrument: instrument, Attribution: attribution}}
}

var _ = Describe("block reasons", func() {
	It("should render one block reason per cause kind, dispatching the machine-full kind on the instrument that measured it", func() {
		Expect(BlockReason(oneCause(CauseKindThrottling, instrumentThrottleRatio, AttributionContainer), degradedSig())).To(Equal("Can't add another bridge: this instance is already hitting its CPU limit. Raise the limit or reduce load first."))
		Expect(BlockReason(oneCause(CauseKindPressure, instrumentPressureAvg60, AttributionUnknown), degradedSig())).To(Equal("Can't add another bridge: tasks on this instance are already waiting for a free CPU core. Reduce load, or give this instance more CPU, first."))
		Expect(BlockReason(oneCause(CauseKindSteal, instrumentStealP95, AttributionHost), degradedSig())).To(Equal("Can't add another bridge: the server isn't giving this instance enough CPU (other VMs are using it). Free up CPU on the server first."))
		// blockGeneric: the default kind arm.
		Expect(BlockReason(oneCause(CauseKind("future-kind"), "", AttributionUnknown), degradedSig())).To(Equal("Can't add another bridge: CPU is degraded."))

		noLimit := degradedSig()
		noLimit.LimitApplies = false

		Expect(BlockReason(oneCause(CauseKindHostCpuFull, instrumentHostHeadroom, AttributionHost), degradedSig())).To(Equal("Can't add another bridge: the machine is full. Add CPU to the machine, or reduce other software running on it, first."))
		Expect(BlockReason(oneCause(CauseKindContainerLimitFull, instrumentLimitHeadroom, AttributionContainer), degradedSig())).To(Equal("Can't add another bridge: this instance is at its CPU limit. Raise the limit, or reduce the load, first."))
		Expect(BlockReason(oneCause(CauseKindHostCpuFull, instrumentUsageFraction, AttributionUnknown), degradedSig())).To(Equal("Can't add another bridge: CPU is running near full and host stats are unavailable. Add CPU capacity, or set a CPU limit, first."))
		Expect(BlockReason(oneCause(CauseKindHostCpuFull, instrumentHostHeadroom, AttributionHost), noLimit)).To(Equal("Can't add another bridge: the machine is full. Add CPU to the machine, or reduce other software running on it, first."))
		// The machine-full default arm (entry 46), reachable only from a cause
		// whose instrument is none of the two that can measure the machine.
		Expect(BlockReason(oneCause(CauseKindHostCpuFull, "", AttributionUnknown), degradedSig())).To(Equal("Can't add another bridge: CPU is running near full. Add CPU capacity, or set a CPU limit, first."))

		// blockHostFull and blockNoLimitHost are byte-identical and the collision is intentional
		// and must survive (the remediation for a full machine is the same with
		// or without a limit).
		Expect(BlockReason(oneCause(CauseKindHostCpuFull, instrumentHostHeadroom, AttributionHost), degradedSig())).
			To(Equal(BlockReason(oneCause(CauseKindHostCpuFull, instrumentHostHeadroom, AttributionHost), noLimit)))
	})

	It("conformance: Decide names the host-headroom instrument on a full host in no-limit mode", func() {
		// No-limit mode: a full host with our own load filling it. The
		// attribution is unknown, but the machine's own reading is what fired,
		// and Details carries the mode rather than a second name for the arm.
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment()
		base := time.Now()
		for i := 0; i < 3; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Quota:       diagnosis.Known(0),
				HostBusy:    diagnosis.Known(3.8),
				LogicalCpus: diagnosis.Known(4),
				HostCpus:    diagnosis.Known(4),
				UsageCores:  diagnosis.Known(3.6),
			}
			verdict, details := Decide(engine, smp, env)
			if i == 2 {
				Expect(verdict.Causes).To(HaveLen(1))
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindHostCpuFull))
				Expect(verdict.Causes[0].Instrument).To(Equal(instrumentHostHeadroom))
				Expect(details.LimitApplies).To(BeFalse(), "no quota applies, so the no-limit wording is the one that renders")
			}
		}
	})

	It("should name the same capacity cause on both customer-facing surfaces when a full machine meets a container at its limit", func() {
		// 4 cores, a 2.0-core quota, host busy 3.8, our usage 1.85, /proc/stat
		// readable. host-headroom reduces to 4 - 3.8 - 1.0 = -0.8 and
		// limit-headroom to 2 - 1.85 - 0.2 = -0.05, so both latches hold on the
		// same tick. The machine's own reading is the measured one, so it is
		// the cause both surfaces speak with.
		//
		// 1.85 / 3.80 = 0.4868 blames the host, and the run needs that: each
		// candidate below is rendered on its own, where the pair that blends
		// the two remedies is not there to be found, so a candidate blamed on
		// the container would render advice the real tick never printed.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)

		base := time.Now()
		var verdict Verdict
		var details Details
		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Quota:       diagnosis.Known(2.0),
				HostBusy:    diagnosis.Known(3.8),
				UsageCores:  diagnosis.Known(1.85),
				LogicalCpus: diagnosis.Known(4),
				HostCpus:    diagnosis.Known(4),
			}
			verdict, details = Decide(engine, smp, env)
		}

		// The two capacity signals produce two causes of two kinds. Collapsing
		// them to one kind is what let two different situations render the same
		// paragraph, so the split is asserted here before anything is rendered.
		Expect(kindsOf(verdict.Causes)).To(ConsistOf(CauseKindHostCpuFull, CauseKindContainerLimitFull))
		Expect(causeOfKind(verdict.Causes, CauseKindHostCpuFull).Instrument).To(Equal(instrumentHostHeadroom),
			"a readable /proc/stat is measured by host-headroom")
		Expect(causeOfKind(verdict.Causes, CauseKindContainerLimitFull).Instrument).To(Equal(instrumentLimitHeadroom))

		// Recover the cause each surface spoke with, without naming a sentence:
		// re-render that surface for each candidate cause and match against what
		// the surface actually printed for the real tick. Both sides read the
		// same constants, so editing a literal moves them together and cannot
		// silence a disagreement.
		//
		// The third candidate is the machine measured by the estimate from our
		// own usage instead. No box can produce it beside a limit, which is what
		// makes it a clean distractor: it must not match either surface.
		candidates := []Cause{
			causeOfKind(verdict.Causes, CauseKindHostCpuFull),
			causeOfKind(verdict.Causes, CauseKindContainerLimitFull),
			{Kind: CauseKindHostCpuFull, Instrument: instrumentUsageFraction},
		}
		name := func(c Cause) string { return string(c.Kind) + "/" + c.Instrument }
		spokeWith := func(surface string, printed string, render func(Cause) string) string {
			var matched []string
			for _, candidate := range candidates {
				if render(candidate) == printed {
					matched = append(matched, name(candidate))
				}
			}
			Expect(matched).To(HaveLen(1),
				"%s printed %q, which %d of the three candidate causes render, so the cause it spoke with cannot be recovered",
				surface, printed, len(matched))

			return matched[0]
		}

		// The two surfaces are probed differently because they differ in what a
		// single cause can produce. causeDetails blends the pair into one
		// sentence, so it is handed the real cause list every time and only the
		// speaker varies. BlockReason has no blended line, so a candidate on its
		// own renders exactly what it would have said.
		detailsSpeaker := spokeWith("the technical details",
			causeDetails(speakingCause(verdict.Causes), verdict.Causes, details),
			func(speaker Cause) string { return causeDetails(speaker, verdict.Causes, details) })
		blockSpeaker := spokeWith("the bridge-refusal reason",
			BlockReason(verdict.Causes, details),
			func(speaker Cause) string { return BlockReason([]Cause{speaker}, details) })

		Expect(detailsSpeaker).To(Equal(blockSpeaker),
			"the two surfaces disagree on one tick: the technical details blame %s while the bridge refusal blames %s, so a customer is given two contradictory remedies",
			detailsSpeaker, blockSpeaker)
		Expect(detailsSpeaker).To(Equal(name(candidates[0])),
			"the machine's own reading is the measured one, so it is the cause both surfaces speak with")
	})
})
