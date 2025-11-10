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

package example_child

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/state"
)

const WorkerType = "example-child"

// ChildWorker implements the FSM v2 Worker interface for resource management
type ChildWorker struct {
	*fsmv2.BaseWorker[*ChildDependencies]
	identity   fsmv2.Identity
	logger     *zap.SugaredLogger
	connection Connection
}

// NewChildWorker creates a new example child worker
func NewChildWorker(
	id string,
	name string,
	connectionPool ConnectionPool,
	logger *zap.SugaredLogger,
) *ChildWorker {
	dependencies := NewChildDependencies(connectionPool, logger)

	return &ChildWorker{
		BaseWorker: fsmv2.NewBaseWorker(dependencies),
		identity: fsmv2.Identity{
			ID:         id,
			Name:       name,
			WorkerType: WorkerType,
		},
		logger: logger,
	}
}

// CollectObservedState returns the current observed state of the child worker
func (w *ChildWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	observed := snapshot.ChildObservedState{
		ID:               w.identity.ID,
		CollectedAt:      time.Now(),
		ConnectionStatus: w.getConnectionStatus(),
		ConnectionHealth: w.getConnectionHealth(),
	}

	return observed, nil
}

// DeriveDesiredState determines what state the child worker should be in
func (w *ChildWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
	return fsmv2types.DesiredState{
		State:         "connected",
		ChildrenSpecs: nil,
	}, nil
}

// GetInitialState returns the state the FSM should start in
func (w *ChildWorker) GetInitialState() fsmv2.State {
	return state.NewStoppedState(w.GetDependencies())
}

func (w *ChildWorker) getConnectionStatus() string {
	if w.connection != nil {
		return "connected"
	}
	return "disconnected"
}

func (w *ChildWorker) getConnectionHealth() string {
	if w.connection == nil {
		return "no connection"
	}
	return "healthy"
}
