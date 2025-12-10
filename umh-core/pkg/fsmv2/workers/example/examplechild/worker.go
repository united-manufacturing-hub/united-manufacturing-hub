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
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/state"
)

// ChildWorker implements the FSM v2 Worker interface for resource management.
type ChildWorker struct {
	*helpers.BaseWorker[*ExamplechildDependencies]
	identity   fsmv2.Identity
	logger     *zap.SugaredLogger
	connection Connection
}

// NewChildWorker creates a new example child worker.
func NewChildWorker(
	identity fsmv2.Identity,
	connectionPool ConnectionPool,
	logger *zap.SugaredLogger,
	stateReader fsmv2.StateReader,
) (*ChildWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	// Set workerType if not already set (derive from snapshot type)
	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.ExamplechildObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}
		identity.WorkerType = workerType
	}
	dependencies := NewExamplechildDependencies(connectionPool, logger, stateReader, identity)

	conn, err := connectionPool.Acquire()
	if err != nil {
		logger.Warnw("Failed to acquire initial connection", "error", err)
	}

	return &ChildWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
		connection: conn,
	}, nil
}

// CollectObservedState returns the current observed state of the child worker.
func (w *ChildWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// TODO: ensure the timeout and context usage here is properly implemented
	// above logic is still stupid

	// Get connection health from dependencies (updated by ConnectAction/DisconnectAction)
	deps := w.GetDependencies()
	connectionHealth := "no connection"
	if deps.IsConnected() {
		connectionHealth = "healthy"
	}

	observed := snapshot.ExamplechildObservedState{
		ID:               w.identity.ID,
		CollectedAt:      time.Now(),
		ConnectionHealth: connectionHealth,
	}

	return observed, nil
}

// DeriveDesiredState determines what state the child worker should be in.
// Uses the DeriveLeafState helper for type-safe parsing and boilerplate reduction.
func (w *ChildWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	return config.DeriveLeafState[ChildUserSpec](spec)
}

// GetInitialState returns the state the FSM should start in.
func (w *ChildWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	// Register both worker and supervisor factories atomically.
	// The worker type is derived from ExamplechildObservedState, ensuring consistency.
	if err := factory.RegisterWorkerType[snapshot.ExamplechildObservedState, *snapshot.ExamplechildDesiredState](
		func(id fsmv2.Identity, logger *zap.SugaredLogger, stateReader fsmv2.StateReader) fsmv2.Worker {
			pool := &DefaultConnectionPool{}
			worker, _ := NewChildWorker(id, pool, logger, stateReader)
			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.ExamplechildObservedState, *snapshot.ExamplechildDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}
