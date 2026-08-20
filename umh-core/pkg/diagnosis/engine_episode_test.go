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

// Every other spec in this package drives one property in isolation. This one
// drives a single engine through one episode — healthy, degraded, healthy again
// — on one clock, and checks that the properties hold together and in sequence.
// The interaction is the point: a refinement's window warming before its parent
// fires is what makes the report at the firing tick worth anything, and neither
// half shows that on its own.
//
// The whole episode is one spec on one engine. Splitting it into several specs
// over a shared fixture would give each one a fresh engine and lose exactly the
// thing being checked, which is what the engine carries from one tick to the
// next.
var _ = Describe("Engine.Observe over one degradation episode", func() {

	// snap is the caller's snapshot type: one field per signal, so a phase can
	// move one signal's own number without touching any other's.
	type snap struct{ p, q, c, g float64 }

	// instrument is the one instrument name every signal here declares. The
	// engine keys a window by (signal path, instrument name), so the hysteresis
	// phase reads C's number back through this same constant rather than a
	// typed-out copy of it.
	const instrument = "I"

	// C's marks, named because the hysteresis phase asserts that C's number sits
	// between them. The other three signals' marks are written inline.
	const cFire, cClear = 0.60, 0.30

	base := time.Unix(1_000_000, 0)

	// at names a tick by its index. The table interval is one second, so tick i
	// is i seconds into the episode, and every instant asserted below is stated
	// as the tick it belongs to.
	at := func(i int) time.Time { return base.Add(time.Duration(i) * time.Second) }

	// names lists one level of a fired set by signal name, so a phase can state
	// the shape of that level in one line.
	names := func(fired []Fired) []string {
		out := make([]string, len(fired))
		for i, f := range fired {
			out[i] = f.Identity.Signal
		}

		return out
	}

	// G narrows C, the third level of the tree. It reduces through Last, so it
	// fires on the tick its own value crosses 0.70 and releases on the tick that
	// value falls below 0.40. Tier 3 is the least urgent in the table.
	signalG := Signal[snap]{
		Name:       "G",
		Tier:       3,
		DemoteSpan: time.Minute,
		Instruments: []Instrument[snap]{{
			Measurement: Measurement[snap]{
				Name:      instrument,
				Extract:   func(s snap) Reading { return Known(s.g) },
				Reduction: Last,
				Span:      10 * time.Second,
			},
			Marks: Marks{Unit: "ratio", Fire: Mark{At: 0.70}, Clear: Mark{At: 0.40}, Worst: 1.0, Polarity: HigherIsWorse},
		}},
	}

	// C narrows P. It reduces a three-second window through Mean, which at a
	// one-second interval holds four samples and needs two before it can answer
	// at all. That is what makes the degrading phase mean something: C cannot
	// answer off the single sample it has on the tick its value first goes bad.
	//
	// Tier 0 makes C the most urgent signal in the table, more urgent than either
	// top-level signal. A refinement that leaked into the ranked set would
	// therefore sort first, where it cannot be missed.
	signalC := Signal[snap]{
		Name:        "C",
		Tier:        0,
		DemoteSpan:  time.Minute,
		Refinements: []Signal[snap]{signalG},
		Instruments: []Instrument[snap]{{
			Measurement: Measurement[snap]{
				Name:      instrument,
				Extract:   func(s snap) Reading { return Known(s.c) },
				Reduction: Mean,
				Span:      3 * time.Second,
			},
			Marks: Marks{Unit: "ratio", Fire: Mark{At: cFire}, Clear: Mark{At: cClear}, Worst: 1.0, Polarity: HigherIsWorse},
		}},
	}

	// P is the top-level signal the episode degrades and recovers. Last again, so
	// P fires on the tick its own value crosses 0.80 and releases on the tick that
	// value falls below 0.50.
	signalP := Signal[snap]{
		Name:        "P",
		Tier:        1,
		DemoteSpan:  time.Minute,
		Refinements: []Signal[snap]{signalC},
		Instruments: []Instrument[snap]{{
			Measurement: Measurement[snap]{
				Name:      instrument,
				Extract:   func(s snap) Reading { return Known(s.p) },
				Reduction: Last,
				Span:      10 * time.Second,
			},
			Marks: Marks{Unit: "ratio", Fire: Mark{At: 0.80}, Clear: Mark{At: 0.50}, Worst: 1.0, Polarity: HigherIsWorse},
		}},
	}

	// Q is a second top-level signal with no refinements, so ranking has two
	// entries to order and readiness has two roots to list.
	signalQ := Signal[snap]{
		Name:       "Q",
		Tier:       2,
		DemoteSpan: time.Minute,
		Instruments: []Instrument[snap]{{
			Measurement: Measurement[snap]{
				Name:      instrument,
				Extract:   func(s snap) Reading { return Known(s.q) },
				Reduction: Last,
				Span:      10 * time.Second,
			},
			Marks: Marks{Unit: "ratio", Fire: Mark{At: 0.80}, Clear: Mark{At: 0.50}, Worst: 1.0, Polarity: HigherIsWorse},
		}},
	}

	// Q is declared FIRST. Observe returns the fired set in table order, so the
	// set comes back Q then P while Rank must put P first on tier. Declaring P
	// first would leave the set already in ranked order, and the ranking
	// assertion would hold on an engine that never sorted anything.
	table := Table[snap]{Signals: []Signal[snap]{signalQ, signalP}, Interval: time.Second}

	// Readiness carries one row per signal at every depth, named by path, in
	// depth-first order over the table: Q, then P and the subtree under it. Every
	// tick of this episode is readable for every signal, so this is what
	// readiness must look like at every point in it.
	allReady := []Readiness{
		{Signal: "Q", Availability: Ready},
		{Signal: "P", Availability: Ready},
		{Signal: "P/C", Availability: Ready},
		{Signal: "P/C/G", Availability: Ready},
	}

	It("warms, nests, ranks, holds through the hysteresis band, and empties from the inside out", func() {
		e, err := NewEngine(table)
		Expect(err).ToNot(HaveOccurred())

		// drive runs ticks from..to inclusive on one snapshot and hands back what
		// the last of those ticks returned. Each phase below states its own tick
		// range, so which tick an assertion reads is on the line above it.
		drive := func(from, to int, s snap) ([]Fired, []Readiness) {
			var (
				fired     []Fired
				readiness []Readiness
			)

			for i := from; i <= to; i++ {
				fired, readiness = e.Observe(s, NewEnvironment(), at(i))
			}

			return fired, readiness
		}

		// Phase 1, healthy, ticks 0 to 4. Every value sits far below every fire
		// mark, so nothing may fire — and every signal must still be reported as
		// readable, since "measured, and fine" is not the same answer as "could
		// not measure".
		//
		// Read at tick 4 rather than tick 0 because C's mean needs two samples:
		// at tick 0 C is NoneReady, which is a warm-up state rather than a
		// healthy one.
		fired, readiness := drive(0, 4, snap{p: 0.10, q: 0.10, c: 0.10, g: 0.10})

		Expect(fired).To(BeEmpty(), "nothing crossed a fire mark, at any depth")
		Expect(readiness).To(Equal(allReady),
			"readiness reports every signal at every depth, refinements included, whether or not anything fired")

		// Phase 2, degrading, ticks 5 to 11. C's and G's own values go bad while
		// P stays far below its own mark. Their windows fill and their latches
		// fire during these ticks, and none of it is reported: a refinement is
		// reported only under a parent that fired, and P has not fired.
		//
		// This emptiness and the Since values read in phase 3 are one assertion in
		// two halves. Emptiness alone would also hold on a build that never
		// judged a refinement until its parent fired.
		fired, readiness = drive(5, 11, snap{p: 0.10, q: 0.10, c: 1.0, g: 1.0})

		Expect(fired).To(BeEmpty(), "C and G have fired by now, and a silent parent reports neither")
		Expect(readiness).To(Equal(allReady), "a refinement has a readiness row whichever way its parent went")

		// Phase 3, degraded, tick 12. P's own value crosses its fire mark, and the
		// verdicts latched during phase 2 are reported, nested under it.
		fired, _ = drive(12, 12, snap{p: 0.90, q: 0.10, c: 1.0, g: 1.0})

		Expect(names(fired)).To(Equal([]string{"P"}),
			"P is the only top-level entry: a refinement is never a verdict of its own")
		Expect(fired[0].Since).To(Equal(at(12)))

		Expect(names(fired[0].Refinements)).To(Equal([]string{"C"}), "C nests under the parent it narrows")

		nested := fired[0].Refinements[0]

		// C's mean over ticks 4 to 7 is 0.775, the first window in which the three
		// bad samples outweigh the healthy one still in it. So C answered off a
		// full four-sample window five ticks before P fired, and did it while P
		// was silent. A build that started C's window when P fired would stamp C
		// at tick 12 with the same value, which is why the tick is asserted and
		// not only the value.
		Expect(nested.Since).To(Equal(at(7)), "C fired on tick 7, off a window warmed while P was below its mark")
		Expect(nested.Since).To(BeTemporally("<", fired[0].Since), "C's episode began before P's")
		Expect(nested.Value).To(Equal(0.775), "the mean C latched at tick 7 is what phase 5 must not disturb")

		Expect(names(nested.Refinements)).To(Equal([]string{"G"}), "the second level of nesting: G under C, not under P")
		Expect(nested.Refinements[0].Since).To(Equal(at(5)),
			"G reads its newest sample, so it fired on the first bad tick, before both C and P")

		// Phase 4, ranked, tick 13. Q's own value crosses its fire mark too, so
		// there are two top-level signals to order.
		fired, _ = drive(13, 13, snap{p: 0.90, q: 0.90, c: 1.0, g: 1.0})

		Expect(names(fired)).To(Equal([]string{"Q", "P"}), "the fired set comes back in table order")
		Expect(names(Rank(fired))).To(Equal([]string{"P", "Q"}),
			"Rank orders the top-level set on tier, P at 1 ahead of Q at 2, and has no refinement to sort: C at tier 0 would come first")

		// Phase 5, hysteresis, ticks 14 to 21. C's own value moves to 0.45, which
		// is past neither of C's marks — below the 0.60 it fires at, above the
		// 0.30 it clears at. Nothing else moves.
		//
		// Ticks 14 to 16 flush the 1.0 samples out of C's three-second window. By
		// tick 17 the window holds four samples of 0.45 and nothing else, so the
		// mean sits in the band for the rest of the phase.
		drive(14, 16, snap{p: 0.90, q: 0.90, c: 0.45, g: 1.0})

		// Five ticks in the band. C must stay reported on every one of them, with
		// the Since it was stamped with on tick 7. This is the whole point of a
		// latch: a build that re-decided from the window each tick would drop C
		// here, and one that re-stamped on every tick it was still bad would move
		// its Since.
		for i := 17; i <= 21; i++ {
			fired, _ = e.Observe(snap{p: 0.90, q: 0.90, c: 0.45, g: 1.0}, NewEnvironment(), at(i))

			value, state := e.Reduction("P/C", instrument).Get()
			Expect(state).To(Equal(StateValue), "C's window is answering, so the hold is not an artefact of an unreadable tick")
			Expect(value).To(BeNumerically(">", cClear), "C's number has not reached its clear mark")
			Expect(value).To(BeNumerically("<", cFire), "C's number would not fire the latch if it were unfired")

			Expect(names(fired)).To(Equal([]string{"Q", "P"}))
			Expect(names(fired[1].Refinements)).To(Equal([]string{"C"}), "C is still reported at tick %d", i)
			Expect(fired[1].Refinements[0].Since).To(Equal(at(7)), "Since is stamped once, on the firing tick, and never refreshed")
			Expect(fired[1].Refinements[0].Value).To(Equal(0.775), "the latched value does not follow the signal either")
		}

		// Phase 6, recovering, ticks 22 to 24. The values return past the clear
		// marks and the report empties from the inside out.
		//
		// Tick 22: G reads its newest sample, 0.10, past its 0.40 clear mark, so G
		// releases. C's mean over ticks 19 to 22 is 0.3625, short of C's 0.30
		// clear mark, so C holds — and holds with nothing under it.
		fired, _ = drive(22, 22, snap{p: 0.90, q: 0.90, c: 0.10, g: 0.10})

		Expect(names(fired)).To(Equal([]string{"Q", "P"}))
		Expect(names(fired[1].Refinements)).To(Equal([]string{"C"}))
		Expect(fired[1].Refinements[0].Refinements).To(BeEmpty(), "G released, so C reports nothing under it")

		// Tick 23: C's mean over ticks 20 to 23 is 0.275, past its clear mark, so
		// C releases. P's own value has not moved, so P holds, now alone.
		fired, _ = drive(23, 23, snap{p: 0.90, q: 0.90, c: 0.10, g: 0.10})

		Expect(names(fired)).To(Equal([]string{"Q", "P"}))
		Expect(fired[1].Refinements).To(BeEmpty(), "C released, so P reports nothing under it")

		// Tick 24: P's and Q's own values return past their clear marks, and the
		// episode is over.
		fired, readiness = drive(24, 24, snap{p: 0.10, q: 0.10, c: 0.10, g: 0.10})

		Expect(fired).To(BeEmpty(), "nothing is fired at any depth")
		Expect(readiness).To(Equal(allReady),
			"readiness lists every signal at every depth after the episode exactly as it did before it")
	})
})
