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

// Package hello_world implements a minimal FSMv2 worker that demonstrates
// the three required Worker interface methods, factory registration,
// state transitions, and action execution.
//
// Use this as a template when creating new workers. See README.md for the step-by-step guide.
//
// Naming convention: The package name uses underscore (hello_world) but the
// folder name is "helloworld". The type prefix must be "Helloworld" (one capital)
// to derive correctly as worker type "helloworld".
package hello_world

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

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
// A worker is the core building block of FSMv2. Each worker:
//   - Collects its current observed state (CollectObservedState)
//   - Derives what state it should be in (DeriveDesiredState)
//   - Provides an initial state (GetInitialState)
//
// The supervisor drives the state machine by calling these methods each tick.
type HelloworldWorker struct {
	*helpers.BaseWorker[*HelloworldDependencies]
	logger   deps.FSMLogger
	identity deps.Identity
}

// NewHelloworldWorker creates a new helloworld worker.
func NewHelloworldWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
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

// CollectObservedState collects and returns the current observed state.
//
// This method is called by the supervisor each tick to collect what the
// worker currently observes about itself. The observed state is then
// passed to the current FSM state's Next() method.
func (w *HelloworldWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
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

	// Read the mood file at DesiredState.MoodFilePath.
	// This is blocking disk I/O, which is why CollectObservedState runs in a
	// background goroutine. The supervisor ensures slow reads never block the tick loop.
	if d, ok := desired.(*snapshot.HelloworldDesiredState); ok && d.MoodFilePath != "" {
		if data, err := os.ReadFile(d.MoodFilePath); err == nil {
			observed.Mood = strings.TrimSpace(string(data))
		}
	}

	return observed, nil
}

// DeriveDesiredState parses UserSpec.Config YAML into typed HelloworldDesiredState.
//
// Uses config.ParseUserSpec instead of config.DeriveLeafState.
// DeriveLeafState returns a generic config.DesiredState that drops
// worker-specific fields like MoodFilePath. ParseUserSpec returns the
// full typed struct so custom fields flow through to CollectObservedState.
func (w *HelloworldWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &snapshot.HelloworldDesiredState{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
		}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	renderedConfig, err := config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	renderedSpec := config.UserSpec{Config: renderedConfig, Variables: userSpec.Variables}
	hwSpec, err := config.ParseUserSpec[HelloworldUserSpec](renderedSpec)
	if err != nil {
		return nil, fmt.Errorf("config parse failed: %w", err)
	}

	return &snapshot.HelloworldDesiredState{
		BaseDesiredState: config.BaseDesiredState{State: hwSpec.GetState()},
		MoodFilePath:     hwSpec.MoodFilePath,
	}, nil
}

// GetInitialState returns the state the FSM should start in.
//
// All workers start in StoppedState. It waits for the desired state
// to request running before transitioning.
func (w *HelloworldWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

// init registers the worker with the factory so the supervisor can discover it.
//
// The factory uses the type parameters to:
//   - Match incoming worker specs to the correct worker type
//   - Create worker instances with proper typing
//   - Create typed supervisors for the worker
//
// The first function creates worker instances.
// The second function creates supervisor instances.
func init() {
	if err := factory.RegisterWorkerType[snapshot.HelloworldObservedState, *snapshot.HelloworldDesiredState](
		// Worker factory: creates worker instances
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
			worker, err := NewHelloworldWorker(id, logger, stateReader)
			if err != nil {
				if logger != nil {
					logger.SentryError(deps.FeatureExamples, id.HierarchyPath, err, "helloworld_worker_creation_failed")
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
