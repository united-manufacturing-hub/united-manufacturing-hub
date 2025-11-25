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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
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
	return &ApplicationWorker{
		id:   id,
		name: name,
	}
}

// CollectObservedState returns the current observed state of the application supervisor.
// Since the application supervisor has minimal internal state, this mainly tracks
// the deployed desired state for comparison.
func (w *ApplicationWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	// Check context cancellation first.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return snapshot.ApplicationObservedState{
		ID:          w.id,
		CollectedAt: time.Now(),
		Name:        w.name,
		DeployedDesiredState: snapshot.ApplicationDesiredState{
			DesiredState: config.DesiredState{
				State: "running",
			},
			Name: w.name,
		},
	}, nil
}

// childrenConfig is the structure for parsing children from YAML.
type childrenConfig struct {
	Children []config.ChildSpec `yaml:"children"`
}

// DeriveDesiredState parses the YAML configuration to extract children specifications.
// This is the core of the passthrough pattern - the application doesn't need to know about
// child types, it just passes through the ChildSpec array from the config.
//
// Example YAML config:
//
//	children:
//	  - name: "child-1"
//	    workerType: "example-child"
//	    userSpec:
//	      config: |
//	        value: 10
//	  - name: "child-2"
//	    workerType: "example-child"
//	    userSpec:
//	      config: |
//	        value: 20
func (w *ApplicationWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	// Handle nil spec (used during initialization in AddWorker).
	if spec == nil {
		return config.DesiredState{
			State:         "running",
			ChildrenSpecs: nil,
		}, nil
	}

	// Get UserSpec from the spec interface.
	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return config.DesiredState{}, fmt.Errorf("invalid spec type: expected config.UserSpec, got %T", spec)
	}

	// Parse children from YAML config.
	var childrenCfg childrenConfig
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &childrenCfg); err != nil {
			return config.DesiredState{}, fmt.Errorf("failed to parse children config: %w", err)
		}
	}

	return config.DesiredState{
		State:         "running",
		ChildrenSpecs: childrenCfg.Children,
	}, nil
}

// GetInitialState returns the starting state for this application worker.
// Currently returns nil as the state machine is not yet implemented.
// TODO: Implement state machine for application supervisor.
func (w *ApplicationWorker) GetInitialState() fsmv2.State[any, any] {
	// Return nil for now - state machine will be implemented later.
	// The supervisor can handle nil initial state gracefully.
	return nil
}

// SupervisorConfig contains configuration for creating an application supervisor.
type SupervisorConfig struct {
	// ID is the unique identifier for the application worker.
	ID string

	// Name is a human-readable name for the application worker.
	Name string

	// Store is the triangular store for state persistence.
	Store storage.TriangularStoreInterface

	// Logger is the logger for supervisor operations.
	Logger *zap.SugaredLogger

	// TickInterval is how often the supervisor evaluates state transitions.
	// Defaults to 100ms if not set.
	TickInterval time.Duration

	// YAMLConfig is the raw YAML configuration containing children specifications.
	// This is passed to the application worker to parse and extract children.
	//
	// Example:
	//   children:
	//     - name: "child-1"
	//       workerType: "example-child"
	//       userSpec:
	//         config: |
	//           value: 10
	YAMLConfig string
}

// NewApplicationSupervisor creates a supervisor with an application worker already added.
// The application worker parses YAML config to dynamically discover child workers.
// Child workers are created automatically via reconcileChildren() based on
// the ChildrenSpecs in the config.
//
// This is the KEY API that encapsulates the application worker pattern. Users simply
// call this function with YAML config and get a fully initialized supervisor
// ready to Start().
//
// The passthrough pattern allows managing ANY registered worker type as children
// without hardcoding types in the application supervisor.
//
// Example usage:
//
//	sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
//	    ID:     "app-001",
//	    Name:   "My Application Supervisor",
//	    Store:  myStore,
//	    Logger: myLogger,
//	    YAMLConfig: `
//	children:
//	  - name: "worker-1"
//	    workerType: "example-child"
//	    userSpec:
//	      config: |
//	        value: 10
//	`,
//	})
//	if err != nil {
//	    return err
//	}
//	done := sup.Start(ctx)
func NewApplicationSupervisor(cfg SupervisorConfig) (*supervisor.Supervisor[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState], error) {
	tickInterval := cfg.TickInterval
	if tickInterval == 0 {
		tickInterval = 100 * time.Millisecond
	}

	// Derive worker type from observed state type.
	appWorkerType := storage.DeriveWorkerType[snapshot.ApplicationObservedState]()

	// Create supervisor.
	sup := supervisor.NewSupervisor[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState](supervisor.Config{
		WorkerType:   appWorkerType,
		Store:        cfg.Store,
		Logger:       cfg.Logger,
		TickInterval: tickInterval,
	})

	// Create application worker identity.
	appIdentity := fsmv2.Identity{
		ID:         cfg.ID,
		Name:       cfg.Name,
		WorkerType: appWorkerType,
	}

	// Create application worker.
	appWorker := NewApplicationWorker(cfg.ID, cfg.Name)

	// KEY PATTERN: Application workers need explicit AddWorker().
	// Child workers are created automatically via reconcileChildren().
	// This is the fundamental asymmetry that this setup helper encapsulates.
	err := sup.AddWorker(appIdentity, appWorker)
	if err != nil {
		return nil, err
	}

	return sup, nil
}

// init registers the application worker with the factory.
// This enables automatic creation via factory.NewWorkerByType() and factory.NewSupervisorByType().
//
// Registration happens automatically when this package is imported.
// The application worker can create any registered worker type as children.
func init() {
	// Register ApplicationWorker factory.
	// This allows creating application workers via factory.NewWorkerByType().
	if err := factory.RegisterFactory[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState](
		func(identity fsmv2.Identity) fsmv2.Worker {
			return NewApplicationWorker(identity.ID, identity.Name)
		}); err != nil {
		panic(fmt.Sprintf("failed to register ApplicationWorker factory: %v", err))
	}

	// Register ApplicationWorker supervisor factory.
	// This allows creating supervisors for application workers via factory.NewSupervisorByType().
	if err := factory.RegisterSupervisorFactory[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState](
		func(cfg interface{}) interface{} {
			supervisorCfg := cfg.(supervisor.Config)

			return supervisor.NewSupervisor[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState](supervisorCfg)
		}); err != nil {
		panic(fmt.Sprintf("failed to register ApplicationWorker supervisor factory: %v", err))
	}
}
