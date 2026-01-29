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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// TestHandleWorkerRestartTOCTOU tests for TOCTOU race in handleWorkerRestart.
// The function had two TOCTOU issues:
// 1. Reading workerCtx.currentState without holding workerCtx.mu (lines 449, 525-526)
// 2. Using newWorkerCtx.collector/executor after releasing s.mu (lines 493-499)
// Run with: go test -race -run "HandleWorkerRestart TOCTOU" ./pkg/fsmv2/supervisor/...
var _ = Describe("HandleWorkerRestart TOCTOU Race Conditions", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("TOCTOU in handleWorkerRestart lines 488-500", func() {
		// This tests the TOCTOU race condition where:
		// 1. handleWorkerRestart reads s.workers[workerID] under lock (line 490)
		// 2. Releases lock (line 491)
		// 3. Uses newWorkerCtx.collector and newWorkerCtx.executor (lines 493-499)
		//
		// Between step 2 and 3, another goroutine could modify s.workers[workerID]
		// via RemoveWorker or AddWorker.

		Context("when handleWorkerRestart races with RemoveWorker", func() {
			It("should not have data races when accessing worker context after lock release", func() {
				// This test exposes the race by concurrently:
				// 1. Triggering handleWorkerRestart (via SignalNeedsRemoval with pendingRestart)
				// 2. Calling RemoveWorker on the same worker

				const numIterations = 50
				var wg sync.WaitGroup

				for range numIterations {
					workerID := "toctou-race-worker"

					// Create a fresh supervisor for each iteration to isolate test runs
					triangularStore := createTestTriangularStore()
					localSupervisor := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
						WorkerType: "test",
						Store:      triangularStore,
						Logger:     zap.NewNop().Sugar(),
					})

					identity := deps.Identity{
						ID:         workerID,
						Name:       "TOCTOU Race Test Worker",
						WorkerType: "test",
					}

					// Create worker with SignalNeedsRemoval to trigger handleWorkerRestart
					stoppedState := &mockState{
						signal: fsmv2.SignalNeedsRemoval,
					}
					stoppedState.nextState = stoppedState

					worker := &mockWorker{
						observed:     createMockObservedStateWithID(identity.ID),
						initialState: stoppedState,
					}

					err := localSupervisor.AddWorker(identity, worker)
					if err != nil {
						continue
					}

					// Start the supervisor to enable collector/executor starting
					supervisorDone := localSupervisor.Start(ctx)

					// Mark worker as pending restart to trigger handleWorkerRestart path
					localSupervisor.TestSetPendingRestart(workerID)

					// Launch concurrent operations
					wg.Add(2)

					// Goroutine 1: Trigger tick which will call handleWorkerRestart
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						// Multiple ticks to increase chance of hitting the race window
						for range 10 {
							_ = localSupervisor.TestTick(ctx)
							time.Sleep(100 * time.Microsecond)
						}
					}()

					// Goroutine 2: Concurrently try to remove the same worker
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						// Multiple removal attempts to increase chance of hitting race window
						for range 10 {
							_ = localSupervisor.RemoveWorker(ctx, workerID)
							time.Sleep(100 * time.Microsecond)
						}
					}()

					wg.Wait()

					// Clean up
					localSupervisor.Shutdown()
					<-supervisorDone
				}

				// If we get here without the race detector complaining, the test passes.
				// With -race flag, unprotected access will be detected.
				Expect(true).To(BeTrue())
			})
		})

		Context("when handleWorkerRestart races with concurrent AddWorker", func() {
			It("should not have data races when worker is replaced during restart", func() {
				// This tests the scenario where:
				// 1. handleWorkerRestart reads workerCtx from s.workers[workerID]
				// 2. Lock is released
				// 3. Another goroutine calls RemoveWorker then AddWorker (replacing the worker)
				// 4. handleWorkerRestart uses the old workerCtx

				const numIterations = 50
				var wg sync.WaitGroup

				for range numIterations {
					workerID := "replace-race-worker"

					triangularStore := createTestTriangularStore()
					localSupervisor := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
						WorkerType: "test",
						Store:      triangularStore,
						Logger:     zap.NewNop().Sugar(),
					})

					identity := deps.Identity{
						ID:         workerID,
						Name:       "Replace Race Test Worker",
						WorkerType: "test",
					}

					stoppedState := &mockState{
						signal: fsmv2.SignalNeedsRemoval,
					}
					stoppedState.nextState = stoppedState

					worker := &mockWorker{
						observed:     createMockObservedStateWithID(identity.ID),
						initialState: stoppedState,
					}

					err := localSupervisor.AddWorker(identity, worker)
					if err != nil {
						continue
					}

					supervisorDone := localSupervisor.Start(ctx)
					localSupervisor.TestSetPendingRestart(workerID)

					wg.Add(2)

					// Goroutine 1: Trigger handleWorkerRestart via tick
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range 10 {
							_ = localSupervisor.TestTick(ctx)
							time.Sleep(50 * time.Microsecond)
						}
					}()

					// Goroutine 2: Replace the worker (remove + add)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for range 10 {
							// Remove existing worker
							_ = localSupervisor.RemoveWorker(ctx, workerID)

							// Add a new worker with the same ID
							newWorker := &mockWorker{
								observed:     createMockObservedStateWithID(identity.ID),
								initialState: &mockState{},
							}
							_ = localSupervisor.AddWorker(identity, newWorker)

							time.Sleep(50 * time.Microsecond)
						}
					}()

					wg.Wait()

					localSupervisor.Shutdown()
					<-supervisorDone
				}

				Expect(true).To(BeTrue())
			})
		})
	})
})
