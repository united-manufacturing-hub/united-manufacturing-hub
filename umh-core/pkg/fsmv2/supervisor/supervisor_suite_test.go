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

package supervisor_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

func TestSupervisor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Supervisor Suite")
}

var _ = BeforeEach(func() {
	registerTestWorkerFactories()
})

var _ = AfterEach(func() {
	factory.ResetRegistry()
})

func registerTestWorkerFactories() {
	workerTypes := []string{
		"child",
		"parent",
		"mqtt_client",
		"mqtt_broker",
		"opcua_client",
		"opcua_server",
		"s7comm_client",
		"modbus_client",
		"http_client",
		"child1",
		"child2",
		"grandchild",
		"valid_child",
		"another_child",
		"working-child",
		"failing-child",
		"working",
		"failing",
		"container",
		"s6_service",
		"type_a",
		"type_b",
		"test", // Used by handleWorkerRestart tests for full worker recreation
	}

	for _, workerType := range workerTypes {
		wt := workerType
		// Register worker factory
		err := factory.RegisterFactoryByType(wt, func(identity fsmv2.Identity, _ *zap.SugaredLogger, _ fsmv2.StateReader) fsmv2.Worker {
			return &supervisor.TestWorkerWithType{
				Worker:     supervisor.TestWorker{},
				WorkerType: wt,
			}
		})
		if err != nil {
			panic(fmt.Sprintf("failed to register test worker factory for %s: %v", wt, err))
		}

		// Register supervisor factory for hierarchical composition
		err = factory.RegisterSupervisorFactoryByType(wt, func(cfg interface{}) interface{} {
			supervisorCfg := cfg.(supervisor.Config)

			return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisorCfg)
		})
		if err != nil {
			panic(fmt.Sprintf("failed to register test supervisor factory for %s: %v", wt, err))
		}
	}
}

type mockObservedState struct {
	ID          string             `json:"id"`
	CollectedAt time.Time          `json:"collectedAt"`
	Desired     fsmv2.DesiredState `json:"-"`
}

func (m *mockObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return m.Desired
}

func (m *mockObservedState) GetTimestamp() time.Time {
	return m.CollectedAt
}

type mockDesiredState struct {
	ShutdownRequested bool
}

func (m *mockDesiredState) IsShutdownRequested() bool {
	return m.ShutdownRequested
}

type mockWorker struct {
	collectErr          error
	observed            fsmv2.ObservedState
	initialState        fsmv2.State[any, any]
	collectFunc         func(ctx context.Context) (fsmv2.ObservedState, error)
	requestShutdownFunc func() // Callback for when RequestShutdown is called
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

func (m *mockWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	return config.DesiredState{State: "running"}, nil
}

func (m *mockWorker) GetInitialState() fsmv2.State[any, any] {
	if m.initialState != nil {
		return m.initialState
	}

	return &mockState{}
}

type mockState struct {
	nextState fsmv2.State[any, any]
	signal    fsmv2.Signal
	action    fsmv2.Action[any]
}

func (m *mockState) Next(snapshot any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
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
	saveObserved func(ctx context.Context, workerType string, id string, observed interface{}) (bool, error)
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

func (m *mockStore) SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) (bool, error) {
	if m.saveDesired != nil {
		return true, m.saveDesired(ctx, workerType, id, desired)
	}

	if m.desired[workerType] == nil {
		m.desired[workerType] = make(map[string]persistence.Document)
	}

	m.desired[workerType][id] = desired

	return true, m.saveErr
}

func (m *mockStore) LoadDesired(ctx context.Context, workerType string, id string) (interface{}, error) {
	if m.desired[workerType] == nil || m.desired[workerType][id] == nil {
		return nil, persistence.ErrNotFound
	}

	return m.desired[workerType][id], nil
}

func (m *mockStore) LoadDesiredTyped(ctx context.Context, workerType string, id string, dest interface{}) error {
	result, err := m.LoadDesired(ctx, workerType, id)
	if err != nil {
		return err
	}

	doc, ok := result.(persistence.Document)
	if !ok {
		return fmt.Errorf("expected Document, got %T", result)
	}

	jsonBytes, _ := json.Marshal(doc)

	return json.Unmarshal(jsonBytes, dest)
}

func (m *mockStore) SaveObserved(ctx context.Context, workerType string, id string, observed interface{}) (bool, error) {
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

	if m.saveErr != nil {
		return false, m.saveErr
	}

	return true, nil
}

func (m *mockStore) LoadObserved(ctx context.Context, workerType string, id string) (interface{}, error) {
	if m.observed[workerType] == nil || m.observed[workerType][id] == nil {
		return nil, persistence.ErrNotFound
	}

	return m.observed[workerType][id], nil
}

func (m *mockStore) LoadObservedTyped(ctx context.Context, workerType string, id string, dest interface{}) error {
	result, err := m.LoadObserved(ctx, workerType, id)
	if err != nil {
		return err
	}

	doc, ok := result.(persistence.Document)
	if !ok {
		return fmt.Errorf("expected Document, got %T", result)
	}

	jsonBytes, _ := json.Marshal(doc)

	return json.Unmarshal(jsonBytes, dest)
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

func (m *mockStore) GetChangesSince(ctx context.Context, sinceSyncID int64, limit int) ([]storage.Event, error) {
	return []storage.Event{}, nil
}

func (m *mockStore) GetLatestSyncID(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockStore) GetDeltas(ctx context.Context, sub storage.Subscription) (storage.DeltasResponse, error) {
	return storage.DeltasResponse{}, nil
}

func mockIdentity() fsmv2.Identity {
	return fsmv2.Identity{
		ID:         "test-worker",
		Name:       "Test Worker",
		WorkerType: "test",
	}
}

func newSupervisorWithWorker(worker *mockWorker, customStore storage.TriangularStoreInterface, cfg supervisor.CollectorHealthConfig) *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState] {
	identity := mockIdentity()
	ctx := context.Background()
	workerType := "test"

	var triangularStore storage.TriangularStoreInterface
	if customStore != nil {
		triangularStore = customStore
	} else {
		basicStore := memory.NewInMemoryStore()

		if err := basicStore.CreateCollection(ctx, workerType+"_identity", nil); err != nil {
			panic(fmt.Sprintf("failed to create identity collection: %v", err))
		}

		if err := basicStore.CreateCollection(ctx, workerType+"_desired", nil); err != nil {
			panic(fmt.Sprintf("failed to create desired collection: %v", err))
		}

		if err := basicStore.CreateCollection(ctx, workerType+"_observed", nil); err != nil {
			panic(fmt.Sprintf("failed to create observed collection: %v", err))
		}

		triangularStore = storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())
	}

	s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
		WorkerType:              workerType,
		Logger:                  zap.NewNop().Sugar(),
		CollectorHealth:         cfg,
		Store:                   triangularStore,
		GracefulShutdownTimeout: 100 * time.Millisecond, // Short timeout for tests
	})

	err := s.AddWorker(identity, worker)
	if err != nil {
		panic(err)
	}

	desiredDoc := persistence.Document{
		"id":                identity.ID,
		"ShutdownRequested": false,
	}
	if _, err := triangularStore.SaveDesired(ctx, workerType, identity.ID, desiredDoc); err != nil {
		panic(fmt.Sprintf("failed to save initial desired state: %v", err))
	}

	// Save initial observed state so TestTick can load snapshot
	// Use persistence.Document to avoid JSON unmarshal errors with interface fields
	observedDoc := persistence.Document{
		"id":          identity.ID,
		"collectedAt": time.Now(),
		"desired": persistence.Document{
			"ShutdownReq": false,
		},
	}
	if _, err := triangularStore.SaveObserved(ctx, workerType, identity.ID, observedDoc); err != nil {
		panic(fmt.Sprintf("failed to save initial observed state: %v", err))
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

	// Create collections for common worker types used in tests
	workerTypes := []string{"test", "container"}
	for _, workerType := range workerTypes {
		if err := basicStore.CreateCollection(ctx, workerType+"_identity", nil); err != nil {
			panic(fmt.Sprintf("failed to create identity collection for %s: %v", workerType, err))
		}

		if err := basicStore.CreateCollection(ctx, workerType+"_desired", nil); err != nil {
			panic(fmt.Sprintf("failed to create desired collection for %s: %v", workerType, err))
		}

		if err := basicStore.CreateCollection(ctx, workerType+"_observed", nil); err != nil {
			panic(fmt.Sprintf("failed to create observed collection for %s: %v", workerType, err))
		}
	}

	return storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())
}

type mockTriangularStore struct {
	mu sync.RWMutex

	SaveIdentityErr error
	LoadIdentityErr error
	SaveDesiredErr  error
	LoadDesiredErr  error
	SaveObservedErr error
	LoadObservedErr error
	LoadSnapshotErr error

	identity map[string]map[string]persistence.Document
	desired  map[string]map[string]persistence.Document
	Observed map[string]map[string]interface{}

	SaveDesiredCalled  int
	LoadDesiredCalled  int
	SaveObservedCalled int
	LoadObservedCalled int
}

func newMockTriangularStore() *mockTriangularStore {
	return &mockTriangularStore{
		identity: make(map[string]map[string]persistence.Document),
		desired:  make(map[string]map[string]persistence.Document),
		Observed: make(map[string]map[string]interface{}),
	}
}

func (m *mockTriangularStore) SaveIdentity(ctx context.Context, workerType string, id string, identity persistence.Document) error {
	if m.SaveIdentityErr != nil {
		return m.SaveIdentityErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.identity[workerType] == nil {
		m.identity[workerType] = make(map[string]persistence.Document)
	}

	m.identity[workerType][id] = identity

	return nil
}

func (m *mockTriangularStore) LoadIdentity(ctx context.Context, workerType string, id string) (persistence.Document, error) {
	if m.LoadIdentityErr != nil {
		return nil, m.LoadIdentityErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.identity[workerType] == nil {
		return nil, persistence.ErrNotFound
	}

	doc, ok := m.identity[workerType][id]
	if !ok {
		return nil, persistence.ErrNotFound
	}

	return doc, nil
}

func (m *mockTriangularStore) SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) (bool, error) {
	if m.SaveDesiredErr != nil {
		return false, m.SaveDesiredErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.SaveDesiredCalled++

	if m.desired[workerType] == nil {
		m.desired[workerType] = make(map[string]persistence.Document)
	}

	m.desired[workerType][id] = desired

	return true, nil
}

func (m *mockTriangularStore) LoadDesired(ctx context.Context, workerType string, id string) (interface{}, error) {
	if m.LoadDesiredErr != nil {
		return nil, m.LoadDesiredErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.LoadDesiredCalled++

	if m.desired[workerType] == nil {
		return nil, persistence.ErrNotFound
	}

	doc, ok := m.desired[workerType][id]
	if !ok {
		return nil, persistence.ErrNotFound
	}

	return doc, nil
}

func (m *mockTriangularStore) LoadDesiredTyped(ctx context.Context, workerType string, id string, dest interface{}) error {
	result, err := m.LoadDesired(ctx, workerType, id)
	if err != nil {
		return err
	}

	doc, ok := result.(persistence.Document)
	if !ok {
		return fmt.Errorf("expected Document, got %T", result)
	}

	jsonBytes, _ := json.Marshal(doc)

	return json.Unmarshal(jsonBytes, dest)
}

func (m *mockTriangularStore) SaveObserved(ctx context.Context, workerType string, id string, observed interface{}) (bool, error) {
	if m.SaveObservedErr != nil {
		return false, m.SaveObservedErr
	}

	// Do JSON marshaling before acquiring lock to minimize hold time
	var doc persistence.Document
	if observedDoc, ok := observed.(persistence.Document); ok {
		doc = observedDoc
	} else {
		jsonBytes, err := json.Marshal(observed)
		if err != nil {
			return false, err
		}

		if err := json.Unmarshal(jsonBytes, &doc); err != nil {
			return false, err
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.SaveObservedCalled++

	if m.Observed[workerType] == nil {
		m.Observed[workerType] = make(map[string]interface{})
	}

	m.Observed[workerType][id] = doc

	return true, nil
}

func (m *mockTriangularStore) LoadObserved(ctx context.Context, workerType string, id string) (interface{}, error) {
	if m.LoadObservedErr != nil {
		return nil, m.LoadObservedErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.LoadObservedCalled++

	if m.Observed[workerType] == nil {
		return nil, persistence.ErrNotFound
	}

	val, ok := m.Observed[workerType][id]
	if !ok {
		return nil, persistence.ErrNotFound
	}

	doc, ok := val.(persistence.Document)
	if !ok {
		// I16: Type mismatch detected - this is a programming error
		// The stored type doesn't match the expected Document type
		resultType := fmt.Sprintf("%T", val)
		if len(resultType) > 1 && resultType[0] == '*' {
			resultType = resultType[1:]
		}

		panic(fmt.Sprintf("Invariant I16 violated: worker %s expected type *supervisor.TestObservedState but got %s", id, resultType))
	}

	return doc, nil
}

func (m *mockTriangularStore) LoadObservedTyped(ctx context.Context, workerType string, id string, dest interface{}) error {
	result, err := m.LoadObserved(ctx, workerType, id)
	if err != nil {
		return err
	}

	doc, ok := result.(persistence.Document)
	if !ok {
		// I16: Type mismatch detected - this is a programming error
		// The stored type doesn't match the expected Document type
		// Get the expected type name from dest
		destType := fmt.Sprintf("%T", dest)
		// Remove pointer prefix for cleaner display
		if len(destType) > 1 && destType[0] == '*' {
			destType = destType[1:]
		}

		resultType := fmt.Sprintf("%T", result)
		if len(resultType) > 1 && resultType[0] == '*' {
			resultType = resultType[1:]
		}

		panic(fmt.Sprintf("Invariant I16 violated: worker %s expected type %s but got %s", id, destType, resultType))
	}

	jsonBytes, _ := json.Marshal(doc)

	return json.Unmarshal(jsonBytes, dest)
}

func (m *mockTriangularStore) LoadSnapshot(ctx context.Context, workerType string, id string) (*storage.Snapshot, error) {
	if m.LoadSnapshotErr != nil {
		return nil, m.LoadSnapshotErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := &storage.Snapshot{}
	observedFound := false

	if idMap, ok := m.identity[workerType]; ok {
		snapshot.Identity = idMap[id]
	}

	if desMap, ok := m.desired[workerType]; ok {
		snapshot.Desired = desMap[id]
	}

	if obsMap, ok := m.Observed[workerType]; ok {
		if observedDoc, ok := obsMap[id]; ok {
			snapshot.Observed = observedDoc
			observedFound = true
		}
	}

	// If observed state was not found in the map at all (not explicitly set to nil),
	// return an empty document to prevent invariant violation.
	// This happens when auto-created child workers haven't had their observed state saved yet.
	// Tests that explicitly set observed to nil will still trigger the panic.
	if !observedFound {
		snapshot.Observed = persistence.Document{
			"id":          id,
			"collectedAt": time.Now(),
		}
	}

	return snapshot, nil
}

func (m *mockTriangularStore) GetChangesSince(ctx context.Context, sinceSyncID int64, limit int) ([]storage.Event, error) {
	return []storage.Event{}, nil
}

func (m *mockTriangularStore) GetLatestSyncID(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockTriangularStore) GetDeltas(ctx context.Context, sub storage.Subscription) (storage.DeltasResponse, error) {
	return storage.DeltasResponse{}, nil
}

var _ storage.TriangularStoreInterface = (*mockTriangularStore)(nil)
