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

// The window: append, prune, freeze, empty, restart, denominator-gate.
//
// These specs observe window behaviour through Reduce().Get() State
// (StateAbsent/StateUntrusted/StateValue) and Coverage() (Full/Span) — never
// through the fold-computed number v. This file asserts State and Coverage
// only; the correctness of v is covered in the reduction specs. The window
// State logic under test:
//   - empty                                  -> StateAbsent
//   - newest older than the demote span       -> StateAbsent
//   - count < reduction Min                  -> StateUntrusted
//   - nothing appended this tick             -> StateUntrusted
//   - otherwise                              -> StateValue
//
// Only State and Coverage are asserted here; the fold-computed number v is
// covered in the reduction specs.
var _ = Describe("Window", func() {
	It("should append a reading and prune entries older than the span", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Last, false)

		t0 := time.Unix(1_000_000, 0)
		w.Append(Known(5), Unknown(), t0)
		w.Age(t0)

		// A second reading lands just past the span; the older entry is pruned.
		later := t0.Add(span + time.Millisecond)
		w.Append(Known(6), Unknown(), later)
		w.Age(later)

		// One recent entry remains under Last (Min 1): a value.
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue),
			"the remaining recent entry should reduce to a value under Last")

		// Pruning (not "kept both") is observable via coverage: a single entry
		// spans 0, so the window is not Full; two entries spanning span+eps would be.
		Expect(w.Coverage().Full()).To(BeFalse(),
			"the older entry was pruned, so the window does not span the full window duration")
	})

	It("should not append a reading whose read failed", func() {
		w, _ := NewWindow(10*time.Second, 60*time.Second, Last, false)

		t0 := time.Unix(2_000_000, 0)
		w.Append(Unknown(), Unknown(), t0)
		w.Age(t0)

		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateAbsent),
			"a failed read appends nothing and the window stays empty")
	})

	It("should not append a value that is not a number — NaN, +Inf or -Inf — and append nothing rather than a zero", func() {
		const span = 10 * time.Second
		refused, _ := NewWindow(span, 60*time.Second, Last, false)

		t0 := time.Unix(3_000_000, 0)
		refused.Append(Known(math.NaN()), Unknown(), t0)
		refused.Append(Known(math.Inf(1)), Unknown(), t0.Add(time.Second))
		refused.Append(Known(math.Inf(-1)), Unknown(), t0.Add(2*time.Second))
		refused.Age(t0.Add(2 * time.Second))

		_, state := refused.Reduce().Get()
		Expect(state).To(Equal(StateAbsent),
			"NaN and the infinities are not numbers and append nothing; the window stays empty")

		// A negative number IS a value and IS appended; the window must not range-check it.
		neg, _ := NewWindow(span, 60*time.Second, Last, false)
		neg.Append(Known(-5), Unknown(), t0)
		neg.Age(t0)

		_, state = neg.Reduce().Get()
		Expect(state).To(Equal(StateValue),
			"a negative number is a value and is appended")
	})

	It("should start the window over when a series declared a monotone counter goes backwards, and keep its entries when a series that is not one merely falls", func() {
		const span = 10 * time.Second
		t0 := time.Unix(4_000_000, 0)

		// Arm A: a COUNTER window discards stored entries on a backwards step and
		// starts over from the new one. Mean (Min 2): one entry -> Untrusted.
		counter, _ := NewWindow(span, 60*time.Second, Mean, true)
		counter.Append(Known(10), Unknown(), t0)
		counter.Age(t0)
		counter.Append(Known(5), Unknown(), t0.Add(time.Second)) // a fall: 5 < 10
		counter.Age(t0.Add(time.Second))
		_, counterState := counter.Reduce().Get()
		Expect(counterState).To(Equal(StateUntrusted),
			"a counter window restarts on a backwards step, holding one entry (below Mean Min 2)")

		// Arm B: a NON-counter window over the same falling series KEEPS its
		// entries. Two entries -> Value under Mean (Min 2).
		noncounter, _ := NewWindow(span, 60*time.Second, Mean, false)
		noncounter.Append(Known(10), Unknown(), t0)
		noncounter.Age(t0)
		noncounter.Append(Known(5), Unknown(), t0.Add(time.Second)) // an ordinary fall
		noncounter.Age(t0.Add(time.Second))
		_, noncounterState := noncounter.Reduce().Get()
		Expect(noncounterState).To(Equal(StateValue),
			"a non-counter window keeps a falling series, holding two entries (meets Mean Min 2)")
	})

	It("should stop pruning while reads are failing, so the window holds its last known contents", func() {
		// span=1s, demote=60s. Two readable ticks one second apart reach a
		// trustworthy (Value) state and span the full window.
		const span = 1 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, false)

		t0 := time.Unix(5_000_000, 0)
		w.Append(Known(5), Unknown(), t0)
		w.Age(t0)
		w.Append(Known(6), Unknown(), t0.Add(span))
		w.Age(t0.Add(span))
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue), "the window first reaches a trustworthy state")

		// Drive failed reads while advancing time past the span. The window
		// HOLDS its last known contents: it does not shrink or empty.
		w.Append(Unknown(), Unknown(), t0.Add(5*time.Second))
		w.Age(t0.Add(5 * time.Second))
		w.Append(Unknown(), Unknown(), t0.Add(30*time.Second))
		w.Age(t0.Add(30 * time.Second))

		_, heldState := w.Reduce().Get()
		// Held, not Absent: nothing appended this tick, but the entries are
		// frozen, so the window is Untrusted rather than emptied.
		Expect(heldState).To(Equal(StateUntrusted),
			"the window holds its contents through failed reads (Untrusted, not Absent)")
		// The freeze is load-bearing: the held entries still span the full
		// window. A build that prunes during the outage drops to spanned 0.
		Expect(w.Coverage().Full()).To(BeTrue(),
			"pruning is frozen during failed reads, so the window still spans the full span")
	})

	It("should empty itself once no read has succeeded for longer than the demote span", func() {
		// span and demote span DIFFER: span=10s, demote=60s. A single reading is
		// appended, then no successful reads follow.
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, false)

		t0 := time.Unix(6_000_000, 0)
		w.Append(Known(5), Unknown(), t0)
		w.Age(t0)

		// At t0+11s the entry is older than the span, but the window does NOT
		// empty — the failed read freezes pruning, so the entry is held (not
		// pruned), and the last successful read is still within the demote span.
		t11 := t0.Add(11 * time.Second)
		w.Append(Unknown(), Unknown(), t11) // failed read appends nothing
		w.Age(t11)
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateUntrusted),
			"at 11s the entry is frozen (held, not pruned): one entry below Mean Min 2")

		// After the demote span with no successful read, the window empties.
		t61 := t0.Add(61 * time.Second)
		w.Append(Unknown(), Unknown(), t61)
		w.Age(t61)
		_, emptiedState := w.Reduce().Get()
		Expect(emptiedState).To(Equal(StateAbsent),
			"after the demote span with no successful read the window empties")
	})

	It("should append nothing when the denominator is absent and its own reduction divides by a denominator, and append the point when its reduction does not", func() {
		const span = 10 * time.Second
		t0 := time.Unix(7_000_000, 0)

		// The SAME call Append(Known(5), Unknown(), t) behaves oppositely under
		// two reductions. The gate reads the window's reduction (against) only.

		// Arm A: a DeltaRatio window (against=true) stores NOTHING when against
		// is absent -> empty -> StateAbsent.
		ratio, _ := NewWindow(span, 60*time.Second, DeltaRatio, false)
		ratio.Append(Known(5), Unknown(), t0)
		ratio.Age(t0)
		_, ratioState := ratio.Reduce().Get()
		Expect(ratioState).To(Equal(StateAbsent),
			"under DeltaRatio an absent denominator drops the point; the window is empty")

		// Arm B: a Mean window (against=false) STORES the point — an absent
		// Against is the ordinary single-series case. One entry -> Untrusted
		// (below Mean Min 2).
		mean, _ := NewWindow(span, 60*time.Second, Mean, false)
		mean.Append(Known(5), Unknown(), t0)
		mean.Age(t0)
		_, meanState := mean.Reduce().Get()
		Expect(meanState).To(Equal(StateUntrusted),
			"under Mean an absent denominator is ordinary; the point is stored (one entry, below Min 2)")
	})

	It("should keep a sample landing exactly on the cutoff", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, false)

		t0 := time.Unix(8_000_000, 0)
		w.Append(Known(5), Unknown(), t0)
		w.Age(t0)

		// Advance exactly one span. The entry at t0 now lands on the cutoff
		// (cutoff = now - span = t0); a strict-Before prune keeps it.
		now := t0.Add(span)
		w.Append(Known(6), Unknown(), now)
		w.Age(now)

		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue),
			"the boundary entry at cutoff is kept, so two entries meet Mean Min 2")
		Expect(w.Coverage().Full()).To(BeTrue(),
			"two entries spanning exactly the span read Full")
	})

	It("should store a point whose denominator is present under a ratio reduction, and drop one whose denominator is not a number", func() {
		const span = 10 * time.Second
		t0 := time.Unix(9_000_000, 0)

		// The permissive arm of the gate: a DeltaRatio window with a PRESENT
		// denominator stores its points; two of them meet Min 2 -> StateValue.
		// Removing the store-when-present path would drop these and leave the
		// window empty.
		ratio, _ := NewWindow(span, 60*time.Second, DeltaRatio, false)
		ratio.Append(Known(5), Known(10), t0)
		ratio.Age(t0)
		ratio.Append(Known(6), Known(12), t0.Add(time.Second))
		ratio.Age(t0.Add(time.Second))
		_, presentState := ratio.Reduce().Get()
		Expect(presentState).To(Equal(StateValue),
			"a present denominator stores the point; two points meet DeltaRatio Min 2")

		// A non-finite denominator — NaN or Inf — is not a number and is dropped,
		// mirroring the numerator guard. The window stays empty -> StateAbsent.
		nan, _ := NewWindow(span, 60*time.Second, DeltaRatio, false)
		nan.Append(Known(5), Known(math.NaN()), t0)
		nan.Age(t0)
		_, nanState := nan.Reduce().Get()
		Expect(nanState).To(Equal(StateAbsent),
			"a NaN denominator is not a number; the point is dropped and the window is empty")

		inf, _ := NewWindow(span, 60*time.Second, DeltaRatio, false)
		inf.Append(Known(5), Known(math.Inf(1)), t0)
		inf.Age(t0)
		_, infState := inf.Reduce().Get()
		Expect(infState).To(Equal(StateAbsent),
			"an Inf denominator is not a number; the point is dropped and the window is empty")
	})

	It("should return to a value after a freeze once a successful append lands", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, false)

		t0 := time.Unix(10_000_000, 0)
		w.Append(Known(5), Unknown(), t0)
		w.Age(t0)
		w.Append(Known(6), Unknown(), t0.Add(time.Second))
		w.Age(t0.Add(time.Second))
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue), "the window first reaches a trustworthy state")

		// A failed read freezes the window: nothing appended this tick -> Untrusted,
		// and pruning is held so the entries survive the outage.
		w.Append(Unknown(), Unknown(), t0.Add(2*time.Second))
		w.Age(t0.Add(2 * time.Second))
		_, frozen := w.Reduce().Get()
		Expect(frozen).To(Equal(StateUntrusted), "a failed read leaves the window Untrusted, holding its contents")

		// The next successful append re-stores and re-enables pruning. A
		// regression that pins lastAppendStored false permanently would stay
		// Untrusted here; the window must return to StateValue.
		recover := t0.Add(3 * time.Second)
		w.Append(Known(7), Unknown(), recover)
		w.Age(recover)
		_, recovered := w.Reduce().Get()
		Expect(recovered).To(Equal(StateValue),
			"a successful append after a freeze returns the window to StateValue")
	})

	It("should re-accumulate to a value after a counter restart", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, true)

		t0 := time.Unix(11_000_000, 0)
		w.Append(Known(10), Unknown(), t0)
		w.Age(t0)
		w.Append(Known(12), Unknown(), t0.Add(time.Second))
		w.Age(t0.Add(time.Second))
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue), "two forward samples meet Mean Min 2")

		// A backwards step restarts the window: stored entries discarded, one
		// entry held -> Untrusted.
		w.Append(Known(5), Unknown(), t0.Add(2*time.Second)) // 5 < 12: reset
		w.Age(t0.Add(2 * time.Second))
		_, restarted := w.Reduce().Get()
		Expect(restarted).To(Equal(StateUntrusted), "a counter reset holds one entry (below Min 2)")

		// A third FORWARD sample after the reset accumulates back to two entries
		// -> StateValue. A regression that fails to re-accumulate after a reset
		// (e.g. pinning the window empty) would stay Untrusted here.
		w.Append(Known(8), Unknown(), t0.Add(3*time.Second)) // 8 > 5: forward
		w.Age(t0.Add(3 * time.Second))
		_, recovered := w.Reduce().Get()
		Expect(recovered).To(Equal(StateValue),
			"a forward sample after a counter reset re-accumulates to StateValue")
	})

	It("should treat the first post-demote point as Untrusted below the minimum", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, false)

		t0 := time.Unix(12_000_000, 0)
		w.Append(Known(5), Unknown(), t0)
		w.Age(t0)
		w.Append(Known(6), Unknown(), t0.Add(time.Second))
		w.Age(t0.Add(time.Second))
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue), "two samples meet Mean Min 2")

		// Past the demote span with no successful read, the window empties. The
		// last successful read was at t0+1s, so 61s after it (t0+62s) crosses the
		// 60s demote boundary.
		t62 := t0.Add(62 * time.Second)
		w.Append(Unknown(), Unknown(), t62)
		w.Age(t62)
		_, emptied := w.Reduce().Get()
		Expect(emptied).To(Equal(StateAbsent), "the demote span empties the window")

		// The first successful point after the demote re-stores: one entry below
		// Mean Min 2 -> Untrusted, not Value. A regression that fails to reset
		// accumulation after demote would report Value here.
		t63 := t0.Add(63 * time.Second)
		w.Append(Known(7), Unknown(), t63)
		w.Age(t63)
		_, rebuilt := w.Reduce().Get()
		Expect(rebuilt).To(Equal(StateUntrusted),
			"the first post-demote point is a single entry (below Mean Min 2)")
	})

	It("should prune stale pre-outage points on the recovery tick under the production Age-before-Append order, so a recovered window does not fold them as trusted", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, false)
		t0 := time.Unix(30_000_000, 0)

		// Production order is Age THEN Append, each tick (not the reversed
		// order the other window specs use, which masks this).
		w.Age(t0)
		w.Append(Known(5), Unknown(), t0)
		w.Age(t0.Add(time.Second))
		w.Append(Known(6), Unknown(), t0.Add(time.Second))

		// An outage: Age then a failed Append on each tick. The window holds.
		for i := 2; i <= 6; i++ {
			at := t0.Add(time.Duration(i) * time.Second)
			w.Age(at)
			w.Append(Unknown(), Unknown(), at)
		}

		// Recovery at 20s: Age runs first (freezes on the prior tick's failure and
		// does not prune), then a successful Append(100) lands. The fresh point
		// must prune the stale out-of-span 5 and 6, so the window reports 100,
		// not the folded mean 37.
		rec := t0.Add(20 * time.Second)
		w.Age(rec)
		w.Append(Known(100), Unknown(), rec)

		v, st := w.Reduce().Get()
		// The stale 5 and 6 are pruned, so the mean is 100 — not the folded 37 the
		// bug produced. One recovered point is still below Mean's minimum of 2
		// (honest-Untrusted), whereas the bug reported a wrong TRUSTED 37.
		Expect(v).To(Equal(100.0),
			"the recovery append prunes the stale out-of-span points; a build that folds them reports a wrong mean of 37")
		Expect(st).To(Equal(StateUntrusted),
			"one recovered point is below Mean's minimum of 2: honest-Untrusted, not a folded trusted value")
	})

	It("should not restart a counter window on an equal value — only a strict fall resets", func() {
		const span = 10 * time.Second
		w, _ := NewWindow(span, 60*time.Second, Mean, true)

		t0 := time.Unix(20_000_000, 0)
		w.Append(Known(10), Unknown(), t0)
		w.Age(t0)
		// An EQUAL value is not a backwards step: a monotone counter reporting the
		// same value twice must not reset. A build that uses <= would empty on
		// every unchanged counter reading.
		w.Append(Known(10), Unknown(), t0.Add(time.Second))
		w.Age(t0.Add(time.Second))

		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue),
			"an equal value is not a backwards step; the counter window keeps both entries and meets Mean Min 2")
	})

	It("should not demote at exactly the demote span — only strictly past it", func() {
		const span = 10 * time.Second
		const demote = 60 * time.Second
		w, _ := NewWindow(span, demote, Mean, false)

		t0 := time.Unix(21_000_000, 0)
		w.Append(Known(5), Unknown(), t0)
		w.Age(t0)
		w.Append(Known(6), Unknown(), t0.Add(time.Second))
		w.Age(t0.Add(time.Second))
		_, state := w.Reduce().Get()
		Expect(state).To(Equal(StateValue), "the window first reaches a trustworthy state")

		// A failed read begins the freeze (no pruning), so the two entries survive
		// past the span and the demote boundary becomes observable in its own right
		// rather than being masked by span-pruning.
		w.Append(Unknown(), Unknown(), t0.Add(2*time.Second))
		w.Age(t0.Add(2 * time.Second))

		// Exactly one demote span after the last successful read (t0+1s), the window
		// is frozen-HOLDING (not emptied): now.Sub(lastSuccess) == demote, and the
		// demote empties only STRICTLY past the span.
		exact := t0.Add(time.Second).Add(demote)
		w.Age(exact)
		_, atBoundary := w.Reduce().Get()
		Expect(atBoundary).To(Equal(StateUntrusted),
			"at exactly the demote span the frozen window holds (Untrusted), not emptied")

		// One tick past the boundary, the window empties.
		w.Age(exact.Add(time.Second))
		_, pastBoundary := w.Reduce().Get()
		Expect(pastBoundary).To(Equal(StateAbsent),
			"just past the demote span the window empties")
	})
})
