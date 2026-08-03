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

// Instrument is one way of measuring a signal. S is the caller's snapshot type;
// pkg/diagnosis holds machinery, never a domain vocabulary.
type Instrument[S any] struct {
	// Extract reads the value, or a delta ratio's numerator counter.
	Extract func(S) Reading
	// Against reads the denominator counter. It is nil for every instrument
	// that folds a single series, and a nil Against yields an absence rather
	// than a zero denominator.
	Against  func(S) Reading
	Name     string
	Requires []Capability
	Red      Reduction
	Marks    Marks
	Span     time.Duration
	// Boolean says the series is zero or one and nothing between. NewEngine
	// refuses an ordered reduction — a percentile — on such a series.
	Boolean bool
	// Counter says both series this instrument reads are monotone counters, so
	// a fall means the source reset rather than the quantity dropping. It is
	// what gates the window's restart rule, and it is a declaration because
	// nothing downstream can infer it: a falling ratio and a reset counter are
	// the same two floats. One flag covers Extract and Against together — a
	// cgroup that resets resets both counters of a pair at once.
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

// Signal is a question, with one or more instruments that can answer it.
type Signal[S any] struct {
	Name        string
	Instruments []Instrument[S]
	DemoteSpan  time.Duration
	// Tier and External are the signal's ranking facts; the engine copies them
	// onto every Fired it produces.
	Tier            int
	ReleaseOnAbsent bool
	External        bool
}

// Capable returns the instruments whose required capabilities the environment
// has, in table order.
//
// ⚠️ This is the CAPABILITY gate and nothing more — a startup fact about whether
// a source exists on this box at all. It deliberately does not pick a winner:
// two instruments may declare the SAME capability, so a filter that returned the
// first match would return the percentile arm on every capable box forever and
// the fallback arm could never be selected. The second gate is readiness, it is
// per-tick, and it lives on the engine because only the engine holds the
// windows.
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
