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

package example_failing

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-failing/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-failing/state"
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
	id string,
	name string,
	connectionPool ConnectionPool,
	logger *zap.SugaredLogger,
) (*FailingWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	workerType := storage.DeriveWorkerType[snapshot.FailingObservedState]()
	dependencies := NewFailingDependencies(connectionPool, logger, workerType, id)

	conn, err := connectionPool.Acquire()
	if err != nil {
		logger.Warnf("Failed to acquire initial connection: %v", err)
	}

	return &FailingWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity: fsmv2.Identity{
			ID:         id,
			Name:       name,
			WorkerType: workerType,
		},
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

	observed := snapshot.FailingObservedState{
		ID:               w.identity.ID,
		CollectedAt:      time.Now(),
		ConnectionStatus: w.getConnectionStatus(),
		ConnectionHealth: w.getConnectionHealth(),
	}

	return observed, nil
}

// DeriveDesiredState determines what state the failing worker should be in.
func (w *FailingWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
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

	var failingSpec FailingUserSpec
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &failingSpec); err != nil {
			return fsmv2types.DesiredState{}, fmt.Errorf("failed to parse failing spec: %w", err)
		}
	}

	return fsmv2types.DesiredState{
		State:         "connected",
		ChildrenSpecs: nil,
	}, nil
}

// GetInitialState returns the state the FSM should start in.
func (w *FailingWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func (w *FailingWorker) getConnectionStatus() string {
	if w.connection != nil {
		return "connected"
	}

	return "disconnected"
}

func (w *FailingWorker) getConnectionHealth() string {
	if w.connection == nil {
		return "no connection"
	}

	return "healthy"
}

func init() {
	// Register supervisor factory for creating failing supervisors
	_ = factory.RegisterSupervisorFactory[snapshot.FailingObservedState, *snapshot.FailingDesiredState](
		func(cfg interface{}) interface{} {
			supervisorCfg := cfg.(supervisor.Config)

			return supervisor.NewSupervisor[snapshot.FailingObservedState, *snapshot.FailingDesiredState](supervisorCfg)
		})

	// Register worker factory for ApplicationWorker to create failing workers via YAML config
	_ = factory.RegisterFactoryByType("failing", func(identity fsmv2.Identity) fsmv2.Worker {
		logger := zap.NewNop().Sugar()
		pool := &DefaultConnectionPool{}
		worker, _ := NewFailingWorker(identity.ID, identity.Name, pool, logger)
		return worker
	})
}
