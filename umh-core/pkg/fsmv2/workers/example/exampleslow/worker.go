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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// workerType is the registered type name for this worker.
const workerType = "exampleslow"

type ExampleslowWorker struct {
	deps *ExampleslowDependencies
	fsmv2.WorkerBase[ExampleslowConfig, ExampleslowStatus, *ExampleslowDependencies]
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
		identity.WorkerType = workerType
	}

	w := &ExampleslowWorker{}
	baseDeps := w.InitBase(identity, logger, stateReader)
	w.deps = NewExampleslowDependencies(connectionPool, baseDeps)
	w.BindDeps(w.deps)

	return w, nil
}

// CollectObservedState returns the current observed state of the slow worker.
// Returns NewObservation; the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically.
func (w *ExampleslowWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	connectionHealth := "no connection"

	if w.deps.IsConnected() {
		connectionHealth = "healthy"
	}

	status := ExampleslowStatus{
		ConnectionHealth: connectionHealth,
	}

	return fsmv2.NewObservation(status), nil
}

func init() {
	register.Worker[ExampleslowConfig, ExampleslowStatus, *ExampleslowDependencies](
		workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			pool := &DefaultConnectionPool{}
			return NewExampleslowWorker(id, pool, logger, sr)
		},
	)
}
