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

var _ = Describe("R4 — absence of evidence is not health", func() {
	Describe("spec 1 — the capable/measured evidence counts", func() {
		It("reports, alongside the verdict, how many signals are capable and how many have first-measured", func() {
			// Bare metal, no quota. Measured by the engine: throttling and steal
			// are NoInstrument (not capable); pressure (Ready this tick) and
			// saturation (AllAbsent) are capable.
			d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				return cpuhealth.Sample{
					Timestamp: time.Now(),
					Quota:     diagnosis.Known(0),
					NrPeriods: diagnosis.Known(1),
					Pressure:  diagnosis.Known(0.2),
					// PSI present: the box is pressurable (F17 rung 1 derives
					// HasPressureStats from the sticky PsiAvailable).
					PsiAvailable: true,
				}, nil
			}}, 4, 0)

			status, err := Poll(context.Background(), d, CPUConfig{})
			Expect(err).NotTo(HaveOccurred())
			// Pressure is Ready (capable and first-measured); saturation is
			// AllAbsent (capable, not yet measured). So one measured of two
			// capable.
			Expect(status.SignalsCapable).To(Equal(2), "pressure + saturation are capable on this bare box")
			Expect(status.SignalsMeasured).To(Equal(1), "pressure judged on its first sample; saturation has not")
			Expect(status.SignalsMeasured).To(BeNumerically("<=", status.SignalsCapable),
				"measured is never greater than capable")
		})
	})

	Describe("spec 2 — refuse while a capable signal has not first-measured", func() {
		It("keeps measured below capable while a capable signal has produced no first measurement", func() {
			// The same bare box, but pressure is ABSENT this tick, so neither
			// capable signal has measured yet: measured < capable, which is the
			// refusal the seam turns into a refused bridge. The verdict itself
			// stays "healthy" — the counts, not the verdict, carry the refusal.
			d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				return cpuhealth.Sample{
					Timestamp: time.Now(),
					Quota:     diagnosis.Known(0),
					NrPeriods: diagnosis.Known(1),
					// Pressure absent (Unknown), not a failed sample. The box
					// still has PSI (sticky PsiAvailable is true) — this tick's
					// read is just not landing.
					Pressure:    diagnosis.Unknown(),
					PsiAvailable: true,
				}, nil
			}}, 4, 0)

			status, err := Poll(context.Background(), d, CPUConfig{})
			Expect(err).NotTo(HaveOccurred())
			Expect(status.SignalsCapable).To(Equal(2), "pressure + saturation are still capable")
			Expect(status.SignalsMeasured).To(Equal(0), "no capable signal has measured yet")
			Expect(status.SignalsMeasured).To(BeNumerically("<", status.SignalsCapable),
				"a capable signal that has never measured must keep measured < capable (the refusal)")
		})
	})

	Describe("spec 3 — do NOT refuse for a signal no instrument can answer", func() {
		It("does not count (and so cannot refuse on) a signal with no instrument on this box", func() {
			// Bare metal: steal has no instrument (Virtualized=false), throttling
			// has none (no quota). These NoInstrument signals are NOT capable, so
			// the capable count reflects only what this box can actually answer.
			// If NoInstrument were (wrongly) counted capable, a bare box would
			// look mostly-measured and never refuse, or refuse forever — either
			// way the count is the thing that must exclude them.
			d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				return cpuhealth.Sample{
					Timestamp: time.Now(),
					Quota:     diagnosis.Known(0),
					NrPeriods: diagnosis.Known(1),
					Pressure:  diagnosis.Known(0.2),
					// PSI present: the box is pressurable (F17 rung 1 derives
					// HasPressureStats from the sticky PsiAvailable).
					PsiAvailable: true,
				}, nil
			}}, 4, 0)

			status, err := Poll(context.Background(), d, CPUConfig{})
			Expect(err).NotTo(HaveOccurred())
			// Table has 4 signals; only 2 are capable (pressure, saturation).
			// throttling and steal are NoInstrument and must not appear here.
			Expect(status.SignalsCapable).To(Equal(2),
				"NoInstrument signals are excluded from the capable count, so a bare box shows its real answerable set")
		})
	})

	Describe("spec 4 — keep admitting through a read outage on a signal that already measured", func() {
		It("keeps counting a first-measured signal as measured, however long a later outage lasts", func() {
			// Tick 1: pressure first-measures (Ready, judged on the first
			// sample), so its first-fill bit is set.
			d := newDeps(stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				return cpuhealth.Sample{
					Timestamp: time.Now(),
					Quota:     diagnosis.Known(0),
					NrPeriods: diagnosis.Known(1),
					Pressure:  diagnosis.Known(0.2),
					// PSI present: the box is pressurable (F17 rung 1 derives
					// HasPressureStats from the sticky PsiAvailable).
					PsiAvailable: true,
				}, nil
			}}, 4, 0)

			status1, err := Poll(context.Background(), d, CPUConfig{})
			Expect(err).NotTo(HaveOccurred())
			Expect(status1.SignalsMeasured).To(Equal(1), "pressure first-measured on tick 1")

			// Tick 2 onward: a read outage makes pressure unreadable. The
			// first-fill bit must NOT be cleared, so a signal that has measured
			// keeps the worker admitting (F10's 'refuses on thin evidence, never
			// on stale evidence' — the frozen arm of NoneReady must not refuse).
			d.sampler = stubSampler{read: func(context.Context) (cpuhealth.Sample, error) {
				return cpuhealth.Sample{
					Timestamp: time.Now(),
					Quota:     diagnosis.Known(0),
					NrPeriods: diagnosis.Known(1),
					// Pressure unreadable during the outage. PsiAvailable stays
					// true (sticky, F17 rung 1) — an outage removes the reading,
					// not the box's pressurability, so pressure stays capable.
					Pressure:    diagnosis.Unknown(),
					PsiAvailable: true,
				}, nil
			}}

			status2, err := Poll(context.Background(), d, CPUConfig{})
			Expect(err).NotTo(HaveOccurred())
			Expect(status2.SignalsMeasured).To(Equal(1),
				"a signal that already measured keeps counting measured through the outage (the first-fill bit is never cleared)")
		})
	})
})
