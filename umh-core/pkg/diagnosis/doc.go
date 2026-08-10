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

// Package diagnosis turns a stream of snapshots into a ranked list of causes.
//
// It is machinery with no vocabulary of its own: it never learns what a CPU or
// a queue is. A caller declares a [Table] saying what to measure and where the
// thresholds sit, ticks an [Engine] with its own snapshot type, and gets back
// the causes that crossed a threshold, in report order.
//
// # One tick
//
// Each stage narrows what is known. The name on the left performs the step; it
// is not a return type.
//
//	snapshot S
//	  Instrument.Extract  reads     Reading       a float64, or an absence
//	  Window.Observe      stores    Point         into a sliding window
//	  Window.Reduce       folds     Reduced       a number, and whether to trust it
//	  the engine          resolves  Availability  what one signal can say now
//	  Latch.Update        judges    Fired         a cause that crossed its mark
//	  Rank                orders    []Fired       report order
//
// # Vocabulary
//
// Each term is defined once, in the file named:
//
//	Reading       a float64 that may be absent                    reading.go
//	Point         one stored reading: instant, value, denominator reduction.go
//	Window        a sliding window of Points, one per instrument  window.go
//	Reduction     how a window folds: last, mean, slope, p95, ... reduction.go
//	Reduced       a folded number bound to the State to trust it  reduction.go
//	State         one window's outcome                            reduction.go
//	Coverage      how much of its span a window actually covers   window.go
//	Capability    a startup fact: does this source exist here     environment.go
//	Instrument    one way of measuring a signal, with its Marks   instrument.go
//	Signal        a question one or more Instruments can answer   instrument.go
//	Availability  one signal's outcome, across all its windows    engine.go
//	Marks         a hysteresis pair: Fire, Clear, polarity, scale latch.go
//	Latch         a Schmitt trigger over one signal               latch.go
//	Fired         what a fired latch contributes to a verdict     latch.go
//	Severity      a fired cause normalised to one 0..1 scale      ranking.go
//	Track         a window with no verdict: folded, never judged  table.go
//	Table         the whole declaration for one resource          table.go
//
// # How much is known
//
// The same idea recurs at two altitudes, and the two ladders line up. Both are
// ordered by strength of evidence, not by severity: further down is more known,
// never worse.
//
//	one window (State)      one signal (Availability)
//	------------------      -------------------------
//	-                       NoInstrument    no capable instrument at all
//	StateAbsent             AllAbsent       capable, but every window is empty
//	StateUntrusted          NoneReady       something, but not enough to trust
//	StateValue              Ready           a number worth judging
//
// A signal's Availability is the maximum State across its capable windows, with
// NoInstrument standing in for the maximum over an empty set.
//
// # Three gates, easily confused
//
//	capability   a startup fact:  does this source exist on this box
//	readability  a per-tick fact: did this tick's read succeed
//	readiness    a per-tick fact: can this window supply a number now
//
// Readability is never a verdict input. A failed read makes a window hold its
// contents, and the latch sees only the resulting State.
//
// # Using it
//
//	table := diagnosis.Table[Snapshot]{
//		Interval: time.Second,
//		Signals:  []diagnosis.Signal[Snapshot]{ ... },
//	}
//	engine, err := diagnosis.NewEngine(table) // the one place a bad table is refused
//	env := diagnosis.NewEnvironment(caps...)
//
//	// once per Interval:
//	fired, readiness := engine.Observe(snapshot, env, now)
//	causes := diagnosis.Rank(fired)
//
// The two returns answer different questions: what fired, and whether each
// signal could be read at all, which is how a caller tells "measured, and fine"
// from "could not measure". For a number to publish whether or not it fired,
// use [Engine.Reduction] or [Engine.Track].
//
// An Engine owns its windows and latches and is driven by one goroutine.
// Nothing here is synchronized.
//
// [Suite] and [Run] generate a conformance suite from a table: six tick shapes
// per signal, driven through the real [Engine.Observe], so a new signal cannot
// quietly skip the unreadable path.
package diagnosis
