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

var _ = Describe("the admission-deadline decision", func() {
	// shortfallAtDeadline reads only its arguments, so nothing here builds a
	// worker, a sampler or a clock. Each spec names the condition a worker
	// would be in and asserts whether it then reports.
	//
	// window is this file's own width, not the shipped constant: the specs that
	// vary it prove the function reads the argument, and the last Describe
	// binds the behaviour back to the constant the worker actually ships.
	const window = 10 * time.Second

	// The evidence counts, named so a reader does not have to decode a pair of
	// integers at each call. A shortfall is one capable signal that has never
	// produced a reading — the condition the deadline report exists for.
	const (
		neverMeasured, oneCapable = 0, 1
		allMeasured               = 1
	)

	Describe("a box whose capable signal has never measured", func() {
		It("has not reached the deadline nine seconds in", func() {
			Expect(shortfallAtDeadline(9*time.Second, window, neverMeasured, oneCapable)).To(BeFalse(),
				"nine seconds has not reached a ten-second deadline, so the worker is still waiting")
		})

		It("has not reached the deadline at the last instant inside the window", func() {
			Expect(shortfallAtDeadline(window-time.Nanosecond, window, neverMeasured, oneCapable)).To(BeFalse(),
				"one nanosecond short of the window is still inside it")
		})

		It("reaches the deadline at exactly the window", func() {
			// The boundary is closed: this single tick decides both directions,
			// because a worker that waited for elapsed to exceed the window
			// would still be silent here.
			Expect(shortfallAtDeadline(window, window, neverMeasured, oneCapable)).To(BeTrue(),
				"the deadline is the instant the window closes, with the counts unchanged")
		})

		It("stays at the deadline after the window has closed", func() {
			Expect(shortfallAtDeadline(11*time.Second, window, neverMeasured, oneCapable)).To(BeTrue(),
				"a shortfall that outlives the window keeps reaching the deadline")
		})

		It("treats a sample timestamp that stepped backwards as still inside the window", func() {
			// A negative delta cannot arise in production, where the timestamps
			// are monotonic, but a synthetic clock can step backwards. It reads
			// as inside the window exactly like zero does, so a backward step
			// prolongs the wait rather than reaching the deadline early.
			Expect(shortfallAtDeadline(-3*time.Second, window, neverMeasured, oneCapable)).To(BeFalse(),
				"a negative elapsed is inside the window, like the anchor tick itself")
		})
	})

	Describe("a box with no shortfall to report", func() {
		It("says nothing about a box no instrument can answer, at any point in the window", func() {
			// Nothing capable means nothing to be missing. A worker that
			// reported here would warn about every box that can never satisfy
			// it, the deadline included.
			for _, elapsed := range []time.Duration{0, 9 * time.Second, window, 11 * time.Second} {
				Expect(shortfallAtDeadline(elapsed, window, 0, 0)).To(BeFalse(),
					"no capable signal means no shortfall, at elapsed %s", elapsed)
			}
		})

		It("says nothing when more signals measured than are capable, the deadline included", func() {
			Expect(shortfallAtDeadline(window, window, 2, oneCapable)).To(BeFalse(),
				"a count above capable is a surplus, never a shortfall")
		})

		It("says nothing at the deadline when there was no shortfall to report", func() {
			// The deadline is what makes the worker give up waiting and warn.
			// Reaching it is not on its own a reason to warn: a box that
			// measured everything, and a box with nothing to measure, both
			// reach it with nothing wrong. Only the box still missing a reading
			// is worth an operator's attention.
			measuredBox := shortfallAtDeadline(window, window, allMeasured, oneCapable)
			emptyBox := shortfallAtDeadline(window, window, 0, 0)
			shortBox := shortfallAtDeadline(window, window, neverMeasured, oneCapable)

			Expect(measuredBox).To(BeFalse(), "a fully measured box reaches the deadline with nothing to say")
			Expect(emptyBox).To(BeFalse(), "nor does a box no instrument can answer")
			Expect(shortBox).To(BeTrue(),
				"the box still missing a reading at the deadline is the one that gets reported")
		})
	})

	Describe("the width it is given", func() {
		It("moves the boundary to the window in its arguments, not to the shipped one", func() {
			// Handed five seconds, the decision must turn at five. If it read
			// admissionWindow instead of its parameter, nine seconds would not
			// have reached the deadline here.
			const short = 5 * time.Second

			Expect(shortfallAtDeadline(4*time.Second, short, neverMeasured, oneCapable)).To(BeFalse(),
				"four seconds is inside a five-second window")
			Expect(shortfallAtDeadline(short, short, neverMeasured, oneCapable)).To(BeTrue(),
				"five seconds closes a five-second window")
			Expect(shortfallAtDeadline(9*time.Second, short, neverMeasured, oneCapable)).To(BeTrue(),
				"nine seconds is past a five-second window, however long the shipped one is")
		})
	})

	Describe("the window the worker ships", func() {
		It("says nothing about a never-measured box at nine seconds and reports it at ten", func() {
			// The specs above choose their own width, so they would all survive
			// a change to admissionWindow. This one would not: it names the two
			// seconds either side of ten and hands the decision the constant the
			// worker actually polls with.
			Expect(shortfallAtDeadline(9*time.Second, admissionWindow, neverMeasured, oneCapable)).To(BeFalse(),
				"the shipped window is still open at nine seconds")
			Expect(shortfallAtDeadline(10*time.Second, admissionWindow, neverMeasured, oneCapable)).To(BeTrue(),
				"the shipped window has closed by ten seconds")
		})
	})
})
