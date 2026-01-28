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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/testutil"
	"go.uber.org/zap"
)

type TestObservedState = testutil.ObservedState

type TestDesiredState = testutil.DesiredState

type TestWorker = testutil.Worker

type TestWorkerWithType = testutil.WorkerWithType

type TestState = testutil.State

func TestIdentity() deps.Identity {
	return testutil.Identity()
}

func CreateTestTriangularStore() *storage.TriangularStore {
	return testutil.CreateTriangularStore()
}

func CreateTestTriangularStoreForWorkerType(workerType string) *storage.TriangularStore {
	return testutil.CreateTriangularStoreForWorkerType(workerType)
}

func CreateTestObservedStateWithID(id string) *TestObservedState {
	return testutil.CreateObservedStateWithID(id)
}

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
