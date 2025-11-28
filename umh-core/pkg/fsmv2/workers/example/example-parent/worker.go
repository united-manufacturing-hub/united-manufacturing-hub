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

package example_parent

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/state"
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
) (*ParentWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	// Set workerType if not already set (derive from snapshot type)
	if identity.WorkerType == "" {
		identity.WorkerType = storage.DeriveWorkerType[snapshot.ParentObservedState]()
	}
	dependencies := NewParentDependencies(logger, identity)

	return &ParentWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger: logger,
	}, nil
}

// CollectObservedState returns the current observed state of the parent worker.
func (w *ParentWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	observed := snapshot.ParentObservedState{
		ID:          w.identity.ID,
		CollectedAt: time.Now(),
	}

	return observed, nil
}

// DeriveDesiredState determines what state the parent worker should be in
// This method must be PURE - it only uses the spec parameter, never dependencies.
// TODO: check why spec interface{} and not spec fsmv2types.UserSpec
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
	var childrenCount int

	// Handle nil spec - return default state with no children
	if spec == nil {
		return fsmv2types.DesiredState{
			State:         "running",
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
			State:         "running",
			ChildrenSpecs: nil,
		}, nil
	}

	childrenSpecs := make([]fsmv2types.ChildSpec, childrenCount)
	for i := range childrenCount {
		childrenSpecs[i] = fsmv2types.ChildSpec{
			Name:       fmt.Sprintf("child-%d", i),
			WorkerType: "child",
			UserSpec:   fsmv2types.UserSpec{},
		}
	}

	return fsmv2types.DesiredState{
		State:         "running",
		ChildrenSpecs: childrenSpecs,
	}, nil
}

// GetInitialState returns the state the FSM should start in.
func (w *ParentWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	// Register supervisor factory for creating child supervisors
	_ = factory.RegisterSupervisorFactory[snapshot.ParentObservedState, *snapshot.ParentDesiredState](
		func(cfg interface{}) interface{} {
			supervisorCfg := cfg.(supervisor.Config)

			return supervisor.NewSupervisor[snapshot.ParentObservedState, *snapshot.ParentDesiredState](supervisorCfg)
		})

	// Register worker factory for ApplicationWorker to create parent workers via YAML config
	_ = factory.RegisterFactoryByType("parent", func(identity fsmv2.Identity, logger *zap.SugaredLogger) fsmv2.Worker {
		worker, _ := NewParentWorker(identity, logger)
		return worker
	})
}
