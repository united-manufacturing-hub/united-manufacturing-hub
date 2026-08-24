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

// The guard. A healthy no-PSI box has limited visibility (no positive
// quota AND no PSI), so its customer-visible output must be byte-identical
// however the answerability mechanisms (LimitedVisibility, PressureApplies) evolve:
// verdict healthy, no causes, PressureApplies false, LimitedVisibility true, and the
// limited-visibility advisory present in the message. The whole point is the
// REAL chain — DeriveEnvironment, Decide and ComposeMessage run back to back on
// one sample stream, not a hand-assembled Details bag — so the spec breaks the
// moment any load-bearing derivation is changed. This is a GUARD: it passes on
// today's already-fixed code and exists to fail if the invariant is regressed.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("byte-identical output on a healthy no-PSI box", func() {
	It("should leave verdict healthy, no causes, PressureApplies false, LimitedVisibility true, and the advisory present", func() {
		// A limited-visibility box: cores=4 so the host-cpu-full instrument IS
		// present, but Quota present 0 (no positive quota -> no HasLimit) and PsiAvailable
		// false (no HasPressureStats). Virtualized false keeps it bare metal so
		// no steal instrument is offered. Every reading is benign and
		// host-scoped, so nothing fires and the box stays healthy.
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())

		base := time.Now()
		var verdict Verdict
		var sig Details
		for i := 0; i < 3; i++ {
			s := Sample{
				Timestamp:    base.Add(time.Duration(i) * time.Second),
				CpuScope:     ScopeHost,
				Quota:        diagnosis.Known(0),
				UsageCores:   diagnosis.Known(0.3),
				HostBusy:     diagnosis.Known(1.0),
				Pressure:     diagnosis.Unknown(),
				Steal:        diagnosis.Known(0.0),
				NrThrottled:  diagnosis.Known(0),
				NrPeriods:    diagnosis.Known(100),
				LogicalCpus:  diagnosis.Known(4),
				HostCpus:     diagnosis.Known(4),
				PsiAvailable: false,
				Virtualized:  false,
			}
			env := DeriveEnvironment(s)
			verdict, sig = Decide(engine, s, env)
		}

		msg := ComposeMessage(verdict, sig)

		// The full invariant, asserted together on the genuinely no-PSI chain.
		Expect(verdict.State).To(Equal(StateHealthy), "a benign limited-visibility box stays healthy")
		Expect(verdict.Causes).To(BeEmpty(), "no cause may fire on a healthy box")
		Expect(sig.PressureApplies).To(BeFalse(), "a no-PSI host must not claim PSI applies")
		Expect(sig.LimitedVisibility).To(BeTrue(), "no limit AND no PSI must set LimitedVisibility")
		// The healthy dashboard rendered, not the below-floor "CPU: starting up."
		// single-liner — proving the real chain produced the full message.
		Expect(msg).To(ContainSubstring("CPU healthy."))
		Expect(msg).To(ContainSubstring(limitedVisibilityNote),
			"the limited-visibility advisory must reach the customer on a limited-visibility box")
	})
})

// The capability gate withholds pressure even when a reading is present. This
// spec deliberately builds a Sample no real box can produce — PsiAvailable
// false with a Pressure reading past the fire mark — because that is exactly
// the point: it isolates the HasPressureStats Requires gate as the thing
// holding the pressure signal back, on a reading that would otherwise fire.
// PressureApplies and LimitedVisibility cannot stand in for this: both are set
// straight from PsiAvailable in buildDetails, never through the engine's
// capability selection, so they pass unchanged whether the gate is present or
// not. Only a produced cause routes through the gate. Without this spec, a
// future PsiAvailable regression that keeps handing out readings — for
// example the sticky flag getting reset while cpu.pressure keeps succeeding —
// has nothing checking that the gate still withholds the signal.
var _ = Describe("the capability gate withholds pressure even when a reading is present", func() {
	It("should produce no pressure cause though the reading sits above the fire mark", func() {
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())

		base := time.Now()
		var verdict Verdict
		for i := 0; i < 3; i++ {
			s := Sample{
				Timestamp:  base.Add(time.Duration(i) * time.Second),
				CpuScope:   ScopeHost,
				Quota:      diagnosis.Known(0),
				UsageCores: diagnosis.Known(0.3),
				HostBusy:   diagnosis.Known(1.0),
				// 0.5, well past the pressure signal's Fire{At: 0.20} — a value
				// that would fire the instant the gate let it through.
				Pressure:     diagnosis.Known(0.5),
				Steal:        diagnosis.Known(0.0),
				NrThrottled:  diagnosis.Known(0),
				NrPeriods:    diagnosis.Known(100),
				LogicalCpus:  diagnosis.Known(4),
				HostCpus:     diagnosis.Known(4),
				PsiAvailable: false,
				Virtualized:  false,
			}
			env := DeriveEnvironment(s)
			verdict, _ = Decide(engine, s, env)
		}

		for _, c := range verdict.Causes {
			Expect(c.Kind).NotTo(Equal(CauseKindPressure),
				"HasPressureStats is absent, so the gate must withhold the pressure signal regardless of the reading")
		}
	})
})
