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

// Case is one shape of tick sequence a generated suite drives a signal through.
type Case int

const (
	// CaseLive is a signal that is readable every tick.
	CaseLive Case = iota
	// CaseBriefOutage is a signal that fills and then stops filling for less than
	// its demote span, so its window freezes and holds.
	CaseBriefOutage
	// CaseLongOutage is a signal that stops filling long enough that its window
	// demotes and empties.
	CaseLongOutage
	// CaseUnsupported is a signal driven on a box with none of its capabilities.
	CaseUnsupported
	// CasePostOutageDip is a signal that comes back readable after a long outage.
	CasePostOutageDip
	// CaseBelowFloor is a signal that stays readable but never reaches its
	// reduction's minimum.
	CaseBelowFloor
)

// Scenario is one row of a generated suite: one signal, one case.
type Scenario struct {
	Signal string
	Case   Case
}

// Suite generates one Scenario per (signal, case) from a table. It ranges only
// over t.Signals and never over t.Tracks: a suite drives signals, and a track
// has no availability for a scenario to assert. A table that declares tracks
// still emits 6 × len(t.Signals) scenarios, one per signal per case, in case
// order — so adding a row to the table adds six scenarios, and a signal that
// skips the readability path has nowhere to hide.
func Suite[S any](t Table[S]) []Scenario {
	cases := []Case{CaseLive, CaseBriefOutage, CaseLongOutage, CaseUnsupported, CasePostOutageDip, CaseBelowFloor}
	scenarios := make([]Scenario, 0, len(t.Signals)*len(cases))
	for _, s := range t.Signals {
		for _, c := range cases {
			scenarios = append(scenarios, Scenario{Signal: s.Name, Case: c})
		}
	}
	return scenarios
}

// Feed is the caller's half of a generated suite. Readable returns a snapshot in
// which every source the table reads answers; Unreadable returns one in which
// none of them does — every Reading an extractor touches is Unknown().
//
// The feed is SIGNAL-BLIND: two methods, no signal argument. A feed that could
// answer per signal would let a new table row supply its own definition of
// unreadable, and a row that skips the readability path would walk straight
// through the suite generated to catch it.
//
// Readable(at) MUST return strictly increasing values, derived from at, for
// every series an instrument declares through Against or Counter; a constant
// snapshot gives a delta ratio a zero denominator delta, which reduces to
// StateUntrusted and never reaches Ready however long it is ticked. Unreadable
// must return a snapshot in which every Reading an extractor touches is
// Unknown(), so the window stops filling and eventually demotes. Single-series
// instruments fold rates and may repeat a value freely.
type Feed[S any] interface {
	Readable(at time.Time) S
	Unreadable(at time.Time) S
}

// Outcome is what one Scenario concluded: the availability the signal reached on
// the scenario's last tick. One per Scenario, in the order Suite emitted them.
type Outcome struct {
	Scenario
	Availability Availability
}

// Run drives every Scenario that Suite generates through Observe — the
// production entry point, not a reimplementation of it — and reports what each
// one reached. It builds one Engine per scenario, so no scenario inherits
// another's windows.
//
// env is the fully-capable environment. Each case is a tick sequence at
// t.Interval, and m is the SMALLEST Reduction.Min among the signal's capable
// instruments under env — the tick on which the signal first becomes Ready. m is
// always computed under env, even for CaseUnsupported, which ignores it and
// drives the tick under NewEnvironment() with no capabilities at all.
//
//	CaseLive          m readable                             -> Ready
//	CaseBriefOutage   m readable, then DemoteSpan/Interval-1  -> NoneReady
//	                  unreadable                                (the window froze,
//	                                                            so it holds)
//	CaseLongOutage    m readable, then DemoteSpan/Interval+1  -> AllAbsent
//	                  unreadable
//	CaseUnsupported   m readable, no capabilities            -> NoInstrument, or
//	                                                            Ready when some
//	                                                            instrument
//	                                                            requires nothing
//	CasePostOutageDip the long outage, then 1 readable       -> NoneReady, or
//	                                                            Ready when m == 1
//	CaseBelowFloor    m-1 readable                           -> NoneReady, or
//	                                                            AllAbsent when
//	                                                            m == 1 and the
//	                                                            sequence is empty
func Run[S any](t Table[S], env Environment, f Feed[S]) []Outcome {
	outcomes := make([]Outcome, 0, len(t.Signals)*6)
	for _, sc := range Suite(t) {
		outcomes = append(outcomes, runScenario(t, sc, env, f))
	}
	return outcomes
}

// runScenario drives one Scenario through one freshly built engine and returns
// what the signal reached on its last tick.
func runScenario[S any](t Table[S], sc Scenario, env Environment, f Feed[S]) Outcome {
	var sig Signal[S]
	for _, s := range t.Signals {
		if s.Name == sc.Signal {
			sig = s
			break
		}
	}

	// One engine per scenario, over a table holding only this signal, so no
	// scenario inherits another's windows. A valid full table has a valid
	// single-signal subset, so construction only fails on a bug in the table.
	one := Table[S]{Signals: []Signal[S]{sig}, Interval: t.Interval}
	e, err := NewEngine(one)
	if err != nil {
		panic("diagnosis: suite scenario cannot build its engine: " + err.Error())
	}

	m := minCapableMin(sig, env)

	interval := t.Interval
	if interval <= 0 {
		interval = time.Second
	}
	demoteTicks := int(sig.DemoteSpan / interval)
	// Belt-and-suspenders with validate: a demote span below the interval gives a
	// zero tick count, and CaseBriefOutage would then drive bools(-1) into a
	// makeslice panic. validate refuses such a table, but the generator must not
	// be able to panic on a table an engine could accept, so floor the count at
	// one tick.
	if demoteTicks < 1 {
		demoteTicks = 1
	}

	var seq []bool
	driveEnv := env
	switch sc.Case {
	case CaseLive:
		seq = bools(m, true)
	case CaseBriefOutage:
		seq = append(bools(m, true), bools(demoteTicks-1, false)...)
	case CaseLongOutage:
		seq = append(bools(m, true), bools(demoteTicks+1, false)...)
	case CaseUnsupported:
		seq = bools(m, true)
		driveEnv = NewEnvironment()
	case CasePostOutageDip:
		seq = append(append(bools(m, true), bools(demoteTicks+1, false)...), true)
	case CaseBelowFloor:
		seq = bools(m-1, true)
	}

	return Outcome{Scenario: sc, Availability: drive(e, interval, driveEnv, seq, f)}
}

// drive runs the engine through seq readability ticks at interval and returns
// the signal's availability on the last tick, read from the engine's Readiness
// row. An empty sequence — a below-floor m == 1 — is driven once unreadable so
// there is a readiness row to read: the empty window is AllAbsent.
func drive[S any](e *Engine[S], interval time.Duration, env Environment, seq []bool, f Feed[S]) Availability {
	if len(seq) == 0 {
		seq = []bool{false}
	}
	at := time.Unix(0, 0)
	availability := NoInstrument
	for _, readable := range seq {
		var sample S
		if readable {
			sample = f.Readable(at)
		} else {
			sample = f.Unreadable(at)
		}
		_, readiness := e.Observe(sample, env, at)
		availability = readiness[0].Availability
		at = at.Add(interval)
	}
	return availability
}

// minCapableMin is m: the smallest Reduction.Min among the signal's capable
// instruments under env. When nothing is capable under env it falls back to the
// smallest minimum anywhere on the signal, so the generator still drives a
// defined tick count.
func minCapableMin[S any](s Signal[S], env Environment) int {
	m := 0
	for _, inst := range s.Capable(env) {
		if m == 0 || inst.Red.Min < m {
			m = inst.Red.Min
		}
	}
	if m == 0 {
		for _, inst := range s.Instruments {
			if m == 0 || inst.Red.Min < m {
				m = inst.Red.Min
			}
		}
	}
	if m == 0 {
		m = 1
	}
	return m
}

// bools is a slice of n copies of v.
func bools(n int, v bool) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = v
	}
	return out
}
