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

package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("the usage-cores p95 floor", func() {
	It("should leave P95UsageCores absent through nineteen readings and fill it on the twentieth", func() {
		// The p95 reduction over usage-cores declares Min 20, so the twentieth
		// stored reading is the first the window can reduce to a value. Absence
		// is asserted through Reading's second return, never against 0: a
		// filled field and an unfilled one must not read alike.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit, HasPressureStats)
		base := time.Now()

		var details Details
		for i := 0; i < 20; i++ {
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

			if i == 18 {
				// Nineteen readings sit in the window, one short of the floor.
				_, ok := details.P95UsageCores.Get()
				Expect(ok).To(BeFalse(), "nineteen readings are below the p95 floor, so P95UsageCores must answer absent")

				// A sibling the same tick fills. Without it this spec would also
				// pass on a Decide that filled nothing at all, and the absence
				// above would carry no evidence the ticks ran.
				Expect(details.AvgUsageCores).To(BeNumerically("~", 0.2, 1e-9), "the tick must have run for the absence above to mean anything")
			}
		}

		// The twentieth reading reaches the floor, so the window reduces to a
		// value and the Reading becomes present.
		p95, ok := details.P95UsageCores.Get()
		Expect(ok).To(BeTrue(), "the twentieth reading clears the p95 floor, so P95UsageCores must answer present")
		Expect(p95).To(Equal(0.2), "the p95 of twenty 0.2 readings is exactly 0.2")
	})

	It("should fill P95UsageCores with a measured zero when every reading is zero", func() {
		// An idle box reads 0 cores on every tick, so its p95 is a measured
		// zero, not an absence: the field must answer present with 0. The wire
		// spec in fsmv2/cpu builds this status by hand; this one drives the
		// producer.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit, HasPressureStats)
		base := time.Now()

		var details Details
		for i := 0; i < 20; i++ {
			_, details = Decide(engine, Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Pressure:    diagnosis.Known(0.1),
				Steal:       diagnosis.Known(0),
				HostBusy:    diagnosis.Known(0.5),
				UsageCores:  diagnosis.Known(0),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}, env)
		}

		v, ok := details.P95UsageCores.Get()
		Expect(ok).To(BeTrue(), "an idle box's p95 is a measured zero, so P95UsageCores must answer present")
		Expect(v).To(Equal(0.0))
	})
})
