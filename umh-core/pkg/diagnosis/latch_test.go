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

// These specs drive the latch through Update, Reset, ReleaseAfter and Fired,
// handing it Reduced and Coverage values built here, because the latch must
// derive everything from those two and must never see a readability fact.
// Their unexported fields are reachable from inside the package;
// reduced_access_test.go proves they are not from outside, and
// latch_spec6_test.go pins the shape of Reduced, Coverage and Update.
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

	// Two mark pairs for one signal, in different units and opposite polarities:
	// what an instrument change hands the latch.
	ratio := func() Marks {
		return Marks{
			Fire:     Mark{At: 0.10, Inclusive: false},
			Clear:    Mark{At: 0.06, Inclusive: false},
			Polarity: HigherIsWorse,
			Unit:     "ratio",
			Worst:    1.0,
		}
	}
	cores := func() Marks {
		return Marks{
			Fire:     Mark{At: 0, Inclusive: false},
			Clear:    Mark{At: 0.5, Inclusive: false},
			Polarity: LowerIsWorse,
			Unit:     "cores",
			Worst:    -4.0,
		}
	}

	It("should fire and clear at the marks with the inclusivity each mark declares, and hold between them", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.10, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeFalse(), "an exclusive fire mark is not crossed by a value exactly at it")

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a value strictly above the fire mark fires the latch")

		l.Update(Reduced{v: 0.08, state: StateValue}, full(), march(), t0.Add(2*time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a value between the marks holds the fired latch")

		l.Update(Reduced{v: 0.06, state: StateValue}, full(), march(), t0.Add(3*time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "an exclusive clear mark is not crossed by a value exactly at it")

		l.Update(Reduced{v: 0.02, state: StateValue}, full(), march(), t0.Add(4*time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "a value strictly below the clear mark clears the fired latch")
	})

	It("should not fire an unfired latch on a value strictly between the clear and fire marks", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		// 0.08 lies strictly between clear (0.06) and fire (0.10), so the fire arm
		// must be gated on crossing the fire mark, not on any trustworthy value.
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

		// 0.02 is below the clear mark, so only its untrustworthiness can stop the
		// clear arm.
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

		// short() is what a window ageing without appending reports.
		l.Update(Reduced{v: 0.02, state: StateValue}, short(), march(), t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a below-clear value does not clear a latch whose window does not span the full duration")

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

		// Establish that the clear arm is otherwise holding the latch.
		l.Update(Reduced{v: 0.02, state: StateValue}, short(), march(), t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "the additional below-clear value on non-full coverage holds the latch")

		// A Reset routed through the coverage-gated clear arm would refuse here
		// and never release.
		l.Reset()
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "Reset releases a fired latch whatever its coverage")
	})

	It("should re-fire from the last trusted update after a reset, so a reset never permanently blocks re-firing", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")

		// Reset takes no clock, so it anchors the re-fire bar at the last trusted
		// Update (the firing one at t0), never at the wall clock.
		l.Reset()
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "Reset releases the fired latch")

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0.Add(latchSpan-time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "a reset latch does not re-fire before one full window has elapsed since the last update")

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0.Add(latchSpan))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a reset latch re-fires once one full window has elapsed since the last update")
	})

	It("should not fire again until one full window has elapsed since the release", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		// Here the release happens on the clearing Update at tClear, and the
		// re-fire bar counts one Coverage.Span from that release.
		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		tClear := t0.Add(time.Second)
		l.Update(Reduced{v: 0.02, state: StateValue}, full(), march(), tClear)
		_, fired := l.Fired()
		Expect(fired).To(BeFalse(), "a below-clear value with full coverage clears the fired latch")

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), tClear.Add(latchSpan-time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "a value above the fire mark does not re-fire before one full window has elapsed since the release")

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), tClear.Add(latchSpan))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark re-fires once one full window has elapsed since the release")
	})

	It("should not discard a fired latch merely because a different instrument became usable", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), ratio(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the first mark pair's fire mark fires")

		// The marks change because a different instrument became usable. Under
		// cores, worse(0.3) = -0.3 is neither above worse(Fire.At) = 0 nor below
		// worse(Clear.At) = -0.5, so 0.3 lands in the hold band and the new pair
		// says HOLD.
		l.Update(Reduced{v: 0.3, state: StateValue}, full(), cores(), t0.Add(time.Second))
		f, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a changed mark pair holds the fired latch rather than resetting it")
		Expect(f.Marks).To(Equal(ratio()), "the held verdict keeps the pair its value fired against, not the pair of an instrument that never measured it")

		// A structural backstop, because a behaviour can be re-broken and a method
		// set cannot: adding a method that lets the latch act on an instrument
		// change turns this red.
		lt := reflect.TypeOf(&Latch{})
		Expect(lt.NumMethod()).To(Equal(4),
			"the method set is exactly four; a method that resets a held latch on an instrument change would add a fifth")
		for _, name := range []string{"Fired", "ReleaseAfter", "Reset", "Update"} {
			_, ok := lt.MethodByName(name)
			Expect(ok).To(BeTrue(), "the method set must expose "+name)
		}
	})

	It("should score a held latch against the mark pair it fired under, not a later instrument's", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), ratio(), t0)
		l.Update(Reduced{v: 0.3, state: StateValue}, full(), cores(), t0.Add(time.Second))

		f, fired := l.Fired()
		Expect(fired).To(BeTrue(), "the changed mark pair holds the fired latch")
		// 0.20 fired a tenth of the way up the ratio pair's 0.10-to-1.0 span. Scored
		// against the cores pair instead, a ratio of 0.20 sits on the safe side of a
		// fire mark of 0 cores and clamps to 0, sinking the cause to the bottom of
		// its tier.
		Expect(f.Severity()).To(BeNumerically("~", 1.0/9.0, 1e-12),
			"severity divides the value that fired by the span it fired across, both from the same instrument")
	})

	It("should not repaint the reported mark pair on an untrusted reduction", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), ratio(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the first mark pair's fire mark fires")

		// The untrusted half of the same invariant: a tick that cannot move the
		// latch cannot move the pair it reports either.
		l.Update(Reduced{v: 0.3, state: StateUntrusted}, full(), cores(), t0.Add(time.Second))
		f, fired := l.Fired()
		Expect(fired).To(BeTrue(), "an untrusted reduction holds the fired latch")
		Expect(f.Marks).To(Equal(ratio()), "an untrusted reduction does not repaint the reported mark pair")
	})

	It("should include an inclusive mark at the exact boundary", func() {
		t0 := time.Unix(1_000_000, 0)
		// Inclusivity is a property of the mark itself, so both operators are
		// driven at exactly the threshold.
		m := Marks{
			Fire:     Mark{At: 0.10, Inclusive: true},
			Clear:    Mark{At: 0.06, Inclusive: true},
			Polarity: HigherIsWorse,
		}
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.10, state: StateValue}, full(), m, t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "an inclusive fire mark is crossed by a value exactly at it")

		l.Update(Reduced{v: 0.06, state: StateValue}, full(), m, t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "an inclusive clear mark is crossed by a value exactly at it")
	})

	It("should include an inclusive mark at the exact boundary under LowerIsWorse too", func() {
		t0 := time.Unix(1_000_000, 0)
		// Under LowerIsWorse worse(v) = -v, so the inclusive arms are sign-flipped:
		// fire when -v >= -Fire.At, clear when -v <= -Clear.At. The HigherIsWorse
		// inclusive spec above never drives those.
		m := Marks{
			Fire:     Mark{At: 0.5, Inclusive: true},
			Clear:    Mark{At: 0.9, Inclusive: true},
			Polarity: LowerIsWorse,
		}
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.5, state: StateValue}, full(), m, t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "an inclusive LowerIsWorse fire mark is crossed by a value exactly at it")

		l.Update(Reduced{v: 0.9, state: StateValue}, full(), m, t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "an inclusive LowerIsWorse clear mark is crossed by a value exactly at it")
	})

	It("should fire and clear under LowerIsWorse, where a LOWER value is the worse side", func() {
		t0 := time.Unix(1_000_000, 0)
		// The positive control for the sign-flipped fire and clear paths the
		// ratio-only specs never touch.
		m := Marks{
			Fire:     Mark{At: 0.5, Inclusive: false},
			Clear:    Mark{At: 0.9, Inclusive: false},
			Polarity: LowerIsWorse,
		}
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.4, state: StateValue}, full(), m, t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value below a LowerIsWorse fire mark fires")

		l.Update(Reduced{v: 0.7, state: StateValue}, full(), m, t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a value between LowerIsWorse marks holds the fired latch")

		l.Update(Reduced{v: 1.0, state: StateValue}, full(), m, t0.Add(2*time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "a value above a LowerIsWorse clear mark clears the fired latch")
	})

	It("should release on the demote clock measured from the last Update, even when the release calls are interleaved with ticks that do not release", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		// The Update at t0 is the only clock origin in this spec.
		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")

		// ReleaseAfter on odd seconds, nothing on even ones, and never another
		// Update: a latch that restarted its clock per ReleaseAfter call would
		// never reach the bar.
		for i := 1; i < 60; i++ {
			if i%2 == 1 {
				l.ReleaseAfter(latchSpan, t0.Add(time.Duration(i)*time.Second))
			}
		}
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "the latch still holds this side of one full window")

		// t0+span is one window after the Update, though the previous call was at
		// t0+59s.
		l.ReleaseAfter(latchSpan, t0.Add(latchSpan))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "the latch releases when now reaches one full window after the last Update")
	})

	It("should not advance the demote clock on an untrusted tick", func() {
		t0 := time.Unix(1_000_000, 0)
		l := NewLatch(Identity{})

		l.Update(Reduced{v: 0.20, state: StateValue}, full(), march(), t0)
		_, fired := l.Fired()
		Expect(fired).To(BeTrue(), "a value above the fire mark fires")

		// An untrusted tick returns before the clock write.
		l.Update(Reduced{v: 0.02, state: StateUntrusted}, full(), march(), t0.Add(time.Second))

		// One second short of a window after t0, but a full window after the
		// untrusted tick, had that advanced the clock.
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

		// A trustworthy between-marks value is still an Update, so it re-anchors
		// the clock even though it only holds the latch.
		l.Update(Reduced{v: 0.08, state: StateValue}, full(), march(), t0.Add(time.Second))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(), "a trustworthy between-marks value holds the fired latch")

		// t0+span is a full window after the FIRING tick but not after the hold
		// tick at t0+1s.
		l.ReleaseAfter(latchSpan, t0.Add(latchSpan))
		_, fired = l.Fired()
		Expect(fired).To(BeTrue(),
			"a trustworthy between-marks hold advances the demote clock, so the latch still holds before one full window after the hold tick")

		l.ReleaseAfter(latchSpan, t0.Add(time.Second).Add(latchSpan))
		_, fired = l.Fired()
		Expect(fired).To(BeFalse(), "the latch releases one full window after the last trustworthy between-marks tick")
	})
})
