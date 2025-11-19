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

package root

import (
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// SupervisorConfig contains configuration for creating a root supervisor.
type SupervisorConfig struct {
	// ID is the unique identifier for the root worker.
	ID string

	// Name is a human-readable name for the root worker.
	Name string

	// Store is the triangular store for state persistence.
	Store storage.TriangularStoreInterface

	// Logger is the logger for supervisor operations.
	Logger *zap.SugaredLogger

	// TickInterval is how often the supervisor evaluates state transitions.
	// Defaults to 100ms if not set.
	TickInterval time.Duration

	// YAMLConfig is the raw YAML configuration containing children specifications.
	// This is passed to the passthrough worker to parse and extract children.
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

// NewRootSupervisor creates a supervisor with a passthrough root worker already added.
// The root worker parses YAML config to dynamically discover child workers.
// Child workers are created automatically via reconcileChildren() based on
// the ChildrenSpecs in the config.
//
// This is the KEY API that encapsulates the root worker pattern. Users simply
// call this function with YAML config and get a fully initialized supervisor
// ready to Start().
//
// The passthrough pattern allows managing ANY registered worker type as children
// without hardcoding types in the root supervisor.
//
// Example usage:
//
//	sup, err := root.NewRootSupervisor(root.SupervisorConfig{
//	    ID:     "root-001",
//	    Name:   "My Root Supervisor",
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
func NewRootSupervisor(cfg SupervisorConfig) (*supervisor.Supervisor[PassthroughObservedState, *PassthroughDesiredState], error) {
	tickInterval := cfg.TickInterval
	if tickInterval == 0 {
		tickInterval = 100 * time.Millisecond
	}

	// Derive worker type from observed state type.
	rootWorkerType := storage.DeriveWorkerType[PassthroughObservedState]()

	// Create supervisor.
	sup := supervisor.NewSupervisor[PassthroughObservedState, *PassthroughDesiredState](supervisor.Config{
		WorkerType:   rootWorkerType,
		Store:        cfg.Store,
		Logger:       cfg.Logger,
		TickInterval: tickInterval,
	})

	// Create root worker identity.
	rootIdentity := fsmv2.Identity{
		ID:         cfg.ID,
		Name:       cfg.Name,
		WorkerType: rootWorkerType,
	}

	// Create passthrough root worker.
	rootWorker := NewPassthroughWorker(cfg.ID, cfg.Name)

	// KEY PATTERN: Root workers need explicit AddWorker().
	// Child workers are created automatically via reconcileChildren().
	// This is the fundamental asymmetry that this setup helper encapsulates.
	err := sup.AddWorker(rootIdentity, rootWorker)
	if err != nil {
		return nil, err
	}

	return sup, nil
}
