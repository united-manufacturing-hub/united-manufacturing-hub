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

// The fallback metric set. When host stats are absent AND nothing better can
// answer, the host-cpu-full signal falls back to usage-fraction:
// host-headroom's window empties past the demote span, selection walks to
// usage-fraction and JUDGES on it, and the number it judged on reaches Details
// as AvgUsageFraction. "Nothing better" is HasLimitedVisibility — no CPU limit
// and no PSI — which is the same condition Details.LimitedVisibility reports.
// Where PSI or a limit does exist, the estimate is not capable at all, so an
// unreadable /proc/stat leaves the signal with nothing to read. Limited
// visibility — quota nil or non-positive AND PSI absent — is an annotation on a
// healthy verdict, carried by Details.LimitedVisibility, never a state.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("the fallback metric set", func() {
	It("should fall back to usage against logical CPUs when host stats are absent", func() {
		// No quota and no PSI: the box where usage-fraction is allowed to
		// answer at all. Its Requires gates it on exactly that, so a fixture
		// carrying a limit would select nothing here.
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimitedVisibility)
		base := time.Now()
		sat := signalNamed(cpuTable(4, 0), "host-cpu-full")

		for i := 0; i < 130; i++ {
			hbKnown := i < 60
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				HostBusy:    diagnosis.Unknown(),
				UsageCores:  diagnosis.Known(0.4),
				Pressure:    diagnosis.Known(0),
				Steal:       diagnosis.Known(0),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}
			if hbKnown {
				smp.HostBusy = diagnosis.Known(0.5)
			}
			_, sig := Decide(engine, smp, env)
			if i == 129 {
				// Sixty readable ticks filled host-headroom; sixty absent ones
				// emptied it. Selection walks to usage-fraction, which stayed
				// ready on our own usage the whole way.
				_, hhst := engine.Reduction(sigHostCpuFull, instHostHeadroom).Get()
				Expect(hhst).To(Equal(diagnosis.StateAbsent), "the host window must be absent, not untrusted")
				sel, red, _, avail := engine.Select(sat, env)
				Expect(avail).To(Equal(diagnosis.Ready))
				Expect(sel.Name).To(Equal(instUsageFraction), "the fallback instrument is selected by name, not merely the verdict")
				v, st := red.Get()
				Expect(st).To(Equal(diagnosis.StateValue))
				Expect(v).To(BeNumerically("~", 0.1, 1e-9))
				// The metric is usage-fraction's OWN reduction — not the
				// usage-cores measurement divided by anything.
				Expect(sig.AvgUsageFraction).To(BeNumerically("~", 0.1, 1e-9))
			}
		}

		// The fallback JUDGES, not merely selects: on a no-limit box where the
		// usage fraction is 0.75 (above the 0.70 fire mark), the host-cpu-full
		// latch fires on the fallback and the verdict is degraded with one
		// host-cpu-full cause carrying the fraction.
		engine2, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env2 := diagnosis.NewEnvironment(HasLimitedVisibility)
		base2 := time.Now()
		for i := 0; i < 130; i++ {
			hbKnown := i < 60
			smp := Sample{
				Timestamp:   base2.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				HostBusy:    diagnosis.Unknown(),
				UsageCores:  diagnosis.Known(3.0),
				Pressure:    diagnosis.Known(0),
				Steal:       diagnosis.Known(0),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}
			if hbKnown {
				smp.HostBusy = diagnosis.Known(0.5)
			}
			verdict, _ := Decide(engine2, smp, env2)
			if i == 129 {
				Expect(verdict.State).To(Equal(StateDegraded), "the fallback must judge, not merely answer")
				Expect(verdict.Causes).To(HaveLen(1))
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindHostCpuFull))
				Expect(verdict.Causes[0].Value).To(BeNumerically("~", 0.75, 1e-9))
				Expect(verdict.Causes[0].Unit).To(Equal(Unit("fraction")))
			}
		}
	})

	It("should annotate limited visibility rather than treating it as a state", func() {
		// Limited visibility: no quota AND no PSI. Verdict.State has two values
		// and neither is it, so the annotation lives on Details and the verdict
		// is healthy. Nothing here fires.
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment()
		base := time.Now()

		for i := 0; i < 5; i++ {
			smp := Sample{
				Timestamp:    base.Add(time.Duration(i) * time.Second),
				CpuScope:     ScopeHost,
				Quota:        diagnosis.Unknown(),
				PsiAvailable: false,
				Pressure:     diagnosis.Known(0),
				Steal:        diagnosis.Known(0),
				NrPeriods:    diagnosis.Known(0),
				NrThrottled:  diagnosis.Known(0),
				UsageCores:   diagnosis.Known(0.2),
				HostBusy:     diagnosis.Known(0.5),
			}
			verdict, sig := Decide(engine, smp, env)
			Expect(verdict.State).To(Equal(StateHealthy))
			if i == 4 {
				Expect(sig.LimitedVisibility).To(BeTrue(), "no quota and no PSI is limited visibility")
			}
		}

		// A positive quota is NOT limited visibility even without PSI.
		engine2, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env2 := diagnosis.NewEnvironment(HasLimit)
		for i := 0; i < 5; i++ {
			smp := Sample{
				Timestamp:    base.Add(time.Duration(i) * time.Second),
				CpuScope:     ScopeHost,
				Quota:        diagnosis.Known(2.0),
				PsiAvailable: false,
				Pressure:     diagnosis.Known(0),
				Steal:        diagnosis.Known(0),
				NrPeriods:    diagnosis.Known(0),
				NrThrottled:  diagnosis.Known(0),
				UsageCores:   diagnosis.Known(0.2),
				HostBusy:     diagnosis.Known(0.5),
			}
			_, sig := Decide(engine2, smp, env2)
			if i == 4 {
				Expect(sig.LimitedVisibility).To(BeFalse(), "a positive quota lifts limited visibility even without PSI")
			}
		}
	})
})

// The gate on the fallback arm. usage-fraction reserves 30% of the CPUs we may
// run on, where host-headroom reserves one core of the machine, so the two arms
// disagree about what a full machine is and the gap widens with the core count.
// The fallback therefore answers only where nothing better exists at all, and
// these three specs drive one fixture across that boundary: the same 4-core box
// with /proc/stat unreadable and 3.0 cores of our own usage, so usage-fraction
// reduces to 0.75 — past its 0.70 fire mark — in every one of them. The only
// thing that varies is what else the box can see.
var _ = Describe("the gate on the usage estimate", func() {
	// gatedSample is that one fixture. HostBusy is absent, so host-headroom
	// cannot answer and the estimate is the only arm left. Pressure follows
	// Sample's sticky contract: a known reading needs PsiAvailable.
	gatedSample := func(at time.Time, quota diagnosis.Reading, psi bool) Sample {
		pressure := diagnosis.Unknown()
		if psi {
			pressure = diagnosis.Known(0)
		}

		return Sample{
			Timestamp:    at,
			CpuScope:     ScopeHost,
			Quota:        quota,
			PsiAvailable: psi,
			Pressure:     pressure,
			Steal:        diagnosis.Known(0),
			NrPeriods:    diagnosis.Known(0),
			NrThrottled:  diagnosis.Known(0),
			HostBusy:     diagnosis.Unknown(),
			UsageCores:   diagnosis.Known(3.0),
		}
	}

	// driveGate runs six one-second ticks of that fixture and returns the
	// environment the box derived, plus the last tick's verdict and Details.
	// The environment comes from DeriveEnvironment rather than a literal, so
	// these specs run the chain that decides the gate rather than restating its
	// answer.
	driveGate := func(engine *diagnosis.Engine[Sample], quota diagnosis.Reading, psi bool) (diagnosis.Environment, Verdict, Details) {
		var verdict Verdict
		var sig Details
		base := time.Now()
		env := DeriveEnvironment(gatedSample(base, quota, psi))

		for i := 0; i <= 5; i++ {
			verdict, sig = Decide(engine, gatedSample(base.Add(time.Duration(i)*time.Second), quota, psi), env)
		}

		return env, verdict, sig
	}

	// estimateWithheld asserts the shape both no-verdict specs share: the gate
	// leaves host-headroom as the signal's only capable arm, that arm has
	// nothing to read, nothing fired, and the estimate itself still reduced to
	// a value past its fire mark — so the quiet tick is the gate withholding a
	// number, not a box that measured fine.
	estimateWithheld := func(engine *diagnosis.Engine[Sample], sat diagnosis.Signal[Sample], env diagnosis.Environment, verdict Verdict, sig Details) {
		Expect(instrumentNames(sat.Capable(env))).To(Equal([]string{instHostHeadroom}),
			"the gate takes the estimate out of the capable set and leaves the /proc/stat arm alone")
		_, _, _, avail := engine.Select(sat, env)
		Expect(avail).To(Equal(diagnosis.AllAbsent),
			"the one capable arm read nothing, which the engine reports as an empty window rather than as a measurement inside its marks")
		Expect(kindsOf(verdict.Causes)).NotTo(ContainElement(CauseKindHostCpuFull))
		Expect(verdict.State).To(Equal(StateHealthy))

		fraction, state := engine.Reduction(sigHostCpuFull, instUsageFraction).Get()
		Expect(state).To(Equal(diagnosis.StateValue))
		Expect(fraction).To(BeNumerically("~", 0.75, 1e-9),
			"0.75 is past the 0.70 fire mark, so the gate is what kept the signal quiet")
		Expect(sig.AvgUsageFraction).To(BeNumerically("~", 0.75, 1e-9),
			"the number is still published, so a reader sees a busy box rather than a measured-and-fine one")
	}

	It("should leave the machine full question unanswered where the kernel reports pressure statistics", func() {
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env, verdict, sig := driveGate(engine, diagnosis.Unknown(), true)
		Expect(env.Has(HasLimitedVisibility)).To(BeFalse(), "PSI is better evidence than the estimate")

		estimateWithheld(engine, signalNamed(cpuTable(4, 0), sigHostCpuFull), env, verdict, sig)
		Expect(sig.PressureApplies).To(BeTrue(), "the pressure signal is what covers this box instead")
	})

	It("should leave the machine full question unanswered where a CPU limit is set", func() {
		// Quota 8.0 against 3.0 cores used leaves 8.0 - 3.0 - 0.8 = 4.2 cores of
		// the container's own budget, so container-limit-full stays quiet too
		// and the tick is healthy for want of any capacity reading at all.
		engine, err := NewEngine(4, 8.0)
		Expect(err).NotTo(HaveOccurred())
		env, verdict, sig := driveGate(engine, diagnosis.Known(8.0), false)
		Expect(env.Has(HasLimitedVisibility)).To(BeFalse(), "a CPU limit is better evidence than the estimate")

		estimateWithheld(engine, signalNamed(cpuTable(4, 8.0), sigHostCpuFull), env, verdict, sig)
		Expect(sig.LimitApplies).To(BeTrue(), "throttling and container-limit-full are what cover this box instead")
	})

	It("should still answer from our own usage where there is no CPU limit and no pressure statistic", func() {
		// The control for the two above: the same fixture, the same absent
		// /proc/stat, the same 0.75, and only the gate varies.
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env, verdict, sig := driveGate(engine, diagnosis.Unknown(), false)
		Expect(env.Has(HasLimitedVisibility)).To(BeTrue(), "no limit and no PSI is where nothing better exists")

		sat := signalNamed(cpuTable(4, 0), sigHostCpuFull)
		Expect(instrumentNames(sat.Capable(env))).To(Equal([]string{instHostHeadroom, instUsageFraction}),
			"both arms are capable here, and only the second has anything to read")
		_, _, _, avail := engine.Select(sat, env)
		Expect(avail).To(Equal(diagnosis.Ready))
		Expect(verdict.State).To(Equal(StateDegraded))
		Expect(verdict.Causes).To(HaveLen(1))
		Expect(verdict.Causes[0].Kind).To(Equal(CauseKindHostCpuFull))
		Expect(verdict.Causes[0].Instrument).To(Equal(instUsageFraction))
		Expect(verdict.Causes[0].Value).To(BeNumerically("~", 0.75, 1e-9))
		Expect(sig.LimitedVisibility).To(BeTrue(), "the same condition the gate reads, reported on Details")
	})
})

// instrumentNames names the instruments in declared order, so a spec can pin
// which arms an environment leaves capable rather than only what the signal
// concluded from them.
func instrumentNames(insts []diagnosis.Instrument[Sample]) []string {
	out := make([]string, 0, len(insts))
	for _, inst := range insts {
		out = append(out, inst.Name)
	}

	return out
}
