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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Instrument is one way of measuring a signal, and Signal.CapableInstruments is
// the capability gate and nothing more: it returns every instrument whose
// required capabilities are present, in table order, and does not pick a winner.
// The gate filters on startup facts, so tests use bare capability names and unit
// strings, which are the caller's vocabulary.
var _ = Describe("Instrument", func() {
	// snap is a generic caller snapshot type holding both counters of a pair.
	type snap struct {
		value, against float64
	}

	It("should read both counters of a delta ratio off one snapshot by construction", func() {
		withAgainst := Instrument[snap]{
			Measurement: Measurement[snap]{
				Extract: func(s snap) Reading { return Known(s.value) },
				Against: func(s snap) Reading { return Known(s.against) },
			},
		}

		value, against := withAgainst.Read(snap{value: 3, against: 40})
		vv, vok := value.Get()
		Expect(vok).To(BeTrue(), "the extractor's reading is present")
		Expect(vv).To(Equal(3.0), "the extractor's reading is the value")

		av, aok := against.Get()
		Expect(aok).To(BeTrue(), "the denominator counter is present")
		Expect(av).To(Equal(40.0), "the denominator counter comes off the same snapshot")
	})

	It("should read an absence denominator when the instrument has no Against", func() {
		noAgainst := Instrument[snap]{
			Measurement: Measurement[snap]{
				Extract: func(s snap) Reading { return Known(s.value) },
			},
		}

		value, against := noAgainst.Read(snap{value: 5})
		vv, ok := value.Get()
		Expect(ok).To(BeTrue(), "a single-series instrument still reads its value, present")
		Expect(vv).To(Equal(5.0), "a single-series instrument still reads its value")
		_, ok = against.Get()
		Expect(ok).To(BeFalse(), "a nil Against yields an absence, never a zero denominator")
	})

	It("should carry its own fire and clear marks, so one signal can hold several mark pairs in different units", func() {
		ratio := Instrument[snap]{
			Measurement: Measurement[snap]{
				Name: "B-ratio",
			},
			Marks: Marks{
				Unit:     "ratio",
				Fire:     Mark{At: 0.9, Inclusive: true},
				Clear:    Mark{At: 0.7, Inclusive: true},
				Polarity: HigherIsWorse,
			},
		}
		cores := Instrument[snap]{
			Measurement: Measurement[snap]{
				Name: "B-cores",
			},
			Marks: Marks{
				Unit:     "cores",
				Fire:     Mark{At: 2, Inclusive: true},
				Clear:    Mark{At: 1, Inclusive: true},
				Polarity: LowerIsWorse,
			},
		}

		signal := Signal[snap]{Name: "B", Instruments: []Instrument[snap]{ratio, cores}}

		Expect(signal.Instruments[0].Marks.Unit).To(Equal("ratio"), "the first instrument carries its own mark pair in its own unit")
		Expect(signal.Instruments[1].Marks.Unit).To(Equal("cores"), "the second instrument carries a different mark pair in a different unit")
	})

	It("should return every instrument whose required capabilities the environment has, in table order, and none whose it does not", func() {
		signal := Signal[snap]{
			Name: "A",
			Instruments: []Instrument[snap]{
				{Measurement: Measurement[snap]{Name: "A-free", Extract: func(s snap) Reading { return Known(s.value) }}},
				{Measurement: Measurement[snap]{Name: "A-1", Requires: []Capability{"source-1"}, Extract: func(s snap) Reading { return Known(s.value) }}},
				{Measurement: Measurement[snap]{Name: "A-2", Requires: []Capability{"source-2"}, Extract: func(s snap) Reading { return Known(s.value) }}},
			},
		}

		got := signal.CapableInstruments(NewEnvironment("source-1"))
		Expect(got).To(HaveLen(2), "the environment satisfies the free instrument and the source-1 instrument only")
		Expect(got[0].Name).To(Equal("A-free"), "a zero-Requires instrument is satisfied by any environment")
		Expect(got[1].Name).To(Equal("A-1"), "the satisfied instruments come back in table order")
	})

	It("should let an environment with no capabilities satisfy only the zero-Requires instruments", func() {
		signal := Signal[snap]{
			Name: "A",
			Instruments: []Instrument[snap]{
				{Measurement: Measurement[snap]{Name: "A-1", Requires: []Capability{"source-1"}, Extract: func(s snap) Reading { return Known(s.value) }}},
				{Measurement: Measurement[snap]{Name: "A-free", Extract: func(s snap) Reading { return Known(s.value) }}},
			},
		}

		got := signal.CapableInstruments(NewEnvironment())
		Expect(got).To(HaveLen(1), "an empty environment satisfies only the zero-Requires instrument")
		Expect(got[0].Name).To(Equal("A-free"))
	})

	It("should return all satisfied instruments rather than the first, so two declaring the same capability both survive the gate", func() {
		signal := Signal[snap]{
			Name: "A",
			Instruments: []Instrument[snap]{
				{Measurement: Measurement[snap]{Name: "A-p95", Requires: []Capability{"source-1"}, Extract: func(s snap) Reading { return Known(s.value) }}},
				{Measurement: Measurement[snap]{Name: "A-mean", Requires: []Capability{"source-1"}, Extract: func(s snap) Reading { return Known(s.value) }}},
			},
		}

		got := signal.CapableInstruments(NewEnvironment("source-1"))
		Expect(got).To(HaveLen(2), "both instruments declaring the same capability survive the gate")
		Expect(got[0].Name).To(Equal("A-p95"), "the first match keeps its place in table order")
		Expect(got[1].Name).To(Equal("A-mean"), "the second match is not discarded by a gate that picks a winner")
	})
})
