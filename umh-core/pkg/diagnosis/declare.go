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

// This file holds the declarations a caller writes to describe one resource.

package diagnosis

import (
	"time"
)

// Capability is something the underlying system either has or has not: PSI is
// not available on every kernel. An instrument names what it needs in Requires,
// so on a box without it that instrument stays out of the calculation.
type Capability string

// Environment is the set of capabilities present on this box.
type Environment struct {
	caps map[Capability]bool
}

// NewEnvironment builds an Environment; the set is fixed once built.
func NewEnvironment(caps ...Capability) Environment {
	set := make(map[Capability]bool, len(caps))
	for _, c := range caps {
		set[c] = true
	}

	return Environment{caps: set}
}

// Has reports whether a capability is present.
func (e Environment) Has(c Capability) bool {
	return e.caps[c]
}

// Measurement is a number sampled over time: what to read, over how long a
// window, under which reduction. It carries no thresholds, so nothing about a
// measurement can answer a signal. Its number is judged only where an
// Instrument pairs it with marks.
type Measurement[S any] struct {
	Name string
	// Extract reads the value from a snapshot, the numerator under DeltaRatio.
	Extract func(S) Reading
	// Against reads the DENOMINATOR of a ratio: DeltaRatio divides the delta of
	// Extract's counter by the delta of this one. Nil for a single series.
	Against   func(S) Reading
	Span      time.Duration
	Reduction Reduction
	Requires  []Capability
	// Boolean says the series is zero or one and nothing between.
	Boolean bool
	// Counter says both series are monotone counters, so a backwards step means
	// the source reset rather than the quantity dropping. A counter window
	// discards what it holds when that happens.
	Counter bool
}

// Instrument is one way of answering a signal: a measurement plus the marks its
// number is judged against.
type Instrument[S any] struct {
	Measurement[S]
	// Marks are the thresholds this instrument's number is judged against.
	Marks Marks
}

// Read applies both extractors to one snapshot, so both counters of a delta
// ratio come off the same instant. With no Against it hands back an absence.
func (m Measurement[S]) Read(s S) (value, against Reading) {
	if m.Against == nil {
		return m.Extract(s), Unknown()
	}

	return m.Extract(s), m.Against(s)
}

// Signal is a QUESTION about the resource, such as whether the CPU is saturated.
type Signal[S any] struct {
	Name string
	// Instruments each answer the signal differently. The order is significant:
	// the first instrument that can supply a number is the one used.
	Instruments []Instrument[S]
	// Refinements are signals declared under this one, narrowing its answer with
	// their own instruments and marks. A refinement appears in Fired.Refinements
	// under a parent that fired, and never as a verdict of its own. The
	// Refinements section of the package doc works through an example.
	Refinements []Signal[S]
	// DemoteSpan is how long a signal may go unread before its window empties:
	// stale detection, so an old number cannot stand forever.
	DemoteSpan time.Duration
	// Tier is a rank class the caller assigns, lower meaning more urgent. The
	// numbers themselves are the caller's vocabulary.
	Tier int
	// Attribution is who the caller blames, an opaque int. The numbers are the
	// caller's vocabulary, and this package never interprets them.
	Attribution int
	// ReleaseOnAbsent stops reporting the signal the moment nothing can answer
	// it at all, rather than waiting out DemoteSpan.
	ReleaseOnAbsent bool
}

// Capable returns the instruments the environment satisfies, in declared order.
func (s Signal[S]) Capable(env Environment) []Instrument[S] {
	capable := make([]Instrument[S], 0, len(s.Instruments))
	for _, inst := range s.Instruments {
		satisfied := true

		for _, req := range inst.Requires {
			if !env.Has(req) {
				satisfied = false

				break
			}
		}

		if satisfied {
			capable = append(capable, inst)
		}
	}

	return capable
}

// Table is the whole declaration for one resource: every signal and every
// measurement.
type Table[S any] struct {
	// The order is significant: see Identity.Index.
	Signals []Signal[S]
	// Measurements are the numbers no signal judges: each has a window sampled
	// every tick like an instrument's, but no thresholds, and it is reduced only
	// when the caller reads it through Engine.Measurement. Use one for a number
	// the caller must publish without a verdict.
	Measurements []Measurement[S]
	// Interval is how often the caller ticks.
	Interval time.Duration
}
