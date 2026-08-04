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

import "time"

// Polarity records which direction is worse. Both saturation instruments fire
// on a quantity where LOWER is worse, and two marks are in absolute cores
// rather than a fraction, so a severity normaliser needs unit and polarity.
type Polarity int

const (
	HigherIsWorse Polarity = iota
	LowerIsWorse
)

// Mark is a threshold together with its inclusivity. Inclusivity is part of the
// mark: whether the boundary is crossed at exactly the threshold is a property
// of the mark itself, and a generic comparison silently moves it.
type Mark struct {
	At        float64
	Inclusive bool
}

// Marks is a two-mark Schmitt pair. Clear must be strictly less severe than
// Fire, judged under Polarity — NewEngine refuses a pair that is not.
type Marks struct {
	Unit     string
	Fire     Mark
	Clear    Mark
	Polarity Polarity
	// Capacity normalises severity across instruments. It is the value at which
	// severity reaches 1, stated positively here; Severity signs it by polarity,
	// or the worst case clamps to zero.
	Capacity float64
}

// Identity is what a signal is called and where it ranks — the facts S1 R6 needs
// and a latch cannot learn from a reduction. The engine stamps it at
// construction from the table, so a Fired carries a tier, an external flag and a
// table position without the latch ever consulting the table.
type Identity struct {
	// Signal names the question.
	Signal string
	// Tier is the rank class; lower ranks first, and every cause in a lower tier
	// outranks every cause in a higher one regardless of severity. The numbers
	// are the caller's vocabulary, as Marks.Unit is — this package holds
	// machinery, not words.
	Tier int
	// External marks a cause attributed outside this box. It is R6's third
	// tie-break.
	External bool
	// Index is the signal's position in the table, and R6's last tie-break. It
	// is declared rather than incidental, because Causes[0] picks the customer's
	// refusal string and sort.SliceStable would otherwise decide it.
	Index int
}

// Fired is what a fired latch contributes to a verdict. It carries enough for
// severity ranking without exposing the latch itself.
type Fired struct {
	// Since is stamped on the transition, never on every tick a latch stays
	// fired. It is in-process only and must not become a Cause field.
	Since time.Time
	Identity
	Marks Marks
	// Value is the number the latch fired under, in the mark pair's own unit,
	// before any polarity transform. Severity signs it against the pair.
	Value float64
}

// Latch is a two-mark Schmitt latch, keyed per SIGNAL, not per instrument. A
// per-instrument latch is by construction never fired on the tick its instrument
// is selected, which makes F7 unimplementable.
//
// Latch is not synchronized: it must be owned by exactly one goroutine — the
// observe loop that drives Update — and never shared with a reader that calls
// Fired concurrently. Serial ownership is the contract; a mutex is deliberately
// absent because sharing is a wiring bug, not a latch property.
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

// worse maps a value onto the severity axis so that "more worse" is always a
// larger number, letting both polarities share one comparison on the marks.
func worse(v float64, m Marks) float64 {
	if m.Polarity == LowerIsWorse {
		return -v
	}
	return v
}

// crossedFire reports whether a trustworthy value crosses the fire mark under
// its declared inclusivity — the "bad" side that arms the latch.
func crossedFire(v float64, m Marks) bool {
	x, fx := worse(v, m), worse(m.Fire.At, m)
	if m.Fire.Inclusive {
		return x >= fx
	}
	return x > fx
}

// crossedClear reports whether a trustworthy value crosses the clear mark under
// its declared inclusivity — the "good" side that releases a fired latch.
func crossedClear(v float64, m Marks) bool {
	x, cx := worse(v, m), worse(m.Clear.At, m)
	if m.Clear.Inclusive {
		return x <= cx
	}
	return x < cx
}

// Update judges a trustworthy reduction against the marks.
//
// Coverage is the window's extent and it is what the clear arm and the re-fire
// arm are gated on. ⚠️ There is no readability parameter here and there must
// never be one: that absence is how S1 R5 spec 6 closes F1's reintroduction site
// by signature, which outranks any generated test.
func (l *Latch) Update(r Reduced, c Coverage, m Marks, now time.Time) {
	// Only a trustworthy reduction drives the arms and the marks. An untrusted
	// or absent reduction holds whatever state the latch is in, keeps the marks
	// that accompany the last trusted value, and does not move the clocks.
	if r.state != StateValue {
		return
	}
	l.marks = m
	l.lastUpdate = now

	// Clear arm: a below-clear trustworthy value with full coverage releases.
	if l.fired && crossedClear(r.v, m) && c.Full() {
		l.release(now)
		return
	}

	// Fire arm: crossing the fire mark fires, unless the re-fire arm still
	// blocks — one full window must elapse since the last release.
	if crossedFire(r.v, m) {
		if !l.fired && (l.lastRelease.IsZero() || !now.Before(l.lastRelease.Add(c.span))) {
			l.fired = true
			l.value = r.v
			l.since = now
		}
		return
	}

	// Between the marks: hold whatever state the latch is in.
}

// Reset releases the latch immediately, whatever its coverage. Used when every
// capable instrument's window is absent and the signal declares
// release-on-absent.
//
// ⚠️ The coverage gate on the CLEAR arm does not apply here. An emptied window
// never reports full coverage, so a Reset routed through the clear arm can
// never release — and every AllAbsent release in the design stops working,
// silently. That is S1 R5 spec 4.
//
// The latch has no ReleaseOnAbsent field and no Signal: whether to call this is
// Observe's decision, and S1 R7b spec 6 is where the condition is asserted,
// against two signals that declare it differently.
func (l *Latch) Reset() {
	if l.fired {
		// Reset is an immediate release. With no timestamp to stamp the re-fire
		// bar, anchor it at the last trusted Update (the demote-ish clock); an
		// AllAbsent reset has a stale lastUpdate, so re-fire is effectively
		// immediate, which is the point.
		l.fired = false
		l.lastRelease = l.lastUpdate
	}
}

// ReleaseAfter releases the latch once span has elapsed with nothing able to
// answer the question. This is the time bound that stops a held latch outliving
// its evidence forever.
//
// The clock runs from the last Update — the most recent trustworthy value this
// latch was handed. A fired latch demotes only once that much time has passed
// since the last trustworthy tick. ⚠️ NOT from the first ReleaseAfter call: a
// signal alternating between this arm and any other would restart the clock on
// every alternation and never release.
func (l *Latch) ReleaseAfter(span time.Duration, now time.Time) {
	if l.fired && !now.Before(l.lastUpdate.Add(span)) {
		l.release(now)
	}
}

// release leaves the fired state and stamps the release time. It is what the
// clear arm, Reset and ReleaseAfter all do, and its lastRelease is what the
// re-fire arm measures one full window from.
func (l *Latch) release(now time.Time) {
	l.fired = false
	l.lastRelease = now
}

// Fired reports whether the latch is fired and what it contributes.
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
