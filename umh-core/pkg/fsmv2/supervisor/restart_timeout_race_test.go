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

// TestCheckRestartTimeoutsRace tests for race conditions in checkRestartTimeouts.
// The function accesses workerCtx.currentState without holding workerCtx.mu,
// which creates a data race when other code modifies worker state concurrently.
// Run with: go test -race -run "Restart Timeout Race" ./pkg/fsmv2/supervisor/...
var _ = Describe("Restart Timeout Race Conditions", func() {
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

		// Add a worker so checkRestartTimeouts can access workerCtx
		identity := deps.Identity{
			ID:         "race-test-worker",
			Name:       "Race Test Worker",
			WorkerType: "test",
		}

		worker := &mockWorker{
			observed:     createMockObservedStateWithID(identity.ID),
			initialState: &mockState{},
		}

		err := s.AddWorker(identity, worker)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Concurrent access to workerCtx.currentState", func() {
		Context("when checkRestartTimeouts reads currentState while tick modifies it", func() {
			It("should not have data races on workerCtx.currentState", func() {
				// This test is designed to expose race conditions when run with -race flag.
				// checkRestartTimeouts accesses workerCtx.currentState without workerCtx.mu
				// while tick() modifies currentState under lock - this is a data race.

				const numIterations = 100
				var wg sync.WaitGroup
				workerID := "race-test-worker"

				// Set up pending restart with old timestamp to trigger timeout path
				// in checkRestartTimeouts (lines 899-930 in reconciliation.go)
				s.TestSetPendingRestart(workerID)
				s.TestSetRestartRequestedAt(workerID, time.Now().Add(-1*time.Hour))

				// Goroutines that call tick() which may modify workerCtx.currentState
				for range 5 {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range numIterations {
							// tick() modifies workerCtx.currentState under workerCtx.mu lock
							_ = s.TestTick(ctx)
							time.Sleep(10 * time.Microsecond)
						}
					}()
				}

				// Goroutines that trigger checkRestartTimeouts via tick()
				// checkRestartTimeouts reads workerCtx.currentState at lines 916, 918, 930
				// WITHOUT holding workerCtx.mu - this should trigger race detection
				for range 5 {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range numIterations {
							// Set up pending restart each iteration to ensure
							// checkRestartTimeouts accesses workerCtx.currentState
							s.TestSetPendingRestart(workerID)
							s.TestSetRestartRequestedAt(workerID, time.Now().Add(-1*time.Hour))

							// This tick will call checkRestartTimeouts which reads
							// workerCtx.currentState without lock
							_ = s.TestTick(ctx)
							time.Sleep(10 * time.Microsecond)
						}
					}()
				}

				wg.Wait()

				// If we get here without the race detector complaining, test passes
				// (but with -race flag, unprotected access will be detected)
				Expect(true).To(BeTrue())
			})
		})

		Context("when checkRestartTimeouts writes currentState while GetWorkerState reads it", func() {
			It("should not have data races on concurrent state access", func() {
				// checkRestartTimeouts writes workerCtx.currentState at line 918
				// without holding workerCtx.mu, while GetWorkerState reads it under lock

				const numIterations = 100
				var wg sync.WaitGroup
				workerID := "race-test-worker"

				// Goroutines reading worker state via GetWorkerState (which properly locks)
				for range 5 {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range numIterations {
							// GetWorkerState reads currentState under workerCtx.mu lock
							_, _, _ = s.GetWorkerState(workerID)
							time.Sleep(10 * time.Microsecond)
						}
					}()
				}

				// Goroutines triggering checkRestartTimeouts write to currentState
				for range 5 {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range numIterations {
							// Set up pending restart to trigger the write path
							s.TestSetPendingRestart(workerID)
							s.TestSetRestartRequestedAt(workerID, time.Now().Add(-1*time.Hour))

							// This tick will call checkRestartTimeouts which writes
							// workerCtx.currentState = workerCtx.worker.GetInitialState()
							// at line 918 without workerCtx.mu lock
							_ = s.TestTick(ctx)
							time.Sleep(10 * time.Microsecond)
						}
					}()
				}

				wg.Wait()
				Expect(true).To(BeTrue())
			})
		})
	})
})
