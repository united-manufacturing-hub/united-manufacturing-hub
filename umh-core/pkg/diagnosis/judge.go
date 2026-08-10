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

// This file holds the Latch, which turns one signal's readings into a
// fired-or-not verdict against a fire and a clear threshold, and Rank, which
// puts the signals that did fire in the order to show them, main reason first.

package diagnosis

import (
	"math"
	"sort"
	"time"
)

// Mark is a threshold: the value At, and whether Inclusive makes landing exactly
// on it count as crossing.
type Mark struct {
	At        float64
	Inclusive bool
}

// Polarity says which direction is worse: CPU at 90% is bad high (HigherIsWorse),
// free memory at 5% is bad low (LowerIsWorse).
type Polarity int

const (
	HigherIsWorse Polarity = iota
	LowerIsWorse
)

// Marks is a threshold pair: a value past Fire fires the signal, a value past
// Clear releases it again.
type Marks struct {
	// Unit is the caller's word for what the marks measure; unread here.
	Unit     string
	Fire     Mark
	Clear    Mark
	Polarity Polarity
	// Capacity is the value at which severity reaches 1, stated positively
	// whatever the polarity. Zero leaves it undeclared; see Severity.
	Capacity float64
}

// Identity is the four sort keys Rank needs, copied onto every Fired so ranking
// never reads the table: which Signal, its Tier (rank class, sorted ascending),
// whether the cause is External to this box, and its table Index.
type Identity struct {
	Signal   string
	Tier     int
	External bool
	Index    int
}

// Fired is one signal's verdict once it has fired: value, marks, and since when.
type Fired struct {
	// Since is stamped on the fire transition and never refreshed; a restart
	// loses it, so it dates this process's observation, not the condition.
	Since time.Time
	Identity
	// Marks is the pair from the most recent trusted update, not the pair at fire.
	Marks Marks
	// Value is the number that fired, untransformed by polarity.
	Value float64
}

// Latch holds one signal's fired-or-not verdict, one per signal and never one per
// instrument: only the instrument picked for this tick reaches Update. It is not
// synchronized, so only the loop driving Update may call Fired.
type Latch struct {
	since       time.Time
	lastUpdate  time.Time
	lastRelease time.Time
	identity    Identity
	marks       Marks
	value       float64
	fired       bool
}

// NewLatch builds an unfired latch for one signal.
func NewLatch(id Identity) *Latch { return &Latch{identity: id} }

// worse negates a value when lower is worse, so both polarities share one test.
func worse(v float64, m Marks) float64 {
	if m.Polarity == LowerIsWorse {
		return -v
	}
	return v
}

// crossedFire reports whether v is on the firing side of Fire, per its inclusivity.
func crossedFire(v float64, m Marks) bool {
	x, fx := worse(v, m), worse(m.Fire.At, m)
	if m.Fire.Inclusive {
		return x >= fx
	}
	return x > fx
}

// crossedClear reports whether v is on the releasing side of Clear, per its inclusivity.
func crossedClear(v float64, m Marks) bool {
	x, cx := worse(v, m), worse(m.Clear.At, m)
	if m.Clear.Inclusive {
		return x <= cx
	}
	return x < cx
}

// Update judges one trustworthy reduction against the marks, the only way into
// the fired state. An untrusted or absent reduction is ignored. Three arms:
//
//	clear: fired, past Clear, Coverage.Full() -> release.
//	fire:  unfired, past Fire, no release yet or a whole Coverage span since one -> fire.
//	hold:  anything else -> state unchanged.
func (l *Latch) Update(r Reduced, c Coverage, m Marks, now time.Time) {
	if r.state != StateValue {
		return
	}
	l.marks = m
	l.lastUpdate = now

	if l.fired && crossedClear(r.v, m) && c.Full() {
		l.release(now)
		return
	}

	if crossedFire(r.v, m) {
		if !l.fired && (l.lastRelease.IsZero() || !now.Before(l.lastRelease.Add(c.span))) {
			l.fired = true
			l.value = r.v
			l.since = now
		}
		return
	}
}

// Reset releases the latch immediately whatever the coverage, stamping the last
// trusted Update as the release time. Used for a signal declaring ReleaseOnAbsent.
func (l *Latch) Reset() {
	if l.fired {
		l.fired = false
		l.lastRelease = l.lastUpdate
	}
}

// ReleaseAfter releases the latch span after the last trusted Update, so a
// verdict cannot be held forever on stale evidence.
func (l *Latch) ReleaseAfter(span time.Duration, now time.Time) {
	if l.fired && !now.Before(l.lastUpdate.Add(span)) {
		l.release(now)
	}
}

func (l *Latch) release(now time.Time) {
	l.fired = false
	l.lastRelease = now
}

// Fired reports the latch's verdict; it is the zero value unless ok.
func (l *Latch) Fired() (Fired, bool) {
	if !l.fired {
		return Fired{}, false
	}
	return Fired{
		Identity: l.identity,
		Value:    l.value,
		Marks:    l.marks,
		Since:    l.since,
	}, true
}

// clamp01 bounds a ratio to 0..1 and maps NaN to 0, so Rank stays a total order.
// The only NaN is a 0/0: no headroom, read exactly on the fire mark.
func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Severity normalises a fired cause onto 0..1 against its own marks, so causes in
// different units and directions compare. It is frozen at the firing value.
//
//	rising:   clamp01( (value − fire) / (capacity − fire) )
//	falling:  clamp01( (fire − value) / (fire + capacity) )
//
// Capacity zero leaves the fire mark as the whole denominator, negated for a
// rising pair; where that is negative every cause clamps to 0 and ties at the
// bottom (a rising pair over a positive fire mark, a falling pair over a negative
// one). Both are declarable. A falling cause reaches 1 only at value == −capacity,
// so where the quantity cannot go negative the worst reachable score is
// fire/(fire+capacity), at value 0.
func (f Fired) Severity() float64 {
	m := f.Marks
	if m.Polarity == LowerIsWorse {
		return clamp01((m.Fire.At - f.Value) / (m.Fire.At - (-m.Capacity)))
	}
	return clamp01((f.Value - m.Fire.At) / (m.Capacity - m.Fire.At))
}

// Rank orders the signals that fired so the caller can show the main reason
// first: tier ascending (a lower tier outranks a higher one), then severity
// descending, then external attribution first, then the signal's table index.
// It sorts in place and returns the same slice.
func Rank(fired []Fired) []Fired {
	sort.Slice(fired, func(i, j int) bool {
		a, b := fired[i], fired[j]
		if a.Identity.Tier != b.Identity.Tier {
			return a.Identity.Tier < b.Identity.Tier
		}
		if sa, sb := a.Severity(), b.Severity(); sa != sb {
			return sa > sb
		}
		if a.Identity.External != b.Identity.External {
			return a.Identity.External
		}
		return a.Identity.Index < b.Identity.Index
	})
	return fired
}
