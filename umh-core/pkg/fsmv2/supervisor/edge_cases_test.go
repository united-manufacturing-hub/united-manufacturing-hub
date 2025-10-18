// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"go.uber.org/zap"
)

var _ = Describe("Edge Cases", func() {
	Describe("FreshnessChecker edge cases", func() {
		Context("when snapshot has nil observed state", func() {
			It("should return false for Check", func() {
				checker := supervisor.NewFreshnessChecker(
					10*time.Second,
					20*time.Second,
					zap.NewNop().Sugar(),
				)

				snapshot := &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Observed: nil,
					Desired:  &mockDesiredState{},
				}

				Expect(checker.Check(snapshot)).To(BeFalse())
			})

			It("should return false for IsTimeout", func() {
				checker := supervisor.NewFreshnessChecker(
					10*time.Second,
					20*time.Second,
					zap.NewNop().Sugar(),
				)

				snapshot := &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Observed: nil,
					Desired:  &mockDesiredState{},
				}

				Expect(checker.IsTimeout(snapshot)).To(BeFalse())
			})
		})
	})

	Describe("Collector error handling", func() {
		Context("when CollectObservedState fails", func() {
			It("should continue observation loop", func() {
				callCount := 0
				collector := supervisor.NewCollector(supervisor.CollectorConfig{
					Worker: &mockWorker{
						collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
							callCount++
							if callCount == 1 {
								return nil, errors.New("collection error")
							}

							return &mockObservedState{timestamp: time.Now()}, nil
						},
					},
					Identity:            mockIdentity(),
					Store:               &mockStore{},
					Logger:              zap.NewNop().Sugar(),
					ObservationInterval: 50 * time.Millisecond,
					ObservationTimeout:  1 * time.Second,
				})

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				err := collector.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(200 * time.Millisecond)

				Expect(callCount).To(BeNumerically(">=", 2))

				cancel()
				time.Sleep(100 * time.Millisecond)
			})
		})

		Context("when SaveObserved fails", func() {
			It("should continue observation loop", func() {
				saveCallCount := 0
				store := &mockStore{
					saveErr: errors.New("save error"),
				}

				collector := supervisor.NewCollector(supervisor.CollectorConfig{
					Worker: &mockWorker{
						collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
							saveCallCount++

							return &mockObservedState{timestamp: time.Now()}, nil
						},
					},
					Identity:            mockIdentity(),
					Store:               store,
					Logger:              zap.NewNop().Sugar(),
					ObservationInterval: 50 * time.Millisecond,
					ObservationTimeout:  1 * time.Second,
				})

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				err := collector.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(200 * time.Millisecond)

				Expect(saveCallCount).To(BeNumerically(">=", 2))

				cancel()
				time.Sleep(100 * time.Millisecond)
			})
		})

		Context("when context is canceled during collection", func() {
			It("should stop observation loop", func() {
				collector := supervisor.NewCollector(supervisor.CollectorConfig{
					Worker:              &mockWorker{},
					Identity:            mockIdentity(),
					Store:               &mockStore{},
					Logger:              zap.NewNop().Sugar(),
					ObservationInterval: 1 * time.Second,
					ObservationTimeout:  3 * time.Second,
				})

				ctx, cancel := context.WithCancel(context.Background())

				err := collector.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(50 * time.Millisecond)
				Expect(collector.IsRunning()).To(BeTrue())

				cancel()

				Eventually(func() bool {
					return collector.IsRunning()
				}, 2*time.Second).Should(BeFalse())
			})
		})

		Context("when Restart is called with pending restart", func() {
			It("should not block", func() {
				collector := supervisor.NewCollector(supervisor.CollectorConfig{
					Worker:              &mockWorker{},
					Identity:            mockIdentity(),
					Store:               &mockStore{},
					Logger:              zap.NewNop().Sugar(),
					ObservationInterval: 1 * time.Second,
					ObservationTimeout:  3 * time.Second,
				})

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				err := collector.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				collector.Restart()
				collector.Restart()

				time.Sleep(100 * time.Millisecond)

				cancel()
			})
		})
	})

	Describe("Supervisor signal processing", func() {
		Context("when signal is SignalNone", func() {
			It("should process normally", func() {
				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now()},
					},
				}

				initialState := &mockState{signal: fsmv2.SignalNone}

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{initialState: initialState},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
				})

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Describe("RequestShutdown", func() {
		Context("when shutdown is requested", func() {
			It("should save desired state with shutdown flag", func() {
				savedDesired := false
				store := &mockStore{
					saveDesired: func(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
						savedDesired = true
						shutdownDesired, ok := desired.(interface{ ShutdownRequested() bool })
						Expect(ok).To(BeTrue())
						Expect(shutdownDesired.ShutdownRequested()).To(BeTrue())

						return nil
					},
				}

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
				})

				err := s.RequestShutdown(context.Background(), "test reason")
				Expect(err).ToNot(HaveOccurred())
				Expect(savedDesired).To(BeTrue())
			})
		})

		Context("when SaveDesired fails", func() {
			It("should return error", func() {
				store := &mockStore{
					saveErr: errors.New("save error"),
				}

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
				})

				err := s.RequestShutdown(context.Background(), "test reason")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to save desired state"))
			})
		})
	})

	Describe("Action execution with retry", func() {
		Context("when action succeeds on first try", func() {
			It("should execute without retry", func() {
				callCount := 0
				action := &mockAction{
					executeFunc: func(ctx context.Context) error {
						callCount++

						return nil
					},
				}

				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now()},
					},
				}

				initialState := &mockState{}
				initialState.nextState = initialState
				initialState.action = action

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{initialState: initialState},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
				})

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(callCount).To(Equal(1))
			})
		})

		Context("when action fails once then succeeds", func() {
			It("should retry and succeed", func() {
				callCount := 0
				action := &mockAction{
					executeFunc: func(ctx context.Context) error {
						callCount++
						if callCount == 1 {
							return errors.New("temporary error")
						}

						return nil
					},
				}

				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now()},
					},
				}

				initialState := &mockState{}
				initialState.nextState = initialState
				initialState.action = action

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{initialState: initialState},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
				})

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(callCount).To(Equal(2))
			})
		})

		Context("when action always fails", func() {
			It("should retry max times then fail", func() {
				callCount := 0
				action := &mockAction{
					executeFunc: func(ctx context.Context) error {
						callCount++

						return errors.New("persistent error")
					},
				}

				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now()},
					},
				}

				initialState := &mockState{}
				initialState.nextState = initialState
				initialState.action = action

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{initialState: initialState},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
				})

				err := s.Tick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed after"))
				Expect(callCount).To(Equal(3))
			})
		})
	})

	Describe("Tick timeout handling", func() {
		Context("when data times out and restart count is below max", func() {
			It("should restart collector", func() {
				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now().Add(-25 * time.Second)},
					},
				}

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold:     10 * time.Second,
						Timeout:            20 * time.Second,
						MaxRestartAttempts: 3,
					},
				})

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(s.GetRestartCount()).To(Equal(1))
			})
		})

		Context("when data times out and restart count is at max", func() {
			It("should request shutdown", func() {
				store := &mockStore{
					snapshot: &fsmv2.Snapshot{
						Identity: mockIdentity(),
						Desired:  &mockDesiredState{},
						Observed: &mockObservedState{timestamp: time.Now().Add(-25 * time.Second)},
					},
				}

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold:     10 * time.Second,
						Timeout:            20 * time.Second,
						MaxRestartAttempts: 3,
					},
				})

				s.SetRestartCount(3)

				err := s.Tick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unresponsive"))
			})
		})

		Context("when collector recovers after restarts", func() {
			It("should reset restart count", func() {
				store := &mockStore{}

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
				})

				s.SetRestartCount(2)

				store.snapshot = &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Desired:  &mockDesiredState{},
					Observed: &mockObservedState{timestamp: time.Now()},
				}

				err := s.Tick(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(s.GetRestartCount()).To(Equal(0))
			})
		})
	})

	Describe("Tick LoadSnapshot error handling", func() {
		Context("when LoadSnapshot fails", func() {
			It("should return error", func() {
				store := &mockStore{
					loadSnapshot: func(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
						return nil, errors.New("load snapshot error")
					},
				}

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
				})

				err := s.Tick(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load snapshot"))
			})
		})
	})
})

type mockAction struct {
	executeFunc func(ctx context.Context) error
	name        string
}

func (m *mockAction) Execute(ctx context.Context) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx)
	}

	return nil
}

func (m *mockAction) Name() string {
	if m.name != "" {
		return m.name
	}

	return "MockAction"
}
