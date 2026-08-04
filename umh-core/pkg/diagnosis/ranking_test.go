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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Rank is the only producer of the total order over causes; Severity is the
// normaliser that lets a falling mark in cores compare against a rising one in
// a ratio. Identity.Tier is the rank class a latch cannot learn from a
// reduction, the engine stamps it at construction, and lower tiers outrank
// higher ones regardless of severity. Tests use bare tier numbers, because the
// vocabulary (which tier means what) lives with the caller, not here.
var _ = Describe("Ranking", func() {

	// indexes pulls the declared table positions out of a ranked set, so a
	// sequence is compared by the fact that decides it rather than by object
	// identity.
	indexes := func(fired []Fired) []int {
		out := make([]int, len(fired))
		for i, f := range fired {
			out[i] = f.Index
		}
		return out
	}

	// rises builds a HigherIsWorse ratio cause whose severity is
	// (value-fire)/(capacity-fire), clamped to [0,1].
	rises := func(tier, index int, value, fire, capacity float64) Fired {
		return Fired{
			Identity: Identity{Signal: "r", Tier: tier, Index: index},
			Value:    value,
			Marks: Marks{
				Fire:     Mark{At: fire},
				Polarity: HigherIsWorse,
				Capacity: capacity,
			},
		}
	}

	// falls builds a LowerIsWorse headroom cause whose severity is
	// (fire-value)/(fire-(-capacity)), clamped to [0,1].
	falls := func(tier, index int, value, fire, capacity float64, external bool) Fired {
		return Fired{
			Identity: Identity{Signal: "f", Tier: tier, Index: index, External: external},
			Value:    value,
			Marks: Marks{
				Fire:     Mark{At: fire},
				Polarity: LowerIsWorse,
				Capacity: capacity,
			},
		}
	}

	It("should rank a lower-tier cause above a higher-tier one regardless of severity", func() {
		// Tier 0 cause with severity 0 sorts above a Tier 1 cause with severity 1.0.
		top := rises(0, 0, 0.5, 0.5, 1.0) // severity (0.5-0.5)/0.5 = 0
		bot := rises(1, 1, 1.0, 0.5, 1.0) // severity (1.0-0.5)/0.5 = 1.0
		Expect(Rank([]Fired{bot, top})).To(HaveLen(2))
		Expect(indexes(Rank([]Fired{bot, top}))).To(Equal([]int{0, 1}))
	})

	It("should rank within a tier by severity normalised against the instrument's own marks and capacity", func() {
		// Two Tier 1 causes whose severity DISAGREES with their declared position:
		// the higher-severity cause has the HIGHER Index, so only a comparator that
		// actually reads severity puts it first. A rank missing the severity level
		// would order by Index and get this wrong.
		worse := rises(1, 3, 0.8, 0.5, 1.0) // severity (0.8-0.5)/0.5 = 0.6, Index 3
		less := rises(1, 0, 0.6, 0.5, 1.0)  // severity (0.6-0.5)/0.5 = 0.2, Index 0
		sorted := Rank([]Fired{less, worse})
		Expect(indexes(sorted)).To(Equal([]int{3, 0}),
			"the higher-severity cause ranks first within the tier, despite its higher declared position")
	})

	It("should give a falling mark the same severity scale as a rising one, so an absolute quantity and a ratio compare", func() {
		// Worked case: a 4-core box two cores past its headroom mark.
		falling := falls(1, 0, -2, 0, 4, false) // (0-(-2))/(0-(-4)) = 0.5
		Expect(falling.Severity()).To(Equal(0.5),
			"the falling arm uses MINUS capacity; +capacity gives 2/-4, clamped to 0")

		// A rising ratio at the same normalised severity compares on the same scale.
		rising := rises(1, 1, 0.75, 0.5, 1.0) // (0.75-0.5)/0.5 = 0.5
		Expect(rising.Severity()).To(Equal(falling.Severity()),
			"an absolute quantity and a ratio with the same normalised severity share a scale")
	})

	It("should break a remaining tie in favour of the externally-attributed cause", func() {
		internal := rises(1, 0, 0.5, 0.5, 1.0) // severity 0
		external := rises(1, 1, 0.5, 0.5, 1.0) // severity 0
		external.External = true
		sorted := Rank([]Fired{internal, external})
		Expect(indexes(sorted)).To(Equal([]int{1, 0}))
	})

	It("should break a last tie by the signal's declared position in the table", func() {
		a := rises(1, 0, 0.5, 0.5, 1.0) // severity 0, not external
		b := rises(1, 3, 0.5, 0.5, 1.0) // severity 0, not external, lower index wins
		sorted := Rank([]Fired{b, a})
		Expect(indexes(sorted)).To(Equal([]int{0, 3}))
	})

	It("should return a total order, so ranking the same set twice in different append orders gives the same sequence", func() {
		// A set deliberately chosen to cluster on tier, severity and externality
		// so that only the total order (all four keys) settles it.
		set := []Fired{
			rises(2, 0, 0.7, 0.5, 1.0), // tier 2, severity 0.4
			rises(0, 1, 0.5, 0.5, 1.0), // tier 0, severity 0
			falls(1, 2, -2, 0, 4, false),
			falls(1, 3, -2, 0, 4, true), // same tier+severity, external
			rises(1, 4, 0.5, 0.5, 1.0),  // same tier, severity 0, not external
		}
		want := indexes(Rank(append([]Fired{}, set...)))
		shuffled := []Fired{set[4], set[1], set[3], set[0], set[2]}
		Expect(indexes(Rank(shuffled))).To(Equal(want),
			"append order must not matter: the order is total, not stable-by-append")
	})

	It("should keep a degenerate mark with no headroom finite and total-ordered", func() {
		// capacity == fire makes the rising denominator zero; a value sitting at
		// the fire mark then gives 0/0 = NaN, which must fall back to the lowest
		// severity so Rank stays a strict total order.
		degenerate := rises(1, 0, 0.5, 0.5, 0.5) // capacity == fire == value
		s := degenerate.Severity()
		Expect(math.IsNaN(s)).To(BeFalse(), "a degenerate mark must not leak NaN into the comparator")
		Expect(s).To(BeNumerically(">=", 0))
		Expect(s).To(BeNumerically("<=", 1))

		set := []Fired{degenerate, rises(1, 1, 0.7, 0.5, 1.0)}
		want := indexes(Rank(append([]Fired{}, set...)))
		Expect(indexes(Rank([]Fired{set[1], set[0]}))).To(Equal(want),
			"the degenerate cause is still ranked deterministically")
	})

	It("should keep a degenerate falling mark with capacity equal to minus the fire mark finite and total-ordered", func() {
		// capacity == -fire zeroes the falling denominator (fire - (-capacity)).
		// A value below the fire mark then gives a non-zero / 0 = +Inf, which must
		// clamp to the top of the scale; a value sitting exactly at the fire mark
		// gives 0/0 = NaN, which must fall back to the bottom, the rising-arm
		// degenerate handling, on the other polarity. Rank stays a total order.
		below := falls(1, 0, 4, 8, -8, false) // (8-4)/(8-8) = 4/0 = +Inf
		at := falls(1, 1, 8, 8, -8, false)    // (8-8)/(8-8) = 0/0 = NaN
		for _, f := range []Fired{below, at} {
			s := f.Severity()
			Expect(math.IsNaN(s)).To(BeFalse(), "a degenerate falling mark must not leak NaN into the comparator")
			Expect(math.IsInf(s, 0)).To(BeFalse(), "a degenerate falling mark must not leak ±Inf into the comparator")
			Expect(s).To(BeNumerically(">=", 0))
			Expect(s).To(BeNumerically("<=", 1))
		}
		Expect(below.Severity()).To(Equal(1.0), "the overshooting arm clamps to the top of the severity scale")
		Expect(at.Severity()).To(Equal(0.0), "the 0/0 arm falls back to the bottom of the severity scale")

		set := []Fired{below, falls(1, 2, -2, 0, 4, false)}
		want := indexes(Rank(append([]Fired{}, set...)))
		Expect(indexes(Rank([]Fired{set[1], set[0]}))).To(Equal(want),
			"the degenerate falling cause is still ranked deterministically")
	})

	It("should clamp severity to the [0,1] scale when the value overshoots or undershoots", func() {
		above := rises(1, 0, 2.0, 0.5, 1.0)   // (2.0-0.5)/0.5 = 3.0 -> clamped 1
		beneath := rises(1, 1, 0.0, 0.5, 1.0) // (0.0-0.5)/0.5 = -1.0 -> clamped 0
		Expect(above.Severity()).To(Equal(1.0))
		Expect(beneath.Severity()).To(Equal(0.0))
	})

	It("should order two falling causes within a tier by their severity at a non-zero fire mark", func() {
		// A non-zero fire mark exercises the falling denominator

		// (fire - (-capacity)), which degenerates to just capacity when fire==0.
		// The two causes carry different severities (0.333 vs 0.111), so it is the
		// severity level, not the Index tie-break, that decides which ranks first
		// within the tier.
		worse := falls(1, 0, 2, 5, 4, false) // (5-2)/(5-(-4)) = 3/9 -> 0.333
		less := falls(1, 1, 4, 5, 4, false)  // (5-4)/(5-(-4)) = 1/9 -> 0.111
		sorted := Rank([]Fired{less, worse})
		Expect(indexes(sorted)).To(Equal([]int{0, 1}),
			"the higher-severity falling cause ranks first within the tier")
	})
})
