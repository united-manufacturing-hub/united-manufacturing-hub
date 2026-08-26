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

package fsmv2cpu

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// countsAfterPoll drives one Poll on a sampler that always hands back sample,
// then reads that tick's evidence counts through countsFor.
func countsAfterPoll(d *CPUDeps, sample cpuhealth.Sample) (capable, measured int) {
	_, err := Poll(context.Background(), d, CPUConfig{})
	Expect(err).NotTo(HaveOccurred())

	return countsFor(d, sample)
}

var _ = Describe("absence of evidence is not health", func() {
	Describe("the capable/measured evidence counts", func() {
		It("counts how many signals are capable and how many have first-measured", func() {
			// Bare metal, no quota. Measured by the engine: throttling and steal
			// are NoInstrument (not capable); pressure (Ready this tick) and
			// host-cpu-full (AllAbsent) are capable.
			sample := cpuhealth.Sample{
				Timestamp: time.Now(),
				Quota:     diagnosis.Known(0),
				NrPeriods: diagnosis.Known(1),
				Pressure:  diagnosis.Known(0.2),
				// PSI present: the box is pressurable (DeriveEnvironment
				// derives HasPressureStats from the sticky PsiAvailable).
				PsiAvailable: true,
			}
			d := newDeps(fixedSampler(sample), 4, 0)

			// Pressure is Ready (capable and first-measured); host-cpu-full is
			// AllAbsent (capable, not yet measured). So one measured of two
			// capable.
			capable, measured := countsAfterPoll(d, sample)
			Expect(capable).To(Equal(2), "pressure + host-cpu-full are capable on this bare box")
			Expect(measured).To(Equal(1), "pressure judged on its first sample; host-cpu-full has not")
			Expect(measured).To(BeNumerically("<", capable),
				"one signal first-measured and another capable one still has not, so this partial box is short too — "+
					"a regression to `measured == 0` would pass the specs below yet call this box fully measured")
		})
	})

	Describe("a capable signal that has not first-measured", func() {
		It("keeps measured below capable while a capable signal has produced no first measurement", func() {
			// The same bare box, but pressure is ABSENT this tick, so neither
			// capable signal has measured yet: measured < capable, which is the
			// shortfall the deadline warning reports. The verdict itself stays
			// "healthy" — the counts, not the verdict, carry the shortfall.
			sample := cpuhealth.Sample{
				Timestamp: time.Now(),
				Quota:     diagnosis.Known(0),
				NrPeriods: diagnosis.Known(1),
				// Pressure absent (Unknown), not a failed sample. The box
				// still has PSI (sticky PsiAvailable is true) — this tick's
				// read is just not landing.
				Pressure:     diagnosis.Unknown(),
				PsiAvailable: true,
			}
			d := newDeps(fixedSampler(sample), 4, 0)

			capable, measured := countsAfterPoll(d, sample)
			Expect(capable).To(Equal(2), "pressure + host-cpu-full are still capable")
			Expect(measured).To(Equal(0), "no capable signal has measured yet")
			Expect(measured).To(BeNumerically("<", capable),
				"a capable signal that has never measured must keep measured < capable (the shortfall)")
		})
	})

	Describe("a signal no instrument can answer", func() {
		It("does not count a signal with no instrument on this box", func() {
			// Bare metal: steal has no instrument (Virtualized=false), throttling
			// has none (no quota). These NoInstrument signals are NOT capable, so
			// the capable count reflects only what this box can actually answer.
			// If NoInstrument were (wrongly) counted capable, a bare box would
			// look mostly-measured and never be short, or be short forever —
			// either way the count is the thing that must exclude them.
			sample := cpuhealth.Sample{
				Timestamp: time.Now(),
				Quota:     diagnosis.Known(0),
				NrPeriods: diagnosis.Known(1),
				Pressure:  diagnosis.Known(0.2),
				// PSI present: the box is pressurable (DeriveEnvironment
				// derives HasPressureStats from the sticky PsiAvailable).
				PsiAvailable: true,
			}
			d := newDeps(fixedSampler(sample), 4, 0)

			// Table has 4 signals; only 2 are capable (pressure, host-cpu-full).
			// throttling and steal are NoInstrument and must not appear here.
			capable, _ := countsAfterPoll(d, sample)
			Expect(capable).To(Equal(2),
				"NoInstrument signals are excluded from the capable count, so a bare box shows its real answerable set")
		})
	})

	Describe("a read outage on a signal that already measured", func() {
		It("keeps counting a first-measured signal as measured, however long a later outage lasts", func() {
			// Tick 1: pressure first-measures (Ready, judged on the first
			// sample), so its everMeasured bit is set.
			first := cpuhealth.Sample{
				Timestamp: time.Now(),
				Quota:     diagnosis.Known(0),
				NrPeriods: diagnosis.Known(1),
				Pressure:  diagnosis.Known(0.2),
				// PSI present: the box is pressurable (DeriveEnvironment
				// derives HasPressureStats from the sticky PsiAvailable).
				PsiAvailable: true,
			}
			d := newDeps(fixedSampler(first), 4, 0)

			_, measured1 := countsAfterPoll(d, first)
			Expect(measured1).To(Equal(1), "pressure first-measured on tick 1")

			// Tick 2 onward: a read outage makes pressure unreadable. The
			// everMeasured bit must NOT be cleared, so a signal that has measured
			// keeps counting as measured. The worker reports thin evidence, never
			// stale evidence: the frozen arm of NoneReady must not be reported.
			outage := cpuhealth.Sample{
				Timestamp: time.Now(),
				Quota:     diagnosis.Known(0),
				NrPeriods: diagnosis.Known(1),
				// Pressure unreadable during the outage. PsiAvailable stays
				// true (sticky) — an outage removes the reading, not the
				// box's pressurability, so pressure stays capable.
				Pressure:     diagnosis.Unknown(),
				PsiAvailable: true,
			}
			d.sampler = fixedSampler(outage)

			_, measured2 := countsAfterPoll(d, outage)
			Expect(measured2).To(Equal(1),
				"a signal that already measured keeps counting measured through the outage (the everMeasured bit is never cleared)")
		})
	})
})
