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

package supervisor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// TestSupervisorUsesTriangularStore verifies that Supervisor uses TriangularStore instead of basic Store.
// This test ensures the supervisor properly integrates with the triangular persistence model.
func TestSupervisorUsesTriangularStore(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	// Create in-memory store for testing
	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_identity",
		WorkerType:    "test",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_desired",
		WorkerType:    "test",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_observed",
		WorkerType:    "test",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	// Create collections in database
	var err error
	err = basicStore.CreateCollection(ctx, "test_identity", nil)
	if err != nil {
		t.Fatalf("Failed to create identity collection: %v", err)
	}
	err = basicStore.CreateCollection(ctx, "test_desired", nil)
	if err != nil {
		t.Fatalf("Failed to create desired collection: %v", err)
	}
	err = basicStore.CreateCollection(ctx, "test_observed", nil)
	if err != nil {
		t.Fatalf("Failed to create observed collection: %v", err)
	}

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

	if supervisor == nil {
		t.Fatal("Expected supervisor to be created")
	}
}

// TestSupervisorSavesIdentityToTriangularStore verifies that Supervisor uses TriangularStore.SaveIdentity
// when adding a worker.
func TestSupervisorSavesIdentityToTriangularStore(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	// Create in-memory store for testing
	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_identity",
		WorkerType:    "test",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_desired",
		WorkerType:    "test",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_observed",
		WorkerType:    "test",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	// Create collections in database
	var err error
	err = basicStore.CreateCollection(ctx, "test_identity", nil)
	if err != nil {
		t.Fatalf("Failed to create identity collection: %v", err)
	}
	err = basicStore.CreateCollection(ctx, "test_desired", nil)
	if err != nil {
		t.Fatalf("Failed to create desired collection: %v", err)
	}
	err = basicStore.CreateCollection(ctx, "test_observed", nil)
	if err != nil {
		t.Fatalf("Failed to create observed collection: %v", err)
	}

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "worker-1",
		Name:       "Test Worker",
		WorkerType: "test",
	}

	worker := &mockWorker{
		identity:     identity,
		initialState: &mockState{name: "initial"},
		observed: persistence.Document{
			"id":     "worker-1",
			"status": "running",
		},
	}

	err = supervisor.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add worker: %v", err)
	}

	loadedIdentity, err := triangularStore.LoadIdentity(ctx, "test", "worker-1")
	if err != nil {
		t.Fatalf("Failed to load identity: %v", err)
	}

	if loadedIdentity["id"] != "worker-1" {
		t.Errorf("Expected identity id 'worker-1', got %v", loadedIdentity["id"])
	}
	if loadedIdentity["name"] != "Test Worker" {
		t.Errorf("Expected identity name 'Test Worker', got %v", loadedIdentity["name"])
	}
}

// TestSupervisorLoadsSnapshotFromTriangularStore verifies that Supervisor uses TriangularStore.LoadSnapshot
// during tick operations.
func TestSupervisorLoadsSnapshotFromTriangularStore(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop().Sugar()

	// Create in-memory store for testing
	basicStore := memory.NewInMemoryStore()
	defer func() {
		_ = basicStore.Close(ctx)
	}()

	registry := storage.NewRegistry()
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_identity",
		WorkerType:    "test",
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_desired",
		WorkerType:    "test",
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})
	_ = registry.Register(&storage.CollectionMetadata{
		Name:          "test_observed",
		WorkerType:    "test",
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	})

	// Create collections in database
	var err error
	err = basicStore.CreateCollection(ctx, "test_identity", nil)
	if err != nil {
		t.Fatalf("Failed to create identity collection: %v", err)
	}
	err = basicStore.CreateCollection(ctx, "test_desired", nil)
	if err != nil {
		t.Fatalf("Failed to create desired collection: %v", err)
	}
	err = basicStore.CreateCollection(ctx, "test_observed", nil)
	if err != nil {
		t.Fatalf("Failed to create observed collection: %v", err)
	}

	triangularStore := storage.NewTriangularStore(basicStore, registry)

	supervisorCfg := Config{
		WorkerType: "test",
		Store:      triangularStore,
		Logger:     logger,
	}

	supervisor := NewSupervisor(supervisorCfg)

	identity := fsmv2.Identity{
		ID:         "worker-1",
		Name:       "Test Worker",
		WorkerType: "test",
	}

	worker := &mockWorker{
		identity:     identity,
		initialState: &mockState{name: "initial"},
		observed: persistence.Document{
			"id":     "worker-1",
			"status": "running",
		},
	}

	err = supervisor.AddWorker(identity, worker)
	if err != nil {
		t.Fatalf("Failed to add worker: %v", err)
	}

	err = triangularStore.SaveDesired(ctx, "test", "worker-1", persistence.Document{
		"id":     "worker-1",
		"config": "production",
	})
	if err != nil {
		t.Fatalf("Failed to save desired: %v", err)
	}

	err = supervisor.Tick(ctx)
	if err != nil {
		t.Fatalf("Failed to tick: %v", err)
	}
}

type mockWorker struct {
	identity     fsmv2.Identity
	initialState fsmv2.State
	observed     persistence.Document
}

func (m *mockWorker) CollectObservedState(_ context.Context) (fsmv2.ObservedState, error) {
	return &mockObservedState{
		doc:       m.observed,
		timestamp: time.Now(),
	}, nil
}

func (m *mockWorker) DeriveDesiredState(_ interface{}) (types.DesiredState, error) {
	return types.DesiredState{State: "running"}, nil
}

func (m *mockWorker) GetInitialState() fsmv2.State {
	return m.initialState
}

type mockObservedState struct {
	doc       persistence.Document
	timestamp time.Time
}

func (m *mockObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &mockDesiredState{}
}

func (m *mockObservedState) GetTimestamp() time.Time {
	return m.timestamp
}

// MarshalJSON ensures mockObservedState serializes to its document content
func (m *mockObservedState) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.doc)
}

type mockDesiredState struct{}

func (m *mockDesiredState) ShutdownRequested() bool {
	return false
}

func (m *mockDesiredState) SetShutdownRequested(_ bool) {
}

type mockState struct {
	name   string
	reason string
}

func (m *mockState) String() string {
	return m.name
}

func (m *mockState) Reason() string {
	return m.reason
}

func (m *mockState) Next(_ fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	return m, fsmv2.SignalNone, nil
}
