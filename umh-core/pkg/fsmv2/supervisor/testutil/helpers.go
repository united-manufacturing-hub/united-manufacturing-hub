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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
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
	ShutdownReq bool
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
