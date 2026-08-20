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

// The host-cpu-full signal answers one question through two instruments, its
// two arms, and a verdict has to say which arm answered. diagnosis.Identity
// names only the signal, so causeOf recovers the arm from Fired.Instrument and
// reads the cause's value back from that arm's own reduction. The two specs
// below drive each arm through Decide and check what the verdict then says.

package cpuhealth

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// declaresArm reports whether the table production builds its engine from hangs
// one instrument under one signal. Nothing here writes the table down; the
// specs read it back so neither can pass against an arm the table dropped.
func declaresArm(signal, instrument string) bool {
	for _, s := range Table(4, 2.0).Signals {
		if s.Name != signal {
			continue
		}
		for _, in := range s.Instruments {
			if in.Name == instrument {
				return true
			}
		}
	}
	return false
}

// coupling renders the failure message the two specs share. It names the
// mechanism the outcome hangs on, because a reader who sees only the mismatch
// cannot tell which of the two sides moved.
func coupling(instrument, outcome string) string {
	return fmt.Sprintf(
		"%s\n\n"+
			"This spec drove the %s arm of host-cpu-full. diagnosis.Identity names only the\n"+
			"signal, so causeOf reads the arm that fired out of Fired.Instrument and takes the\n"+
			"cause's value from that arm's reduction. The blame comes from whichever share\n"+
			"refinement fired, never from the arm. Read causeOf in attribute.go and\n"+
			"shareRefinements in signal_saturation.go before changing what this expects.",
		outcome, instrument)
}

var _ = Describe("the host-cpu-full arms are told apart in the verdict", func() {
	It("should blame the host for a full machine the host-headroom arm found", func() {
		Expect(declaresArm(sigHostCpuFull, instHostHeadroom)).To(BeTrue(),
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

	It("should name the fallback arm in the cause when a machine has no host stats", func() {
		Expect(declaresArm(sigHostCpuFull, instUsageFraction)).To(BeTrue(),
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
