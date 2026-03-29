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

package pull

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"

	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

const workerType = "pull"

var _ fsmv2.Worker = (*PullWorker)(nil)

// PullWorker implements the FSM Worker interface for inbound message pulling.
// It polls the backend relay server and delivers messages to the inbound channel.
// PullWorker is a child of TransportWorker and shares its parent's JWT token and transport.
type PullWorker struct {
	*helpers.BaseWorker[*PullDependencies]
	logger   deps.FSMLogger
	identity deps.Identity
}

// NewPullWorker creates a new PullWorker in Stopped state.
// parentDeps must not be nil — the pull worker delegates auth and transport to the parent.
func NewPullWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	parentDeps *transport_pkg.TransportDependencies,
) (*PullWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	dependencies, err := NewPullDependencies(parentDeps, identity, logger, stateReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull dependencies: %w", err)
	}

	return &PullWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
	}, nil
}

// CollectObservedState snapshots the current pull worker state.
// Returns NewObservation — the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically after COS returns.
func (w *PullWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.GetDependencies()

	return fsmv2.NewObservation(PullStatus{
		HasTransport:        d.GetTransport() != nil,
		HasValidToken:       d.IsTokenValid(),
		IsBackpressured:     d.IsBackpressured(),
		ConsecutiveErrors:   d.GetConsecutiveErrors(),
		PendingMessageCount: d.PendingMessageCount(),
		LastErrorType:       d.GetLastErrorType(),
		LastRetryAfter:      d.GetLastRetryAfter(),
		DegradedEnteredAt:   d.GetDegradedEnteredAt(),
		LastErrorAt:         d.GetLastErrorAt(),
	}), nil
}

// DeriveDesiredState determines the desired state from the provided spec.
// Must be PURE — only uses the spec parameter, never dependencies.
func (w *PullWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[PullConfig]{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
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

	leafDesired, err := config.DeriveLeafState[PullConfig](renderedSpec)
	if err != nil {
		return nil, err
	}

	return &fsmv2.WrappedDesiredState[PullConfig]{
		BaseDesiredState: leafDesired.BaseDesiredState,
	}, nil
}

// GetInitialState returns the registered initial state for the pull worker.
func (w *PullWorker) GetInitialState() fsmv2.State[any, any] {
	s := fsmv2.LookupInitialState(workerType)
	if s == nil {
		panic(fmt.Sprintf("no initial state registered for worker type %q", workerType))
	}
	return s
}

func init() {
	if err := factory.RegisterWorkerAndSupervisorFactoryByType(
		workerType,
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, extraDeps map[string]any) fsmv2.Worker {
			parentDepsRaw, ok := extraDeps["transport_deps"]
			if !ok {
				panic("pull worker requires transport_deps in extraDeps")
			}

			parentDeps, ok := parentDepsRaw.(*transport_pkg.TransportDependencies)
			if !ok {
				panic("transport_deps must be *TransportDependencies")
			}

			worker, err := NewPullWorker(id, logger, stateReader, parentDeps)
			if err != nil {
				panic(fmt.Sprintf("failed to create pull worker: %v", err))
			}

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[fsmv2.Observation[PullStatus], *fsmv2.WrappedDesiredState[PullConfig]](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(fmt.Sprintf("failed to register pull worker: %v", err))
	}

	observedType := reflect.TypeOf(fsmv2.Observation[PullStatus]{})
	desiredType := reflect.TypeOf(fsmv2.WrappedDesiredState[PullConfig]{})

	if err := storage.GlobalRegistry().RegisterWorkerType(workerType, observedType, desiredType); err != nil {
		panic(fmt.Sprintf("failed to register pull CSE types: %v", err))
	}
}
