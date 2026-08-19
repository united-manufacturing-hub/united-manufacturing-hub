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

package diagnosis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// A refinement is a signal that narrows the answer of the signal it hangs
// under. It is sampled every tick and judged every tick, whichever way its
// parent went; only whether the parent REPORTS it turns on the parent having
// fired. Most specs here read the reporting side on a tick where the parent HAS
// fired: which refinements nest under it, and that a refinement is nested
// rather than returned beside its parent. The last one reads the other side,
// where the parent is silent and its fired refinement is reported nowhere.
var _ = Describe("Engine.Observe on a signal's refinements", func() {

	type snap struct {
		parent float64
		first  float64
		second float64
	}

	base := time.Unix(1_000_000, 0)

	// The parent reads its newest sample, so it fires on the tick its own value
	// crosses 0.5 and needs no warm-up: every spec below can put the parent's
	// firing tick wherever it wants the child observed.
	parentOver := func(refinements ...Signal[snap]) Signal[snap] {
		return Signal[snap]{
			Name:        "P",
			DemoteSpan:  60 * time.Second,
			Refinements: refinements,
			Instruments: []Instrument[snap]{{
				Measurement: Measurement[snap]{
					Name:      "I",
					Extract:   func(s snap) Reading { return Known(s.parent) },
					Reduction: Last,
					Span:      10 * time.Second,
				},
				Marks: Marks{Unit: "ratio", Fire: Mark{At: 0.5}, Clear: Mark{At: 0.4}, Worst: 1.0, Polarity: HigherIsWorse},
			}},
		}
	}

	// A refinement fires past 0.6 on whichever field of the snapshot it is told
	// to read, under whichever reduction the spec needs.
	refinement := func(name string, reduction Reduction, read func(snap) float64) Signal[snap] {
		return Signal[snap]{
			Name:       name,
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[snap]{{
				Measurement: Measurement[snap]{
					Name:      "I",
					Extract:   func(s snap) Reading { return Known(read(s)) },
					Reduction: reduction,
					Span:      10 * time.Second,
				},
				Marks: Marks{Unit: "ratio", Fire: Mark{At: 0.6}, Clear: Mark{At: 0.4}, Worst: 1.0, Polarity: HigherIsWorse},
			}},
		}
	}

	engineOver := func(s Signal[snap]) *Engine[snap] {
		e, err := NewEngine(Table[snap]{Signals: []Signal[snap]{s}, Interval: time.Second})
		Expect(err).ToNot(HaveOccurred())

		return e
	}

	// The child's window is filled on each of the parent's ten silent ticks, a
	// 1.0 every time, so the child reaches Mean's minimum of two samples and
	// fires on tick 1 — nine ticks before its parent fires at all. Since is what
	// proves that: a child sampled or judged only while its parent fires would
	// hold the single 0.0 of tick 10, sit below Mean's minimum, and never fire.
	//
	// Two choices in the fixture are load-bearing. Mean over a minimum of two is
	// what makes a cold window unable to answer; Last would answer off one
	// sample. And the 0.0 at tick 10 is what a build reducing the window at
	// REPORT time would report, 10/11 rather than the 1.0 the child latched.
	It("reports a fired parent's refinement nested under it, warm over ten ticks, not as a top-level entry beside it", func() {
		e := engineOver(parentOver(refinement("C", Mean, func(s snap) float64 { return s.first })))

		for i := 0; i < 10; i++ {
			e.Observe(snap{parent: 0.0, first: 1.0}, NewEnvironment(), base.Add(time.Duration(i)*time.Second))
		}

		// The parent crosses its fire mark (0.0 -> 1.0) while the child's newest
		// sample drops below the child's own (1.0 -> 0.0). The child's mean over
		// the warm window, 10/11, stays past its 0.4 clear mark, so the child is
		// still held.
		fired, _ := e.Observe(snap{parent: 1.0, first: 0.0}, NewEnvironment(), base.Add(10*time.Second))

		Expect(fired).To(HaveLen(1), "only the fired parent appears at the top level; the refinement nests under it")
		Expect(fired[0].Identity.Signal).To(Equal("P"))
		Expect(fired[0].Refinements).To(HaveLen(1), "the fired parent reports the refinement its own latch holds")
		Expect(fired[0].Refinements[0].Identity.Signal).To(Equal("C"), "the nested entry names the refinement")
		Expect(fired[0].Refinements[0].Since).To(Equal(base.Add(time.Second)),
			"the child fired on tick 1, off a window warmed while the parent was silent")
		Expect(fired[0].Refinements[0].Value).To(Equal(1.0),
			"the value is the mean the child fired on, not the window's 10/11 at report time")
	})

	It("reports only the refinement past its own fire mark, naming it", func() {
		e := engineOver(parentOver(
			refinement("C1", Last, func(s snap) float64 { return s.first }),
			refinement("C2", Last, func(s snap) float64 { return s.second }),
		))

		// One tick is enough: Last needs a single sample, so the parent and both
		// children reduce to a trustworthy value on the first tick. The parent
		// and C1 cross their marks; C2 sits below its own.
		fired, _ := e.Observe(snap{parent: 1.0, first: 1.0, second: 0.0}, NewEnvironment(), base)

		Expect(fired).To(HaveLen(1), "the parent is the only top-level entry; a fired refinement nests under it")
		Expect(fired[0].Identity.Signal).To(Equal("P"))
		Expect(fired[0].Refinements).To(HaveLen(1))
		Expect(fired[0].Refinements[0].Identity.Signal).To(Equal("C1"),
			"the refinement past its mark is the one reported, not whichever came first")
		Expect(fired[0].Refinements[0].Value).To(Equal(1.0))
	})

	It("leaves out a refinement whose window cannot yet reduce", func() {
		declaration := func() Signal[snap] {
			return parentOver(refinement("C", Mean, func(s snap) float64 { return s.first }))
		}

		// The control: two samples reach Mean's minimum, so the same refinement
		// on the same declaration IS reported. Without it the absence below would
		// also hold on a build that reports no refinement ever.
		warmed := engineOver(declaration())
		warmed.Observe(snap{parent: 1.0, first: 1.0}, NewEnvironment(), base)
		reported, _ := warmed.Observe(snap{parent: 1.0, first: 1.0}, NewEnvironment(), base.Add(time.Second))

		Expect(reported).To(HaveLen(1))
		Expect(reported[0].Refinements).To(HaveLen(1), "two samples are enough for the refinement to be judged")
		Expect(reported[0].Refinements[0].Identity.Signal).To(Equal("C"))

		// One sample is below Mean's minimum of two, so the child's window holds
		// nothing trustworthy to judge. That sample sits past the child's fire
		// mark, so a build that reported the window without asking whether it was
		// ready would report the child here too.
		cold := engineOver(declaration())
		fired, _ := cold.Observe(snap{parent: 1.0, first: 1.0}, NewEnvironment(), base)

		Expect(fired).To(HaveLen(1), "the parent fires on its own single sample")
		Expect(fired[0].Identity.Signal).To(Equal("P"))
		Expect(fired[0].Refinements).To(BeEmpty(),
			"an unready refinement is absent, not present at Value 0 with no marks and no instrument")
	})

	It("stamps a held refinement's Since once, on the tick it fired", func() {
		e := engineOver(parentOver(refinement("C", Last, func(s snap) float64 { return s.first })))

		first, _ := e.Observe(snap{parent: 1.0, first: 1.0}, NewEnvironment(), base)
		second, _ := e.Observe(snap{parent: 1.0, first: 1.0}, NewEnvironment(), base.Add(time.Second))

		Expect(first).To(HaveLen(1), "the parent is the only top-level entry on this tick")
		Expect(second).To(HaveLen(1))
		Expect(first[0].Refinements).To(HaveLen(1))
		Expect(second[0].Refinements).To(HaveLen(1))
		Expect(first[0].Refinements[0].Since).To(Equal(base), "Since is the tick the refinement fired")
		Expect(second[0].Refinements[0].Since).To(Equal(first[0].Refinements[0].Since),
			"the second tick holds the same Since: a refinement's verdict is latched, not re-decided each tick")
	})

	// The three specs above all read a tick where the parent fired, so none of
	// them can tell a build that suppresses a fired refinement from one that
	// emits it beside its parent. This one drives the parent below its fire mark
	// throughout and reads the whole returned slice.
	It("reports nothing at all while the parent is silent, though its refinement has fired and stays warm", func() {
		e := engineOver(parentOver(refinement("C", Mean, func(s snap) float64 { return s.first })))

		// Tick 0 leaves the child below Mean's minimum of two. Tick 1 reaches it
		// at a mean of 1.0, past the child's 0.6 fire mark, so the child fires
		// here and this is the Since the later ticks must still carry.
		e.Observe(snap{parent: 0.0, first: 1.0}, NewEnvironment(), base)
		e.Observe(snap{parent: 0.0, first: 1.0}, NewEnvironment(), base.Add(time.Second))

		// The tick after the child fired. The parent is still at 0.0, below its
		// own 0.5 fire mark.
		silent, _ := e.Observe(snap{parent: 0.0, first: 1.0}, NewEnvironment(), base.Add(2*time.Second))

		Expect(silent).To(BeEmpty(),
			"a fired refinement under a silent parent is reported nowhere: not nested, and not as a top-level entry of its own")

		// Suppression is about reporting only, so the child's window keeps taking
		// samples across the silent ticks and stays trustworthy.
		value, state := e.Reduction("P/C", "I").Get()
		Expect(state).To(Equal(StateValue), "the refinement's window is still warm while the parent is silent")
		Expect(value).To(Equal(1.0), "and still reduces to the mean it fired on")

		// Nothing about the child changes on this tick; only the parent crosses
		// its mark. The child appearing now, with the Since it stamped on tick 1,
		// is what shows its latch held across the two silent ticks rather than
		// firing fresh here.
		fired, _ := e.Observe(snap{parent: 1.0, first: 1.0}, NewEnvironment(), base.Add(3*time.Second))

		Expect(fired).To(HaveLen(1))
		Expect(fired[0].Identity.Signal).To(Equal("P"))
		Expect(fired[0].Refinements).To(HaveLen(1))
		Expect(fired[0].Refinements[0].Identity.Signal).To(Equal("C"))
		Expect(fired[0].Refinements[0].Since).To(Equal(base.Add(time.Second)),
			"the child fired on tick 1 and held: the empty slice above was suppression, not a child that had not fired")
	})
})
