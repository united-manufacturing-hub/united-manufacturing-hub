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

// This file is the engine. A caller declares a Table; NewEngine builds the
// windows and latches it declares, and Observe runs one tick against them:
// append the snapshot to every window, resolve what each signal's windows can
// collectively say, an Availability, and drive that signal's latch with it.
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
	"sort"
	"strings"
	"time"
)

// Availability is what one signal can say this tick: the MAXIMUM State across
// its capable windows, plus a value for having no capable window at all.
type Availability int

const (
	// NoInstrument: there is no capable window to take a maximum over, because
	// this environment satisfies no instrument of the signal, or because no
	// capable instrument has a window under the path read. The latch runs its
	// demote clock,
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
// fired set: one row per signal at every depth, refinements included, always.
// []Fired reports only what fired, so this is the only route to "was this signal
// readable at all".
type Readiness struct {
	// Signal is the path, not the bare name: "A/X" for a refinement, its own
	// name for a top-level signal. Two parents may each declare a refinement
	// called X, and a bare name would not say which of them this row is about.
	// It is the path Reduction takes, and NOT the bare name Identity.Signal
	// carries on a Fired.
	Signal       string
	Availability Availability
}

// key indexes the engine's window map by (path, instrument) pair. A top-level
// signal's path is its bare name; a refinement's is its parent's path, the
// pathSeparator, and its own name, e.g. "A/X". This is where that rule is
// stated; the sites that compose or consume a path point here.
type key struct{ Path, Instrument string }

// signalState is one signal narrowed to what the engine judges it by, the latch
// that judges it, and the same pairing for each of its refinements. Pairing them
// makes "one latch per signal" the type, rather than two slices agreeing about
// their length and their order.
type signalState[S any] struct {
	// instruments are this signal's own, deep-copied out of the caller's table.
	instruments []Instrument[S]
	// refinements is the same pairing one level down, in declared order.
	refinements []signalState[S]
	// path is what this signal's windows are keyed under; see key.
	path string
	// latch is the single latch that judges this signal. Every node in the tree
	// has its own, at every depth.
	latch Latch
	// demoteSpan and releaseOnAbsent are Signal.DemoteSpan and
	// Signal.ReleaseOnAbsent: what judge drives the latch with on a tick that
	// reaches no capable window holding a trustworthy number.
	demoteSpan      time.Duration
	releaseOnAbsent bool
}

// capable applies Signal.Capable to the instruments this state holds.
func (st *signalState[S]) capable(env Environment) []Instrument[S] {
	return Signal[S]{Instruments: st.instruments}.Capable(env)
}

// measurementState is one measurement and the single window it reduces through,
// paired for the same reason.
type measurementState[S any] struct {
	measurement Measurement[S]
	window      SlidingWindow
}

// Engine owns every window, every measurement and one latch per signal, refinements
// included, for one table.
// It is not synchronized: the goroutine that calls Observe owns it, and a reader
// calling Select, Reduction or Measurement from another races on points and latches.
type Engine[S any] struct {
	// windows is keyed by key. Select keys on
	// a bare name instead, so a caller passing a refinement reads some other
	// signal's window or none at all. That is what makes resolve's nil arm
	// reachable, and Select's doc carries the restriction it leaves on a caller.
	// The two slices below are built as pairs by NewEngine and cannot miss.
	windows      map[key]*SlidingWindow
	signals      []signalState[S]
	measurements []measurementState[S]
}

// NewEngine validates the table, then builds one window per (signal, instrument)
// pair, one window per measurement, and one latch per signal and per refinement,
// in declared order. It is the single place a malformed table is refused;
// validate holds the list.
//
// A measurement's demote span is its own span, because a measurement belongs to
// no signal and so has no hold for the demote clock to bound.
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
		if err := buildWindows(e, s.Name, s); err != nil {
			return nil, err
		}
	}

	for _, m := range t.Measurements {
		w, err := NewSlidingWindow(m.Span, m.Span, m.Reduction, false)
		if err != nil {
			return nil, err
		}

		e.measurements = append(e.measurements, measurementState[S]{measurement: m, window: *w})
	}

	for i, s := range t.Signals {
		e.signals = append(e.signals, buildSignalState(s.Name, s, i))
	}

	return e, nil
}

// pathSeparator joins a refinement's name onto its parent's path.
// validateSignal refuses it inside any name, so no two signals can compose
// the same path and share a window.
const pathSeparator = "/"

// buildSignalState pairs one signal with the latch that judges it, then recurses
// into its refinements under the same paths buildWindows keyed their windows
// under. index is the signal's position among its siblings, which Identity.Index
// carries.
func buildSignalState[S any](path string, s Signal[S], index int) signalState[S] {
	// The caller keeps the table they passed, so the engine must own the
	// instruments it stores: the caller's slice header points into their own
	// array, and a later edit there would rename the engine's instruments while
	// the windows above stay keyed by the names as they were at construction.
	// Deep-copy the instruments and, one level down, each instrument's
	// capability list, which is a second slice header into the same table.
	instruments := append([]Instrument[S](nil), s.Instruments...)
	for j := range instruments {
		instruments[j].Requires = append([]Capability(nil), instruments[j].Requires...)
	}

	st := signalState[S]{
		instruments: instruments,
		path:        path,
		latch: Latch{identity: Identity{
			Signal:      s.Name,
			Tier:        s.Tier,
			Attribution: s.Attribution,
			Index:       index,
		}},
		refinements:     make([]signalState[S], 0, len(s.Refinements)),
		demoteSpan:      s.DemoteSpan,
		releaseOnAbsent: s.ReleaseOnAbsent,
	}

	for i, r := range s.Refinements {
		st.refinements = append(st.refinements, buildSignalState(path+pathSeparator+r.Name, r, i))
	}

	return st
}

// buildWindows opens one sliding window per (path, instrument) pair in a
// signal's tree, recursing into refinements under their own paths, so A/X and
// B/X keep separate windows even though both are named X.
func buildWindows[S any](e *Engine[S], path string, s Signal[S]) error {
	for _, inst := range s.Instruments {
		w, err := NewSlidingWindow(inst.Span, s.DemoteSpan, inst.Reduction, inst.Counter)
		if err != nil {
			return err
		}

		e.windows[key{Path: path, Instrument: inst.Name}] = w
	}

	for _, r := range s.Refinements {
		if err := buildWindows(e, path+pathSeparator+r.Name, r); err != nil {
			return err
		}
	}

	return nil
}

// validate walks a table once and stops at the first row that could never
// produce a verdict, could never hold a point, or names itself twice: a
// reduction with no calculation to apply, a percentile over a boolean series, a
// non-finite mark, a clear mark not on the holding side of its fire mark, a nil
// extractor, a zero span, a duplicate name, a name holding the separator paths
// are composed with. The error names the row. Two rules need more than an error
// string:
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

		if err := validateSignal(s.Name, s, t.Interval); err != nil {
			return err
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

// validateSignal applies every rule a Signal is held to, then recurses into its
// refinements under the refinement's own path, so an error names where in the
// tree the row lives rather than the bare refinement. Instrument and refinement names are
// unique among their siblings only.
func validateSignal[S any](path string, s Signal[S], interval time.Duration) error {
	// A name holding the separator could compose a path equal to another
	// signal's, e.g. a top-level signal named "A/X" against the refinement X of
	// a signal A, and the two would silently share one window. Refusing it in
	// every segment makes that collision unreachable.
	if strings.Contains(s.Name, pathSeparator) {
		return fmt.Errorf("signal %q: a name may not contain %q", path, pathSeparator)
	}

	if len(s.Instruments) == 0 {
		return fmt.Errorf("signal %q: no instruments", path)
	}

	if s.DemoteSpan <= 0 {
		return fmt.Errorf("signal %q: demote span %v is zero or negative", path, s.DemoteSpan)
	}

	if interval > 0 && s.DemoteSpan < interval {
		return fmt.Errorf("signal %q: demote span %v is below the table interval %v", path, s.DemoteSpan, interval)
	}

	seenInstrument := make(map[string]bool, len(s.Instruments))
	for _, inst := range s.Instruments {
		if err := validateMeasurement(fmt.Sprintf("signal %q instrument %q", path, inst.Name), "window span", inst.Measurement, interval); err != nil {
			return err
		}

		if seenInstrument[inst.Name] {
			return fmt.Errorf("signal %q: duplicate instrument name %q", path, inst.Name)
		}

		seenInstrument[inst.Name] = true

		if inst.Reduction.ordered && inst.Boolean {
			return fmt.Errorf("signal %q instrument %q: ordered reduction %q on a boolean series", path, inst.Name, inst.Reduction.Name)
		}

		if inst.Reduction.divides && inst.Against == nil {
			return fmt.Errorf("signal %q instrument %q: reduction %q divides but the instrument declares no against extractor", path, inst.Name, inst.Reduction.Name)
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
				return fmt.Errorf("signal %q instrument %q: %s %v is not finite", path, inst.Name, mark.name, mark.value)
			}
		}

		if worse(inst.Marks.Clear.At, inst.Marks) >= worse(inst.Marks.Fire.At, inst.Marks) {
			return fmt.Errorf("signal %q instrument %q: clear mark is not on the holding side of its fire mark under its polarity", path, inst.Name)
		}

		if worse(inst.Marks.Worst, inst.Marks) <= worse(inst.Marks.Fire.At, inst.Marks) {
			return fmt.Errorf("signal %q instrument %q: worst value %v is not strictly worse than fire mark %v under its polarity", path, inst.Name, inst.Marks.Worst, inst.Marks.Fire.At)
		}

		if span := worse(inst.Marks.Worst, inst.Marks) - worse(inst.Marks.Fire.At, inst.Marks); math.IsInf(span, 0) {
			return fmt.Errorf("signal %q instrument %q: the distance from fire mark %v to worst value %v overflows, so the severity denominator is not finite", path, inst.Name, inst.Marks.Fire.At, inst.Marks.Worst)
		}
	}

	seen := make(map[string]bool, len(s.Refinements))
	for _, r := range s.Refinements {
		if seen[r.Name] {
			return fmt.Errorf("signal %q: duplicate refinement name %q", path, r.Name)
		}

		seen[r.Name] = true

		if err := validateSignal(path+pathSeparator+r.Name, r, interval); err != nil {
			return err
		}
	}

	return nil
}

// validateMeasurement applies the five rules every Measurement is held to,
// whether it answers a signal (among a Signal's Instruments) or sits in
// Table.Measurements. row names the ownership for the error, e.g.
// `signal "A" instrument "I1"`, and spanNoun is the word the loop uses for the
// window's size, "window span" for an instrument as against "span" for a bare
// table measurement. NewEngine's two specs for a zero span each assert one of
// the two wordings, so the pair cannot collapse to a single noun unnoticed.
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
// Select takes a top-level signal of the table. Passing a refinement is not
// supported: windows are keyed by path and Select keys on the bare name, so a
// refinement named X under A reads the key a top-level X would hold. If the
// table declares no top-level X the result is the zero instrument and
// NoInstrument. If it declares one, and that X carries an instrument the
// refinement names too, Select returns THAT signal's instrument and number,
// marked Ready, with nothing to tell the caller it is the wrong signal's. Read
// a refinement through Reduction, which takes the path.
//
// Select must follow an Observe on the same tick. Only Observe ages the windows,
// so a window reduced without a preceding Observe reports entries left over from
// an earlier tick as trusted.
func (e *Engine[S]) Select(s Signal[S], env Environment) (Instrument[S], Reduced, Coverage, Availability) {
	return e.resolve(s.Name, s.Capable(env))
}

// resolve applies the readiness gate to a signal's already-capable instruments:
// the first window reducing to StateValue wins, with Ready; otherwise the zero
// instrument, and whichever Availability the State table above names. That is
// the maximum State over those windows, so StateValue returns at once, no later
// window being able to beat it. Select and Observe both call it.
//
// path names the windows to read, so a refinement resolves against its own,
// "A/X" rather than the "X" a top-level signal of that name holds.
func (e *Engine[S]) resolve(path string, capable []Instrument[S]) (Instrument[S], Reduced, Coverage, Availability) {
	if len(capable) == 0 {
		return Instrument[S]{}, Reduced{}, Coverage{}, NoInstrument
	}

	seen := 0
	absent := 0
	untrusted := false

	for _, inst := range capable {
		w := e.windows[key{Path: path, Instrument: inst.Name}]
		if w == nil { // reachable through Select; Engine.windows names the route
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

// observeWindows appends the snapshot to every window in a signal's tree,
// refinements included, keyed by path. It reads no latch, so the recursion into
// refinements is unconditional; Signal.Refinements states what that gives a
// caller.
func observeWindows[S any](e *Engine[S], st *signalState[S], sample S, at time.Time) {
	for _, inst := range st.instruments {
		w := e.windows[key{Path: st.path, Instrument: inst.Name}]
		if w == nil { // NewEngine builds a window for every pair; never nil here
			continue
		}

		value, against := inst.Read(sample)
		w.Observe(value, against, at)
	}

	for i := range st.refinements {
		observeWindows(e, &st.refinements[i], sample, at)
	}
}

// judge drives one signal's latch from its own windows, then does the same for
// each of its refinements, whichever way this signal went; Signal.Refinements
// states why. Only REPORTING a refinement waits on the parent, which firedTree
// does.
//
// It appends one Readiness row per signal it drives, carrying the Availability
// that signal's latch was driven with: this signal's row first, then its
// refinements' as the recursion reaches them, so a parent's row comes
// immediately before the rows of the subtree under it.
func (e *Engine[S]) judge(st *signalState[S], env Environment, at time.Time, into []Readiness) []Readiness {
	inst, reduced, cov, avail := e.resolve(st.path, st.capable(env))
	l := &st.latch

	switch avail {
	case Ready:
		l.Update(inst.Name, reduced, cov, inst.Marks, at)
	case AllAbsent:
		if st.releaseOnAbsent {
			l.Reset()
		} else {
			l.ReleaseAfter(st.demoteSpan, at)
		}
	default: // NoInstrument, NoneReady: hold, then release on the demote clock.
		l.ReleaseAfter(st.demoteSpan, at)
	}

	into = append(into, Readiness{Signal: st.path, Availability: avail})

	for i := range st.refinements {
		into = e.judge(&st.refinements[i], env, at, into)
	}

	return into
}

// firedTree reports one signal's verdict with the verdicts of the refinements
// under it nested inside, and nothing at all if this signal did not fire. Every
// field of a nested entry comes off that refinement's own latch, so its Since is
// the tick IT fired, not the tick its parent did.
//
// It sorts the nested entries into the order Fired.Refinements states, in every
// frame of the recursion.
func firedTree[S any](st *signalState[S]) (Fired, bool) {
	f, ok := st.latch.Fired()
	if !ok {
		return Fired{}, false
	}

	for i := range st.refinements {
		if r, ok := firedTree(&st.refinements[i]); ok {
			f.Refinements = append(f.Refinements, r)
		}
	}

	// Index is the declaration position among these siblings and no two share
	// one, so Tier and Index together are a total order that an unstable sort
	// cannot disturb. Tier alone would not do: sort.Slice is explicitly not
	// stable, and it leaves a short slice alone only because Go sorts twelve
	// elements or fewer by insertion.
	sort.Slice(f.Refinements, func(i, j int) bool {
		a, b := f.Refinements[i], f.Refinements[j]
		if a.Tier != b.Tier {
			return a.Tier < b.Tier
		}

		return a.Index < b.Index
	})

	return f, true
}

// Observe runs one tick against a snapshot and returns the fired set, unranked
// (Rank puts it worst first), and one Readiness row per signal at every depth,
// refinements included. The fired set holds top-level signals in table order;
// the readiness rows are depth-first, each signal immediately before the subtree
// under it, and each named by its path. In order: append to every measurement's
// and every instrument's window; then walk the signals, resolving each one's
// Availability over the instruments Signal.Capable allows it this tick, driving
// its single latch and emitting its Readiness row whether or not it fired, and
// collecting each top-level signal whose latch is fired.
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

	for i := range e.signals {
		observeWindows(e, &e.signals[i], sample, at)
	}

	var fired []Fired

	// One row per signal at every depth, so len(e.signals) is a floor on the
	// count rather than the count itself; judge grows the slice past it.
	readiness := make([]Readiness, 0, len(e.signals))
	for i := range e.signals {
		st := &e.signals[i]

		readiness = e.judge(st, env, at, readiness)
		if f, ok := firedTree(st); ok {
			fired = append(fired, f)
		}
	}

	return fired, readiness
}

// Reduction returns one named instrument's window reduced as it stands, the only
// route to a number the caller must publish whether or not the latch fired. Like
// Select it reduces without ageing, so it too must follow an Observe. path names
// the signal: a refinement's path such as "A/X", or a top-level signal's bare
// name. An unnamed pair returns the zero Reduced, StateAbsent, indistinguishable
// from a window that exists and is empty: name windows through the SAME
// constants the table declares them with, since a typo reads as a permanent
// absence.
func (e *Engine[S]) Reduction(path, instrument string) Reduced {
	w := e.windows[key{Path: path, Instrument: instrument}]
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
