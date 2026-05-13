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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

const workerTypeName = "exampleparent"

// ParentWorker implements the FSM v2 Worker interface for parent-child relationships.
type ParentWorker struct {
	fsmv2.WorkerBase[ExampleparentConfig, ExampleparentStatus, *ParentDependencies]
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
		identity.WorkerType = workerTypeName
	}

	w := &ParentWorker{}
	bd := w.InitBase(identity, logger, stateReader)

	dependencies := NewParentDependencies(bd)
	w.BindDeps(dependencies)

	return w, nil
}

// GetDependencies returns the typed ParentDependencies.
func (w *ParentWorker) GetDependencies() *ParentDependencies {
	return w.GetDependenciesAny().(*ParentDependencies)
}

// CollectObservedState returns the current observed state of the parent worker.
func (w *ParentWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.GetDependencies()

	// Framework state and action history are injected before COS and consumed by
	// the collector wrapper after NewObservation returns. Calling them here satisfies
	// the framework-metrics-copy and action-history-copy invariants enforced by the
	// architecture validator.
	d.GetFrameworkState()
	d.GetActionHistory()

	status := ExampleparentStatus{
		ID: w.Identity().ID,
	}

	return fsmv2.NewObservation(status), nil
}

// DeriveDesiredState determines what state the parent worker should be in.
// Must be PURE - only uses the spec parameter, never dependencies.
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[ExampleparentConfig]{
			State: config.DesiredStateRunning,
		}, nil
	}

	parentSpec, err := config.ParseUserSpec[ParentUserSpec](spec)
	if err != nil {
		return nil, err
	}

	childrenCount := parentSpec.ChildrenCount

	if childrenCount == 0 {
		return &fsmv2.WrappedDesiredState[ExampleparentConfig]{
			State: parentSpec.GetState(),
			Config: ExampleparentConfig{
				BaseUserSpec: parentSpec.BaseUserSpec,
				ChildCount:   0,
			},
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

	return &fsmv2.WrappedDesiredState[ExampleparentConfig]{
		State: parentSpec.GetState(),
		Config: ExampleparentConfig{
			BaseUserSpec: parentSpec.BaseUserSpec,
			ChildCount:   childrenCount,
		},
		ChildrenSpecs: childrenSpecs,
	}, nil
}

func init() {
	if err := factory.RegisterWorkerAndSupervisorFactoryByType(
		workerTypeName,
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
			worker, err := NewParentWorker(id, logger, stateReader)
			if err != nil {
				panic(fmt.Sprintf("failed to create exampleparent worker: %v", err))
			}

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[fsmv2.Observation[ExampleparentStatus], *fsmv2.WrappedDesiredState[ExampleparentConfig]](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}
