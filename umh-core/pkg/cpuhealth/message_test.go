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

// S4 R1 — the healthy headline. composeHealthy renders the two-layer healthy
// budget dashboard. The headline is a two-by-two dispatch over
// (LimitApplies && rounded total > 0) and (displayed headroom < 0.05); the
// subject follows the mode and not the column; the displayed headroom is
// derived from the already-rounded total/used/reserve. The R2 withholding
// ("CPU: starting up.") and the R3 readiness-gated budget lines are later
// rungs, so their fields (UsageRingActive/HostBusyRingActive/proc-readability)
// are set true here to keep the assertions stable across the ladder.
package cpuhealth

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// healthySig builds a Signals bag with every healthy-message input in a
// usable state, so individual fields can be overridden per assertion.
func healthySig() Signals {
	return Signals{
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

var _ = Describe("S4 R1 — the healthy headline", func() {
	It("should render the headline in limit mode with a percentage, and in no-limit mode with 'The machine' as subject", func() {
		// Limit mode: entry 9 "CPU healthy. This instance is using %s of %s cores (%d%% of its limit) and can use %s more before it is marked degraded."
		limit := healthySig()
		Expect(composeHealthy(limit)).To(ContainSubstring(
			"CPU healthy. This instance is using 0.0 of 2 cores (0% of its limit) and can use 1.8 more before it is marked degraded."))

		// No-limit mode: entry 13 with subject entry 10 "The machine"; used is host-busy.
		nolimit := healthySig()
		nolimit.LimitApplies = false
		nolimit.CapacityCores = 8
		nolimit.AvgUsageCores = 0
		nolimit.HostBusyCores60sMean = 0.0
		nolimit.ReserveCores = 1.0
		Expect(composeHealthy(nolimit)).To(ContainSubstring(
			"CPU healthy. The machine is using 0.0 of 8 cores and can use 7.0 more before it is marked degraded."))
	})

	It("should derive the displayed headroom from the already-rounded total, used and reserve so the printed arithmetic is exact", func() {
		sig := healthySig()
		sig.AvgUsageCores = 0.3
		msg := composeHealthy(sig)
		// 2 total - 0.3 used - 0.2 reserved = 1.5, printed exactly.
		Expect(msg).To(ContainSubstring("Headroom 1.5 cores = 2 total - 0.3 used - 0.2 reserved (degraded below 0)."))
		Expect(msg).To(ContainSubstring("can use 1.5 more before it is marked degraded."))
	})

	It("should omit the percentage suffix when the rounded total is zero, as a sub-0.05-core quota produces", func() {
		sig := healthySig()
		sig.CapacityCores = 0.04 // rounds to 0.0 total
		sig.AvgUsageCores = 0.01
		sig.ReserveCores = 0.0
		msg := composeHealthy(sig)
		// Entry 12 with subject 11: no "(N% of its limit)" suffix, "This instance".
		Expect(msg).To(ContainSubstring("CPU healthy. This instance is using 0.0 of 0 cores and is close to being marked degraded."))
		Expect(msg).NotTo(ContainSubstring("% of its limit"))
	})

	It("should say the instance is close to being marked degraded once the displayed headroom falls below 0.05 cores, instead of offering more", func() {
		sig := healthySig()
		sig.AvgUsageCores = 1.8 // headroom 2.0-1.8-0.2 = 0.0 < 0.05
		Expect(composeHealthy(sig)).To(ContainSubstring("and is close to being marked degraded."))
		Expect(composeHealthy(sig)).NotTo(ContainSubstring("and can use"))

		above := healthySig()
		above.AvgUsageCores = 0.0 // headroom 1.8 >= 0.05
		Expect(composeHealthy(above)).To(ContainSubstring("and can use 1.8 more before it is marked degraded."))
	})

	It("should put the limited-visibility advisory between the headline and the technical details when the dead-zone annotation is set", func() {
		sig := healthySig()
		sig.LimitedVisibility = true
		msg := composeHealthy(sig)
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
		sig := healthySig()
		sig.CapacityCores = 0
		Expect(composeHealthy(sig)).To(Equal(
			"CPU monitoring unavailable: cgroup read failed. Defaulting to healthy."))
	})

	It("should say host headroom is unavailable, naming both core counts, when the container's count describes only the CPUs it may run on", func() {
		sig := healthySig()
		sig.HostHeadroomAvailable = false
		sig.HostCpus = 8
		sig.LogicalCpus = 2
		msg := composeHealthy(sig)
		Expect(msg).To(ContainSubstring("host headroom unavailable: this container is pinned to 2 of 8 CPUs"))
		// It is an advisory-slot line, not the whole message.
		Expect(msg).To(ContainSubstring("CPU healthy."))
		Expect(msg).To(ContainSubstring("Technical Details:"))
		// On ScopeUnknown HostCpus is 0 (bare float64, unknown Get leaves it 0),
		// so the sentence is not rendered for an unknown machine count.
		unknown := healthySig()
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
			// PsiAvailable false => dead zone; supply pressure to keep it healthy.
			Pressure: diagnosis.Known(0.0),
		}
		_, sig := Decide(engine, smp, env)
		Expect(sig.CapacityCores).To(Equal(2.0), "limit mode: capacity is the quota")
		Expect(sig.ReserveCores).To(Equal(0.2), "limit mode: reserve is 0.10 x quota")
		Expect(sig.HostBusyCoresAvailable).To(BeTrue(), "the sample's HostBusy ok bit rides the readability flag")

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

var _ = Describe("S4 R2 — the healthy message reports only what it measured", func() {
	It("should not report host usage or headroom when the host reading is absent", func() {
		sig := healthySig()
		sig.LimitApplies = false
		sig.CapacityCores = 8
		sig.HostBusyCores60sMean = 0.0
		sig.ReserveCores = 1.0
		sig.HostBusyCoresAvailable = false // read failed, window still full
		Expect(composeHealthy(sig)).To(Equal("CPU: starting up."))
	})

	It("should not report the limit-mode usage figure when the container's own window holds too few samples to reduce, even though the reading succeeded", func() {
		sig := healthySig()
		sig.UsageRingActive = false
		Expect(composeHealthy(sig)).To(Equal("CPU: starting up."))
	})

	It("should not report the no-limit usage figure when the host window holds too few samples to reduce, even though the reading succeeded", func() {
		sig := healthySig()
		sig.LimitApplies = false
		sig.CapacityCores = 8
		sig.HostBusyCores60sMean = 0.0
		sig.ReserveCores = 1.0
		sig.HostBusyCoresAvailable = true
		sig.HostBusyRingActive = false
		Expect(composeHealthy(sig)).To(Equal("CPU: starting up."))
	})

	It("should render no headline at all on a tick whose usage figure is withheld, returning through the same single-line path the zero-capacity guard uses rather than a headline with a hole in it", func() {
		sig := healthySig()
		sig.UsageRingActive = false
		msg := composeHealthy(sig)
		Expect(msg).To(Equal("CPU: starting up."))
		Expect(msg).NotTo(ContainSubstring("CPU healthy."))
		Expect(msg).NotTo(ContainSubstring("Technical Details:"))

		// The floors are per track, not one flag: a limit-mode headline uses
		// the container's usage-cores, so a thin host-busy window must NOT
		// withhold it.
		limitOK := healthySig()
		limitOK.HostBusyRingActive = false
		Expect(composeHealthy(limitOK)).To(ContainSubstring("CPU healthy."))
	})
})

var _ = Describe("S4 R3 — the budget lines", func() {
	It("should list the headroom budget always, and each of throttle, pressure and steal only when this tick's reading is usable", func() {
		all := healthySig()
		all.ThrottleRatio = 0.02
		all.PressureAvg60Out = 0.05
		all.StealP95 = 0.30
		msg := composeHealthy(all)
		// Headroom is the only unconditional line.
		Expect(msg).To(ContainSubstring("Headroom 1.8 cores = 2 total - 0.0 used - 0.2 reserved (degraded below 0)."))
		Expect(msg).To(ContainSubstring("Throttling 2% (degraded above 5%)."))
		Expect(msg).To(ContainSubstring("Pressure 5% (degraded above 20%)."))
		Expect(msg).To(ContainSubstring("Steal 30% (degraded above 10%)."))
	})

	It("should print each budget line from its signal's readiness, never from the capability flag", func() {
		// A virtualized box (StealApplies true) whose steal window has no usable
		// value this tick must not print a confident 0% steal line.
		vm := healthySig()
		vm.StealApplies = true
		vm.StealSignalReady = false
		msg := composeHealthy(vm)
		Expect(msg).NotTo(ContainSubstring("Steal"))
		Expect(msg).To(ContainSubstring("Pressure"))
		Expect(msg).To(ContainSubstring("Headroom"))

		// The throttle gate is readiness, not LimitApplies.
		noThrottle := healthySig()
		noThrottle.LimitApplies = true
		noThrottle.ThrottleSignalReady = false
		Expect(composeHealthy(noThrottle)).NotTo(ContainSubstring("Throttling"))

		// The pressure gate is readiness, not PsiApplies.
		noPressure := healthySig()
		noPressure.PsiApplies = true
		noPressure.PressureSignalReady = false
		Expect(composeHealthy(noPressure)).NotTo(ContainSubstring("Pressure"))

		// Readiness is the gate, not capability: a ready steal signal prints
		// even when StealApplies is false (they agree on bare metal, but this
		// pins the rung to the readiness trio).
		ready := healthySig()
		ready.StealApplies = false
		ready.StealSignalReady = true
		ready.StealP95 = 0.30
		Expect(composeHealthy(ready)).To(ContainSubstring("Steal 30% (degraded above 10%)."))
	})
})

// degradedSig returns a Signals with the headroom family populated and no
// saturation arm fired, so a single arm can be set per assertion.
func degradedSig() Signals {
	return Signals{
		CapacityCores:          4.0,
		AvgUsageCores:          0.5,
		HostBusyCores60sMean:   1.0,
		HostBusyCoresAvailable: true,
		ReserveCores:           1.0,
		LimitApplies:           true,
		PsiApplies:             true,
	}
}

// degradedVerdict builds a one-cause degraded verdict.
func degradedVerdict(kind CauseKind, value float64) Verdict {
	return Verdict{State: StateDegraded, Attribution: AttributionHost, Causes: []Cause{{Kind: kind, Value: value}}}
}

var _ = Describe("S4 R4 — degraded copy", func() {
	It("should render one headline per cause kind", func() {
		Expect(causeHeadline(CauseKindThrottling)).To(Equal("CPU limited"))
		Expect(causeHeadline(CauseKindPressure)).To(Equal("CPU contention"))
		Expect(causeHeadline(CauseKindSteal)).To(Equal("CPU taken by the server"))
		Expect(causeHeadline(CauseKindSaturation)).To(Equal("CPU running near full"))
		// Entry 25: the default arm, unreachable through today's five kinds but
		// still written so the enum can grow.
		Expect(causeHeadline(CauseKind("future-kind"))).To(Equal("CPU degraded"))
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

	It("should dispatch the saturation paragraph on which arm fired, in the fold's order, and append the two clauses rather than replacing their paragraphs", func() {
		// Arm 2 — HostFullFired alone, entry 30.
		msg := ComposeMessage(degradedVerdict(CauseKindSaturation, 0.5), func() Signals { s := degradedSig(); s.HostFullFired = true; return s }())
		Expect(msg).To(ContainSubstring("The machine is full. Add CPU to the machine, or reduce other software running on it."))

		// Arm 1 — HostFullFired AND LimitSaturationFired, entry 29 with the limit.
		hfl := degradedSig()
		hfl.HostFullFired = true
		hfl.LimitSaturationFired = true
		hfl.CapacityCores = 2
		msg = ComposeMessage(degradedVerdict(CauseKindSaturation, 0.5), hfl)
		Expect(msg).To(ContainSubstring("The machine is full and this instance's CPU limit cannot help. Add CPU to the machine, or reduce other software running on it. (This instance is also at its 2-core limit.)"))

		// Arm 5 — LimitSaturationFired, entry 33, with entry 34 appended when
		// host stats are unreadable (the clause is appended, not a replacement).
		ls := degradedSig()
		ls.LimitSaturationFired = true
		ls.AvgUsageCores = 1.9
		ls.CapacityCores = 2.0
		ls.HostBusyCoresAvailable = false
		msg = ComposeMessage(degradedVerdict(CauseKindSaturation, 0.5), ls)
		Expect(msg).To(ContainSubstring("CPU averaged 95% of its limit over the last minute and this instance has little headroom left. Raise its CPU limit, or reduce the load on it."))
		Expect(msg).To(ContainSubstring(" Host stats are unavailable, so host-side contention is not visible."))

		// Arm 6 — NoLimitHostFired with host unreadable, entry 35.
		nl6 := degradedSig()
		nl6.LimitApplies = false
		nl6.NoLimitHostFired = true
		nl6.HostBusyCoresAvailable = false
		msg = ComposeMessage(degradedVerdict(CauseKindSaturation, 0.5), nl6)
		Expect(msg).To(ContainSubstring("CPU is degraded. Host CPU usage is not readable right now (host stats temporarily unavailable), so the host-busy percentage cannot be shown. Add CPU capacity, or reduce the load on it."))

		// Arm 7 — a readable no-limit full host, entry 36 with entry 37 appended
		// when LimitedVisibility.
		arm7 := degradedSig()
		arm7.LimitApplies = false
		arm7.NoLimitHostFired = true
		arm7.HostBusyCores60sMean = 3.8
		arm7.CapacityCores = 4.0
		arm7.LimitedVisibility = true
		msg = ComposeMessage(degradedVerdict(CauseKindSaturation, 0.5), arm7)
		Expect(msg).To(ContainSubstring("CPU averaged 95% of the machine over the last minute and this instance has little headroom left. Add CPU capacity, or reduce the load on it."))
		Expect(msg).To(ContainSubstring(" Pressure stats are unavailable; enable Linux pressure stats (boot with psi=1) for richer detail."))

		// Arm 3 & 4 — no-host-stats with and without PSI. The PsiApplies-false
		// arm is the one that EARNS the psi advice.
		on := degradedSig()
		on.NoHostStatsSaturationFired = true
		msg = ComposeMessage(degradedVerdict(CauseKindSaturation, 0.8), on)
		Expect(msg).To(ContainSubstring("CPU averaged 80% of the machine over the last minute and this instance has little headroom left. Host contention is not visible here (host CPU usage is not readable). Consider adding CPU capacity."))
		Expect(msg).NotTo(ContainSubstring("Enable Linux pressure stats"))

		off := degradedSig()
		off.NoHostStatsSaturationFired = true
		off.PsiApplies = false
		msg = ComposeMessage(degradedVerdict(CauseKindSaturation, 0.8), off)
		Expect(msg).To(ContainSubstring("Enable Linux pressure stats (boot with psi=1) for richer detail. Consider adding CPU capacity."))
	})

	It("should render the generic degraded paragraph for an unknown cause kind", func() {
		msg := ComposeMessage(degradedVerdict(CauseKind("future-kind"), 0.5), degradedSig())
		Expect(msg).To(ContainSubstring("CPU is degraded."))
	})
})

var _ = Describe("S4 R5 — block reasons", func() {
	It("should render one block reason per cause kind, dispatching saturation on which member of its family survived the fold", func() {
		Expect(BlockReason(CauseKindThrottling, degradedSig())).To(Equal("Can't add another bridge: this instance is already hitting its CPU limit. Raise the limit or reduce load first."))
		Expect(BlockReason(CauseKindPressure, degradedSig())).To(Equal("Can't add another bridge: tasks on this instance are already waiting for a free CPU core. Reduce load, or give this instance more CPU, first."))
		Expect(BlockReason(CauseKindSteal, degradedSig())).To(Equal("Can't add another bridge: the server isn't giving this instance enough CPU (other VMs are using it). Free up CPU on the server first."))
		// Entry 47: the default kind arm.
		Expect(BlockReason(CauseKind("future-kind"), degradedSig())).To(Equal("Can't add another bridge: CPU is degraded."))

		hf := degradedSig()
		hf.HostFullFired = true
		nl := degradedSig()
		nl.NoLimitHostFired = true
		ls := degradedSig()
		ls.LimitSaturationFired = true
		ns := degradedSig()
		ns.NoHostStatsSaturationFired = true

		Expect(BlockReason(CauseKindSaturation, hf)).To(Equal("Can't add another bridge: the machine is full. Add CPU to the machine, or reduce other software running on it, first."))
		Expect(BlockReason(CauseKindSaturation, ls)).To(Equal("Can't add another bridge: this instance is at its CPU limit. Raise the limit, or reduce the load, first."))
		Expect(BlockReason(CauseKindSaturation, ns)).To(Equal("Can't add another bridge: CPU is running near full and host stats are unavailable. Add CPU capacity, or set a CPU limit, first."))
		Expect(BlockReason(CauseKindSaturation, nl)).To(Equal("Can't add another bridge: the machine is full. Add CPU to the machine, or reduce other software running on it, first."))
		// The saturation default arm (entry 46), reachable only if a saturation
		// latch fired with none of the four arm flags.
		impossible := degradedSig()
		impossible.SaturationFired = true
		Expect(BlockReason(CauseKindSaturation, impossible)).To(Equal("Can't add another bridge: CPU is running near full. Add CPU capacity, or set a CPU limit, first."))

		// Entries 42 and 45 are byte-identical and the collision is intentional
		// and must survive (the remediation for a full machine is the same with
		// or without a limit).
		Expect(BlockReason(CauseKindSaturation, hf)).To(Equal(BlockReason(CauseKindSaturation, nl)))
	})

	It("conformance: Decide sets NoLimitHostFired for a full host in no-limit mode and HostFullFired in limit mode", func() {
		// No-limit mode: a full host with our own load filling it (D1: the
		// attribution is unknown, but the host-headroom arm still fires and is
		// reported as NoLimitHostFired, not HostFullFired).
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
			_, sig := Decide(engine, smp, env)
			if i == 2 {
				Expect(sig.NoLimitHostFired).To(BeTrue(), "no-limit full host fires the no-limit-host arm")
				Expect(sig.HostFullFired).To(BeFalse(), "HostFullFired is the limit-mode name")
			}
		}
	})
})
