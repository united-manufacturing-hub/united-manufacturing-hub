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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("Supervisor Race Conditions", func() {
	var (
		s    *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		ctx  context.Context
		once sync.Once
	)

	BeforeEach(func() {
		ctx = context.Background()
		triangularStore := createTestTriangularStore()

		s = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: "test",
			Store:      triangularStore,
			Logger:     zap.NewNop().Sugar(),
		})
	})

	Describe("Worker Registry Concurrent Access", func() {
		Context("when multiple goroutines access the worker registry simultaneously", func() {
			It("should detect race conditions with AddWorker, RemoveWorker, and GetWorker", func() {
				const numWorkers = 10
				const numGoroutinesPerOperation = 10

				var wg sync.WaitGroup

				addWorkers := func() {
					defer GinkgoRecover()
					defer wg.Done()

					for i := 0; i < numWorkers; i++ {
						identity := fsmv2.Identity{
							ID:         fmt.Sprintf("worker-%d", i),
							Name:       fmt.Sprintf("Test Worker %d", i),
							WorkerType: "test",
						}

						worker := &mockWorker{
							observed:     createMockObservedStateWithID(identity.ID),
							initialState: &mockState{},
						}

						err := s.AddWorker(identity, worker)
						if err != nil {
							By(fmt.Sprintf("AddWorker failed for %s: %v", identity.ID, err))
						}

						time.Sleep(1 * time.Millisecond)
					}
				}

				removeWorkers := func() {
					defer GinkgoRecover()
					defer wg.Done()

					for i := 0; i < numWorkers; i++ {
						workerID := fmt.Sprintf("worker-%d", i)
						err := s.RemoveWorker(ctx, workerID)
						if err != nil {
							By(fmt.Sprintf("RemoveWorker failed for %s: %v", workerID, err))
						}

						time.Sleep(1 * time.Millisecond)
					}
				}

				getWorkers := func() {
					defer GinkgoRecover()
					defer wg.Done()

					for i := 0; i < numWorkers*2; i++ {
						workerID := fmt.Sprintf("worker-%d", i%numWorkers)
						_, err := s.GetWorker(workerID)
						if err != nil {
							By(fmt.Sprintf("GetWorker failed for %s: %v", workerID, err))
						}

						time.Sleep(1 * time.Millisecond)
					}
				}

				listWorkers := func() {
					defer GinkgoRecover()
					defer wg.Done()

					for i := 0; i < numWorkers; i++ {
						workers := s.ListWorkers()
						By(fmt.Sprintf("ListWorkers iteration %d: found %d workers", i, len(workers)))
						time.Sleep(1 * time.Millisecond)
					}
				}

				for i := 0; i < numGoroutinesPerOperation; i++ {
					wg.Add(4)
					go addWorkers()
					go removeWorkers()
					go getWorkers()
					go listWorkers()
				}

				wg.Wait()

				once.Do(func() {
					By("Test completed - race detector should report issues if mutex is missing")
				})
			})
		})

		Context("when adding the same worker concurrently", func() {
			It("should detect race conditions on duplicate worker addition", func() {
				const numGoroutines = 10
				var wg sync.WaitGroup

				identity := fsmv2.Identity{
					ID:         "duplicate-worker",
					Name:       "Duplicate Test Worker",
					WorkerType: "test",
				}

				for i := 0; i < numGoroutines; i++ {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						worker := &mockWorker{
							observed:     createMockObservedStateWithID(identity.ID),
							initialState: &mockState{},
						}

						err := s.AddWorker(identity, worker)
						if err != nil {
							By(fmt.Sprintf("AddWorker failed (expected for duplicates): %v", err))
						}
					}()
				}

				wg.Wait()
			})
		})

		Context("when reading and writing worker state concurrently", func() {
			It("should detect race conditions on GetWorkerState", func() {
				identity := fsmv2.Identity{
					ID:         "state-test-worker",
					Name:       "State Test Worker",
					WorkerType: "test",
				}

				worker := &mockWorker{
					observed:     createMockObservedStateWithID(identity.ID),
					initialState: &mockState{},
				}

				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				const numReaders = 10
				var wg sync.WaitGroup

				for i := 0; i < numReaders; i++ {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for j := 0; j < 100; j++ {
							state, reason, err := s.GetWorkerState(identity.ID)
							if err != nil {
								By(fmt.Sprintf("GetWorkerState failed: %v", err))
							} else {
								By(fmt.Sprintf("Worker state: %s (reason: %s)", state, reason))
							}
							time.Sleep(1 * time.Millisecond)
						}
					}()
				}

				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()

					for i := 0; i < 100; i++ {
						err := s.TestTick(ctx)
						if err != nil {
							By(fmt.Sprintf("Tick failed: %v", err))
						}
						time.Sleep(1 * time.Millisecond)
					}
				}()

				wg.Wait()
			})
		})

		Context("when modifying children map concurrently", func() {
			It("should detect race conditions on GetChildren", func() {
				const numReaders = 10
				var wg sync.WaitGroup

				for i := 0; i < numReaders; i++ {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()

						for j := 0; j < 100; j++ {
							children := s.GetChildren()
							By(fmt.Sprintf("Found %d children", len(children)))
							time.Sleep(1 * time.Millisecond)
						}
					}()
				}

				wg.Wait()
			})
		})
	})
})
