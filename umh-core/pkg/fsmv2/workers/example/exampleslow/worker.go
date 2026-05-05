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

package exampleslow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/state"
)

type ExampleslowWorker struct {
	deps *ExampleslowDependencies
	fsmv2.WorkerBase[ExampleslowConfig, snapshot.ExampleslowObservedState, *ExampleslowDependencies]
}

func NewExampleslowWorker(
	identity deps.Identity,
	connectionPool ConnectionPool,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*ExampleslowWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}

	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.ExampleslowObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}

		identity.WorkerType = workerType
	}

	w := &ExampleslowWorker{}
	baseDeps := w.InitBase(identity, logger, stateReader)
	w.deps = NewExampleslowDependencies(connectionPool, baseDeps)
	w.BindDeps(w.deps)

	return w, nil
}

func (w *ExampleslowWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	connectionHealth := "no connection"

	if w.deps.IsConnected() {
		connectionHealth = "healthy"
	}

	observed := snapshot.ExampleslowObservedState{
		ID:               w.Identity().ID,
		CollectedAt:      time.Now(),
		ConnectionHealth: connectionHealth,
	}

	if fm := w.deps.GetFrameworkState(); fm != nil {
		observed.Metrics.Framework = *fm
	}

	observed.LastActionResults = w.deps.GetActionHistory()

	return observed, nil
}

func (w *ExampleslowWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &snapshot.ExampleslowDesiredState{}, nil
	}

	parsed, err := config.ParseUserSpec[ExampleslowConfig](spec)
	if err != nil {
		return nil, err
	}

	return &snapshot.ExampleslowDesiredState{
		DelaySeconds: parsed.DelaySeconds,
	}, nil
}

func (w *ExampleslowWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	if err := factory.RegisterWorkerType[snapshot.ExampleslowObservedState, *snapshot.ExampleslowDesiredState](
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
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
