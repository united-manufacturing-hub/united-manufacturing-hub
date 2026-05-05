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
	fsmv2.WorkerBase[ExamplefailingConfig, snapshot.ExamplefailingObservedState, *FailingDependencies]
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
func (w *FailingWorker) CollectObservedState(ctx context.Context, desiredAny fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	desired, ok := desiredAny.(*snapshot.ExamplefailingDesiredState)
	if !ok || desired == nil {
		w.deps.GetLogger().SentryWarn(deps.FeatureExamples, w.Identity().HierarchyPath,
			"examplefailing_cos_desired_state_assertion_failed")
		desired = &snapshot.ExamplefailingDesiredState{}
	}

	// Advance the observation counter early in COS, before the snapshot is assembled.
	// The counter increments only while recovery delay is active: desired.RecoveryDelayObservations > 0
	// and the current count has not yet reached the threshold. Incrementing here (rather than in a
	// state action) gives tests deterministic control: each COS call = exactly one observation step.
	recoveryDelayObservations := desired.RecoveryDelayObservations
	if recoveryDelayObservations > 0 && w.deps.GetObservationsSinceFailure() < recoveryDelayObservations {
		w.deps.IncrementObservationsSinceFailure()
	}

	connectionHealth := "no connection"

	if w.deps.IsConnected() {
		connectionHealth = "healthy"
	}

	allCyclesComplete := w.deps.GetCurrentCycle() >= desired.FailureCycles

	observed := snapshot.ExamplefailingObservedState{
		ID:                       w.Identity().ID,
		CollectedAt:              time.Now(),
		ConnectionHealth:         connectionHealth,
		ConnectAttempts:          w.deps.GetAttempts(),
		AllCyclesComplete:        allCyclesComplete,
		TicksInConnectedState:    w.deps.GetTicksInConnected(),
		CurrentCycle:             w.deps.GetCurrentCycle(),
		RecoveryDelayActive:      recoveryDelayObservations > 0 && w.deps.GetObservationsSinceFailure() < recoveryDelayObservations,
		ObservationsSinceFailure: w.deps.GetObservationsSinceFailure(),
	}
	observed.ShouldFail = desired.ShouldFail
	observed.MaxFailures = desired.MaxFailures
	observed.FailureCycles = desired.FailureCycles
	observed.RestartAfterFailures = desired.RestartAfterFailures
	observed.RecoveryDelayObservations = desired.RecoveryDelayObservations

	if fm := w.deps.GetFrameworkState(); fm != nil {
		observed.Metrics.Framework = *fm
	}

	observed.LastActionResults = w.deps.GetActionHistory()

	return observed, nil
}

// DeriveDesiredState determines what state the failing worker should be in.
func (w *FailingWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &snapshot.ExamplefailingDesiredState{}, nil
	}

	parsed, err := fsmv2types.ParseUserSpec[ExamplefailingConfig](spec)
	if err != nil {
		return nil, err
	}

	return &snapshot.ExamplefailingDesiredState{
		ShouldFail:                parsed.ShouldFail,
		MaxFailures:               parsed.GetMaxFailures(),
		FailureCycles:             parsed.GetFailureCycles(),
		RestartAfterFailures:      parsed.GetRestartAfterFailures(),
		RecoveryDelayObservations: parsed.GetRecoveryDelayObservations(),
	}, nil
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
