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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

func TestSupervisor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Supervisor Suite")
}

type mockObservedState struct {
	collectedAt time.Time
	desired     fsmv2.DesiredState
}

func (m *mockObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return m.desired
}

func (m *mockObservedState) GetTimestamp() time.Time {
	return m.collectedAt
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
		collectedAt: time.Now(),
		desired:     &mockDesiredState{},
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

	s := supervisor.NewSupervisor(supervisor.Config{
		WorkerType:      "container",
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

