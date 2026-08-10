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

// This file is the conformance test generator for a Table. Suite emits one
// Scenario per (signal, case) across the six cases below; Run drives each
// through a fresh Engine, fed by the caller's Feed, and reports the Availability
// it reached. Adding a signal to a table adds six scenarios.
//
//	scenarios := Suite(table)          // what will be driven
//	outcomes := Run(table, env, feed)  // drive it, then assert each
//	                                   // Outcome.Availability

package diagnosis

import "time"

// Case is one shape of tick sequence a generated suite drives a signal through.
type Case int

const (
	// CaseLive: every tick readable.
	CaseLive Case = iota
	// CaseBriefOutage: readable ticks, then an outage short of the demote span.
	CaseBriefOutage
	// CaseLongOutage: readable ticks, then an outage past the demote span.
	CaseLongOutage
	// CaseUnsupported: readable ticks, on a box with none of the capabilities.
	CaseUnsupported
	// CasePostOutageDip: a long outage, then one readable tick.
	CasePostOutageDip
	// CaseBelowFloor: readable ticks, one short of the reduction's minimum.
	CaseBelowFloor
)

// Scenario is one row of a generated suite: one signal, one case.
type Scenario struct {
	Signal string
	Case   Case
}

// Suite generates one Scenario per (signal, case) over t.Signals, never t.Tracks.
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

// Feed is the caller's half of a generated suite: one snapshot builder for a
// tick on which every source the table reads answers, one for a tick on which
// none of them does.
//
// Readable MUST return strictly increasing values, derived from at, for every
// series an instrument declares through Against or Counter. A constant snapshot
// gives a delta ratio a zero denominator delta, which reduces to StateUntrusted
// and never reaches Ready. Single-series instruments fold rates and may repeat a
// value freely. Unreadable must return Unknown() for every Reading an extractor
// touches, so the window stops filling and eventually demotes.
type Feed[S any] interface {
	Readable(at time.Time) S
	Unreadable(at time.Time) S
}

// Outcome is the Availability one Scenario reached on its last tick.
type Outcome struct {
	Scenario
	Availability Availability
}

// Run drives every Scenario Suite generates and reports the Availability each
// reached on its last tick, one Outcome per Scenario in Suite's order.
//
//	CaseLive          m readable                            -> Ready
//	CaseBriefOutage   m readable, then                      -> NoneReady
//	                  max(DemoteSpan/Interval-1, 1)            (the window froze,
//	                  unreadable                                so it holds)
//	CaseLongOutage    m readable, then DemoteSpan/Interval+1 -> AllAbsent
//	                  unreadable
//	CaseUnsupported   m readable, no capabilities           -> NoInstrument *
//	CasePostOutageDip the long outage, then 1 readable      -> NoneReady *
//	CaseBelowFloor    m-1 readable                          -> NoneReady *
//
// The three starred rows reach a different Availability in one case each:
//
//   - CaseUnsupported, when an instrument requires nothing, reaches whatever its
//     window reached in m ticks: Ready if that instrument's own Reduction.Min is
//     m, else NoneReady.
//   - CasePostOutageDip reaches Ready when m == 1.
//   - CaseBelowFloor reaches AllAbsent when m == 1, which drives no readable tick
//     at all.
//
// env is the fully-capable environment. Each case is a tick sequence at
// t.Interval, and m is the SMALLEST Reduction.Min among the signal's capable
// instruments under env, the tick on which the signal first becomes Ready. m is
// always computed under env, even for CaseUnsupported, which drives its ticks
// under NewEnvironment() with no capabilities at all.
//
// The rows hold for a table whose instrument spans, and whose demote span, are at
// least the interval Run drives at: t.Interval, or one second when it is unset.
// Below that the window empties on the first unreadable tick and CaseBriefOutage
// reads AllAbsent instead; validate refuses neither table.
func Run[S any](t Table[S], env Environment, f Feed[S]) []Outcome {
	outcomes := make([]Outcome, 0, len(t.Signals)*6)
	for _, sc := range Suite(t) {
		outcomes = append(outcomes, runScenario(t, sc, env, f))
	}
	return outcomes
}

// runScenario drives one Scenario and returns the Availability it reached.
func runScenario[S any](t Table[S], sc Scenario, env Environment, f Feed[S]) Outcome {
	var sig Signal[S]
	for _, s := range t.Signals {
		if s.Name == sc.Signal {
			sig = s
			break
		}
	}

	// One engine per scenario, so no scenario inherits another's windows.
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
	// Floor at one tick: a zero count would drive CaseLongOutage through the same
	// sequence as CaseBriefOutage.
	if demoteTicks < 1 {
		demoteTicks = 1
	}

	var seq []bool
	driveEnv := env
	switch sc.Case {
	case CaseLive:
		seq = bools(m, true)
	case CaseBriefOutage:
		seq = append(bools(m, true), bools(max(demoteTicks-1, 1), false)...)
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

// drive ticks the engine through seq at interval and returns the signal's
// availability on the last tick. An empty seq is driven once unreadable.
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
// instruments, falling back to the smallest on the signal, then to one.
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

// bools returns a slice of n copies of v.
func bools(n int, v bool) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = v
	}
	return out
}
