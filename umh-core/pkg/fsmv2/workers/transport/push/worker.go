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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"

	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

const workerType = "push"

var _ fsmv2.Worker = (*PushWorker)(nil)

// PushWorker implements the FSM Worker interface for outbound message pushing.
// It drains the outbound channel and pushes messages to the backend relay server.
// PushWorker is a child of TransportWorker and shares its parent's JWT token and transport.
type PushWorker struct {
	fsmv2.WorkerBase[PushConfig, PushStatus, *PushDependencies]
}

// NewPushWorker creates a new PushWorker in Stopped state with the supplied
// PushDependencies. The deps argument is built by the SetDepsBuilder callback
// registered in init(), which reads parent transport deps via register.GetDeps.
func NewPushWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	dependencies *PushDependencies,
) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}
	if dependencies == nil {
		return nil, errors.New("push worker requires non-nil dependencies; ensure transport worker has published deps via register.SetDeps[*TransportDependencies] before push instantiation")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	w := &PushWorker{}
	w.InitBase(identity, logger, stateReader)
	w.BindDeps(dependencies)

	return w, nil
}

// GetDependencies returns the typed PushDependencies for use in tests and
// internal call-sites that need direct access without the any-typed accessor.
func (w *PushWorker) GetDependencies() *PushDependencies {
	d, _ := w.GetDependenciesAny().(*PushDependencies)
	return d
}

// CollectObservedState snapshots the current push worker state.
// Returns NewObservation — the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically after COS returns.
func (w *PushWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	d := w.GetDependencies()

	return fsmv2.NewObservation(PushStatus{
		HasTransport:        d.GetTransport() != nil,
		HasValidToken:       d.IsTokenValid(),
		ConsecutiveErrors:   d.GetConsecutiveErrors(),
		PendingMessageCount: d.PendingMessageCount(),
		LastErrorType:       d.GetLastErrorType(),
		LastRetryAfter:      d.GetLastRetryAfter(),
		DegradedEnteredAt:   d.GetDegradedEnteredAt(),
		LastErrorAt:         d.GetLastErrorAt(),
	}), nil
}

// DeriveDesiredState determines the desired state from the provided spec.
// Returns DesiredStateRunning when spec is nil (child workers default to running).
// Must be PURE — only uses the spec parameter, never dependencies.
func (w *PushWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[PushConfig]{
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

	leafDesired, err := config.DeriveLeafState[PushConfig](renderedSpec)
	if err != nil {
		return nil, err
	}

	return &fsmv2.WrappedDesiredState[PushConfig]{
		BaseDesiredState: leafDesired.BaseDesiredState,
	}, nil
}

// GetInitialState returns the registered initial state for the push worker.
func (w *PushWorker) GetInitialState() fsmv2.State[any, any] {
	s := fsmv2.LookupInitialState(workerType)
	if s == nil {
		panic(fmt.Sprintf("no initial state registered for worker type %q", workerType))
	}
	return s
}

// init registers the push worker via the generic register.Worker helper with
// typed TDeps = *PushDependencies. The factory closure pulls per-instance deps
// via register.GetDepsBuilder, which in turn reads parent transport deps via
// register.GetDeps[*transport_pkg.TransportDependencies] published by the
// transport worker constructor.
func init() {
	register.Worker[PushConfig, PushStatus, *PushDependencies](workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			builder, ok := register.GetDepsBuilder(workerType)
			if !ok {
				return nil, errors.New("push worker requires deps builder; transport worker must initialise before push instantiation")
			}
			rawDeps := builder(id, logger, sr)
			pdeps, ok := rawDeps.(*PushDependencies)
			if !ok || pdeps == nil {
				return nil, fmt.Errorf("push deps builder returned %T; want *PushDependencies (parent transport deps may not be published)", rawDeps)
			}

			return NewPushWorker(id, logger, sr, pdeps)
		})

	register.SetDepsBuilder[*PushDependencies](workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) *PushDependencies {
			parentDeps := register.GetDeps[*transport_pkg.TransportDependencies]("transport")
			if parentDeps == nil {
				return nil
			}

			d, err := NewPushDependencies(parentDeps, id, logger, sr)
			if err != nil {
				return nil
			}

			return d
		})
}
