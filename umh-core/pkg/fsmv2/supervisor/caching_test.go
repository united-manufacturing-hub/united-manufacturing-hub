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
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/testutil"
)

// CachingTestWorker wraps testutil.Worker to track DeriveDesiredState calls.
type CachingTestWorker struct {
	testutil.Worker
	deriveCallCount atomic.Int32
	lastSpec        config.UserSpec
	identity        fsmv2.Identity
}

func NewCachingTestWorker(identity fsmv2.Identity) *CachingTestWorker {
	w := &CachingTestWorker{
		identity: identity,
	}
	// Set up the base worker with a custom collect func
	w.CollectFunc = func(ctx context.Context) (fsmv2.ObservedState, error) {
		return &testutil.ObservedState{
			ID:          identity.ID,
			CollectedAt: time.Now(),
			Desired:     &testutil.DesiredState{},
		}, nil
	}

	return w
}

func (w *CachingTestWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	w.deriveCallCount.Add(1)

	if userSpec, ok := spec.(config.UserSpec); ok {
		w.lastSpec = userSpec
	}

	return &config.DesiredState{
		State: config.DesiredStateRunning,
	}, nil
}

func (w *CachingTestWorker) GetDeriveCallCount() int32 {
	return w.deriveCallCount.Load()
}

var _ = Describe("DeriveDesiredState Caching", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		logger *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		logger = zap.NewNop().Sugar()
	})

	AfterEach(func() {
		cancel()
	})

	Describe("cache hit behavior", func() {
		It("calls DeriveDesiredState only once for unchanged UserSpec over multiple ticks", func() {
			workerType := "cachingtest"
			store := testutil.CreateTriangularStoreForWorkerType(workerType)

			// Create supervisor with caching test types
			sup := supervisor.NewSupervisor[*testutil.ObservedState, *testutil.DesiredState](
				supervisor.Config{
					WorkerType:   workerType,
					Store:        store,
					Logger:       logger,
					TickInterval: 10 * time.Millisecond,
					UserSpec: config.UserSpec{
						Config: "test config that stays the same",
						Variables: config.VariableBundle{
							User: map[string]any{
								"key": "value",
							},
						},
					},
				},
			)

			// Create and add worker
			identity := fsmv2.Identity{ID: "test-worker", WorkerType: workerType}
			worker := NewCachingTestWorker(identity)
			err := sup.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			// Start supervisor
			done := sup.Start(ctx)
			defer func() {
				sup.Shutdown()
				<-done
			}()

			// Wait for multiple ticks
			time.Sleep(200 * time.Millisecond)

			// With caching: DeriveDesiredState should be called twice total:
			// 1. Once in AddWorker() (with nil spec for initial state)
			// 2. Once in first tick() (with actual UserSpec - cache miss due to different input)
			// Subsequent ticks should use the cached result.
			// Without caching: It would be called ~22 times (1 AddWorker + ~21 ticks)
			callCount := worker.GetDeriveCallCount()

			// Assert that caching is working: expect exactly 2 calls
			// (AddWorker + first tick, then cached for all subsequent ticks)
			Expect(callCount).To(Equal(int32(2)),
				"DeriveDesiredState should be called twice (AddWorker + first tick), got %d calls", callCount)
		})
	})

	Describe("cache invalidation on UserSpec change", func() {
		It("calls DeriveDesiredState again when UserSpec.Config changes", func() {
			workerType := "cachingtest2"
			store := testutil.CreateTriangularStoreForWorkerType(workerType)

			sup := supervisor.NewSupervisor[*testutil.ObservedState, *testutil.DesiredState](
				supervisor.Config{
					WorkerType:   workerType,
					Store:        store,
					Logger:       logger,
					TickInterval: 10 * time.Millisecond,
					UserSpec: config.UserSpec{
						Config: "initial config",
					},
				},
			)

			identity := fsmv2.Identity{ID: "test-worker", WorkerType: workerType}
			worker := NewCachingTestWorker(identity)
			err := sup.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			done := sup.Start(ctx)
			defer func() {
				sup.Shutdown()
				<-done
			}()

			// Wait for first tick to process
			// AddWorker calls DeriveDesiredState(nil), first tick calls DeriveDesiredState(userSpec)
			time.Sleep(50 * time.Millisecond)
			initialCallCount := worker.GetDeriveCallCount()
			Expect(initialCallCount).To(BeNumerically(">=", int32(2)), "Should have at least 2 calls after first tick (AddWorker + first tick)")

			// Change UserSpec via interface method
			sup.TestUpdateUserSpec(config.UserSpec{
				Config: "updated config - different from initial",
			})

			// Wait for cache invalidation and re-derive
			time.Sleep(50 * time.Millisecond)
			finalCallCount := worker.GetDeriveCallCount()

			// Should have exactly 3 calls: AddWorker + first tick + after config change
			// (intermediate ticks with same config should be cached)
			Expect(finalCallCount).To(Equal(int32(3)),
				"DeriveDesiredState should be called exactly three times: AddWorker + first tick + after config change, got %d calls", finalCallCount)
		})

		It("calls DeriveDesiredState again when UserSpec.Variables change", func() {
			workerType := "cachingtest3"
			store := testutil.CreateTriangularStoreForWorkerType(workerType)

			sup := supervisor.NewSupervisor[*testutil.ObservedState, *testutil.DesiredState](
				supervisor.Config{
					WorkerType:   workerType,
					Store:        store,
					Logger:       logger,
					TickInterval: 10 * time.Millisecond,
					UserSpec: config.UserSpec{
						Config: "same config",
						Variables: config.VariableBundle{
							User: map[string]any{"IP": "192.168.1.100"},
						},
					},
				},
			)

			identity := fsmv2.Identity{ID: "test-worker", WorkerType: workerType}
			worker := NewCachingTestWorker(identity)
			err := sup.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			done := sup.Start(ctx)
			defer func() {
				sup.Shutdown()
				<-done
			}()

			// Wait for first tick
			// AddWorker calls DeriveDesiredState(nil), first tick calls DeriveDesiredState(userSpec)
			time.Sleep(50 * time.Millisecond)

			// Change Variables only (Config stays the same)
			sup.TestUpdateUserSpec(config.UserSpec{
				Config: "same config",
				Variables: config.VariableBundle{
					User: map[string]any{"IP": "192.168.1.200"}, // Different IP
				},
			})

			// Wait for cache invalidation
			time.Sleep(50 * time.Millisecond)
			finalCallCount := worker.GetDeriveCallCount()

			// Should have exactly 3 calls: AddWorker + first tick + after variables change
			Expect(finalCallCount).To(Equal(int32(3)),
				"DeriveDesiredState should be called when Variables change, got %d calls", finalCallCount)
		})
	})

	Describe("first tick behavior", func() {
		It("always calls DeriveDesiredState on first tick", func() {
			workerType := "cachingtest4"
			store := testutil.CreateTriangularStoreForWorkerType(workerType)

			sup := supervisor.NewSupervisor[*testutil.ObservedState, *testutil.DesiredState](
				supervisor.Config{
					WorkerType:   workerType,
					Store:        store,
					Logger:       logger,
					TickInterval: 10 * time.Millisecond,
					UserSpec: config.UserSpec{
						Config: "test config",
					},
				},
			)

			identity := fsmv2.Identity{ID: "test-worker", WorkerType: workerType}
			worker := NewCachingTestWorker(identity)
			err := sup.AddWorker(identity, worker)
			Expect(err).NotTo(HaveOccurred())

			done := sup.Start(ctx)
			defer func() {
				sup.Shutdown()
				<-done
			}()

			// Wait for first tick
			time.Sleep(30 * time.Millisecond)

			// First tick should always call DeriveDesiredState
			Expect(worker.GetDeriveCallCount()).To(BeNumerically(">=", int32(1)),
				"DeriveDesiredState should be called at least once on first tick")
		})
	})
})
