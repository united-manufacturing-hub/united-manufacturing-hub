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

// This file drives the package. NewEngine turns a Table into windows and
// latches; Observe runs a tick: update every window, work out what each signal's
// windows can collectively say, then drive that signal's latch on the answer.
// That collective answer is Availability, where the measuring half in window.go
// meets the judging half in latch.go.

package diagnosis

import (
	"fmt"
	"time"
)

// Availability is the MAXIMUM State across a signal's capable windows, plus a
// value for having none. It is the only thing the latch arm switches on.
//
// resolve implements that correspondence exactly:
//
//	no capable window   -> NoInstrument   (0)
//	StateAbsent    (0)  -> AllAbsent      (1)
//	StateUntrusted (1)  -> NoneReady      (2)
//	StateValue     (2)  -> Ready          (3)
type Availability int

const (
	// NoInstrument: no instrument's required capabilities are present, so there
	// is no capable window to take a maximum over. The latch releases on the
	// demote clock rather than holding.
	NoInstrument Availability = iota
	// AllAbsent: every capable window reduced to StateAbsent. The latch resets
	// if the signal declares release-on-absent, and otherwise runs the demote
	// clock.
	AllAbsent
	// NoneReady: NO capable window reduced to StateValue, and at least one
	// reduced to StateUntrusted; the latch holds, bounded by the demote clock.
	// Covers both a window that never reached its minimum and one that reached
	// it and then froze through a read outage.
	NoneReady
	// Ready: a capable window reduced to StateValue. The engine selects that
	// window's instrument and hands its reduction to the latch.
	Ready
)

// Readiness is one signal's Availability, handed back by Observe beside the
// fired set. There is one row per signal, in table order, ALWAYS: a signal that
// fired and a signal that could not be read both appear.
//
// []Fired reports what fired, not what could have, so this is the only route to
// "is this reading usable this tick". The Availability comes from the same walk
// the latch arm switched on, so the two cannot drift.
type Readiness struct {
	Signal       string
	Availability Availability
}

// key indexes the engine's window map by (signal, instrument) pair.
type key struct{ Signal, Instrument string }

// Engine owns every window, every track and one latch per signal for one table.
//
// Engine is not synchronized: it is owned by exactly one goroutine, the observe
// loop, exactly as Latch is. Sharing it with a reader that calls Select,
// Reduction or Track concurrently races on the window points and latch state.
type Engine[S any] struct {
	windows  map[key]*Window
	tracked  map[string]*Window
	latches  map[string]*Latch
	signals  []Signal[S]
	tracks   []Track[S]
	interval time.Duration
}

// NewEngine validates the table, then builds one window per (signal, instrument)
// pair, one window per track, and one latch per signal keyed by signal name. It
// is the single place a malformed table is refused; validate holds the list.
//
// A track's demote span is its own span, because a track belongs to no signal
// and so has no hold for the demote clock to bound, and its counter flag is
// false, because a counter track would need the restart rule declared.
func NewEngine[S any](t Table[S]) (*Engine[S], error) {
	if err := validate(t); err != nil {
		return nil, err
	}

	e := &Engine[S]{
		signals:  t.Signals,
		tracks:   t.Tracks,
		interval: t.Interval,
		windows:  make(map[key]*Window),
		tracked:  make(map[string]*Window),
		latches:  make(map[string]*Latch),
	}
	for _, s := range t.Signals {
		for _, inst := range s.Instruments {
			w, err := NewWindow(inst.Span, s.DemoteSpan, inst.Red, inst.Counter)
			if err != nil {
				return nil, err
			}
			e.windows[key{Signal: s.Name, Instrument: inst.Name}] = w
		}
	}
	for _, tr := range t.Tracks {
		w, err := NewWindow(tr.Span, tr.Span, tr.Red, false)
		if err != nil {
			return nil, err
		}
		e.tracked[tr.Name] = w
	}
	for i, s := range t.Signals {
		e.latches[s.Name] = NewLatch(Identity{
			Signal:   s.Name,
			Tier:     s.Tier,
			External: s.External,
			Index:    i,
		})
	}

	return e, nil
}

// validate walks a table once and stops at the first row that could never
// produce a verdict, could never hold a point, or names itself twice. The error
// names the row and says what failed. Two rules it cannot say in a string:
//
//   - Capacity is the value at which severity reaches 1, stated positively. Zero
//     is unset, declares no cross-instrument normalisation, and is the one value
//     not judged. A non-zero capacity must clear the fire mark under
//     HigherIsWorse, or severity collapses to 0, and must not equal minus it
//     under LowerIsWorse, which zeroes the severity denominator.
//   - A minimum sample count must fit the span at the table interval: a p99 min
//     of 100 over a 60s span at 1s holds 61 entries, so the window would sit at
//     StateUntrusted forever.
func validate[S any](t Table[S]) error {
	seenSignal := make(map[string]bool, len(t.Signals))
	for _, s := range t.Signals {
		if seenSignal[s.Name] {
			return fmt.Errorf("duplicate signal name %q", s.Name)
		}
		seenSignal[s.Name] = true

		if len(s.Instruments) == 0 {
			return fmt.Errorf("signal %q: no instruments", s.Name)
		}
		if s.DemoteSpan <= 0 {
			return fmt.Errorf("signal %q: demote span %v is zero or negative", s.Name, s.DemoteSpan)
		}
		if t.Interval > 0 && s.DemoteSpan < t.Interval {
			return fmt.Errorf("signal %q: demote span %v is below the table interval %v", s.Name, s.DemoteSpan, t.Interval)
		}

		seenInstrument := make(map[string]bool, len(s.Instruments))
		for _, inst := range s.Instruments {
			if inst.Extract == nil {
				return fmt.Errorf("signal %q instrument %q: nil extract", s.Name, inst.Name)
			}
			if seenInstrument[inst.Name] {
				return fmt.Errorf("signal %q: duplicate instrument name %q", s.Name, inst.Name)
			}
			seenInstrument[inst.Name] = true

			if inst.Span <= 0 {
				return fmt.Errorf("signal %q instrument %q: window span %v is zero or negative", s.Name, inst.Name, inst.Span)
			}
			if inst.Red.Min < 1 {
				return fmt.Errorf("signal %q instrument %q: reduction %q minimum sample count %d is below one", s.Name, inst.Name, inst.Red.Name, inst.Red.Min)
			}
			if inst.Red.fold == nil {
				return fmt.Errorf("signal %q instrument %q: reduction %q has no fold", s.Name, inst.Name, inst.Red.Name)
			}
			if inst.Red.ordered && inst.Boolean {
				return fmt.Errorf("signal %q instrument %q: ordered reduction %q on a boolean series", s.Name, inst.Name, inst.Red.Name)
			}
			if inst.Red.against && inst.Against == nil {
				return fmt.Errorf("signal %q instrument %q: reduction %q divides but the instrument declares no against extractor", s.Name, inst.Name, inst.Red.Name)
			}
			if worse(inst.Marks.Clear.At, inst.Marks) >= worse(inst.Marks.Fire.At, inst.Marks) {
				return fmt.Errorf("signal %q instrument %q: clear mark is not on the holding side of its fire mark under its polarity", s.Name, inst.Name)
			}
			if inst.Marks.Capacity < 0 {
				return fmt.Errorf("signal %q instrument %q: mark capacity %v is negative", s.Name, inst.Name, inst.Marks.Capacity)
			}
			if inst.Marks.Polarity == HigherIsWorse && inst.Marks.Capacity != 0 && inst.Marks.Capacity <= inst.Marks.Fire.At {
				return fmt.Errorf("signal %q instrument %q: mark capacity %v leaves no positive headroom over fire mark %v", s.Name, inst.Name, inst.Marks.Capacity, inst.Marks.Fire.At)
			}
			if inst.Marks.Polarity == LowerIsWorse && inst.Marks.Capacity != 0 && inst.Marks.Capacity == -inst.Marks.Fire.At {
				return fmt.Errorf("signal %q instrument %q: mark capacity %v equals minus the fire mark, zeroing the severity denominator", s.Name, inst.Name, inst.Marks.Capacity)
			}
			if t.Interval > 0 && int(inst.Span/t.Interval)+1 < inst.Red.Min {
				return fmt.Errorf("signal %q instrument %q: reduction %q minimum sample count %d exceeds what its window span %v can hold at table interval %v", s.Name, inst.Name, inst.Red.Name, inst.Red.Min, inst.Span, t.Interval)
			}
		}
	}

	seenTrack := make(map[string]bool, len(t.Tracks))
	for _, tr := range t.Tracks {
		if seenTrack[tr.Name] {
			return fmt.Errorf("duplicate track name %q", tr.Name)
		}
		seenTrack[tr.Name] = true

		if tr.Extract == nil {
			return fmt.Errorf("track %q: nil extract", tr.Name)
		}
		if tr.Span <= 0 {
			return fmt.Errorf("track %q: span %v is zero or negative", tr.Name, tr.Span)
		}
		if tr.Red.Min < 1 {
			return fmt.Errorf("track %q: reduction %q minimum sample count %d is below one", tr.Name, tr.Red.Name, tr.Red.Min)
		}
		if tr.Red.fold == nil {
			return fmt.Errorf("track %q: reduction %q has no fold", tr.Name, tr.Red.Name)
		}
		if tr.Red.against {
			return fmt.Errorf("track %q: reduction %q divides but a track declares no denominator series", tr.Name, tr.Red.Name)
		}
		if t.Interval > 0 && int(tr.Span/t.Interval)+1 < tr.Red.Min {
			return fmt.Errorf("track %q: reduction %q minimum sample count %d exceeds what its window span %v can hold at table interval %v", tr.Name, tr.Red.Name, tr.Red.Min, tr.Span, t.Interval)
		}
	}
	return nil
}

// Select applies the two gates, in order, and returns the instrument that won,
// its reduction, its window's extent and the Availability that justifies them.
// It takes the first capable instrument whose window reduces to StateValue.
//
// Gate one is CAPABILITY, Signal.Capable, a startup fact: does this source exist
// on this box at all. Gate two is READINESS, a per-tick fact: can this
// instrument's window supply a trustworthy value right now. They stay separate
// because the percentile arm and the fallback arm declare the SAME capability: a
// capability-only selector returns the percentile arm forever, its window reports
// StateUntrusted below twenty samples, the latch holds, and the fallback arm, the
// reason the series is judgeable at two seconds instead of twenty, is dead code.
//
// Select must follow an Observe on the same tick. Only Observe ages the windows,
// so a window reduced without a preceding Observe reports entries left over from
// an earlier tick as trusted.
func (e *Engine[S]) Select(s Signal[S], env Environment) (Instrument[S], Reduced, Coverage, Availability) {
	return e.resolve(s, s.Capable(env))
}

// resolve applies the readiness gate to a signal's already-capable instruments
// and returns the first whose window reduces to StateValue, with Ready. Absent
// one, it returns the zero instrument and NoneReady if any capable window is
// untrusted, AllAbsent if every one is absent, NoInstrument if there was no
// capable window to judge at all. That fan-out is the maximum State over the
// capable windows, which is why StateValue returns immediately: no later window
// can beat it.
func (e *Engine[S]) resolve(s Signal[S], capable []Instrument[S]) (Instrument[S], Reduced, Coverage, Availability) {
	if len(capable) == 0 {
		return Instrument[S]{}, Reduced{}, Coverage{}, NoInstrument
	}
	seen := 0
	absent := 0
	untrusted := false
	for _, inst := range capable {
		w := e.windows[key{Signal: s.Name, Instrument: inst.Name}]
		if w == nil { // NewEngine builds a window for every pair; never nil here
			continue
		}
		seen++
		red := w.Reduce()
		_, st := red.Get()
		switch st {
		case StateValue:
			return inst, red, w.Coverage(), Ready
		case StateUntrusted:
			untrusted = true
		case StateAbsent:
			absent++
		}
	}
	if untrusted {
		return Instrument[S]{}, Reduced{}, Coverage{}, NoneReady
	}
	if seen > 0 && absent == seen {
		return Instrument[S]{}, Reduced{}, Coverage{}, AllAbsent
	}

	return Instrument[S]{}, Reduced{}, Coverage{}, NoInstrument
}

// Observe runs one tick against a snapshot and returns the fired set, unranked,
// and every signal's Readiness, both in table order.
//
// One tick, in order: update every track's and every instrument's window; then,
// per signal, resolve the signal's Availability, drive its single latch, collect
// whatever fired, and emit a Readiness row whether or not it did.
//
// The update pass covers every instrument's window unconditionally, including
// instruments not selected this tick, because a window that freezes while
// unselected is the freeze-via-read-outage shape. No arm leaves a held latch
// unbounded: everything but Ready either resets it or runs the demote clock.
func (e *Engine[S]) Observe(sample S, env Environment, at time.Time) ([]Fired, []Readiness) {
	for _, tr := range e.tracks { // folded, never judged
		w := e.tracked[tr.Name]
		w.Observe(tr.Extract(sample), Unknown(), at) // same clock as every window below
	}

	for _, s := range e.signals {
		for _, inst := range s.Instruments {
			w := e.windows[key{Signal: s.Name, Instrument: inst.Name}]
			if w == nil { // NewEngine builds a window for every pair; never nil here
				continue
			}
			value, against := inst.Read(sample)
			w.Observe(value, against, at)
		}
	}

	var fired []Fired
	readiness := make([]Readiness, 0, len(e.signals))
	for _, s := range e.signals {
		inst, red, cov, avail := e.resolve(s, s.Capable(env))
		l := e.latches[s.Name] // NewEngine builds one latch per signal; never nil here
		switch avail {
		case Ready:
			l.Update(red, cov, inst.Marks, at)
		case AllAbsent:
			if s.ReleaseOnAbsent {
				l.Reset()
			} else {
				l.ReleaseAfter(s.DemoteSpan, at)
			}
		default: // NoInstrument, NoneReady: hold, then release on the demote clock.
			l.ReleaseAfter(s.DemoteSpan, at)
		}
		if f, ok := l.Fired(); ok {
			fired = append(fired, f)
		}
		readiness = append(readiness, Readiness{Signal: s.Name, Availability: avail})
	}

	return fired, readiness
}

// Reduction returns one named instrument's window folded as it stands right now,
// the only route to a number the caller must publish whether or not the latch
// fired. An unnamed pair returns the zero Reduced, which is StateAbsent and is
// indistinguishable from an existing window that is empty: name windows through
// the SAME constants the table declares them with, not string literals, where a
// typo reads as a permanent absence instead of failing.
//
// It selects nothing and reduces without ageing, so it must follow an Observe on
// the same tick, for the same reason Select does.
func (e *Engine[S]) Reduction(signal, instrument string) Reduced {
	w := e.windows[key{Signal: signal, Instrument: instrument}]
	if w == nil {
		return Reduced{}
	}
	return w.Reduce()
}

// Track returns one named track's window folded as it stands right now. An
// unnamed track returns the zero Reduced, StateAbsent.
//
// Same contract as Reduction, on the folds that belong to no signal: it selects
// nothing, reduces what Observe has already appended this tick, and must
// therefore follow an Observe.
func (e *Engine[S]) Track(name string) Reduced {
	w := e.tracked[name]
	if w == nil {
		return Reduced{}
	}
	return w.Reduce()
}
