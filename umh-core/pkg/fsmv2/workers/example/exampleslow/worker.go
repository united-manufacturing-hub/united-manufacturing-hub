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

package example_slow

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/state"
)

type ExampleslowWorker struct {
	*helpers.BaseWorker[*ExampleslowDependencies]
	identity fsmv2.Identity
	logger   *zap.SugaredLogger
}

func NewExampleslowWorker(
	identity fsmv2.Identity,
	connectionPool ConnectionPool,
	logger *zap.SugaredLogger,
	stateReader fsmv2.StateReader,
) (*ExampleslowWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}

	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	// Set workerType if not already set (derive from snapshot type)
	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.ExampleslowObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}

		identity.WorkerType = workerType
	}

	dependencies := NewExampleslowDependencies(connectionPool, logger, stateReader, identity)

	return &ExampleslowWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
	}, nil
}

func (w *ExampleslowWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Get connection health from dependencies (updated by ConnectAction/DisconnectAction)
	deps := w.GetDependencies()

	connectionHealth := "no connection"

	if deps.IsConnected() {
		connectionHealth = "healthy"
	}

	observed := snapshot.ExampleslowObservedState{
		ID:               w.identity.ID,
		CollectedAt:      time.Now(),
		ConnectionHealth: connectionHealth,
	}

	return observed, nil
}

func (w *ExampleslowWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	desired, err := config.DeriveLeafState[ExampleslowUserSpec](spec)
	if err != nil {
		return nil, err
	}

	// Update dependencies with configuration from spec
	w.updateDependenciesFromSpec(spec)

	return &desired, nil
}

// updateDependenciesFromSpec configures dependencies based on the user spec.
// This is separate from DeriveDesiredState to avoid PURE_DERIVE violations.
func (w *ExampleslowWorker) updateDependenciesFromSpec(spec interface{}) {
	if spec == nil {
		return
	}

	parsed, err := config.ParseUserSpec[ExampleslowUserSpec](spec)
	if err != nil {
		return
	}

	deps := w.GetDependencies()
	deps.SetDelaySeconds(parsed.DelaySeconds)
}

func (w *ExampleslowWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	// Register both worker and supervisor factories atomically.
	// The worker type is derived from ExampleslowObservedState, ensuring consistency.
	// NOTE: This fixes a previous key mismatch where supervisor was "exampleslow" but worker was "slow".
	if err := factory.RegisterWorkerType[snapshot.ExampleslowObservedState, *snapshot.ExampleslowDesiredState](
		func(id fsmv2.Identity, logger *zap.SugaredLogger, stateReader fsmv2.StateReader, _ map[string]any) fsmv2.Worker {
			pool := &DefaultConnectionPool{}
			worker, _ := NewExampleslowWorker(id, pool, logger, stateReader)

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.ExampleslowObservedState, *snapshot.ExampleslowDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}
