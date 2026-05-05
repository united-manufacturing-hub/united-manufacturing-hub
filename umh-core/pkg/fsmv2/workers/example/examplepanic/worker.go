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

package example_panic

import (
	"context"
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// workerType is the registered type name for this worker.
const workerType = "examplepanic"

type ExamplepanicWorker struct {
	deps *ExamplepanicDependencies
	fsmv2.WorkerBase[ExamplepanicConfig, ExamplepanicStatus, *ExamplepanicDependencies]
}

func NewExamplepanicWorker(
	identity deps.Identity,
	connectionPool ConnectionPool,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*ExamplepanicWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}

	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	w := &ExamplepanicWorker{}
	w.InitBase(identity, logger, stateReader)
	w.deps = NewExamplepanicDependencies(connectionPool, logger, stateReader, identity)
	w.BindDeps(w.deps)

	return w, nil
}

// CollectObservedState returns the current observed state of the panic worker.
// Returns NewObservation; the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically.
//
// The ShouldPanic flag from the typed config is propagated to dependencies here
// so the connect action can read it via IsShouldPanic() at execution time.
func (w *ExamplepanicWorker) CollectObservedState(_ context.Context, desiredAny fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	cfg := fsmv2.ExtractConfig[ExamplepanicConfig](desiredAny)
	w.deps.SetShouldPanic(cfg.GetShouldPanic())

	connectionHealth := "no connection"

	if w.deps.IsConnected() {
		connectionHealth = "healthy"
	}

	status := ExamplepanicStatus{
		ConnectionHealth: connectionHealth,
	}

	return fsmv2.NewObservation(status), nil
}

func init() {
	register.Worker[ExamplepanicConfig, ExamplepanicStatus, *ExamplepanicDependencies](
		workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			pool := &DefaultConnectionPool{}
			return NewExamplepanicWorker(id, pool, logger, sr)
		},
	)
}
