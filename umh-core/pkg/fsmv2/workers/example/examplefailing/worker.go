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

package examplefailing

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
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/state"
)

// WorkerTypeName is the canonical worker-type identifier for the examplefailing
// worker, used in config YAML and CSE storage.
const WorkerTypeName = "examplefailing"

// Compile-time interface check: FailingWorker implements fsmv2.Worker.
var _ fsmv2.Worker = (*FailingWorker)(nil)

// FailingWorker implements the FSM Worker interface for the examplefailing
// worker. It demonstrates deterministic failure and recovery scenarios via a
// configurable number of failure cycles, restart signaling, and observation-
// based recovery delays.
type FailingWorker struct {
	fsmv2.WorkerBase[ExamplefailingConfig, ExamplefailingStatus]
	deps *FailingDependencies
}

// NewFailingWorker creates a new examplefailing worker. The dependencies
// parameter is optional: when nil the constructor provisions a
// DefaultConnectionPool and builds fresh dependencies around the framework
// logger/stateReader/identity. Tests pass fully-built dependencies directly
// to exercise the worker with a custom ConnectionPool.
//
// Returns fsmv2.Worker to align with the register.Worker constructor contract
// used by transport/push/pull and persistence.
func NewFailingWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	dependencies *FailingDependencies,
) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = WorkerTypeName
	}

	if dependencies == nil {
		dependencies = NewFailingDependencies(&DefaultConnectionPool{}, logger, stateReader, identity)
	} else if dependencies.GetConnectionPool() == nil {
		return nil, errors.New("connectionPool must not be nil")
	}

	w := &FailingWorker{deps: dependencies}
	w.InitBase(identity, logger, stateReader)

	// Sync user-provided config fields from the parsed config into the
	// runtime dependencies so actions can observe the current values.
	// The post-parse hook is the only DeriveDesiredState seam that reaches
	// the worker struct without violating the PURE_DERIVE invariant
	// (it receives only *TConfig and closes over w for the side-effect).
	w.SetPostParseHook(func(cfg *ExamplefailingConfig) error {
		w.deps.SetShouldFail(cfg.ShouldFail)
		w.deps.SetMaxFailures(cfg.GetMaxFailures())
		w.deps.SetFailureCycles(cfg.GetFailureCycles())
		w.deps.SetRestartAfterFailures(cfg.GetRestartAfterFailures())
		w.deps.SetRecoveryDelayMs(cfg.RecoveryDelayMs)
		w.deps.SetRecoveryDelayObservations(cfg.GetRecoveryDelayObservations())

		return nil
	})

	return w, nil
}

// GetDependencies returns the typed failing dependencies.
// Used by tests and by external callers that need to observe worker state.
func (w *FailingWorker) GetDependencies() *FailingDependencies {
	return w.deps
}

// GetDependenciesAny returns the custom FailingDependencies.
// Overrides WorkerBase's default which returns *BaseDependencies.
// Required by architecture test: custom deps must be visible to the supervisor.
func (w *FailingWorker) GetDependenciesAny() any {
	return w.deps
}

// CollectObservedState snapshots the current connection health, cycle
// counters, and recovery-delay state. Returns fsmv2.NewObservation — the
// collector handles CollectedAt, framework metrics, action history, and
// metric accumulation automatically after COS returns.
func (w *FailingWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// IMPORTANT: Increment observation counter at START of collection when recovery delay is active.
	// This ensures deterministic counting for tests.
	if w.deps.ShouldDelayRecovery() {
		w.deps.IncrementObservationsSinceFailure()
	}

	connectionHealth := "no connection"
	if w.deps.IsConnected() {
		connectionHealth = "healthy"
	}

	return fsmv2.NewObservation(ExamplefailingStatus{
		ConnectionHealth:         connectionHealth,
		ConnectAttempts:          w.deps.GetAttempts(),
		RestartAfterFailures:     w.deps.GetRestartAfterFailures(),
		TicksInConnectedState:    w.deps.GetTicksInConnected(),
		CurrentCycle:             w.deps.GetCurrentCycle(),
		TotalCycles:              w.deps.GetFailureCycles(),
		ObservationsSinceFailure: w.deps.GetObservationsSinceFailure(),
		ShouldFail:               w.deps.GetShouldFail(),
		AllCyclesComplete:        w.deps.AllCyclesComplete(),
		RecoveryDelayActive:      w.deps.ShouldDelayRecovery(),
	}), nil
}

// init registers the examplefailing worker via the generic register.Worker
// helper with typed TDeps = *FailingDependencies. Callers that want to inject
// a custom ConnectionPool can publish deps via register.SetDeps before the
// supervisor starts; otherwise the constructor provisions a
// DefaultConnectionPool.
func init() {
	register.Worker[ExamplefailingConfig, ExamplefailingStatus, *FailingDependencies](WorkerTypeName, NewFailingWorker)
}
