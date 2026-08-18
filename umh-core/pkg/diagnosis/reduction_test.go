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

const reduceHour = time.Hour

// The six reductions and the three States. These specs drive a window, then
// read the (v, State) pair back through Reduced.Get(); Reduced exposes no field
// to read it from directly.
var _ = Describe("Reduction", func() {
	It("should reduce under the reduction it was given at construction, so two windows fed the same points under different reductions return different numbers", func() {
		meanW, _ := NewSlidingWindow(reduceHour, 60*time.Second, Mean, false)
		p95W, _ := NewSlidingWindow(reduceHour, 60*time.Second, P95, false)

		t0 := time.Unix(1_000_000, 0)
		for i := range 20 {
			at := t0.Add(time.Duration(i) * time.Second)
			meanW.appendPoint(Known(float64(i)), Unknown(), at)
			p95W.appendPoint(Known(float64(i)), Unknown(), at)
		}

		meanN, meanS := meanW.Reduce().Get()
		p95N, p95S := p95W.Reduce().Get()

		Expect(meanS).To(Equal(StateValue), "twenty points reduce under Mean")
		Expect(p95S).To(Equal(StateValue), "twenty points reduce under P95")
		Expect(meanN).NotTo(Equal(p95N),
			"the same points under different reductions return different numbers")
	})

	It("should reduce by last value, mean, slope, delta ratio, nearest-rank p95 and nearest-rank p99", func() {
		t0 := time.Unix(1_000_000, 0)

		// Last: the newest entry.
		lastW, _ := NewSlidingWindow(reduceHour, 60*time.Second, Last, false)
		lastW.appendPoint(Known(4), Unknown(), t0)
		lastW.appendPoint(Known(7), Unknown(), t0.Add(2*time.Second))
		n, s := lastW.Reduce().Get()
		Expect(s).To(Equal(StateValue))
		Expect(n).To(Equal(7.0), "last is the newest entry")

		// Mean: the arithmetic mean.
		meanW, _ := NewSlidingWindow(reduceHour, 60*time.Second, Mean, false)
		for i := range 4 {
			meanW.appendPoint(Known(float64(i+1)), Unknown(), t0.Add(time.Duration(i)*time.Second))
		}
		n, s = meanW.Reduce().Get()
		Expect(s).To(Equal(StateValue))
		Expect(n).To(Equal(2.5), "mean of 1..4 is 2.5")

		// Slope: two endpoints, not a least-squares fit.
		slopeW, _ := NewSlidingWindow(reduceHour, 60*time.Second, Slope, false)
		slopeW.appendPoint(Known(10), Unknown(), t0)
		slopeW.appendPoint(Known(30), Unknown(), t0.Add(10*time.Second))
		n, s = slopeW.Reduce().Get()
		Expect(s).To(Equal(StateValue))
		Expect(n).To(Equal(2.0), "(30-10)/(10s-0s) reduces to 2.0 per second")

		// Delta ratio: (v_last-v_first)/(a_last-a_first), edges only.
		drW, _ := NewSlidingWindow(reduceHour, 60*time.Second, DeltaRatio, false)
		drW.appendPoint(Known(5), Known(100), t0)
		drW.appendPoint(Known(11), Known(300), t0.Add(10*time.Second))
		n, s = drW.Reduce().Get()
		Expect(s).To(Equal(StateValue))
		Expect(n).To(BeNumerically("~", (11.0-5.0)/(300.0-100.0), 1e-9),
			"(11-5)/(300-100) = 6/200 = 0.03")

		// P95: nearest-rank, ceil(0.95*20)=19th order statistic of 0..19.
		p95W, _ := NewSlidingWindow(reduceHour, 60*time.Second, P95, false)
		for i := range 20 {
			p95W.appendPoint(Known(float64(i)), Unknown(), t0.Add(time.Duration(i)*time.Second))
		}
		n, s = p95W.Reduce().Get()
		Expect(s).To(Equal(StateValue))
		Expect(n).To(Equal(18.0), "nearest-rank 95th percentile of 0..19 is 18")

		// P99: nearest-rank, ceil(0.99*100)=99th order statistic of 0..99.
		p99W, _ := NewSlidingWindow(reduceHour, 60*time.Second, P99, false)
		for i := range 100 {
			p99W.appendPoint(Known(float64(i)), Unknown(), t0.Add(time.Duration(i)*time.Second))
		}
		n, s = p99W.Reduce().Get()
		Expect(s).To(Equal(StateValue))
		Expect(n).To(Equal(98.0), "nearest-rank 99th percentile of 0..99 is 98")
	})

	It("should compute slope over the window's own first and last timestamps, never against wall-clock now", func() {
		slopeW, _ := NewSlidingWindow(reduceHour, 60*time.Second, Slope, false)

		t0 := time.Unix(1_000_000, 0)
		slopeW.appendPoint(Known(10), Unknown(), t0)
		slopeW.appendPoint(Known(11), Unknown(), t0.Add(5*time.Second))
		slopeW.appendPoint(Known(30), Unknown(), t0.Add(10*time.Second))

		n, s := slopeW.Reduce().Get()
		Expect(s).To(Equal(StateValue))
		Expect(n).To(Equal(2.0),
			"an intermediate point does not move the endpoint slope")
	})

	It("should let each reduction declare its own minimum sample count", func() {
		Expect(Last.Min).To(Equal(1))
		Expect(Mean.Min).To(Equal(2))
		Expect(Slope.Min).To(Equal(2))
		Expect(DeltaRatio.Min).To(Equal(2))
		Expect(P95.Min).To(Equal(20))
		Expect(P99.Min).To(Equal(100))
	})

	It("should report StateUntrusted below that minimum, while stale, or when the fold's divisor is zero, StateAbsent only when the window is empty or its newest entry is older than the demote span, and StateValue otherwise", func() {
		t0 := time.Unix(1_000_000, 0)

		// Below the minimum: one Mean entry is below Mean.Min of 2.
		belowW, _ := NewSlidingWindow(reduceHour, 60*time.Second, Mean, false)
		belowW.appendPoint(Known(7), Unknown(), t0)
		_, s := belowW.Reduce().Get()
		Expect(s).To(Equal(StateUntrusted), "one entry is below the mean's minimum of 2")

		// Empty: nothing stored.
		emptyW, _ := NewSlidingWindow(reduceHour, 60*time.Second, Mean, false)
		_, s = emptyW.Reduce().Get()
		Expect(s).To(Equal(StateAbsent), "an empty window reports absence")

		// Divisor zero: the denominator counter did not move.
		divW, _ := NewSlidingWindow(reduceHour, 60*time.Second, DeltaRatio, false)
		divW.appendPoint(Known(5), Known(100), t0)
		divW.appendPoint(Known(11), Known(100), t0.Add(10*time.Second))
		_, s = divW.Reduce().Get()
		Expect(s).To(Equal(StateUntrusted), "a zero denominator delta is untrusted, not a value")

		// Nothing appended this tick: a frozen tick, not a value.
		frozenW, _ := NewSlidingWindow(reduceHour, 60*time.Second, Last, false)
		t1 := t0.Add(1 * time.Second)
		frozenW.appendPoint(Known(9), Unknown(), t1)
		frozenW.age(t1)

		t2 := t0.Add(2 * time.Second)
		frozenW.age(t2)
		frozenW.appendPoint(Unknown(), Unknown(), t2)
		nFrozen, s := frozenW.Reduce().Get()
		Expect(s).To(Equal(StateUntrusted),
			"nothing appended this tick is untrusted, not a value")
		Expect(nFrozen).To(Equal(9.0),
			"a frozen StateUntrusted window still carries its last folded number")
	})

	It("should still carry the reduced number when it reports StateUntrusted, and carry none when it reports StateAbsent", func() {
		t0 := time.Unix(1_000_000, 0)

		// Below-min under Mean: the thin series still reduces to its own value.
		belowW, _ := NewSlidingWindow(reduceHour, 60*time.Second, Mean, false)
		belowW.appendPoint(Known(7), Unknown(), t0)
		n, s := belowW.Reduce().Get()
		Expect(s).To(Equal(StateUntrusted))
		Expect(n).To(Equal(7.0), "a StateUntrusted window carries its folded number")

		// Divisor-zero: the reduction cannot compute, so it carries zero.
		divW, _ := NewSlidingWindow(reduceHour, 60*time.Second, DeltaRatio, false)
		divW.appendPoint(Known(5), Known(100), t0)
		divW.appendPoint(Known(11), Known(100), t0.Add(10*time.Second))
		n, s = divW.Reduce().Get()
		Expect(s).To(Equal(StateUntrusted))
		Expect(n).To(Equal(0.0), "a zero-divisor StateUntrusted window carries 0")

		// Empty: absence carries no number.
		emptyW, _ := NewSlidingWindow(reduceHour, 60*time.Second, Mean, false)
		n, s = emptyW.Reduce().Get()
		Expect(s).To(Equal(StateAbsent))
		Expect(n).To(Equal(0.0), "a StateAbsent window carries no number")
	})

	It("should not report a slope over a single instant or equal timestamps, nor a ratio over a resetting denominator, as a finite trusted value", func() {
		t0 := time.Unix(1_000_000, 0)

		// Slope over a single point: dt is zero, the reduction cannot divide.
		single, _ := NewSlidingWindow(reduceHour, 60*time.Second, Slope, false)
		single.appendPoint(Known(10), Unknown(), t0)
		n, s := single.Reduce().Get()
		Expect(s).To(Equal(StateUntrusted), "a slope needs two endpoints")
		Expect(n).To(Equal(0.0), "a non-computable slope carries zero, not NaN")

		// Same edge timestamp: dt is zero and the reduction divides by it.
		equal, _ := NewSlidingWindow(reduceHour, 60*time.Second, Slope, false)
		equal.appendPoint(Known(10), Unknown(), t0)
		equal.appendPoint(Known(30), Unknown(), t0)
		n, s = equal.Reduce().Get()
		Expect(s).To(Equal(StateUntrusted), "equal edge timestamps make the slope undefined")
		Expect(math.IsNaN(n)).To(BeFalse(), "the value must not be NaN")
		Expect(math.IsInf(n, 0)).To(BeFalse(), "the value must not be infinite")
		Expect(n).To(Equal(0.0), "an undefined slope carries zero")

		// A denominator that decreases across the edges is a reset, not a fall
		// the ratio may trust.
		reset, _ := NewSlidingWindow(reduceHour, 60*time.Second, DeltaRatio, false)
		reset.appendPoint(Known(5), Known(100), t0)
		reset.appendPoint(Known(11), Known(5), t0.Add(10*time.Second))
		n, s = reset.Reduce().Get()
		Expect(s).To(Equal(StateUntrusted), "a decreasing denominator is untrusted, not a value")
		Expect(n).To(Equal(0.0), "a resetting denominator carries zero, not a trusted negative ratio")
	})

	It("should refuse to build a reduction below its minimum floor or without a fold", func() {
		_, err := NewReduction("bad-min", 0, foldMean)
		Expect(err).To(HaveOccurred(), "a minimum sample count below one is refused at construction")

		_, err = NewReduction("no-fold", 2, nil)
		Expect(err).To(HaveOccurred(), "a nil fold is refused at construction")

		// A reduction that passes both checks builds.
		reduction, err := NewReduction("valid", 2, foldMean)
		Expect(err).NotTo(HaveOccurred())
		Expect(reduction.Min).To(Equal(2))
	})
})
