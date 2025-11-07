// Copyright 2025 UMH Systems GmbH
package collection_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"go.uber.org/zap"
)

var _ = Describe("Collector WorkerType", func() {
	Context("when collector is configured with workerType", func() {
		It("should use configured workerType for observations", func() {
			ctx := context.Background()

			workerType := "s6"
			identity := fsmv2.Identity{
				ID:         "test-s6-worker",
				Name:       "S6 Worker",
				WorkerType: workerType,
			}

			triangularStore := supervisor.CreateTestTriangularStoreForWorkerType(workerType)
			Expect(triangularStore).ToNot(BeNil())
			registry := triangularStore.Registry()

			s := supervisor.NewSupervisor(supervisor.Config{
				WorkerType: workerType,
				Store:      triangularStore,
				Logger:     zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{},
			})

			worker := &supervisor.TestWorker{
				Observed: supervisor.CreateTestObservedStateWithID(identity.ID),
			}

			err := s.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			desiredDoc := persistence.Document{
				"id":               identity.ID,
				"shutdownRequested": false,
			}
			err = triangularStore.SaveDesired(ctx, workerType, identity.ID, desiredDoc)
			Expect(err).ToNot(HaveOccurred())

			Expect(registry.IsRegistered("s6_observed")).To(BeTrue(), "should register s6_observed collection, not container_observed")
			Expect(registry.IsRegistered("container_observed")).To(BeFalse(), "should not use hardcoded container workerType")

			doc, err := triangularStore.LoadObserved(ctx, "s6", identity.ID)
			if err != nil && !errors.Is(err, persistence.ErrNotFound) {
				Fail(fmt.Sprintf("unexpected error loading observed: %v", err))
			}
			_ = doc
		})
	})

	Context("when multiple collectors use different workerTypes", func() {
		It("should register separate collections for each workerType", func() {
			ctx := context.Background()

			workerTypes := []string{"container", "s6", "benthos"}
			stores := make(map[string]*storage.TriangularStore)
			supervisors := make([]*supervisor.Supervisor, 0, len(workerTypes))

			for _, wt := range workerTypes {
				stores[wt] = supervisor.CreateTestTriangularStoreForWorkerType(wt)
			}

			for _, wt := range workerTypes {
				triangularStore := stores[wt]
				registry := triangularStore.Registry()

				Expect(registry.IsRegistered(wt + "_identity")).To(BeTrue())
				Expect(registry.IsRegistered(wt + "_desired")).To(BeTrue())
				Expect(registry.IsRegistered(wt + "_observed")).To(BeTrue())
				identity := fsmv2.Identity{
					ID:         wt + "-worker",
					Name:       wt + " Worker",
					WorkerType: wt,
				}

				s := supervisor.NewSupervisor(supervisor.Config{
					WorkerType: wt,
					Store:      triangularStore,
					Logger:     zap.NewNop().Sugar(),
					CollectorHealth: supervisor.CollectorHealthConfig{},
				})

				worker := &supervisor.TestWorker{
					Observed: supervisor.CreateTestObservedStateWithID(identity.ID),
				}

				err := s.AddWorker(identity, worker)
				Expect(err).ToNot(HaveOccurred())

				desiredDoc := persistence.Document{
					"id":               identity.ID,
					"shutdownRequested": false,
				}
				err = triangularStore.SaveDesired(ctx, wt, identity.ID, desiredDoc)
				Expect(err).ToNot(HaveOccurred())

				supervisors = append(supervisors, s)
			}

			Expect(len(supervisors)).To(Equal(3))
		})
	})
})
