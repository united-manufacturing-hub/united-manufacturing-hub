// Copyright 2025 UMH Systems GmbH
package explorer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

type ContainerScenario struct {
	containerID     string
	supervisor      *supervisor.Supervisor
	worker          *container.ContainerWorker
	workerIdentity  fsmv2.Identity
	triangularStore *storage.TriangularStore
	basicStore      basic.Store
	dbPath          string
	logger          *zap.SugaredLogger
}

func NewContainerScenario() *ContainerScenario {
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	zapLogger, _ := config.Build()
	logger := zapLogger.Sugar()

	return &ContainerScenario{
		logger: logger,
	}
}

func (c *ContainerScenario) Setup(ctx context.Context) error {
	containerID := "host-system"
	c.containerID = containerID

	tempDir, err := os.MkdirTemp("", "explorer-test-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	dbPath := filepath.Join(tempDir, "test.db")
	c.dbPath = dbPath

	cfg := basic.Config{
		DBPath:                dbPath,
		MaintenanceOnShutdown: false,
		JournalMode:           basic.JournalModeDELETE,
	}
	basicStore, err := basic.NewStore(cfg)
	if err != nil {
		return fmt.Errorf("failed to create basic store: %w", err)
	}
	c.basicStore = basicStore

	registry := storage.NewRegistry()
	err = registry.Register(&storage.CollectionMetadata{
		Name:          "container_identity",
		WorkerType:    "container",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	if err != nil {
		return fmt.Errorf("failed to register identity collection: %w", err)
	}

	err = registry.Register(&storage.CollectionMetadata{
		Name:          "container_desired",
		WorkerType:    "container",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	if err != nil {
		return fmt.Errorf("failed to register desired collection: %w", err)
	}

	err = registry.Register(&storage.CollectionMetadata{
		Name:          "container_observed",
		WorkerType:    "container",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	if err != nil {
		return fmt.Errorf("failed to register observed collection: %w", err)
	}

	err = basicStore.CreateCollection(ctx, "container_identity", nil)
	if err != nil {
		return fmt.Errorf("failed to create identity collection: %w", err)
	}
	err = basicStore.CreateCollection(ctx, "container_desired", nil)
	if err != nil {
		return fmt.Errorf("failed to create desired collection: %w", err)
	}
	err = basicStore.CreateCollection(ctx, "container_observed", nil)
	if err != nil {
		return fmt.Errorf("failed to create observed collection: %w", err)
	}

	c.triangularStore = storage.NewTriangularStore(basicStore, registry)

	c.workerIdentity = fsmv2.Identity{
		ID:         containerID,
		Name:       "host-system",
		WorkerType: "container",
	}

	fsService := filesystem.NewDefaultService()
	monitorService := container_monitor.NewContainerMonitorServiceWithPath(fsService, tempDir)
	c.worker = container.NewContainerWorker(containerID, "host-system", monitorService)

	adapter := persistence.NewTriangularStoreAdapter(c.triangularStore)
	adapter.RegisterTypes(
		"container",
		reflect.TypeOf(&container.ContainerDesiredState{}),
		reflect.TypeOf(&container.ContainerObservedState{}),
		reflect.TypeOf(&container.ContainerIdentity{}),
	)

	sup := supervisor.NewSupervisor(supervisor.Config{
		WorkerType: "container",
		Store:      adapter,
		Logger:     c.logger,
	})

	err = sup.AddWorker(c.workerIdentity, c.worker)
	if err != nil {
		return fmt.Errorf("failed to add worker: %w", err)
	}

	c.supervisor = sup

	err = adapter.SaveIdentity(ctx, "container", containerID, c.workerIdentity)
	if err != nil {
		return fmt.Errorf("failed to save identity: %w", err)
	}

	initialDesired := &container.ContainerDesiredState{
		ID:       containerID,
		Shutdown: false,
	}
	err = adapter.SaveDesired(ctx, "container", containerID, initialDesired)
	if err != nil {
		return fmt.Errorf("failed to save initial desired state: %w", err)
	}

	sup.Start(ctx)

	maxWait := 5 * time.Second
	pollInterval := 100 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	c.logger.Infof("Waiting for first observation collection...")
	for time.Now().Before(deadline) {
		_, err := adapter.LoadSnapshot(ctx, "container", containerID)
		if err == nil {
			c.logger.Infof("First observation collected and saved successfully")
			break
		}
		time.Sleep(pollInterval)
	}

	time.Sleep(2 * time.Second)

	return nil
}

func (c *ContainerScenario) Tick(ctx context.Context) error {
	return c.supervisor.Tick(ctx)
}

func (c *ContainerScenario) GetCurrentState() (string, string) {
	if c.supervisor == nil {
		return "NotSetup", "scenario not initialized"
	}

	stateName, reason, err := c.supervisor.GetWorkerState(c.containerID)
	if err != nil {
		return "Error", err.Error()
	}
	return stateName, reason
}

func (c *ContainerScenario) GetObservedState() interface{} {
	if c.supervisor == nil {
		return nil
	}

	// NOTE: Supervisor API does not expose observed state data.
	// This would require LoadSnapshot() which is storage layer implementation detail.
	// Returning nil indicates this limitation for now.
	return nil
}

func (c *ContainerScenario) GetDesiredState() interface{} {
	if c.supervisor == nil {
		return nil
	}

	// NOTE: Supervisor API does not expose desired state data.
	// This would require LoadDesired() which is storage layer implementation detail.
	// Returning nil indicates this limitation for now.
	return nil
}

func (c *ContainerScenario) InjectShutdown() error {
	if c.supervisor == nil {
		return fmt.Errorf("supervisor not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return c.supervisor.RequestShutdown(ctx, c.containerID, "test shutdown injection")
}

func (c *ContainerScenario) Cleanup(ctx context.Context) error {
	var errs []error

	if c.basicStore != nil {
		if err := c.basicStore.Close(ctx); err != nil {
			c.logger.Errorf("cleanup error: failed to close store: %v", err)
			errs = append(errs, fmt.Errorf("failed to close store: %w", err))
		}
	}

	if c.dbPath != "" {
		tempDir := filepath.Dir(c.dbPath)
		if err := os.RemoveAll(tempDir); err != nil {
			c.logger.Errorf("cleanup error: failed to remove temp dir: %v", err)
			errs = append(errs, fmt.Errorf("failed to remove temp dir: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}

	return nil
}
