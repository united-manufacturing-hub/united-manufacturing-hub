// Copyright 2025 UMH Systems GmbH
package supervisor

import (
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
	"go.uber.org/zap"
)

// TestObservedState is a mock ObservedState for testing subdirectories.
type TestObservedState struct {
	ID          string                `json:"id"`
	CollectedAt time.Time             `json:"collectedAt"`
	Desired     fsmv2.DesiredState    `json:"desired"`
}

func (t *TestObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return t.Desired
}

func (t *TestObservedState) GetTimestamp() time.Time {
	return t.CollectedAt
}

// TestDesiredState is a mock DesiredState for testing subdirectories.
type TestDesiredState struct {
	ShutdownReq bool
}

func (t *TestDesiredState) ShutdownRequested() bool {
	return t.ShutdownReq
}

func (t *TestDesiredState) SetShutdownRequested(requested bool) {
	t.ShutdownReq = requested
}

// TestWorker is a mock Worker for testing subdirectories.
type TestWorker struct {
	CollectErr   error
	Observed     fsmv2.ObservedState
	InitialState fsmv2.State
	CollectFunc  func(ctx context.Context) (fsmv2.ObservedState, error)
}

func (m *TestWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	if m.CollectFunc != nil {
		return m.CollectFunc(ctx)
	}

	if m.CollectErr != nil {
		return nil, m.CollectErr
	}

	if m.Observed != nil {
		return m.Observed, nil
	}

	return &TestObservedState{
		ID:          "test-worker",
		CollectedAt: time.Now(),
		Desired:     &TestDesiredState{},
	}, nil
}

func (m *TestWorker) DeriveDesiredState(spec interface{}) (types.DesiredState, error) {
	return types.DesiredState{State: "running"}, nil
}

func (m *TestWorker) GetInitialState() fsmv2.State {
	if m.InitialState != nil {
		return m.InitialState
	}

	return &TestState{}
}

// TestWorkerWithType extends TestWorker to support workerType-specific test data.
// This allows tests to create workers that return observed states appropriate for
// different workerTypes (e.g., "s6", "container", "benthos").
// It maintains full backward compatibility with TestWorker.
type TestWorkerWithType struct {
	TestWorker
	WorkerType string
}

// CollectObservedState returns workerType-specific observed state if no custom behavior is set.
// If TestWorker.CollectFunc is set, it delegates to the parent TestWorker behavior.
// This ensures backward compatibility while allowing workerType-specific test data.
func (m *TestWorkerWithType) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	if m.CollectFunc != nil {
		return m.CollectFunc(ctx)
	}

	if m.CollectErr != nil {
		return nil, m.CollectErr
	}

	if m.Observed != nil {
		return m.Observed, nil
	}

	return &TestObservedState{
		ID:          m.WorkerType + "-worker",
		CollectedAt: time.Now(),
		Desired:     &TestDesiredState{},
	}, nil
}

// TestState is a mock State for testing subdirectories.
type TestState struct {
	NextState fsmv2.State
	Signal    fsmv2.Signal
	Action    fsmv2.Action
}

func (m *TestState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	if m.NextState == nil {
		return m, fsmv2.SignalNone, nil
	}

	return m.NextState, m.Signal, m.Action
}

func (m *TestState) String() string { return "TestState" }
func (m *TestState) Reason() string { return "test state" }

// TestIdentity creates a test identity for subdirectory tests.
func TestIdentity() fsmv2.Identity {
	return fsmv2.Identity{
		ID:         "test-worker",
		Name:       "Test Worker",
		WorkerType: "container",
	}
}

// CreateTestTriangularStore creates a triangular store for subdirectory tests.
func CreateTestTriangularStore() *storage.TriangularStore {
	ctx := context.Background()
	basicStore := memory.NewInMemoryStore()

	registry := storage.NewRegistry()
	workerType := "container"

	if err := registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_identity",
		WorkerType:    workerType,
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	}); err != nil {
		panic(err)
	}
	if err := registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_desired",
		WorkerType:    workerType,
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	}); err != nil {
		panic(err)
	}
	if err := registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_observed",
		WorkerType:    workerType,
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	}); err != nil {
		panic(err)
	}

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

// CreateTestTriangularStoreForWorkerType creates a triangular store for a specific workerType.
// This allows tests to work with different workerTypes (e.g., "s6", "container", "benthos").
// The store is configured with three collections: {workerType}_identity, {workerType}_desired, {workerType}_observed.
// Each collection has appropriate CSEFields for its role:
//   - identity: FieldSyncID, FieldVersion, FieldCreatedAt (immutable)
//   - desired: FieldSyncID, FieldVersion, FieldCreatedAt, FieldUpdatedAt (version increments)
//   - observed: FieldSyncID, FieldVersion, FieldCreatedAt, FieldUpdatedAt (version doesn't increment)
func CreateTestTriangularStoreForWorkerType(workerType string) *storage.TriangularStore {
	ctx := context.Background()
	basicStore := memory.NewInMemoryStore()

	registry := storage.NewRegistry()

	if err := registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_identity",
		WorkerType:    workerType,
		Role:          storage.RoleIdentity,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	}); err != nil {
		panic(err)
	}
	if err := registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_desired",
		WorkerType:    workerType,
		Role:          storage.RoleDesired,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	}); err != nil {
		panic(err)
	}
	if err := registry.Register(&storage.CollectionMetadata{
		Name:          workerType + "_observed",
		WorkerType:    workerType,
		Role:          storage.RoleObserved,
		CSEFields:     []string{storage.FieldSyncID, storage.FieldVersion, storage.FieldCreatedAt, storage.FieldUpdatedAt},
		IndexedFields: []string{storage.FieldSyncID},
	}); err != nil {
		panic(err)
	}

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

// CreateTestObservedStateWithID creates a mock observed state with a specific ID.
func CreateTestObservedStateWithID(id string) *TestObservedState {
	return &TestObservedState{
		ID:          id,
		CollectedAt: time.Now(),
		Desired:     &TestDesiredState{},
	}
}

// CreateTestSupervisorWithCircuitState creates a test supervisor with a specific circuit breaker state.
// This is used for testing infrastructure health checking.
func CreateTestSupervisorWithCircuitState(circuitOpen bool) *Supervisor {
	logger := zap.NewNop().Sugar()
	s := NewSupervisor(Config{
		WorkerType:      "test",
		Store:           CreateTestTriangularStore(),
		Logger:          logger,
		CollectorHealth: CollectorHealthConfig{},
	})
	s.circuitOpen = circuitOpen
	return s
}
