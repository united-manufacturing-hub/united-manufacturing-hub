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
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
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
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
	var childrenCount int

	// Handle nil spec - return default state with no children
	if spec == nil {
		return fsmv2types.DesiredState{
			State:         fsmv2types.DesiredStateRunning, // Default when no config provided
			ChildrenSpecs: nil,
		}, nil
	}

	userSpec, ok := spec.(fsmv2types.UserSpec)
	if !ok {
		return fsmv2types.DesiredState{}, fmt.Errorf("invalid spec type: expected fsmv2types.UserSpec, got %T", spec)
	}

	var parentSpec ParentUserSpec
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &parentSpec); err != nil {
			return fsmv2types.DesiredState{}, fmt.Errorf("failed to parse parent spec: %w", err)
		}
	}

	childrenCount = parentSpec.ChildrenCount

	if childrenCount == 0 {
		return fsmv2types.DesiredState{
			State:         parentSpec.GetState(), // Uses BaseUserSpec.GetState() with default "running"
			ChildrenSpecs: nil,
		}, nil
	}

	childrenSpecs := make([]fsmv2types.ChildSpec, childrenCount)
	for i := range childrenCount {
		childrenSpecs[i] = fsmv2types.ChildSpec{
			Name:       fmt.Sprintf("child-%d", i),
			WorkerType: "examplechild",
			UserSpec:   fsmv2types.UserSpec{},
			StateMapping: map[string]string{
				"TryingToStart": "running",
				"Running":       "running",
				"TryingToStop":  "stopped",
				"Stopped":       "stopped",
			},
		}
	}

	return fsmv2types.DesiredState{
		State:         parentSpec.GetState(), // Uses BaseUserSpec.GetState() with default "running"
		ChildrenSpecs: childrenSpecs,
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
