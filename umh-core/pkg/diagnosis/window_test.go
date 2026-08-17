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

// These specs assert State (through Reduce().Get()) and Coverage; the reduced
// number itself is asserted in the reduction specs.
//
// Every tick here runs age BEFORE appendPoint, the order Observe uses. age on
// tick N reads whether the PREVIOUS tick stored a point, so the first failed
// tick still prunes to span and the freeze only holds from the tick after;
// storing first would hide that. The two halves are unexported and are called
// separately so each can be exercised alone. The last spec drives Observe.
var _ = Describe("Window", func() {
	It("should append a reading and prune entries older than the span", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Last, false)

		t0 := time.Unix(1_000_000, 0)
		w.age(t0)
		w.appendPoint(Known(5), Unknown(), t0)

		later := t0.Add(span + time.Millisecond)
		w.age(later)
		w.appendPoint(Known(6), Unknown(), later)

		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue),
			"the remaining recent entry should reduce to a value under Last")

		// One surviving entry spans 0, so a pruned window is not Full; had both
		// been kept they would span span+eps and read Full.
		Expect(w.Coverage().Full()).To(BeFalse(),
			"the older entry was pruned, so the window does not span the full window duration")
	})

	It("should not append a reading whose read failed", func() {
		w, _ := NewWindow(10*time.Second, 60*time.Second, Last, false)

		t0 := time.Unix(2_000_000, 0)
		w.age(t0)
		w.appendPoint(Unknown(), Unknown(), t0)

		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateAbsent),
			"a failed read appends nothing and the window stays empty")
	})

	It("should not append a value that is not a number — NaN, +Inf or -Inf — and append nothing rather than a zero", func() {
		const span = 10 * time.Second
		refused, _ := NewWindow(span, 60*time.Second, Last, false)

		t0 := time.Unix(3_000_000, 0)
		refused.age(t0)
		refused.appendPoint(Known(math.NaN()), Unknown(), t0)
		refused.age(t0.Add(time.Second))
		refused.appendPoint(Known(math.Inf(1)), Unknown(), t0.Add(time.Second))
		refused.age(t0.Add(2 * time.Second))
		refused.appendPoint(Known(math.Inf(-1)), Unknown(), t0.Add(2*time.Second))

		_, state := refused.Reduce().Get()
		Expect(state).To(Equal(StateAbsent),
			"NaN and the infinities are not numbers and append nothing; the window stays empty")

		// The window must not range-check: a negative number is still a number.
		neg, _ := NewWindow(span, 60*time.Second, Last, false)
		neg.age(t0)
		neg.appendPoint(Known(-5), Unknown(), t0)

		_, state = neg.Reduce().Get()
		Expect(state).To(Equal(StateValue),
			"a negative number is a value and is appended")
	})

	It("should start the window over when a series declared a monotone counter goes backwards, and keep its entries when a series that is not one merely falls", func() {
		const span = 10 * time.Second
		t0 := time.Unix(4_000_000, 0)

		// Arm A: a counter window (the trailing true).
		counter, _ := NewWindow(span, 60*time.Second, Mean, true)
		counter.age(t0)
		counter.appendPoint(Known(10), Unknown(), t0)
		counter.age(t0.Add(time.Second))
		counter.appendPoint(Known(5), Unknown(), t0.Add(time.Second)) // a fall: 5 < 10
		_, counterState := counter.Reduce().Get()
		Expect(counterState).To(Equal(StateUntrusted),
			"a counter window restarts on a backwards step, holding one entry (below Mean Min 2)")

		// Arm B: the same falling series on a window that is not a counter.
		noncounter, _ := NewWindow(span, 60*time.Second, Mean, false)
		noncounter.age(t0)
		noncounter.appendPoint(Known(10), Unknown(), t0)
		noncounter.age(t0.Add(time.Second))
		noncounter.appendPoint(Known(5), Unknown(), t0.Add(time.Second)) // an ordinary fall
		_, noncounterState := noncounter.Reduce().Get()
		Expect(noncounterState).To(Equal(StateValue),
			"a non-counter window keeps a falling series, holding two entries (meets Mean Min 2)")
	})

	It("should stop pruning while reads are failing, so the window holds its last known contents", func() {
		// span=1s so the held entry is far out of span by the end; demote=60s so
		// the demote arm cannot reach it.
		const span = 1 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, false)

		t0 := time.Unix(5_000_000, 0)
		w.age(t0)
		w.appendPoint(Known(5), Unknown(), t0)
		w.age(t0.Add(span))
		w.appendPoint(Known(6), Unknown(), t0.Add(span))
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue), "the window first reaches a trustworthy state")

		w.age(t0.Add(2 * time.Second))
		w.appendPoint(Unknown(), Unknown(), t0.Add(2*time.Second))
		w.age(t0.Add(30 * time.Second))
		w.appendPoint(Unknown(), Unknown(), t0.Add(30*time.Second))

		_, heldState := w.Reduce().Get()
		// The surviving entry is 29s old against a 1s span. The first failed tick
		// already pruned to span; what must not continue is the pruning after it.
		Expect(heldState).To(Equal(StateUntrusted),
			"the window holds its last entry through failed reads (Untrusted, not Absent)")
		Expect(w.Coverage().Full()).To(BeFalse(),
			"a single frozen entry does not span the window; what matters is that pruning stopped, not that coverage stayed full")
	})

	It("should empty itself once no read has succeeded for longer than the demote span", func() {
		// span (10s) and demote (60s) differ, so the entries outlive the span and
		// only the demote clock can empty the window.
		const span = 10 * time.Second
		const demoteSpan = 60 * time.Second
		w, _ := NewWindow(span, demoteSpan, Mean, false)

		t0 := time.Unix(6_000_000, 0)
		w.age(t0)
		w.appendPoint(Known(5), Unknown(), t0)
		w.age(t0.Add(time.Second))
		w.appendPoint(Known(6), Unknown(), t0.Add(time.Second))
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue), "two entries meet Mean Min 2")

		w.age(t0.Add(2 * time.Second))
		w.appendPoint(Unknown(), Unknown(), t0.Add(2*time.Second))
		w.age(t0.Add(30 * time.Second))
		w.appendPoint(Unknown(), Unknown(), t0.Add(30*time.Second))
		_, held := w.Reduce().Get()
		Expect(held).To(Equal(StateUntrusted),
			"within the demote span the frozen entries are held (not pruned nor emptied)")

		w.age(t0.Add(70 * time.Second))
		w.appendPoint(Unknown(), Unknown(), t0.Add(70*time.Second))
		_, emptiedState := w.Reduce().Get()
		Expect(emptiedState).To(Equal(StateAbsent),
			"after the demote span with no successful read the window empties")
	})

	It("should append nothing when the denominator is absent and its own reduction divides by a denominator, and append the point when its reduction does not", func() {
		const span = 10 * time.Second
		t0 := time.Unix(7_000_000, 0)

		// The same call behaves oppositely under two reductions: the gate reads
		// only whether the window's own reduction divides by a denominator.

		// Arm A: DeltaRatio does.
		ratio, _ := NewWindow(span, 60*time.Second, DeltaRatio, false)
		ratio.age(t0)
		ratio.appendPoint(Known(5), Unknown(), t0)
		_, ratioState := ratio.Reduce().Get()
		Expect(ratioState).To(Equal(StateAbsent),
			"under DeltaRatio an absent denominator drops the point; the window is empty")

		// Arm B: Mean does not, so an absent denominator is the ordinary
		// single-series case.
		mean, _ := NewWindow(span, 60*time.Second, Mean, false)
		mean.age(t0)
		mean.appendPoint(Known(5), Unknown(), t0)
		_, meanState := mean.Reduce().Get()
		Expect(meanState).To(Equal(StateUntrusted),
			"under Mean an absent denominator is ordinary; the point is stored (one entry, below Min 2)")
	})

	It("should keep a sample landing exactly on the cutoff", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, false)

		t0 := time.Unix(8_000_000, 0)
		w.age(t0)
		w.appendPoint(Known(5), Unknown(), t0)

		// Exactly one span on, the t0 entry sits on the cutoff (now - span == t0),
		// which a strict-Before prune keeps.
		now := t0.Add(span)
		w.age(now)
		w.appendPoint(Known(6), Unknown(), now)

		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue),
			"the boundary entry at cutoff is kept, so two entries meet Mean Min 2")
		Expect(w.Coverage().Full()).To(BeTrue(),
			"two entries spanning exactly the span read Full")
	})

	It("should store a point whose denominator is present under a ratio reduction, and drop one whose denominator is not a number", func() {
		const span = 10 * time.Second
		t0 := time.Unix(9_000_000, 0)

		// The permissive arm: without it these points would be dropped and the
		// window left empty.
		ratio, _ := NewWindow(span, 60*time.Second, DeltaRatio, false)
		ratio.age(t0)
		ratio.appendPoint(Known(5), Known(10), t0)
		ratio.age(t0.Add(time.Second))
		ratio.appendPoint(Known(6), Known(12), t0.Add(time.Second))
		_, presentState := ratio.Reduce().Get()
		Expect(presentState).To(Equal(StateValue),
			"a present denominator stores the point; two points meet DeltaRatio Min 2")

		nan, _ := NewWindow(span, 60*time.Second, DeltaRatio, false)
		nan.age(t0)
		nan.appendPoint(Known(5), Known(math.NaN()), t0)
		_, nanState := nan.Reduce().Get()
		Expect(nanState).To(Equal(StateAbsent),
			"a NaN denominator is not a number; the point is dropped and the window is empty")

		inf, _ := NewWindow(span, 60*time.Second, DeltaRatio, false)
		inf.age(t0)
		inf.appendPoint(Known(5), Known(math.Inf(1)), t0)
		_, infState := inf.Reduce().Get()
		Expect(infState).To(Equal(StateAbsent),
			"an Inf denominator is not a number; the point is dropped and the window is empty")
	})

	It("should return to a value after a freeze once a successful append lands", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, false)

		t0 := time.Unix(10_000_000, 0)
		w.age(t0)
		w.appendPoint(Known(5), Unknown(), t0)
		w.age(t0.Add(time.Second))
		w.appendPoint(Known(6), Unknown(), t0.Add(time.Second))
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue), "the window first reaches a trustworthy state")

		w.age(t0.Add(2 * time.Second))
		w.appendPoint(Unknown(), Unknown(), t0.Add(2*time.Second))
		_, frozen := w.Reduce().Get()
		Expect(frozen).To(Equal(StateUntrusted), "a failed read leaves the window Untrusted, holding its contents")

		// A build that pinned lastAppendStored false would stay Untrusted here.
		recover := t0.Add(3 * time.Second)
		w.age(recover)
		w.appendPoint(Known(7), Unknown(), recover)
		_, recovered := w.Reduce().Get()
		Expect(recovered).To(Equal(StateValue),
			"a successful append after a freeze returns the window to StateValue")
	})

	It("should re-accumulate to a value after a counter restart", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, true)

		t0 := time.Unix(11_000_000, 0)
		w.age(t0)
		w.appendPoint(Known(10), Unknown(), t0)
		w.age(t0.Add(time.Second))
		w.appendPoint(Known(12), Unknown(), t0.Add(time.Second))
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue), "two forward samples meet Mean Min 2")

		w.age(t0.Add(2 * time.Second))
		w.appendPoint(Known(5), Unknown(), t0.Add(2*time.Second)) // 5 < 12: reset
		_, restarted := w.Reduce().Get()
		Expect(restarted).To(Equal(StateUntrusted), "a counter reset holds one entry (below Min 2)")

		// A build that pinned the window empty after a reset would stay Untrusted.
		w.age(t0.Add(3 * time.Second))
		w.appendPoint(Known(8), Unknown(), t0.Add(3*time.Second)) // 8 > 5: forward
		_, recovered := w.Reduce().Get()
		Expect(recovered).To(Equal(StateValue),
			"a forward sample after a counter reset re-accumulates to StateValue")
	})

	It("should treat the first post-demote point as Untrusted below the minimum", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, false)

		t0 := time.Unix(12_000_000, 0)
		w.age(t0)
		w.appendPoint(Known(5), Unknown(), t0)
		w.age(t0.Add(time.Second))
		w.appendPoint(Known(6), Unknown(), t0.Add(time.Second))
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue), "two samples meet Mean Min 2")

		// The last successful read was at t0+1s, so t0+62s is 61s after it and
		// crosses the 60s demote boundary.
		t62 := t0.Add(62 * time.Second)
		w.age(t62)
		w.appendPoint(Unknown(), Unknown(), t62)
		_, emptied := w.Reduce().Get()
		Expect(emptied).To(Equal(StateAbsent), "the demote span empties the window")

		// A build that failed to reset accumulation after a demote would report
		// Value here.
		t63 := t0.Add(63 * time.Second)
		w.age(t63)
		w.appendPoint(Known(7), Unknown(), t63)
		_, rebuilt := w.Reduce().Get()
		Expect(rebuilt).To(Equal(StateUntrusted),
			"the first post-demote point is a single entry (below Mean Min 2)")
	})

	It("should prune stale pre-outage points on the recovery tick under the production Age-before-Append order, so a recovered window does not fold them as trusted", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, false)
		t0 := time.Unix(30_000_000, 0)

		w.age(t0)
		w.appendPoint(Known(5), Unknown(), t0)
		w.age(t0.Add(time.Second))
		w.appendPoint(Known(6), Unknown(), t0.Add(time.Second))

		// An outage: age, then a failed append, on each tick.
		for i := 2; i <= 6; i++ {
			at := t0.Add(time.Duration(i) * time.Second)
			w.age(at)
			w.appendPoint(Unknown(), Unknown(), at)
		}

		// Recovery at 20s: age runs first and, frozen by the prior tick's failure,
		// prunes nothing; the successful append that follows must prune the stale
		// out-of-span 5 and 6 itself.
		rec := t0.Add(20 * time.Second)
		w.age(rec)
		w.appendPoint(Known(100), Unknown(), rec)

		v, st := w.Reduce().Get()
		// The bug this pins reported a trusted mean of 37 over 5, 6 and 100.
		Expect(v).To(Equal(100.0),
			"the recovery append prunes the stale out-of-span points; a build that folds them reports a wrong mean of 37")
		Expect(st).To(Equal(StateUntrusted),
			"one recovered point is below Mean's minimum of 2: honest-Untrusted, not a folded trusted value")
	})

	It("should not restart a counter window on an equal value — only a strict fall resets", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, true)

		t0 := time.Unix(20_000_000, 0)
		w.age(t0)
		w.appendPoint(Known(10), Unknown(), t0)
		// A build using <= would empty on every unchanged counter reading.
		w.age(t0.Add(time.Second))
		w.appendPoint(Known(10), Unknown(), t0.Add(time.Second))

		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue),
			"an equal value is not a backwards step; the counter window keeps both entries and meets Mean Min 2")
	})

	It("should not demote at exactly the demote span — only strictly past it", func() {
		const span = 10 * time.Second
		const demoteSpan = 60 * time.Second
		w, _ := NewWindow(span, demoteSpan, Mean, false)

		t0 := time.Unix(21_000_000, 0)
		w.age(t0)
		w.appendPoint(Known(5), Unknown(), t0)
		w.age(t0.Add(time.Second))
		w.appendPoint(Known(6), Unknown(), t0.Add(time.Second))
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue), "the window first reaches a trustworthy state")

		// A failed read begins the freeze, so the entries outlive the span and the
		// demote boundary is observable rather than masked by span-pruning.
		w.age(t0.Add(2 * time.Second))
		w.appendPoint(Unknown(), Unknown(), t0.Add(2*time.Second))

		// exact is one demote span after the last successful read at t0+1s, so
		// now.Sub(lastStored()) == demote to the nanosecond.
		exact := t0.Add(time.Second).Add(demoteSpan)
		w.age(exact)
		_, atBoundary := w.Reduce().Get()
		Expect(atBoundary).To(Equal(StateUntrusted),
			"at exactly the demote span the frozen window holds (Untrusted), not emptied")

		w.age(exact.Add(time.Second))
		_, pastBoundary := w.Reduce().Get()
		Expect(pastBoundary).To(Equal(StateAbsent),
			"just past the demote span the window empties")
	})

	It("should age before storing, so the first failed read still prunes", func() {
		// The demote span is an hour so the demote arm cannot reach the window and
		// confuse the two effects, and the reduction is Mean because the number it
		// reduces to is what separates the two orders: only the pruned window
		// answers 2.
		const span = 10 * time.Second
		w, err := NewWindow(span, time.Hour, Mean, false)
		Expect(err).NotTo(HaveOccurred())

		t0 := time.Unix(1_000_000, 0)
		w.Observe(Known(1), Unknown(), t0)
		w.Observe(Known(2), Unknown(), t0.Add(5*time.Second))

		// First failed read, 12s in: the cutoff is t0+2s, so the t0 entry is out
		// of span and goes.
		w.Observe(Unknown(), Unknown(), t0.Add(12*time.Second))

		v, state := w.Reduce().Get()
		Expect(v).To(Equal(2.0),
			"the t0 entry aged out before the failed read froze the window; "+
				"storing first would freeze immediately and fold 1 and 2 to 1.5")
		Expect(state).To(Equal(StateUntrusted),
			"nothing was stored this tick, so the number is not trustworthy")
	})
})
