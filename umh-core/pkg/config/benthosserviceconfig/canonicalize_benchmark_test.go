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
	"sync"
	"testing"
	"time"
)

// Cost of canonicalization, old path vs new, at several config sizes and bridge
// counts. Kept out of the Ginkgo suite so it does not add wall time to every
// `go test` run: use `go test -run XXX -bench Canonicalize`.
//
// CAVEAT: comparisonsPerBridgePerTick is an estimate from reading the
// protocolconverter -> dataflowcomponent -> benthos call paths, NOT a measured
// count. The speedup ratio does not depend on it; every per-tick figure scales
// directly with it and is an order of magnitude at best.
const (
	comparisonsPerBridgePerTick = 8
	sidesPerComparison          = 2
	tickBudget                  = 100 * time.Millisecond
)

// timeIt returns the average cost of one call, warming up first and then running
// until both a minimum count and a minimum wall time are reached.
//
// A fixed iteration count measured the sub-millisecond fast path so poorly that
// its reported speedup swung between 5x and 317x across runs of unchanged code.
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

// benchOne times both paths on the same normalized config. The new path is timed as
// the reconcile loop meets it, with its cache warm, because an unchanged config is
// what it canonicalizes on nearly every tick.
func benchOne(cfg BenthosServiceConfig) (slow, fast time.Duration) {
	norm := NewNormalizer().NormalizeConfig(cfg)

	slow = timeIt(func() { sectionRoundTrip(norm) })
	fast = timeIt(func() { _ = canonicalize(norm) })

	return slow, fast
}

func budgetNote(d time.Duration) string {
	if d > tickBudget {
		return "OVER BUDGET"
	}

	return "ok"
}

// The two tables are printed once per process. Ignoring b.N means the testing
// package keeps re-invoking the benchmark to reach its time target, which
// printed the whole table eleven times in a default `-bench` run; the reported
// ns/op is meaningless here and the tables below are the actual output.
var (
	configSizeTableOnce  sync.Once
	bridgeCountTableOnce sync.Once
)

// BenchmarkCanonicalizeByConfigSize holds the bridge count at 41 and grows the
// per-bridge config.
func BenchmarkCanonicalizeByConfigSize(b *testing.B) {
	configSizeTableOnce.Do(func() {
		const bridges = 41

		passes := time.Duration(bridges * comparisonsPerBridgePerTick * sidesPerComparison)

		fmt.Printf("\n%d bridges, growing per-bridge config size (%d canonicalize passes per tick)\n",
			bridges, passes)
		fmt.Printf("  %10s | %12s | %12s | %8s | %-14s | %s\n",
			"cfg size", "old/tick", "new/tick", "speedup", "old vs 100ms", "new vs 100ms")

		for _, blocks := range []int{10, 50, 100, 250, 500, 1000} {
			cfg := bridgeConfig(blocks)

			encoded, err := marshalConfig(cfg)
			if err != nil {
				b.Fatal(err)
			}

			slow, fast := benchOne(cfg)
			slowTick, fastTick := slow*passes, fast*passes

			fmt.Printf("  %9dB | %12v | %12v | %7.0fx | %-14s | %s\n",
				len(encoded), slowTick.Round(time.Microsecond), fastTick.Round(time.Microsecond),
				float64(slow)/float64(fast), budgetNote(slowTick), budgetNote(fastTick))
		}
	})
}

// BenchmarkConfigsEqualPerTick times what the reconcile loop actually pays: a
// comparison that finds no difference, of a config against the config read back from
// the file it renders to.
//
// The two desired-config variants are the two ways a component gets its config.
// "templated" arrived parsed from rendered template text, which is what a bridge
// has, and needs no canonicalization at all. "go-built" was assembled in Go, so it
// has to be canonicalized before it can match the file. "old" is the comparison
// this package did before: both sides re-serialized per section, every time.
func BenchmarkConfigsEqualPerTick(b *testing.B) {
	oldConfigsEqual := func(desired, observed BenthosServiceConfig) bool {
		norm := NewNormalizer()

		return configsEqualNormalized(
			sectionRoundTrip(norm.NormalizeConfig(desired)),
			sectionRoundTrip(norm.NormalizeConfig(observed)))
	}

	for _, blocks := range []int{10, 100, 500} {
		goBuilt := bridgeConfig(blocks)

		observed, err := renderAndParse(goBuilt)
		if err != nil {
			b.Fatal(err)
		}

		templated, err := templatedFrom(goBuilt)
		if err != nil {
			b.Fatal(err)
		}

		encoded, err := marshalConfig(goBuilt)
		if err != nil {
			b.Fatal(err)
		}

		cases := []struct {
			name    string
			desired BenthosServiceConfig
			equal   func(desired, observed BenthosServiceConfig) bool
		}{
			{"old", goBuilt, oldConfigsEqual},
			{"go-built", goBuilt, ConfigsEqual},
			{"templated", templated, ConfigsEqual},
		}

		for _, c := range cases {
			b.Run(fmt.Sprintf("bytes=%d/desired=%s", len(encoded), c.name), func(b *testing.B) {
				if !c.equal(c.desired, observed) {
					b.Fatal("fixture is not equal to its own file; this would time the difference path")
				}

				b.ReportAllocs()

				for b.Loop() {
					c.equal(c.desired, observed)
				}
			})
		}
	}
}

// BenchmarkCanonicalizeByBridgeCount holds the per-bridge config at customer size
// and grows the number of bridges.
func BenchmarkCanonicalizeByBridgeCount(b *testing.B) {
	bridgeCountTableOnce.Do(func() {
		cfg := bridgeConfigOfBytes(42 * 1024)

		encoded, err := marshalConfig(cfg)
		if err != nil {
			b.Fatal(err)
		}

		slow, fast := benchOne(cfg)

		fmt.Printf("\n%d-byte bridge config, growing bridge count\n", len(encoded))
		fmt.Printf("  %8s | %12s | %12s | %-14s | %s\n",
			"bridges", "old/tick", "new/tick", "old vs 100ms", "new vs 100ms")

		for _, bridges := range []int{1, 5, 10, 20, 41, 80} {
			passes := time.Duration(bridges * comparisonsPerBridgePerTick * sidesPerComparison)
			slowTick, fastTick := slow*passes, fast*passes

			fmt.Printf("  %8d | %12v | %12v | %-14s | %s\n",
				bridges, slowTick.Round(time.Microsecond), fastTick.Round(time.Microsecond),
				budgetNote(slowTick), budgetNote(fastTick))
		}
	})
}
