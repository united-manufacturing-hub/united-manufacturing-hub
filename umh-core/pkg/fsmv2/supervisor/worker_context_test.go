// Copyright 2025 UMH Systems GmbH
package supervisor

import (
	"context"
	"testing"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/persistence"
	"go.uber.org/zap"
)

func TestWorkerContext(t *testing.T) {
	identity := fsmv2.Identity{
		ID:         "test-worker",
		Name:       "Test Worker",
		WorkerType: "container",
	}

	worker := &mockWorker{}
	state := &mockState{}
	collector := NewCollector(CollectorConfig{
		Worker:              worker,
		Identity:            identity,
		Store:               &mockStore{},
		Logger:              zap.NewNop().Sugar(),
		ObservationInterval: time.Second,
		ObservationTimeout:  time.Second,
	})

	ctx := WorkerContext{
		identity:     identity,
		worker:       worker,
		currentState: state,
		collector:    collector,
	}

	if ctx.identity != identity {
		t.Errorf("identity field not accessible or incorrect: got %v, want %v", ctx.identity, identity)
	}

	if ctx.worker != worker {
		t.Errorf("worker field not accessible or incorrect")
	}

	if ctx.currentState != state {
		t.Errorf("currentState field not accessible or incorrect")
	}

	if ctx.collector != collector {
		t.Errorf("collector field not accessible or incorrect")
	}
}

type mockWorker struct{}

func (m *mockWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	return &mockObservedState{timestamp: time.Now()}, nil
}

func (m *mockWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &mockDesiredState{}, nil
}

func (m *mockWorker) GetInitialState() fsmv2.State {
	return &mockState{}
}

type mockState struct{}

func (m *mockState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	return m, fsmv2.SignalNone, nil
}

func (m *mockState) String() string { return "MockState" }
func (m *mockState) Reason() string { return "mock state" }

type mockObservedState struct {
	timestamp time.Time
}

func (m *mockObservedState) GetTimestamp() time.Time { return m.timestamp }
func (m *mockObservedState) ShutdownRequested() bool { return false }
func (m *mockObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &mockDesiredState{}
}

type mockDesiredState struct{}

func (m *mockDesiredState) ShutdownRequested() bool { return false }

type mockStore struct{}

func (m *mockStore) SaveIdentity(ctx context.Context, workerType string, id string, data interface{}) error {
	return nil
}

func (m *mockStore) LoadIdentity(ctx context.Context, workerType string, id string) (interface{}, error) {
	return nil, nil
}

func (m *mockStore) SaveDesired(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
	return nil
}

func (m *mockStore) LoadDesired(ctx context.Context, workerType string, id string) (fsmv2.DesiredState, error) {
	return &mockDesiredState{}, nil
}

func (m *mockStore) SaveObserved(ctx context.Context, workerType string, id string, observed fsmv2.ObservedState) error {
	return nil
}

func (m *mockStore) LoadObserved(ctx context.Context, workerType string, id string) (fsmv2.ObservedState, error) {
	return &mockObservedState{timestamp: time.Now()}, nil
}

func (m *mockStore) LoadSnapshot(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
	return &fsmv2.Snapshot{
		Identity: fsmv2.Identity{
			ID:         id,
			Name:       "Test",
			WorkerType: workerType,
		},
		Desired:  &mockDesiredState{},
		Observed: &mockObservedState{timestamp: time.Now()},
	}, nil
}

func (m *mockStore) GetLastSyncID(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockStore) IncrementSyncID(ctx context.Context) (int64, error) {
	return 1, nil
}

func (m *mockStore) BeginTx(ctx context.Context) (persistence.Tx, error) {
	return &mockTx{store: m}, nil
}

func (m *mockStore) Close() error {
	return nil
}

type mockTx struct {
	store *mockStore
}

func (m *mockTx) SaveIdentity(ctx context.Context, workerType string, id string, data interface{}) error {
	return m.store.SaveIdentity(ctx, workerType, id, data)
}

func (m *mockTx) LoadIdentity(ctx context.Context, workerType string, id string) (interface{}, error) {
	return m.store.LoadIdentity(ctx, workerType, id)
}

func (m *mockTx) SaveDesired(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
	return m.store.SaveDesired(ctx, workerType, id, desired)
}

func (m *mockTx) LoadDesired(ctx context.Context, workerType string, id string) (fsmv2.DesiredState, error) {
	return m.store.LoadDesired(ctx, workerType, id)
}

func (m *mockTx) SaveObserved(ctx context.Context, workerType string, id string, observed fsmv2.ObservedState) error {
	return m.store.SaveObserved(ctx, workerType, id, observed)
}

func (m *mockTx) LoadObserved(ctx context.Context, workerType string, id string) (fsmv2.ObservedState, error) {
	return m.store.LoadObserved(ctx, workerType, id)
}

func (m *mockTx) LoadSnapshot(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
	return m.store.LoadSnapshot(ctx, workerType, id)
}

func (m *mockTx) GetLastSyncID(ctx context.Context) (int64, error) {
	return m.store.GetLastSyncID(ctx)
}

func (m *mockTx) IncrementSyncID(ctx context.Context) (int64, error) {
	return m.store.IncrementSyncID(ctx)
}

func (m *mockTx) BeginTx(ctx context.Context) (persistence.Tx, error) {
	return m, nil
}

func (m *mockTx) Close() error {
	return nil
}

func (m *mockTx) Commit() error {
	return nil
}

func (m *mockTx) Rollback() error {
	return nil
}
