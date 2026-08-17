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

// Instrument is one way of measuring a signal: what to read, over how long a
// window, under which reduction, against which thresholds.
type Instrument[S any] struct {
	// Extract reads the value from a snapshot, the numerator under DeltaRatio.
	Extract func(S) Reading
	// Against reads the DENOMINATOR of a ratio: DeltaRatio divides the delta of
	// Extract's counter by the delta of this one. Nil for a single series.
	Against   func(S) Reading
	Name      string
	Requires  []Capability
	Reduction Reduction
	// Marks are the thresholds this instrument's number is judged against.
	Marks Marks
	Span  time.Duration
	// Boolean says the series is zero or one and nothing between.
	Boolean bool
	// Counter says both series are monotone counters, so a backwards step means
	// the source reset rather than the quantity dropping. A counter window
	// discards what it holds when that happens.
	Counter bool
}

// Read applies both extractors to one snapshot, so both counters of a delta
// ratio come off the same instant. With no Against it hands back an absence.
func (i Instrument[S]) Read(s S) (value, against Reading) {
	if i.Against == nil {
		return i.Extract(s), Unknown()
	}
	return i.Extract(s), i.Against(s)
}

// Signal is a QUESTION about the resource, such as whether the CPU is saturated.
// Each of its Instruments answers it differently. The order is significant: the
// first instrument that can supply a number is the one used.
type Signal[S any] struct {
	Name        string
	Instruments []Instrument[S]
	// DemoteSpan is how long a signal may go unread before its window empties:
	// stale detection, so an old number cannot stand forever.
	DemoteSpan time.Duration
	// Tier is a rank class the caller assigns, lower meaning more urgent. The
	// numbers themselves are the caller's vocabulary.
	Tier int
	// ReleaseOnAbsent stops reporting the signal the moment nothing can answer
	// it at all, rather than waiting out DemoteSpan.
	ReleaseOnAbsent bool
	// External marks a cause attributed outside this box rather than to it.
	External bool
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

// Track is a quantity measured but never judged: reduced every tick like an
// instrument, with no thresholds. Use it for a number the caller must publish.
type Track[S any] struct {
	Extract   func(S) Reading
	Name      string
	Reduction Reduction
	Span      time.Duration
}

// Table is the whole declaration for one resource: every signal, every track,
// and the interval the caller ticks at. The order of Signals is significant.
type Table[S any] struct {
	Signals  []Signal[S]
	Tracks   []Track[S]
	Interval time.Duration
}
