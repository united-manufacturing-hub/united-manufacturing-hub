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

// Track is a window with no verdict: a named quantity the engine folds every
// tick so the caller can read the reduction back, with no latch, no marks and no
// selection. It exists because the caller needs means of quantities no
// instrument judges — an instrument's window holds whatever its extractor
// computed, which for a derived instrument is a difference and not the term
// underneath it.
//
// It declares no Requires, and that is the point: a track whose source is missing
// folds nothing and reduces to absence, which is the answer. A capability gate
// here would be a second selector.
type Track[S any] struct {
	Extract func(S) Reading
	Name    string
	Red     Reduction
	Span    time.Duration
}

// Table is the whole declaration for one resource: every signal, in the order
// that breaks Rank's last tie, every track, and the interval at which the caller
// ticks it.
//
// Giving the stages the table rather than a loose slice is what makes "declare
// every signal in one table" checkable: there is no other route to a signal, and
// NewEngine is the only thing that reads it.
type Table[S any] struct {
	Signals []Signal[S]
	// Tracks are folded every tick and judged never; a track has no marks, no
	// latch and no selection.
	Tracks []Track[S]
	// Interval is the cadence the caller ticks at.
	Interval time.Duration
}
