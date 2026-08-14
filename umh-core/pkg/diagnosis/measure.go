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

// This file holds the numbers: a Reading (a value or an absence), a Point (one
// stored reading), and a Reduction, which is a calculation over the stored
// readings such as an average, a slope, or a p95.

package diagnosis

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Reading is an optional float64: either a value or an absence. The zero
// Reading is an absence; a known zero is a value.
type Reading struct {
	v  float64
	ok bool
}

// Known returns a Reading carrying a value.
func Known(f float64) Reading { return Reading{v: f, ok: true} }

// Unknown returns a Reading carrying an absence; it is the zero Reading.
func Unknown() Reading { return Reading{} }

// Get returns the value and whether it is present; an absent Reading returns 0.
func (r Reading) Get() (float64, bool) { return r.v, r.ok }

// Point is one stored reading: the instant it was taken, its value, and the
// denominator a ratio divides by.
type Point struct {
	At      time.Time
	Value   float64
	Against Reading
}

// State is how much the readings behind a reduced number can be trusted. The
// iota order runs StateAbsent < StateUntrusted < StateValue.
type State int

const (
	// StateAbsent means nothing is stored, so there is no number to reduce.
	StateAbsent State = iota
	// StateUntrusted means readings are stored but the number is not worth acting
	// on: fewer readings than the calculation needs, the latest read stored
	// nothing, a denominator whose delta is zero or negative, or a NaN or
	// infinite result.
	StateUntrusted
	// StateValue means a finite number over enough readings, the most recent read
	// among them.
	StateValue
)

// Reduced is a reduced number bound to the State that says whether to trust it.
type Reduced struct {
	v     float64
	state State
}

// Get returns the reduced number and its State, together and only together.
func (r Reduced) Get() (float64, State) { return r.v, r.state }

// Reduction is a calculation over the stored readings, such as an average, a
// slope, or a p95, plus the fewest readings below which its result is untrusted.
type Reduction struct {
	fold func([]Point) float64
	Name string
	Min  int
	// ordered is true for a calculation that sorts its input: only a percentile does.
	ordered bool
	// divides is true for a calculation that divides by Point.Against, which is
	// what makes an absent denominator mean opposite things: under Mean the
	// reading is stored, under DeltaRatio it is dropped.
	divides bool
}

var (
	// Last is the newest stored reading. Min is 1: one reading is the whole answer.
	Last = Reduction{Name: "last", Min: 1, fold: foldLast}
	// Mean is the arithmetic mean of the stored values. Min is 2; over one it is Last.
	Mean = Reduction{Name: "mean", Min: 2, fold: foldMean}
	// Slope is (v_last − v_first) / (t_last − t_first) in seconds over the oldest
	// and newest readings, never a least-squares fit. Min is 2 for a time base.
	Slope = Reduction{Name: "slope", Min: 2, fold: foldSlope}
	// DeltaRatio divides the numerator's delta by the denominator's delta, both
	// oldest to newest. Min is 2: over one reading both deltas are zero.
	DeltaRatio = Reduction{Name: "deltaRatio", Min: 2, fold: foldDeltaRatio, divides: true}
	// P95 is the nearest-rank 95th percentile. Min is 20: below twenty readings
	// the rank ceil(0.95n) equals n, so the percentile IS the maximum.
	P95 = Reduction{Name: "p95", Min: 20, fold: foldP95, ordered: true}
	// P99 is the nearest-rank 99th percentile, Min 100 for the same reason.
	P99 = Reduction{Name: "p99", Min: 100, fold: foldP99, ordered: true}
)

// NewReduction builds a seventh calculation over a single series: divides stays
// false. It refuses a minimum below one and a nil function.
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

// foldDeltaRatio requires a positive denominator delta; zero divides by zero.
func foldDeltaRatio(points []Point) float64 {
	first, last := points[0], points[len(points)-1]
	firstD, _ := first.Against.Get()
	lastD, _ := last.Against.Get()

	return (last.Value - first.Value) / (lastD - firstD)
}

// nearestRank builds a nearest-rank percentile: the value at 1-indexed rank
// ceil(p·n) of the sorted values. It sorts a copy, leaving points in time order.
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
