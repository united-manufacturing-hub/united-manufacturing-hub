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

package supervisor_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"go.uber.org/zap"
)

// BenchmarkSupervisorTick100Workers measures supervisor tick loop performance with 100 workers.
// This establishes a baseline for expected single-tick latency at moderate scale.
func BenchmarkSupervisorTick100Workers(b *testing.B) {
	benchmarkSupervisorTick(b, 100)
}

// BenchmarkSupervisorTick500Workers measures supervisor tick loop performance with 500 workers.
// This tests scaling behavior at medium scale - should be ~5x baseline latency.
func BenchmarkSupervisorTick500Workers(b *testing.B) {
	benchmarkSupervisorTick(b, 500)
}

// BenchmarkSupervisorTick1000Workers measures supervisor tick loop performance with 1000 workers.
// This validates performance at expected maximum scale - should be ~10x baseline latency.
func BenchmarkSupervisorTick1000Workers(b *testing.B) {
	benchmarkSupervisorTick(b, 1000)
}

// benchmarkSupervisorTick is the core benchmark implementation.
// It creates N workers and measures time to execute one complete tick loop across all workers.
//
// Key metrics:
// - Time per tick (should scale linearly with worker count)
// - Memory allocation per worker (should remain constant)
// - Throughput (workers processed per second)
//
// Using mockWorker to minimize external overhead - this isolates supervisor tick loop performance.
func benchmarkSupervisorTick(b *testing.B, workerCount int) {
	ctx := context.Background()

	// Create supervisor with triangular store
	triangularStore := createTestTriangularStore()
	s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     zap.NewNop().Sugar(),
	})

	// Add N workers to supervisor
	for i := range workerCount {
		workerID := fmt.Sprintf("worker-%d", i)
		identity := fsmv2.Identity{
			ID:   workerID,
			Name: fmt.Sprintf("Benchmark Worker %d", i),
		}

		// Use simple mock worker to minimize overhead
		worker := &mockWorker{
			observed: &mockObservedState{
				ID:          workerID,
				CollectedAt: time.Now(),
				Desired:     &mockDesiredState{},
			},
		}

		err := s.AddWorker(identity, worker)
		if err != nil {
			b.Fatalf("Failed to add worker %s: %v", workerID, err)
		}
	}

	// Verify all workers were added
	workers := s.ListWorkers()
	if len(workers) != workerCount {
		b.Fatalf("Expected %d workers, got %d", workerCount, len(workers))
	}

	// Reset timer to exclude setup time
	b.ResetTimer()

	// Benchmark the tick loop
	for range b.N {
		err := s.TestTickAll(ctx)
		if err != nil {
			b.Fatalf("Tick failed: %v", err)
		}
	}

	// Report custom metrics
	// ns/op is automatic, but we also want workers/sec
	b.ReportMetric(float64(workerCount), "workers")
}
