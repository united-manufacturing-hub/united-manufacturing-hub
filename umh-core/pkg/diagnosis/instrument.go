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

// Instrument is one way of MEASURING a signal: an extractor, a window span, a
// reduction to fold it and marks to judge it. Tried in the order declared.
type Instrument[S any] struct {
	// Extract reads the value from a snapshot. Under DeltaRatio it reads the
	// numerator counter.
	Extract func(S) Reading
	// Against reads the DENOMINATOR of a ratio: DeltaRatio divides the delta of
	// Extract's counter by the delta of this one. Nil for a single series, where
	// Read hands back an absence; NewEngine refuses a dividing reduction with nil.
	Against  func(S) Reading
	Name     string
	Requires []Capability
	Red      Reduction
	Marks    Marks
	Span     time.Duration
	// Boolean says the series is zero or one and nothing between. NewEngine
	// refuses an ordered reduction, a percentile, on such a series.
	Boolean bool
	// Counter says both series are monotone counters, so a fall means the source
	// reset rather than the quantity dropping. It gates the window's restart rule.
	Counter bool
}

// Read applies both extractors to one snapshot, so both counters of a delta
// ratio come off the same instant by construction.
func (i Instrument[S]) Read(s S) (value, against Reading) {
	if i.Against == nil {
		return i.Extract(s), Unknown()
	}
	return i.Extract(s), i.Against(s)
}

// Signal is a QUESTION about the resource, such as whether the CPU is saturated.
// Its Instruments each measure it differently; one latch per signal, one verdict.
type Signal[S any] struct {
	Name        string
	Instruments []Instrument[S]
	// DemoteSpan is how long a signal may go unread before its evidence expires:
	// it empties the window, releases the latch, and sizes Suite's outage cases.
	DemoteSpan time.Duration
	// Tier is the signal's rank class, copied onto every Fired. Rank sorts it
	// ascending, so a lower tier outranks a higher one whatever their
	// severities; the numbers themselves are the caller's vocabulary.
	Tier int
	// ReleaseOnAbsent drops the verdict the moment nothing can answer the signal
	// at all, rather than waiting out DemoteSpan.
	ReleaseOnAbsent bool
	// External marks a cause attributed outside this box, and is Rank's third
	// tie-break.
	External bool
}

// Capable returns every instrument whose required capabilities the environment
// has, in table order. It does not pick a winner; Engine.Select does.
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
