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
// rather than returned beside its parent. One reads the other side, where the
// parent is silent and its fired refinement is reported nowhere. The last one
// hands the returned set to Rank, which orders top-level signals only.
var _ = Describe("Engine.Observe on a signal's refinements", func() {

	type snap struct {
		parent float64
		first  float64
		second float64
	}

	base := time.Unix(1_000_000, 0)

	// A top-level signal reads its newest sample, so it fires on the tick its own
	// value crosses 0.5 and needs no warm-up: every spec below can put the firing
	// tick wherever it wants the child observed. Name, tier and the field it
	// reads are the caller's, so two of them can sit in one table and rank
	// against each other.
	topLevel := func(name string, tier int, read func(snap) float64, refinements ...Signal[snap]) Signal[snap] {
		return Signal[snap]{
			Name:        name,
			Tier:        tier,
			DemoteSpan:  60 * time.Second,
			Refinements: refinements,
			Instruments: []Instrument[snap]{{
				Measurement: Measurement[snap]{
					Name:      "I",
					Extract:   func(s snap) Reading { return Known(read(s)) },
					Reduction: Last,
					Span:      10 * time.Second,
				},
				Marks: Marks{Unit: "ratio", Fire: Mark{At: 0.5}, Clear: Mark{At: 0.4}, Worst: 1.0, Polarity: HigherIsWorse},
			}},
		}
	}

	// The parent the reporting specs hang their refinements under: named "P", at
	// tier 0, reading the snapshot's parent field.
	parentOver := func(refinements ...Signal[snap]) Signal[snap] {
		return topLevel("P", 0, func(s snap) float64 { return s.parent }, refinements...)
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
	// Rank is asked a question about the top-level set: which of the signals the
	// caller can act on comes first. A refinement's urgency is meaningful under
	// its parent, not against its parent's peers, so it must not enter that
	// comparison at any position. The fixture makes the refinement the entry
	// that would win every key it could be ranked on -- the lowest tier in the
	// table, and the index that wins the last tie-break -- so a build that
	// ranked it would put it first and fail loudly, rather than hiding at the
	// end of the slice where a sorted-last refinement would pass either way.
	It("ranks the top-level signals only, leaving a refinement more urgent than any of them nested and out of the ranked set", func() {
		names := func(fired []Fired) []string {
			out := make([]string, len(fired))
			for i, f := range fired {
				out[i] = f.Identity.Signal
			}

			return out
		}

		// Tier 0 is more urgent than either top-level signal, and Index 0 (its
		// position among its siblings) is the value that wins the last tie-break,
		// so no single field carries the fixture. Attribution differs from the
		// parents' too, though Rank never reads it.
		child := refinement("C", Last, func(s snap) float64 { return s.first })
		child.Tier = 0
		child.Attribution = 7

		// Both fire on their first sample, both at tier 1, so only severity
		// separates them: Severe reads 1.0 against a fire mark of 0.5 and a worst
		// of 1.0, scoring 1.0; Mild reads 0.6 and scores 0.2. The refinement hangs
		// under Mild, the one that must stay second, so a comparator reaching into
		// a nested tier to break the tier tie would hoist Mild and be caught.
		severe := topLevel("Severe", 1, func(s snap) float64 { return s.parent })
		mild := topLevel("Mild", 1, func(s snap) float64 { return s.second }, child)

		e, err := NewEngine(Table[snap]{Signals: []Signal[snap]{severe, mild}, Interval: time.Second})
		Expect(err).ToNot(HaveOccurred())

		fired, _ := e.Observe(snap{parent: 1.0, second: 0.6, first: 1.0}, NewEnvironment(), base)

		Expect(names(fired)).To(Equal([]string{"Severe", "Mild"}),
			"Observe returns the two top-level signals that fired, in table order, and the refinement is not one of them")

		ranked := Rank(fired)

		Expect(names(ranked)).To(Equal([]string{"Severe", "Mild"}),
			"the higher-severity top-level signal ranks first; the tier-0 refinement is not ranked at all")
		Expect(names(ranked)).ToNot(ContainElement("C"),
			"a refinement never competes with a top-level signal for rank, however urgent it is")

		// Without this the spec would also pass on a build that simply dropped the
		// refinement, which is a different defect, not the absence being asserted.
		Expect(ranked[1].Refinements).To(HaveLen(1), "the refinement is still reported, nested under the parent it narrows")
		Expect(ranked[1].Refinements[0].Identity.Signal).To(Equal("C"))
		Expect(ranked[1].Refinements[0].Tier).To(Equal(0),
			"the nested entry keeps the urgent tier it was declared with; it is unranked, not demoted")
		Expect(ranked[1].Refinements[0].Index).To(Equal(0),
			"and the index that wins the last tie-break, so the absence above is not the refinement sorting last")
	})

})
