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

package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

type ObservedState struct {
	ID          string             `json:"id"`
	CollectedAt time.Time          `json:"collectedAt"`
	Desired     fsmv2.DesiredState `json:"-"`
}

func (t *ObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return t.Desired
}

func (t *ObservedState) GetTimestamp() time.Time {
	return t.CollectedAt
}

type DesiredState struct {
	ShutdownReq bool `json:"ShutdownRequested"`
}

func (t *DesiredState) IsShutdownRequested() bool {
	return t.ShutdownReq
}

func (t *DesiredState) SetShutdownRequested(requested bool) {
	t.ShutdownReq = requested
}

type Worker struct {
	CollectErr   error
	Observed     fsmv2.ObservedState
	InitialState fsmv2.State[any, any]
	CollectFunc  func(ctx context.Context) (fsmv2.ObservedState, error)
}

func (m *Worker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	if m.CollectFunc != nil {
		return m.CollectFunc(ctx)
	}

	if m.CollectErr != nil {
		return nil, m.CollectErr
	}

	if m.Observed != nil {
		return m.Observed, nil
	}

	return &ObservedState{
		ID:          "test-worker",
		CollectedAt: time.Now(),
		Desired:     &DesiredState{},
	}, nil
}

func (m *Worker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	return config.DesiredState{State: "running"}, nil
}

func (m *Worker) GetInitialState() fsmv2.State[any, any] {
	if m.InitialState != nil {
		return m.InitialState
	}

	return &State{}
}

type WorkerWithType struct {
	Worker
	WorkerType string
}

func (m *WorkerWithType) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	if m.CollectFunc != nil {
		return m.CollectFunc(ctx)
	}

	if m.CollectErr != nil {
		return nil, m.CollectErr
	}

	if m.Observed != nil {
		return m.Observed, nil
	}

	return &ObservedState{
		ID:          m.WorkerType + "-worker",
		CollectedAt: time.Now(),
		Desired:     &DesiredState{},
	}, nil
}

type State struct {
	NextState fsmv2.State[any, any]
	Signal    fsmv2.Signal
	Action    fsmv2.Action[any]
}

func (m *State) Next(snapshot any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	if m.NextState == nil {
		return m, fsmv2.SignalNone, nil
	}

	return m.NextState, m.Signal, m.Action
}

func (m *State) String() string { return "State" }
func (m *State) Reason() string { return "test state" }

func Identity() fsmv2.Identity {
	return fsmv2.Identity{
		ID:         "test-worker",
		Name:       "Test Worker",
		WorkerType: "container",
	}
}

func CreateTriangularStore() *storage.TriangularStore {
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

	return storage.NewTriangularStore(basicStore, nil)
}

func CreateTriangularStoreForWorkerType(workerType string) *storage.TriangularStore {
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

	return storage.NewTriangularStore(basicStore, nil)
}

func CreateObservedStateWithID(id string) *ObservedState {
	return &ObservedState{
		ID:          id,
		CollectedAt: time.Now(),
		Desired:     &DesiredState{},
	}
}

type Store struct {
	Identity         map[string]map[string]persistence.Document
	Desired          map[string]map[string]persistence.Document
	Observed         map[string]map[string]persistence.Document
	SaveErr          error
	LoadSnapshotFunc func(ctx context.Context, workerType string, id string) (*storage.Snapshot, error)
	SaveDesiredFunc  func(ctx context.Context, workerType string, id string, desired persistence.Document) error
	SaveObservedFunc func(ctx context.Context, workerType string, id string, observed interface{}) (bool, error)
}

func NewStore() *Store {
	return &Store{
		Identity: make(map[string]map[string]persistence.Document),
		Desired:  make(map[string]map[string]persistence.Document),
		Observed: make(map[string]map[string]persistence.Document),
	}
}

func (s *Store) SaveIdentity(ctx context.Context, workerType string, id string, identity persistence.Document) error {
	if s.Identity[workerType] == nil {
		s.Identity[workerType] = make(map[string]persistence.Document)
	}
	s.Identity[workerType][id] = identity
	return s.SaveErr
}

func (s *Store) LoadIdentity(ctx context.Context, workerType string, id string) (persistence.Document, error) {
	if s.Identity[workerType] == nil || s.Identity[workerType][id] == nil {
		return nil, persistence.ErrNotFound
	}
	return s.Identity[workerType][id], nil
}

func (s *Store) SaveDesired(ctx context.Context, workerType string, id string, desired persistence.Document) error {
	if s.SaveDesiredFunc != nil {
		return s.SaveDesiredFunc(ctx, workerType, id, desired)
	}
	if s.Desired[workerType] == nil {
		s.Desired[workerType] = make(map[string]persistence.Document)
	}
	s.Desired[workerType][id] = desired
	return s.SaveErr
}

func (s *Store) LoadDesired(ctx context.Context, workerType string, id string) (interface{}, error) {
	if s.Desired[workerType] == nil || s.Desired[workerType][id] == nil {
		return nil, persistence.ErrNotFound
	}
	return s.Desired[workerType][id], nil
}

func (s *Store) LoadDesiredTyped(ctx context.Context, workerType string, id string, dest interface{}) error {
	result, err := s.LoadDesired(ctx, workerType, id)
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

func (s *Store) SaveObserved(ctx context.Context, workerType string, id string, observed interface{}) (bool, error) {
	if s.SaveObservedFunc != nil {
		return s.SaveObservedFunc(ctx, workerType, id, observed)
	}
	if s.Observed[workerType] == nil {
		s.Observed[workerType] = make(map[string]persistence.Document)
	}
	var doc persistence.Document
	if observedDoc, ok := observed.(persistence.Document); ok {
		doc = observedDoc
	} else {
		doc = persistence.Document{"data": observed, "collectedAt": time.Now()}
	}
	s.Observed[workerType][id] = doc
	if s.SaveErr != nil {
		return false, s.SaveErr
	}
	return true, nil
}

func (s *Store) LoadObserved(ctx context.Context, workerType string, id string) (interface{}, error) {
	if s.Observed[workerType] == nil || s.Observed[workerType][id] == nil {
		return nil, persistence.ErrNotFound
	}
	return s.Observed[workerType][id], nil
}

func (s *Store) LoadObservedTyped(ctx context.Context, workerType string, id string, dest interface{}) error {
	result, err := s.LoadObserved(ctx, workerType, id)
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

func (s *Store) LoadSnapshot(ctx context.Context, workerType string, id string) (*storage.Snapshot, error) {
	if s.LoadSnapshotFunc != nil {
		return s.LoadSnapshotFunc(ctx, workerType, id)
	}
	identity := persistence.Document{
		"id":         id,
		"name":       "Test Worker",
		"workerType": workerType,
	}
	desired := persistence.Document{}
	observed := persistence.Document{"collectedAt": time.Now()}
	if s.Identity[workerType] != nil && s.Identity[workerType][id] != nil {
		identity = s.Identity[workerType][id]
	}
	if s.Desired[workerType] != nil && s.Desired[workerType][id] != nil {
		desired = s.Desired[workerType][id]
	}
	if s.Observed[workerType] != nil && s.Observed[workerType][id] != nil {
		observed = s.Observed[workerType][id]
	}
	return &storage.Snapshot{
		Identity: identity,
		Desired:  desired,
		Observed: observed,
	}, nil
}

type Action struct {
	ExecuteFunc func(ctx context.Context, snapshot any) error
	Executed    bool
}

func (a *Action) Execute(ctx context.Context, snapshot any) error {
	a.Executed = true
	if a.ExecuteFunc != nil {
		return a.ExecuteFunc(ctx, snapshot)
	}
	return nil
}

// VerifyActionIdempotency tests that an action is idempotent by executing it
// multiple times and verifying the result is the same as executing it once.
// Re-exported from execution package for external test usage.
func VerifyActionIdempotency(action fsmv2.Action[any], iterations int, verifyState func()) {
	ctx := context.Background()

	for i := range iterations {
		err := action.Execute(ctx, nil)
		if err != nil {
			panic(fmt.Sprintf("Action should succeed on iteration %d: %v", i+1, err))
		}
	}

	verifyState()
}
