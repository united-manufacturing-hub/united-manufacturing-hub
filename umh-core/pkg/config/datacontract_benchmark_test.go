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

package config

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// The existing parse benchmarks use fixtures with neither contract section, so they
// gate nothing here. These two cover the paths the merge actually made more
// expensive:
//
//   - parse, which now absorbs and then round-trips the result to verify it
//   - Clone, which runs on every GetConfig fast path and now deep-copies the
//     contracts as well
//
// Fifty contracts is well past what any real instance carries; the point is that the
// per-tick cost stays negligible at that size rather than to model a typical one.
const benchmarkContractCount = 50

func benchmarkContractYAML(count int) []byte {
	var b strings.Builder

	b.WriteString("agent:\n  metricsPort: 8080\ndataContracts:\n")

	for i := range count {
		fmt.Fprintf(&b, `  - model: model%d
    description: benchmark model %d
    versions:
      v1:
        name: _model%d_v1
        structure:
          temperature:
            _payloadshape: timeseries-number
          pressure:
            _payloadshape: timeseries-number
          nested:
            depth:
              _payloadshape: timeseries-number
`, i, i, i)
	}

	return []byte(b.String())
}

func BenchmarkParseConfigWithContracts(b *testing.B) {
	data := benchmarkContractYAML(benchmarkContractCount)
	ctx := context.Background()

	// Confirm the fixture is doing what the benchmark claims before measuring it.
	parsed, err := ParseConfig(data, ctx, false)
	if err != nil {
		b.Fatalf("fixture must parse: %v", err)
	}

	if len(parsed.Contracts) != benchmarkContractCount {
		b.Fatalf("expected %d contracts, got %d", benchmarkContractCount, len(parsed.Contracts))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := ParseConfig(data, ctx, false); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCloneWithContracts is the one that matters for steady state: parse happens
// when the file changes, Clone happens on every tick.
func BenchmarkCloneWithContracts(b *testing.B) {
	parsed, err := ParseConfig(benchmarkContractYAML(benchmarkContractCount), context.Background(), false)
	if err != nil {
		b.Fatalf("fixture must parse: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		clone := parsed.Clone()
		if len(clone.Contracts) != benchmarkContractCount {
			b.Fatal("clone dropped contracts")
		}
	}
}

// BenchmarkContractsAreLossless isolates the self-check, since it is the part of
// parse that the merge added rather than made bigger.
func BenchmarkContractsAreLossless(b *testing.B) {
	parsed, err := ParseConfig(benchmarkContractYAML(benchmarkContractCount), context.Background(), false)
	if err != nil {
		b.Fatalf("fixture must parse: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if !ContractsAreLossless(parsed.Contracts) {
			b.Fatal("fixture must be lossless")
		}
	}
}
