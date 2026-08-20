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
// parent went; only whether the parent reports its VERDICT turns on the parent
// having fired. Most specs here read the reporting side on a tick where the
// parent HAS fired: which refinements nest under it, and that a refinement is
// nested rather than returned beside its parent. One reads the other side, where
// the parent is silent and its fired refinement is reported nowhere. One hands
// the returned set to Rank, which orders top-level signals only. Three read the
// order the refinements under one signal come back in. One reads Observe's
// other return value, the readiness rows, which every signal gets at every depth
// whichever way it or its parent went. Two read how a refinement LEAVES a
// parent's report once it is in: on its own clear mark, and on its own demote
// clock.
var _ = Describe("Engine.Observe on a signal's refinements", func() {

	type snap struct {
		parent float64
		first  float64
		second float64
		third  float64
	}

	base := time.Unix(1_000_000, 0)

	names := func(fired []Fired) []string {
		out := make([]string, len(fired))
		for i, f := range fired {
			out[i] = f.Identity.Signal
		}

		return out
	}

	// A top-level signal reads its newest sample, so it fires on the tick its own
	// value crosses 0.5 and needs no warm-up: every spec below can put the firing
	// tick wherever it wants the refinement observed. Name, tier and the field it
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
	// to read, under whichever reduction the spec needs. Tier is the caller's, as
	// it is on topLevel, so two refinements under one parent can differ in
	// urgency; refinements of its own let a spec hang a third level under it.
	refinement := func(name string, tier int, reduction Reduction, read func(snap) float64, refinements ...Signal[snap]) Signal[snap] {
		return Signal[snap]{
			Name:        name,
			Tier:        tier,
			DemoteSpan:  60 * time.Second,
			Refinements: refinements,
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

	// The refinement's window is filled on each of the parent's ten silent
	// ticks, a 1.0 every time, so the refinement reaches Mean's minimum of two
	// samples and fires on tick 1 — nine ticks before its parent fires at all.
	// Since is what proves that: a refinement sampled or judged only while its
	// parent fires would hold the single 0.0 of tick 10, sit below Mean's
	// minimum, and never fire.
	//
	// Two choices in the fixture are load-bearing. Mean over a minimum of two is
	// what makes a cold window unable to answer; Last would answer off one
	// sample. And the 0.0 at tick 10 is what a build reducing the window at
	// REPORT time would report, 10/11 rather than the 1.0 the refinement latched.
	It("reports a fired parent's refinement nested under it, warm over ten ticks, not as a top-level entry beside it", func() {
		e := engineOver(parentOver(refinement("C", 0, Mean, func(s snap) float64 { return s.first })))

		for i := 0; i < 10; i++ {
			e.Observe(snap{parent: 0.0, first: 1.0}, NewEnvironment(), base.Add(time.Duration(i)*time.Second))
		}

		// The parent crosses its fire mark (0.0 -> 1.0) while the refinement's
		// newest sample drops below the refinement's own (1.0 -> 0.0). Its mean
		// over the warm window, 10/11, stays past its 0.4 clear mark, so the
		// refinement is still held.
		fired, _ := e.Observe(snap{parent: 1.0, first: 0.0}, NewEnvironment(), base.Add(10*time.Second))

		Expect(fired).To(HaveLen(1), "only the fired parent is a top-level entry; the refinement nests under it")
		Expect(fired[0].Identity.Signal).To(Equal("P"))
		Expect(fired[0].Refinements).To(HaveLen(1), "the fired parent reports the refinement its own latch holds")
		Expect(fired[0].Refinements[0].Identity.Signal).To(Equal("C"), "the nested entry names the refinement")
		Expect(fired[0].Refinements[0].Since).To(Equal(base.Add(time.Second)),
			"the refinement fired on tick 1, off a window warmed while the parent was silent")
		Expect(fired[0].Refinements[0].Value).To(Equal(1.0),
			"the value is the mean the refinement fired on, not the window's 10/11 at report time")
	})

	It("reports only the refinement past its own fire mark, naming it", func() {
		e := engineOver(parentOver(
			refinement("C1", 0, Last, func(s snap) float64 { return s.first }),
			refinement("C2", 0, Last, func(s snap) float64 { return s.second }),
		))

		// One tick is enough: Last needs a single sample, so the parent and both
		// refinements reduce to a trustworthy value on the first tick. The parent
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
			return parentOver(refinement("C", 0, Mean, func(s snap) float64 { return s.first }))
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

		// One sample is below Mean's minimum of two, so the refinement's window
		// holds nothing trustworthy to judge. That sample sits past the
		// refinement's fire mark, so a build that reported the window without
		// asking whether it was ready would report the refinement here too.
		cold := engineOver(declaration())
		fired, _ := cold.Observe(snap{parent: 1.0, first: 1.0}, NewEnvironment(), base)

		Expect(fired).To(HaveLen(1), "the parent fires on its own single sample")
		Expect(fired[0].Identity.Signal).To(Equal("P"))
		Expect(fired[0].Refinements).To(BeEmpty(),
			"an unready refinement is absent, not present at Value 0 with no marks and no instrument")
	})

	It("stamps a held refinement's Since once, on the tick it fired", func() {
		e := engineOver(parentOver(refinement("C", 0, Last, func(s snap) float64 { return s.first })))

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
		e := engineOver(parentOver(refinement("C", 0, Mean, func(s snap) float64 { return s.first })))

		// Tick 0 leaves the refinement below Mean's minimum of two. Tick 1
		// reaches it at a mean of 1.0, past the refinement's 0.6 fire mark, so
		// the refinement fires here and this is the Since the later ticks must
		// still carry.
		e.Observe(snap{parent: 0.0, first: 1.0}, NewEnvironment(), base)
		e.Observe(snap{parent: 0.0, first: 1.0}, NewEnvironment(), base.Add(time.Second))

		// The tick after the refinement fired. The parent is still at 0.0, below
		// its own 0.5 fire mark.
		silent, _ := e.Observe(snap{parent: 0.0, first: 1.0}, NewEnvironment(), base.Add(2*time.Second))

		Expect(silent).To(BeEmpty(),
			"a fired refinement under a silent parent is reported nowhere: not nested, and not as a top-level entry of its own")

		// Suppression is about reporting only, so the refinement's window keeps
		// taking samples across the silent ticks and stays trustworthy.
		value, state := e.Reduction("P/C", "I").Get()
		Expect(state).To(Equal(StateValue), "the refinement's window is still warm while the parent is silent")
		Expect(value).To(Equal(1.0), "and still reduces to the mean it fired on")

		// Nothing about the refinement changes on this tick; only the parent
		// crosses its mark. The refinement appearing now, with the Since it
		// stamped on tick 1, is what shows its latch held across the two silent
		// ticks rather than firing fresh here.
		fired, _ := e.Observe(snap{parent: 1.0, first: 1.0}, NewEnvironment(), base.Add(3*time.Second))

		Expect(fired).To(HaveLen(1))
		Expect(fired[0].Identity.Signal).To(Equal("P"))
		Expect(fired[0].Refinements).To(HaveLen(1))
		Expect(fired[0].Refinements[0].Identity.Signal).To(Equal("C"))
		Expect(fired[0].Refinements[0].Since).To(Equal(base.Add(time.Second)),
			"the refinement fired on tick 1 and held: the empty slice above was suppression, not a refinement that had not fired")
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
		// Tier 0 is more urgent than either top-level signal, and Index 0 (its
		// position among its siblings) is the value that wins the last tie-break,
		// so no single field carries the fixture. Attribution differs from the
		// parents' too: Rank never reads it, but the caller it routes blame to
		// does, and the last assertion in this spec is the only one holding it to
		// reaching a nested Fired.
		ref := refinement("C", 0, Last, func(s snap) float64 { return s.first })
		ref.Attribution = 7

		// Both fire on their first sample, both at tier 1, so only severity
		// separates them: Severe reads 1.0 against a fire mark of 0.5 and a worst
		// of 1.0, scoring 1.0; Mild reads 0.6 and scores 0.2. The refinement hangs
		// under Mild, the one that must stay second, so a comparator reaching into
		// a nested tier to break the tier tie would hoist Mild and be caught.
		severe := topLevel("Severe", 1, func(s snap) float64 { return s.parent })
		mild := topLevel("Mild", 1, func(s snap) float64 { return s.second }, ref)

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
		Expect(ranked[1].Refinements[0].Attribution).To(Equal(7),
			"and the Attribution it was declared with, the field the consumer routes blame on, carried down to the nested verdict rather than the parent's")
	})

	// Rank orders the top-level set. The three specs below are about the order
	// INSIDE one signal's Refinements, which Rank never touches: lower tier
	// first, so the caller can read the first nested entry as the most urgent
	// narrowing without sorting anything itself.

	// The two refinements are declared in the order a build that ignored Tier
	// would return them, less urgent first. Declaring them already in the
	// reported order would pass whether or not anything ordered them.
	It("reports a parent's refinements lowest tier first, against the order they were declared in", func() {
		e := engineOver(parentOver(
			refinement("LessUrgent", 2, Last, func(s snap) float64 { return s.first }),
			refinement("MoreUrgent", 1, Last, func(s snap) float64 { return s.second }),
		))

		// Last reduces off a single sample, so the parent and both refinements
		// are judged on the first tick and all three cross their marks. The
		// refinements read 0.7, just past their own 0.6, so it is that mark
		// admitting them; a 1.0 would sit past whatever mark they carried.
		fired, _ := e.Observe(snap{parent: 1.0, first: 0.7, second: 0.7}, NewEnvironment(), base)

		Expect(fired).To(HaveLen(1))
		Expect(fired[0].Refinements).To(HaveLen(2), "both refinements crossed their own fire mark")
		Expect(names(fired[0].Refinements)).To(Equal([]string{"MoreUrgent", "LessUrgent"}),
			"tier 1 is reported before tier 2 though it was declared second")
	})

	It("keeps refinements of one tier in the order they were declared", func() {
		reported := func(refinements ...Signal[snap]) []string {
			e := engineOver(parentOver(refinements...))
			fired, _ := e.Observe(snap{parent: 1.0, first: 0.7, second: 0.7}, NewEnvironment(), base)

			Expect(fired).To(HaveLen(1))
			Expect(fired[0].Refinements).To(HaveLen(len(refinements)), "every refinement crossed its own fire mark")

			return names(fired[0].Refinements)
		}

		// A pair, both directions, because either direction alone also holds on a
		// build that emits whatever order it was handed.
		a := refinement("A", 1, Last, func(s snap) float64 { return s.first })
		b := refinement("B", 1, Last, func(s snap) float64 { return s.second })

		Expect(reported(a, b)).To(Equal([]string{"A", "B"}))
		Expect(reported(b, a)).To(Equal([]string{"B", "A"}))

		// The pair above cannot tell a declared tie-break from a sort that
		// happens to leave it alone: Go sorts twelve elements or fewer by
		// insertion, which never moves two entries their comparator calls equal.
		// Thirteen across two tiers is the smallest table where a sort comparing
		// tier alone visibly scrambles each tier's declaration order.
		const wide = 13

		spread := make([]Signal[snap], 0, wide)
		lower := make([]string, 0, wide)
		upper := make([]string, 0, wide)

		for i := range wide {
			name := string(rune('A' + i))
			tier := 1 + i%2

			spread = append(spread, refinement(name, tier, Last, func(s snap) float64 { return s.first }))

			if tier == 1 {
				lower = append(lower, name)
			} else {
				upper = append(upper, name)
			}
		}

		expected := make([]string, 0, wide)
		expected = append(expected, lower...)
		expected = append(expected, upper...)

		Expect(reported(spread...)).To(Equal(expected),
			"each tier comes back in declaration order, the lower tier first")
	})

	It("orders the refinements of a refinement by the same rule, so every level is ordered and not just the first", func() {
		ref := refinement("C", 0, Last, func(s snap) float64 { return s.first },
			refinement("LessUrgent", 2, Last, func(s snap) float64 { return s.second }),
			refinement("MoreUrgent", 1, Last, func(s snap) float64 { return s.third }),
		)

		e := engineOver(parentOver(ref))

		fired, _ := e.Observe(snap{parent: 1.0, first: 0.7, second: 0.7, third: 0.7}, NewEnvironment(), base)

		Expect(fired).To(HaveLen(1))
		Expect(fired[0].Refinements).To(HaveLen(1), "the refinement fired and nests under the parent")
		Expect(fired[0].Refinements[0].Refinements).To(HaveLen(2), "both of its own refinements fired too")
		Expect(names(fired[0].Refinements[0].Refinements)).To(Equal([]string{"MoreUrgent", "LessUrgent"}),
			"two levels down the tiers still decide, though these were also declared less urgent first")
	})

	// Every spec above reads the fired set. This one reads the other return
	// value, the readiness rows, which say what each signal's own instruments
	// could tell the engine this tick whether or not it fired. A refinement is
	// judged every tick, so it has an availability of its own, and a row of its
	// own is the only way a caller sees it.
	It("returns one readiness row per signal at every depth, depth-first, named by path and carrying that signal's own availability", func() {
		// G's instrument requires nothing, so it is capable in the same empty
		// environment that leaves C above it with no instrument at all. That is
		// what separates a row resolved per signal from a row handed down the
		// tree.
		nested := refinement("G", 1, Last, func(s snap) float64 { return s.third })

		// C is the one signal here this environment cannot satisfy: its
		// instrument requires a capability the empty environment does not have,
		// so C resolves to NoInstrument while P above it and G below it both
		// read their sample.
		ref := refinement("C", 1, Last, func(s snap) float64 { return s.first }, nested)
		ref.Instruments[0].Requires = []Capability{"psi"}

		// A second top-level signal, declared after P, is what makes the
		// depth-first claim testable: it must come after P's whole subtree and
		// not between P and the subtree, which is where a breadth-first walk
		// would put it.
		e, err := NewEngine(Table[snap]{
			Signals:  []Signal[snap]{parentOver(ref), topLevel("Q", 1, func(s snap) float64 { return s.second })},
			Interval: time.Second,
		})
		Expect(err).ToNot(HaveOccurred())

		_, readiness := e.Observe(snap{parent: 1.0, first: 1.0, second: 1.0, third: 1.0}, NewEnvironment(), base)

		rows := make([]string, len(readiness))
		for i, r := range readiness {
			rows[i] = r.Signal
		}

		Expect(rows).To(Equal([]string{"P", "P/C", "P/C/G", "Q"}),
			"depth-first, a parent immediately before its own refinements, each row named by its path: two parents may each declare a refinement named X, so a bare name would not say which one this is")

		Expect(readiness[0].Availability).To(Equal(Ready), "P reads its sample")
		Expect(readiness[1].Availability).To(Equal(NoInstrument),
			"C's own instrument is unsatisfied here, so its row says so rather than repeating the Ready of the parent it hangs under")
		Expect(readiness[2].Availability).To(Equal(Ready),
			"G reads its sample through an instrument of its own, so an unreadable parent does not make it unreadable")
		Expect(readiness[3].Availability).To(Equal(Ready), "Q reads its sample")
	})

	// A refinement ENTERS a parent's report on its own fire mark, which the spec
	// "reports only the refinement past its own fire mark, naming it" reads. It
	// LEAVES on one of two others: its own clear mark, and its own demote clock.
	// The two specs named for those follow. Both keep the parent firing
	// throughout, so the parent's report is the only thing the refinement can be
	// missing from.

	// The clear arm needs Coverage.Full(), which on the ten-second window this
	// fixture builds takes eleven ticks at the table's one-second interval: t0
	// through t10 span exactly ten seconds. Dropping the refinement earlier
	// would be a release granted on a window that had not filled, which is a
	// different rule.
	It("drops a refinement, and the refinement under it, on the tick the refinement crosses its own clear mark", func() {
		nested := refinement("G", 1, Last, func(s snap) float64 { return s.second })
		e := engineOver(parentOver(refinement("C", 1, Last, func(s snap) float64 { return s.first }, nested)))

		var warm []Fired
		for i := 0; i <= 10; i++ {
			warm, _ = e.Observe(snap{parent: 1.0, first: 0.7, second: 0.7}, NewEnvironment(), base.Add(time.Duration(i)*time.Second))
		}

		Expect(names(warm)).To(Equal([]string{"P"}))
		Expect(names(warm[0].Refinements)).To(Equal([]string{"C"}), "C fired on its first sample and is held")
		Expect(names(warm[0].Refinements[0].Refinements)).To(Equal([]string{"G"}), "and so is G, the refinement nested under C")

		// 0.3 is past C's own 0.4 clear mark. Nothing else moves: the parent
		// still reads 1.0, and G still reads the 0.7 that is past its own fire
		// mark and nowhere near its own clear mark.
		released, _ := e.Observe(snap{parent: 1.0, first: 0.3, second: 0.7}, NewEnvironment(), base.Add(11*time.Second))

		Expect(names(released)).To(Equal([]string{"P"}),
			"the parent is untouched: its own value never left its own marks")
		Expect(released[0].Refinements).To(BeEmpty(),
			"C released on its own clear mark, and G leaves with it: a refinement of an unreported refinement is reported nowhere")

		// What G lost was its route to the report, not its verdict. Latch re-arms
		// one Coverage span after a release, so C cannot fire again before t21
		// however far past its fire mark it reads. When it does, G reappears
		// under it carrying the Since it stamped on the first tick of all, which
		// it could not do had it released too.
		var refired []Fired
		for i := 12; i <= 21; i++ {
			refired, _ = e.Observe(snap{parent: 1.0, first: 0.7, second: 0.7}, NewEnvironment(), base.Add(time.Duration(i)*time.Second))
		}

		Expect(refired[0].Refinements).To(HaveLen(1))
		Expect(refired[0].Refinements[0].Since).To(Equal(base.Add(21*time.Second)),
			"C fired fresh, one window span after the release above")
		Expect(refired[0].Refinements[0].Refinements).To(HaveLen(1))
		Expect(refired[0].Refinements[0].Refinements[0].Since).To(Equal(base),
			"G held the verdict it stamped on tick 0 through the ten ticks it was reported nowhere")
	})

	// A refinement holds a verdict on stale evidence for its OWN DemoteSpan. The
	// refinement here declares five seconds against the parent's sixty, so a
	// build reading the parent's span would still be reporting it on every tick
	// this spec looks at.
	//
	// The tick numbers come from the span and the table interval and nothing
	// else. Latch.ReleaseAfter releases once now is no longer before the last
	// trusted update plus the span, so five seconds at a one-second interval
	// releases on tick 5: the refinement fires and is last trusted on tick 0,
	// holds through ticks 1 to 4, and is gone on tick 5.
	It("releases a refinement on its own demote span, not its parent's, when its instrument goes absent", func() {
		ref := refinement("C", 1, Last, func(s snap) float64 { return s.first })
		ref.DemoteSpan = 5 * time.Second

		// The instrument stops reading rather than reading a low value, so what
		// removes the refinement below is the demote clock and not the clear mark
		// the spec above covers.
		absent := false
		ref.Instruments[0].Extract = func(s snap) Reading {
			if absent {
				return Unknown()
			}

			return Known(s.first)
		}

		e := engineOver(parentOver(ref))

		fired, _ := e.Observe(snap{parent: 1.0, first: 0.7}, NewEnvironment(), base)

		Expect(names(fired)).To(Equal([]string{"P"}))
		Expect(names(fired[0].Refinements)).To(Equal([]string{"C"}), "tick 0 is the refinement's last trusted update")

		absent = true

		// Ticks 1 to 4. The window keeps the point it stored on tick 0 but stores
		// nothing on these, so it reduces untrusted and no tick moves the
		// refinement's clock forward.
		var held []Fired
		for i := 1; i <= 4; i++ {
			held, _ = e.Observe(snap{parent: 1.0}, NewEnvironment(), base.Add(time.Duration(i)*time.Second))
		}

		Expect(names(held[0].Refinements)).To(Equal([]string{"C"}),
			"one tick short of its own five-second span the refinement still reports what it latched")

		gone, _ := e.Observe(snap{parent: 1.0}, NewEnvironment(), base.Add(5*time.Second))

		Expect(names(gone)).To(Equal([]string{"P"}),
			"the parent is sixty seconds from its own release and still firing, so the refinement leaves on a clock of its own")
		Expect(gone[0].Refinements).To(BeEmpty(),
			"five seconds after its last trusted update the refinement releases")
	})

})
