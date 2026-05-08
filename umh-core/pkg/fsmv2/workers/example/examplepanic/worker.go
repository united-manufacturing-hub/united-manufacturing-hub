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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

type ExamplepanicWorker struct {
	*helpers.BaseWorker[*ExamplepanicDependencies]
	logger   deps.FSMLogger
	identity deps.Identity
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
		identity.WorkerType = "examplepanic"
	}

	dependencies := NewExamplepanicDependencies(connectionPool, logger, stateReader, identity)

	return &ExamplepanicWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
	}, nil
}

func (w *ExamplepanicWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
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

	if desired != nil {
		cfg := fsmv2.ExtractConfig[ExamplepanicConfig](desired)
		d.SetShouldPanic(cfg.ShouldPanic)
	}

	connectionHealth := "no connection"

	if d.IsConnected() {
		connectionHealth = "healthy"
	}

	status := ExamplepanicStatus{
		ConnectionHealth: connectionHealth,
	}

	return fsmv2.NewObservation(status), nil
}

func (w *ExamplepanicWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[ExamplepanicConfig]{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
		}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	parsed, err := config.ParseUserSpec[ExamplepanicUserSpec](userSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse examplepanic spec: %w", err)
	}

	state := parsed.GetState()
	if state == "" {
		state = config.DesiredStateRunning
	}

	return &fsmv2.WrappedDesiredState[ExamplepanicConfig]{
		BaseDesiredState: config.BaseDesiredState{State: state},
		Config: ExamplepanicConfig{
			BaseUserSpec: parsed.BaseUserSpec,
			ShouldRun:    parsed.ShouldRun,
			ShouldPanic:  parsed.ShouldPanic,
		},
	}, nil
}

// GetInitialState returns the state the FSM should start in.
// Uses the initial state registry populated by the state package's init() function.
func (w *ExamplepanicWorker) GetInitialState() fsmv2.State[any, any] {
	return fsmv2.LookupInitialState("examplepanic")
}

func init() {
	if err := factory.RegisterWorkerAndSupervisorFactoryByType(
		"examplepanic",
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
			pool := &DefaultConnectionPool{}
			worker, _ := NewExamplepanicWorker(id, pool, logger, stateReader)

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[fsmv2.Observation[ExamplepanicStatus], *fsmv2.WrappedDesiredState[ExamplepanicConfig]](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}
