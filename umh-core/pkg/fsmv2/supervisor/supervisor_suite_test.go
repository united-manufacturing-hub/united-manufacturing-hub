// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/persistence"
)

func TestSupervisor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Supervisor Suite")
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

	return &mockObservedState{timestamp: time.Now()}, nil
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

type mockObservedState struct {
	timestamp time.Time
	shutdown  bool
}

func (m *mockObservedState) GetTimestamp() time.Time              { return m.timestamp }
func (m *mockObservedState) ShutdownRequested() bool              { return m.shutdown }
func (m *mockObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &mockDesiredState{}
}

type mockDesiredState struct {
	shutdown bool
}

func (m *mockDesiredState) ShutdownRequested() bool { return m.shutdown }

type mockStore struct {
	snapshot     *fsmv2.Snapshot
	saveErr      error
	loadSnapshot func(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error)
	loadDesired  func(ctx context.Context, workerType string, id string) (fsmv2.DesiredState, error)
	saveDesired  func(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error
}

func (m *mockStore) SaveIdentity(ctx context.Context, workerType string, id string, data interface{}) error {
	return nil
}

func (m *mockStore) LoadIdentity(ctx context.Context, workerType string, id string) (interface{}, error) {
	return nil, nil
}

func (m *mockStore) SaveDesired(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
	if m.saveDesired != nil {
		return m.saveDesired(ctx, workerType, id, desired)
	}

	return m.saveErr
}

func (m *mockStore) LoadDesired(ctx context.Context, workerType string, id string) (fsmv2.DesiredState, error) {
	if m.loadDesired != nil {
		return m.loadDesired(ctx, workerType, id)
	}

	return &mockDesiredState{}, nil
}

func (m *mockStore) SaveObserved(ctx context.Context, workerType string, id string, observed fsmv2.ObservedState) error {
	return m.saveErr
}

func (m *mockStore) LoadObserved(ctx context.Context, workerType string, id string) (fsmv2.ObservedState, error) {
	if m.snapshot != nil && m.snapshot.Observed != nil {
		return m.snapshot.Observed, nil
	}

	return &mockObservedState{timestamp: time.Now()}, nil
}

func (m *mockStore) LoadSnapshot(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
	if m.loadSnapshot != nil {
		return m.loadSnapshot(ctx, workerType, id)
	}

	if m.snapshot != nil {
		return m.snapshot, nil
	}

	return &fsmv2.Snapshot{
		Identity: mockIdentity(),
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

func mockIdentity() fsmv2.Identity {
	return fsmv2.Identity{
		ID:   "test-worker",
		Name: "Test Worker",
	}
}
