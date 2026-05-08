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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

const workerTypeName = "examplefailing"

// FailingWorker implements the FSM v2 Worker interface for testing failure scenarios.
type FailingWorker struct {
	connection Connection
	*helpers.BaseWorker[*FailingDependencies]
	logger   deps.FSMLogger
	identity deps.Identity
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
		identity.WorkerType = workerTypeName
	}

	dependencies := NewFailingDependencies(connectionPool, logger, stateReader, identity)

	conn, err := connectionPool.Acquire()
	if err != nil {
		logger.SentryWarn(deps.FeatureExamples, identity.HierarchyPath, "initial_connection_failed",
			deps.Err(err))
	}

	return &FailingWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
		connection: conn,
	}, nil
}

// CollectObservedState returns the current observed state of the failing worker.
func (w *FailingWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.GetDependencies()

	// Framework state and action history are injected before COS and consumed by
	// the collector wrapper after NewObservation returns. Calling them here satisfies
	// the framework-metrics-copy and action-history-copy invariants enforced by the
	// architecture validator.
	d.GetFrameworkState()
	d.GetActionHistory()

	// Demonstration-only: this worker writes into deps from CollectObservedState
	// to simulate runtime conditions (failure cycles / panic flags) that
	// production configuration drives directly. Real workers MUST keep
	// CollectObservedState pure I/O reads. See pkg/fsmv2/README.md
	// "I/O isolation rule".
	if desired != nil {
		cfg := fsmv2.ExtractConfig[ExamplefailingConfig](desired)
		w.updateDependenciesFromConfig(cfg)
	}

	if d.ShouldDelayRecovery() {
		d.IncrementObservationsSinceFailure()
	}

	connectionHealth := "no connection"
	if d.IsConnected() {
		connectionHealth = "healthy"
	}

	status := ExamplefailingStatus{
		ConnectionHealth:         connectionHealth,
		ConnectAttempts:          d.GetAttempts(),
		RestartAfterFailures:     d.GetRestartAfterFailures(),
		AllCyclesComplete:        d.AllCyclesComplete(),
		TicksInConnectedState:    d.GetTicksInConnected(),
		CurrentCycle:             d.GetCurrentCycle(),
		TotalCycles:              d.GetFailureCycles(),
		RecoveryDelayActive:      d.ShouldDelayRecovery(),
		ObservationsSinceFailure: d.GetObservationsSinceFailure(),
	}

	return fsmv2.NewObservation(status), nil
}

// DeriveDesiredState determines what state the failing worker should be in.
func (w *FailingWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[ExamplefailingConfig]{
			State: fsmv2types.DesiredStateRunning,
		}, nil
	}

	parsed, err := fsmv2types.ParseUserSpec[FailingUserSpec](spec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse examplefailing spec: %w", err)
	}

	state := parsed.State
	if state == "" {
		state = fsmv2types.DesiredStateRunning
	}

	return &fsmv2.WrappedDesiredState[ExamplefailingConfig]{
		State: state,
		Config: ExamplefailingConfig{
			BaseUserSpec:              parsed.BaseUserSpec,
			ShouldFail:                parsed.ShouldFail,
			MaxFailures:               parsed.GetMaxFailures(),
			FailureCycles:             parsed.GetFailureCycles(),
			RestartAfterFailures:      parsed.GetRestartAfterFailures(),
			RecoveryDelayMs:           parsed.RecoveryDelayMs,
			RecoveryDelayObservations: parsed.GetRecoveryDelayObservations(),
		},
	}, nil
}

// updateDependenciesFromConfig applies config values to dependencies.
// Called from CollectObservedState so dependency mutations happen outside DeriveDesiredState.
func (w *FailingWorker) updateDependenciesFromConfig(cfg ExamplefailingConfig) {
	d := w.GetDependencies()
	d.SetShouldFail(cfg.ShouldFail)
	d.SetMaxFailures(cfg.MaxFailures)
	d.SetRestartAfterFailures(cfg.RestartAfterFailures)
	d.SetFailureCycles(cfg.FailureCycles)
	d.SetRecoveryDelayMs(cfg.RecoveryDelayMs)
	d.SetRecoveryDelayObservations(cfg.RecoveryDelayObservations)
}

// GetInitialState returns the state the FSM should start in.
// Uses the initial state registry populated by the state package's init() function.
func (w *FailingWorker) GetInitialState() fsmv2.State[any, any] {
	return fsmv2.LookupInitialState(workerTypeName)
}

func init() {
	if err := factory.RegisterWorkerAndSupervisorFactoryByType(
		workerTypeName,
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
			pool := &DefaultConnectionPool{}
			worker, _ := NewFailingWorker(id, pool, logger, stateReader)

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[fsmv2.Observation[ExamplefailingStatus], *fsmv2.WrappedDesiredState[ExamplefailingConfig]](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}
