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
// fired-or-not verdict against a fire and a clear threshold (hysteresis), and
// Rank, which puts the signals that did fire in the order to show them, main
// reason first.

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
	// Worst is the value at which severity reaches 1, in the quantity's own unit
	// and on the worse side of Fire. A cpu fraction rising past 0.70 reaches 1 at
	// 1.0; headroom firing at 0 after a one-core reserve reaches 1 at -1.0, where
	// the reserve is gone. NewEngine refuses a Worst that is not strictly worse
	// than Fire.
	Worst float64
}

// Identity is what a consumer needs to know about a fired signal, copied onto
// every Fired so ranking and consumers never read the table: which Signal, its
// Tier, who the caller blames (Attribution), and its table Index. Rank sorts
// on Tier, severity and Index; Attribution is payload it never reads, the
// caller's vocabulary.
type Identity struct {
	Signal      string
	Tier        int
	Attribution int
	Index       int
}

// Fired is one signal's verdict once it has fired: value, marks, and since when.
type Fired struct {
	// Since is stamped on the fire transition and never refreshed; a restart
	// loses it, so it dates this process's observation, not the condition.
	Since time.Time
	// Instrument is the instrument that fired, stamped on the fire transition and
	// never refreshed; a later live winner does not overwrite it.
	Instrument string
	Identity
	// Marks is the pair Value fired against, stamped with it: Severity scores the
	// one against the other, so a later instrument's pair would not measure it.
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
	instrument  string
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
//	clear: fired, on the firing mark pair, past its Clear, Coverage.Full() -> release.
//	fire:  unfired, past Fire, no release yet or a whole Coverage span since one -> fire.
//	hold:  anything else -> state unchanged.
//
// The clear arm acts only on a reduction measured against the SAME mark pair the
// episode fired under, which is the scale recovery has to be read on: a signal
// answered in spare cores and in a usage fraction cannot have the cores episode
// released by a fraction that happens to sit past a cores threshold.
//
// The gate is the pair, not the instrument, because arms that share a pair are by
// construction answering one question in one unit and differ only in how they
// reduce it — a p95 and the mean fallback behind it. Selection moves between
// those on the tick the p95 reaches its minimum sample count, on every start, and
// their values are interchangeable for judging recovery. Gating on the instrument
// name instead strands such an episode fired: the arm that fired it is never
// selected again, so nothing can ever release it.
//
// instrument names the instrument the reduction came from. It is stamped beside
// the marks and value when the latch fires, and never refreshed afterwards, so a
// Fired names the instrument that fired, not whichever one a later tick selected.
// It is attribution only; the clear arm does not read it.
//
// Arms measuring genuinely different quantities are the remaining gap: while a
// foreign pair keeps answering Ready, no tick can release the episode and every
// tick refreshes lastUpdate, so the Signal.DemoteSpan fallback in Engine.Observe
// never runs either. Such an episode holds until its own arm answers again.
func (l *Latch) Update(instrument string, r Reduced, c Coverage, m Marks, now time.Time) {
	if r.state != StateValue {
		return
	}

	l.lastUpdate = now

	if l.fired && m == l.marks && crossedClear(r.v, l.marks) && c.Full() {
		l.release(now)

		return
	}

	if crossedFire(r.v, m) {
		if !l.fired && (l.lastRelease.IsZero() || !now.Before(l.lastRelease.Add(c.span))) {
			l.fired = true
			l.value = r.v
			l.marks = m
			l.instrument = instrument
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
		Identity:   l.identity,
		Value:      l.value,
		Marks:      l.marks,
		Since:      l.since,
		Instrument: l.instrument,
	}, true
}

// clamp01 bounds a ratio to 0..1 and maps NaN to 0, so Rank stays a total order.
// A table NewEngine accepted cannot reach the NaN arm: it refuses a Worst equal
// to Fire and a Fire-to-Worst span that overflows, which are the two ways the
// division goes 0/0 or Inf/Inf. The arm is for a Fired assembled by hand, which
// Fired and Marks being exported allows: the zero Marks divide 0 by 0.
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

// Severity puts a fired signal on one 0..1 scale, so causes measured in different
// units and directions compare: 0 at its fire mark, 1 at its Worst. Rank orders
// by it.
//
//	clamp01( (value − fire) / (worst − fire) ), each term signed by polarity
//
// worse signs the three terms, so one expression serves both polarities. Value
// and marks are both frozen at the firing tick; Severity does not follow the
// signal afterwards.
func (f Fired) Severity() float64 {
	m := f.Marks
	fire := worse(m.Fire.At, m)

	return clamp01((worse(f.Value, m) - fire) / (worse(m.Worst, m) - fire))
}

// Rank orders the signals that fired so the caller can show the main reason
// first: tier ascending (a lower tier outranks a higher one), then severity
// descending, then the signal's table index. It sorts in place and returns the
// same slice.
func Rank(fired []Fired) []Fired {
	sort.Slice(fired, func(i, j int) bool {
		a, b := fired[i], fired[j]
		if a.Tier != b.Tier {
			return a.Tier < b.Tier
		}

		if sa, sb := a.Severity(), b.Severity(); sa != sb {
			return sa > sb
		}

		return a.Index < b.Index
	})

	return fired
}
