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

package benthosserviceconfig

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// Cost of canonicalizing one config, walk against YAML round-trip, at several
// config sizes. Benchmarks are skipped unless -bench is passed, so this costs a
// plain `go test` run nothing.
//
// Both paths are the same production canonicalize, run with useCanonicalizeFast
// toggled. Timing a copy of the old implementation kept in this file would drift
// from production silently, and did: the copy walked four of the six sections.

// configSizes are the bridgeConfig block counts the tables sweep. 10 blocks is
// roughly a small bridge, 1000 an implausibly large one; the middle of the range
// is where real configs sit.
var configSizes = []int{10, 50, 100, 250, 500, 1000}

// withWalkGate runs f with the walk forced on or off, restoring the process-wide
// setting afterwards. Benchmarks in this file are sequential, so a package
// variable is safe to swap.
func withWalkGate(on bool, f func()) {
	prev := useCanonicalizeFast
	useCanonicalizeFast = on

	defer func() { useCanonicalizeFast = prev }()

	f()
}

// BenchmarkCanonicalize is the real measurement: one sub-benchmark per config
// size and path, honoring b.N so the numbers can be fed to benchstat.
//
//	go test -run XXX -bench BenchmarkCanonicalize/ -count=10 ./pkg/config/benthosserviceconfig/ > new.txt
//	benchstat new.txt
//
// Prefer this over the report below whenever a number has to be defended: it
// reports ns/op and allocations per call, and benchstat gives the spread that a
// single printed figure hides.
func BenchmarkCanonicalize(b *testing.B) {
	for _, blocks := range configSizes {
		cfg := NewNormalizer().NormalizeConfig(bridgeConfig(blocks))

		encoded, err := marshalConfig(cfg)
		if err != nil {
			b.Fatal(err)
		}

		for _, path := range []struct {
			name string
			walk bool
		}{{"walk", true}, {"roundtrip", false}} {
			b.Run(fmt.Sprintf("%dB/%s", len(encoded), path.name), func(b *testing.B) {
				withWalkGate(path.walk, func() {
					b.ReportAllocs()
					b.ResetTimer()

					for range b.N {
						_ = canonicalize(cfg)
					}
				})
			})
		}
	}
}

// reportSamples is how many times each cell of the tables below is measured. The
// walk is fast enough that a single sample swung its reported speedup by ±20%
// across runs of unchanged code, so the tables print a median and a range.
const reportSamples = 5

// The per-tick projection. bridges and tickBudget are real; comparisonsPerBridge
// is an estimate from reading the protocolconverter -> dataflowcomponent -> benthos
// call paths and was never measured, so caveat() prints it alongside the numbers it
// produces. sidesPerComparison is 2 because the path this replaced canonicalized
// both sides of every comparison; the current one usually canonicalizes only the
// desired side, which makes the walk column an upper bound.
const (
	bridges              = 41
	comparisonsPerBridge = 8
	sidesPerComparison   = 2
	tickBudget           = 100 * time.Millisecond

	passesPerTick = time.Duration(bridges * comparisonsPerBridge * sidesPerComparison)
)

// sampleSet holds one cell's measurements.
type sampleSet []time.Duration

func (s sampleSet) median() time.Duration {
	sorted := append(sampleSet(nil), s...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	return sorted[len(sorted)/2]
}

func (s sampleSet) span() (low, high time.Duration) {
	low, high = s[0], s[0]

	for _, d := range s {
		if d < low {
			low = d
		}

		if d > high {
			high = d
		}
	}

	return low, high
}

// timeIt returns the average cost of one call, warming up first and then running
// until both a minimum count and a minimum wall time are reached.
//
// A fixed iteration count measured the sub-millisecond walk so poorly that its
// reported speedup swung between 5x and 317x across runs of unchanged code.
// testing.Benchmark would size the loop properly but deadlocks when called from
// inside a running benchmark, so the sizing is done here.
func timeIt(f func()) time.Duration {
	const (
		warmup    = 3
		minRuns   = 10
		minsample = 200 * time.Millisecond
	)

	for range warmup {
		f()
	}

	runs := 0
	start := time.Now()

	for runs < minRuns || time.Since(start) < minsample {
		f()

		runs++
	}

	return time.Since(start) / time.Duration(runs)
}

// sampleBoth measures both paths reportSamples times, alternating between them so
// a machine that slows down partway through skews both columns equally.
func sampleBoth(cfg BenthosServiceConfig) (walk, roundTrip sampleSet) {
	for range reportSamples {
		withWalkGate(false, func() { roundTrip = append(roundTrip, timeIt(func() { _ = canonicalize(cfg) })) })
		withWalkGate(true, func() { walk = append(walk, timeIt(func() { _ = canonicalize(cfg) })) })
	}

	return walk, roundTrip
}

func caveat() string {
	return fmt.Sprintf(
		"  Cost of one canonicalize call: median of %d samples, full range in brackets.\n"+
			"  /tick projects that onto %d passes per tick (%d bridges x %d comparisons x %d\n"+
			"  sides against a %v tick). CAVEAT: the %d comparisons per bridge were read off\n"+
			"  the call paths, never measured, so every /tick figure scales with a guess and\n"+
			"  is an order of magnitude at best. Quote the per-call columns, not /tick, and\n"+
			"  use BenchmarkCanonicalize with benchstat for a number that has to be defended.\n",
		reportSamples, passesPerTick, bridges, comparisonsPerBridge, sidesPerComparison,
		tickBudget, comparisonsPerBridge)
}

// The table is printed once per process. Ignoring b.N means the testing package
// keeps re-invoking the function to reach its time target, which printed the whole
// table eleven times in a default -bench run; the reported ns/op is meaningless
// here and the table is the actual output.
var costTableOnce sync.Once

// BenchmarkCanonicalizeCostReport prints a readable cost table rather than
// measuring anything benchstat can consume. It carries the Benchmark prefix only
// so that a plain `go test` run skips it.
func BenchmarkCanonicalizeCostReport(b *testing.B) {
	costTableOnce.Do(func() {
		fmt.Printf("\ncanonicalize cost by config size\n%s", caveat())
		fmt.Printf("  %10s | %26s | %26s | %9s | %13s | %s\n",
			"cfg size", "round-trip", "walk", "speedup", "round-trip/tick", "walk/tick")

		for _, blocks := range configSizes {
			cfg := NewNormalizer().NormalizeConfig(bridgeConfig(blocks))

			encoded, err := marshalConfig(cfg)
			if err != nil {
				b.Fatal(err)
			}

			walk, roundTrip := sampleBoth(cfg)

			fmt.Printf("  %9dB | %26s | %26s | %9s | %15v | %v\n",
				len(encoded), format(roundTrip), format(walk), speedup(roundTrip, walk),
				(roundTrip.median() * passesPerTick).Round(time.Millisecond),
				(walk.median() * passesPerTick).Round(time.Millisecond))
		}
	})
}

// format renders a cell as "median [low-high]".
func format(s sampleSet) string {
	low, high := s.span()

	return fmt.Sprintf("%v [%v-%v]",
		s.median().Round(time.Microsecond), low.Round(time.Microsecond), high.Round(time.Microsecond))
}

// speedup reports the range the ratio can take, not a single number: the two
// medians divided would read as a precision the samples do not support.
func speedup(slow, fast sampleSet) string {
	slowLow, slowHigh := slow.span()
	fastLow, fastHigh := fast.span()

	return fmt.Sprintf("%.0f-%.0fx", float64(slowLow)/float64(fastHigh), float64(slowHigh)/float64(fastLow))
}
