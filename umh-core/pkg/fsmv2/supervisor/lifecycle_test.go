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
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"go.uber.org/zap"
)

var _ = Describe("Supervisor Lifecycle", func() {
	Describe("Start and tickLoop integration", func() {
		Context("when supervisor is started", func() {
			It("should run tick loop until context is cancelled", func() {
				store := createTestTriangularStore()

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:   "container",
					Store:        store,
					Logger:       zap.NewNop().Sugar(),
					TickInterval: 50 * time.Millisecond,
				})

				identity := mockIdentity()
				worker := &mockWorker{}
				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				ctx, cancel := context.WithCancel(context.Background())

				done := s.Start(ctx)

				time.Sleep(200 * time.Millisecond)

				cancel()

				Eventually(done, 2*time.Second).Should(BeClosed())
			})
		})
	})

	Describe("Tick with shutdown request error", func() {
		Context("when RequestShutdown fails during timeout handling", func() {
			It("should still return error about unresponsive collector", func() {
				store := newMockTriangularStore()

				s := newSupervisorWithWorker(&mockWorker{
					observed: &mockObservedState{
						ID:          mockIdentity().ID,
						CollectedAt: time.Now().Add(-25 * time.Second),
						Desired:     &mockDesiredState{},
					},
				}, store, supervisor.CollectorHealthConfig{
					StaleThreshold:     10 * time.Second,
					Timeout:            20 * time.Second,
					MaxRestartAttempts: 1,
				})

				// Update the observed state in the store with stale data
				// This is necessary because newSupervisorWithWorker saves fresh data
				identity := mockIdentity()
				store.Observed["test"] = map[string]interface{}{
					identity.ID: persistence.Document{
						"id":          identity.ID,
						"collectedAt": time.Now().Add(-25 * time.Second),
					},
				}

				s.TestSetRestartCount(1)

				store.SaveDesiredErr = errors.New("save error")

				err := s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unresponsive"))
			})
		})
	})

	Describe("Tick state transition edge case", func() {
		Context("when state violates invariant by switching state AND emitting action", func() {
			It("should panic", func() {
				store := newMockTriangularStore()

				nextState := &mockState{}
				action := &mockAction{}
				initialState := &mockState{
					nextState: nextState,
					action:    action,
				}

				s := newSupervisorWithWorker(&mockWorker{initialState: initialState}, store, supervisor.CollectorHealthConfig{})

				Expect(func() {
					_ = s.TestTick(context.Background())
				}).To(Panic())
			})
		})
	})

	Describe("observationLoop ticker and restart channel", func() {
		Context("when restart is requested during observation", func() {
			It("should collect immediately", func() {
				var collectCountMutex sync.Mutex
				collectCount := 0
				store := newMockTriangularStore()
				collector := collection.NewCollector[mockObservedState](collection.CollectorConfig[mockObservedState]{
					Worker: &mockWorker{
						collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
							collectCountMutex.Lock()
							collectCount++
							collectCountMutex.Unlock()

							return &mockObservedState{
								ID:          "test-worker",
								CollectedAt: time.Now(),
								Desired:     &mockDesiredState{},
							}, nil
						},
					},
					Identity:            mockIdentity(),
					Store:               store,
					Logger:              zap.NewNop().Sugar(),
					ObservationInterval: 5 * time.Second,
					ObservationTimeout:  1 * time.Second,
				})

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				err := collector.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(100 * time.Millisecond)
				collectCountMutex.Lock()
				initialCount := collectCount
				collectCountMutex.Unlock()

				collector.Restart()

				time.Sleep(200 * time.Millisecond)

				collectCountMutex.Lock()
				finalCount := collectCount
				collectCountMutex.Unlock()
				Expect(finalCount).To(BeNumerically(">", initialCount))

				cancel()
				time.Sleep(100 * time.Millisecond)
			})
		})
	})

	Describe("processSignal error handling", func() {
		Context("when SignalNeedsRemoval is received", func() {
			It("should remove worker from registry", func() {
				store := newMockTriangularStore()

				state := &mockState{
					signal: fsmv2.SignalNeedsRemoval,
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{})

				workersBefore := s.ListWorkers()
				Expect(workersBefore).To(HaveLen(1))

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				workersAfter := s.ListWorkers()
				Expect(workersAfter).To(BeEmpty())
			})
		})

		Context("when SignalNeedsRestart is received", func() {
			It("should restart collector and increment restart count", func() {
				store := newMockTriangularStore()

				state := &mockState{
					signal: fsmv2.SignalNeedsRestart,
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{
					MaxRestartAttempts: 3,
				})

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(s.TestGetRestartCount()).To(Equal(1))
			})
		})

		Context("when unknown signal is received", func() {
			It("should return error for invalid signal", func() {
				store := newMockTriangularStore()

				state := &mockState{
					signal: fsmv2.Signal(999),
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{})

				err := s.TestTick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown signal"))
			})
		})
	})
})
