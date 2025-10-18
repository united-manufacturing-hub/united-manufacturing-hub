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

var _ = Describe("Additional Coverage Tests", func() {
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
					Worker:       &mockWorker{},
					Identity:     mockIdentity(),
					Store:        store,
					Logger:       zap.NewNop().Sugar(),
					TickInterval: 50 * time.Millisecond,
				})

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

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{
						StaleThreshold:     10 * time.Second,
						Timeout:            20 * time.Second,
						MaxRestartAttempts: 1,
					},
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

				s := supervisor.NewSupervisor(supervisor.Config{
					Worker:   &mockWorker{initialState: initialState},
					Identity: mockIdentity(),
					Store:    store,
					Logger:   zap.NewNop().Sugar(),
				})

				Expect(func() {
					_ = s.Tick(context.Background())
				}).To(Panic())
			})
		})
	})

	Describe("observationLoop ticker and restart channel", func() {
		Context("when restart is requested during observation", func() {
			It("should collect immediately", func() {
				collectCount := 0
				collector := supervisor.NewCollector(supervisor.CollectorConfig{
					Worker: &mockWorker{
						collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
							collectCount++

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
				initialCount := collectCount

				collector.Restart()

				time.Sleep(200 * time.Millisecond)

				Expect(collectCount).To(BeNumerically(">", initialCount))

				cancel()
				time.Sleep(100 * time.Millisecond)
			})
		})
	})
})

