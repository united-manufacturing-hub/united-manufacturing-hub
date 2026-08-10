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

// Suite is the generator spec. This package has no consumers, so the generator
// is exercised over a fake table whose two signals are generic, never a domain
// vocabulary.

// snap is the fake snapshot the suite ranges over: a single Reading that a
// one-series instrument extracts. Its value is set by the feed's two methods.
type snap struct{ r Reading }

// feed is the suite's other half. Readable returns a strictly increasing value
// derived from at, as the Feed contract requires; Unreadable returns an
// all-absent snapshot, or, under the mutant feed, Known(0) so the window keeps
// filling through the outage.
type feed struct{ mutant bool }

func (f feed) Readable(at time.Time) snap { return snap{r: Known(float64(at.UnixNano()))} }

func (f feed) Unreadable(at time.Time) snap {
	if f.mutant {
		return snap{r: Known(0)}
	}
	return snap{r: Unknown()}
}

var _ = Describe("Suite", func() {
	extract := func(s snap) Reading { return s.r }

	marks := func() Marks {
		return Marks{Unit: "ratio", Fire: Mark{At: 2.0, Inclusive: true}, Worst: 4.0, Clear: Mark{At: 1.0, Inclusive: true}, Polarity: HigherIsWorse}
	}

	instrument := func(red Reduction) Instrument[snap] {
		return Instrument[snap]{Name: "I", Requires: []Capability{"source-1"}, Extract: extract, Red: red, Span: 60 * time.Second, Marks: marks()}
	}

	// Signal A reduces by Last (m == 1), signal B by Mean (m == 2). Both require
	// the same capability, so m for each is defined under the fully capable env.
	table := func() Table[snap] {
		return Table[snap]{
			Signals: []Signal[snap]{
				{Name: "A", DemoteSpan: 60 * time.Second, Instruments: []Instrument[snap]{instrument(Last)}},
				{Name: "B", DemoteSpan: 60 * time.Second, Instruments: []Instrument[snap]{instrument(Mean)}},
			},
			Interval: time.Second,
		}
	}

	// mFor is the smallest Reduction.Min among a signal's capable instruments
	// under NewEnvironment("source-1"): 1 for A, 2 for B.
	mFor := func(name string) int {
		if name == "B" {
			return 2
		}
		return 1
	}

	expectedAvailability := func(c Case, m int) Availability {
		switch c {
		case CaseLive:
			return Ready
		case CaseBriefOutage:
			return NoneReady
		case CaseLongOutage:
			return AllAbsent
		case CaseUnsupported:
			return NoInstrument
		case CasePostOutageDip:
			if m == 1 {
				return Ready
			}
			return NoneReady
		case CaseBelowFloor:
			if m == 1 {
				return AllAbsent
			}
			return NoneReady
		}
		return NoInstrument
	}

	It("generates one scenario per (signal, case): 6 x len(signals), never over tracks", func() {
		cases := []Case{CaseLive, CaseBriefOutage, CaseLongOutage, CaseUnsupported, CasePostOutageDip, CaseBelowFloor}

		tbl := table()
		scenarios := Suite(tbl)
		Expect(scenarios).To(HaveLen(12), "6 cases x 2 signals")

		for _, name := range []string{"A", "B"} {
			seen := map[Case]bool{}
			for _, sc := range scenarios {
				if sc.Signal == name {
					seen[sc.Case] = true
				}
			}
			for _, c := range cases {
				Expect(seen[c]).To(BeTrue(), "signal %s should appear in case %v", name, c)
			}
		}

		// A table that also declares tracks still emits 6 x len(signals): a
		// track has no availability to assert, so no scenario may name one.
		tbl.Tracks = []Track[snap]{{Name: "T", Extract: extract, Red: Mean, Span: 60 * time.Second}}
		Expect(Suite(tbl)).To(HaveLen(6 * len(tbl.Signals)))

		// A table with only tracks emits zero scenarios.
		Expect(Suite(Table[snap]{Tracks: tbl.Tracks, Interval: time.Second})).To(BeEmpty())
	})

	It("refuses a table whose instrument cannot reach its reduction's minimum within its span at the interval", func() {
		sig := Signal[snap]{Name: "A", DemoteSpan: 60 * time.Second, Instruments: []Instrument[snap]{
			{Name: "I", Requires: []Capability{"source-1"}, Extract: extract, Red: P99, Span: 60 * time.Second, Marks: marks()},
		}}
		tbl := Table[snap]{Signals: []Signal[snap]{sig}, Interval: time.Second}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("100"), "a p99 over 60s at 1s holds 61 entries against a minimum of 100")
	})

	It("does not refuse an instrument whose span can hold its reduction's minimum at the interval", func() {
		sig := Signal[snap]{Name: "A", DemoteSpan: 60 * time.Second, Instruments: []Instrument[snap]{
			{Name: "I", Requires: []Capability{"source-1"}, Extract: extract, Red: P95, Span: 60 * time.Second, Marks: marks()},
		}}
		tbl := Table[snap]{Signals: []Signal[snap]{sig}, Interval: time.Second}
		_, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred(), "a p95 over 60s at 1s holds 61 entries against a minimum of 20")
	})

	It("refuses a track whose span cannot hold its reduction's minimum at the interval", func() {
		tbl := Table[snap]{
			Tracks:   []Track[snap]{{Name: "T", Extract: extract, Red: P99, Span: 60 * time.Second}},
			Interval: time.Second,
		}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("T"))
	})

	It("drives every generated scenario through Observe and reports the availability on the scenario's last tick", func() {
		tbl := table()
		env := NewEnvironment("source-1")
		outcomes := Run(tbl, env, feed{})

		Expect(outcomes).To(HaveLen(12))

		// Every scenario must reach its cell in the six-case table.
		for _, o := range outcomes {
			Expect(o.Availability).To(Equal(expectedAvailability(o.Case, mFor(o.Signal))),
				"signal %s case %v reached %v", o.Signal, o.Case, o.Availability)
		}
	})

	It("proves the generator has teeth: a feed that fills through an outage makes the outage cases reach the wrong availability", func() {
		tbl := table()
		env := NewEnvironment("source-1")

		correct := Run(tbl, env, feed{})
		mutant := Run(tbl, env, feed{mutant: true})

		byScenario := func(outcomes []Outcome) map[Scenario]Availability {
			m := make(map[Scenario]Availability, len(outcomes))
			for _, o := range outcomes {
				m[o.Scenario] = o.Availability
			}
			return m
		}
		correctBy := byScenario(correct)
		mutantBy := byScenario(mutant)

		sc := func(sig string, c Case) Scenario { return Scenario{Signal: sig, Case: c} }

		// The mutant returns Known(0) where the correct feed returns Unknown, so
		// the window never freezes and never empties. On signal B (m == 2) all
		// three outage cases must regress to Ready, each differing from what the
		// correct feed reached.
		Expect(mutantBy[sc("B", CaseBriefOutage)]).To(Equal(Ready))
		Expect(mutantBy[sc("B", CaseBriefOutage)]).ToNot(Equal(correctBy[sc("B", CaseBriefOutage)]))
		Expect(mutantBy[sc("B", CaseLongOutage)]).To(Equal(Ready))
		Expect(mutantBy[sc("B", CaseLongOutage)]).ToNot(Equal(correctBy[sc("B", CaseLongOutage)]))
		Expect(mutantBy[sc("B", CasePostOutageDip)]).To(Equal(Ready))
		Expect(mutantBy[sc("B", CasePostOutageDip)]).ToNot(Equal(correctBy[sc("B", CasePostOutageDip)]))

		// On signal A (m == 1) the brief and long outages regress; the post-outage
		// dip already expects Ready at m == 1, so the mutant is indistinguishable
		// there.
		Expect(mutantBy[sc("A", CaseBriefOutage)]).To(Equal(Ready))
		Expect(mutantBy[sc("A", CaseBriefOutage)]).ToNot(Equal(correctBy[sc("A", CaseBriefOutage)]))
		Expect(mutantBy[sc("A", CaseLongOutage)]).To(Equal(Ready))
		Expect(mutantBy[sc("A", CaseLongOutage)]).ToNot(Equal(correctBy[sc("A", CaseLongOutage)]))
		Expect(mutantBy[sc("A", CasePostOutageDip)]).To(Equal(correctBy[sc("A", CasePostOutageDip)]),
			"at m == 1 the dip already expects Ready, so the mutant is indistinguishable")

		// CaseBelowFloor never calls Unreadable, every tick is readable, so the
		// mutant is indistinguishable from the correct feed there.
		Expect(mutantBy[sc("B", CaseBelowFloor)]).To(Equal(correctBy[sc("B", CaseBelowFloor)]))
	})

	It("yields NoneReady for CaseBriefOutage even at the DemoteSpan == Interval boundary, because a brief outage always drives at least one unreadable tick", func() {
		// DemoteSpan == Interval is legal (validate refuses only a demote BELOW the
		// interval), so demoteTicks == 1. A brief outage must still drive one
		// unreadable tick; without it the window would report Ready instead of the
		// documented NoneReady.
		sig := Signal[snap]{Name: "A", DemoteSpan: time.Second, Instruments: []Instrument[snap]{instrument(Last)}}
		tbl := Table[snap]{Signals: []Signal[snap]{sig}, Interval: time.Second}
		env := NewEnvironment("source-1")
		outcomes := Run(tbl, env, feed{})
		byScenario := make(map[Scenario]Availability, len(outcomes))
		for _, o := range outcomes {
			byScenario[o.Scenario] = o.Availability
		}
		Expect(byScenario[Scenario{Signal: "A", Case: CaseBriefOutage}]).To(Equal(NoneReady),
			"a one-interval DemoteSpan is not refused, but the brief outage still freezes the window into NoneReady rather than Ready")
	})

	It("should reach Ready in CaseUnsupported for a signal whose instrument requires nothing, since an empty requirement is satisfied by any environment", func() {
		tbl := Table[snap]{
			Signals: []Signal[snap]{
				{Name: "N", DemoteSpan: 60 * time.Second, Instruments: []Instrument[snap]{
					{Name: "I", Requires: []Capability{}, Extract: extract, Red: Last, Span: 60 * time.Second, Marks: marks()},
				}},
			},
			Interval: time.Second,
		}
		env := NewEnvironment("source-1")
		outcomes := Run(tbl, env, feed{})
		byScenario := make(map[Scenario]Availability, len(outcomes))
		for _, o := range outcomes {
			byScenario[o.Scenario] = o.Availability
		}
		Expect(byScenario[Scenario{Signal: "N", Case: CaseUnsupported}]).To(Equal(Ready),
			"an instrument that requires nothing is satisfied even by the empty CaseUnsupported environment, so the signal is Ready rather than NoInstrument")
	})
})
