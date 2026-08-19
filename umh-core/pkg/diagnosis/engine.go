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

// This file is the engine. A caller declares a Table; NewEngine reads it once
// and builds one SlidingWindow per (signal, instrument) pair, one SlidingWindow per measurement and
// one Latch per signal, refusing a malformed table. Observe then appends the
// snapshot to every window, resolves what each signal's windows can collectively
// say, an Availability, and drives that signal's latch with it.
//
// The environment is not checked while building: NewEngine takes no Environment
// and builds a window for every instrument, capable or not. Signal.Capable runs
// inside Observe, every tick, so one table runs unchanged on boxes with
// different capabilities and a capability that appears or disappears takes
// effect on the next tick.
//
// Using it:
//
//	engine, err := NewEngine(table)                         // once
//	fired, readiness := engine.Observe(snapshot, env, now)  // every tick
//	causes := Rank(fired)                                   // report order

package diagnosis

import (
	"fmt"
	"math"
	"time"
)

// Availability is what one signal can say this tick: the MAXIMUM State across
// its capable windows, plus a value for having no capable window at all.
type Availability int

const (
	// NoInstrument: this environment satisfies no instrument, so there is no
	// capable window to take a maximum over. The latch runs its demote clock,
	// releasing once DemoteSpan has passed since the last trusted update.
	NoInstrument Availability = iota
	// AllAbsent: every capable window is empty. The latch resets at once if the
	// signal sets ReleaseOnAbsent, and otherwise runs the demote clock.
	AllAbsent
	// NoneReady: no capable window reduced to StateValue and at least one reduced
	// to StateUntrusted, whether below its minimum sample count or frozen by a
	// read outage. The latch holds, bounded by the same demote clock.
	NoneReady
	// Ready: a capable window reduced to StateValue. Its instrument, reduction
	// and coverage go to the latch.
	Ready
)

// The four ascend by how much is known, and follow one window's State exactly:
//
//	no capable window   -> NoInstrument   (0)
//	StateAbsent    (0)  -> AllAbsent      (1)
//	StateUntrusted (1)  -> NoneReady      (2)
//	StateValue     (2)  -> Ready          (3)

// Readiness is one signal's Availability, handed back by Observe beside the
// fired set: one row per signal, in table order, always. []Fired reports only
// what fired, so this is the only route to "was this signal readable at all".
type Readiness struct {
	Signal       string
	Availability Availability
}

// key indexes the engine's window map by (signal, instrument) pair.
type key struct{ Signal, Instrument string }

// signalState is one signal and the single latch that judges it. 7e8301485 took
// these out of name-keyed maps into slices held in the same order; pairing them
// finishes that move, because "one latch per signal" is now the type rather than
// two slices agreeing about their length and their order.
type signalState[S any] struct {
	signal Signal[S]
	latch  Latch
}

// measurementState is one measurement and the single window it reduces through,
// paired for the same reason. Measurement no longer walks one slice while
// indexing another.
type measurementState[S any] struct {
	measurement Measurement[S]
	window      SlidingWindow
}

// Engine owns every window, every measurement and one latch per signal, for one table.
// It is not synchronized: the goroutine that calls Observe owns it, and a reader
// calling Select, Reduction or Measurement from another races on points and latches.
type Engine[S any] struct {
	// windows is the last name-keyed map in here, and it stays one: Select
	// resolves against the CALLER'S Signal, so a lookup can legitimately miss
	// and resolve's nil arms are reachable. The two below are built by
	// NewEngine and cannot miss, which is why they are pairs instead.
	windows      map[key]*SlidingWindow
	signals      []signalState[S]
	measurements []measurementState[S]
}

// NewEngine validates the table, then builds one window per (signal, instrument)
// pair, one window per measurement, and one latch per signal, in signal order. It
// is the single place a malformed table is refused; validate holds the list.
//
// A measurement's demote span is its own span, because a measurement belongs to
// no signal and so has no hold for the demote clock to bound, and its counter
// flag is
// false, because a counter measurement would need the restart rule declared.
func NewEngine[S any](t Table[S]) (*Engine[S], error) {
	if err := validate(t); err != nil {
		return nil, err
	}

	e := &Engine[S]{
		windows:      make(map[key]*SlidingWindow),
		signals:      make([]signalState[S], 0, len(t.Signals)),
		measurements: make([]measurementState[S], 0, len(t.Measurements)),
	}
	for _, s := range t.Signals {
		for _, inst := range s.Instruments {
			w, err := NewSlidingWindow(inst.Span, s.DemoteSpan, inst.Reduction, inst.Counter)
			if err != nil {
				return nil, err
			}

			e.windows[key{Signal: s.Name, Instrument: inst.Name}] = w
		}
	}

	for _, tr := range t.Measurements {
		w, err := NewSlidingWindow(tr.Span, tr.Span, tr.Reduction, false)
		if err != nil {
			return nil, err
		}

		e.measurements = append(e.measurements, measurementState[S]{measurement: tr, window: *w})
	}

	for i, s := range t.Signals {
		// The caller keeps the table they passed, so the engine must own the
		// instruments it stores: a copy of the Signal would still share the
		// Instruments backing array, and a later edit to the caller's own table
		// would rename the engine's instruments while the windows above stay
		// keyed by the names as they were at construction. Deep-copy the
		// instruments and, one level down, each instrument's capability list,
		// which is a second slice header into the same table.
		s.Instruments = append([]Instrument[S](nil), s.Instruments...)
		for j := range s.Instruments {
			s.Instruments[j].Requires = append([]Capability(nil), s.Instruments[j].Requires...)
		}

		e.signals = append(e.signals, signalState[S]{
			signal: s,
			latch: Latch{identity: Identity{
				Signal:      s.Name,
				Tier:        s.Tier,
				Attribution: s.Attribution,
				External:    s.External,
				Index:       i,
			}},
		})
	}

	return e, nil
}

// validate walks a table once and stops at the first row that could never
// produce a verdict, could never hold a point, or names itself twice: a
// reduction with no calculation to apply, a percentile over a boolean series, a
// non-finite mark, a clear mark not on the holding side of its fire mark, a nil
// extractor, a zero span, a duplicate name. The error names the row. Two rules
// need more than an error string:
//
//   - Marks must be finite, Worst, the value where severity reaches 1, must be
//     strictly worse than Fire under the pair's own polarity, and the span from
//     Fire to Worst must not overflow. Otherwise the severity denominator is zero,
//     points the wrong way, or is infinite despite two finite marks, and every
//     cause on that instrument scores 0 and ties at the bottom. Finiteness comes
//     first, since NaN compares false against every ordering test. There is no
//     unset case: a Worst left at zero is refused unless zero really is the worse
//     side of Fire.
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
			if err := validateMeasurement(fmt.Sprintf("signal %q instrument %q", s.Name, inst.Name), "window span", inst.Measurement, t.Interval); err != nil {
				return err
			}

			if seenInstrument[inst.Name] {
				return fmt.Errorf("signal %q: duplicate instrument name %q", s.Name, inst.Name)
			}

			seenInstrument[inst.Name] = true

			if inst.Reduction.ordered && inst.Boolean {
				return fmt.Errorf("signal %q instrument %q: ordered reduction %q on a boolean series", s.Name, inst.Name, inst.Reduction.Name)
			}

			if inst.Reduction.divides && inst.Against == nil {
				return fmt.Errorf("signal %q instrument %q: reduction %q divides but the instrument declares no against extractor", s.Name, inst.Name, inst.Reduction.Name)
			}

			for _, mark := range []struct {
				name  string
				value float64
			}{
				{name: "fire mark", value: inst.Marks.Fire.At},
				{name: "clear mark", value: inst.Marks.Clear.At},
				{name: "worst value", value: inst.Marks.Worst},
			} {
				if math.IsNaN(mark.value) || math.IsInf(mark.value, 0) {
					return fmt.Errorf("signal %q instrument %q: %s %v is not finite", s.Name, inst.Name, mark.name, mark.value)
				}
			}

			if worse(inst.Marks.Clear.At, inst.Marks) >= worse(inst.Marks.Fire.At, inst.Marks) {
				return fmt.Errorf("signal %q instrument %q: clear mark is not on the holding side of its fire mark under its polarity", s.Name, inst.Name)
			}

			if worse(inst.Marks.Worst, inst.Marks) <= worse(inst.Marks.Fire.At, inst.Marks) {
				return fmt.Errorf("signal %q instrument %q: worst value %v is not strictly worse than fire mark %v under its polarity", s.Name, inst.Name, inst.Marks.Worst, inst.Marks.Fire.At)
			}

			if span := worse(inst.Marks.Worst, inst.Marks) - worse(inst.Marks.Fire.At, inst.Marks); math.IsInf(span, 0) {
				return fmt.Errorf("signal %q instrument %q: the distance from fire mark %v to worst value %v overflows, so the severity denominator is not finite", s.Name, inst.Name, inst.Marks.Fire.At, inst.Marks.Worst)
			}
		}
	}

	seenMeasurement := make(map[string]bool, len(t.Measurements))
	for _, m := range t.Measurements {
		if seenMeasurement[m.Name] {
			return fmt.Errorf("duplicate measurement name %q", m.Name)
		}

		seenMeasurement[m.Name] = true

		// Against, Requires, Boolean and Counter are judging-only: an instrument
		// uses them under the marks a signal judges by, but no signal judges a
		// table-level measurement, so declaring one would build and then be
		// silently ignored (the window is still created with Counter false).
		if m.Against != nil {
			return fmt.Errorf("measurement %q: Against is only meaningful inside a signal", m.Name)
		}

		if len(m.Requires) > 0 {
			return fmt.Errorf("measurement %q: Requires is only meaningful inside a signal", m.Name)
		}

		if m.Boolean {
			return fmt.Errorf("measurement %q: Boolean is only meaningful inside a signal", m.Name)
		}

		if m.Counter {
			return fmt.Errorf("measurement %q: Counter is only meaningful inside a signal", m.Name)
		}

		if err := validateMeasurement(fmt.Sprintf("measurement %q", m.Name), "span", m, t.Interval); err != nil {
			return err
		}

		if m.Reduction.divides {
			return fmt.Errorf("measurement %q: reduction %q divides but a measurement declares no denominator series", m.Name, m.Reduction.Name)
		}
	}

	return nil
}

// validateMeasurement applies the five rules every Measurement is held to,
// whether it answers a signal (among a Signal's Instruments) or sits in
// Table.Measurements. row names the ownership for the error, e.g.
// `signal "A" instrument "I1"`, and spanNoun is the word the loop uses for the
// window's size, "window span" for an instrument as against "span" for a bare
// table measurement: each loop's wording is part of the message the tests
// assert on, so the two stay exact.
func validateMeasurement[S any](row string, spanNoun string, m Measurement[S], interval time.Duration) error {
	if m.Extract == nil {
		return fmt.Errorf("%s: nil extract", row)
	}

	if m.Span <= 0 {
		return fmt.Errorf("%s: %s %v is zero or negative", row, spanNoun, m.Span)
	}

	if m.Reduction.Min < 1 {
		return fmt.Errorf("%s: reduction %q minimum sample count %d is below one", row, m.Reduction.Name, m.Reduction.Min)
	}

	if m.Reduction.fold == nil {
		return fmt.Errorf("%s: reduction %q has no fold", row, m.Reduction.Name)
	}

	if interval > 0 && int(m.Span/interval)+1 < m.Reduction.Min {
		return fmt.Errorf("%s: reduction %q minimum sample count %d exceeds what its window span %v can hold at table interval %v", row, m.Reduction.Name, m.Reduction.Min, m.Span, interval)
	}

	return nil
}

// Select applies the two gates, in order, and returns the first capable
// instrument whose window reduces to StateValue, its reduction, that window's
// extent and the Availability that justifies them. It is caller-facing API:
// nothing in the package calls it, and Observe applies the same two gates itself.
//
// Gate one is CAPABILITY, Signal.Capable, re-evaluated every tick against the
// Environment handed in. Gate two is READINESS: can this instrument's window
// supply a trustworthy value right now. They stay separate because a percentile
// instrument and the cheap fallback behind it usually declare the SAME
// capability: a capability-only selector would return the percentile forever,
// leave it untrusted below twenty samples, and never reach the fallback.
//
// Select must follow an Observe on the same tick. Only Observe ages the windows,
// so a window reduced without a preceding Observe reports entries left over from
// an earlier tick as trusted.
func (e *Engine[S]) Select(s Signal[S], env Environment) (Instrument[S], Reduced, Coverage, Availability) {
	return e.resolve(s, s.Capable(env))
}

// resolve applies the readiness gate to a signal's already-capable instruments:
// the first window reducing to StateValue wins, with Ready; otherwise the zero
// instrument, and whichever Availability the State table above names. That is
// the maximum State over those windows, so StateValue returns at once, no later
// window being able to beat it. Select and Observe both call it.
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
		reduced := w.Reduce()

		_, st := reduced.Get()
		switch st {
		case StateValue:
			return inst, reduced, w.Coverage(), Ready
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

// Observe runs one tick against a snapshot and returns the fired set, unranked
// (Rank puts it worst first), and every signal's Readiness, both in table
// order. In order: append to every measurement's and every instrument's window;
// then, per signal, resolve its Availability over the instruments Signal.Capable
// allows this tick, drive its single latch, collect whatever fired, and emit a
// Readiness row whether or not it did.
//
// The append pass is unconditional, instruments this environment cannot satisfy
// included, because Observe is the only call that ages a window: skip it once
// and its stale entries count as current when it is next selected. No held latch
// is left unbounded: AllAbsent on a signal setting ReleaseOnAbsent calls
// Latch.Reset, and every other branch short of Ready runs the demote clock.
//
// # One tick
//
// Each stage narrows what is known. The name on the left performs the step; it
// is not a return type.
//
//	snapshot S
//	  Instrument.Extract  reads     Reading       a float64, or an absence
//	  SlidingWindow.Observe      stores    Point         into a sliding window
//	  SlidingWindow.Reduce       reduces   Reduced       one number, plus whether it is trustworthy
//	  the engine          resolves  Availability  what one signal can say now
//	  Latch.Update        judges    Fired         a signal that crossed its mark
func (e *Engine[S]) Observe(sample S, env Environment, at time.Time) ([]Fired, []Readiness) {
	for i := range e.measurements { // reduced, never judged
		ts := &e.measurements[i]
		ts.window.Observe(ts.measurement.Extract(sample), Unknown(), at) // same clock as every window below
	}

	for _, st := range e.signals {
		for _, inst := range st.signal.Instruments {
			w := e.windows[key{Signal: st.signal.Name, Instrument: inst.Name}]
			if w == nil { // NewEngine builds a window for every pair; never nil here
				continue
			}

			value, against := inst.Read(sample)
			w.Observe(value, against, at)
		}
	}

	var fired []Fired

	readiness := make([]Readiness, 0, len(e.signals))
	for i := range e.signals {
		st := &e.signals[i]
		s := st.signal
		inst, reduced, cov, avail := e.resolve(s, s.Capable(env))
		l := &st.latch

		switch avail {
		case Ready:
			l.Update(inst.Name, reduced, cov, inst.Marks, at)
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

// Reduction returns one named instrument's window reduced as it stands, the only
// route to a number the caller must publish whether or not the latch fired. Like
// Select it reduces without ageing, so it too must follow an Observe. An unnamed
// pair returns the zero Reduced, StateAbsent, indistinguishable from a window
// that exists and is empty: name windows through the SAME constants the table
// declares them with, since a typo in a literal reads as a permanent absence.
func (e *Engine[S]) Reduction(signal, instrument string) Reduced {
	w := e.windows[key{Signal: signal, Instrument: instrument}]
	if w == nil {
		return Reduced{}
	}

	return w.Reduce()
}

// Measurement returns one named measurement's window reduced as it stands; an
// unnamed measurement returns the zero Reduced, StateAbsent. Same contract as
// Reduction, on the windows that belong to no signal: it must follow an Observe
// on the same tick.
func (e *Engine[S]) Measurement(name string) Reduced {
	for i := range e.measurements {
		if e.measurements[i].measurement.Name == name {
			return e.measurements[i].window.Reduce()
		}
	}

	return Reduced{}
}
