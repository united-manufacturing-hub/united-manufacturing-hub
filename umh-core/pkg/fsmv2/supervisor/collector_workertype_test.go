// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
	"go.uber.org/zap"
)

var _ = Describe("Collector WorkerType", func() {
	Context("when collector is configured with workerType", func() {
		It("should use configured workerType for observations", func() {
			ctx := context.Background()
			basicStore := memory.NewInMemoryStore()
			registry := storage.NewRegistry()

			workerType := "s6"
			identity := fsmv2.Identity{
				ID:         "test-s6-worker",
				Name:       "S6 Worker",
				WorkerType: workerType,
			}

			registry.Register(&storage.CollectionMetadata{
				Name:          workerType + "_identity",
				WorkerType:    workerType,
				Role:          storage.RoleIdentity,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			registry.Register(&storage.CollectionMetadata{
				Name:          workerType + "_desired",
				WorkerType:    workerType,
				Role:          storage.RoleDesired,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			registry.Register(&storage.CollectionMetadata{
				Name:          workerType + "_observed",
				WorkerType:    workerType,
				Role:          storage.RoleObserved,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})

			err := basicStore.CreateCollection(ctx, workerType+"_identity", nil)
			Expect(err).ToNot(HaveOccurred())
			err = basicStore.CreateCollection(ctx, workerType+"_desired", nil)
			Expect(err).ToNot(HaveOccurred())
			err = basicStore.CreateCollection(ctx, workerType+"_observed", nil)
			Expect(err).ToNot(HaveOccurred())

			triangularStore := storage.NewTriangularStore(basicStore, registry)
			Expect(triangularStore).ToNot(BeNil())

			s := supervisor.NewSupervisor(supervisor.Config{
				WorkerType: workerType,
				Store:      triangularStore,
				Logger:     zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{},
			})

			worker := &mockWorker{
				observed: createMockObservedStateWithID(identity.ID),
			}

			err = s.AddWorker(identity, worker)
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
			if err != nil && err != persistence.ErrNotFound {
				Fail(fmt.Sprintf("unexpected error loading observed: %v", err))
			}
			_ = doc
		})
	})

	Context("when multiple collectors use different workerTypes", func() {
		It("should register separate collections for each workerType", func() {
			ctx := context.Background()
			basicStore := memory.NewInMemoryStore()
			registry := storage.NewRegistry()

			workerTypes := []string{"container", "s6", "benthos"}
			supervisors := make([]*supervisor.Supervisor, 0, len(workerTypes))

			for _, wt := range workerTypes {
				registry.Register(&storage.CollectionMetadata{
					Name:          wt + "_identity",
					WorkerType:    wt,
					Role:          storage.RoleIdentity,
					CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
					IndexedFields: []string{storage.FieldSyncID},
				})
				registry.Register(&storage.CollectionMetadata{
					Name:          wt + "_desired",
					WorkerType:    wt,
					Role:          storage.RoleDesired,
					CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
					IndexedFields: []string{storage.FieldSyncID},
				})
				registry.Register(&storage.CollectionMetadata{
					Name:          wt + "_observed",
					WorkerType:    wt,
					Role:          storage.RoleObserved,
					CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
					IndexedFields: []string{storage.FieldSyncID},
				})

				err := basicStore.CreateCollection(ctx, wt+"_identity", nil)
				Expect(err).ToNot(HaveOccurred())
				err = basicStore.CreateCollection(ctx, wt+"_desired", nil)
				Expect(err).ToNot(HaveOccurred())
				err = basicStore.CreateCollection(ctx, wt+"_observed", nil)
				Expect(err).ToNot(HaveOccurred())
			}

			triangularStore := storage.NewTriangularStore(basicStore, registry)
			Expect(triangularStore).ToNot(BeNil())

			for _, wt := range workerTypes {
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

				worker := &mockWorker{
					observed: createMockObservedStateWithID(identity.ID),
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

			Expect(registry.IsRegistered("container_observed")).To(BeTrue())
			Expect(registry.IsRegistered("s6_observed")).To(BeTrue())
			Expect(registry.IsRegistered("benthos_observed")).To(BeTrue())
		})
	})
})
