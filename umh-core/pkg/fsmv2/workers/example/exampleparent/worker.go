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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
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
	logger   *zap.SugaredLogger
	identity deps.Identity
}

// NewParentWorker creates a new example parent worker.
func NewParentWorker(
	identity deps.Identity,
	logger *zap.SugaredLogger,
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
func (w *ParentWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
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
		return &config.DesiredState{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
			ChildrenSpecs:    nil,
			OriginalUserSpec: nil,
		}, nil
	}

	parentSpec, err := config.ParseUserSpec[ParentUserSpec](spec)
	if err != nil {
		return nil, err
	}

	childrenCount := parentSpec.ChildrenCount

	if childrenCount == 0 {
		return &config.DesiredState{
			BaseDesiredState: config.BaseDesiredState{State: parentSpec.GetState()},
			ChildrenSpecs:    nil,
			OriginalUserSpec: spec,
		}, nil
	}

	childrenSpecs := make([]config.ChildSpec, childrenCount)
	childWorkerType := parentSpec.GetChildWorkerType()

	for i := range childrenCount {
		childVariables := config.VariableBundle{
			User: map[string]any{
				"DEVICE_ID": fmt.Sprintf("device-%d", i),
			},
		}

		var childConfig string
		if parentSpec.ChildConfig != "" {
			childConfig = parentSpec.ChildConfig
		} else {
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
			ChildStartStates: []string{"TryingToStart", "Running"},
		}
	}

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
		func(id deps.Identity, logger *zap.SugaredLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
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
