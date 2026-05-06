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
)

// workerType is the registered type name for this worker.
const workerType = "examplefailing"

// FailingWorker implements the FSM v2 Worker interface for testing failure scenarios.
type FailingWorker struct {
	fsmv2.WorkerBase[ExamplefailingConfig, ExamplefailingStatus, *FailingDependencies]
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
		identity.WorkerType = workerType
	}

	w := &FailingWorker{}
	baseDeps := w.InitBase(identity, logger, stateReader)
	w.BindDeps(NewFailingDependencies(connectionPool, baseDeps))

	return w, nil
}

// GetDependencies returns the typed FailingDependencies for use in tests
// and internal call-sites that need direct access without the any-typed accessor.
func (w *FailingWorker) GetDependencies() *FailingDependencies {
	d, _ := w.GetDependenciesAny().(*FailingDependencies)
	return d
}

// CollectObservedState returns the current observed state of the failing worker.
// Returns NewObservation; the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically.
//
// Demonstration-only: this worker writes into deps from CollectObservedState
// (IncrementObservationsSinceFailure below) to simulate runtime conditions
// (failure cycles) that production configuration drives directly. Real workers
// MUST keep CollectObservedState pure I/O reads. See pkg/fsmv2/README.md
// "I/O isolation rule".
func (w *FailingWorker) CollectObservedState(_ context.Context, desiredAny fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	cfg := fsmv2.ExtractConfig[ExamplefailingConfig](desiredAny)
	d := w.GetDependencies()

	// Advance the observation counter early in COS, before the snapshot is assembled.
	// The counter increments only while recovery delay is active: cfg.GetRecoveryDelayObservations() > 0
	// and the current count has not yet reached the threshold. Incrementing here (rather than in a
	// state action) gives tests deterministic control: each COS call = exactly one observation step.
	recoveryDelayObservations := cfg.GetRecoveryDelayObservations()
	if recoveryDelayObservations > 0 && d.GetObservationsSinceFailure() < recoveryDelayObservations {
		d.IncrementObservationsSinceFailure()
	}

	connectionHealth := "no connection"

	if d.IsConnected() {
		connectionHealth = "healthy"
	}

	allCyclesComplete := d.GetCurrentCycle() >= cfg.GetFailureCycles()

	status := ExamplefailingStatus{
		ConnectionHealth:         connectionHealth,
		ConnectAttempts:          d.GetAttempts(),
		TicksInConnectedState:    d.GetTicksInConnected(),
		CurrentCycle:             d.GetCurrentCycle(),
		AllCyclesComplete:        allCyclesComplete,
		RecoveryDelayActive:      recoveryDelayObservations > 0 && d.GetObservationsSinceFailure() < recoveryDelayObservations,
		ObservationsSinceFailure: d.GetObservationsSinceFailure(),
	}

	return fsmv2.NewObservation(status), nil
}

func init() {
	register.Worker[ExamplefailingConfig, ExamplefailingStatus, *FailingDependencies](
		workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			pool := &DefaultConnectionPool{}
			return NewFailingWorker(id, pool, logger, sr)
		},
	)
}
