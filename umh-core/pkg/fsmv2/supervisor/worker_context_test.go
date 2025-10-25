// Copyright 2025 UMH Systems GmbH
package supervisor

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/persistence"
	"go.uber.org/zap"
)

var _ = Describe("WorkerContext", func() {
	It("should correctly store all fields", func() {
		identity := fsmv2.Identity{
			ID:         "test-worker",
			Name:       "Test Worker",
			WorkerType: "container",
		}

		worker := &testWorker{initialState: &testState{}}
		state := &testState{}
		collector := NewCollector(CollectorConfig{
			Worker:              worker,
			Identity:            identity,
			Store:               &testStore{},
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

		Expect(ctx.identity).To(Equal(identity))
		Expect(ctx.worker).To(Equal(worker))
		Expect(ctx.currentState).To(Equal(state))
		Expect(ctx.collector).To(Equal(collector))
	})
})

type testWorker struct {
	initialState fsmv2.State
}

func (m *testWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	return &testObservedState{timestamp: time.Now()}, nil
}

func (m *testWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &testDesiredState{}, nil
}

func (m *testWorker) GetInitialState() fsmv2.State {
	if m.initialState != nil {
		return m.initialState
	}

	return &testState{}
}

type testState struct{}

func (m *testState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	return m, fsmv2.SignalNone, nil
}

func (m *testState) String() string { return "TestState" }
func (m *testState) Reason() string { return "test state" }

type testObservedState struct {
	timestamp time.Time
}

func (m *testObservedState) GetTimestamp() time.Time                         { return m.timestamp }
func (m *testObservedState) ShutdownRequested() bool                         { return false }
func (m *testObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &testDesiredState{}
}

type testDesiredState struct{}

func (m *testDesiredState) ShutdownRequested() bool { return false }

type testStore struct{}

func (m *testStore) SaveIdentity(ctx context.Context, workerType string, id string, data interface{}) error {
	return nil
}

func (m *testStore) LoadIdentity(ctx context.Context, workerType string, id string) (interface{}, error) {
	return nil, nil
}

func (m *testStore) SaveDesired(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
	return nil
}

func (m *testStore) LoadDesired(ctx context.Context, workerType string, id string) (fsmv2.DesiredState, error) {
	return &testDesiredState{}, nil
}

func (m *testStore) SaveObserved(ctx context.Context, workerType string, id string, observed fsmv2.ObservedState) error {
	return nil
}

func (m *testStore) LoadObserved(ctx context.Context, workerType string, id string) (fsmv2.ObservedState, error) {
	return &testObservedState{timestamp: time.Now()}, nil
}

func (m *testStore) LoadSnapshot(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
	return &fsmv2.Snapshot{
		Identity: fsmv2.Identity{
			ID:         id,
			Name:       "Test",
			WorkerType: workerType,
		},
		Desired:  &testDesiredState{},
		Observed: &testObservedState{timestamp: time.Now()},
	}, nil
}

func (m *testStore) GetLastSyncID(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *testStore) IncrementSyncID(ctx context.Context) (int64, error) {
	return 1, nil
}

func (m *testStore) BeginTx(ctx context.Context) (persistence.Tx, error) {
	return &testTx{store: m}, nil
}

func (m *testStore) Close() error {
	return nil
}

type testTx struct {
	store *testStore
}

func (m *testTx) SaveIdentity(ctx context.Context, workerType string, id string, data interface{}) error {
	return m.store.SaveIdentity(ctx, workerType, id, data)
}

func (m *testTx) LoadIdentity(ctx context.Context, workerType string, id string) (interface{}, error) {
	return m.store.LoadIdentity(ctx, workerType, id)
}

func (m *testTx) SaveDesired(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error {
	return m.store.SaveDesired(ctx, workerType, id, desired)
}

func (m *testTx) LoadDesired(ctx context.Context, workerType string, id string) (fsmv2.DesiredState, error) {
	return m.store.LoadDesired(ctx, workerType, id)
}

func (m *testTx) SaveObserved(ctx context.Context, workerType string, id string, observed fsmv2.ObservedState) error {
	return m.store.SaveObserved(ctx, workerType, id, observed)
}

func (m *testTx) LoadObserved(ctx context.Context, workerType string, id string) (fsmv2.ObservedState, error) {
	return m.store.LoadObserved(ctx, workerType, id)
}

func (m *testTx) LoadSnapshot(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
	return m.store.LoadSnapshot(ctx, workerType, id)
}

func (m *testTx) GetLastSyncID(ctx context.Context) (int64, error) {
	return m.store.GetLastSyncID(ctx)
}

func (m *testTx) IncrementSyncID(ctx context.Context) (int64, error) {
	return m.store.IncrementSyncID(ctx)
}

func (m *testTx) BeginTx(ctx context.Context) (persistence.Tx, error) {
	return m, nil
}

func (m *testTx) Close() error {
	return nil
}

func (m *testTx) Commit() error {
	return nil
}

func (m *testTx) Rollback() error {
	return nil
}
