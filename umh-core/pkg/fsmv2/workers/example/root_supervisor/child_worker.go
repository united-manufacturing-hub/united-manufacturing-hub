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

package root_supervisor

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/root_supervisor/state"
)

// ChildWorkerDependencies provides access to tools needed by child worker actions.
type ChildWorkerDependencies struct {
	*fsmv2.BaseDependencies
}

// NewChildWorkerDependencies creates new dependencies for the child worker.
func NewChildWorkerDependencies(logger *zap.SugaredLogger) *ChildWorkerDependencies {
	return &ChildWorkerDependencies{
		BaseDependencies: fsmv2.NewBaseDependencies(logger),
	}
}

// ChildWorker implements the FSM v2 Worker interface for a leaf worker.
// This worker is a leaf node in the supervision tree - it has no children of its own.
//
// This example demonstrates how to create a child worker type that can be
// dynamically instantiated by the generic root package based on YAML configuration.
type ChildWorker struct {
	*fsmv2.BaseWorker[*ChildWorkerDependencies]
	identity fsmv2.Identity
	logger   *zap.SugaredLogger
}

// NewChildWorker creates a new child worker (leaf node).
func NewChildWorker(
	id string,
	name string,
	logger *zap.SugaredLogger,
) *ChildWorker {
	dependencies := NewChildWorkerDependencies(logger)

	return &ChildWorker{
		BaseWorker: fsmv2.NewBaseWorker(dependencies),
		identity: fsmv2.Identity{
			ID:         id,
			Name:       name,
			WorkerType: storage.DeriveWorkerType[ChildObservedState](),
		},
		logger: logger,
	}
}

// CollectObservedState returns the current observed state of the child worker.
// This method monitors the actual system state and is called in a separate goroutine.
func (w *ChildWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	observed := ChildObservedState{
		ID:               w.identity.ID,
		CollectedAt:      time.Now(),
		ConnectionStatus: "connected",
		ConnectionHealth: "healthy",
	}

	return observed, nil
}

// DeriveDesiredState determines what state the child worker should be in.
// This method transforms user configuration into desired state.
//
// KEY CHARACTERISTIC: Child workers return nil ChildrenSpecs
// because they are leaf nodes in the supervision tree.
//
// This method must be PURE - it only uses the spec parameter, never dependencies.
func (w *ChildWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
	// Handle nil spec (used during initialization in AddWorker)
	if spec == nil {
		return fsmv2types.DesiredState{
			State:         "connected",
			ChildrenSpecs: nil, // Leaf node - no children
		}, nil
	}

	_, ok := spec.(fsmv2types.UserSpec)
	if !ok {
		return fsmv2types.DesiredState{}, fmt.Errorf("invalid spec type: expected fsmv2types.UserSpec, got %T", spec)
	}

	// Child workers don't have children - they are leaf nodes
	return fsmv2types.DesiredState{
		State:         "connected",
		ChildrenSpecs: nil, // KEY: Leaf node returns nil ChildrenSpecs
	}, nil
}

// GetInitialState returns the state the FSM should start in.
func (w *ChildWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.ChildStoppedState{}
}
