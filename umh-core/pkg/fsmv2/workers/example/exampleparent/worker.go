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

package exampleparent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
)

// ParentWorker implements the FSM v2 Worker interface for parent-child relationships.
type ParentWorker struct {
	*helpers.BaseWorker[*ParentDependencies]
	identity fsmv2.Identity
	logger   *zap.SugaredLogger
}

// NewParentWorker creates a new example parent worker.
func NewParentWorker(
	identity fsmv2.Identity,
	logger *zap.SugaredLogger,
	stateReader fsmv2.StateReader,
) (*ParentWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	// Set workerType if not already set (derive from snapshot type)
	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.ExampleparentObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}
		identity.WorkerType = workerType
	}
	dependencies := NewParentDependencies(logger, stateReader, identity)

	return &ParentWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
	}, nil
}

// CollectObservedState returns the current observed state of the parent worker.
func (w *ParentWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	deps := w.GetDependencies()
	tracker := deps.GetStateTracker()

	// Load previous observed state to detect state changes
	stateReader := deps.GetStateReader()
	if stateReader != nil {
		var previousObserved snapshot.ExampleparentObservedState
		err := stateReader.LoadObservedTyped(ctx, w.identity.WorkerType, w.identity.ID, &previousObserved)
		if err == nil && previousObserved.State != "" {
			// Record state change - resets timer if state changed
			tracker.RecordStateChange(previousObserved.State)
		}
		// If LoadObservedTyped fails or State is empty, this is the first tick - continue without calling RecordStateChange
	}

	observed := snapshot.ExampleparentObservedState{
		ID:             w.identity.ID,
		CollectedAt:    time.Now(),
		StateEnteredAt: tracker.GetStateEnteredAt(),
		Elapsed:        tracker.Elapsed(), // Pre-computed using Clock (mockable in tests)
	}

	return observed, nil
}

// DeriveDesiredState determines what state the parent worker should be in.
// This method must be PURE - it only uses the spec parameter, never dependencies.
// Note: spec is interface{} for flexibility across different worker types (each has its own UserSpec).
//
// Uses ParseUserSpec helper for type-safe parsing.
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	// Handle nil spec - return default state with no children
	if spec == nil {
		return config.DesiredState{
			State:            config.DesiredStateRunning,
			ChildrenSpecs:    nil,
			OriginalUserSpec: nil,
		}, nil
	}

	// Use ParseUserSpec helper for type-safe parsing
	parentSpec, err := config.ParseUserSpec[ParentUserSpec](spec)
	if err != nil {
		return config.DesiredState{}, err
	}

	childrenCount := parentSpec.ChildrenCount

	if childrenCount == 0 {
		return config.DesiredState{
			State:            parentSpec.GetState(),
			ChildrenSpecs:    nil,
			OriginalUserSpec: spec,
		}, nil
	}

	// Create child specs using the new ChildStartStates approach
	childrenSpecs := make([]config.ChildSpec, childrenCount)
	childWorkerType := parentSpec.GetChildWorkerType()
	for i := range childrenCount {
		// Each child gets its own DEVICE_ID variable.
		// Parent's variables (IP, PORT, etc.) will be merged in by the supervisor
		// via config.Merge() in reconcileChildren(). Child variables override parent.
		childVariables := config.VariableBundle{
			User: map[string]any{
				"DEVICE_ID": fmt.Sprintf("device-%d", i),
			},
		}

		// Determine child config: use explicit ChildConfig if provided, otherwise use default template.
		// Different child worker types to receive appropriate configuration.
		var childConfig string
		if parentSpec.ChildConfig != "" {
			// Use the explicitly provided child config (e.g., for examplefailing workers)
			childConfig = parentSpec.ChildConfig
		} else {
			// Build default config template that uses variables from both parent and child.
			// Parent provides: IP, PORT (defined in parent's variables)
			// Child provides: DEVICE_ID (defined above in childVariables)
			// The supervisor merges these so children have access to all three.
			childConfig = `address: {{ .IP }}:{{ .PORT }}
device: {{ .DEVICE_ID }}`
		}

		childrenSpecs[i] = config.ChildSpec{
			Name:       fmt.Sprintf("child-%d", i),
			WorkerType: childWorkerType,
			UserSpec: config.UserSpec{
				Config:    childConfig,
				Variables: childVariables,
			},
			// New approach: list parent states where children should run
			ChildStartStates: []string{"TryingToStart", "Running"},
		}
	}

	return config.DesiredState{
		State:            parentSpec.GetState(),
		ChildrenSpecs:    childrenSpecs,
		OriginalUserSpec: spec,
	}, nil
}

// GetInitialState returns the state the FSM should start in.
func (w *ParentWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	// Register both worker and supervisor factories atomically.
	// The worker type is derived from ExampleparentObservedState, ensuring consistency.
	if err := factory.RegisterWorkerType[snapshot.ExampleparentObservedState, *snapshot.ExampleparentDesiredState](
		func(id fsmv2.Identity, logger *zap.SugaredLogger, stateReader fsmv2.StateReader) fsmv2.Worker {
			worker, _ := NewParentWorker(id, logger, stateReader)
			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.ExampleparentObservedState, *snapshot.ExampleparentDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}
