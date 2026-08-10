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

// This file holds the judging half of the package: the marks a signal is judged
// against, the latch that judges it, and what a fired latch reports.
//
// Latch is a Schmitt trigger over one signal and Marks is its hysteresis
// threshold pair: the gap between the marks is what keeps a value resting on a
// threshold from flapping the verdict tick after tick.
//
// A fired latch reports a Fired. Rank, in ranking.go, orders those by tier, then
// severity, then external attribution, then table position.

package diagnosis

import "time"

// Polarity says which direction is worse, declared per mark pair.
type Polarity int

const (
	// HigherIsWorse: the latch fires above its Fire mark.
	HigherIsWorse Polarity = iota
	// LowerIsWorse: the latch fires below its Fire mark.
	LowerIsWorse
)

// Mark is a threshold, plus whether landing exactly on At counts as crossing.
type Mark struct {
	At        float64
	Inclusive bool
}

// Marks is a threshold pair: Fire arms the latch, Clear releases it. NewEngine
// refuses a pair whose Clear is not strictly less severe than Fire.
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

// Identity is the four ranking keys NewEngine stamps on a signal from the table.
type Identity struct {
	// Signal names the question the latch answers.
	Signal string
	// Tier is the rank class, in the caller's vocabulary as Marks.Unit is.
	Tier int
	// External marks a cause attributed outside this box.
	External bool
	// Index is the signal's position in the table.
	Index int
}

// Fired is what a fired latch contributes to a verdict, enough to rank it.
type Fired struct {
	// Since is stamped on the fire transition and never refreshed; a restart
	// loses it, so it dates this process's observation, not the condition.
	Since time.Time
	Identity
	// Marks is the pair from the latch's most recent trusted update.
	Marks Marks
	// Value is the number the latch fired at, untransformed by polarity.
	Value float64
}

// Latch holds one signal's verdict. One latch per signal, never one per
// instrument: only the selected instrument's reduction reaches Update.
//
// Three ways out of the fired state, each stamping the release time that the
// re-fire bar then measures one full window from:
//
//	Update clear arm  a trustworthy value past Clear with full coverage; stamps now
//	ReleaseAfter      span elapsed since the last trusted Update; stamps now
//	Reset             immediate, whatever the coverage; stamps the last trusted Update
//
// Latch is not synchronized: the observe loop that drives Update owns it, and no
// reader may call Fired concurrently.
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

// crossedFire reports whether v is on the arming side of Fire, per its inclusivity.
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

// Update judges one trustworthy reduction against the marks. Three arms:
//
//	clear: fired, past Clear, full coverage -> release.
//	fire:  unfired, past Fire, and no release yet or a full window since one -> fire.
//	hold:  anything else -> keep the current state.
//
// An untrusted or absent reduction takes no arm and changes nothing.
func (l *Latch) Update(r Reduced, c Coverage, m Marks, now time.Time) {
	// Trust comes from the reduction's State and coverage, nothing else.
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

// Reset releases the latch immediately; Observe calls it when the signal
// declares ReleaseOnAbsent and every capable window is absent.
func (l *Latch) Reset() {
	if l.fired {
		l.fired = false
		l.lastRelease = l.lastUpdate
	}
}

// ReleaseAfter releases the latch span after the last trusted Update, bounding
// how long a held latch outlives its evidence.
func (l *Latch) ReleaseAfter(span time.Duration, now time.Time) {
	if l.fired && !now.Before(l.lastUpdate.Add(span)) {
		l.release(now)
	}
}

// release leaves the fired state and stamps lastRelease at now.
func (l *Latch) release(now time.Time) {
	l.fired = false
	l.lastRelease = now
}

// Fired reports the latch's contribution; it is the zero value unless ok.
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
