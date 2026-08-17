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

// This file holds the sliding Window that accumulates readings for one measured
// series, and the Coverage it reports about its own extent. Using one:
//
//	w, err := NewWindow(60*time.Second, 60*time.Second, Mean, false)
//	// then, once per tick, in this order:
//	w.Observe(value, against, now) // age out, then store this tick's reading
//	v, state := w.Reduce().Get()   // the reduced number, and whether to trust it
//	full := w.Coverage().Full()    // has it filled its whole span, or just started?

package diagnosis

import (
	"fmt"
	"math"
	"time"
)

// Coverage is how much of its span a window's readings cover: the span it was
// built with, and oldest-to-newest of what it holds.
type Coverage struct {
	span    time.Duration
	spanned time.Duration
}

// Full reports whether the stored readings span the window's whole duration: it
// separates a filled window from one that has only just started.
func (c Coverage) Full() bool { return c.span > 0 && c.spanned >= c.span }

// Window is a sliding window of readings for one measured series: a time-ordered
// slice of Points, pruned from the front once they age past the span.
type Window struct {
	points  []Point
	red     Reduction
	span    time.Duration
	demote  time.Duration
	counter bool
	// lastAppendStored is whether the most recent appendPoint stored a reading.
	// age runs before this tick's store, so there it means the previous tick.
	lastAppendStored bool
}

// NewWindow builds an empty window, refusing a non-positive span or demote span.
//
//	span    how far back the window reaches; entries older than this are pruned
//	demote  how long without a successful read before the window empties; once a
//	        source goes silent that long, Reduce says StateAbsent, not a stale number
//	red     the reduction Reduce applies to the stored points
//	counter whether the series is a monotone counter, so a backwards step is a reset
func NewWindow(span, demote time.Duration, red Reduction, counter bool) (*Window, error) {
	if span <= 0 {
		return nil, fmt.Errorf("window: span %v is not positive", span)
	}
	if demote <= 0 {
		return nil, fmt.Errorf("window: demote span %v is not positive", demote)
	}
	return &Window{span: span, demote: demote, red: red, counter: counter}, nil
}

// Observe advances the window by one tick, ageing out entries past the span
// before storing this tick's reading. It is the only call that moves a window
// forward: skip a tick and the previous tick's entries count as current. Pass
// Unknown() when the read failed.
func (w *Window) Observe(value, against Reading, at time.Time) {
	w.age(at)
	w.appendPoint(value, against, at)
}

// age drops entries that have aged out of the span, subject to the demote and
// freeze rules below. Observe runs it on EVERY tick, failed reads included.
// lastStored is the instant of the last STORED reading, not of the last call,
// and the zero time when the window holds nothing. Points are only ever pruned
// from the front, so the newest one is the last success for as long as there is
// one at all; keeping a separate field would be the same fact written twice.
func (w *Window) lastStored() time.Time {
	if len(w.points) == 0 {
		return time.Time{}
	}

	return w.points[len(w.points)-1].At
}

func (w *Window) age(now time.Time) {
	// Demote: no successful read for the demote span empties the window, freeze or not.
	// An already-empty window has nothing to demote and falls through to the rules
	// below, which are no-ops on no points.
	if last := w.lastStored(); !last.IsZero() && now.Sub(last) > w.demote {
		w.points = nil

		return
	}
	// Freeze: the previous tick stored nothing, so hold the contents through it.
	// A short outage must not shrink the window one tick at a time.
	if !w.lastAppendStored {
		return
	}

	w.prune(now.Add(-w.span))
}

// prune drops entries older than the cutoff, keeping one landing exactly on it.
// Entries are appended in time order, so it drops from the front.
func (w *Window) prune(cutoff time.Time) {
	i := 0
	for i < len(w.points) && w.points[i].At.Before(cutoff) {
		i++
	}
	w.points = w.points[i:]
}

// appendPoint stores one instant's reading; an absent or non-finite value stores
// nothing, and callers must not pre-filter.
func (w *Window) appendPoint(value, against Reading, at time.Time) {
	w.lastAppendStored = false

	v, vok := value.Get()
	if !vok || math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	// Under an against reduction an absent or non-finite denominator appends nothing.
	if w.red.divides {
		a, aok := against.Get()
		if !aok || math.IsNaN(a) || math.IsInf(a, 0) {
			return
		}
	}

	// A source reset moves both counters at once, so on a counter window a backwards
	// step in the value (or, under an against reduction, in the denominator) means
	// the stored entries came from a different origin. Discard them. On any other
	// window a fall is normal.
	if w.counter && len(w.points) > 0 {
		prev := w.points[len(w.points)-1]

		restart := v < prev.Value
		if w.red.divides {
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
	w.lastAppendStored = true
	// A successful store prunes even when the tick began frozen and age skipped it.
	w.prune(at.Add(-w.span))
}

// Reduce applies the window's reduction to the stored points and returns one
// number: a mean, a p95, whatever the window was built with, bound to a State.
func (w *Window) Reduce() Reduced {
	if len(w.points) == 0 {
		return Reduced{state: StateAbsent}
	}
	// A denominator delta of zero or less: the counter did not move, or it reset.
	if w.red.divides && denominatorDelta(w.points) <= 0 {
		return Reduced{v: 0, state: StateUntrusted}
	}
	// Backstop for a bare Reduction{}, which carries no calculation to apply.
	if w.red.fold == nil {
		return Reduced{v: 0, state: StateUntrusted}
	}

	v := w.red.fold(w.points)

	// A reduction can emit NaN or ±Inf on finite inputs: a slope over equal timestamps.
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return Reduced{v: 0, state: StateUntrusted}
	}

	state := StateValue
	if len(w.points) < w.red.Min {
		state = StateUntrusted
	}
	// Stale by one tick: the window holds its contents but nothing was stored.
	if !w.lastAppendStored {
		state = StateUntrusted
	}

	return Reduced{v: v, state: state}
}

// denominatorDelta is the denominator counter's delta across the window edges.
func denominatorDelta(points []Point) float64 {
	first, _ := points[0].Against.Get()
	last, _ := points[len(points)-1].Against.Get()

	return last - first
}

// Coverage reports how much of its span the stored readings cover.
func (w *Window) Coverage() Coverage {
	var spanned time.Duration
	if len(w.points) >= 2 {
		spanned = w.points[len(w.points)-1].At.Sub(w.points[0].At)
	}

	return Coverage{span: w.span, spanned: spanned}
}
