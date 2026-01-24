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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic/state"
)

type ExamplepanicWorker struct {
	*helpers.BaseWorker[*ExamplepanicDependencies]
	logger   *zap.SugaredLogger
	identity deps.Identity
}

func NewExamplepanicWorker(
	identity deps.Identity,
	connectionPool ConnectionPool,
	logger *zap.SugaredLogger,
	stateReader deps.StateReader,
) (*ExamplepanicWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}

	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.ExamplepanicObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}

		identity.WorkerType = workerType
	}

	dependencies := NewExamplepanicDependencies(connectionPool, logger, stateReader, identity)

	return &ExamplepanicWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
	}, nil
}

func (w *ExamplepanicWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
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

	observed := snapshot.ExamplepanicObservedState{
		ID:               w.identity.ID,
		CollectedAt:      time.Now(),
		ConnectionHealth: connectionHealth,
	}

	if fm := deps.GetFrameworkState(); fm != nil {
		observed.Metrics.Framework = *fm
	}

	observed.LastActionResults = deps.GetActionHistory()

	return observed, nil
}

func (w *ExamplepanicWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	desired, err := config.DeriveLeafState[ExamplepanicUserSpec](spec)
	if err != nil {
		return nil, err
	}

	w.updateDependenciesFromSpec(spec)

	return &desired, nil
}

// updateDependenciesFromSpec configures dependencies based on the user spec (separate to avoid PURE_DERIVE violations).
func (w *ExamplepanicWorker) updateDependenciesFromSpec(spec interface{}) {
	if spec == nil {
		return
	}

	parsed, err := config.ParseUserSpec[ExamplepanicUserSpec](spec)
	if err != nil {
		return
	}

	deps := w.GetDependencies()
	deps.SetShouldPanic(parsed.ShouldPanic)
}

func (w *ExamplepanicWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	if err := factory.RegisterWorkerType[snapshot.ExamplepanicObservedState, *snapshot.ExamplepanicDesiredState](
		func(id deps.Identity, logger *zap.SugaredLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
			pool := &DefaultConnectionPool{}
			worker, _ := NewExamplepanicWorker(id, pool, logger, stateReader)

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.ExamplepanicObservedState, *snapshot.ExamplepanicDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}
