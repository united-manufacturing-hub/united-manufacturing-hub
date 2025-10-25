// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package persistence_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

func TestTriangularAdapter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TriangularStoreAdapter Suite")
}

// TestDesiredState implements fsmv2.DesiredState for testing
type TestDesiredState struct {
	ID                   string    `json:"id"`
	Config               string    `json:"config"`
	RequestedAt          time.Time `json:"requested_at"`
	shutdownRequested    bool
}

func (d TestDesiredState) ShutdownRequested() bool {
	return d.shutdownRequested
}

// TestObservedState implements fsmv2.ObservedState for testing
type TestObservedState struct {
	ID              string            `json:"id"`
	Status          string            `json:"status"`
	CPU             float64           `json:"cpu"`
	ObservedAt      time.Time         `json:"observed_at"`
	DesiredConfig   string            `json:"desired_config"`
}

func (o TestObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return TestDesiredState{
		ID:     o.ID,
		Config: o.DesiredConfig,
	}
}

func (o TestObservedState) GetTimestamp() time.Time {
	return o.ObservedAt
}

var _ = Describe("TriangularStoreAdapter", func() {
	var (
		ctx        context.Context
		store      basic.Store
		registry   *storage.Registry
		triangular *storage.TriangularStore
		adapter    *persistence.TriangularStoreAdapter
		workerType string
		workerID   string
		dbPath     string
	)

	BeforeEach(func() {
		ctx = context.Background()
		workerType = "container"
		workerID = "test-worker-123"

		// Create temporary SQLite store (in-memory doesn't work with journal modes)
		var err error
		tmpfile, err := os.CreateTemp("", "test-triangular-*.db")
		Expect(err).ToNot(HaveOccurred())
		dbPath = tmpfile.Name()
		tmpfile.Close()

		cfg := basic.DefaultConfig(dbPath)
		store, err = basic.NewStore(cfg)
		Expect(err).ToNot(HaveOccurred())

		// Create collections in SQLite
		store.CreateCollection(ctx, workerType+"_identity", nil)
		store.CreateCollection(ctx, workerType+"_desired", nil)
		store.CreateCollection(ctx, workerType+"_observed", nil)

		// Create registry and register container collections
		registry = storage.NewRegistry()

		err = registry.Register(&storage.CollectionMetadata{
			Name:          workerType + "_identity",
			WorkerType:    workerType,
			Role:          storage.RoleIdentity,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})
		Expect(err).ToNot(HaveOccurred())

		err = registry.Register(&storage.CollectionMetadata{
			Name:          workerType + "_desired",
			WorkerType:    workerType,
			Role:          storage.RoleDesired,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})
		Expect(err).ToNot(HaveOccurred())

		err = registry.Register(&storage.CollectionMetadata{
			Name:          workerType + "_observed",
			WorkerType:    workerType,
			Role:          storage.RoleObserved,
			CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
			IndexedFields: []string{storage.FieldSyncID},
		})
		Expect(err).ToNot(HaveOccurred())

		// Create triangular store
		triangular = storage.NewTriangularStore(store, registry)

		// Create adapter
		adapter = persistence.NewTriangularStoreAdapter(triangular)

		// Register types for our test worker
		adapter.RegisterTypes(
			workerType,
			reflect.TypeOf(TestDesiredState{}),
			reflect.TypeOf(TestObservedState{}),
			reflect.TypeOf(fsmv2.Identity{}),
		)
	})

	AfterEach(func() {
		if store != nil {
			store.Close(ctx)
		}
		if dbPath != "" {
			os.Remove(dbPath)
		}
	})

	Describe("Identity Operations", func() {
		It("should save and load identity", func() {
			identity := fsmv2.Identity{
				ID:         workerID,
				Name:       "Test Worker",
				WorkerType: workerType,
			}

			// Save
			err := adapter.SaveIdentity(ctx, workerType, workerID, identity)
			Expect(err).ToNot(HaveOccurred())

			// Load
			loaded, err := adapter.LoadIdentity(ctx, workerType, workerID)
			Expect(err).ToNot(HaveOccurred())
			Expect(loaded).ToNot(BeNil())

			loadedIdentity, ok := loaded.(fsmv2.Identity)
			Expect(ok).To(BeTrue())
			Expect(loadedIdentity.ID).To(Equal(identity.ID))
			Expect(loadedIdentity.Name).To(Equal(identity.Name))
			Expect(loadedIdentity.WorkerType).To(Equal(identity.WorkerType))
		})

		It("should return nil for non-existent identity", func() {
			loaded, err := adapter.LoadIdentity(ctx, workerType, "non-existent")
			Expect(err).To(HaveOccurred())
			Expect(loaded).To(BeNil())
		})
	})

	Describe("Desired State Operations", func() {
		It("should save and load desired state", func() {
			now := time.Now().UTC().Truncate(time.Second)
			desired := TestDesiredState{
				ID:          workerID,
				Config:      "production",
				RequestedAt: now,
			}

			// Save
			err := adapter.SaveDesired(ctx, workerType, workerID, desired)
			Expect(err).ToNot(HaveOccurred())

			// Load
			loaded, err := adapter.LoadDesired(ctx, workerType, workerID)
			Expect(err).ToNot(HaveOccurred())
			Expect(loaded).ToNot(BeNil())

			loadedDesired, ok := loaded.(TestDesiredState)
			Expect(ok).To(BeTrue())
			Expect(loadedDesired.ID).To(Equal(desired.ID))
			Expect(loadedDesired.Config).To(Equal(desired.Config))
			Expect(loadedDesired.RequestedAt.Unix()).To(Equal(now.Unix()))
		})

		It("should return nil for non-existent desired state", func() {
			loaded, err := adapter.LoadDesired(ctx, workerType, "non-existent")
			Expect(err).To(HaveOccurred())
			Expect(loaded).To(BeNil())
		})

		It("should handle ShutdownRequested field", func() {
			desired := TestDesiredState{
				ID:                workerID,
				Config:            "production",
				RequestedAt:       time.Now().UTC(),
				shutdownRequested: true,
			}

			err := adapter.SaveDesired(ctx, workerType, workerID, desired)
			Expect(err).ToNot(HaveOccurred())

			loaded, err := adapter.LoadDesired(ctx, workerType, workerID)
			Expect(err).ToNot(HaveOccurred())

			// Note: shutdownRequested is private, so it won't round-trip
			// This is expected behavior - shutdown is managed by supervisor
			Expect(loaded.ShutdownRequested()).To(BeFalse())
		})
	})

	Describe("Observed State Operations", func() {
		It("should save and load observed state", func() {
			now := time.Now().UTC().Truncate(time.Second)
			observed := TestObservedState{
				ID:            workerID,
				Status:        "running",
				CPU:           45.2,
				ObservedAt:    now,
				DesiredConfig: "production",
			}

			// Save
			err := adapter.SaveObserved(ctx, workerType, workerID, observed)
			Expect(err).ToNot(HaveOccurred())

			// Load
			loaded, err := adapter.LoadObserved(ctx, workerType, workerID)
			Expect(err).ToNot(HaveOccurred())
			Expect(loaded).ToNot(BeNil())

			loadedObserved, ok := loaded.(TestObservedState)
			Expect(ok).To(BeTrue())
			Expect(loadedObserved.ID).To(Equal(observed.ID))
			Expect(loadedObserved.Status).To(Equal(observed.Status))
			Expect(loadedObserved.CPU).To(Equal(observed.CPU))
			Expect(loadedObserved.ObservedAt.Unix()).To(Equal(now.Unix()))
			Expect(loadedObserved.DesiredConfig).To(Equal(observed.DesiredConfig))
		})

		It("should return nil for non-existent observed state", func() {
			loaded, err := adapter.LoadObserved(ctx, workerType, "non-existent")
			Expect(err).To(HaveOccurred())
			Expect(loaded).To(BeNil())
		})

		It("should preserve timestamp precision", func() {
			now := time.Now().UTC()
			observed := TestObservedState{
				ID:         workerID,
				Status:     "running",
				ObservedAt: now,
			}

			err := adapter.SaveObserved(ctx, workerType, workerID, observed)
			Expect(err).ToNot(HaveOccurred())

			loaded, err := adapter.LoadObserved(ctx, workerType, workerID)
			Expect(err).ToNot(HaveOccurred())

			loadedObserved := loaded.(TestObservedState)
			// SQLite stores timestamps with second precision
			Expect(loadedObserved.ObservedAt.Unix()).To(Equal(now.Unix()))
		})
	})

	Describe("Snapshot Operations", func() {
		It("should load complete snapshot", func() {
			// Save all three parts
			identity := fsmv2.Identity{
				ID:         workerID,
				Name:       "Test Worker",
				WorkerType: workerType,
			}
			err := adapter.SaveIdentity(ctx, workerType, workerID, identity)
			Expect(err).ToNot(HaveOccurred())

			now := time.Now().UTC().Truncate(time.Second)
			desired := TestDesiredState{
				ID:          workerID,
				Config:      "production",
				RequestedAt: now,
			}
			err = adapter.SaveDesired(ctx, workerType, workerID, desired)
			Expect(err).ToNot(HaveOccurred())

			observed := TestObservedState{
				ID:            workerID,
				Status:        "running",
				CPU:           45.2,
				ObservedAt:    now,
				DesiredConfig: "production",
			}
			err = adapter.SaveObserved(ctx, workerType, workerID, observed)
			Expect(err).ToNot(HaveOccurred())

			// Load snapshot
			snapshot, err := adapter.LoadSnapshot(ctx, workerType, workerID)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).ToNot(BeNil())

			// Verify identity
			Expect(snapshot.Identity.ID).To(Equal(identity.ID))
			Expect(snapshot.Identity.Name).To(Equal(identity.Name))

			// Verify desired
			loadedDesired, ok := snapshot.Desired.(TestDesiredState)
			Expect(ok).To(BeTrue())
			Expect(loadedDesired.Config).To(Equal(desired.Config))

			// Verify observed
			loadedObserved, ok := snapshot.Observed.(TestObservedState)
			Expect(ok).To(BeTrue())
			Expect(loadedObserved.Status).To(Equal(observed.Status))
			Expect(loadedObserved.CPU).To(Equal(observed.CPU))
		})

		It("should return error if any part is missing", func() {
			// Only save identity, not desired or observed
			identity := fsmv2.Identity{
				ID:         workerID,
				Name:       "Test Worker",
				WorkerType: workerType,
			}
			err := adapter.SaveIdentity(ctx, workerType, workerID, identity)
			Expect(err).ToNot(HaveOccurred())

			// Try to load snapshot
			_, err = adapter.LoadSnapshot(ctx, workerType, workerID)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Sync ID Operations", func() {
		It("should start with sync ID 0", func() {
			syncID, err := adapter.GetLastSyncID(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(syncID).To(Equal(int64(0)))
		})

		It("should increment sync ID", func() {
			// First increment
			syncID, err := adapter.IncrementSyncID(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(syncID).To(Equal(int64(1)))

			// Second increment
			syncID, err = adapter.IncrementSyncID(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(syncID).To(Equal(int64(2)))

			// Verify GetLastSyncID returns current value
			syncID, err = adapter.GetLastSyncID(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(syncID).To(Equal(int64(2)))
		})

		It("should auto-increment sync ID on save operations", func() {
			// Save operations automatically increment sync ID via TriangularStore
			identity := fsmv2.Identity{
				ID:         workerID,
				Name:       "Test Worker",
				WorkerType: workerType,
			}
			err := adapter.SaveIdentity(ctx, workerType, workerID, identity)
			Expect(err).ToNot(HaveOccurred())

			syncID, err := adapter.GetLastSyncID(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(syncID).To(Equal(int64(1)))

			desired := TestDesiredState{
				ID:     workerID,
				Config: "production",
			}
			err = adapter.SaveDesired(ctx, workerType, workerID, desired)
			Expect(err).ToNot(HaveOccurred())

			syncID, err = adapter.GetLastSyncID(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(syncID).To(Equal(int64(2)))
		})
	})

	Describe("Type Conversion", func() {
		It("should handle nil values correctly", func() {
			// This tests that nil is preserved through round-trip
			desired := TestDesiredState{
				ID:          workerID,
				Config:      "",
				RequestedAt: time.Time{}, // Zero time
			}

			err := adapter.SaveDesired(ctx, workerType, workerID, desired)
			Expect(err).ToNot(HaveOccurred())

			loaded, err := adapter.LoadDesired(ctx, workerType, workerID)
			Expect(err).ToNot(HaveOccurred())

			loadedDesired := loaded.(TestDesiredState)
			Expect(loadedDesired.Config).To(Equal(""))
			Expect(loadedDesired.RequestedAt.IsZero()).To(BeTrue())
		})

		It("should handle nested structures", func() {
			// TestObservedState contains a mirror of desired state
			observed := TestObservedState{
				ID:            workerID,
				Status:        "running",
				DesiredConfig: "production-v2",
				ObservedAt:    time.Now().UTC(),
			}

			err := adapter.SaveObserved(ctx, workerType, workerID, observed)
			Expect(err).ToNot(HaveOccurred())

			loaded, err := adapter.LoadObserved(ctx, workerType, workerID)
			Expect(err).ToNot(HaveOccurred())

			loadedObserved := loaded.(TestObservedState)
			Expect(loadedObserved.DesiredConfig).To(Equal(observed.DesiredConfig))

			// Verify GetObservedDesiredState works
			observedDesired := loadedObserved.GetObservedDesiredState()
			Expect(observedDesired).ToNot(BeNil())
		})
	})

	Describe("Error Handling", func() {
		It("should return error for unregistered worker type", func() {
			_, err := adapter.LoadDesired(ctx, "unknown-type", workerID)
			Expect(err).To(HaveOccurred())
		})

		It("should return error for invalid document structure", func() {
			// Save a document directly via TriangularStore with invalid structure
			invalidDoc := basic.Document{
				"id":       workerID,
				"invalid":  make(chan int), // Channels can't be serialized
			}
			err := triangular.SaveDesired(ctx, workerType, workerID, invalidDoc)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Close", func() {
		It("should close without error", func() {
			err := adapter.Close()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

func TestTriangularStoreAdapter_TimestampPersistence(t *testing.T) {
	ctx := context.Background()
	tempDir, err := os.MkdirTemp("", "timestamp-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	cfg := basic.Config{
		DBPath:      filepath.Join(tempDir, "test.db"),
		JournalMode: basic.JournalModeDELETE,
	}
	basicStore, err := basic.NewStore(cfg)
	require.NoError(t, err)
	defer basicStore.Close(ctx)

	registry := storage.NewRegistry()

	err = registry.Register(&storage.CollectionMetadata{
		Name:       "container_identity",
		WorkerType: "container",
		Role:       storage.RoleIdentity,
		CSEFields:  []string{storage.FieldSyncID, storage.FieldCreatedAt, storage.FieldUpdatedAt},
	})
	require.NoError(t, err)

	err = registry.Register(&storage.CollectionMetadata{
		Name:       "container_desired",
		WorkerType: "container",
		Role:       storage.RoleDesired,
		CSEFields:  []string{storage.FieldSyncID, storage.FieldCreatedAt, storage.FieldUpdatedAt},
	})
	require.NoError(t, err)

	err = registry.Register(&storage.CollectionMetadata{
		Name:       "container_observed",
		WorkerType: "container",
		Role:       storage.RoleObserved,
		CSEFields:  []string{storage.FieldSyncID, storage.FieldCreatedAt, storage.FieldUpdatedAt},
	})
	require.NoError(t, err)

	err = basicStore.CreateCollection(ctx, "container_identity", nil)
	require.NoError(t, err)

	err = basicStore.CreateCollection(ctx, "container_desired", nil)
	require.NoError(t, err)

	err = basicStore.CreateCollection(ctx, "container_observed", nil)
	require.NoError(t, err)

	triangularStore := storage.NewTriangularStore(basicStore, registry)
	adapter := persistence.NewTriangularStoreAdapter(triangularStore)

	adapter.RegisterTypes(
		"container",
		reflect.TypeOf(&container.ContainerDesiredState{}),
		reflect.TypeOf(&container.ContainerObservedState{}),
		reflect.TypeOf(&fsmv2.Identity{}),
	)

	now := time.Now()
	observed := &container.ContainerObservedState{
		ID:             "test-container",
		CPUUsageMCores: 1500.0,
		CollectedAt:    now,
	}

	err = adapter.SaveObserved(ctx, "container", "test-container", observed)
	require.NoError(t, err)

	loaded, err := adapter.LoadObserved(ctx, "container", "test-container")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	loadedObserved := loaded.(*container.ContainerObservedState)

	assert.Equal(t, now.Unix(), loadedObserved.CollectedAt.Unix(),
		"CollectedAt timestamp should be preserved (second precision)")
	assert.False(t, loadedObserved.CollectedAt.IsZero(),
		"CollectedAt should not be zero time")
}
