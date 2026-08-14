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

// The handover at twenty samples. The twentieth sample always
// arrives on a virtualized box, so the swap from the mean arm to the p95 arm
// happens on every start. The p95 is the second-highest of twenty ascending
// entries, and that one fact gives all three specs their numbers. These specs
// bind the engine's per-signal latch to the CPU table's two steal arms
// sharing one mark pair: the swap must move the instrument, not the latch, and
// must judge on the p95's own value from the tick it becomes usable.
package cpuhealth

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

var _ = Describe("the handover at twenty samples", func() {
	// spikeBurst drives steal/spike-below-minsamples' own shape: four samples
	// at 0.90 on ticks 3..6, the rest 0. From n=7 the window holds all four
	// spikes, so the mean is 3.6/n and the p95 is 0.90 at every window size
	// this run reaches.
	spikeBurst := func(i int) float64 {
		if i >= 3 && i <= 6 {
			return 0.90
		}
		return 0
	}

	It("should keep a fired steal latch fired when selection moves from the mean to the p95", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)

		base := time.Now()
		for i := 0; i < 20; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				Steal:       diagnosis.Known(spikeBurst(i)),
			}
			fired, _ := engine.Observe(smp, env, smp.Timestamp)
			// The mean crosses the 0.10 fire mark at n=4 (0.225) and holds.
			if i == 3 {
				Expect(firedSignalNames(fired)).To(ContainElement("steal"), "a 0.225 mean must fire steal at the fourth sample")
			}
			if i == 19 {
				// The twentieth sample: selection has moved to the p95 (0.90),
				// and the latch is still fired. The swap moves the instrument,
				// not the latch.
				Expect(firedSignalNames(fired)).To(ContainElement("steal"), "the fired steal latch must survive the swap to the p95")
				sel, red, _, _ := engine.Select(signalNamed(cpuTable(4, 2.0), "steal"), env)
				Expect(sel.Name).To(Equal("steal-p95"))
				v, st := red.Get()
				Expect(st).To(Equal(diagnosis.StateValue))
				Expect(v).To(Equal(0.90))
			}
		}
	})

	It("should judge steal on the p95's own value from the tick the p95 becomes usable, not on the mean's", func() {
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		stealSignal := signalNamed(cpuTable(4, 2.0), "steal")

		base := time.Now()
		for i := 0; i < 20; i++ {
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				Steal:       diagnosis.Known(spikeBurst(i)),
			}
			engine.Observe(smp, env, smp.Timestamp)
			if i == 18 {
				// The nineteenth sample: still the mean arm (3.6/19 = 0.189),
				// and the value the latch was judged on is the mean's.
				sel, red, _, _ := engine.Select(stealSignal, env)
				Expect(sel.Name).To(Equal("steal-mean"))
				v, st := red.Get()
				Expect(st).To(Equal(diagnosis.StateValue))
				Expect(v).To(BeNumerically("~", 3.6/19, 1e-9))
			}
			if i == 19 {
				// The twentieth sample: the value STEPS to the p95's own 0.90.
				// A build that kept reducing with the mean (0.18) stays fired and
				// passes any state-only assertion, so assert the number.
				_, red, _, _ := engine.Select(stealSignal, env)
				v, st := red.Get()
				Expect(st).To(Equal(diagnosis.StateValue))
				Expect(v).To(Equal(0.90), "the handover must publish the p95's value, not the mean's")
			}
		}
	})

	It("should not fire steal at the handover on a window whose mean sits below the mark", func() {
		// One sample at 0.90, nineteen at 0 — built by hand, because
		// steal/spike-below-minsamples holds four spikes and fires on both arms.
		// The spike sits at the LAST sample so the mean is below the fire mark
		// for the whole run: a spike at sample 0 would fire the mean at n=2
		// (0.45) and then HOLD through the handover, because the clear arm is
		// gated on full window coverage and the window is not full at n=20 —
		// "nothing fires" would be unobservable. With the spike last, the mean
		// peaks at 0.047 and the twenty-sample p95 is the second-highest, 0.
		engine, err := NewEngine(4, 2.0)
		Expect(err).NotTo(HaveOccurred())
		env := diagnosis.NewEnvironment(HasVirtualization, HasLimit)
		stealSignal := signalNamed(cpuTable(4, 2.0), "steal")

		base := time.Now()
		for i := 0; i < 20; i++ {
			steal := 0.0
			if i == 19 {
				steal = 0.90
			}
			smp := Sample{
				Timestamp:   base.Add(time.Duration(i) * time.Second),
				CpuScope:    ScopeHost,
				Virtualized: true,
				Steal:       diagnosis.Known(steal),
			}
			fired, _ := engine.Observe(smp, env, smp.Timestamp)
			Expect(firedSignalNames(fired)).NotTo(ContainElement("steal"), "steal must not fire at any tick, tick %d included", i)
			if i == 19 {
				// The handover: selection has moved to the p95, whose value here
				// is 0 — below the fire mark. Nothing fires at the swap.
				sel, red, _, _ := engine.Select(stealSignal, env)
				Expect(sel.Name).To(Equal("steal-p95"))
				v, st := red.Get()
				Expect(st).To(Equal(diagnosis.StateValue))
				Expect(v).To(Equal(0.0), "the p95 of twenty entries with a single spike is the second-highest: 0")
			}
		}
	})
})
