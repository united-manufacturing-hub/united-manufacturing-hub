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

// Availability is what a signal's instruments could collectively say this tick.
// It is the multi-instrument generalisation of one window's State, and it is the
// only thing the latch arm switches on.
//
// The four values are not four spellings of two: a signal with no capable
// instrument, a signal whose every capable window is empty, and a signal holding
// below a minimum take three different latch actions, and collapsing any two of
// them would route two different evidence conditions to the same latch arm.
type Availability int

const (
	// NoInstrument: no instrument's required capabilities are present. Nothing
	// on this box can answer the question, and the latch releases on the demote
	// clock — never `continue`, which would let a fired latch outlive its
	// evidence with no time bound at all.
	NoInstrument Availability = iota
	// AllAbsent: every capable instrument's window is empty or its newest entry
	// is older than the demote span. Release if the signal declares
	// release-on-absent.
	AllAbsent
	// NoneReady: NO capable window reduced to StateValue, and at least one is
	// StateUntrusted — below its reduction's minimum, or nothing appended this
	// tick, or its divisor was zero. The latch holds, bounded by the demote
	// clock.
	//
	// Both conditions, not either: the fallback arm reaches StateValue at two
	// samples while the percentile arm is still untrusted at nineteen, and
	// answering NoneReady there would hold the series below twenty samples and
	// bury the fallback arm entirely.
	//
	// ⚠️ This value covers two different windows — one that has never reached
	// its minimum, and one that reached it and then froze through a read
	// outage. Anything branching on the difference must say which it means.
	NoneReady
	// Ready: a capable instrument's window reduced to StateValue.
	Ready
)

// Readiness is one signal's Availability, handed back by Observe beside the
// fired set. One row per signal, in table order, whether or not it fired.
//
// 🔥 It exists because []Fired reports what fired, not what could have. A
// signal that is Ready and sitting below its mark contributes nothing to the
// fired set, so a caller that must say "this reading is usable this tick" — a
// budget line, an admission count — has no other route to the answer.
//
// The Availability here is the SAME value the latch arm switched on, taken from
// the same pass. That is the point of returning it rather than letting the
// caller call Select again: one walk, so the number a message prints and the
// number a refusal reads cannot drift.
type Readiness struct {
	Signal       string
	Availability Availability
}

// key is unexported, and so are the two maps it indexes. Exporting either is
// what would force every resource adopting this package to reimplement the loop.
type key struct{ Signal, Instrument string }

// Engine owns every window and every latch for one table, and runs the per-tick
// loop.
//
// Engine is not synchronized: it is owned by exactly one goroutine — the observe
// loop — exactly as Latch is. Sharing it with a reader that calls Select,
// Reduction or Track concurrently races on the window points and latch state.
type Engine[S any] struct {
	windows  map[key]*Window
	tracked  map[string]*Window
	latches  map[string]*Latch
	signals  []Signal[S]
	tracks   []Track[S]
	interval time.Duration
}

// NewEngine builds the windows, the tracks and one latch per signal for a table.
//
// Each window is built as NewWindow(inst.Span, s.DemoteSpan, inst.Red,
// inst.Counter) — the counter declaration is carried from the instrument to the
// window here, and this is the only place it travels.
//
// A track's window is built as NewWindow(t.Span, t.Span, t.Red, false). Its
// demote span is its own span because a track belongs to no signal, so there is
// no hold for the demote clock to bound; counter is false because a track folds
// a rate, and a counter track would need the restart rule declared.
//
// One latch is built per signal, keyed by name, so Observe can drive the
// per-signal latch arms and report what fired without consulting the table.
func NewEngine[S any](t Table[S]) (*Engine[S], error) {
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
			e.windows[key{Signal: s.Name, Instrument: inst.Name}] =
				NewWindow(inst.Span, s.DemoteSpan, inst.Red, inst.Counter)
		}
	}
	for _, tr := range t.Tracks {
		e.tracked[tr.Name] = NewWindow(tr.Span, tr.Span, tr.Red, false)
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

// Select applies the two gates, in order, and reports what they concluded.
//
// Gate one is CAPABILITY — Signal.Capable, a startup fact, "does this source
// exist on this box at all". Gate two is READINESS — "can this instrument's
// window supply a trustworthy value right now" — which is a per-tick question
// only the engine can answer, because only the engine holds the windows. The
// engine takes the first capable instrument whose window reduces to StateValue.
//
// 🔥 Keeping the two gates separate is the whole point. The percentile arm
// and the fallback arm declare the SAME capability, so a capability-only
// selector returns the percentile arm forever, its window reports
// StateUntrusted below twenty samples, the latch holds, and the fallback arm —
// the entire reason the series is judgeable at two seconds instead of twenty —
// is dead code. Merging the two gates into one predicate loses the distinction
// quietly.
//
// The instrument, its reduction and its window's extent are returned in the same
// call as the availability that justifies them; there is no field to reach past.
//
// Select must follow an Observe on the same tick. Only Observe ages the windows;
// a window this call reduces without a prior Age reports entries left over from
// an earlier tick as trusted, which is the stale-read shape Window.Reduce warns
// against. A tick that only wants readiness rows can read Observe's second return
// instead.
func (e *Engine[S]) Select(s Signal[S], env Environment) (Instrument[S], Reduced, Coverage, Availability) {
	return e.resolve(s, s.Capable(env))
}

// resolve applies the readiness gate to a signal's already-capable instruments
// and reports the first whose window can supply a trustworthy value; absent one,
// the fan-out that the latch arms and readiness rows consume. Observe and Select
// share it so the number a tick reports and the value a caller reads come from
// the same walk over the same windows.
func (e *Engine[S]) resolve(s Signal[S], capable []Instrument[S]) (Instrument[S], Reduced, Coverage, Availability) {
	if len(capable) == 0 {
		return Instrument[S]{}, Reduced{}, Coverage{}, NoInstrument
	}
	seen := 0
	absent := 0
	untrusted := false
	for _, inst := range capable {
		w := e.windows[key{Signal: s.Name, Instrument: inst.Name}]
		if w == nil {
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

// Observe runs one tick against a snapshot and returns two things, both in
// table order: the fired set, unranked, and every signal's Readiness.
//
// It ages every window and appends to every instrument's window unconditionally
// — including instruments that are not selected — because a window that freezes
// while unselected is the freeze-via-read-outage shape one layer up.
//
// Then, for each signal, it resolves the signal's Availability in one walk and
// drives that signal's single latch: a Ready signal hands the selected
// instrument's reduction, coverage and marks to Update; an AllAbsent signal with
// release-on-absent Resets; and every other Availability runs the demote clock
// via ReleaseAfter, so a held latch always has a time bound. Whatever fired is
// collected, and a Readiness row is emitted for the signal whether or not it
// fired.
//
// 🔥 The second return is not a convenience. The loop already computes each
// signal's Availability to decide what the latch does with it, and handed it
// back here is what lets a caller distinguish "we read it and it is fine" from
// "we could not read it" without walking the table a second time. The
// alternative — the caller calling Select per signal — re-runs the capability
// gate and can disagree with the walk the verdict was built from, which is the
// defect class this package exists to close.
//
// One row per signal, ALWAYS: a signal that fired and a signal that could not
// be read both appear, because the absence of a row is the ambiguity the return
// is here to remove.
func (e *Engine[S]) Observe(sample S, env Environment, at time.Time) ([]Fired, []Readiness) {
	for _, tr := range e.tracks { // folded, never judged
		w := e.tracked[tr.Name]
		w.Age(at) // same clock as every window below
		w.Append(tr.Extract(sample), Unknown(), at)
	}

	for _, s := range e.signals {
		for _, inst := range s.Instruments {
			w := e.windows[key{Signal: s.Name, Instrument: inst.Name}]
			if w == nil {
				continue
			}
			w.Age(at)
			value, against := inst.Read(sample)
			w.Append(value, against, at)
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
				l.Reset(at)
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

// Reduction reads back one named instrument's window as it stands right now. It
// is the route from a window to a number the caller must publish whether or not
// the latch fired — Observe returns which latches fired and how ready each
// signal is, and neither return carries a number, so without this a value below
// its mark is unreachable.
//
// It selects nothing, judges nothing and computes nothing: it reduces the window
// Observe has already appended to this tick. That is what separates it from
// calling Select a second time, which re-runs the capability gate and can reach
// a different instrument than the one the verdict was built from.
//
// 🔥 It returns a Reduced and not a (float64, bool), and that is the same rule
// the Reading readers follow: a number and a second value saying whether to
// believe it. Reduced already carries its own outcome, and StateUntrusted
// carries the number with it.
//
// An unnamed pair reduces to the zero Reduced, which is StateAbsent — the
// correct answer for a window that does not exist, and indistinguishable from
// one that is empty. ⚠️ Callers therefore name windows through the SAME
// constants the table declares them with; a string literal at a call site is a
// typo that reads as a permanent absence.
//
// Reduction reduces without aging, so it must also follow an Observe on the same
// tick, for the same reason Select does.
func (e *Engine[S]) Reduction(signal, instrument string) Reduced {
	w := e.windows[key{Signal: signal, Instrument: instrument}]
	if w == nil {
		return Reduced{}
	}
	return w.Reduce()
}

// Track reads back one named track's window. Same contract as Reduction, on the
// folds that belong to no signal — it selects nothing and reduces what Observe
// has already appended to this tick, so it must also follow an Observe on the
// same tick. An unnamed track reduces to the zero Reduced, StateAbsent.
func (e *Engine[S]) Track(name string) Reduced {
	w := e.tracked[name]
	if w == nil {
		return Reduced{}
	}
	return w.Reduce()
}
