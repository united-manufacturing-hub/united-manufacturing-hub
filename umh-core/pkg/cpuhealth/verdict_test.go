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
// observable metrics, both measurement floors and each signal's readiness from
// the same pass even when no latch has fired — and leaves the fields it does
// not fill reporting absent rather than a measured zero.
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

	It("should put steal first when a same-tier peer ties it on severity", func() {
		// Severity is the last key Rank can decide on before the declaration
		// index, and clamp01 makes it a tie whenever two signals both sit at or
		// past their worst value — the common case under real load. Pressure is
		// the peer here because its reduction is Last, so one tick at 1.0 puts
		// it exactly at worst with no counter arithmetic in the way.
		//
		// With both severities pinned at 1.0, the only thing left that can put
		// steal ahead of pressure is steal's position in cpuTable's signal
		// list. Move stealSignal() back below pressureSignal() and this spec
		// reports pressure as the headline cause.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit, HasPressureStats)
		base := time.Now()

		// Five ticks, which clears steal's mean arm's two-sample minimum. Its
		// p95 arm needs twenty and stays absent, so the mean is what steal is
		// judged on.
		var verdict Verdict
		for i := 0; i < 5; i++ {
			verdict, _ = Decide(engine, Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				Pressure:    diagnosis.Known(1.0),
				Steal:       diagnosis.Known(1.0),
				HostBusy:    diagnosis.Known(0.5),
				UsageCores:  diagnosis.Known(0.2),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}, env)
		}

		Expect(verdict.Causes).To(HaveLen(2))
		// Both causes sit at their shared worst value of 1.0. Without this the
		// spec would still pass on a fixture that had drifted into a severity
		// difference, and would then be asserting the severity key again rather
		// than the index key.
		Expect(verdict.Causes[0].Value).To(BeNumerically("~", 1.0, 1e-9))
		Expect(verdict.Causes[1].Value).To(BeNumerically("~", 1.0, 1e-9))
		Expect(verdict.Causes[0].Kind).To(Equal(CauseKindSteal), "a severity tie must fall to the declaration index, where steal leads")
		Expect(verdict.Causes[1].Kind).To(Equal(CauseKindPressure))
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

	It("should fill the observable metrics, both measurement floors and each signal's readiness from the same pass, even when no latch has fired", func() {
		// Drive throttle-ratio to a steady 0.02, below its 0.05 fire mark, for a
		// full window: nothing fires and the verdict is healthy, yet
		// Details.ThrottleRatio reaches Details as 0.02 — not a confident 0
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
			verdict, details := Decide(engine, smp, env)
			if i == 65 {
				Expect(verdict.State).To(Equal(StateHealthy))
				Expect(verdict.Causes).To(BeEmpty())

				// Metrics, from the same pass, below their marks.
				Expect(details.ThrottleRatio).To(BeNumerically("~", 0.02, 1e-9), "a quiet throttle latch must not publish 0")
				Expect(details.PressureAvg60).To(BeNumerically("~", 0.1, 1e-9))
				Expect(details.AvgUsageCores).To(BeNumerically("~", 0.2, 1e-9))
				Expect(details.AvgHostBusyCores).To(BeNumerically("~", 0.5, 1e-9))

				// The two measurement floors.
				Expect(details.UsageRingActive).To(BeTrue())
				Expect(details.HostBusyRingActive).To(BeTrue())

				// Readiness trio: ready where a capable window has a value, false
				// on the bare-metal steal signal that no instrument can answer.
				Expect(details.ThrottleSignalReady).To(BeTrue())
				Expect(details.PressureSignalReady).To(BeTrue())
				Expect(details.StealSignalReady).To(BeFalse(), "a bare-metal box has no steal answer, whatever its window holds")
			}
		}
	})

	It("should report a Reading nothing fills as absent rather than as a measured zero", func() {
		// A Reading is declared for a future frontend projection and Decide has
		// no assignment for it yet. A float64 could not say so: 0 is a
		// legitimate usage figure, so an unfilled field read as a measurement.
		// It must answer through Reading's second return, which a Known(0)
		// would fail — the distinction is the whole point of the type.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit, HasPressureStats)
		base := time.Now()

		var details Details
		for i := 0; i <= 65; i++ {
			_, details = Decide(engine, Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Pressure:    diagnosis.Known(0.1),
				Steal:       diagnosis.Known(0),
				HostBusy:    diagnosis.Known(0.5),
				UsageCores:  diagnosis.Known(0.2),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}, env)
		}

		// A sibling the same pass DOES fill. Without it this spec would also pass
		// on a Decide that filled nothing at all.
		Expect(details.AvgUsageCores).To(BeNumerically("~", 0.2, 1e-9), "the pass must have run for the absences below to mean anything")

		absent := func(r diagnosis.Reading) bool { _, ok := r.Get(); return !ok }
		Expect(absent(details.P95UsageCores)).To(BeTrue(), "P95UsageCores")
	})
})
