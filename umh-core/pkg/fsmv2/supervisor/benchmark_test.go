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
	"sync"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"go.uber.org/zap"
)

// =============================================================================
// Tick Loop Responsiveness Benchmarks
// =============================================================================
// These benchmarks establish baselines for tick loop performance under varying
// worker counts. Key expectations:
// - Tick latency should scale linearly with worker count (not exponentially)
// - Each worker should add approximately constant overhead
// - Memory allocation per worker should remain stable
// =============================================================================

// BenchmarkSupervisorTick1Worker measures baseline tick performance with 1 worker.
// This establishes the minimum overhead of a single tick loop iteration.
func BenchmarkSupervisorTick1Worker(b *testing.B) {
	benchmarkSupervisorTick(b, 1)
}

// BenchmarkSupervisorTick10Workers measures tick performance at small scale.
// Should be ~10x baseline latency if scaling is linear.
func BenchmarkSupervisorTick10Workers(b *testing.B) {
	benchmarkSupervisorTick(b, 10)
}

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
		identity := deps.Identity{
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
	b.ReportAllocs()

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

// =============================================================================
// Long-Running Action Isolation Benchmarks
// =============================================================================
// These benchmarks verify that long-running actions in workers don't block
// the supervisor tick loop. The tick loop should remain responsive even when
// individual workers are performing slow operations.
// =============================================================================

// slowCollectWorker simulates a worker with slow observation collection.
// This helps test that the tick loop remains responsive during slow operations.
type slowCollectWorker struct {
	collectDelay time.Duration
	observed     *mockObservedState
}

func (w *slowCollectWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	// Simulate slow collection (e.g., network timeout, slow database query)
	select {
	case <-time.After(w.collectDelay):
		return w.observed, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *slowCollectWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &mockDesiredState{}, nil
}

func (w *slowCollectWorker) GetInitialState() fsmv2.State[any, any] {
	return &mockState{}
}

// BenchmarkTickLoopWithSlowWorker measures tick loop latency when one worker is slow.
// Expectations:
// - Tick loop should NOT wait for slow worker indefinitely.
// - Total tick time should be bounded by timeout, not by worker delay.
// - This verifies action isolation is working correctly.
func BenchmarkTickLoopWithSlowWorker(b *testing.B) {
	ctx := context.Background()

	triangularStore := createTestTriangularStore()
	s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     zap.NewNop().Sugar(),
	})

	// Add one slow worker (100ms delay) and several fast workers
	slowWorkerID := "slow-worker-0"
	slowIdentity := deps.Identity{
		ID:   slowWorkerID,
		Name: "Slow Worker",
	}
	slowWorker := &slowCollectWorker{
		collectDelay: 100 * time.Millisecond,
		observed: &mockObservedState{
			ID:          slowWorkerID,
			CollectedAt: time.Now(),
			Desired:     &mockDesiredState{},
		},
	}

	err := s.AddWorker(slowIdentity, slowWorker)
	if err != nil {
		b.Fatalf("Failed to add slow worker: %v", err)
	}

	// Add 10 fast workers
	for i := 1; i <= 10; i++ {
		workerID := fmt.Sprintf("fast-worker-%d", i)
		identity := deps.Identity{
			ID:   workerID,
			Name: fmt.Sprintf("Fast Worker %d", i),
		}

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

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		// Use a short timeout context to ensure we don't wait forever
		tickCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		_ = s.TestTickAll(tickCtx)

		cancel()
	}
}

// BenchmarkTickLoopConcurrentActions measures tick performance when multiple
// workers execute actions concurrently.
func BenchmarkTickLoopConcurrentActions(b *testing.B) {
	ctx := context.Background()

	triangularStore := createTestTriangularStore()
	s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     zap.NewNop().Sugar(),
	})

	// Add 50 workers with small random delays to simulate real-world variance
	for i := range 50 {
		workerID := fmt.Sprintf("concurrent-worker-%d", i)
		identity := deps.Identity{
			ID:   workerID,
			Name: fmt.Sprintf("Concurrent Worker %d", i),
		}

		// Small delay (1-5ms) to simulate realistic collection variance
		worker := &slowCollectWorker{
			collectDelay: time.Duration(1+i%5) * time.Millisecond,
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

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		tickCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		_ = s.TestTickAll(tickCtx)

		cancel()
	}

	b.ReportMetric(50, "workers")
}

// =============================================================================
// Observation Collection Latency Benchmarks
// =============================================================================
// These benchmarks measure the time from observation change to observed state
// update being reflected in the supervisor. This is critical for ensuring
// timely state transitions and responsive system behavior.
// =============================================================================

// observationLatencyWorker tracks when observations are collected to measure latency.
type observationLatencyWorker struct {
	mu            sync.Mutex
	lastCollected time.Time
	observed      *mockObservedState
}

func (w *observationLatencyWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	w.mu.Lock()
	w.lastCollected = time.Now()
	w.observed.CollectedAt = w.lastCollected
	w.mu.Unlock()

	return w.observed, nil
}

func (w *observationLatencyWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &mockDesiredState{}, nil
}

func (w *observationLatencyWorker) GetInitialState() fsmv2.State[any, any] {
	return &mockState{}
}

func (w *observationLatencyWorker) GetLastCollected() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.lastCollected
}

// BenchmarkObservationCollectionSingle measures observation collection latency
// for a single worker. This establishes the baseline collection overhead.
func BenchmarkObservationCollectionSingle(b *testing.B) {
	ctx := context.Background()

	triangularStore := createTestTriangularStore()
	s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     zap.NewNop().Sugar(),
	})

	workerID := "latency-worker-0"
	identity := deps.Identity{
		ID:   workerID,
		Name: "Latency Worker",
	}

	worker := &observationLatencyWorker{
		observed: &mockObservedState{
			ID:          workerID,
			CollectedAt: time.Now(),
			Desired:     &mockDesiredState{},
		},
	}

	err := s.AddWorker(identity, worker)
	if err != nil {
		b.Fatalf("Failed to add worker: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		startTime := time.Now()
		_ = s.TestTickAll(ctx)
		collectionTime := worker.GetLastCollected()

		// Report the delta between tick start and observation collection
		latencyNs := collectionTime.Sub(startTime).Nanoseconds()
		if latencyNs > 0 {
			b.ReportMetric(float64(latencyNs), "collection-latency-ns")
		}
	}
}

// BenchmarkObservationCollectionMultiple measures observation collection latency
// across multiple workers to understand parallel collection overhead.
func BenchmarkObservationCollectionMultiple(b *testing.B) {
	benchmarkObservationCollectionN(b, 10)
}

// BenchmarkObservationCollection100Workers measures observation collection latency
// at moderate scale.
func BenchmarkObservationCollection100Workers(b *testing.B) {
	benchmarkObservationCollectionN(b, 100)
}

func benchmarkObservationCollectionN(b *testing.B, workerCount int) {
	ctx := context.Background()

	triangularStore := createTestTriangularStore()
	s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     zap.NewNop().Sugar(),
	})

	workers := make([]*observationLatencyWorker, workerCount)

	for i := range workerCount {
		workerID := fmt.Sprintf("latency-worker-%d", i)
		identity := deps.Identity{
			ID:   workerID,
			Name: fmt.Sprintf("Latency Worker %d", i),
		}

		workers[i] = &observationLatencyWorker{
			observed: &mockObservedState{
				ID:          workerID,
				CollectedAt: time.Now(),
				Desired:     &mockDesiredState{},
			},
		}

		err := s.AddWorker(identity, workers[i])
		if err != nil {
			b.Fatalf("Failed to add worker %s: %v", workerID, err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		startTime := time.Now()
		_ = s.TestTickAll(ctx)

		// Measure max latency across all workers (worst case)
		var maxLatency time.Duration

		for _, w := range workers {
			latency := w.GetLastCollected().Sub(startTime)
			if latency > maxLatency {
				maxLatency = latency
			}
		}

		b.ReportMetric(float64(maxLatency.Nanoseconds()), "max-latency-ns")
	}

	b.ReportMetric(float64(workerCount), "workers")
}

// =============================================================================
// Tick Loop Scaling Analysis Benchmarks
// =============================================================================
// These benchmarks help verify that tick loop scaling is linear, not exponential.
// Run with: go test -bench=BenchmarkTickScaling -benchmem ./pkg/fsmv2/supervisor/... -run=^$
// Then compare ns/op values to verify linear scaling.
// =============================================================================

// BenchmarkTickScaling runs tick benchmarks at various scales for comparison.
// Use benchstat to compare results and verify O(n) scaling.
func BenchmarkTickScaling(b *testing.B) {
	workerCounts := []int{1, 10, 50, 100, 250, 500}

	for _, count := range workerCounts {
		b.Run(fmt.Sprintf("Workers_%d", count), func(b *testing.B) {
			benchmarkSupervisorTick(b, count)
		})
	}
}
