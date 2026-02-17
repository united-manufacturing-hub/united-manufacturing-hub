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

package push

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/state"

	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

var _ fsmv2.Worker = (*PushWorker)(nil)

type PushWorker struct {
	*helpers.BaseWorker[*PushDependencies]
	logger   deps.FSMLogger
	identity deps.Identity
}

func NewPushWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	parentDeps *transport_pkg.TransportDependencies,
) (*PushWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.PushObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}

		identity.WorkerType = workerType
	}

	dependencies, err := NewPushDependencies(parentDeps, identity, logger, stateReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create push dependencies: %w", err)
	}

	return &PushWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
	}, nil
}

func (w *PushWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.GetDependencies()

	observed := snapshot.PushObservedState{
		CollectedAt:   time.Now(),
		HasTransport:  d.GetTransport() != nil,
		HasValidToken: d.IsTokenValid(),
	}

	observed.ConsecutiveErrors = d.GetConsecutiveErrors()
	observed.PendingMessageCount = d.PendingMessageCount()
	observed.LastErrorType = d.GetLastErrorType()
	observed.LastRetryAfter = d.GetLastRetryAfter()
	observed.DegradedEnteredAt = d.GetDegradedEnteredAt()
	observed.LastErrorAt = d.GetLastErrorAt()

	if fm := d.GetFrameworkState(); fm != nil {
		observed.Metrics.Framework = *fm
	}

	observed.LastActionResults = d.GetActionHistory()

	return observed, nil
}

func (w *PushWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &config.DesiredState{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
			OriginalUserSpec: nil,
		}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	renderedConfig, err := config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	renderedSpec := config.UserSpec{
		Config:    renderedConfig,
		Variables: userSpec.Variables,
	}

	desired, err := config.DeriveLeafState[PushUserSpec](renderedSpec)
	if err != nil {
		return nil, err
	}

	return &desired, nil
}

func (w *PushWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	if err := factory.RegisterWorkerType[snapshot.PushObservedState, *snapshot.PushDesiredState](
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, extraDeps map[string]any) fsmv2.Worker {
			parentDepsRaw, ok := extraDeps["transport_deps"]
			if !ok {
				panic("push worker requires transport_deps in extraDeps")
			}

			parentDeps, ok := parentDepsRaw.(*transport_pkg.TransportDependencies)
			if !ok {
				panic("transport_deps must be *TransportDependencies")
			}

			worker, err := NewPushWorker(id, logger, stateReader, parentDeps)
			if err != nil {
				panic(fmt.Sprintf("failed to create push worker: %v", err))
			}

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.PushObservedState, *snapshot.PushDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(fmt.Sprintf("failed to register push worker: %v", err))
	}
}
