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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// TestCollectorHealthRace tests for race conditions in collectorHealth access.
// Run with: go test -race -run TestCollectorHealthRace ./pkg/fsmv2/supervisor/...
var _ = Describe("CollectorHealth Race Conditions", func() {
	var (
		s   *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		triangularStore := createTestTriangularStore()

		s = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: "test",
			Store:      triangularStore,
			Logger:     zap.NewNop().Sugar(),
		})

		// Add a worker so tick operations can proceed
		identity := deps.Identity{
			ID:         "test-worker-1",
			Name:       "Test Worker 1",
			WorkerType: "test",
		}

		worker := &mockWorker{
			observed:     createMockObservedStateWithID(identity.ID),
			initialState: &mockState{},
		}

		err := s.AddWorker(identity, worker)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Concurrent collectorHealth access", func() {
		Context("when tick loop and restartCollector access collectorHealth simultaneously", func() {
			It("should not have data races on restartCount and lastRestart", func() {
				// This test is designed to expose race conditions when run with -race flag.
				// Concurrent access to s.collectorHealth.restartCount and s.collectorHealth.lastRestart
				// from tick loop (reconciliation.go) and restartCollector without mutex protection
				// will be detected by the race detector.

				const numGoroutines = 10
				const numIterations = 50
				var wg sync.WaitGroup

				// Goroutines that read restartCount (simulating tick loop checks)
				for range numGoroutines {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range numIterations {
							// This reads collectorHealth.restartCount without mutex
							// via TestGetRestartCount (which is protected)
							// But we want to expose the race in the actual tick code path
							_ = s.TestGetRestartCount()
							time.Sleep(100 * time.Microsecond)
						}
					}()
				}

				// Goroutines that write restartCount (simulating restartCollector)
				for range numGoroutines {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range numIterations {
							// This writes collectorHealth.restartCount
							s.TestSetRestartCount(0)
							time.Sleep(100 * time.Microsecond)
						}
					}()
				}

				// Goroutines that call tick (which accesses collectorHealth)
				for range numGoroutines / 2 {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range numIterations / 2 {
							// tick() accesses collectorHealth.restartCount directly
							// without mutex protection - this should trigger race detection
							_ = s.TestTick(ctx)
							time.Sleep(200 * time.Microsecond)
						}
					}()
				}

				// Goroutines that call restartCollector (which modifies collectorHealth)
				for range numGoroutines / 2 {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range numIterations / 2 {
							// Set restart count to 0 first so restartCollector doesn't panic
							s.TestSetRestartCount(0)
							// Set lastRestart to past so backoff is elapsed
							s.TestSetLastRestart(time.Now().Add(-10 * time.Second))

							// restartCollector modifies collectorHealth without mutex
							_ = s.TestRestartCollector(ctx, "test-worker-1")
							time.Sleep(200 * time.Microsecond)
						}
					}()
				}

				wg.Wait()

				// If we get here without the race detector complaining, test passes
				// (but with -race flag, unprotected access will be detected)
				Expect(true).To(BeTrue())
			})
		})

		Context("when checkDataFreshness reads while restartCollector writes", func() {
			It("should not have data races on collectorHealth fields", func() {
				const numGoroutines = 10
				const numIterations = 100
				var wg sync.WaitGroup

				// Goroutines running tick (which calls checkDataFreshness)
				for range numGoroutines {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range numIterations {
							// tick calls checkDataFreshness which reads collectorHealth
							_ = s.TestTick(ctx)
							time.Sleep(50 * time.Microsecond)
						}
					}()
				}

				// Goroutines modifying restartCount and lastRestart
				for range numGoroutines {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range numIterations {
							s.TestSetRestartCount(0)
							s.TestSetLastRestart(time.Now().Add(-10 * time.Second))
							time.Sleep(50 * time.Microsecond)
						}
					}()
				}

				wg.Wait()
				Expect(true).To(BeTrue())
			})
		})
	})
})
