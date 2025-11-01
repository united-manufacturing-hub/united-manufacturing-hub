// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

func TestSupervisor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Supervisor Suite")
}

type mockObservedState struct {
	ID          string                `json:"id"`
	CollectedAt time.Time             `json:"collectedAt"`
	Desired     fsmv2.DesiredState    `json:"-"`
}

func (m *mockObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return m.Desired
}

func (m *mockObservedState) GetTimestamp() time.Time {
	return m.CollectedAt
}

type mockDesiredState struct {
	shutdownRequested bool
}

func (m *mockDesiredState) ShutdownRequested() bool {
	return m.shutdownRequested
}

type mockWorker struct {
	collectErr     error
	observed       fsmv2.ObservedState
	initialState   fsmv2.State
	collectFunc    func(ctx context.Context) (fsmv2.ObservedState, error)
}

func (m *mockWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	if m.collectFunc != nil {
		return m.collectFunc(ctx)
	}

	if m.collectErr != nil {
		return nil, m.collectErr
	}

	if m.observed != nil {
		return m.observed, nil
	}

	return &mockObservedState{
		ID:          "test-worker",
		CollectedAt: time.Now(),
		Desired:     &mockDesiredState{},
	}, nil
}

func (m *mockWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &mockDesiredState{}, nil
}

func (m *mockWorker) GetInitialState() fsmv2.State {
	if m.initialState != nil {
		return m.initialState
	}

	return &mockState{}
}

type mockState struct {
	nextState fsmv2.State
	signal    fsmv2.Signal
	action    fsmv2.Action
}

func (m *mockState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	if m.nextState == nil {
		return m, fsmv2.SignalNone, nil
	}

	return m.nextState, m.signal, m.action
}

func (m *mockState) String() string { return "MockState" }
func (m *mockState) Reason() string { return "mock state" }

type mockStore struct {
	identity     map[string]map[string]persistence.Document // workerType -> id -> document
	desired      map[string]map[string]persistence.Document
	observed     map[string]map[string]persistence.Document
	saveErr      error
	loadSnapshot func(ctx context.Context, workerType string, id string) (*storage.Snapshot, error)
	saveDesired  func(ctx context.Context, workerType string, id string, desired persistence.Document) error
	saveObserved func(ctx context.Context, workerType string, id string, observed interface{}) error
}

func newMockStore() *mockStore {
	return &mockStore{
		identity: make(map[string]map[string]persistence.Document),
		desired:  make(map[string]map[string]persistence.Document),
		observed: make(map[string]map[string]persistence.Document),
	}
}

func (m *mockStore) SaveIdentity(ctx context.Context, workerType string, id string, identity persistence.Document) error {
	if m.identity[workerType] == nil {
		m.identity[workerType] = make(map[string]persistence.Document)
	}
	m.identity[workerType][id] = identity
	return m.saveErr
}

func (m *mockStore) LoadIdentity(ctx context.Context, workerType string, id string) (persistence.Document, error) {
	if m.identity[workerType] == nil || m.identity[workerType][id] == nil {
		return nil, persistence.ErrNotFound
	}
	return m.identity[workerType][id], nil
}

func (m *mockStore) SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) error {
	if m.saveDesired != nil {
		return m.saveDesired(ctx, workerType, id, desired)
	}

	if m.desired[workerType] == nil {
		m.desired[workerType] = make(map[string]persistence.Document)
	}
	m.desired[workerType][id] = desired
	return m.saveErr
}

func (m *mockStore) LoadDesired(ctx context.Context, workerType string, id string) (persistence.Document, error) {
	if m.desired[workerType] == nil || m.desired[workerType][id] == nil {
		return nil, persistence.ErrNotFound
	}
	return m.desired[workerType][id], nil
}

func (m *mockStore) SaveObserved(ctx context.Context, workerType string, id string, observed interface{}) error {
	if m.saveObserved != nil {
		return m.saveObserved(ctx, workerType, id, observed)
	}

	if m.observed[workerType] == nil {
		m.observed[workerType] = make(map[string]persistence.Document)
	}

	// Convert observed to persistence.Document if it isn't already
	var doc persistence.Document
	if observedDoc, ok := observed.(persistence.Document); ok {
		doc = observedDoc
	} else {
		// For non-Document types (like fsmv2.ObservedState), create a simple wrapper
		doc = persistence.Document{"data": observed, "collectedAt": time.Now()}
	}

	m.observed[workerType][id] = doc
	return m.saveErr
}

func (m *mockStore) LoadObserved(ctx context.Context, workerType string, id string) (persistence.Document, error) {
	if m.observed[workerType] == nil || m.observed[workerType][id] == nil {
		return nil, persistence.ErrNotFound
	}
	return m.observed[workerType][id], nil
}

func (m *mockStore) LoadSnapshot(ctx context.Context, workerType string, id string) (*storage.Snapshot, error) {
	if m.loadSnapshot != nil {
		return m.loadSnapshot(ctx, workerType, id)
	}

	// Return default snapshot with identity
	identity := persistence.Document{
		"id":         id,
		"name":       "Test Worker",
		"workerType": workerType,
	}
	desired := persistence.Document{}
	observed := persistence.Document{"collectedAt": time.Now()}

	// Load from stored data if available
	if m.identity[workerType] != nil && m.identity[workerType][id] != nil {
		identity = m.identity[workerType][id]
	}
	if m.desired[workerType] != nil && m.desired[workerType][id] != nil {
		desired = m.desired[workerType][id]
	}
	if m.observed[workerType] != nil && m.observed[workerType][id] != nil {
		observed = m.observed[workerType][id]
	}

	return &storage.Snapshot{
		Identity: identity,
		Desired:  desired,
		Observed: observed,
	}, nil
}

func (m *mockStore) DeleteWorker(ctx context.Context, workerType string, id string) error {
	if m.identity[workerType] != nil {
		delete(m.identity[workerType], id)
	}
	if m.desired[workerType] != nil {
		delete(m.desired[workerType], id)
	}
	if m.observed[workerType] != nil {
		delete(m.observed[workerType], id)
	}
	return m.saveErr
}

func mockIdentity() fsmv2.Identity {
	return fsmv2.Identity{
		ID:         "test-worker",
		Name:       "Test Worker",
		WorkerType: "container",
	}
}

func newSupervisorWithWorker(worker *mockWorker, cfg supervisor.CollectorHealthConfig) *supervisor.Supervisor {
	identity := mockIdentity()

	// Create a proper TriangularStore for testing
	ctx := context.Background()
	basicStore := memory.NewInMemoryStore()

	registry := storage.NewRegistry()
	workerType := "container"

	// Auto-register collections following the supervisor convention
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

	// Create collections in the basic store
	if err := basicStore.CreateCollection(ctx, workerType+"_identity", nil); err != nil {
		panic(fmt.Sprintf("failed to create identity collection: %v", err))
	}
	if err := basicStore.CreateCollection(ctx, workerType+"_desired", nil); err != nil {
		panic(fmt.Sprintf("failed to create desired collection: %v", err))
	}
	if err := basicStore.CreateCollection(ctx, workerType+"_observed", nil); err != nil {
		panic(fmt.Sprintf("failed to create observed collection: %v", err))
	}

	triangularStore := storage.NewTriangularStore(basicStore, registry)
	if triangularStore == nil {
		panic("triangular store is nil")
	}

	s := supervisor.NewSupervisor(supervisor.Config{
		WorkerType:      workerType,
		Logger:          zap.NewNop().Sugar(),
		CollectorHealth: cfg,
		Store:           triangularStore,
	})

	err := s.AddWorker(identity, worker)
	if err != nil {
		panic(err)
	}

	// Pre-populate desired state document so tests don't fail with "document not found"
	desiredDoc := persistence.Document{
		"id":               identity.ID,
		"shutdownRequested": false,
	}
	if err := triangularStore.SaveDesired(ctx, workerType, identity.ID, desiredDoc); err != nil {
		panic(fmt.Sprintf("failed to save initial desired state: %v", err))
	}

	return s
}

func createMockObservedStateWithID(id string) *mockObservedState {
	return &mockObservedState{
		ID:          id,
		CollectedAt: time.Now(),
		Desired:     &mockDesiredState{},
	}
}

func createTestTriangularStore() *storage.TriangularStore {
	ctx := context.Background()
	basicStore := memory.NewInMemoryStore()

	registry := storage.NewRegistry()
	workerType := "container"

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

	if err := basicStore.CreateCollection(ctx, workerType+"_identity", nil); err != nil {
		panic(fmt.Sprintf("failed to create identity collection: %v", err))
	}
	if err := basicStore.CreateCollection(ctx, workerType+"_desired", nil); err != nil {
		panic(fmt.Sprintf("failed to create desired collection: %v", err))
	}
	if err := basicStore.CreateCollection(ctx, workerType+"_observed", nil); err != nil {
		panic(fmt.Sprintf("failed to create observed collection: %v", err))
	}

	return storage.NewTriangularStore(basicStore, registry)
}
