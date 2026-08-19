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
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// NewEngine is the single choke point that refuses a malformed table. Each spec
// builds a table that is valid except for exactly one defect and asserts that
// construction fails, naming the row that is malformed, so a bad state cannot
// be built, and a caller finds out once, at construction, whether the whole
// table is buildable.
var _ = Describe("NewEngine", func() {
	type snap struct{ v float64 }
	extract := func(s snap) Reading { return Known(s.v) }
	against := func(s snap) Reading { return Known(s.v) }

	// Rising, so worse() is the identity: worst 4.0 sits strictly worse than the
	// fire mark 2.0, which is what validate demands of every pair below.
	validMarks := func() Marks {
		return Marks{Unit: "ratio", Fire: Mark{At: 2.0, Inclusive: true}, Clear: Mark{At: 1.0, Inclusive: true}, Polarity: HigherIsWorse, Worst: 4.0}
	}

	validSignal := func(name string) Signal[snap] {
		return Signal[snap]{
			Name:       name,
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[snap]{
				{
					Measurement: Measurement[snap]{
						Name: "I1", Requires: []Capability{"source-1"}, Extract: extract, Reduction: Last, Span: 60 * time.Second,
					},
					Marks: validMarks(),
				},
				{
					Measurement: Measurement[snap]{
						Name: "I2", Requires: []Capability{"source-1"}, Extract: extract, Reduction: Mean, Span: 3 * time.Second,
					},
					Marks: Marks{Unit: "cores", Fire: Mark{At: 8, Inclusive: true}, Clear: Mark{At: 4, Inclusive: true}, Polarity: HigherIsWorse, Worst: 16},
				},
			},
		}
	}

	validTable := func(signals []Signal[snap]) Table[snap] {
		return Table[snap]{Signals: signals, Interval: time.Second}
	}

	It("builds a valid table into an engine without error", func() {
		tbl := validTable([]Signal[snap]{validSignal("A"), validSignal("B")})
		e, err := NewEngine(tbl)
		Expect(err).ToNot(HaveOccurred())
		Expect(e).ToNot(BeNil())
	})

	It("refuses a table whose instrument declares a window span that is zero or negative", func() {
		sig := validSignal("A")
		sig.Instruments[0].Span = 0
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
		Expect(err.Error()).To(ContainSubstring("window span"),
			"an instrument sizes a window, and the measurement spec below reads the other wording; the pair is why validateMeasurement takes the noun as a parameter")
	})

	It("refuses a HigherIsWorse mark pair whose clear mark is not below its fire mark", func() {
		sig := validSignal("A")
		m := validMarks()
		m.Fire = Mark{At: 1.0, Inclusive: true}
		m.Clear = Mark{At: 2.0, Inclusive: true}
		m.Polarity = HigherIsWorse
		sig.Instruments[0].Marks = m
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses a LowerIsWorse mark pair whose clear mark is not above its fire mark", func() {
		sig := validSignal("A")
		m := validMarks()
		m.Fire = Mark{At: 2.0, Inclusive: true}
		m.Clear = Mark{At: 1.0, Inclusive: true}
		m.Polarity = LowerIsWorse
		sig.Instruments[0].Marks = m
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses an ordered reduction on an instrument that declares a boolean series", func() {
		sig := validSignal("A")
		sig.Instruments[0].Reduction = P95
		sig.Instruments[0].Boolean = true
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses an instrument whose reduction has nil fold, which a caller can leave nil by writing a Reduction literal", func() {
		sig := validSignal("A")
		sig.Instruments[0].Reduction = Reduction{Name: "x", Min: 2}
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses a measurement whose reduction has nil fold, which a caller can leave nil by writing a Reduction literal", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Measurements = []Measurement[snap]{{Name: "T", Extract: extract, Span: 60 * time.Second, Reduction: Reduction{Name: "x", Min: 2}}}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("T"))
	})

	It("refuses a table-level measurement that sets Against, which is only meaningful for an instrument", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Measurements = []Measurement[snap]{{Name: "M-against", Extract: extract, Span: 60 * time.Second, Reduction: Last, Against: against}}
		_, err := NewEngine(tbl)
		if err == nil {
			Fail("expected NewEngine to reject a table-level measurement that sets Against")
		}
		Expect(err.Error()).To(ContainSubstring("M-against"))
		Expect(err.Error()).To(ContainSubstring("only meaningful inside a signal"))
	})

	It("refuses a table-level measurement that sets Requires, which is only meaningful for an instrument", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Measurements = []Measurement[snap]{{Name: "M-requires", Extract: extract, Span: 60 * time.Second, Reduction: Last, Requires: []Capability{"psi"}}}
		_, err := NewEngine(tbl)
		if err == nil {
			Fail("expected NewEngine to reject a table-level measurement that sets Requires")
		}
		Expect(err.Error()).To(ContainSubstring("M-requires"))
		Expect(err.Error()).To(ContainSubstring("only meaningful inside a signal"))
	})

	It("refuses a table-level measurement that sets Boolean, which is only meaningful for an instrument", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Measurements = []Measurement[snap]{{Name: "M-boolean", Extract: extract, Span: 60 * time.Second, Reduction: Last, Boolean: true}}
		_, err := NewEngine(tbl)
		if err == nil {
			Fail("expected NewEngine to reject a table-level measurement that sets Boolean")
		}
		Expect(err.Error()).To(ContainSubstring("M-boolean"))
		Expect(err.Error()).To(ContainSubstring("only meaningful inside a signal"))
	})

	It("refuses a table-level measurement that sets Counter, which is only meaningful for an instrument", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Measurements = []Measurement[snap]{{Name: "M-counter", Extract: extract, Span: 60 * time.Second, Reduction: Last, Counter: true}}
		_, err := NewEngine(tbl)
		if err == nil {
			Fail("expected NewEngine to reject a table-level measurement that sets Counter")
		}
		Expect(err.Error()).To(ContainSubstring("M-counter"))
		Expect(err.Error()).To(ContainSubstring("only meaningful inside a signal"))
	})

	It("refuses an instrument whose reduction minimum sample count is below one", func() {
		sig := validSignal("A")
		sig.Instruments[0].Reduction = Reduction{Name: "low", Min: 0, fold: func([]Point) float64 { return 0 }}
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	// A window reaches its floor only if its span can hold that many entries at
	// the interval the caller ticks at; below that it sits at StateUntrusted for
	// good and the row can never produce a verdict. suite_test.go covers the two
	// refusals with minimums far from the capacity, which leaves where each rule
	// cuts unpinned. Capacity counts both edges, span/interval + 1: a 60s span at
	// 1s holds the entries at 0s and at 60s and the 59 between them, so 61.
	It("cuts the capacity rule at exactly the entries a span holds, 61 over a 60s span at a 1s interval and not 60, for an instrument and a measurement alike", func() {
		for _, count := range []int{61, 62} {
			reduction, rerr := NewReduction("boundary", count, func([]Point) float64 { return 0 })
			Expect(rerr).ToNot(HaveOccurred())

			sig := validSignal("A")
			sig.Instruments[0].Reduction = reduction // I1 spans 60s
			byTrack := validTable([]Signal[snap]{validSignal("A")})
			byTrack.Measurements = []Measurement[snap]{{Name: "T", Extract: extract, Span: 60 * time.Second, Reduction: reduction}}

			for _, row := range []struct {
				name string
				tbl  Table[snap]
			}{
				{name: "instrument", tbl: validTable([]Signal[snap]{sig})},
				{name: "measurement", tbl: byTrack},
			} {
				_, err := NewEngine(row.tbl)
				if count <= 61 {
					Expect(err).ToNot(HaveOccurred(), "%s: a minimum of %d is within the 61 entries a 60s span holds at a 1s interval", row.name, count)

					continue
				}
				Expect(err).To(HaveOccurred(), "%s: a minimum of %d is one past the 61 entries a 60s span holds at a 1s interval", row.name, count)
				Expect(err.Error()).To(ContainSubstring("exceeds"), row.name)
			}
		}
	})

	It("refuses an instrument that names a dividing reduction with a nil Against, because its window can never hold a point", func() {
		sig := validSignal("A")
		sig.Instruments[0].Reduction = DeltaRatio
		sig.Instruments[0].Against = nil
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses a signal whose instrument has a nil Extract, which would panic in the observe loop", func() {
		sig := validSignal("A")
		sig.Instruments[0].Extract = nil
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses duplicate signal names, which would share one latch and corrupt the fired set", func() {
		tbl := validTable([]Signal[snap]{validSignal("A"), validSignal("A")})
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
	})

	It("refuses duplicate instrument names within one signal, which would share one window", func() {
		sig := validSignal("A")
		sig.Instruments = append(sig.Instruments, sig.Instruments[0])
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses a measurement whose span is zero or negative", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Measurements = []Measurement[snap]{{Name: "T", Extract: extract, Span: 0, Reduction: Mean}}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("T"))
		Expect(err.Error()).To(ContainSubstring("span"))
		Expect(err.Error()).ToNot(ContainSubstring("window span"),
			"a table measurement hangs under no signal, so there is no signal's window to name and the error says the shorter word")
	})

	It("refuses a measurement whose reduction minimum sample count is below one", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Measurements = []Measurement[snap]{{Name: "T", Extract: extract, Span: 60 * time.Second, Reduction: Reduction{Name: "low", Min: 0, fold: func([]Point) float64 { return 0 }}}}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("T"))
	})

	It("refuses a measurement that names a dividing reduction, because a measurement declares no denominator series", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Measurements = []Measurement[snap]{{Name: "T", Extract: extract, Span: 60 * time.Second, Reduction: DeltaRatio}}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("T"))
	})

	It("refuses a measurement whose Extract is nil, which would panic in the observe loop", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Measurements = []Measurement[snap]{{Name: "T", Span: 60 * time.Second, Reduction: Mean}}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("T"))
	})

	It("accepts an instrument that declares an against extractor under a non-dividing reduction, because the declaration is redundant rather than invalid", func() {
		sig := validSignal("A")
		sig.Instruments[0].Against = against
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).ToNot(HaveOccurred(), "a non-nil against under a non-dividing reduction is a redundant declaration, not a refusal")
	})

	It("refuses duplicate measurement names, which would leave Measurement returning whichever one it reached first", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Measurements = []Measurement[snap]{
			{Name: "T", Extract: extract, Span: 60 * time.Second, Reduction: Mean},
			{Name: "T", Extract: extract, Span: 60 * time.Second, Reduction: Mean},
		}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("T"))
	})

	It("refuses a signal whose demote span is zero or negative", func() {
		sig := validSignal("A")
		sig.DemoteSpan = 0
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
	})

	It("refuses a signal whose demote span is below the table interval, which the suite generator would turn into a zero tick count", func() {
		sig := validSignal("A")
		sig.DemoteSpan = 500 * time.Millisecond
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
	})

	It("refuses a signal with no instruments, which resolve would report as NoInstrument forever", func() {
		sig := validSignal("A")
		sig.Instruments = nil
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
	})

	It("refuses a LowerIsWorse mark pair whose worst value is on the better side of its fire mark", func() {
		sig := validSignal("A")
		m := validMarks()
		m.Polarity = LowerIsWorse
		m.Fire = Mark{At: 8, Inclusive: true}
		m.Clear = Mark{At: 12, Inclusive: true}
		m.Worst = 10 // falling, so worse() negates: worse(10) = -10 sits below worse(8) = -8, making 10 the better side of fire
		sig.Instruments[0].Marks = m
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses a HigherIsWorse mark whose worst value is at or below the fire mark, leaving no room between the two", func() {
		sig := validSignal("A")
		m := validMarks()
		m.Worst = m.Fire.At // worst == fire: zero denominator, severity collapses
		sig.Instruments[0].Marks = m
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses a LowerIsWorse mark whose worst value equals its fire mark, zeroing the severity denominator", func() {
		sig := validSignal("A")
		m := validMarks()
		m.Polarity = LowerIsWorse
		m.Fire = Mark{At: 8, Inclusive: true}
		m.Clear = Mark{At: 12, Inclusive: true}
		m.Worst = m.Fire.At // worse(8) = -8 on both sides: the denominator is zero under either polarity
		sig.Instruments[0].Marks = m
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses a rising pair that leaves its worst value unset, because zero is on the better side of a positive fire mark", func() {
		sig := validSignal("A")
		m := validMarks()
		m.Worst = 0 // the zero value of the field: worse(0) = 0 is below worse(2.0) = 2.0
		sig.Instruments[0].Marks = m
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred(), "there is no unset case: a caller who declares no worst value gets a refusal, not an unnormalised severity")
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	// Finiteness is a refusal of its own because the ordering checks cannot stand in
	// for it: every comparison against NaN is false, so a NaN fire, clear or worst
	// slips both of them, and an infinite worst overflows the severity denominator.
	// A table is built from runtime numbers, so no caller has to write the literal.
	It("refuses an instrument whose fire, clear or worst mark is not finite", func() {
		for _, tc := range []struct {
			mutate func(*Marks)
			name   string
		}{
			{name: "fire NaN", mutate: func(m *Marks) { m.Fire.At = math.NaN() }},
			{name: "fire +Inf", mutate: func(m *Marks) { m.Fire.At = math.Inf(1) }},
			{name: "clear NaN", mutate: func(m *Marks) { m.Clear.At = math.NaN() }},
			{name: "clear -Inf", mutate: func(m *Marks) { m.Clear.At = math.Inf(-1) }},
			{name: "worst NaN", mutate: func(m *Marks) { m.Worst = math.NaN() }},
			{name: "worst +Inf", mutate: func(m *Marks) { m.Worst = math.Inf(1) }},
		} {
			sig := validSignal("A")
			m := validMarks()
			tc.mutate(&m)
			sig.Instruments[0].Marks = m
			_, err := NewEngine(validTable([]Signal[snap]{sig}))
			Expect(err).To(HaveOccurred(), tc.name)
			Expect(err.Error()).To(ContainSubstring("A"), tc.name)
			Expect(err.Error()).To(ContainSubstring("I1"), tc.name)
			Expect(err.Error()).To(ContainSubstring("is not finite"), tc.name)
		}
	})

	// Two finite marks, correctly ordered, can still sit too far apart to subtract:
	// 1e308 - (-1e308) overflows to +Inf, which divides every numerator to 0. The
	// signal fires as usual and ranks dead last however bad the reading is, worst
	// value included, so the refusal cannot be left to the finiteness check above.
	It("refuses an instrument whose fire and worst marks are finite but too far apart to subtract", func() {
		for _, tc := range []struct {
			name  string
			marks Marks
		}{
			{
				name:  "rising",
				marks: Marks{Unit: "ratio", Fire: Mark{At: -1e308, Inclusive: true}, Clear: Mark{At: -1.5e308, Inclusive: true}, Polarity: HigherIsWorse, Worst: 1e308},
			},
			{
				name:  "falling",
				marks: Marks{Unit: "cores", Fire: Mark{At: 1e308, Inclusive: true}, Clear: Mark{At: 1.5e308, Inclusive: true}, Polarity: LowerIsWorse, Worst: -1e308},
			},
		} {
			sig := validSignal("A")
			sig.Instruments[0].Marks = tc.marks
			_, err := NewEngine(validTable([]Signal[snap]{sig}))
			Expect(err).To(HaveOccurred(), tc.name)
			Expect(err.Error()).To(ContainSubstring("A"), tc.name)
			Expect(err.Error()).To(ContainSubstring("I1"), tc.name)
			Expect(err.Error()).To(ContainSubstring("overflows"), tc.name)
		}
	})

	// Severity is anchored on the two marks validate compares, so for any pair it
	// accepts the endpoints are exact, whatever the unit or direction. Rising
	// ratio: (4.0 - 2.0) / (4.0 - 2.0) = 1. Falling headroom, worse() negating
	// every term: (1.0 - 0.0) / (1.0 - 0.0) = 1 at the value -1.0 against fire 0.0.
	// Under the earlier design the falling denominator was fire - (-worst), which
	// put severity 1 at a value the quantity could not reach and scored a signal
	// at its worst near zero.
	It("scores exactly 0.0 at the fire mark and exactly 1.0 at the worst value, under both polarities", func() {
		rising := Marks{Unit: "ratio", Fire: Mark{At: 2.0, Inclusive: true}, Clear: Mark{At: 1.0, Inclusive: true}, Polarity: HigherIsWorse, Worst: 4.0}
		falling := Marks{Unit: "cores", Fire: Mark{At: 0.0, Inclusive: true}, Clear: Mark{At: 0.5, Inclusive: true}, Polarity: LowerIsWorse, Worst: -1.0}

		for _, m := range []Marks{rising, falling} {
			sig := validSignal("A")
			sig.Instruments = sig.Instruments[:1] // the pair under test is the table's only one
			sig.Instruments[0].Marks = m
			_, err := NewEngine(validTable([]Signal[snap]{sig}))
			Expect(err).ToNot(HaveOccurred(), m.Unit)

			Expect(Fired{Marks: m, Value: m.Worst}.Severity()).To(Equal(1.0), m.Unit)
			Expect(Fired{Marks: m, Value: m.Fire.At}.Severity()).To(Equal(0.0), m.Unit)
		}
	})

	It("rejects a refinement under its parent's path, so the error names A/A1 not the bare child", func() {
		ref := validSignal("A1")
		ref.DemoteSpan = 0
		parent := validSignal("A")
		parent.Refinements = []Signal[snap]{ref}
		_, err := NewEngine(validTable([]Signal[snap]{parent}))
		if err == nil {
			Fail("expected NewEngine to reject a refinement whose demote span is zero")
		}
		Expect(err.Error()).To(ContainSubstring("A/A1"),
			"the error names the parent and the child, not the bare child")
	})

	It("refuses two sibling refinements with the same name under one parent", func() {
		parent := validSignal("A")
		parent.Refinements = []Signal[snap]{validSignal("A1"), validSignal("A1")}
		_, err := NewEngine(validTable([]Signal[snap]{parent}))
		if err == nil {
			Fail("expected NewEngine to reject duplicate sibling refinement names")
		}
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("A1"))
	})

	It("accepts the same refinement name under different parents, because uniqueness is among siblings only", func() {
		a := validSignal("A")
		a.Refinements = []Signal[snap]{validSignal("X")}
		b := validSignal("B")
		b.Refinements = []Signal[snap]{validSignal("X")}
		_, err := NewEngine(validTable([]Signal[snap]{a, b}))
		Expect(err).ToNot(HaveOccurred(),
			"a refinement name is unique among its parent's refinements, not across the tree")
	})

	// A refinement's windows are keyed under its parent's path plus its own
	// name, joined with "/", so a top-level signal named "A/X" is keyed under
	// the same "A/X" as the refinement X of a signal A. The second one built
	// overwrites the first, and from then on both signals judge whichever window
	// survived. Refusing "/" inside any name makes that collision unreachable:
	// no segment can hold the separator, so no composed path can equal a bare
	// name.
	It("refuses a top-level signal whose name holds the path separator", func() {
		_, err := NewEngine(validTable([]Signal[snap]{validSignal("A/X")}))
		if err == nil {
			Fail("expected NewEngine to reject a top-level signal named A/X")
		}
		Expect(err.Error()).To(ContainSubstring("A/X"))
		Expect(err.Error()).To(ContainSubstring(`may not contain "/"`))
	})

	It("refuses a refinement whose name holds the path separator, so the rule reaches below the top level", func() {
		parent := validSignal("A")
		parent.Refinements = []Signal[snap]{validSignal("B/C")}
		_, err := NewEngine(validTable([]Signal[snap]{parent}))
		if err == nil {
			Fail("expected NewEngine to reject a refinement named B/C")
		}
		Expect(err.Error()).To(ContainSubstring("B/C"))
		Expect(err.Error()).To(ContainSubstring(`may not contain "/"`))
	})

	// Two parents each declaring a refinement called X is the case paths exist
	// for, and the refusal above must not reach it. Reading both reduced values
	// is what pins that down: a rule that rejected this table, or one that let
	// the two share a window, would leave the two refusals above green while the
	// feature was broken. Each X reads the snapshot through its own extractor,
	// so one shared window could not report both numbers.
	It("keeps the windows of a refinement named X under A and one named X under B separate", func() {
		refinementReading := func(read func(snap) float64) Signal[snap] {
			r := validSignal("X")
			r.Instruments[0].Extract = func(s snap) Reading { return Known(read(s)) }

			return r
		}

		a := validSignal("A")
		a.Refinements = []Signal[snap]{refinementReading(func(s snap) float64 { return s.v })}
		b := validSignal("B")
		b.Refinements = []Signal[snap]{refinementReading(func(s snap) float64 { return s.v * 3 })}

		e, err := NewEngine(validTable([]Signal[snap]{a, b}))
		Expect(err).ToNot(HaveOccurred(),
			"two parents may each declare a refinement called X: the paths A/X and B/X keep them apart")

		e.Observe(snap{v: 2}, NewEnvironment("source-1"), time.Unix(1_000_000, 0))

		underA, stateA := e.Reduction("A/X", "I1").Get()
		Expect(stateA).To(Equal(StateValue))
		Expect(underA).To(Equal(2.0))

		underB, stateB := e.Reduction("B/X", "I1").Get()
		Expect(stateB).To(Equal(StateValue))
		Expect(underB).To(Equal(6.0), "B/X reads three times what A/X does, so one shared window could not report both")
	})
})
