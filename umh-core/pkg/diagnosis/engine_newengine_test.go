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

// NewEngine is the single choke point that refuses a malformed table. Each spec
// builds a table that is valid except for exactly one defect and asserts that
// construction fails, naming the row that is malformed — so a bad state cannot
// be built, and a caller finds out once, at construction, whether the whole
// table is buildable.
var _ = Describe("NewEngine", func() {
	type snap struct{ v float64 }
	extract := func(s snap) Reading { return Known(s.v) }
	against := func(s snap) Reading { return Known(s.v) }

	validMarks := func() Marks {
		return Marks{Unit: "ratio", Fire: Mark{At: 2.0, Inclusive: true}, Clear: Mark{At: 1.0, Inclusive: true}, Polarity: HigherIsWorse}
	}

	validSignal := func(name string) Signal[snap] {
		return Signal[snap]{
			Name:       name,
			DemoteSpan: 60 * time.Second,
			Instruments: []Instrument[snap]{
				{
					Name: "I1", Requires: []Capability{"source-1"}, Extract: extract, Red: Last, Span: 60 * time.Second, Marks: validMarks(),
				},
				{
					Name: "I2", Requires: []Capability{"source-1"}, Extract: extract, Red: Mean, Span: 3 * time.Second,
					Marks: Marks{Unit: "cores", Fire: Mark{At: 8, Inclusive: true}, Clear: Mark{At: 4, Inclusive: true}, Polarity: HigherIsWorse},
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
		sig.Instruments[0].Red = P95
		sig.Instruments[0].Boolean = true
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses an instrument whose reduction minimum sample count is below one", func() {
		sig := validSignal("A")
		sig.Instruments[0].Red = Reduction{Name: "low", Min: 0, fold: func([]Point) float64 { return 0 }}
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("A"))
		Expect(err.Error()).To(ContainSubstring("I1"))
	})

	It("refuses an instrument that names a dividing reduction with a nil Against, because its window can never hold a point", func() {
		sig := validSignal("A")
		sig.Instruments[0].Red = DeltaRatio
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

	It("refuses a track whose span is zero or negative", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Tracks = []Track[snap]{{Name: "T", Extract: extract, Span: 0, Red: Mean}}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("T"))
	})

	It("refuses a track whose reduction minimum sample count is below one", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Tracks = []Track[snap]{{Name: "T", Extract: extract, Span: 60 * time.Second, Red: Reduction{Name: "low", Min: 0, fold: func([]Point) float64 { return 0 }}}}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("T"))
	})

	It("refuses a track that names a dividing reduction, because a track declares no denominator series", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Tracks = []Track[snap]{{Name: "T", Extract: extract, Span: 60 * time.Second, Red: DeltaRatio}}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("T"))
	})

	It("refuses a track whose Extract is nil, which would panic in the observe loop", func() {
		tbl := validTable([]Signal[snap]{validSignal("A")})
		tbl.Tracks = []Track[snap]{{Name: "T", Span: 60 * time.Second, Red: Mean}}
		_, err := NewEngine(tbl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("T"))
	})

	It("refuses a valid instrument whose mark pair carries a valid but redundant non-dividing against extractor — and does not refuse it", func() {
		sig := validSignal("A")
		sig.Instruments[0].Against = against
		_, err := NewEngine(validTable([]Signal[snap]{sig}))
		Expect(err).ToNot(HaveOccurred(), "a non-nil against under a non-dividing reduction is a redundant declaration, not a refusal")
	})
})
