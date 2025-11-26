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
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-panic/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-panic/state"
)

type PanicWorker struct {
	*helpers.BaseWorker[*PanicDependencies]
	identity   fsmv2.Identity
	logger     *zap.SugaredLogger
	connection Connection
}

func NewPanicWorker(
	id string,
	name string,
	connectionPool ConnectionPool,
	logger *zap.SugaredLogger,
) (*PanicWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	workerType := storage.DeriveWorkerType[snapshot.PanicObservedState]()
	dependencies := NewPanicDependencies(connectionPool, logger, workerType, id)

	conn, err := connectionPool.Acquire()
	if err != nil {
		logger.Warnf("Failed to acquire initial connection: %v", err)
	}

	return &PanicWorker{
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

func (w *PanicWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	observed := snapshot.PanicObservedState{
		ID:               w.identity.ID,
		CollectedAt:      time.Now(),
		ConnectionStatus: w.getConnectionStatus(),
		ConnectionHealth: w.getConnectionHealth(),
	}

	return observed, nil
}

func (w *PanicWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
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

	var panicSpec PanicUserSpec
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &panicSpec); err != nil {
			return fsmv2types.DesiredState{}, fmt.Errorf("failed to parse panic spec: %w", err)
		}
	}

	return fsmv2types.DesiredState{
		State:         "connected",
		ChildrenSpecs: nil,
	}, nil
}

func (w *PanicWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func (w *PanicWorker) getConnectionStatus() string {
	if w.connection != nil {
		return "connected"
	}

	return "disconnected"
}

func (w *PanicWorker) getConnectionHealth() string {
	if w.connection == nil {
		return "no connection"
	}

	return "healthy"
}

func init() {
	_ = factory.RegisterSupervisorFactory[snapshot.PanicObservedState, *snapshot.PanicDesiredState](
		func(cfg interface{}) interface{} {
			supervisorCfg := cfg.(supervisor.Config)

			return supervisor.NewSupervisor[snapshot.PanicObservedState, *snapshot.PanicDesiredState](supervisorCfg)
		})

	_ = factory.RegisterFactoryByType("panic", func(identity fsmv2.Identity) fsmv2.Worker {
		logger := zap.NewNop().Sugar()
		pool := &DefaultConnectionPool{}
		worker, _ := NewPanicWorker(identity.ID, identity.Name, pool, logger)
		return worker
	})
}
