// Copyright 2025 UMH Systems GmbH
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
	"go.uber.org/zap"
)

var _ = Describe("Supervisor Lifecycle", func() {
	Describe("Start and tickLoop integration", func() {
		Context("when supervisor is started", func() {
			It("should run tick loop until context is cancelled", func() {
				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now()},
					},
				}

				s := supervisor.NewSupervisor(supervisor.Config{
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
				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now().Add(-25 * time.Second)},
					},
					saveErr: errors.New("save error"),
				}

				s := newSupervisorWithWorker(&mockWorker{}, store, supervisor.CollectorHealthConfig{
					StaleThreshold:     10 * time.Second,
					Timeout:            20 * time.Second,
					MaxRestartAttempts: 1,
				})

				s.SetRestartCount(1)

				err := s.Tick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unresponsive"))
			})
		})
	})

	Describe("Tick state transition edge case", func() {
		Context("when state violates invariant by switching state AND emitting action", func() {
			It("should panic", func() {
				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now()},
					},
				}

				nextState := &mockState{}
				action := &mockAction{}
				initialState := &mockState{
					nextState: nextState,
					action:    action,
				}

				s := newSupervisorWithWorker(&mockWorker{initialState: initialState}, store, supervisor.CollectorHealthConfig{})

				Expect(func() {
					_ = s.Tick(context.Background())
				}).To(Panic())
			})
		})
	})

	Describe("observationLoop ticker and restart channel", func() {
		Context("when restart is requested during observation", func() {
			It("should collect immediately", func() {
				var collectCountMutex sync.Mutex
				collectCount := 0
				collector := supervisor.NewCollector(supervisor.CollectorConfig{
					Worker: &mockWorker{
						collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
							collectCountMutex.Lock()
							collectCount++
							collectCountMutex.Unlock()

							return &mockObservedState{timestamp: time.Now()}, nil
						},
					},
					Identity:            mockIdentity(),
					Store:               &mockStore{},
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
				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now()},
					},
				}

				state := &mockState{
					signal: fsmv2.SignalNeedsRemoval,
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{})

				workersBefore := s.ListWorkers()
				Expect(workersBefore).To(HaveLen(1))

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())

				workersAfter := s.ListWorkers()
				Expect(workersAfter).To(BeEmpty())
			})
		})

		Context("when SignalNeedsRestart is received", func() {
			It("should restart collector and increment restart count", func() {
				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now()},
					},
				}

				state := &mockState{
					signal: fsmv2.SignalNeedsRestart,
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{
					MaxRestartAttempts: 3,
				})

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(s.GetRestartCount()).To(Equal(1))
			})
		})

		Context("when unknown signal is received", func() {
			It("should return error for invalid signal", func() {
				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now()},
					},
				}

				state := &mockState{
					signal: fsmv2.Signal(999),
				}
				state.nextState = state

				s := newSupervisorWithWorker(&mockWorker{initialState: state}, store, supervisor.CollectorHealthConfig{})

				err := s.Tick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown signal"))
			})
		})
	})
})

