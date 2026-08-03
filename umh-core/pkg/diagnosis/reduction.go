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

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Point is one stored entry, as a reduction sees it: the instant it was read,
// the value, and the counter the value is divided by when the reduction is a
// delta ratio.
//
// A reduction folds Points and not floats because a []float64 carries neither
// the timestamps the slope-reduction measures against nor the second counter a
// delta ratio divides by.
//
// Against is a Reading rather than a float: an instrument that folds a single
// series has no denominator, and an absent denominator must not arrive as a
// zero one.
type Point struct {
	At      time.Time
	Value   float64
	Against Reading
}

// State is the outcome of reducing a window. Three outcomes, not two: empty is
// not a special case of below-minimum.
type State int

const (
	// StateAbsent means the window is empty, or its newest entry is older than
	// the demote span. The latch is released if the signal declares it.
	StateAbsent State = iota
	// StateUntrusted means non-empty and recent, but below the reduction's
	// minimum, nothing appended this tick, or the fold's divisor is zero. The
	// latch HOLDS.
	StateUntrusted
	// StateValue means at least the reduction's minimum, newest entry recent.
	StateValue
)

// Reduced is a reduced number bound to its outcome.
//
// There is no exported field beside the number, so `switch v := w.Reduce();
// v.State` does not compile. The number cannot be read without its outcome.
type Reduced struct {
	v     float64
	state State
}

// Get returns the reduced value and its outcome. There is no way to obtain the
// number alone — the tuple return is the only access, so a caller cannot read
// the value without also reading its state.
func (r Reduced) Get() (float64, State) { return r.v, r.state }

// Reduction declares a window's minimum sample count and whether the window
// must gate on a denominator. The floor belongs to the reduction and only to
// the reduction.
//
// ordered says the fold sorts its input, which only a percentile does. It is
// unexported because it is the package's own six reductions that set it.
//
// against says the window's denominator is load-bearing, and it is the ONLY
// route by which a window learns that. Without it Append is handed the same
// two arguments — a value and an absent denominator — in the two cases that
// must behave oppositely: a single-series reduction stores the point, a ratio
// reduction stores nothing. It is unexported for the same reason ordered is:
// the package's own reductions set it, and a caller who could set it could lie
// about it.
type Reduction struct {
	fold    func([]Point) float64
	Name    string
	Min     int
	ordered bool
	against bool
}

// The six reductions, each carrying the minimum sample count Appendix A gives
// it. A caller picks one of these rather than restating a floor at every
// instrument, so the floors live in one place.
var (
	// Last is the newest entry. One reading is the answer, so pressure can fire
	// on tick 0.
	Last = Reduction{Name: "last", Min: 1, fold: foldLast}
	// Mean is the arithmetic mean. Two points, or there is no average.
	Mean = Reduction{Name: "mean", Min: 2, fold: foldMean}
	// Slope is (v_last − v_first) / (t_last − t_first) in seconds — the
	// gradient over the window's own first and last timestamps. Two endpoints,
	// never a least-squares fit.
	Slope = Reduction{Name: "slope", Min: 2, fold: foldSlope}
	// DeltaRatio divides the numerator counter's delta by the denominator
	// counter's delta, both taken at both window edges. It is the only
	// reduction that reads Point.Against, and therefore the only one that
	// declares against.
	DeltaRatio = Reduction{Name: "deltaRatio", Min: 2, fold: foldDeltaRatio, against: true}
	// P95 is the nearest-rank 95th percentile. Below twenty samples the rank
	// ceil(0.95n) equals n, so the percentile IS the maximum.
	P95 = Reduction{Name: "p95", Min: 20, fold: foldP95, ordered: true}
	// P99 is the nearest-rank 99th percentile. Its minimum of 100 exceeds what
	// a 60s window at 1s can hold, so NewEngine refuses it at that cadence.
	P99 = Reduction{Name: "p99", Min: 100, fold: foldP99, ordered: true}
)

// NewReduction builds a seventh reduction. It refuses a minimum below one and
// a nil fold — the same two checks NewEngine re-runs on every reduction in
// the table, so both are checked twice on purpose.
//
// A reduction built here folds a SINGLE series: against stays false, so a
// window under it stores points whose Against is absent.
func NewReduction(name string, min int, fold func([]Point) float64) (Reduction, error) {
	if min < 1 {
		return Reduction{}, fmt.Errorf("reduction %q: minimum sample count %d is below one", name, min)
	}
	if fold == nil {
		return Reduction{}, fmt.Errorf("reduction %q: nil fold", name)
	}

	return Reduction{Name: name, Min: min, fold: fold}, nil
}

// foldLast is the newest entry.
func foldLast(points []Point) float64 { return points[len(points)-1].Value }

// foldMean is the arithmetic mean of the stored values.
func foldMean(points []Point) float64 {
	var sum float64
	for _, p := range points {
		sum += p.Value
	}
	return sum / float64(len(points))
}

// foldSlope is (last value − first value) over (last time − first time) in
// seconds, using the window's own first and last timestamps.
func foldSlope(points []Point) float64 {
	first, last := points[0], points[len(points)-1]
	dt := last.At.Sub(first.At).Seconds()

	return (last.Value - first.Value) / dt
}

// foldDeltaRatio is (numerator_last − numerator_first) over
// (denominator_last − denominator_first), both counters at both window edges.
// The denominator delta is well-defined because Reduce gates on a zero or
// negative delta before folding.
func foldDeltaRatio(points []Point) float64 {
	first, last := points[0], points[len(points)-1]
	firstD, _ := first.Against.Get()
	lastD, _ := last.Against.Get()

	return (last.Value - first.Value) / (lastD - firstD)
}

// nearestRank folds to the nearest-rank percentile: the value at the
// 1-indexed rank ceil(p·n), sorted ascending.
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

// foldP95 is the nearest-rank 95th percentile.
var foldP95 = nearestRank(0.95)

// foldP99 is the nearest-rank 99th percentile.
var foldP99 = nearestRank(0.99)
