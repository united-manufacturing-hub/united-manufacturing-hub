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

// S3 R6 (F1, F4, F5): hold and demote. Do not split per defect — F1, F4 and F5
// are one mechanism: a failed read is not stored as a real zero (the window
// refuses it, so the reduction is StateUntrusted and the latch HOLDS), a fired
// latch does not clear on evidence younger than the window it fired on, and a
// hold is bounded — an emptied window releases the latch rather than waiting
// for a readable tick.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("S3 R6 — hold and demote", func() {
	// hostSample drives one tick of the host-full shape: a known host-busy value
	// when hbKnown is true, a failed read (Unknown) when it is false.
	hostSample := func(base time.Time, i int, hb float64, hbKnown bool) Sample {
		smp := Sample{
			Timestamp:   base.Add(time.Duration(i) * time.Second),
			CpuScope:    ScopeHost,
			HostBusy:    diagnosis.Unknown(),
			Pressure:    diagnosis.Known(0),
			Steal:       diagnosis.Known(0),
			NrPeriods:   diagnosis.Known(0),
			NrThrottled: diagnosis.Known(0),
			UsageCores:  diagnosis.Known(0),
		}
		if hbKnown {
			smp.HostBusy = diagnosis.Known(hb)
		}
		return smp
	}

	It("should hold a fired latch while its input is stale", func() {
		// Host-full fires at the second sample (headroom 4 - 3.5 - 1.0 = -0.5),
		// then host stats go stale: the failed read appends nothing, the window
		// freezes on its last real contents, the reduction is StateUntrusted,
		// and the latch HOLDS — it does not clear on evidence younger than the
		// window it fired on (F4).
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()

		for i := 0; i < 40; i++ {
			fired, _ := engine.Observe(hostSample(base, i, 3.5, i <= 4), env, base.Add(time.Duration(i)*time.Second))
			names := firedSignalNames(fired)
			if i == 1 {
				Expect(names).To(ContainElement("saturation"), "headroom -0.5 must fire host-full at two samples")
			}
			if i >= 5 && i < 40 {
				Expect(names).To(ContainElement("saturation"), "a stale input must hold the fired latch, tick %d", i)
				v, st := engine.Reduction(sigSaturation, instHostHeadroom).Get()
				Expect(st).To(Equal(diagnosis.StateUntrusted), "a stale window is untrusted, not cleared, tick %d", i)
				Expect(v).To(BeNumerically("~", -0.5, 1e-9), "the window must freeze on its last real value, tick %d", i)
			}
		}
	})

	It("should release a held latch when its window reports absent, rather than on the first readable tick", func() {
		// The hold is bounded by the demote span: once the stale window ages past
		// it, the signal reports AllAbsent and the latch RELEASES. BOTH saturation
		// arms must go absent for the signal to report AllAbsent — if usage-fraction
		// stayed ready it would clear the latch on its own clear arm instead, which
		// is the fallback-clear route, not this one. The first readable tick after
		// the outage does not re-fire it when the value is below the mark (F5: no
		// verdict outlives its evidence; F4: no re-fire on the first readable tick
		// without crossing the mark).
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit)
		base := time.Now()

		for i := 0; i <= 70; i++ {
			known := i <= 4
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				HostBusy:    diagnosis.Unknown(),
				UsageCores:  diagnosis.Unknown(),
				Pressure:    diagnosis.Known(0),
				Steal:       diagnosis.Known(0),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
			}
			if known {
				smp.HostBusy = diagnosis.Known(3.5)
				smp.UsageCores = diagnosis.Known(0.2)
			}
			fired, _ := engine.Observe(smp, env, smp.Timestamp)
			names := firedSignalNames(fired)
			if i >= 5 && i < 64 {
				Expect(names).To(ContainElement("saturation"), "the fired latch must hold through the outage, tick %d", i)
			}
			if i == 65 {
				Expect(names).NotTo(ContainElement("saturation"), "the held latch must release once the window reports absent")
				_, st := engine.Reduction(sigSaturation, instHostHeadroom).Get()
				Expect(st).To(Equal(diagnosis.StateAbsent), "the emptied window is absent, not untrusted")
			}
		}

		// First readable tick after the outage, headroom 4 - 0.5 - 1.0 = 2.5
		// (well below the fire mark): the latch must NOT re-fire on it.
		smp := hostSample(base, 71, 0.5, true)
		fired, _ := engine.Observe(smp, env, smp.Timestamp)
		Expect(firedSignalNames(fired)).NotTo(ContainElement("saturation"), "a sub-mark first readable tick must not re-fire the released latch")
	})

	It("should not store a failed read as a real zero", func() {
		// F1 on the pressure signal: a fired latch held through a FAILED read
		// (Unknown, not NaN) — the window refuses it, so the reduction keeps
		// returning the last real value (0.40) as StateUntrusted, never a
		// fabricated zero that would clear the latch.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasLimit, HasPressureStats)
		base := time.Now()

		for i := 0; i < 30; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Pressure:    diagnosis.Unknown(),
				Steal:       diagnosis.Known(0),
				NrPeriods:   diagnosis.Known(0),
				NrThrottled: diagnosis.Known(0),
				UsageCores:  diagnosis.Known(0),
				HostBusy:    diagnosis.Known(0.1),
			}
			if i == 0 {
				smp.Pressure = diagnosis.Known(0.40)
			}
			fired, _ := engine.Observe(smp, env, smp.Timestamp)
			names := firedSignalNames(fired)
			if i == 0 {
				Expect(names).To(ContainElement("pressure"), "a 0.40 reading must fire pressure")
			}
			if i >= 1 {
				Expect(names).To(ContainElement("pressure"), "a failed read must hold the fired pressure latch, tick %d", i)
				v, st := engine.Reduction(sigPressure, instPressureAvg60).Get()
				Expect(st).To(Equal(diagnosis.StateUntrusted), "a failed read leaves the window untrusted, tick %d", i)
				Expect(v).To(Equal(0.40), "the failed read must not be stored as a zero, tick %d", i)
			}
		}
	})
})
