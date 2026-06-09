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

// Package configworker provides the kernel config worker. The worker holds the
// shared registry published by the parent wiring (register.SetDeps ->
// register.GetDeps) so the application control surface can read the dynamic
// child specs recorded via the registry's Upsert.
package configworker

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	registry "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/configworker"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/snapshot"

	// Blank-import the state subpackage so its init() registers the initial
	// state before the supervisor ticks the worker.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/state"
)

// workerType is the canonical name registered in init() and used as the folder name.
const workerType = "configworker"

// ConfigworkerWorker implements the FSMv2 Worker interface and holds the shared
// registry of dynamic child specs.
type ConfigworkerWorker struct {
	registry *registry.Registry

	fsmv2.WorkerBase[snapshot.ConfigworkerConfig, snapshot.ConfigworkerStatus, register.NoDeps]
}

// NewConfigworkerWorker creates a config worker holding the registry published
// under workerType via register.SetDeps. It fails when no registry was published,
// surfacing the missing wiring at construction instead of at the first registry
// read (a nil *Registry would otherwise panic on a method call far from the cause).
func NewConfigworkerWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*ConfigworkerWorker, error) {
	shared := register.GetDeps[*registry.Registry](workerType)
	if shared == nil {
		return nil, fmt.Errorf("no registry published for worker type %q: call register.SetDeps before constructing", workerType)
	}

	w := &ConfigworkerWorker{
		registry: shared,
	}
	w.InitBase(identity, logger, stateReader)

	return w, nil
}

// Registry returns the shared registry the worker holds.
func (w *ConfigworkerWorker) Registry() *registry.Registry {
	return w.registry
}

// GetDependenciesAny returns nil so the framework skips metrics injection for
// this no-deps worker (a boxed struct{}{} would be non-nil).
func (w *ConfigworkerWorker) GetDependenciesAny() any {
	return nil
}

// CollectObservedState returns the current observed state.
func (w *ConfigworkerWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return fsmv2.NewObservation(snapshot.ConfigworkerStatus{}), nil
}

func init() {
	register.Worker[snapshot.ConfigworkerConfig, snapshot.ConfigworkerStatus, register.NoDeps](workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			return NewConfigworkerWorker(id, logger, sr)
		})
}

// ensure ConfigworkerWorker implements Worker interfaces (compile-time check).
var _ fsmv2.Worker = (*ConfigworkerWorker)(nil)
