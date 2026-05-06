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

package examplechild

import (
	"context"
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// workerType is the registered type name for this worker.
const workerType = "examplechild"

// ChildWorker implements the FSM v2 Worker interface for resource management.
type ChildWorker struct {
	fsmv2.WorkerBase[ExamplechildConfig, ExamplechildStatus, *ExamplechildDependencies]
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
		identity.WorkerType = workerType
	}

	w := &ChildWorker{}
	baseDeps := w.InitBase(identity, logger, stateReader)
	w.BindDeps(NewExamplechildDependencies(connectionPool, baseDeps))

	return w, nil
}

// GetDependencies returns the typed ExamplechildDependencies for use in tests
// and internal call-sites that need direct access without the any-typed accessor.
func (w *ChildWorker) GetDependencies() *ExamplechildDependencies {
	d, _ := w.GetDependenciesAny().(*ExamplechildDependencies)
	return d
}

// CollectObservedState returns the current observed state of the child worker.
// Returns NewObservation; the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically.
func (w *ChildWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	connectionHealth := "no connection"

	if w.GetDependencies().IsConnected() {
		connectionHealth = "healthy"
	}

	status := ExamplechildStatus{
		ConnectionHealth: connectionHealth,
	}

	return fsmv2.NewObservation(status), nil
}

func init() {
	register.Worker[ExamplechildConfig, ExamplechildStatus, *ExamplechildDependencies](
		workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			pool := &DefaultConnectionPool{}
			return NewChildWorker(id, pool, logger, sr)
		},
	)
}
