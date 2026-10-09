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

// Package exampleflow provides the example flow worker: the smallest
// WorkerBase worker whose template field a parent renders with the names it
// hands down.
package exampleflow

import (
	"context"
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// WorkerTypeName is the canonical worker type of the example flow worker.
const WorkerTypeName = "exampleflow"

// ExampleflowWorker implements the FSM v2 Worker interface for the example flow.
type ExampleflowWorker struct {
	fsmv2.WorkerBase[ExampleflowConfig, ExampleflowStatus, register.NoDeps]
}

// NewExampleflowWorker creates a new example flow worker.
func NewExampleflowWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*ExampleflowWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = WorkerTypeName
	}

	w := &ExampleflowWorker{}
	w.InitBase(identity, logger, stateReader)

	return w, nil
}

// CollectObservedState returns the current observed state of the example flow
// worker.
func (w *ExampleflowWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return fsmv2.NewObservation(ExampleflowStatus{ID: w.Identity().ID}), nil
}

func init() {
	register.Worker[ExampleflowConfig, ExampleflowStatus, register.NoDeps](WorkerTypeName,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, _ map[string]any) (fsmv2.Worker, error) {
			return NewExampleflowWorker(id, logger, sr)
		})
}

var _ fsmv2.Worker = (*ExampleflowWorker)(nil)
