// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"testing"
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

	return &container.ContainerObservedState{
		CollectedAt: time.Now(),
	}, nil
}

func (m *mockWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &container.ContainerDesiredState{}, nil
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
	snapshot     *fsmv2.Snapshot
	saveErr      error
	loadSnapshot func(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error)
	loadDesired  func(ctx context.Context, workerType string, id string) (fsmv2.DesiredState, error)
	saveDesired  func(ctx context.Context, workerType string, id string, desired fsmv2.DesiredState) error
	saveObserved func(ctx context.Context, workerType string, id string, observed fsmv2.ObservedState) error
}

func (m *mockStore) SaveIdentity(ctx context.Context, workerType string, id string, identity interface{}) error {
	return nil
}

func (m *mockStore) LoadIdentity(ctx context.Context, workerType string, id string) (interface{}, error) {
	return basic.Document{}, nil
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

	return &container.ContainerDesiredState{}, nil
}

func (m *mockStore) SaveObserved(ctx context.Context, workerType string, id string, observed fsmv2.ObservedState) error {
	if m.saveObserved != nil {
		return m.saveObserved(ctx, workerType, id, observed)
	}

	return m.saveErr
}

func (m *mockStore) LoadObserved(ctx context.Context, workerType string, id string) (fsmv2.ObservedState, error) {
	if m.snapshot != nil && m.snapshot.Observed != nil {
		return m.snapshot.Observed, nil
	}

	return &container.ContainerObservedState{CollectedAt: time.Now()}, nil
}

func (m *mockStore) LoadSnapshot(ctx context.Context, workerType string, id string) (*fsmv2.Snapshot, error) {
	if m.loadSnapshot != nil {
		return m.loadSnapshot(ctx, workerType, id)
	}

	identity := &container.ContainerIdentity{
		ID:   "test-worker",
		Name: "Test Worker",
	}

	desired := &container.ContainerDesiredState{}
	observed := &container.ContainerObservedState{CollectedAt: time.Now()}

	return &fsmv2.Snapshot{
		Identity: identity,
		Desired:  desired,
		Observed: observed,
	}, nil
}

func (m *mockStore) GetLastSyncID(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockStore) IncrementSyncID(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockStore) Close() error {
	return nil
}

func mockIdentity() fsmv2.Identity {
	return fsmv2.Identity{
		ID:         "test-worker",
		Name:       "Test Worker",
		WorkerType: "container",
	}
}

func newSupervisorWithWorker(worker *mockWorker, store *mockStore, cfg supervisor.CollectorHealthConfig) *supervisor.Supervisor {
	identity := mockIdentity()

	s := supervisor.NewSupervisor(supervisor.Config{
		WorkerType:      "container",
		Store:           store,
		Logger:          zap.NewNop().Sugar(),
		CollectorHealth: cfg,
	})

	err := s.AddWorker(identity, worker)
	if err != nil {
		panic(err)
	}

	return s
}

var _ = Describe("Identity WorkerType Field", func() {
	It("should have WorkerType field that can be accessed and set", func() {
		identity := fsmv2.Identity{
			ID:         "test-id",
			Name:       "test-name",
			WorkerType: "container",
		}

		Expect(identity.WorkerType).To(Equal("container"))

		identity.WorkerType = "pod"
		Expect(identity.WorkerType).To(Equal("pod"))
	})
})

