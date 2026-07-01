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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/state"

	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

var _ fsmv2.Worker = (*PullWorker)(nil)

// PullWorker implements the FSM v2 Worker interface for inbound message pulling.
// It polls the backend relay server and delivers messages to the inbound channel.
// PullWorker is a child of TransportWorker and shares its parent's JWT token and transport.
type PullWorker struct {
	fsmv2.WorkerBase[snapshot.PullDesiredState, snapshot.PullStatus, *PullDependencies]
}

// NewPullWorker creates a new PullWorker in Stopped state.
// dependencies must not be nil  -  the pull worker delegates auth and transport to the parent.
func NewPullWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	dependencies *PullDependencies,
) (*PullWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if dependencies == nil {
		return nil, errors.New("pull worker requires non-nil dependencies; ensure transport worker has published deps via register.SetDeps[*TransportDependencies] before pull instantiation")
	}

	// Hardcode worker type to avoid DeriveWorkerType dependency on ObservedState struct name.
	if identity.WorkerType == "" {
		identity.WorkerType = "pull"
	}

	w := &PullWorker{}
	w.InitBase(identity, logger, stateReader)
	w.BindDeps(dependencies)

	return w, nil
}

// GetDependencies returns the typed PullDependencies.
// Panics with a clear message if BindDeps was not called before this worker is used.
func (w *PullWorker) GetDependencies() *PullDependencies {
	raw := w.GetDependenciesAny()

	d, ok := raw.(*PullDependencies)
	if !ok || d == nil {
		panic("PullWorker: GetDependencies called before BindDeps")
	}

	return d
}

// CollectObservedState snapshots the current pull worker state.
// Returns NewObservation; the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically.
func (w *PullWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.GetDependencies()

	var cfg snapshot.PullDesiredState
	if desired != nil {
		cfg = fsmv2.ExtractConfig[snapshot.PullDesiredState](desired)
	}

	status := snapshot.PullStatus{
		HasTransport:        d.GetTransport() != nil,
		HasValidToken:       cfg.AuthSession.IsUsable(time.Minute),
		IsBackpressured:     d.IsBackpressured(),
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
// Must be PURE  -  only uses the spec parameter, never dependencies.
func (w *PullWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[snapshot.PullDesiredState]{}, nil
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

	parsed, err := config.ParseUserSpec[PullUserSpec](renderedSpec)
	if err != nil {
		return nil, err
	}

	return &fsmv2.WrappedDesiredState[snapshot.PullDesiredState]{
		Config: snapshot.PullDesiredState{AuthSession: parsed.AuthSession},
	}, nil
}

// GetInitialState returns StoppedState as the pull worker's initial FSM state.
func (w *PullWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	register.Worker[snapshot.PullDesiredState, snapshot.PullStatus, *PullDependencies]("pull",
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			builder, ok := register.GetDepsBuilder("pull")
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

	register.SetDepsBuilder[*PullDependencies]("pull",
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) *PullDependencies {
			parentDeps := register.GetDeps[*transport_pkg.TransportDependencies]("transport")
			if parentDeps == nil {
				logger.SentryError(deps.FeatureForWorker("pull"), id.HierarchyPath,
					errors.New("parent transport deps not published"),
					"pull_parent_transport_deps_missing")

				return nil
			}

			d, err := NewPullDependencies(parentDeps, deps.NewBaseDependencies(logger, sr, id))
			if err != nil {
				logger.SentryError(deps.FeatureForWorker("pull"), id.HierarchyPath, err, "pull_dependencies_creation_failed")

				return nil
			}

			return d
		})
}
