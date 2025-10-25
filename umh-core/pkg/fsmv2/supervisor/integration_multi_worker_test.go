package supervisor_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

var _ = Describe("Multi-Worker Integration", func() {
	Describe("Registry independence", func() {
		It("should manage multiple workers independently in registry", func() {
			ctx := context.Background()

			tmpDir, err := os.MkdirTemp("", "multi-worker-test-*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			dbPath := filepath.Join(tmpDir, "test.db")

			cfg := basic.DefaultConfig(dbPath)
			cfg.MaintenanceOnShutdown = false
			basicStore, err := basic.NewStore(cfg)
			Expect(err).ToNot(HaveOccurred())
			defer basicStore.Close(ctx)

			registry := storage.NewRegistry()
			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_identity",
				WorkerType:    "integration_test",
				Role:          storage.RoleIdentity,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_desired",
				WorkerType:    "integration_test",
				Role:          storage.RoleDesired,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_observed",
				WorkerType:    "integration_test",
				Role:          storage.RoleObserved,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = basicStore.CreateCollection(ctx, "integration_test_identity", nil)
			Expect(err).ToNot(HaveOccurred())
			err = basicStore.CreateCollection(ctx, "integration_test_desired", nil)
			Expect(err).ToNot(HaveOccurred())
			err = basicStore.CreateCollection(ctx, "integration_test_observed", nil)
			Expect(err).ToNot(HaveOccurred())

			triangularStore := storage.NewTriangularStore(basicStore, registry)

			s := supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "integration_test",
				Store:      triangularStore,
				Logger:     zap.NewNop().Sugar(),
			})

			identity1 := fsmv2.Identity{ID: "worker1", Name: "Worker 1"}
			identity2 := fsmv2.Identity{ID: "worker2", Name: "Worker 2"}
			identity3 := fsmv2.Identity{ID: "worker3", Name: "Worker 3"}

			worker1 := &mockWorker{}
			worker2 := &mockWorker{}
			worker3 := &mockWorker{}

			Expect(s.AddWorker(identity1, worker1)).To(Succeed())
			Expect(s.AddWorker(identity2, worker2)).To(Succeed())
			Expect(s.AddWorker(identity3, worker3)).To(Succeed())

			workers := s.ListWorkers()
			Expect(workers).To(HaveLen(3))
			Expect(workers).To(ContainElements("worker1", "worker2", "worker3"))

			workerCtx1, err := s.GetWorker("worker1")
			Expect(err).NotTo(HaveOccurred())
			Expect(workerCtx1).NotTo(BeNil())

			workerCtx2, err := s.GetWorker("worker2")
			Expect(err).NotTo(HaveOccurred())
			Expect(workerCtx2).NotTo(BeNil())

			workerCtx3, err := s.GetWorker("worker3")
			Expect(err).NotTo(HaveOccurred())
			Expect(workerCtx3).NotTo(BeNil())
		})

		It("should remove workers independently without affecting others", func() {
			ctx := context.Background()

			tmpDir, err := os.MkdirTemp("", "multi-worker-test-*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			dbPath := filepath.Join(tmpDir, "test.db")

			cfg := basic.DefaultConfig(dbPath)
			cfg.MaintenanceOnShutdown = false
			basicStore, err := basic.NewStore(cfg)
			Expect(err).ToNot(HaveOccurred())
			defer basicStore.Close(ctx)

			registry := storage.NewRegistry()
			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_identity",
				WorkerType:    "integration_test",
				Role:          storage.RoleIdentity,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_desired",
				WorkerType:    "integration_test",
				Role:          storage.RoleDesired,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_observed",
				WorkerType:    "integration_test",
				Role:          storage.RoleObserved,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = basicStore.CreateCollection(ctx, "integration_test_identity", nil)
			Expect(err).ToNot(HaveOccurred())
			err = basicStore.CreateCollection(ctx, "integration_test_desired", nil)
			Expect(err).ToNot(HaveOccurred())
			err = basicStore.CreateCollection(ctx, "integration_test_observed", nil)
			Expect(err).ToNot(HaveOccurred())

			triangularStore := storage.NewTriangularStore(basicStore, registry)

			s := supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "integration_test",
				Store:      triangularStore,
				Logger:     zap.NewNop().Sugar(),
			})

			identity1 := fsmv2.Identity{ID: "worker1", Name: "Worker 1"}
			identity2 := fsmv2.Identity{ID: "worker2", Name: "Worker 2"}
			identity3 := fsmv2.Identity{ID: "worker3", Name: "Worker 3"}

			worker1 := &mockWorker{}
			worker2 := &mockWorker{}
			worker3 := &mockWorker{}

			Expect(s.AddWorker(identity1, worker1)).To(Succeed())
			Expect(s.AddWorker(identity2, worker2)).To(Succeed())
			Expect(s.AddWorker(identity3, worker3)).To(Succeed())

			Expect(s.ListWorkers()).To(HaveLen(3))

			Expect(s.RemoveWorker(ctx, "worker2")).To(Succeed())

			workers := s.ListWorkers()
			Expect(workers).To(HaveLen(2))
			Expect(workers).To(ContainElements("worker1", "worker3"))
			Expect(workers).NotTo(ContainElement("worker2"))

			_, err = s.GetWorker("worker2")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			_, err = s.GetWorker("worker1")
			Expect(err).NotTo(HaveOccurred())

			_, err = s.GetWorker("worker3")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle concurrent worker operations safely", func() {
			ctx := context.Background()

			tmpDir, err := os.MkdirTemp("", "multi-worker-test-*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			dbPath := filepath.Join(tmpDir, "test.db")

			cfg := basic.DefaultConfig(dbPath)
			cfg.MaintenanceOnShutdown = false
			basicStore, err := basic.NewStore(cfg)
			Expect(err).ToNot(HaveOccurred())
			defer basicStore.Close(ctx)

			registry := storage.NewRegistry()
			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_identity",
				WorkerType:    "integration_test",
				Role:          storage.RoleIdentity,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_desired",
				WorkerType:    "integration_test",
				Role:          storage.RoleDesired,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_observed",
				WorkerType:    "integration_test",
				Role:          storage.RoleObserved,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = basicStore.CreateCollection(ctx, "integration_test_identity", nil)
			Expect(err).ToNot(HaveOccurred())
			err = basicStore.CreateCollection(ctx, "integration_test_desired", nil)
			Expect(err).ToNot(HaveOccurred())
			err = basicStore.CreateCollection(ctx, "integration_test_observed", nil)
			Expect(err).ToNot(HaveOccurred())

			triangularStore := storage.NewTriangularStore(basicStore, registry)

			s := supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "integration_test",
				Store:      triangularStore,
				Logger:     zap.NewNop().Sugar(),
			})

			done := make(chan bool)

			go func() {
				defer GinkgoRecover()
				for i := 0; i < 10; i++ {
					identity := fsmv2.Identity{ID: "concurrent1", Name: "Concurrent Worker 1"}
					worker := &mockWorker{}
					s.AddWorker(identity, worker)
					time.Sleep(5 * time.Millisecond)
					s.RemoveWorker(context.Background(), "concurrent1")
				}
				done <- true
			}()

			go func() {
				defer GinkgoRecover()
				for i := 0; i < 10; i++ {
					identity := fsmv2.Identity{ID: "concurrent2", Name: "Concurrent Worker 2"}
					worker := &mockWorker{}
					s.AddWorker(identity, worker)
					time.Sleep(5 * time.Millisecond)
					s.RemoveWorker(context.Background(), "concurrent2")
				}
				done <- true
			}()

			<-done
			<-done

			workers := s.ListWorkers()
			Expect(workers).To(HaveLen(0))
		})
	})

	Describe("Collector independence", func() {
		It("should maintain separate collectors for each worker", func() {
			ctx := context.Background()

			tmpDir, err := os.MkdirTemp("", "multi-worker-test-*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			dbPath := filepath.Join(tmpDir, "test.db")

			cfg := basic.DefaultConfig(dbPath)
			cfg.MaintenanceOnShutdown = false
			basicStore, err := basic.NewStore(cfg)
			Expect(err).ToNot(HaveOccurred())
			defer basicStore.Close(ctx)

			registry := storage.NewRegistry()
			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_identity",
				WorkerType:    "integration_test",
				Role:          storage.RoleIdentity,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_desired",
				WorkerType:    "integration_test",
				Role:          storage.RoleDesired,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = registry.Register(&storage.CollectionMetadata{
				Name:          "integration_test_observed",
				WorkerType:    "integration_test",
				Role:          storage.RoleObserved,
				CSEFields:     []string{storage.FieldSyncID, storage.FieldCreatedAt, storage.FieldUpdatedAt},
				IndexedFields: []string{storage.FieldSyncID},
			})
			Expect(err).ToNot(HaveOccurred())

			err = basicStore.CreateCollection(ctx, "integration_test_identity", nil)
			Expect(err).ToNot(HaveOccurred())
			err = basicStore.CreateCollection(ctx, "integration_test_desired", nil)
			Expect(err).ToNot(HaveOccurred())
			err = basicStore.CreateCollection(ctx, "integration_test_observed", nil)
			Expect(err).ToNot(HaveOccurred())

			triangularStore := storage.NewTriangularStore(basicStore, registry)

			s := supervisor.NewSupervisor(supervisor.Config{
				WorkerType: "integration_test",
				Store:      triangularStore,
				Logger:     zap.NewNop().Sugar(),
			})

			identity1 := fsmv2.Identity{ID: "worker1", Name: "Worker 1"}
			identity2 := fsmv2.Identity{ID: "worker2", Name: "Worker 2"}

			worker1 := &mockWorker{}
			worker2 := &mockWorker{}

			Expect(s.AddWorker(identity1, worker1)).To(Succeed())
			Expect(s.AddWorker(identity2, worker2)).To(Succeed())

			workerCtx1, _ := s.GetWorker("worker1")
			workerCtx2, _ := s.GetWorker("worker2")

			Expect(workerCtx1).NotTo(Equal(workerCtx2))
		})
	})
})
