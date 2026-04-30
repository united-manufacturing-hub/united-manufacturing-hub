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

// Package register provides the one-line worker registration function.
// Lives in its own package to resolve circular imports:
// register → fsmv2 + factory + supervisor + cse/storage. No reverse deps.
package register

import (
	"fmt"
	"reflect"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// NoDeps is a type alias for struct{}, used as the TDeps parameter for workers
// that require no custom dependencies beyond the standard framework deps
// (Identity, FSMLogger, StateReader).
type NoDeps = struct{}

// Worker registers a worker type with the framework.
// TConfig is the developer's configuration type.
// TStatus is the developer's status/observation type.
// TDeps is passed to the constructor for workers that need custom dependencies
// beyond framework ones. Use register.NoDeps for zero-dep workers. The framework
// consults the package-level deps registry at construction time via
// GetDeps[TDeps](workerType). Parent wiring publishes typed deps by calling
// SetDeps[TDeps](workerType, deps) before factory.NewWorkerByType runs.
// Workers that never publish deps (e.g. those relying on per-package singletons
// like transport.ChildDeps or persistence.Store) receive the Go-native zero
// value of TDeps, preserving the pre-registry nil-forward contract.
// The workerType string is the canonical name used in config YAML and CSE storage.
//
// Workers registered via this function MUST use WorkerBase[TConfig, TStatus] and
// return w.WrapStatus(status) from CollectObservedState. Workers with custom
// ObservedState types or that need parent-injected extraDeps must use
// factory.RegisterWorkerType directly.
//
// Panics on field name collision or duplicate worker type (fail-fast at init time).
func Worker[TConfig any, TStatus any, TDeps any](
	workerType string,
	constructor func(deps.Identity, deps.FSMLogger, deps.StateReader, TDeps) (fsmv2.Worker, error),
) {
	if workerType == "" {
		panic("register.Worker: workerType must be non-empty")
	}

	if constructor == nil {
		panic("register.Worker: constructor must be non-nil")
	}

	// Step 1: Detect field name collisions between TStatus and framework fields.
	if err := fsmv2.DetectFieldCollisions[TStatus](); err != nil {
		panic(fmt.Sprintf("register.Worker(%q): %v", workerType, err))
	}

	// Step 2: Wrap constructor to match existing factory signature.
	// The closure consults the package-level deps registry (SetDeps/GetDeps)
	// at construction time. If nothing was published for this worker type,
	// GetDeps returns the Go-native zero value of TDeps, preserving the
	// pre-C8 nil-forward contract for singleton-backed workers
	// (transport/push/pull/persistence).
	wrappedFactory := func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, _ map[string]any) fsmv2.Worker {
		typedDeps := GetDeps[TDeps](workerType)
		w, err := constructor(id, logger, sr, typedDeps)
		if err != nil {
			panic(fmt.Sprintf("register.Worker(%q): constructor failed for %s: %v", workerType, id.String(), err))
		}
		if w == nil {
			panic(fmt.Sprintf("register.Worker(%q): constructor returned nil worker for %s", workerType, id.String()))
		}

		return w
	}

	// Step 3: Auto-generate supervisor factory with concrete wrapper types.
	supervisorFactory := func(cfg interface{}) interface{} {
		return supervisor.NewSupervisor[
			fsmv2.Observation[TStatus],
			*fsmv2.WrappedDesiredState[TConfig],
		](cfg.(supervisor.Config))
	}

	// Step 4: Register with factory (worker + supervisor).
	// Factory first: if it panics on duplicate, CSE never gets an orphaned entry.
	if err := factory.RegisterWorkerAndSupervisorFactoryByType(workerType, wrappedFactory, supervisorFactory); err != nil {
		panic(fmt.Sprintf("register.Worker(%q): %v", workerType, err))
	}

	// Step 5: Register with CSE TypeRegistry.
	observedType := reflect.TypeOf(fsmv2.Observation[TStatus]{})
	desiredType := reflect.TypeOf(fsmv2.WrappedDesiredState[TConfig]{})

	if err := storage.GlobalRegistry().RegisterWorkerType(workerType, observedType, desiredType); err != nil {
		panic(fmt.Sprintf("register.Worker(%q): %v", workerType, err))
	}
}
