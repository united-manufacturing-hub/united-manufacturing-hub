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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"go.uber.org/zap"
)

var _ = Describe("Supervisor Lifecycle", func() {
	Describe("Start and tickLoop integration", func() {
		Context("when supervisor is started", func() {
			It("should run tick loop until context is cancelled", func() {
				store := createTestTriangularStore()

				s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
					WorkerType:              "container",
					Store:                   store,
					Logger:                  zap.NewNop().Sugar(),
					TickInterval:            50 * time.Millisecond,
					GracefulShutdownTimeout: 100 * time.Millisecond, // Short timeout for tests
				})

				identity := mockIdentity()
				worker := &mockWorker{}
				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())
				defer s.Shutdown()

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
			It("should mark worker for restart and request graceful shutdown", func() {
				// NOTE: SignalNeedsRestart now triggers a full worker restart (graceful shutdown + reset)
				// instead of just restarting the collector. The restart count is NOT incremented
				// because we're doing a full restart, not a collector-only restart.
				// See "SignalNeedsRestart full worker restart" tests for all FSM state combinations.
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

				// Worker should still exist (not removed yet - waiting for graceful shutdown)
				workers := s.ListWorkers()
				Expect(workers).To(HaveLen(1))
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

	Describe("SignalNeedsRestart full worker restart", func() {
		Context("when SignalNeedsRestart is received", func() {
			It("should mark worker for restart and request graceful shutdown", func() {
				store := newMockTriangularStore()

				state := &mockState{
					signal: fsmv2.SignalNeedsRestart,
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{
					MaxRestartAttempts: 3,
				})

				workersBefore := s.ListWorkers()
				Expect(workersBefore).To(HaveLen(1))

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				workersAfter := s.ListWorkers()
				Expect(workersAfter).To(HaveLen(1))

				identity := mockIdentity()
				var desiredState mockDesiredState
				loadErr := store.LoadDesiredTyped(context.Background(), "test", identity.ID, &desiredState)
				Expect(loadErr).ToNot(HaveOccurred())
				Expect(desiredState.ShutdownRequested).To(BeTrue())
			})
		})

		Context("when SignalNeedsRemoval received for worker in pendingRestart", func() {
			It("should restart worker instead of removing", func() {
				store := newMockTriangularStore()

				initialState := &mockState{}
				initialState.nextState = initialState

				stoppedState := &mockState{
					signal: fsmv2.SignalNeedsRemoval,
				}
				stoppedState.nextState = stoppedState

				worker := &mockWorker{initialState: stoppedState}

				s := newSupervisorWithWorker(worker, store, supervisor.CollectorHealthConfig{})

				identity := mockIdentity()
				s.TestSetPendingRestart(identity.ID)

				workersBefore := s.ListWorkers()
				Expect(workersBefore).To(HaveLen(1))

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				workersAfter := s.ListWorkers()
				Expect(workersAfter).To(HaveLen(1))

				var desiredState mockDesiredState
				loadErr := store.LoadDesiredTyped(context.Background(), "test", identity.ID, &desiredState)
				Expect(loadErr).ToNot(HaveOccurred())
				Expect(desiredState.ShutdownRequested).To(BeFalse())
			})
		})

		Context("when SignalNeedsRemoval received for worker NOT in pendingRestart", func() {
			It("should remove worker normally", func() {
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

		Context("when graceful restart times out", func() {
			It("should force reset worker after timeout", func() {
				store := newMockTriangularStore()

				state := &mockState{}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{})
				identity := mockIdentity()

				s.TestSetPendingRestart(identity.ID)

				s.TestSetRestartRequestedAt(identity.ID, time.Now().Add(-35*time.Second))

				err := s.TestTick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				workers := s.ListWorkers()
				Expect(workers).To(HaveLen(1))

				Expect(s.TestIsPendingRestart(identity.ID)).To(BeFalse())
			})
		})
	})
})
