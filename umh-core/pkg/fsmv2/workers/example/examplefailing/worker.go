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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/state"
)

// FailingWorker implements the FSM v2 Worker interface for testing failure scenarios.
type FailingWorker struct {
	connection Connection
	deps       *FailingDependencies
	fsmv2.WorkerBase[FailingUserSpec, snapshot.ExamplefailingObservedState, *FailingDependencies]
}

// NewFailingWorker creates a new example failing worker.
func NewFailingWorker(
	identity deps.Identity,
	connectionPool ConnectionPool,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*FailingWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}

	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.ExamplefailingObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}

		identity.WorkerType = workerType
	}

	w := &FailingWorker{}
	baseDeps := w.InitBase(identity, logger, stateReader)
	w.deps = NewFailingDependencies(connectionPool, baseDeps)
	w.BindDeps(w.deps)

	conn, err := connectionPool.Acquire()
	if err != nil {
		logger.SentryWarn(deps.FeatureExamples, identity.HierarchyPath, "initial_connection_failed",
			deps.Err(err))
	}

	w.connection = conn

	return w, nil
}

// CollectObservedState returns the current observed state of the failing worker.
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

	observed := snapshot.ExamplefailingObservedState{
		ID:                       w.Identity().ID,
		CollectedAt:              time.Now(),
		ConnectionHealth:         connectionHealth,
		ConnectAttempts:          w.deps.GetAttempts(),
		RestartAfterFailures:     w.deps.GetRestartAfterFailures(),
		AllCyclesComplete:        w.deps.AllCyclesComplete(),
		TicksInConnectedState:    w.deps.GetTicksInConnected(),
		CurrentCycle:             w.deps.GetCurrentCycle(),
		TotalCycles:              w.deps.GetFailureCycles(),
		RecoveryDelayActive:      w.deps.ShouldDelayRecovery(),
		ObservationsSinceFailure: w.deps.GetObservationsSinceFailure(),
	}
	observed.ShouldFail = w.deps.GetShouldFail()

	if fm := w.deps.GetFrameworkState(); fm != nil {
		observed.Metrics.Framework = *fm
	}

	observed.LastActionResults = w.deps.GetActionHistory()

	return observed, nil
}

// DeriveDesiredState determines what state the failing worker should be in.
func (w *FailingWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	desired, err := fsmv2types.DeriveLeafState[FailingUserSpec](spec)
	if err != nil {
		return nil, err
	}

	w.updateDependenciesFromSpec(spec)

	return &desired, nil
}

// updateDependenciesFromSpec configures dependencies based on the user spec.
func (w *FailingWorker) updateDependenciesFromSpec(spec interface{}) {
	if spec == nil {
		return
	}

	parsed, err := fsmv2types.ParseUserSpec[FailingUserSpec](spec)
	if err != nil {
		return
	}

	w.deps.SetShouldFail(parsed.ShouldFail)
	w.deps.SetMaxFailures(parsed.GetMaxFailures())
	w.deps.SetRestartAfterFailures(parsed.GetRestartAfterFailures())
	w.deps.SetFailureCycles(parsed.GetFailureCycles())
	w.deps.SetRecoveryDelayMs(parsed.RecoveryDelayMs)
	w.deps.SetRecoveryDelayObservations(parsed.GetRecoveryDelayObservations())
}

// GetInitialState returns the state the FSM should start in.
func (w *FailingWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	if err := factory.RegisterWorkerType[snapshot.ExamplefailingObservedState, *snapshot.ExamplefailingDesiredState](
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
			pool := &DefaultConnectionPool{}
			worker, _ := NewFailingWorker(id, pool, logger, stateReader)

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.ExamplefailingObservedState, *snapshot.ExamplefailingDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}
