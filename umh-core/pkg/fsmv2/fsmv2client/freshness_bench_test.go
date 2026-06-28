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

package fsmv2client_test

import (
	"context"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
)

// sink defeats compiler elision of the GetFresh return values across the
// benchmark loop. It is package-scoped so the compiler cannot prove the calls
// are dead.
var sink testStatus

// freshBenchRef is the ref registered for the GetFresh benchmarks. It is built
// once per benchmark setup and reused across iterations so the registry lookup
// (Contains) is the only per-op cost on the registration path.
var freshBenchRef = dynamicchildren.Ref{WorkerType: "benthos_monitor", Name: "benthos-bridge-bench"}

// freshBenchMaxAge is wide enough that the staged observation is always Fresh,
// so the benchmark exercises the full happy path (Contains + LoadObservedTyped
// + time.Since) on every iteration.
const freshBenchMaxAge = 10 * time.Second

// stageFreshObservation builds a registered ref with a fresh observation staged
// in the stub reader, returning the client ready for GetFresh calls.
func stageFreshObservation(b *testing.B) *fsmv2client.FSMv2Client {
	b.Helper()

	writer := dynamicchildren.NewWriter()
	if err := writer.Upsert(freshBenchRef, map[string]any{}); err != nil {
		b.Fatalf("writer.Upsert: %v", err)
	}

	obs := &fsmv2.Observation[testStatus]{
		CollectedAt: time.Now().Add(-1 * time.Second),
		Status:      testStatus{V: "observed"},
	}

	stubSr := &stubStateReader{obs: obs}

	return fsmv2client.NewFSMv2Client(writer, stubSr)
}

// BenchmarkContains measures the non-allocating existence check that GetFresh
// uses for its Unregistered guard. Contains must stay at 0 allocs/op; a
// regression here means someone reintroduced a Clone on the read path.
func BenchmarkContains(b *testing.B) {
	writer := dynamicchildren.NewWriter()
	if err := writer.Upsert(freshBenchRef, map[string]any{}); err != nil {
		b.Fatalf("writer.Upsert: %v", err)
	}

	reg := writer.Registry()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if !reg.Contains(freshBenchRef) {
			b.Fatal("Contains returned false for a registered ref")
		}
	}
}

// BenchmarkGetFresh_Fresh measures the full GetFresh happy path: registry
// Contains + stub LoadObservedTyped + time.Since classification. Run with
//
//	go test ./pkg/fsmv2/fsmv2client/... -bench=BenchmarkGetFresh_Fresh -benchmem -count=10 | benchstat
//
// and compare against BenchmarkGetFresh_Baseline to confirm the GetFresh
// overhead is a small multiple of the bare store read.
func BenchmarkGetFresh_Fresh(b *testing.B) {
	client := stageFreshObservation(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		sink, _, _ = fsmv2client.GetFresh[testStatus](ctx, client, freshBenchRef, freshBenchMaxAge)
	}
}

// BenchmarkGetFresh_Baseline measures the bare stubStateReader.LoadObservedTyped
// call in isolation. A developer can run both benchmarks together
// (go test -bench=. -count=10 | benchstat) and eyeball the ratio
// GetFresh_Fresh / GetFresh_Baseline: the GetFresh wrapper should sit at a
// small multiple (~2-3x) of this baseline, confirming the registry Contains
// guard adds negligible overhead over the store read itself.
func BenchmarkGetFresh_Baseline(b *testing.B) {
	writer := dynamicchildren.NewWriter()
	if err := writer.Upsert(freshBenchRef, map[string]any{}); err != nil {
		b.Fatalf("writer.Upsert: %v", err)
	}

	obs := &fsmv2.Observation[testStatus]{
		CollectedAt: time.Now().Add(-1 * time.Second),
		Status:      testStatus{V: "observed"},
	}

	stubSr := &stubStateReader{obs: obs}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		var out fsmv2.Observation[testStatus]
		if err := stubSr.LoadObservedTyped(ctx, freshBenchRef.WorkerType, freshBenchRef.Name, &out); err != nil {
			b.Fatalf("LoadObservedTyped: %v", err)
		}

		sink = out.Status
	}
}

// TestContains_ZeroAllocs is the CI gate for the Contains existence check this
// PR introduces. Contains must not allocate; a non-zero result means the
// read-path existence check has regressed to cloning (e.g. someone replaced
// Contains with Lookup, or added a Clone inside Contains).
func TestContains_ZeroAllocs(t *testing.T) {
	res := testing.Benchmark(BenchmarkContains)
	if res.AllocsPerOp() > 0 {
		t.Fatalf("Contains regressed to %d allocs/op (expected 0)", res.AllocsPerOp())
	}
}

// TestGetFresh_AllocFloor is the CI gate for the full GetFresh path. The
// production floor is 2 allocs/op, both on the Get store-read path (none from
// the registry existence check, which is the non-allocating Contains):
//
//   - 1 alloc: the Observation[TStatus] in Get escapes to the heap because its
//     address is passed through the StateReader interface (the compiler cannot
//     devirtualize a virtual call, so &obs escapes). Fixable only by making
//     StateReader a concrete type, which is outside this PR's scope.
//   - 1 alloc: config.ChildID(ref.Name) concatenates name+"-001", allocating
//     the result string. Fixable only by changing the child-id format, which
//     is outside this PR's scope.
//
// The benchmark sink is typed (testStatus, not any) so it adds zero harness
// allocs — the gate measures the real production floor. Bump the threshold
// only if a deliberate change to Get or ChildID adds a documented allocation;
// investigate first, because a regression to 3+ likely means someone
// reintroduced a Clone or an extra alloc on the read path.
func TestGetFresh_AllocFloor(t *testing.T) {
	res := testing.Benchmark(BenchmarkGetFresh_Fresh)
	if res.AllocsPerOp() > 2 {
		t.Fatalf("GetFresh regressed to %d allocs/op (floor is 2: 1 obs-escape via StateReader interface + 1 from config.ChildID string concat; the registry Contains guard is 0-alloc — see TestContains_ZeroAllocs)", res.AllocsPerOp())
	}
}
