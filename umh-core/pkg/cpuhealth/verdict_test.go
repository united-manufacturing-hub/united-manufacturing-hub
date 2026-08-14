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

// Verdict assembly. Decide derives Verdict.Attribution from the
// dominant cause, orders Verdict.Causes through diagnosis.Rank (no local sort),
// returns healthy with no causes when nothing is fired, and fills the
// observable metrics, the two track floors and each signal's readiness from the
// same pass even when no latch has fired.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("verdict assembly", func() {
	It("should derive Verdict.Attribution from the dominant cause", func() {
		// Steal is external, so when steal is the dominant cause the attribution
		// is host. Pressure and steal are the same tier; drive both and let
		// steal's higher severity make it dominant.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit, HasPressureStats)
		base := time.Now()

		for i := 0; i < 5; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				Pressure:    diagnosis.Known(0.40),
				Steal:       diagnosis.Known(0.9),
				HostBusy:    diagnosis.Known(0.5),
				UsageCores:  diagnosis.Known(0.2),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}
			verdict, _ := Decide(engine, smp, env)
			if i == 4 {
				Expect(verdict.Causes).To(HaveLen(2))
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindSteal), "steal's higher severity must make it dominant")
				Expect(verdict.Attribution).To(Equal(AttributionHost), "a steal-dominant verdict attributes host")
			}
		}
	})

	It("should order Verdict.Causes through diagnosis.Rank and not through a local sort", func() {
		// Same tier (pressure and steal are both starvation), so severity breaks
		// the tie: steal (0.889) outranks pressure (0.25), where the table's
		// declaration order would have given pressure first. A local sort that
		// reproduced today's five signals could diverge on the sixth.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit, HasPressureStats)
		base := time.Now()

		for i := 0; i < 5; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				Pressure:    diagnosis.Known(0.40),
				Steal:       diagnosis.Known(0.9),
				HostBusy:    diagnosis.Known(0.5),
				UsageCores:  diagnosis.Known(0.2),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}
			verdict, _ := Decide(engine, smp, env)
			if i == 4 {
				Expect(verdict.Causes[0].Kind).To(Equal(CauseKindSteal), "Rank must put the higher-severity steal before pressure")
				Expect(verdict.Causes[1].Kind).To(Equal(CauseKindPressure))
			}
		}
	})

	It("should return healthy with no causes when nothing is fired, rather than degraded with an empty list", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit, HasPressureStats)
		base := time.Now()

		for i := 0; i < 5; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				Pressure:    diagnosis.Known(0.1),
				Steal:       diagnosis.Known(0),
				HostBusy:    diagnosis.Known(0.5),
				UsageCores:  diagnosis.Known(0.2),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}
			verdict, _ := Decide(engine, smp, env)
			Expect(verdict.State).To(Equal(StateHealthy))
			Expect(verdict.Causes).To(BeEmpty())
			Expect(verdict.Attribution).To(Equal(Attribution("")), "a healthy verdict carries no attribution")
		}
	})

	It("should fill the observable metrics, the two track floors and each signal's readiness from the same pass, even when no latch has fired", func() {
		// Drive throttle-ratio to a steady 0.02, below its 0.05 fire mark, for a
		// full window: nothing fires and the verdict is healthy, yet
		// Signals.ThrottleRatio reaches Signals as 0.02 — not a confident 0
		// published because the latch is quiet. The box is bare metal (no
		// virtualisation), so steal's window fills with legitimate zeros and
		// reduces to StateValue while Select still reports NoInstrument — and
		// StealSignalReady must be false.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit, HasPressureStats) // bare-metal: has PSI, no HasVirtualization
		base := time.Now()

		for i := 0; i <= 65; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: false,
				Pressure:    diagnosis.Known(0.1),
				Steal:       diagnosis.Known(0),
				HostBusy:    diagnosis.Known(0.5),
				UsageCores:  diagnosis.Known(0.2),
				NrPeriods:   diagnosis.Known(100 * float64(i)),
				NrThrottled: diagnosis.Known(2 * float64(i)),
			}
			verdict, sig := Decide(engine, smp, env)
			if i == 65 {
				Expect(verdict.State).To(Equal(StateHealthy))
				Expect(verdict.Causes).To(BeEmpty())

				// Metrics, from the same pass, below their marks.
				Expect(sig.ThrottleRatio).To(BeNumerically("~", 0.02, 1e-9), "a quiet throttle latch must not publish 0")
				Expect(sig.PressureAvg60Out).To(BeNumerically("~", 0.1, 1e-9))
				Expect(sig.AvgUsageCores).To(BeNumerically("~", 0.2, 1e-9))
				Expect(sig.HostBusyCores60sMean).To(BeNumerically("~", 0.5, 1e-9))

				// The two track floors.
				Expect(sig.UsageRingActive).To(BeTrue())
				Expect(sig.HostBusyRingActive).To(BeTrue())

				// Readiness trio: ready where a capable window has a value, false
				// on the bare-metal steal signal that no instrument can answer.
				Expect(sig.ThrottleSignalReady).To(BeTrue())
				Expect(sig.PressureSignalReady).To(BeTrue())
				Expect(sig.StealSignalReady).To(BeFalse(), "a bare-metal box has no steal answer, whatever its window holds")
			}
		}
	})
})
