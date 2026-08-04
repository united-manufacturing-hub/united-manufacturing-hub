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
	"fmt"
	"math"
	"time"
)

// Coverage is what a window can say about its own extent: the span it was built
// with, and the time actually spanned by the entries it now holds.
//
// The latch needs both and can derive neither from a Reduced. Its clear arm is
// gated on full coverage and its re-fire arm on the span itself, so a Latch
// handed only a number cannot implement either.
//
// Coverage is NOT a readability fact and must never be made into one. It says
// how much time the stored entries span; a window frozen through an outage still
// spans its full 60s. The latch derives its state only from the reduction,
// never from a readability flag, which is why Coverage is two durations and
// nothing else.
type Coverage struct {
	span    time.Duration
	spanned time.Duration
}

// Full reports whether the stored entries span the whole window duration.
func (c Coverage) Full() bool { return c.span > 0 && c.spanned >= c.span }

// Window is a time-bounded ring of readings for one (signal, instrument) pair.
type Window struct {
	lastSuccess      time.Time
	points           []Point
	red              Reduction
	span             time.Duration
	demote           time.Duration
	counter          bool
	lastAppendStored bool
}

// NewWindow builds a window from four facts, and all four are separate.
//
// The demote span is the signal's, not the window's: StateAbsent is defined as
// empty OR newest entry older than the demote span, and freeze-then-empty ends
// at the same boundary, so a window that is not told it cannot report absence at
// all. A window whose span is six hours and whose demote span is 60s must empty
// after 60s. Both are 60s for every consumer signal, so a build that passes one
// value twice is green across all 33 scenarios — the test that proves they are
// distinct needs spans that differ.
//
// counter is Instrument.Counter, carried through because the restart rule is
// the window's to apply — only the window holds the previous Point. It is NOT
// derivable here: a window sees floats, and a ratio that legitimately falls and
// a counter that reset look identical.
//
// It refuses a span or a demote span that is zero or negative (S1 R8).
func NewWindow(span, demote time.Duration, red Reduction, counter bool) (*Window, error) {
	if span <= 0 {
		return nil, fmt.Errorf("window: span %v is not positive", span)
	}
	if demote <= 0 {
		return nil, fmt.Errorf("window: demote span %v is not positive", demote)
	}
	return &Window{span: span, demote: demote, red: red, counter: counter}, nil
}

// Age prunes entries older than the span. Called on EVERY tick, including ticks
// where the read failed — a failed read appends nothing, a failed tick still
// ages. A full ring holds span+1 entries: the prune keeps a sample landing
// exactly on the cutoff.
func (w *Window) Age(now time.Time) {
	// Demote: once no successful read has landed for longer than the demote
	// span, the window empties regardless of any freeze.
	if !w.lastSuccess.IsZero() && now.Sub(w.lastSuccess) > w.demote {
		w.points = nil

		return
	}
	// Freeze: while the most recent read failed, hold the last known contents
	// so the window does not shrink or empty through the outage.
	if !w.lastAppendStored {
		return
	}
	// Prune entries older than the span, keeping a sample landing exactly on
	// the cutoff.
	w.prune(now.Add(-w.span))
}

// prune drops entries older than the cutoff, keeping a sample landing exactly on
// it. Entries are appended in time order, so it drops from the front.
func (w *Window) prune(cutoff time.Time) {
	i := 0
	for i < len(w.points) && w.points[i].At.Before(cutoff) {
		i++
	}
	w.points = w.points[i:]
}

// Append stores one instant's value, and gates on the denominator reading for a
// delta-ratio reduction. It gates on the Readings internally — an absent value
// appends nothing, and an absent or non-finite denominator appends nothing when
// the window's own reduction declares against. Callers must NOT pre-filter, or
// "absent is not zero" becomes every caller's responsibility again.
//
// Append must be called on every tick the engine advances the window, passing
// Unknown() for a failed read. The window distinguishes a failed tick (frozen,
// Untrusted) from a tick it was not polled only if every tick calls Append; a
// tick with no call leaves the prior tick's stored/failed flag in place and the
// window silently reports stale state.
//
// The denominator gate reads the window's reduction and nothing else. Under
// Mean an absent Against is the ordinary case — six of the seven instruments
// fold a single series and pass Unknown() — and the point is stored. Under DeltaRatio the same absence makes the point unusable and it is
// dropped. Same two arguments, opposite outcome, and Reduction.against is the
// only thing that separates them.
//
// One call carries both counters and one timestamp, so a delta ratio can never
// be built from two counters read at two different moments.
//
// On a COUNTER window only, a backwards step discards the stored entries and
// starts over from this one. A backwards step is a value below the previous
// entry's, or — when the window also holds a denominator — a denominator below
// the previous entry's: a monotone counter that fell was reset at the source,
// so a delta taken across the reset is arithmetic on two different origins. On
// a window that is not a counter the same fall is the quantity doing what it is
// supposed to do, and restarting there empties the window on every dip: a
// percentile fallback would never reach its floor and a mean fallback would
// restart on every decrease. The rule is gated on the declaration, never applied
// to every window.
func (w *Window) Append(value, against Reading, at time.Time) {
	w.lastAppendStored = false

	v, vok := value.Get()
	// An absent value, or a value that is not a number, appends nothing.
	if !vok || math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	// The denominator gate: a reduction that divides by a denominator drops the
	// point when the denominator is absent or not a number.
	if w.red.against {
		a, aok := against.Get()
		// A denominator that is absent, or a value that is not a number,
		// appends nothing — mirroring the numerator guard.
		if !aok || math.IsNaN(a) || math.IsInf(a, 0) {
			return
		}
	}

	// Counter restart: a backwards step in either series discards the stored
	// entries and starts over from this one — a source reset moves both counters
	// at once, so a fall in either means the prior entries belong to a different
	// origin and a delta across the reset is arithmetic on two. A window whose
	// denominator is absent has no second series, so only the value fall matters.
	//
	// The denominator fall is examined only when the window's own reduction
	// divides by the denominator; on any other counter window the argument is
	// absent or meaningless, and a fall in it must not wipe the window. Both
	// readings must be present — an absent denominator, which stores as zero,
	// must never order or restart the window.
	if w.counter && len(w.points) > 0 {
		prev := w.points[len(w.points)-1]

		restart := v < prev.Value
		if w.red.against {
			a, aok := against.Get()
			pa, paok := prev.Against.Get()
			if aok && paok && a < pa {
				restart = true
			}
		}

		if restart {
			w.points = nil
		}
	}

	w.points = append(w.points, Point{At: at, Value: v, Against: against})
	w.lastSuccess = at
	w.lastAppendStored = true
	// A successful store prunes to the span even when this tick began frozen.
	// Production order is Age-then-Append: through an outage Age froze on the
	// prior tick's failed append and did not prune, so without this the stale
	// >span points would fold into the first post-recovery reduction as if
	// trusted.
	w.prune(at.Add(-w.span))
}

// Reduce folds the window under its own reduction. It takes no argument: a
// window is 1:1 with an instrument and is handed its reduction once. Passing the
// instrument back in permits reducing a window under a reduction that is not its
// own, and both types check, so nothing catches the mismatch.
//
// The reduced number v is computed by the window's own fold and is carried under
// StateValue; a below-minimum or stale StateUntrusted also carries the folded
// number. Only an empty window (StateAbsent) and an against reduction whose
// denominator delta is zero (StateUntrusted) void it — both carry v as zero. The
// outcome — StateAbsent, StateUntrusted, or StateValue — is returned with the
// number, never alone.
//
// Reduce must be called only after Age on the same tick. The window does not
// verify recency itself; a Reduce without a preceding Age reports stale entries
// as trusted.
func (w *Window) Reduce() Reduced {
	// Empty — or demoted, which Age already turned into empty — is absence.
	if len(w.points) == 0 {
		return Reduced{state: StateAbsent}
	}
	// The fold divides by a denominator whose delta is zero or negative — the
	// counter did not move, or it reset backwards — so the number cannot be
	// computed: the outcome is untrusted and the number carries zero.
	if w.red.against && denominatorDelta(w.points) <= 0 {
		return Reduced{v: 0, state: StateUntrusted}
	}
	// A window whose reduction has no fold — a bare Reduction{} literal handed
	// to NewWindow directly — cannot compute a number. Treat it as untrusted
	// rather than panicking; NewEngine refuses this shape at construction so
	// this is a defensive backstop, not a supported route.
	if w.red.fold == nil {
		return Reduced{v: 0, state: StateUntrusted}
	}

	v := w.red.fold(w.points)

	// A fold can emit NaN or ±Inf even though Append filtered non-finite inputs:
	// a slope over a single instant divides by zero, and two equal timestamps
	// divide by zero time. Such a number is not a value, so the outcome is
	// untrusted and carries zero.
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return Reduced{v: 0, state: StateUntrusted}
	}

	state := StateValue
	// Below the reduction's minimum.
	if len(w.points) < w.red.Min {
		state = StateUntrusted
	}
	// Nothing appended this tick: the window holds its contents but the newest
	// sample is stale-by-one-tick, so the latch holds.
	if !w.lastAppendStored {
		state = StateUntrusted
	}

	return Reduced{v: v, state: state}
}

// denominatorDelta is the denominator counter's delta across the window edges,
// well-defined because both edges stored a present denominator under an against
// reduction. The delta being zero is the normal case the Reduce gate detects.
func denominatorDelta(points []Point) float64 {
	first, _ := points[0].Against.Get()
	last, _ := points[len(points)-1].Against.Get()

	return last - first
}

// Coverage reports the window's extent, for the two latch arms that are gated on
// it rather than on the reduced number.
func (w *Window) Coverage() Coverage {
	var spanned time.Duration
	if len(w.points) >= 2 {
		spanned = w.points[len(w.points)-1].At.Sub(w.points[0].At)
	}

	return Coverage{span: w.span, spanned: spanned}
}
