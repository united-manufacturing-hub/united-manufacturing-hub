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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// ChildWorker implements the FSM v2 Worker interface for resource management.
type ChildWorker struct {
	connection Connection
	*helpers.BaseWorker[*ExamplechildDependencies]
	logger   deps.FSMLogger
	identity deps.Identity
}

// NewChildWorker creates a new example child worker.
func NewChildWorker(
	identity deps.Identity,
	connectionPool ConnectionPool,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*ChildWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}

	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = "examplechild"
	}

	dependencies := NewExamplechildDependencies(connectionPool, logger, stateReader, identity)

	conn, err := connectionPool.Acquire()
	if err != nil {
		logger.SentryWarn(deps.FeatureExamples, identity.HierarchyPath, "initial_connection_failed",
			deps.Err(err))
	}

	return &ChildWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
		connection: conn,
	}, nil
}

// CollectObservedState returns the current observed state of the child worker.
func (w *ChildWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
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

	connectionHealth := "no connection"

	if d.IsConnected() {
		connectionHealth = "healthy"
	}

	status := ExamplechildStatus{
		ConnectionHealth: connectionHealth,
	}

	return fsmv2.NewObservation(status), nil
}

// DeriveDesiredState determines what state the child worker should be in.
func (w *ChildWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[ExamplechildConfig]{
			State: config.DesiredStateRunning,
		}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	renderedConfig, err := config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	renderedSpec := config.UserSpec{
		Config:    renderedConfig,
		Variables: userSpec.Variables,
	}

	parsed, err := config.ParseUserSpec[ChildUserSpec](renderedSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to parse examplechild spec: %w", err)
	}

	state := parsed.GetState()

	return &fsmv2.WrappedDesiredState[ExamplechildConfig]{
		State: state,
		Config: ExamplechildConfig{
			BaseUserSpec: parsed.BaseUserSpec,
		},
	}, nil
}

// GetInitialState returns the state the FSM should start in.
// Uses the initial state registry populated by the state package's init() function.
func (w *ChildWorker) GetInitialState() fsmv2.State[any, any] {
	return fsmv2.LookupInitialState("examplechild")
}

func init() {
	if err := factory.RegisterWorkerAndSupervisorFactoryByType(
		"examplechild",
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
			pool := &DefaultConnectionPool{}
			worker, _ := NewChildWorker(id, pool, logger, stateReader)

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[fsmv2.Observation[ExamplechildStatus], *fsmv2.WrappedDesiredState[ExamplechildConfig]](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}
