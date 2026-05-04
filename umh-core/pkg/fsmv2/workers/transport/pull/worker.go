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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"

	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

const workerType = "pull"

var _ fsmv2.Worker = (*PullWorker)(nil)

// PullWorker implements the FSM Worker interface for inbound message pulling.
// It polls the backend relay server and delivers messages to the inbound channel.
// PullWorker is a child of TransportWorker and shares its parent's JWT token and transport.
type PullWorker struct {
	fsmv2.WorkerBase[PullConfig, PullStatus, *PullDependencies]
}

// NewPullWorker creates a new PullWorker in Stopped state with the supplied
// PullDependencies. The deps argument is built by the SetDepsBuilder callback
// registered in init(), which reads parent transport deps via register.GetDeps.
func NewPullWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	dependencies *PullDependencies,
) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}
	if dependencies == nil {
		return nil, errors.New("pull worker requires non-nil dependencies; ensure transport worker has published deps via register.SetDeps[*TransportDependencies] before pull instantiation")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	w := &PullWorker{}
	w.InitBase(identity, logger, stateReader)
	w.BindDeps(dependencies)

	return w, nil
}

// GetDependencies returns the typed PullDependencies for use in tests and
// internal call-sites that need direct access without the any-typed accessor.
func (w *PullWorker) GetDependencies() *PullDependencies {
	d, _ := w.GetDependenciesAny().(*PullDependencies)
	return d
}

// CollectObservedState snapshots the current pull worker state.
// Returns NewObservation — the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically after COS returns.
func (w *PullWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
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

// init registers the pull worker via the generic register.Worker helper with
// typed TDeps = *PullDependencies. The factory closure pulls per-instance deps
// via register.GetDepsBuilder, which in turn reads parent transport deps via
// register.GetDeps[*transport_pkg.TransportDependencies] published by the
// transport worker constructor.
func init() {
	register.Worker[PullConfig, PullStatus, *PullDependencies](workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			builder, ok := register.GetDepsBuilder(workerType)
			if !ok {
				return nil, errors.New("pull worker requires deps builder; transport worker must initialise before pull instantiation")
			}
			rawDeps := builder(id, logger, sr)
			pdeps, ok := rawDeps.(*PullDependencies)
			if !ok || pdeps == nil {
				return nil, fmt.Errorf("pull deps builder returned %T; want *PullDependencies (parent transport deps may not be published)", rawDeps)
			}

			return NewPullWorker(id, logger, sr, pdeps)
		})

	register.SetDepsBuilder[*PullDependencies](workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) *PullDependencies {
			parentDeps := register.GetDeps[*transport_pkg.TransportDependencies]("transport")
			if parentDeps == nil {
				return nil
			}

			d, err := NewPullDependencies(parentDeps, id, logger, sr)
			if err != nil {
				return nil
			}

			return d
		})
}
