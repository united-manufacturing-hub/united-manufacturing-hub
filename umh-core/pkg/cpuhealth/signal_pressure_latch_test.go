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

// Pressure. The kernel already averaged PSI over 60s, so the
// pressure instrument is Last over its window, minimum 1, and can fire on tick
// 0. The window refuses NaN and infinite readings at append, so a run of them
// freezes the window on its last real contents and the latch HOLDS rather than
// clearing on a fabricated zero; a negative is finite and enters the window, so
// it clears a fired latch because it is below every mark. No clamp exists in
// this package and the tests pin that absence.
package cpuhealth

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("pressure", func() {
	// pressureObserve drives one pressure tick through a fresh engine and
	// returns the fired signal names plus the pressure reduction and state.
	pressureObserve := func(engine *diagnosis.Engine[Sample], base time.Time, i int, p float64) ([]string, float64, diagnosis.State) {
		smp := Sample{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			CpuScope:  ScopeHost,
			Pressure:  diagnosis.Known(p),
		}
		fired, _ := engine.Observe(smp, diagnosis.NewEnvironment(HasLimit, HasPressureStats), smp.Timestamp)
		v, st := engine.Reduction("pressure", "pressure-avg60").Get()
		return firedSignalNames(fired), v, st
	}

	It("should threshold pressure on the last value in its window", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		base := time.Now()

		// Last, minimum 1: a single reading above the 0.20 fire mark fires on
		// tick 0 — pressure does not wait for a window to fill.
		fired, v, st := pressureObserve(engine, base, 0, 0.40)
		Expect(fired).To(ContainElement("pressure"), "a 0.40 reading must fire pressure on the first tick")
		Expect(st).To(Equal(diagnosis.StateValue))
		Expect(v).To(Equal(0.40))

		// A below-clear reading with full coverage releases the latch. The
		// window is full from the tick the first entry falls off (tick 60+).
		for i := 1; i <= 60; i++ {
			fired, _, _ = pressureObserve(engine, base, i, 0.40)
			Expect(fired).To(ContainElement("pressure"), "0.40 stays above the clear mark, tick %d", i)
		}
		fired, v, st = pressureObserve(engine, base, 61, 0.05)
		Expect(fired).NotTo(ContainElement("pressure"), "a 0.05 reading below the 0.12 clear mark must release the latch")
		Expect(st).To(Equal(diagnosis.StateValue))
		Expect(v).To(Equal(0.05))
	})

	It("should hold a fired pressure latch through a run of NaN or infinite readings, rather than clearing on a zero", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		base := time.Now()

		fired, _, _ := pressureObserve(engine, base, 0, 0.40)
		Expect(fired).To(ContainElement("pressure"))

		// NaN, +Inf and -Inf are refused at the window's append: nothing
		// lands, the window freezes on its last real contents (0.40), the
		// reduction is StateUntrusted (nothing appended this tick), and the
		// latch HOLDS. A clamp-to-zero would turn the refusal into a healthy
		// zero and clear — the defect the append refusal exists to remove.
		nonFinite := []struct {
			name string
			v    float64
		}{
			{"NaN", math.NaN()},
			{"+Inf", math.Inf(1)},
			{"-Inf", math.Inf(-1)},
		}
		tick := 1
		for _, nf := range nonFinite {
			for j := 0; j < 10; j++ {
				smp := Sample{
					Timestamp: base.Add(time.Duration(tick) * time.Second),
					CpuScope:  ScopeHost,
					Pressure:  diagnosis.Known(nf.v),
				}
				fired, _ := engine.Observe(smp, diagnosis.NewEnvironment(HasLimit, HasPressureStats), smp.Timestamp)
				Expect(firedSignalNames(fired)).To(ContainElement("pressure"), "the latch must hold through %s", nf.name)
				v, st := engine.Reduction("pressure", "pressure-avg60").Get()
				Expect(st).To(Equal(diagnosis.StateUntrusted), "%s must leave the window untrusted, not cleared", nf.name)
				Expect(v).To(Equal(0.40), "the window must freeze on its last real value, not a fabricated 0")
				tick++
			}
		}
	})

	It("should judge a negative reading as the number it is, and clear on it, because a negative is below every mark", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		base := time.Now()

		// Fill a full-coverage window above the fire mark so the latch is
		// fired AND the clear arm's full-coverage gate is satisfied.
		for i := 0; i <= 60; i++ {
			fired, _, _ := pressureObserve(engine, base, i, 0.40)
			if i == 0 {
				Expect(fired).To(ContainElement("pressure"))
			}
		}

		// A negative is finite, so it is NOT in the NaN/Inf refused class: it
		// enters the window, Last returns it as the number it is, and it clears
		// the fired latch because -0.5 < 0.12. No range check clamps it to 0.
		fired, v, st := pressureObserve(engine, base, 61, -0.5)
		Expect(fired).NotTo(ContainElement("pressure"), "a negative below the clear mark must release the latch")
		Expect(st).To(Equal(diagnosis.StateValue))
		Expect(v).To(Equal(-0.5), "the negative must be judged as the number it is, not clamped to 0")
	})
})
