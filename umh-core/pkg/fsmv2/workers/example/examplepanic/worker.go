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
	w.BindDeps(NewExamplepanicDependencies(connectionPool, logger, stateReader, identity))

	return w, nil
}

// GetDependencies returns the typed ExamplepanicDependencies for use in tests
// and internal call-sites that need direct access without the any-typed accessor.
func (w *ExamplepanicWorker) GetDependencies() *ExamplepanicDependencies {
	d, _ := w.GetDependenciesAny().(*ExamplepanicDependencies)
	return d
}

// DeriveDesiredState parses the user spec via WorkerBase, then synchronously
// pushes the parsed ShouldPanic flag into deps before any state tick runs.
//
// Why this override exists: WorkerBase.DeriveDesiredState runs once per
// userSpec change BEFORE the first CollectObservedState tick. The supervisor
// passes a zero-valued *WrappedDesiredState[ExamplepanicConfig] to the very
// first COS call (the user spec has not yet been reconciled into the
// desired-state pipeline). Without this override, SetShouldPanic(false) wins
// on tick 1, the worker connects normally, reaches Connected, and the
// panic-on-connect action is never re-dispatched — the Panic Scenario
// integration test then fails because no action_panic log is ever recorded.
//
// Routing the SetShouldPanic write through DDS guarantees it lands before the
// state machine takes its first tick, regardless of how the supervisor seeds
// COS on cold start.
func (w *ExamplepanicWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	wds, err := w.WorkerBase.DeriveDesiredState(spec)
	if err != nil {
		return nil, err
	}

	cfg := fsmv2.ExtractConfig[ExamplepanicConfig](wds)
	w.GetDependencies().SetShouldPanic(cfg.GetShouldPanic())

	return wds, nil
}

// CollectObservedState returns the current observed state of the panic worker.
// Returns NewObservation; the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically.
func (w *ExamplepanicWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	connectionHealth := "no connection"

	if w.GetDependencies().IsConnected() {
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
