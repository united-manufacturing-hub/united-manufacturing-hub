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

package fsmv2cpu

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("the admission-window decision", func() {
	// admissionDecision reads only its arguments, so nothing here builds a
	// worker, a sampler or a clock. Each spec names the condition a worker
	// would be in and asserts what it then does.
	//
	// window is this file's own width, not the shipped constant: the specs that
	// vary it prove the function reads the argument, and the last Describe
	// binds the behaviour back to the constant the worker actually ships.
	const window = 10 * time.Second

	// The evidence counts, named so a reader does not have to decode a pair of
	// integers at each call. A shortfall is one capable signal that has never
	// produced a reading — the condition the refusal exists for.
	const (
		neverMeasured, oneCapable = 0, 1
		allMeasured               = 1
	)

	Describe("a box whose capable signal has never measured", func() {
		It("still refuses nine seconds in, and has not reached the deadline", func() {
			refusing, atDeadline := admissionDecision(9*time.Second, window, neverMeasured, oneCapable)

			Expect(refusing).To(BeTrue(),
				"nine seconds is inside a ten-second window and nothing has measured, so the worker refuses")
			Expect(atDeadline).To(BeFalse(),
				"nine seconds has not reached a ten-second deadline")
		})

		It("still refuses at the last instant inside the window", func() {
			refusing, atDeadline := admissionDecision(window-time.Nanosecond, window, neverMeasured, oneCapable)

			Expect(refusing).To(BeTrue(), "one nanosecond short of the window is still inside it")
			Expect(atDeadline).To(BeFalse())
		})

		It("admits at exactly the window, and calls that same instant the deadline", func() {
			// The boundary is closed on the deadline side and open on the
			// refusal side, so this single tick decides both directions: a
			// worker that waited for elapsed to exceed the window would still
			// be refusing here, and one that refused up to and including the
			// window would not have opened yet.
			refusing, atDeadline := admissionDecision(window, window, neverMeasured, oneCapable)

			Expect(refusing).To(BeFalse(),
				"admission opens the instant the window closes, with the counts unchanged")
			Expect(atDeadline).To(BeTrue(),
				"that same instant is the deadline the warning fires on")
		})

		It("stays admitted after the window has closed", func() {
			refusing, atDeadline := admissionDecision(11*time.Second, window, neverMeasured, oneCapable)

			Expect(refusing).To(BeFalse(), "the refusal does not come back once the window has closed")
			Expect(atDeadline).To(BeTrue())
		})

		It("treats a sample timestamp that stepped backwards as still inside the window", func() {
			// A negative delta cannot arise in production, where the timestamps
			// are monotonic, but a synthetic clock can step backwards. It reads
			// as inside the window exactly like zero does, so a backward step
			// prolongs the refusal rather than opening admission early.
			refusing, atDeadline := admissionDecision(-3*time.Second, window, neverMeasured, oneCapable)

			Expect(refusing).To(BeTrue(), "a negative elapsed is inside the window, like the anchor tick itself")
			Expect(atDeadline).To(BeFalse())
		})
	})

	Describe("a box with no shortfall to refuse on", func() {
		It("admits from the first instant once every capable signal has measured", func() {
			refusing, _ := admissionDecision(0, window, allMeasured, oneCapable)

			Expect(refusing).To(BeFalse(),
				"a box whose only capable signal has measured is admitted at once, without waiting out the window")
		})

		It("admits nine seconds in once every capable signal has measured", func() {
			// The same position in the window that refuses above. The counts are
			// the only thing that differs, so this pair is what separates the
			// two terms of the decision.
			refusing, _ := admissionDecision(9*time.Second, window, allMeasured, oneCapable)

			Expect(refusing).To(BeFalse(), "deep inside the window, a measured box is still admitted")
		})

		It("admits a box no instrument can answer, at any point in the window", func() {
			// Nothing capable means nothing to be missing. A worker that refused
			// here would block a box that can never satisfy it.
			for _, elapsed := range []time.Duration{0, 9 * time.Second, window, 11 * time.Second} {
				refusing, _ := admissionDecision(elapsed, window, 0, 0)

				Expect(refusing).To(BeFalse(),
					"no capable signal means no shortfall, at elapsed %s", elapsed)
			}
		})

		It("admits when more signals measured than are capable", func() {
			refusing, _ := admissionDecision(0, window, 2, oneCapable)

			Expect(refusing).To(BeFalse(),
				"a count above capable is a surplus, never a shortfall")
		})

		It("says nothing at the deadline when there was no shortfall to report", func() {
			// The deadline is what makes the worker give up waiting and warn.
			// Reaching it is not on its own a reason to warn: a box that
			// measured everything, and a box with nothing to measure, both
			// reach it with nothing wrong. Only the box still missing a reading
			// is worth an operator's attention.
			_, measuredBox := admissionDecision(window, window, allMeasured, oneCapable)
			_, emptyBox := admissionDecision(window, window, 0, 0)
			_, shortBox := admissionDecision(window, window, neverMeasured, oneCapable)

			Expect(measuredBox).To(BeFalse(), "a fully measured box reaches the deadline with nothing to say")
			Expect(emptyBox).To(BeFalse(), "nor does a box no instrument can answer")
			Expect(shortBox).To(BeTrue(),
				"the box still missing a reading at the deadline is the one that gets reported")
		})
	})

	Describe("the width it is given", func() {
		It("moves the boundary to the window in its arguments, not to the shipped one", func() {
			// Handed five seconds, the decision must turn at five. If it read
			// admissionWindow instead of its parameter, nine seconds would still
			// refuse here and the deadline would not have been reached.
			const short = 5 * time.Second

			refusingBefore, atDeadlineBefore := admissionDecision(4*time.Second, short, neverMeasured, oneCapable)
			Expect(refusingBefore).To(BeTrue(), "four seconds is inside a five-second window")
			Expect(atDeadlineBefore).To(BeFalse())

			refusingAfter, atDeadlineAfter := admissionDecision(short, short, neverMeasured, oneCapable)
			Expect(refusingAfter).To(BeFalse(), "five seconds closes a five-second window")
			Expect(atDeadlineAfter).To(BeTrue())

			refusingLater, atDeadlineLater := admissionDecision(9*time.Second, short, neverMeasured, oneCapable)
			Expect(refusingLater).To(BeFalse(),
				"nine seconds is past a five-second window, however long the shipped one is")
			Expect(atDeadlineLater).To(BeTrue())
		})
	})

	Describe("the two answers taken together", func() {
		It("handles every shortfall exactly once — refused, or reported, never both and never neither", func() {
			// Both true at once would let the worker refuse admission and raise
			// its give-up warning on the same tick. Both false while a signal is
			// still missing would drop the shortfall on the floor: the box would
			// be admitted with a blind spot nobody was ever told about.
			var sawRefusing, sawAtDeadline int

			for _, elapsed := range []time.Duration{-time.Second, 0, time.Second, window - time.Nanosecond, window, window + time.Second} {
				for _, counts := range [][2]int{{0, 0}, {0, 1}, {1, 1}, {1, 2}, {2, 2}} {
					measured, capable := counts[0], counts[1]
					refusing, atDeadline := admissionDecision(elapsed, window, measured, capable)

					Expect(refusing && atDeadline).To(BeFalse(),
						"refused AND reported at elapsed %s with measured %d of %d capable",
						elapsed, measured, capable)
					Expect(refusing || atDeadline).To(Equal(measured < capable),
						"a shortfall is refused or reported, and a box without one is neither, at elapsed %s with measured %d of %d capable",
						elapsed, measured, capable)

					if refusing {
						sawRefusing++
					}
					if atDeadline {
						sawAtDeadline++
					}
				}
			}

			// Without these the loop above would pass on a decision that never
			// returned true at all.
			Expect(sawRefusing).To(BeNumerically(">", 0), "the grid reaches the refusing case")
			Expect(sawAtDeadline).To(BeNumerically(">", 0), "the grid reaches the reported case")
		})
	})

	Describe("the window the worker ships", func() {
		It("refuses a never-measured box at nine seconds and admits it at ten", func() {
			// The specs above choose their own width, so they would all survive
			// a change to admissionWindow. This one would not: it names the two
			// seconds either side of ten and hands the decision the constant the
			// worker actually polls with.
			refusingAtNine, atDeadlineAtNine := admissionDecision(9*time.Second, admissionWindow, neverMeasured, oneCapable)
			Expect(refusingAtNine).To(BeTrue(), "the shipped window is still open at nine seconds")
			Expect(atDeadlineAtNine).To(BeFalse())

			refusingAtTen, atDeadlineAtTen := admissionDecision(10*time.Second, admissionWindow, neverMeasured, oneCapable)
			Expect(refusingAtTen).To(BeFalse(), "the shipped window has closed by ten seconds")
			Expect(atDeadlineAtTen).To(BeTrue())
		})
	})
})
