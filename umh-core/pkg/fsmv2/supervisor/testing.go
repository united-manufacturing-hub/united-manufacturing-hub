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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
	"go.uber.org/zap"
)

// TestObservedState is a mock ObservedState for testing subdirectories.
type TestObservedState struct {
	ID          string             `json:"id"`
	CollectedAt time.Time          `json:"collectedAt"`
	Desired     fsmv2.DesiredState `json:"-"`
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

func (t *TestDesiredState) IsShutdownRequested() bool {
	return t.ShutdownReq
}

func (t *TestDesiredState) SetShutdownRequested(requested bool) {
	t.ShutdownReq = requested
}

// TestWorker is a mock Worker for testing subdirectories.
type TestWorker struct {
	CollectErr   error
	Observed     fsmv2.ObservedState
	InitialState fsmv2.State[any, any]
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

func (m *TestWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	return config.DesiredState{State: "running"}, nil
}

func (m *TestWorker) GetInitialState() fsmv2.State[any, any] {
	if m.InitialState != nil {
		return m.InitialState
	}

	return &TestState{}
}

func (m *TestWorker) RequestShutdown() {}

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
	NextState fsmv2.State[any, any]
	Signal    fsmv2.Signal
	Action    fsmv2.Action[any]
}

func (m *TestState) Next(snapshot any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
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
// Collections follow naming convention: {workerType}_identity, {workerType}_desired, {workerType}_observed.
func CreateTestTriangularStore() *storage.TriangularStore {
	ctx := context.Background()
	basicStore := memory.NewInMemoryStore()

	workerType := "container"

	if err := basicStore.CreateCollection(ctx, workerType+"_identity", nil); err != nil {
		panic(fmt.Sprintf("failed to create identity collection: %v", err))
	}

	if err := basicStore.CreateCollection(ctx, workerType+"_desired", nil); err != nil {
		panic(fmt.Sprintf("failed to create desired collection: %v", err))
	}

	if err := basicStore.CreateCollection(ctx, workerType+"_observed", nil); err != nil {
		panic(fmt.Sprintf("failed to create observed collection: %v", err))
	}

	logger := zap.NewNop().Sugar()
	return storage.NewTriangularStore(basicStore, logger)
}

// CreateTestTriangularStoreForWorkerType creates a triangular store for a specific workerType.
// This allows tests to work with different workerTypes (e.g., "s6", "container", "benthos").
// The store is configured with three collections: {workerType}_identity, {workerType}_desired, {workerType}_observed.
// CSE fields are automatically injected by TriangularStore per role:
//   - identity: FieldSyncID, FieldVersion, FieldCreatedAt (immutable)
//   - desired: FieldSyncID, FieldVersion, FieldCreatedAt, FieldUpdatedAt (version increments)
//   - observed: FieldSyncID, FieldVersion, FieldCreatedAt, FieldUpdatedAt (version doesn't increment)
func CreateTestTriangularStoreForWorkerType(workerType string) *storage.TriangularStore {
	ctx := context.Background()
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

	logger := zap.NewNop().Sugar()
	return storage.NewTriangularStore(basicStore, logger)
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
func CreateTestSupervisorWithCircuitState(circuitOpen bool) *Supervisor[*TestObservedState, *TestDesiredState] {
	logger := zap.NewNop().Sugar()
	s := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
		WorkerType:      "test",
		Store:           CreateTestTriangularStore(),
		Logger:          logger,
		CollectorHealth: CollectorHealthConfig{},
	})
	s.circuitOpen.Store(circuitOpen)

	return s
}
