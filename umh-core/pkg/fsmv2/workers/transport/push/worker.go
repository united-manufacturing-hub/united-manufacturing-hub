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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/state"

	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

var _ fsmv2.Worker = (*PushWorker)(nil)

// PushWorker implements the FSM v2 Worker interface for outbound message pushing.
// It drains the outbound channel and pushes messages to the backend relay server.
// PushWorker is a child of TransportWorker and shares its parent's JWT token and transport.
type PushWorker struct {
	fsmv2.WorkerBase[snapshot.PushDesiredState, snapshot.PushStatus, *PushDependencies]
}

// NewPushWorker creates a new PushWorker in Stopped state.
// dependencies must not be nil  -  the push worker delegates auth and transport to the parent.
func NewPushWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	dependencies *PushDependencies,
) (*PushWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if dependencies == nil {
		return nil, errors.New("push worker requires non-nil dependencies; ensure transport worker has published deps via register.SetDeps[*TransportDependencies] before push instantiation")
	}

	// Hardcode worker type to avoid DeriveWorkerType dependency on ObservedState struct name.
	if identity.WorkerType == "" {
		identity.WorkerType = "push"
	}

	w := &PushWorker{}
	w.InitBase(identity, logger, stateReader)
	w.BindDeps(dependencies)

	return w, nil
}

// GetDependencies returns the typed PushDependencies.
// Panics with a clear message if BindDeps was not called before this worker is used.
func (w *PushWorker) GetDependencies() *PushDependencies {
	raw := w.GetDependenciesAny()

	d, ok := raw.(*PushDependencies)
	if !ok || d == nil {
		panic("PushWorker: GetDependencies called before BindDeps")
	}

	return d
}

// CollectObservedState snapshots the current push worker state.
// Returns NewObservation; the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically.
func (w *PushWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.GetDependencies()

	var cfg snapshot.PushDesiredState
	if desired != nil {
		cfg = fsmv2.ExtractConfig[snapshot.PushDesiredState](desired)
	}

	status := snapshot.PushStatus{
		HasTransport:        d.GetTransport() != nil,
		HasValidToken:       cfg.AuthSession.IsUsable(time.Minute),
		ConsecutiveErrors:   d.GetConsecutiveErrors(),
		PendingMessageCount: d.PendingMessageCount(),
		LastErrorType:       d.GetLastErrorType(),
		LastRetryAfter:      d.GetLastRetryAfter(),
		DegradedEnteredAt:   d.GetDegradedEnteredAt(),
		LastErrorAt:         d.GetLastErrorAt(),
	}

	return fsmv2.NewObservation(status), nil
}

// DeriveDesiredState determines the desired state from the provided spec.
// Returns DesiredStateRunning when spec is nil (child workers default to running).
// Must be PURE  -  only uses the spec parameter, never dependencies.
func (w *PushWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[snapshot.PushDesiredState]{}, nil
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

	// Parse the rendered config to validate it and extract the AuthSession
	// stamped by the parent transport worker.
	parsed, err := config.ParseUserSpec[PushUserSpec](renderedSpec)
	if err != nil {
		return nil, err
	}

	return &fsmv2.WrappedDesiredState[snapshot.PushDesiredState]{
		Config: snapshot.PushDesiredState{AuthSession: parsed.AuthSession},
	}, nil
}

// GetInitialState returns StoppedState as the push worker's initial FSM state.
func (w *PushWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	register.Worker[snapshot.PushDesiredState, snapshot.PushStatus, *PushDependencies]("push",
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			builder, ok := register.GetDepsBuilder("push")
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

	register.SetDepsBuilder[*PushDependencies]("push",
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) *PushDependencies {
			parentDeps := register.GetDeps[*transport_pkg.TransportDependencies]("transport")
			if parentDeps == nil {
				logger.SentryError(deps.FeatureForWorker("push"), id.HierarchyPath,
					errors.New("parent transport deps not published"),
					"push_parent_transport_deps_missing")

				return nil
			}

			d, err := NewPushDependencies(parentDeps, deps.NewBaseDependencies(logger, sr, id))
			if err != nil {
				logger.SentryError(deps.FeatureForWorker("push"), id.HierarchyPath, err, "push_dependencies_creation_failed")

				return nil
			}

			return d
		})
}
