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

// The fallback metric set. When host stats are absent, the saturation
// signal falls back to usage-fraction: host-headroom's window empties past the
// demote span, selection walks to usage-fraction and JUDGES on it, and the
// number it judged on reaches Signals as AvgUsageFraction. The
// dead zone — quota nil or non-positive AND PSI absent — is an annotation on a
// healthy verdict, carried by Signals.LimitedVisibility, never a state.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("the fallback metric set", func() {
	It("should fall back to usage against logical CPUs when host stats are absent", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()
		sat := signalNamed(cpuTable(4, 2.0), "saturation")

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
				_, hhst := engine.Reduction(sigSaturation, instHostHeadroom).Get()
				Expect(hhst).To(Equal(diagnosis.StateAbsent), "the host window must be absent, not untrusted")
				sel, red, _, avail := engine.Select(sat, env)
				Expect(avail).To(Equal(diagnosis.Ready))
				Expect(sel.Name).To(Equal(instUsageFraction), "the fallback instrument is selected by name, not merely the verdict")
				v, st := red.Get()
				Expect(st).To(Equal(diagnosis.StateValue))
				Expect(v).To(BeNumerically("~", 0.1, 1e-9))
				// The metric is usage-fraction's OWN reduction — not the usage
				// track divided by anything.
				Expect(sig.AvgUsageFraction).To(BeNumerically("~", 0.1, 1e-9))
			}
		}

		// The fallback JUDGES, not merely selects: on a no-limit box where the
		// usage fraction is 0.75 (above the 0.70 fire mark), the saturation
		// latch fires on the fallback and the verdict is degraded with one
		// saturation cause carrying the fraction.
		engine2, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env2 := diagnosis.NewEnvironment()
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
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindSaturation))
				Expect(verdict.Causes[0].Value).To(BeNumerically("~", 0.75, 1e-9))
				Expect(verdict.Causes[0].Unit).To(Equal(Unit("fraction")))
			}
		}
	})

	It("should annotate the dead zone rather than treating it as a state", func() {
		// The dead zone: no quota AND no PSI. Verdict.State has two values and
		// neither is it, so the annotation lives on Signals and the verdict is
		// healthy. Nothing here fires.
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
				Expect(sig.LimitedVisibility).To(BeTrue(), "no quota and no PSI is the dead zone")
			}
		}

		// A positive quota is NOT the dead zone even without PSI.
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
				Expect(sig.LimitedVisibility).To(BeFalse(), "a positive quota lifts the dead zone even without PSI")
			}
		}
	})
})
