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

// This file holds the sliding Window that accumulates one instrument's readings,
// and the Coverage it reports about its own extent. Using one:
//
//	w, err := NewWindow(60*time.Second, 60*time.Second, Mean, false)
//	// then, once per tick, in this order:
//	w.Observe(value, against, now) // age out, then store this tick's reading
//	v, state := w.Reduce().Get()   // the folded number, and whether to trust it
//	cov := w.Coverage()            // how much of the span the entries cover

package diagnosis

import (
	"fmt"
	"math"
	"time"
)

// Coverage is how much of its span a window's entries cover: the span it was
// built with, and oldest-to-newest of what it holds. The latch's clear arm
// releases only on Full, and its re-fire arm bars one whole span after that.
type Coverage struct {
	span    time.Duration
	spanned time.Duration
}

// Full reports whether the stored entries span the whole window duration.
func (c Coverage) Full() bool { return c.span > 0 && c.spanned >= c.span }

// Window is a sliding window of readings for one (signal, instrument) pair: a
// time-ordered slice of Points, pruned from the front once they age past the
// span. Two rules, both in age: a tick whose read failed does not prune, so the
// contents survive a short outage (the freeze); and once no read has succeeded
// for the demote span the window empties in one step (the demote).
type Window struct {
	// lastSuccess is the instant of the last STORED reading, not of the last
	// call; the demote rule measures from it.
	lastSuccess time.Time
	points      []Point
	red         Reduction
	span        time.Duration
	demote      time.Duration
	counter     bool
	// lastAppendStored is whether the most recent appendPoint stored a reading.
	// age runs before this tick's store, so there it means the previous tick.
	lastAppendStored bool
}

// NewWindow builds an empty window, refusing a non-positive span or demote span.
//
//	span    how far back the window reaches; entries older than this are pruned
//	demote  how long without a successful read before the window empties outright
//	red     the reduction Reduce folds the stored points under
//	counter whether the series is a monotone counter, arming the restart rule
//
// An emptied window reduces to StateAbsent, so demote is when it stops answering.
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
// forward: skip a tick and the previous tick's entries report as current.
//
// Call it once per tick, passing Unknown() when the read failed.
func (w *Window) Observe(value, against Reading, at time.Time) {
	w.age(at)
	w.appendPoint(value, against, at)
}

// age drops entries that have aged out of the span, subject to the demote and
// freeze rules below. Observe runs it on EVERY tick, failed reads included.
func (w *Window) age(now time.Time) {
	// Demote: no successful read for the demote span empties the window, freeze or not.
	if !w.lastSuccess.IsZero() && now.Sub(w.lastSuccess) > w.demote {
		w.points = nil

		return
	}
	// Freeze: the previous tick stored nothing, so hold the contents through it.
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
//
// against is the DENOMINATOR: under a reduction with against the fold divides by
// its delta across the window edges, and every other reduction ignores it.
func (w *Window) appendPoint(value, against Reading, at time.Time) {
	w.lastAppendStored = false

	v, vok := value.Get()
	if !vok || math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	// Under an against reduction an absent or non-finite denominator appends nothing.
	if w.red.against {
		a, aok := against.Get()
		if !aok || math.IsNaN(a) || math.IsInf(a, 0) {
			return
		}
	}

	// Counter restart: a source reset moves both counters at once, so a fall in
	// the value (or, under an against reduction, the denominator) means the
	// stored entries came from a different origin. Discard them. Counter windows
	// only: elsewhere a fall is the quantity doing its job.
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
	// A successful store prunes even when the tick began frozen and age skipped it.
	w.prune(at.Add(-w.span))
}

// Reduce folds the window's stored points into one number under the window's own
// reduction, bound to a State: StateValue carries the number, StateUntrusted
// carries it when still meaningful (below the reduction's minimum, or stale by
// one tick) and zero otherwise, StateAbsent means the window is empty.
//
// Reduce does not age; it reports whatever the last Observe left.
func (w *Window) Reduce() Reduced {
	if len(w.points) == 0 {
		return Reduced{state: StateAbsent}
	}
	// A denominator delta of zero or less: the counter did not move, or it reset.
	if w.red.against && denominatorDelta(w.points) <= 0 {
		return Reduced{v: 0, state: StateUntrusted}
	}
	// Backstop for a bare Reduction{}; NewEngine already refuses a nil fold.
	if w.red.fold == nil {
		return Reduced{v: 0, state: StateUntrusted}
	}

	v := w.red.fold(w.points)

	// A fold can emit NaN or ±Inf on finite inputs: a slope over equal timestamps.
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

// Coverage reports the window's extent, for the two latch arms gated on it.
func (w *Window) Coverage() Coverage {
	var spanned time.Duration
	if len(w.points) >= 2 {
		spanned = w.points[len(w.points)-1].At.Sub(w.points[0].At)
	}

	return Coverage{span: w.span, spanned: spanned}
}
