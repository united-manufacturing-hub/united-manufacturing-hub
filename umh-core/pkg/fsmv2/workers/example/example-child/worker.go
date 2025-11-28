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
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/state"
)

// ChildWorker implements the FSM v2 Worker interface for resource management.
type ChildWorker struct {
	*helpers.BaseWorker[*ChildDependencies]
	identity   fsmv2.Identity
	logger     *zap.SugaredLogger
	connection Connection
}

// NewChildWorker creates a new example child worker.
func NewChildWorker(
	identity fsmv2.Identity,
	connectionPool ConnectionPool,
	logger *zap.SugaredLogger,
) (*ChildWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	// Set workerType if not already set (derive from snapshot type)
	if identity.WorkerType == "" {
		identity.WorkerType = storage.DeriveWorkerType[snapshot.ChildObservedState]()
	}
	dependencies := NewChildDependencies(connectionPool, logger, identity)

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

	// Get connection status from dependencies (updated by ConnectAction/DisconnectAction)
	deps := w.GetDependencies()
	connectionStatus := "disconnected"
	connectionHealth := "no connection"
	if deps.IsConnected() {
		connectionStatus = "connected"
		connectionHealth = "healthy"
	}

	observed := snapshot.ChildObservedState{
		ID:               w.identity.ID,
		CollectedAt:      time.Now(),
		ConnectionStatus: connectionStatus,
		ConnectionHealth: connectionHealth,
	}

	return observed, nil
}

// DeriveDesiredState determines what state the child worker should be in.
func (w *ChildWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
	// Handle nil spec (used during initialization in AddWorker)
	if spec == nil {
		return fsmv2types.DesiredState{
			State:         "connected",
			ChildrenSpecs: nil,
		}, nil
	}

	userSpec, ok := spec.(fsmv2types.UserSpec)
	if !ok {
		return fsmv2types.DesiredState{}, fmt.Errorf("invalid spec type: expected fsmv2types.UserSpec, got %T", spec)
	}

	var childSpec ChildUserSpec
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &childSpec); err != nil {
			return fsmv2types.DesiredState{}, fmt.Errorf("failed to parse child spec: %w", err)
		}
	}

	return fsmv2types.DesiredState{
		State:         "connected",
		ChildrenSpecs: nil,
	}, nil
}

// GetInitialState returns the state the FSM should start in.
func (w *ChildWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	// Register supervisor factory for creating child supervisors
	_ = factory.RegisterSupervisorFactory[snapshot.ChildObservedState, *snapshot.ChildDesiredState](
		func(cfg interface{}) interface{} {
			supervisorCfg := cfg.(supervisor.Config)

			return supervisor.NewSupervisor[snapshot.ChildObservedState, *snapshot.ChildDesiredState](supervisorCfg)
		})

	// Register worker factory for ApplicationWorker to create child workers via YAML config
	_ = factory.RegisterFactoryByType("child", func(identity fsmv2.Identity, logger *zap.SugaredLogger) fsmv2.Worker {
		pool := &DefaultConnectionPool{}
		worker, _ := NewChildWorker(identity, pool, logger)
		return worker
	})
}
