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
			refusing, overDeadline := admissionDecision(9*time.Second, window, neverMeasured, oneCapable)

			Expect(refusing).To(BeTrue(),
				"nine seconds is inside a ten-second window and nothing has measured, so the worker refuses")
			Expect(overDeadline).To(BeFalse(),
				"nine seconds has not reached a ten-second deadline")
		})

		It("still refuses at the last instant inside the window", func() {
			refusing, overDeadline := admissionDecision(window-time.Nanosecond, window, neverMeasured, oneCapable)

			Expect(refusing).To(BeTrue(), "one nanosecond short of the window is still inside it")
			Expect(overDeadline).To(BeFalse())
		})

		It("admits at exactly the window, and calls that same instant the deadline", func() {
			// The boundary is closed on the deadline side and open on the
			// refusal side, so this single tick decides both directions: a
			// worker that waited for elapsed to exceed the window would still
			// be refusing here, and one that refused up to and including the
			// window would not have opened yet.
			refusing, overDeadline := admissionDecision(window, window, neverMeasured, oneCapable)

			Expect(refusing).To(BeFalse(),
				"admission opens the instant the window closes, with the counts unchanged")
			Expect(overDeadline).To(BeTrue(),
				"that same instant is the deadline the warning fires on")
		})

		It("stays admitted after the window has closed", func() {
			refusing, overDeadline := admissionDecision(11*time.Second, window, neverMeasured, oneCapable)

			Expect(refusing).To(BeFalse(), "the refusal does not come back once the window has closed")
			Expect(overDeadline).To(BeTrue())
		})

		It("treats a sample timestamp that stepped backwards as still inside the window", func() {
			// A negative delta cannot arise in production, where the timestamps
			// are monotonic, but a synthetic clock can step backwards. It reads
			// as inside the window exactly like zero does, so a backward step
			// prolongs the refusal rather than opening admission early.
			refusing, overDeadline := admissionDecision(-3*time.Second, window, neverMeasured, oneCapable)

			Expect(refusing).To(BeTrue(), "a negative elapsed is inside the window, like the anchor tick itself")
			Expect(overDeadline).To(BeFalse())
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

		It("reports the deadline reached whatever the counts say", func() {
			// overDeadline is a fact about time alone. The worker uses it
			// together with the counts, so a version that folded the counts in
			// here would answer the caller's question twice and hide the
			// difference between 'the window closed' and 'something is missing'.
			_, measuredBox := admissionDecision(window, window, allMeasured, oneCapable)
			_, emptyBox := admissionDecision(window, window, 0, 0)

			Expect(measuredBox).To(BeTrue(), "the window closes on a fully measured box too")
			Expect(emptyBox).To(BeTrue(), "and on a box with nothing capable")
		})
	})

	Describe("the width it is given", func() {
		It("moves the boundary to the window in its arguments, not to the shipped one", func() {
			// Handed five seconds, the decision must turn at five. If it read
			// admissionWindow instead of its parameter, nine seconds would still
			// refuse here and the deadline would not have been reached.
			const short = 5 * time.Second

			refusingBefore, deadlineBefore := admissionDecision(4*time.Second, short, neverMeasured, oneCapable)
			Expect(refusingBefore).To(BeTrue(), "four seconds is inside a five-second window")
			Expect(deadlineBefore).To(BeFalse())

			refusingAfter, deadlineAfter := admissionDecision(short, short, neverMeasured, oneCapable)
			Expect(refusingAfter).To(BeFalse(), "five seconds closes a five-second window")
			Expect(deadlineAfter).To(BeTrue())

			refusingLater, deadlineLater := admissionDecision(9*time.Second, short, neverMeasured, oneCapable)
			Expect(refusingLater).To(BeFalse(),
				"nine seconds is past a five-second window, however long the shipped one is")
			Expect(deadlineLater).To(BeTrue())
		})
	})

	Describe("the two answers taken together", func() {
		It("never refuses and reports the deadline reached on the same tick", func() {
			// Refusing means the window is open; the deadline means it has
			// closed. Both true at once would let the worker refuse admission
			// and raise its give-up warning on the same tick.
			var sawRefusing, sawDeadline int

			for _, elapsed := range []time.Duration{-time.Second, 0, time.Second, window - time.Nanosecond, window, window + time.Second} {
				for _, counts := range [][2]int{{0, 0}, {0, 1}, {1, 1}, {1, 2}, {2, 2}} {
					refusing, overDeadline := admissionDecision(elapsed, window, counts[0], counts[1])

					Expect(refusing && overDeadline).To(BeFalse(),
						"refusing and past-deadline at elapsed %s with measured %d of %d capable",
						elapsed, counts[0], counts[1])

					if refusing {
						sawRefusing++
					}
					if overDeadline {
						sawDeadline++
					}
				}
			}

			// Without these the loop above would pass on a decision that never
			// returned true at all.
			Expect(sawRefusing).To(BeNumerically(">", 0), "the grid reaches the refusing case")
			Expect(sawDeadline).To(BeNumerically(">", 0), "the grid reaches the past-deadline case")
		})
	})

	Describe("the window the worker ships", func() {
		It("refuses a never-measured box at nine seconds and admits it at ten", func() {
			// The specs above choose their own width, so they would all survive
			// a change to admissionWindow. This one would not: it names the two
			// seconds either side of ten and hands the decision the constant the
			// worker actually polls with.
			refusingAtNine, deadlineAtNine := admissionDecision(9*time.Second, admissionWindow, neverMeasured, oneCapable)
			Expect(refusingAtNine).To(BeTrue(), "the shipped window is still open at nine seconds")
			Expect(deadlineAtNine).To(BeFalse())

			refusingAtTen, deadlineAtTen := admissionDecision(10*time.Second, admissionWindow, neverMeasured, oneCapable)
			Expect(refusingAtTen).To(BeFalse(), "the shipped window has closed by ten seconds")
			Expect(deadlineAtTen).To(BeTrue())
		})
	})
})
