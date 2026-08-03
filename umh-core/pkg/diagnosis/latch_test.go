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
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The latch: a two-mark Schmitt pair keyed per signal, with a coverage-gated
// clear arm, a span-based re-fire arm, an immediate Reset, and a demote clock
// that measures ReleaseAfter from the last Update.
//
// These specs drive the latch through its exported surface only — Update,
// Reset, ReleaseAfter, Fired — and hand it Reduced and Coverage values built
// directly, because the latch must derive everything from those two and must
// never see a readability fact. Reduced.v/.state and Coverage.span/.spanned
// are reachable here because this file lives in package diagnosis; the
// external access spec (reduced_access_test.go) proves they are not reachable
// from outside. The structural shape of Reduced, Coverage and Update is pinned
// independently in latch_spec6_test.go and must stay green.
var _ = Describe("Latch", func() {
	const latchSpan = 60 * time.Second

	march := func() Marks {
		return Marks{
			Fire:     Mark{At: 0.10, Inclusive: false},
			Clear:    Mark{At: 0.06, Inclusive: false},
			Polarity: HigherIsWorse,
		}
	}
	full := func() Coverage { return Coverage{span: latchSpan, spanned: latchSpan} }
	short := func() Coverage { return Coverage{span: latchSpan, spanned: 30 * time.Second} }

	It("should fire and clear at the marks with the inclusivity each mark declares, and hold between them", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		// An exclusive fire mark is not crossed by a value exactly at it.
		l.Update(Reduced{v: 0.10, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeFalse(), "an exclusive fire mark is not crossed by a value exactly at it")

		// A value strictly above the fire mark fires.
		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a value strictly above the fire mark fires the latch")

		// A value between the clear and fire marks holds a fired latch.
		l.Update(Reduced{v: 0.08, state: StateValue}, full(), march(), t0.Add(2*time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a value between the marks holds the fired latch")

		// An exclusive clear mark is not crossed by a value exactly at it.
		l.Update(Reduced{v: 0.06, state: StateValue}, full(), march(), t0.Add(3*time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "an exclusive clear mark is not crossed by a value exactly at it")

		// A value strictly below the clear mark, with full coverage, clears.
		l.Update(Reduced{v: 0.02, state: StateValue}, full(), march(), t0.Add(4*time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "a value strictly below the clear mark clears the fired latch")
	})

	It("should not fire an unfired latch on a value strictly between the clear and fire marks", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		// 0.08 is strictly between clear (0.06) and fire (0.10): it crosses neither
		// mark, so an unfired latch must stay unfired. The fire arm must be gated on
		// crossing the fire mark, not on any trustworthy value at all.
		l.Update(Reduced{v: 0.08, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeFalse(), "a value strictly between the marks does not fire an unfired latch")
	})

	It("should carry the identity the engine stamped at construction into the Fired it reports", func() {
		t0 := time.Unix(1_000_000, 0)
		id := Identity{Signal: "sig", Tier: 2, External: true, Index: 7}
		l := NewLatch(id)

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		f, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")
		Expect(f.Identity).To(Equal(id),
			"Fired carries the identity the engine stamped at construction, verbatim")
	})

	It("should carry the value it fired under and the time it fired", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{Signal: "sig", Tier: 1})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		f, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")
		Expect(f.Value).To(Equal(0.20),
			"Fired carries the value the latch fired under, for severity ranking against the marks")
		Expect(f.Since.Equal(t0)).To(BeTrue(),
			"Fired carries the transition time, stamped on fire and not re-stamped per tick")
	})

	It("should hold a fired latch when its reduction is untrustworthy", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")

		// An untrustworthy reduction — even one carrying a number below the clear
		// mark — must not clear a fired latch: the clear arm requires a
		// trustworthy (StateValue) reduction.
		l.Update(Reduced{v: 0.02, state: StateUntrusted}, full(), march(), t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "an untrustworthy reduction holds the fired latch rather than clearing it")
	})

	It("should not clear a fired latch until its window spans the full window duration", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")

		// A below-clear value with NON-full coverage holds: the clear arm is gated
		// on Coverage.Full(), and a window ageing without appending reports short
		// coverage.
		l.Update(Reduced{v: 0.02, state: StateValue}, short(), march(), t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a below-clear value does not clear a latch whose window does not span the full duration")

		// Once the window spans the full duration, the same below-clear value clears.
		l.Update(Reduced{v: 0.02, state: StateValue}, full(), march(), t0.Add(2*time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "a below-clear value clears once coverage is full")
	})

	It("should release a fired latch the moment it is reset, whatever its coverage, because the clear arm's coverage gate does not apply to a release", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")

		// The clear arm is otherwise holding the latch: a below-clear value on
		// NON-full coverage does not release.
		l.Update(Reduced{v: 0.02, state: StateValue}, short(), march(), t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "the additional below-clear value on non-full coverage holds the latch")

		// Reset must release immediately even though coverage is not full. A Reset
		// routed through the coverage-gated clear arm would refuse and never
		// release. Reset takes the same injected clock as every sibling arm, so the
		// re-fire window it opens is measured in test time, never the real wall
		// clock.
		l.Reset(t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "Reset releases a fired latch whatever its coverage")
	})

	It("should measure the re-fire window from the injected reset time, so a reset never permanently blocks re-firing", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")

		// Reset at an injected now rather than the wall clock, so the re-fire
		// window that follows is visible to the test clock.
		tReset := t0.Add(time.Second)
		l.Reset(tReset)
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "Reset releases the fired latch at the injected time")

		// Before one full window has elapsed since the reset, the same above-fire
		// value must not re-fire.
		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), tReset.Add(latchSpan-time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "a reset latch does not re-fire before one full window has elapsed since the reset")

		// Once one full window has elapsed since the reset, it fires again.
		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), tReset.Add(latchSpan))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a reset latch re-fires once one full window has elapsed since the reset")
	})

	It("should not fire again until one full window has elapsed since the release", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		// Fire, then clear with full coverage — the release happens on the
		// clearing Update at tClear, and the re-fire bar counts one full window
		// (Coverage.Span) from that release.
		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		tClear := t0.Add(time.Second)
		l.Update(Reduced{v: 0.02, state: StateValue}, full(), march(), tClear)
		_, fired := l.Fired()
		Expect(fired).To(BeFalse(), "a below-clear value with full coverage clears the fired latch")

		// Before one full window has elapsed since the release, the same above-fire
		// value must not re-fire.
		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), tClear.Add(latchSpan-time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "a value above the fire mark does not re-fire before one full window has elapsed since the release")

		// Once one full window has elapsed (now >= release + span), it fires again.
		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), tClear.Add(latchSpan))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark re-fires once one full window has elapsed since the release")
	})

	It("should not discard a fired latch merely because a different instrument became usable", func() {
		t0 := time.Unix(1_000_000, 0)
		ratio := Marks{
			Fire:     Mark{At: 0.10, Inclusive: false},
			Clear:    Mark{At: 0.06, Inclusive: false},
			Polarity: HigherIsWorse,
			Unit:     "ratio",
			Capacity: 1.0,
		}
		cores := Marks{
			Fire:     Mark{At: 0, Inclusive: false},
			Clear:    Mark{At: 0.5, Inclusive: false},
			Polarity: LowerIsWorse,
			Unit:     "cores",
			Capacity: 4.0,
		}
		l := NewLatch(Identity{})

		// Fire under the first pair with a value above its fire mark.
		l.Update(Reduced{v: 0.20, state: StateValue}, full(), ratio, t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the first mark pair's fire mark fires")

		// The mark pair changes on the next Update (a different instrument became
		// usable). Under cores (LowerIsWorse, Fire At 0, Clear At 0.5), worse(0.3)
		// = -0.3 is not above worse(Fire.At) = 0 (so 0.3 does not fire) and not
		// below worse(Clear.At) = -0.5 (so it does not clear): 0.3 falls in the
		// between-marks hold band, and the new pair says HOLD. The latch must
		// hold, not reset, and the Fired it returns carries the new marks.
		l.Update(Reduced{v: 0.3, state: StateValue}, full(), cores, t0.Add(time.Second))
		f, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a changed mark pair holds the fired latch rather than resetting it")
		Expect(f.Marks).To(Equal(cores), "the fired latch reports the current mark pair, not the one it fired under")

		// The structural backstop, because a behaviour can be re-broken and a
		// method set cannot. The method set is exactly four — Fired, ReleaseAfter,
		// Reset, Update — and the moment a method is added that lets the latch act
		// on a changed instrument (one that would reset a held latch) this goes
		// red.
		lt := reflect.TypeOf(&Latch{})
		Expect(lt.NumMethod()).To(Equal(4),
			"the method set is exactly four; a method that resets a held latch on an instrument change would add a fifth")
		for _, name := range []string{"Fired", "ReleaseAfter", "Reset", "Update"} {
			_, ok := lt.MethodByName(name)
			Expect(ok).To(BeTrue(), "the method set must expose "+name)
		}
	})

	It("should not repaint the reported mark pair on an untrusted reduction", func() {
		t0 := time.Unix(1_000_000, 0)
		ratio := Marks{
			Fire:     Mark{At: 0.10, Inclusive: false},
			Clear:    Mark{At: 0.06, Inclusive: false},
			Polarity: HigherIsWorse,
			Unit:     "ratio",
			Capacity: 1.0,
		}
		cores := Marks{
			Fire:     Mark{At: 0, Inclusive: false},
			Clear:    Mark{At: 0.5, Inclusive: false},
			Polarity: LowerIsWorse,
			Unit:     "cores",
			Capacity: 4.0,
		}
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), ratio, t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the first mark pair's fire mark fires")

		// An untrusted reduction carrying the other instrument's marks must not
		// repaint the reported pair: marks and value are gated on trustworthiness
		// together, so the Fired still reports ratio's pair, not cores'.
		l.Update(Reduced{v: 0.3, state: StateUntrusted}, full(), cores, t0.Add(time.Second))
		f, fired := l.Fired()
		Expect(fired).To(BeTrue(), "an untrusted reduction holds the fired latch")
		Expect(f.Marks).To(Equal(ratio), "an untrusted reduction does not repaint the reported mark pair")
	})

	It("should include an inclusive mark at the exact boundary", func() {
		t0 := time.Unix(1_000_000, 0)
		// Inclusivity is part of the mark: whether the boundary is crossed at
		// exactly the threshold is a property of the mark itself. These drive both
		// operators at the exact point.
		m := Marks{
			Fire:     Mark{At: 0.10, Inclusive: true},
			Clear:    Mark{At: 0.06, Inclusive: true},
			Polarity: HigherIsWorse,
		}
		l := NewLatch(Identity{})

		// The inclusive fire mark is crossed by a value exactly at it.
		l.Update(Reduced{v: 0.10, state: StateValue}, full(), m, t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "an inclusive fire mark is crossed by a value exactly at it")

		// The inclusive clear mark is crossed by a value exactly at it, with full
		// coverage.
		l.Update(Reduced{v: 0.06, state: StateValue}, full(), m, t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "an inclusive clear mark is crossed by a value exactly at it")
	})

	It("should include an inclusive mark at the exact boundary under LowerIsWorse too", func() {
		t0 := time.Unix(1_000_000, 0)
		// Under LowerIsWorse worse(v) = -v, so the inclusive arms are sign-flipped
		// as well: fire when -v >= -Fire.At and clear when -v <= -Clear.At. The
		// HigherIsWorse inclusive spec above never drives the sign-flipped
		// operators, so pin them here at the exact marks.
		m := Marks{
			Fire:     Mark{At: 0.5, Inclusive: true},
			Clear:    Mark{At: 0.9, Inclusive: true},
			Polarity: LowerIsWorse,
		}
		l := NewLatch(Identity{})

		// The inclusive LowerIsWorse fire mark is crossed by a value exactly at it.
		l.Update(Reduced{v: 0.5, state: StateValue}, full(), m, t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "an inclusive LowerIsWorse fire mark is crossed by a value exactly at it")

		// The inclusive LowerIsWorse clear mark is crossed by a value exactly at
		// it, with full coverage.
		l.Update(Reduced{v: 0.9, state: StateValue}, full(), m, t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "an inclusive LowerIsWorse clear mark is crossed by a value exactly at it")
	})

	It("should fire and clear under LowerIsWorse, where a LOWER value is the worse side", func() {
		t0 := time.Unix(1_000_000, 0)
		// LowerIsWorse maps worse(v) = -v, so a value below the fire mark fires and
		// one above the clear mark clears — the sign-flipped arm. This positively
		// controls the worse() fire and clear paths the ratio-only specs never touch.
		m := Marks{
			Fire:     Mark{At: 0.5, Inclusive: false},
			Clear:    Mark{At: 0.9, Inclusive: false},
			Polarity: LowerIsWorse,
		}
		l := NewLatch(Identity{})

		// A value below the fire mark (lower is worse) fires.
		l.Update(Reduced{v: 0.4, state: StateValue}, full(), m, t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value below a LowerIsWorse fire mark fires")

		// A value between the marks holds.
		l.Update(Reduced{v: 0.7, state: StateValue}, full(), m, t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a value between LowerIsWorse marks holds the fired latch")

		// A value above the clear mark (away from the worse side) clears.
		l.Update(Reduced{v: 1.0, state: StateValue}, full(), m, t0.Add(2*time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "a value above a LowerIsWorse clear mark clears the fired latch")
	})

	It("should release on the demote clock measured from the last Update, even when the release calls are interleaved with ticks that do not release", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		// Fire on a single Update at t0. That Update is the latch's only clock
		// origin; the demote clock runs from it.
		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")

		// Alternating ticks: ReleaseAfter on odd seconds, nothing on even ones,
		// and never another Update. A latch that restarts its clock per
		// ReleaseAfter call never reaches the bar and never releases.
		for i := 1; i < 60; i++ {
			if i%2 == 1 {
				l.ReleaseAfter(latchSpan, t0.Add(time.Duration(i)*time.Second))
			}
		}
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "the latch still holds this side of one full window")

		// The clock runs from the last Update at t0, so at t0+span the latch
		// releases even though the most recent ReleaseAfter call was only at t0+59.
		l.ReleaseAfter(latchSpan, t0.Add(latchSpan))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "the latch releases when now reaches one full window after the last Update")
	})

	It("should not advance the demote clock on an untrusted tick", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		// Fire on a trustworthy Update at t0; that tick anchors the demote clock.
		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")

		// An untrusted tick returns before the clock write, so the demote clock
		// stays anchored at the StateValue tick.
		l.Update(Reduced{v: 0.02, state: StateUntrusted}, full(), march(), t0.Add(time.Second))

		// At one full window after the StateValue tick — which is before one full
		// window after the untrusted tick, had it advanced the clock — the latch
		// still holds.
		l.ReleaseAfter(latchSpan, t0.Add(latchSpan-time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(),
			"an untrusted tick does not advance the demote clock, so the latch holds this side of a full window from the last trustworthy tick")

		l.ReleaseAfter(latchSpan, t0.Add(latchSpan))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "the latch releases one full window after the last trustworthy tick")
	})

	It("should advance the demote clock on a trustworthy between-marks hold", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")

		// A trustworthy between-marks value at t0+1s is an Update and so re-anchors
		// the demote clock, even though it only holds the fired latch.
		l.Update(Reduced{v: 0.08, state: StateValue}, full(), march(), t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a trustworthy between-marks value holds the fired latch")

		// At t0+span the latch still holds: the clock runs from the hold tick at
		// t0+1s, not from the firing tick at t0.
		l.ReleaseAfter(latchSpan, t0.Add(latchSpan))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(),
			"a trustworthy between-marks hold advances the demote clock, so the latch still holds before one full window after the hold tick")

		// One full window after the hold tick it releases.
		l.ReleaseAfter(latchSpan, t0.Add(time.Second).Add(latchSpan))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "the latch releases one full window after the last trustworthy between-marks tick")
	})
})
