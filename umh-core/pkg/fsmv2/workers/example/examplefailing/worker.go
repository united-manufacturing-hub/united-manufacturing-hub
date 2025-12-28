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

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/state"
)

// FailingWorker implements the FSM v2 Worker interface for testing failure scenarios.
type FailingWorker struct {
	*helpers.BaseWorker[*FailingDependencies]
	identity   fsmv2.Identity
	logger     *zap.SugaredLogger
	connection Connection
}

// NewFailingWorker creates a new example failing worker.
func NewFailingWorker(
	identity fsmv2.Identity,
	connectionPool ConnectionPool,
	logger *zap.SugaredLogger,
	stateReader fsmv2.StateReader,
) (*FailingWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	// Set workerType if not already set (derive from snapshot type)
	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.ExamplefailingObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}
		identity.WorkerType = workerType
	}
	dependencies := NewFailingDependencies(connectionPool, logger, stateReader, identity)

	conn, err := connectionPool.Acquire()
	if err != nil {
		logger.Warnw("Failed to acquire initial connection", "error", err)
	}

	return &FailingWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
		connection: conn,
	}, nil
}

// CollectObservedState returns the current observed state of the failing worker.
func (w *FailingWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	deps := w.GetDependencies()
	connectionHealth := "no connection"
	if deps.IsConnected() {
		connectionHealth = "healthy"
	}

	observed := snapshot.ExamplefailingObservedState{
		ID:                   w.identity.ID,
		CollectedAt:          time.Now(),
		ConnectionHealth:     connectionHealth,
		ConnectAttempts:      deps.GetAttempts(),
		RestartAfterFailures: deps.GetRestartAfterFailures(),
	}

	return observed, nil
}

// DeriveDesiredState determines what state the failing worker should be in.
func (w *FailingWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
	// Handle nil spec (used during initialization in AddWorker)
	if spec == nil {
		return fsmv2types.DesiredState{
			State:         fsmv2types.DesiredStateRunning,
			ChildrenSpecs: nil,
		}, nil
	}

	userSpec, ok := spec.(fsmv2types.UserSpec)
	if !ok {
		return fsmv2types.DesiredState{}, fmt.Errorf("invalid spec type: expected fsmv2types.UserSpec, got %T", spec)
	}

	var failingSpec FailingUserSpec
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &failingSpec); err != nil {
			return fsmv2types.DesiredState{}, fmt.Errorf("failed to parse failing spec: %w", err)
		}
	}

	// Apply configuration to dependencies
	deps := w.GetDependencies()
	deps.SetShouldFail(failingSpec.ShouldFail)
	deps.SetMaxFailures(failingSpec.GetMaxFailures())
	deps.SetRestartAfterFailures(failingSpec.GetRestartAfterFailures())

	return fsmv2types.DesiredState{
		State:         failingSpec.GetState(), // Uses BaseUserSpec.GetState() with default "running"
		ChildrenSpecs: nil,
	}, nil
}

// GetInitialState returns the state the FSM should start in.
func (w *FailingWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	// Register both worker and supervisor factories atomically.
	// The worker type is derived from ExamplefailingObservedState, ensuring consistency.
	// NOTE: This fixes a previous key mismatch where supervisor was "examplefailing" but worker was "failing".
	if err := factory.RegisterWorkerType[snapshot.ExamplefailingObservedState, *snapshot.ExamplefailingDesiredState](
		func(id fsmv2.Identity, logger *zap.SugaredLogger, stateReader fsmv2.StateReader) fsmv2.Worker {
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
