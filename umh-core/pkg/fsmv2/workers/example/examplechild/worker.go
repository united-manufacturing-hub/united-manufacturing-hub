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

package example_child

import (
	"context"
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"

	// Blank import for side effects: registers the "Stopped" initial state
	// via fsmv2.RegisterInitialState in state/state_stopped.go init(). WorkerBase's
	// GetInitialState looks up the state from the registry, so the state
	// package must be loaded whenever the worker is imported — otherwise the
	// registry lookup returns nil and the supervisor panics at first tick.
	// This import is safe because state/ depends on snapshot/ (not on the
	// worker package), so no import cycle is introduced.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/state"
)

// WorkerTypeName is the canonical worker-type identifier for the examplechild
// worker, used in config YAML and CSE storage.
const WorkerTypeName = "examplechild"

const workerType = WorkerTypeName

// Compile-time interface check: ChildWorker implements fsmv2.Worker.
var _ fsmv2.Worker = (*ChildWorker)(nil)

// ChildWorker implements the FSM Worker interface for the example child
// worker. Its lifecycle is orchestrated by the parent via ChildStartStates.
type ChildWorker struct {
	fsmv2.WorkerBase[ExamplechildConfig, ExamplechildStatus]
	deps *ExamplechildDependencies
}

// NewChildWorker creates a new example child worker. The dependencies
// parameter is optional: when nil the constructor provisions a
// DefaultConnectionPool and builds fresh dependencies around the framework
// logger/stateReader/identity. Tests pass fully-built dependencies directly
// to exercise the worker with a custom ConnectionPool.
//
// Returns fsmv2.Worker to align with the register.Worker constructor contract
// used by transport/push/pull and persistence.
func NewChildWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	dependencies *ExamplechildDependencies,
) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	if dependencies == nil {
		dependencies = NewExamplechildDependencies(&DefaultConnectionPool{}, logger, stateReader, identity)
	} else if dependencies.GetConnectionPool() == nil {
		return nil, errors.New("connectionPool must not be nil")
	}

	w := &ChildWorker{deps: dependencies}
	w.InitBase(identity, logger, stateReader)

	return w, nil
}

// GetDependencies returns the typed examplechild dependencies.
// Used by tests and by external callers that need to observe worker state.
func (w *ChildWorker) GetDependencies() *ExamplechildDependencies {
	return w.deps
}

// GetDependenciesAny returns the custom ExamplechildDependencies.
// Overrides WorkerBase's default which returns *BaseDependencies.
// Required by architecture test: custom deps must be visible to the supervisor.
func (w *ChildWorker) GetDependenciesAny() any {
	return w.deps
}

// CollectObservedState snapshots the current connection health. Returns
// fsmv2.NewObservation — the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically after COS returns.
func (w *ChildWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	connectionHealth := "no connection"
	if w.deps.IsConnected() {
		connectionHealth = "healthy"
	}

	return fsmv2.NewObservation(ExamplechildStatus{
		ConnectionHealth: connectionHealth,
	}), nil
}

// init registers the examplechild worker via the generic register.Worker
// helper with typed TDeps = *ExamplechildDependencies. Callers that want to
// inject a custom ConnectionPool can publish deps via register.SetDeps before
// the supervisor starts; otherwise the constructor provisions a
// DefaultConnectionPool.
func init() {
	register.Worker[ExamplechildConfig, ExamplechildStatus, *ExamplechildDependencies](WorkerTypeName, NewChildWorker)
}
