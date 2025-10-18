// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"go.uber.org/zap"
)

var _ = Describe("Tick with Data Freshness", func() {
	var (
		s            *supervisor.Supervisor
		initialState *mockState
		nextState    *mockState
		store        *mockStore
	)

	BeforeEach(func() {
		initialState = &mockState{}
		nextState = &mockState{}
		initialState.nextState = nextState
	})

	Context("when data is stale", func() {
		It("should skip state transition", func() {
			store = &mockStore{
				snapshot: &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Desired:  &mockDesiredState{},
					Observed: &mockObservedState{timestamp: time.Now().Add(-15 * time.Second)},
				},
			}

			s = supervisor.NewSupervisor(supervisor.Config{
				Worker:   &mockWorker{initialState: initialState},
				Identity: mockIdentity(),
				Store:    store,
				Logger:   zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					StaleThreshold: 10 * time.Second,
				},
			})

			err := s.Tick(context.Background())
			Expect(err).ToNot(HaveOccurred())

			Expect(s.GetCurrentState()).To(Equal(initialState.String()))
		})
	})

	Context("when data is fresh", func() {
		It("should progress with state transition", func() {
			store = &mockStore{
				snapshot: &fsmv2.Snapshot{
					Identity: mockIdentity(),
					Desired:  &mockDesiredState{},
					Observed: &mockObservedState{timestamp: time.Now()},
				},
			}

			s = supervisor.NewSupervisor(supervisor.Config{
				Worker:   &mockWorker{initialState: initialState},
				Identity: mockIdentity(),
				Store:    store,
				Logger:   zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					StaleThreshold: 10 * time.Second,
				},
			})

			err := s.Tick(context.Background())
			Expect(err).ToNot(HaveOccurred())

			Expect(s.GetCurrentState()).To(Equal(nextState.String()))
		})
	})
})
