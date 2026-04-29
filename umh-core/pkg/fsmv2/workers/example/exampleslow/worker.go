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

package example_slow

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
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/state"
)

// WorkerTypeName is the canonical worker-type identifier for the exampleslow
// worker, used in config YAML and CSE storage.
const WorkerTypeName = "exampleslow"

// Compile-time interface check: ExampleslowWorker implements fsmv2.Worker.
var _ fsmv2.Worker = (*ExampleslowWorker)(nil)

// ExampleslowWorker implements the FSM Worker interface for the example slow
// worker. It demonstrates long-running action handling and context cancellation
// via a configurable connect delay.
type ExampleslowWorker struct {
	fsmv2.WorkerBase[ExampleslowConfig, ExampleslowStatus]
	deps *ExampleslowDependencies
}

// NewExampleslowWorker creates a new exampleslow worker. The dependencies
// parameter is optional: when nil the constructor provisions a
// DefaultConnectionPool and builds fresh dependencies around the framework
// logger/stateReader/identity. Tests pass fully-built dependencies directly
// to exercise the worker with a custom ConnectionPool.
//
// Returns fsmv2.Worker to align with the register.Worker constructor contract
// used by transport/push/pull and persistence.
func NewExampleslowWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	dependencies *ExampleslowDependencies,
) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = WorkerTypeName
	}

	if dependencies == nil {
		dependencies = NewExampleslowDependencies(&DefaultConnectionPool{}, logger, stateReader, identity)
	} else if dependencies.GetConnectionPool() == nil {
		return nil, errors.New("connectionPool must not be nil")
	}

	w := &ExampleslowWorker{deps: dependencies}
	w.InitBase(identity, logger, stateReader)

	// Sync user-provided DelaySeconds from the parsed config into the
	// runtime dependencies so ConnectAction can observe the current value.
	// The post-parse hook is the only DeriveDesiredState seam that reaches
	// the worker struct without violating the PURE_DERIVE invariant
	// (it receives only *TConfig and closes over w for the side-effect).
	w.SetPostParseHook(func(cfg *ExampleslowConfig) error {
		w.deps.SetDelaySeconds(cfg.DelaySeconds)
		return nil
	})

	return w, nil
}

// GetDependencies returns the typed exampleslow dependencies.
// Used by tests and by external callers that need to observe worker state.
func (w *ExampleslowWorker) GetDependencies() *ExampleslowDependencies {
	return w.deps
}

// GetDependenciesAny returns the custom ExampleslowDependencies.
// Overrides WorkerBase's default which returns *BaseDependencies.
// Required by architecture test: custom deps must be visible to the supervisor.
func (w *ExampleslowWorker) GetDependenciesAny() any {
	return w.deps
}

// CollectObservedState snapshots the current connection health and connect
// attempts. Returns fsmv2.NewObservation — the collector handles CollectedAt,
// framework metrics, action history, and metric accumulation automatically
// after COS returns.
func (w *ExampleslowWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	connectionHealth := "no connection"
	if w.deps.IsConnected() {
		connectionHealth = "healthy"
	}

	return fsmv2.NewObservation(ExampleslowStatus{
		ConnectionHealth: connectionHealth,
	}), nil
}

// init registers the exampleslow worker via the generic register.Worker helper
// with typed TDeps = *ExampleslowDependencies. Callers that want to inject a
// custom ConnectionPool can publish deps via register.SetDeps before the
// supervisor starts; otherwise the constructor provisions a
// DefaultConnectionPool.
func init() {
	register.Worker[ExampleslowConfig, ExampleslowStatus, *ExampleslowDependencies](WorkerTypeName, NewExampleslowWorker)
}
