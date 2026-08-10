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

// A reduction is the aggregation applied to a window's stored points to fold
// them into one number, as a SQL aggregate does over a time window. This file
// declares the fold's vocabulary: Point, Reduction, Reduced and State.

package diagnosis

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Point is one stored entry as the fold reads it: the instant of the reading,
// its value, and the counter whose delta a delta ratio divides by.
type Point struct {
	At      time.Time
	Value   float64
	Against Reading
}

// State is how far the evidence behind a folded number got.
type State int

// The iota order is an ordered ladder of evidence strength: StateAbsent <
// StateUntrusted < StateValue, the middle rung holding something but not enough.
const (
	// StateAbsent means the window is empty, whether nothing was ever stored or
	// the demote span emptied it.
	StateAbsent State = iota
	// StateUntrusted means the window holds entries but the folded number is not
	// worth acting on: fewer entries than the reduction's Min, nothing stored
	// this tick, a delta-ratio denominator whose delta is zero or negative, a
	// fold that produced NaN or an infinity, or a reduction carrying no fold.
	StateUntrusted
	// StateValue means a finite number folded over at least Min entries, one of
	// them stored this tick.
	StateValue
)

// Reduced is a folded number bound to the State that says whether to trust it.
type Reduced struct {
	v     float64
	state State
}

// Get returns the folded number and its outcome, together and only together.
func (r Reduced) Get() (float64, State) { return r.v, r.state }

// Reduction is one aggregation over a window's points, a fold, plus the minimum
// sample count below which its result is untrusted.
type Reduction struct {
	fold func([]Point) float64
	Name string
	Min  int
	// ordered marks a fold that sorts its input, which only a percentile does;
	// NewEngine refuses one over a boolean series.
	ordered bool
	// against marks a fold that divides by Point.Against, and is what makes an
	// absent denominator mean opposite things: under Mean the window stores the
	// point, under DeltaRatio it drops it.
	against bool
}

var (
	// Last is the newest entry. Min is 1: one entry is the whole answer.
	Last = Reduction{Name: "last", Min: 1, fold: foldLast}
	// Mean is the arithmetic mean of the stored values. Min is 2: the mean of a
	// single entry is that entry, which is Last.
	Mean = Reduction{Name: "mean", Min: 2, fold: foldMean}
	// Slope is (v_last − v_first) / (t_last − t_first) in seconds over the two
	// window endpoints, never a least-squares fit. Min is 2: one entry gives a
	// zero time base.
	Slope = Reduction{Name: "slope", Min: 2, fold: foldSlope}
	// DeltaRatio divides the numerator's delta by the denominator's delta, both
	// across the window edges. Min is 2: over one entry both deltas are zero.
	DeltaRatio = Reduction{Name: "deltaRatio", Min: 2, fold: foldDeltaRatio, against: true}
	// P95 is the nearest-rank 95th percentile. Min is 20: below twenty samples
	// the rank ceil(0.95n) equals n, so the percentile IS the maximum.
	P95 = Reduction{Name: "p95", Min: 20, fold: foldP95, ordered: true}
	// P99 is the nearest-rank 99th percentile, Min 100 for the same reason. A 60s
	// window at a 1s interval holds 61 entries, so NewEngine refuses that pairing.
	P99 = Reduction{Name: "p99", Min: 100, fold: foldP99, ordered: true}
)

// NewReduction builds a seventh reduction, folding a single series: against
// stays false. It refuses a minimum below one and a nil fold.
func NewReduction(name string, min int, fold func([]Point) float64) (Reduction, error) {
	if min < 1 {
		return Reduction{}, fmt.Errorf("reduction %q: minimum sample count %d is below one", name, min)
	}
	if fold == nil {
		return Reduction{}, fmt.Errorf("reduction %q: nil fold", name)
	}

	return Reduction{Name: name, Min: min, fold: fold}, nil
}

func foldLast(points []Point) float64 { return points[len(points)-1].Value }

func foldMean(points []Point) float64 {
	var sum float64
	for _, p := range points {
		sum += p.Value
	}
	return sum / float64(len(points))
}

func foldSlope(points []Point) float64 {
	first, last := points[0], points[len(points)-1]
	dt := last.At.Sub(first.At).Seconds()

	return (last.Value - first.Value) / dt
}

// foldDeltaRatio needs a positive denominator delta; Reduce gates on that.
func foldDeltaRatio(points []Point) float64 {
	first, last := points[0], points[len(points)-1]
	firstD, _ := first.Against.Get()
	lastD, _ := last.Against.Get()

	return (last.Value - first.Value) / (lastD - firstD)
}

// nearestRank builds a nearest-rank percentile fold: the value at 1-indexed rank
// ceil(p·n) of the sorted values. It sorts a copy, leaving the points in time order.
func nearestRank(p float64) func([]Point) float64 {
	return func(points []Point) float64 {
		values := make([]float64, len(points))
		for i, pt := range points {
			values[i] = pt.Value
		}
		sort.Float64s(values)
		rank := int(math.Ceil(p * float64(len(values))))

		return values[rank-1]
	}
}

var foldP95 = nearestRank(0.95)

var foldP99 = nearestRank(0.99)
