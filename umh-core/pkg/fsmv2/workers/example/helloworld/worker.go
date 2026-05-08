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
	"os"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"

	"gopkg.in/yaml.v3"
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
	workerDeps *HelloworldDependencies
	logger     deps.FSMLogger
	identity   deps.Identity
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

	baseDeps := deps.NewBaseDependencies(logger, stateReader, identity)
	workerDeps := NewHelloworldDependencies(baseDeps)

	return &HelloworldWorker{
		workerDeps: workerDeps,
		identity:   identity,
		logger:     logger,
	}, nil
}

// CollectObservedState returns the current observed state.
//
// Uses fsmv2.NewObservation which signals the supervisor's collector to perform
// post-COS wrapping (CollectedAt, framework metrics, action history).
func (w *HelloworldWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
	default:
	}

	cfg := fsmv2.ExtractConfig[HelloworldConfig](desired)

	status := HelloworldStatus{
		HelloSaid: w.workerDeps.HasSaidHello(),
		Mood:      readMoodFile(cfg.MoodFilePath),
	}

	return fsmv2.NewObservation(status), nil
}

// DeriveDesiredState determines what state the worker should be in.
//
// Parses user configuration into HelloworldConfig and wraps it in
// WrappedDesiredState for typed access in state files via ConvertWorkerSnapshot.
func (w *HelloworldWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[HelloworldConfig]{
			State: config.DesiredStateRunning,
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

	var cfg HelloworldConfig
	if renderedConfig != "" {
		if err := yaml.Unmarshal([]byte(renderedConfig), &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse helloworld config: %w", err)
		}
	}

	state := cfg.GetState()
	if state == "" {
		state = config.DesiredStateRunning
	}

	return &fsmv2.WrappedDesiredState[HelloworldConfig]{
		State: state,
		Config:           cfg,
	}, nil
}

// GetInitialState returns the state the FSM should start in.
// Uses the initial state registry populated by the state package's init() function.
// The caller must ensure the state package is imported (via blank import in main or test).
func (w *HelloworldWorker) GetInitialState() fsmv2.State[any, any] {
	return fsmv2.LookupInitialState("helloworld")
}

// GetDependenciesAny returns the worker's dependencies for action execution.
// Implements fsmv2.DependencyProvider.
func (w *HelloworldWorker) GetDependenciesAny() any {
	return w.workerDeps
}

// Actions returns the available actions for this worker.
// Implements fsmv2.ActionProvider.
func (w *HelloworldWorker) Actions() map[string]fsmv2.Action[any] {
	return map[string]fsmv2.Action[any]{
		SayHelloActionName: fsmv2.SimpleAction[*HelloworldDependencies](SayHelloActionName, SayHello),
	}
}

// readMoodFile reads the mood from a file path. Returns empty string on error or empty path.
func readMoodFile(path string) string {
	if path == "" {
		return ""
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// init registers the worker with the factory using an explicit worker type name.
// Uses RegisterWorkerAndSupervisorFactoryByType since Observation[T] generics
// cannot be used with DeriveWorkerType (which relies on type name conventions).
func init() {
	if err := factory.RegisterWorkerAndSupervisorFactoryByType(
		"helloworld",
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
			return supervisor.NewSupervisor[fsmv2.Observation[HelloworldStatus], *fsmv2.WrappedDesiredState[HelloworldConfig]](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}

// ensure HelloworldWorker implements Worker interfaces (compile-time check).
var _ fsmv2.Worker = (*HelloworldWorker)(nil)
