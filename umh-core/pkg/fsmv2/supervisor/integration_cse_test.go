// Copyright 2025 UMH Systems GmbH
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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

var _ = Describe("Integration with Real CSE Storage", func() {
	It("should persist worker state and recover from storage", func() {
		ctx := context.Background()

		tmpDir, err := os.MkdirTemp("", "cse-integration-test-*")
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
			Name:          "container_identity",
			WorkerType:    "container",
			Role:          storage.RoleIdentity,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})
		Expect(err).ToNot(HaveOccurred())

		err = registry.Register(&storage.CollectionMetadata{
			Name:          "container_desired",
			WorkerType:    "container",
			Role:          storage.RoleDesired,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})
		Expect(err).ToNot(HaveOccurred())

		err = registry.Register(&storage.CollectionMetadata{
			Name:          "container_observed",
			WorkerType:    "container",
			Role:          storage.RoleObserved,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})
		Expect(err).ToNot(HaveOccurred())

		err = basicStore.CreateCollection(ctx, "container_identity", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "container_desired", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "container_observed", nil)
		Expect(err).ToNot(HaveOccurred())

		triangularStore := storage.NewTriangularStore(basicStore, registry)

		identity := fsmv2.Identity{
			ID:         "test-worker-cse",
			Name:       "Test Worker CSE",
			WorkerType: "container",
		}

		observationTimestamp := time.Now()
		worker := &mockWorker{
			initialState: &mockState{},
			collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
				return &container.ContainerObservedState{CollectedAt: observationTimestamp}, nil
			},
		}

		s1 := supervisor.NewSupervisor(supervisor.Config{
			WorkerType: "container",
			Store:      triangularStore,
			Logger:     zap.NewNop().Sugar(),
			CollectorHealth: supervisor.CollectorHealthConfig{
				StaleThreshold:     5 * time.Second,
				Timeout:            10 * time.Second,
				MaxRestartAttempts: 3,
			},
		})

		err = s1.AddWorker(identity, worker)
		Expect(err).ToNot(HaveOccurred())

		time.Sleep(100 * time.Millisecond)

		err = s1.Tick(ctx)
		Expect(err).ToNot(HaveOccurred())

		snapshot, err := triangularStore.LoadSnapshot(ctx, "container", identity.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(snapshot).ToNot(BeNil())
		Expect(snapshot.Identity).ToNot(BeNil())
		Expect(snapshot.Desired).ToNot(BeNil())
		Expect(snapshot.Observed).ToNot(BeNil())

		Expect(snapshot.Identity["id"]).To(Equal(identity.ID))
		Expect(snapshot.Identity["name"]).To(Equal(identity.Name))
		Expect(snapshot.Identity["workerType"]).To(Equal(identity.WorkerType))

		s2 := supervisor.NewSupervisor(supervisor.Config{
			WorkerType: "container",
			Store:      triangularStore,
			Logger:     zap.NewNop().Sugar(),
			CollectorHealth: supervisor.CollectorHealthConfig{
				StaleThreshold:     5 * time.Second,
				Timeout:            10 * time.Second,
				MaxRestartAttempts: 3,
			},
		})

		worker2 := &mockWorker{
			initialState: &mockState{},
			collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
				return &container.ContainerObservedState{CollectedAt: time.Now()}, nil
			},
		}

		err = s2.AddWorker(identity, worker2)
		Expect(err).ToNot(HaveOccurred())

		err = s2.Tick(ctx)
		Expect(err).ToNot(HaveOccurred())

		snapshot2, err := triangularStore.LoadSnapshot(ctx, "container", identity.ID)
		Expect(err).ToNot(HaveOccurred())
		Expect(snapshot2).ToNot(BeNil())

		Expect(snapshot2.Identity["id"]).To(Equal(snapshot.Identity["id"]))
		Expect(snapshot2.Identity["name"]).To(Equal(snapshot.Identity["name"]))
		Expect(snapshot2.Identity["workerType"]).To(Equal(snapshot.Identity["workerType"]))
	})
})
