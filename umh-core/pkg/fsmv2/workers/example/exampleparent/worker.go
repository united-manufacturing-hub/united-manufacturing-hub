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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
)

// ParentWorker implements the FSM v2 Worker interface for parent-child relationships.
type ParentWorker struct {
	*helpers.BaseWorker[*ParentDependencies]
	logger   deps.FSMLogger
	identity deps.Identity
}

// NewParentWorker creates a new example parent worker.
func NewParentWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*ParentWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

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
func (w *ParentWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	deps := w.GetDependencies()
	tracker := deps.GetStateTracker()

	stateReader := deps.GetStateReader()
	if stateReader != nil {
		var previousObserved snapshot.ExampleparentObservedState

		err := stateReader.LoadObservedTyped(ctx, w.identity.WorkerType, w.identity.ID, &previousObserved)
		if err == nil && previousObserved.State != "" {
			// Record state change - resets timer if state changed
			tracker.RecordStateChange(previousObserved.State)
		}
	}

	observed := snapshot.ExampleparentObservedState{
		ID:          w.identity.ID,
		CollectedAt: time.Now(),
	}

	if fm := deps.GetFrameworkState(); fm != nil {
		observed.Metrics.Framework = *fm
	}

	observed.LastActionResults = deps.GetActionHistory()

	return observed, nil
}

// DeriveDesiredState determines what state the parent worker should be in.
// Must be PURE - only uses the spec parameter, never dependencies.
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		// Byte-equivalent with canonical RenderChildren(nil) = []ChildSpec{}.
		// Pre-PR2-boundary this branch returned nil; PR2 boundary cleanup
		// flipped to []ChildSpec{} so the DDS path emits the same authoritative
		// "zero children right now" sentinel as the canonical path. Closes the
		// G11 DDS-vs-canonical divergence (PR2 boundary DA finding).
		return &config.DesiredState{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
			ChildrenSpecs:    []config.ChildSpec{},
			OriginalUserSpec: nil,
		}, nil
	}

	parentSpec, err := config.ParseUserSpec[ParentUserSpec](spec)
	if err != nil {
		return nil, err
	}

	if parentSpec.ChildrenCount == 0 {
		// Same byte-equivalence with canonical RenderChildren as the spec==nil
		// branch above. If a user shrinks ChildrenCount from N>0 to 0, the
		// supervisor needs an explicit non-nil [] sentinel to despawn existing
		// children authoritatively; nil here would re-route through fallback
		// and leak the prior tick's children-set on parents whose mirror also
		// returns nil (option-a, exampleparent).
		return &config.DesiredState{
			BaseDesiredState: config.BaseDesiredState{State: parentSpec.GetState()},
			ChildrenSpecs:    []config.ChildSpec{},
			OriginalUserSpec: spec,
		}, nil
	}

	// RenderChildren (children.go) is the canonical children-set emitter and
	// the single source of truth for ChildSpec construction; called here so
	// the legacy DDS path and the P2.2 renderChildren-in-state.Next migration
	// remain bit-for-bit identical.
	childrenSpecs := RenderChildren(&parentSpec)

	return &config.DesiredState{
		BaseDesiredState: config.BaseDesiredState{State: parentSpec.GetState()},
		ChildrenSpecs:    childrenSpecs,
		OriginalUserSpec: spec,
	}, nil
}

// GetInitialState returns the state the FSM should start in.
func (w *ParentWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	if err := factory.RegisterWorkerType[snapshot.ExampleparentObservedState, *snapshot.ExampleparentDesiredState](
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
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
