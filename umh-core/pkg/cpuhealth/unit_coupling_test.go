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

// Marks.Unit is display text everywhere else — the word a number is printed
// with. Here it is load-bearing: the host-cpu-full signal holds two
// instruments,
// and the only thing that tells them apart downstream is that word.

package cpuhealth

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// declaredUnit reads back the unit one instrument declares in the table that
// production builds its engine from. Nothing here writes a unit down: the value
// is quoted into the failure message so a reader sees which word the two sides
// disagreed about, never compared against one.
func declaredUnit(signal, instrument string) string {
	for _, s := range Table(4, 2.0).Signals {
		if s.Name != signal {
			continue
		}
		for _, in := range s.Instruments {
			if in.Name == instrument {
				return in.Marks.Unit
			}
		}
	}
	return ""
}

// coupling renders the failure message the two specs share. It names the
// mechanism rather than the mismatch, because the string is the mechanism.
func coupling(instrument, outcome string) string {
	return fmt.Sprintf(
		"%s\n\n"+
			"The host-cpu-full signal carries two instruments and diagnosis.Identity has no field\n"+
			"naming which one fired, so this package recovers the arm by matching Marks.Unit —\n"+
			"a display label — against a literal spelled out in table.go and attribute.go.\n"+
			"The table currently declares Unit %q for the %s arm. If that word was just renamed\n"+
			"on one side, rename it on the other: the two sides are one decision, and only one\n"+
			"of them is the word a customer reads.",
		outcome, declaredUnit(sigHostCpuFull, instrument), instrument)
}

var _ = Describe("the host-cpu-full arms are told apart by a display string", func() {
	It("should still blame the host for a full machine after the host-headroom arm's unit is read back from the table", func() {
		Expect(declaredUnit(sigHostCpuFull, instHostHeadroom)).NotTo(BeEmpty(),
			"the host-headroom arm must be in the table for this spec to mean anything")

		// 4 cores, no quota: host-headroom is 4 - 3.5 - 1.0 = -0.5 and fires,
		// usage-fraction sits at 0.5/4 and does not. The split says host —
		// 3.5 > 2 x 0.5 — so the only thing between here and AttributionHost is
		// attribute.go recognising the arm that fired.
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment()
		base := time.Now()

		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:  base.Add(time.Duration(i) * time.Second),
				CpuScope:   ScopeHost,
				HostBusy:   diagnosis.Known(3.5),
				UsageCores: diagnosis.Known(0.5),
			}
			verdict, _ := Decide(engine, smp, env)
			if i < 5 {
				continue
			}
			Expect(verdict.Causes).To(HaveLen(1))
			Expect(verdict.Causes[0].Kind).To(Equal(CauseKindHostCpuFull))
			Expect(verdict.Causes[0].Instrument).To(Equal(instHostHeadroom), "host-headroom 4 - 3.5 - 1.0 = -0.5 fires")
			Expect(verdict.Attribution).To(Equal(AttributionHost),
				coupling(instHostHeadroom, "Attribution changed: a full machine the split blames on the host now reports an unknown cause."))
		}
	})

	It("should still route a machine with no host stats to the fallback arm after that arm's unit is read back from the table", func() {
		Expect(declaredUnit(sigHostCpuFull, instUsageFraction)).NotTo(BeEmpty(),
			"the usage-fraction arm must be in the table for this spec to mean anything")

		// Host stats unreadable on a box with no quota and no PSI, which is the
		// one place usage-fraction may answer: 3.0/4 = 0.75 fires. No other
		// signal can fire there, so host-cpu-full is the only cause, and the
		// instrument it names picks the reduction the cause value is read from.
		engine, err := NewEngine(4, 0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimitedVisibility)
		base := time.Now()

		for i := 0; i <= 5; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				HostBusy:    diagnosis.Unknown(),
				UsageCores:  diagnosis.Known(3.0),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}
			verdict, _ := Decide(engine, smp, env)
			if i < 5 {
				continue
			}
			Expect(verdict.Causes).To(HaveLen(1))
			Expect(verdict.Causes[0].Kind).To(Equal(CauseKindHostCpuFull))
			Expect(verdict.Causes[0].Instrument).To(Equal(instUsageFraction),
				coupling(instUsageFraction, "The cause misnames its instrument: host-cpu-full answered through the fallback arm, but the verdict was built as if the host-headroom arm had fired."))

			fraction, _ := engine.Reduction(sigHostCpuFull, instUsageFraction).Get()
			Expect(verdict.Causes[0].Value).To(BeNumerically("~", fraction, 1e-9),
				coupling(instUsageFraction, "The customer is shown the wrong number: the cause value came from an instrument other than the one that fired."))
		}
	})
})
