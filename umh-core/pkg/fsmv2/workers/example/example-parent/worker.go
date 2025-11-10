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
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/state"
)

const WorkerType = "example-parent"

// ParentWorker implements the FSM v2 Worker interface for parent-child relationships
type ParentWorker struct {
	*fsmv2.BaseWorker[*ParentDependencies]
	identity fsmv2.Identity
	logger   *zap.SugaredLogger
}

// NewParentWorker creates a new example parent worker
func NewParentWorker(
	id string,
	name string,
	configLoader ConfigLoader,
	logger *zap.SugaredLogger,
) *ParentWorker {
	dependencies := NewParentDependencies(configLoader, logger)

	return &ParentWorker{
		BaseWorker: fsmv2.NewBaseWorker(dependencies),
		identity: fsmv2.Identity{
			ID:         id,
			Name:       name,
			WorkerType: WorkerType,
		},
		logger: logger,
	}
}

// CollectObservedState returns the current observed state of the parent worker
func (w *ParentWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	observed := snapshot.ParentObservedState{
		ID:          w.identity.ID,
		CollectedAt: time.Now(),
	}

	return observed, nil
}

// DeriveDesiredState determines what state the parent worker should be in
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
	config, err := w.GetDependencies().GetConfigLoader().LoadConfig()
	if err != nil {
		return fsmv2types.DesiredState{}, err
	}

	childrenCount, ok := config["children_count"].(int)
	if !ok || childrenCount == 0 {
		return fsmv2types.DesiredState{
			State:         "running",
			ChildrenSpecs: nil,
		}, nil
	}

	childrenSpecs := make([]fsmv2types.ChildSpec, childrenCount)
	for i := 0; i < childrenCount; i++ {
		childrenSpecs[i] = fsmv2types.ChildSpec{
			Name:       fmt.Sprintf("child-%d", i),
			WorkerType: "example-child",
			UserSpec:   fsmv2types.UserSpec{},
		}
	}

	return fsmv2types.DesiredState{
		State:         "running",
		ChildrenSpecs: childrenSpecs,
	}, nil
}

// GetInitialState returns the state the FSM should start in
func (w *ParentWorker) GetInitialState() fsmv2.State {
	return state.NewStoppedState(w.GetDependencies())
}
