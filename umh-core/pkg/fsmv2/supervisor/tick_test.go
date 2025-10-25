// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
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
					Desired:  &container.ContainerDesiredState{},
					Observed: &container.ContainerObservedState{CollectedAt: time.Now().Add(-15 * time.Second)},
				},
			}

			s = newSupervisorWithWorker(&mockWorker{initialState: initialState}, store, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
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
					Desired:  &container.ContainerDesiredState{},
					Observed: &container.ContainerObservedState{CollectedAt: time.Now()},
				},
			}

			s = newSupervisorWithWorker(&mockWorker{initialState: initialState}, store, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})

			err := s.Tick(context.Background())
			Expect(err).ToNot(HaveOccurred())

			Expect(s.GetCurrentState()).To(Equal(nextState.String()))
		})
	})

	Context("when ObservedState is nil (Invariant I16)", func() {
		It("should panic with clear error message about nil ObservedState", func() {
			identity := mockIdentity()
			worker := &mockWorker{
				initialState: initialState,
				observed:     &container.ContainerObservedState{CollectedAt: time.Now()},
			}

			store = &mockStore{}

			s = newSupervisorWithWorker(worker, store, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})

			store.loadSnapshot = func(ctx context.Context, workerType string, id string) (*storage.Snapshot, error) {
				identityDoc := basic.Document{
					"ID":         identity.ID,
					"Name":       identity.Name,
					"WorkerType": identity.WorkerType,
				}

				return &storage.Snapshot{
					Identity: identityDoc,
					Desired:  basic.Document{},
					Observed: basic.Document{},
				}, nil
			}

			Expect(func() {
				_ = s.Tick(context.Background())
			}).Should(PanicWith(MatchRegexp("Invariant I16 violated.*nil ObservedState")))
		})
	})
})
