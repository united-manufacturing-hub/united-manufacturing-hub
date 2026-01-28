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

// Package application provides a generic root supervisor for FSMv2.
// The application supervisor dynamically creates children based on YAML configuration,
// allowing any registered worker type to be instantiated as a child.
package application

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/state"
)

// ApplicationWorker is a generic application worker that parses YAML configuration
// to dynamically discover and create child workers. It doesn't hardcode child
// types - any registered worker type can be instantiated as a child.
//
// This implements the "passthrough pattern" where the application supervisor simply
// passes through ChildrenSpecs from the YAML config without knowing about
// specific child implementations.
type ApplicationWorker struct {
	id   string
	name string
}

// NewApplicationWorker creates a new application worker.
func NewApplicationWorker(id, name string) *ApplicationWorker {
	if id == "" || name == "" {
		return nil
	}

	return &ApplicationWorker{
		id:   id,
		name: name,
	}
}

// CollectObservedState returns the current observed state of the application supervisor.
func (w *ApplicationWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return snapshot.ApplicationObservedState{
		ID:          w.id,
		CollectedAt: time.Now(),
		Name:        w.name,
		ApplicationDesiredState: snapshot.ApplicationDesiredState{
			Name: w.name,
		},
	}, nil
}

// childrenConfig is the structure for parsing children from YAML.
type childrenConfig struct {
	Children []config.ChildSpec `yaml:"children"`
}

// DeriveDesiredState parses the YAML configuration to extract children specifications.
func (w *ApplicationWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &config.DesiredState{
			BaseDesiredState: config.BaseDesiredState{State: "running"},
			ChildrenSpecs:    nil,
		}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected config.UserSpec, got %T", spec)
	}

	var childrenCfg childrenConfig
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &childrenCfg); err != nil {
			return nil, fmt.Errorf("failed to parse children config: %w", err)
		}
	}

	return &config.DesiredState{
		BaseDesiredState: config.BaseDesiredState{State: "running"},
		ChildrenSpecs:    childrenCfg.Children,
	}, nil
}

// GetInitialState returns the starting state for this application worker.
func (w *ApplicationWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.RunningState{}
}

// GetDependenciesAny implements fsmv2.DependencyProvider.
func (w *ApplicationWorker) GetDependenciesAny() any {
	return nil
}

// SupervisorConfig contains configuration for creating an application supervisor.
type SupervisorConfig struct {
	Store              storage.TriangularStoreInterface
	Logger             *zap.SugaredLogger
	Dependencies       map[string]any // Injected into child workers via deps parameter
	ID                 string
	Name               string
	YAMLConfig         string        // Raw YAML containing children specifications
	TickInterval       time.Duration // Defaults to 100ms
	EnableTraceLogging bool          // Verbose lifecycle event logging for debugging
}

// NewApplicationSupervisor creates a supervisor with an application worker already added.
// Child workers are created automatically via reconcileChildren() based on ChildrenSpecs.
func NewApplicationSupervisor(cfg SupervisorConfig) (*supervisor.Supervisor[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState], error) {
	tickInterval := cfg.TickInterval
	if tickInterval == 0 {
		tickInterval = 100 * time.Millisecond
	}

	appWorkerType, err := storage.DeriveWorkerType[snapshot.ApplicationObservedState]()
	if err != nil {
		return nil, fmt.Errorf("failed to derive worker type: %w", err)
	}

	sup := supervisor.NewSupervisor[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState](supervisor.Config{
		WorkerType:         appWorkerType,
		Store:              cfg.Store,
		Logger:             cfg.Logger,
		TickInterval:       tickInterval,
		UserSpec:           config.UserSpec{Config: cfg.YAMLConfig},
		EnableTraceLogging: cfg.EnableTraceLogging,
		Dependencies:       cfg.Dependencies,
	})

	appIdentity := deps.Identity{
		ID:            cfg.ID,
		Name:          cfg.Name,
		WorkerType:    appWorkerType,
		HierarchyPath: fmt.Sprintf("%s(%s)", cfg.ID, appWorkerType),
	}

	appWorker := NewApplicationWorker(cfg.ID, cfg.Name)

	// Application workers need explicit AddWorker; child workers are created via reconcileChildren
	err = sup.AddWorker(appIdentity, appWorker)
	if err != nil {
		return nil, err
	}

	return sup, nil
}

// init registers the application worker with the factory for automatic creation via factory.NewWorkerByType().
func init() {
	if err := factory.RegisterWorkerType[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState](
		func(id deps.Identity, _ *zap.SugaredLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
			return NewApplicationWorker(id.ID, id.Name)
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(fmt.Sprintf("failed to register ApplicationWorker: %v", err))
	}
}
