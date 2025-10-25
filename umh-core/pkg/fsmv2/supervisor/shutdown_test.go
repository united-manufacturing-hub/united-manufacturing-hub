// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

var _ = Describe("Shutdown Escalation", func() {
	Context("when max restart attempts exhausted", func() {
		It("should request graceful shutdown", func() {
			shutdownRequested := false
			store := &mockStore{
				loadSnapshot: func(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
					identity := &container.ContainerIdentity{
						ID:   "test-worker",
						Name: "Test Worker",
					}

					desired := &container.ContainerDesiredState{
						Shutdown: shutdownRequested,
					}

					observed := &container.ContainerObservedState{
						CollectedAt: time.Now().Add(-25 * time.Second),
					}

					return &fsmv2.Snapshot{
						Identity: identity,
						Desired:  desired,
						Observed: observed,
					}, nil
				},
				saveDesired: func(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
					if d, ok := desired.(*container.ContainerDesiredState); ok {
						shutdownRequested = d.ShutdownRequested()
					}

					return nil
				},
			}

			s := newSupervisorWithWorker(&mockWorker{}, store, supervisor.CollectorHealthConfig{
				StaleThreshold:     10 * time.Second,
				Timeout:            20 * time.Second,
				MaxRestartAttempts: 3,
			})

			s.SetRestartCount(3)

			err := s.Tick(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("collector unresponsive"))

			Expect(shutdownRequested).To(BeTrue())
		})
	})

	Context("while restart attempts remain", func() {
		It("should NOT request shutdown", func() {
			shutdownRequested := false
			store := &mockStore{
				loadSnapshot: func(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
					identity := &container.ContainerIdentity{
						ID:   "test-worker",
						Name: "Test Worker",
					}

					desired := &container.ContainerDesiredState{}

					observed := &container.ContainerObservedState{
						CollectedAt: time.Now().Add(-25 * time.Second),
					}

					return &fsmv2.Snapshot{
						Identity: identity,
						Desired:  desired,
						Observed: observed,
					}, nil
				},
				saveDesired: func(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
					if d, ok := desired.(*container.ContainerDesiredState); ok {
						shutdownRequested = d.ShutdownRequested()
					}

					return nil
				},
			}

			s := newSupervisorWithWorker(&mockWorker{}, store, supervisor.CollectorHealthConfig{
				StaleThreshold:     10 * time.Second,
				Timeout:            20 * time.Second,
				MaxRestartAttempts: 3,
			})

			s.SetRestartCount(1)

			err := s.Tick(context.Background())
			Expect(err).ToNot(HaveOccurred())

			Expect(shutdownRequested).To(BeFalse())
		})
	})
})

var _ = Describe("RequestShutdown", func() {
	var (
		ctx             context.Context
		sup             *supervisor.Supervisor
		adapter         *persistence.TriangularStoreAdapter
		triangularStore *storage.TriangularStore
		basicStore      basic.Store
		registry        *storage.Registry
		worker          *mockWorker
		logger          *zap.SugaredLogger
		tempDir         string
	)

	BeforeEach(func() {
		ctx = context.Background()

		zapLogger, _ := zap.NewDevelopment()
		logger = zapLogger.Sugar()

		tempDir, _ = os.MkdirTemp("", "shutdown-test-*")
		cfg := basic.Config{
			DBPath:                filepath.Join(tempDir, "test.db"),
			JournalMode:           basic.JournalModeDELETE,
			MaintenanceOnShutdown: false,
		}
		basicStore, _ = basic.NewStore(cfg)

		registry = storage.NewRegistry()
		registry.Register(&storage.CollectionMetadata{
			Name:          "container_identity",
			WorkerType:    "container",
			Role:          storage.RoleIdentity,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})
		registry.Register(&storage.CollectionMetadata{
			Name:          "container_desired",
			WorkerType:    "container",
			Role:          storage.RoleDesired,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})
		registry.Register(&storage.CollectionMetadata{
			Name:          "container_observed",
			WorkerType:    "container",
			Role:          storage.RoleObserved,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})

		basicStore.CreateCollection(ctx, "container_identity", nil)
		basicStore.CreateCollection(ctx, "container_desired", nil)
		basicStore.CreateCollection(ctx, "container_observed", nil)

		triangularStore = storage.NewTriangularStore(basicStore, registry)
		adapter = persistence.NewTriangularStoreAdapter(triangularStore)

		adapter.RegisterTypes(
			"container",
			reflect.TypeOf(&container.ContainerDesiredState{}),
			reflect.TypeOf(&container.ContainerObservedState{}),
			reflect.TypeOf(&container.ContainerIdentity{}),
		)

		sup = supervisor.NewSupervisor(supervisor.Config{
			WorkerType: "container",
			Store:      adapter,
			Logger:     logger,
		})

		worker = &mockWorker{}
		worker.collectFunc = func(ctx context.Context) (fsmv2.ObservedState, error) {
			return &container.ContainerObservedState{
				ID:             "test-container",
				CPUUsageMCores: 1000,
				CollectedAt:    time.Now(),
			}, nil
		}
	})

	AfterEach(func() {
		if basicStore != nil {
			basicStore.Close(ctx)
		}
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	It("should set shutdown flag in desired state", func() {
		identity := fsmv2.Identity{
			ID:         "test-container",
			Name:       "test-container",
			WorkerType: "container",
		}

		err := sup.AddWorker(identity, worker)
		Expect(err).NotTo(HaveOccurred())

		initialDesired := &container.ContainerDesiredState{
			ID:       "test-container",
			Shutdown: false,
		}
		err = adapter.SaveDesired(ctx, "container", "test-container", initialDesired)
		Expect(err).NotTo(HaveOccurred())

		err = sup.RequestShutdown(ctx, "test-container", "test shutdown")
		Expect(err).NotTo(HaveOccurred())

		loaded, err := adapter.LoadDesired(ctx, "container", "test-container")
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded).NotTo(BeNil())

		desired := loaded.(*container.ContainerDesiredState)
		Expect(desired.ShutdownRequested()).To(BeTrue(), "Shutdown flag should be set")
		Expect(desired.ID).To(Equal("test-container"), "ID should be preserved")
	})
})
