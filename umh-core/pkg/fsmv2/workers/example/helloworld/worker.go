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

// Package hello_world provides a minimal FSMv2 worker example.
//
// This is the simplest possible worker implementation demonstrating:
//   - The 3 required Worker interface methods
//   - Factory registration pattern
//   - State machine transitions
//   - Action execution
//
// Use this as a template when creating new workers. See README.md for details.
//
// NAMING CONVENTION: The package name uses underscore (hello_world) but the
// folder name is "helloworld". The type prefix must be "Helloworld" (one capital)
// to derive correctly as worker type "helloworld".
package hello_world

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/state"
)

// HelloworldWorker implements the FSMv2 Worker interface.
//
// Workers are the core building blocks of FSMv2. Each worker:
//   - Collects its current observed state (CollectObservedState)
//   - Derives what state it should be in (DeriveDesiredState)
//   - Provides an initial state (GetInitialState)
//
// The supervisor drives the state machine by calling these methods each tick.
type HelloworldWorker struct {
	*helpers.BaseWorker[*HelloworldDependencies]
	logger   *zap.SugaredLogger
	identity deps.Identity
}

// NewHelloworldWorker creates a new helloworld worker.
func NewHelloworldWorker(
	identity deps.Identity,
	logger *zap.SugaredLogger,
	stateReader deps.StateReader,
) (*HelloworldWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	// Derive worker type from snapshot type if not set
	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.HelloworldObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}

		identity.WorkerType = workerType
	}

	dependencies := NewHelloworldDependencies(logger, stateReader, identity)

	return &HelloworldWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
	}, nil
}

// CollectObservedState returns the current observed state.
//
// This method is called by the supervisor each tick to collect what the
// worker currently observes about itself. The observed state is then
// passed to the current FSM state's Next() method.
//
// IMPLEMENTATION PATTERN:
//   - Check context cancellation first
//   - Read state from dependencies (what actions have set)
//   - Copy framework metrics if available
//   - Return the observed state
func (w *HelloworldWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	deps := w.GetDependencies()

	observed := snapshot.HelloworldObservedState{
		CollectedAt: time.Now(),
		HelloSaid:   deps.HasSaidHello(),
	}

	// Copy framework metrics (time in state, transition count, etc.)
	if fm := deps.GetFrameworkState(); fm != nil {
		observed.Metrics.Framework = *fm
	}

	// Include action history for debugging
	observed.LastActionResults = deps.GetActionHistory()

	return observed, nil
}

// DeriveDesiredState determines what state the worker should be in.
//
// This method parses user configuration (from scenarios or external config)
// and returns the desired state. The supervisor compares this with observed
// state to drive transitions.
//
// For simple workers, this just returns "running" as the desired state.
func (w *HelloworldWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		// Default: desired state is running
		// Return the registered type (*snapshot.HelloworldDesiredState) to avoid type assertion failures
		return &snapshot.HelloworldDesiredState{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
		}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	// Render any template variables in the config
	renderedConfig, err := config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	renderedSpec := config.UserSpec{
		Config:    renderedConfig,
		Variables: userSpec.Variables,
	}

	// Parse into typed config and derive desired state
	desired, err := config.DeriveLeafState[HelloworldUserSpec](renderedSpec)
	if err != nil {
		return nil, err
	}

	// Return the registered type (*snapshot.HelloworldDesiredState) to avoid type assertion failures
	return &snapshot.HelloworldDesiredState{
		BaseDesiredState: desired.BaseDesiredState,
	}, nil
}

// GetInitialState returns the state the FSM should start in.
//
// All workers start in some initial state. For most workers this is
// a "stopped" state that waits for the desired state to request running.
func (w *HelloworldWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

// init registers the worker with the factory.
//
// CRITICAL: Without this init() function, the worker won't be discoverable
// by the supervisor. The factory uses the type parameters to:
//   - Match incoming worker specs to the correct worker type
//   - Create worker instances with proper typing
//   - Create typed supervisors for the worker
//
// The first function creates worker instances.
// The second function creates supervisor instances.
func init() {
	if err := factory.RegisterWorkerType[snapshot.HelloworldObservedState, *snapshot.HelloworldDesiredState](
		// Worker factory: creates worker instances
		func(id deps.Identity, logger *zap.SugaredLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
			worker, err := NewHelloworldWorker(id, logger, stateReader)
			if err != nil {
				if logger != nil {
					logger.Errorw("helloworld_worker_creation_failed", "error", err)
				}

				return nil
			}

			return worker
		},
		// Supervisor factory: creates supervisor instances
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.HelloworldObservedState, *snapshot.HelloworldDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}
