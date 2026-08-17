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

// The two series of a DeltaRatio pair move together: one is the numerator, the
// other the denominator, and a source reset moves both. A backwards step in
// EITHER therefore discards the stored entries and starts over from that point.
//
// The restart is asserted after re-accumulating, not at the restart itself. A
// window that missed a DENOMINATOR restart holds a negative denominator delta,
// which Reduce's divisor gate reports as StateUntrusted anyway, so that state
// cannot tell the two apart.
var _ = Describe("DeltaRatio", func() {
	It("should start over when either counter of a pair declared monotone goes backwards", func() {
		const span = 10 * time.Second
		t0 := time.Unix(30_000_000, 0)

		// Arm A: the DENOMINATOR falls across the edge (100 -> 50) while the
		// numerator rises, as a cgroup reset would do.
		denomReset, _ := NewSlidingWindow(span, 60*time.Second, DeltaRatio, true)
		denomReset.appendPoint(Known(5), Known(100), t0)
		denomReset.age(t0)
		denomReset.appendPoint(Known(11), Known(50), t0.Add(time.Second)) // denominator 100 -> 50 fell, restart
		denomReset.age(t0.Add(time.Second))
		denomReset.appendPoint(Known(15), Known(60), t0.Add(2*time.Second)) // forward from the reset origin
		denomReset.age(t0.Add(2 * time.Second))
		ratio, denomState := denomReset.Reduce().Get()
		Expect(denomState).To(Equal(StateValue),
			"a denominator that falls restarts the window, discarding the reset origin and re-accumulating to a value")
		Expect(ratio).To(BeNumerically("~", 0.4, 1e-9))

		// Arm B: the NUMERATOR falls (5 -> 3) while the denominator rises.
		numReset, _ := NewSlidingWindow(span, 60*time.Second, DeltaRatio, true)
		numReset.appendPoint(Known(5), Known(100), t0)
		numReset.age(t0)
		numReset.appendPoint(Known(3), Known(200), t0.Add(time.Second)) // value 5 -> 3 fell, restart
		numReset.age(t0.Add(time.Second))
		numReset.appendPoint(Known(8), Known(300), t0.Add(2*time.Second)) // forward from the reset origin
		numReset.age(t0.Add(2 * time.Second))
		numRatio, numState := numReset.Reduce().Get()
		Expect(numState).To(Equal(StateValue),
			"a numerator that falls restarts the window, discarding the reset origin and re-accumulating to a value")
		Expect(numRatio).To(BeNumerically("~", 0.05, 1e-9),
			"a numerator reset re-accumulates to a ratio from the two surviving points")

		// Positive control: a restart rule that fired on any window, or on every
		// fall whichever series it was in, would empty this one.
		forward, _ := NewSlidingWindow(span, 60*time.Second, DeltaRatio, true)
		forward.appendPoint(Known(5), Known(100), t0)
		forward.age(t0)
		forward.appendPoint(Known(11), Known(300), t0.Add(time.Second))
		forward.age(t0.Add(time.Second))
		ratio, forwardState := forward.Reduce().Get()
		Expect(forwardState).To(Equal(StateValue),
			"a forward pair on a counter window does not restart; two entries meet DeltaRatio Min 2")
		Expect(ratio).To(BeNumerically("~", 0.03, 1e-9))

		// Negative control: Mean does not divide by a denominator, so a build that
		// ran the denominator restart for every counter window would empty this
		// window on the dip.
		noDenom, _ := NewSlidingWindow(span, 60*time.Second, Mean, true)
		noDenom.appendPoint(Known(5), Known(100), t0)
		noDenom.age(t0)
		noDenom.appendPoint(Known(6), Known(50), t0.Add(time.Second)) // denominator 100 -> 50 fell, but Mean does not divide by it
		noDenom.age(t0.Add(time.Second))
		_, noDenomState := noDenom.Reduce().Get()
		Expect(noDenomState).To(Equal(StateValue),
			"a falling denominator on a non-against counter window does not restart; both entries meet Mean Min 2")

		// Equality guard, mirroring the value-equality rule: a build using <= on
		// the denominator would restart here, wipe to one entry and read not-Full.
		eqDenom, _ := NewSlidingWindow(span, 60*time.Second, DeltaRatio, true)
		eqDenom.appendPoint(Known(5), Known(100), t0)
		eqDenom.age(t0)
		eqDenom.appendPoint(Known(11), Known(100), t0.Add(span)) // denominator equal, not a fall
		eqDenom.age(t0.Add(span))
		Expect(eqDenom.Coverage().Full()).To(BeTrue(),
			"an equal denominator is not a backwards step; both entries are kept (no restart)")
		// The two gates are layered: append keeps both edges, and Reduce then
		// voids the window because the denominator delta is zero.
		_, eqState := eqDenom.Reduce().Get()
		Expect(eqState).To(Equal(StateUntrusted),
			"an equal denominator delta is zero; the window cannot form a delta-ratio and reduces to StateUntrusted")
	})

	It("should keep a NON-counter DeltaRatio window when its denominator falls", func() {
		const span = 10 * time.Second
		// counter=false, so a build that weakened the w.counter gate on the
		// DENOMINATOR arm would wipe this window on the dip.
		w, _ := NewSlidingWindow(span, 60*time.Second, DeltaRatio, false)
		t0 := time.Unix(40_000_000, 0)
		w.appendPoint(Known(5), Known(100), t0)
		w.age(t0)
		w.appendPoint(Known(11), Known(50), t0.Add(span)) // denominator 100 -> 50 fell
		w.age(t0.Add(span))
		Expect(w.Coverage().Full()).To(BeTrue(),
			"a falling denominator on a NON-counter window does not restart; both entries are kept")
	})

	It("should restart when both counters of a monotone pair fall on the same tick", func() {
		const span = 10 * time.Second
		// A full cgroup death and recreate drops both counters at once; the
		// restart must fire exactly once.
		w, _ := NewSlidingWindow(span, 60*time.Second, DeltaRatio, true)
		t0 := time.Unix(41_000_000, 0)
		w.appendPoint(Known(5), Known(100), t0)
		w.age(t0)
		w.appendPoint(Known(3), Known(50), t0.Add(time.Second)) // value 5->3 AND denom 100->50 both fell
		w.age(t0.Add(time.Second))
		w.appendPoint(Known(8), Known(60), t0.Add(2*time.Second)) // forward from the reset origin
		w.age(t0.Add(2 * time.Second))
		ratio, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue),
			"a both-counter reset wipes the window once and re-accumulates to a value")
		Expect(ratio).To(BeNumerically("~", 0.5, 1e-9),
			"the surviving pair (3,50)-(8,60) reduces to (8-3)/(60-50) = 0.5")
	})
})
