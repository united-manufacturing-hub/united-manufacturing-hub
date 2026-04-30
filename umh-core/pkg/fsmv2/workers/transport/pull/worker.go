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

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"

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
// The TDeps parameter is accepted to match register.Worker's constructor
// signature but is currently ignored; parent transport deps are obtained
// from the transport.ChildDeps() singleton populated by the transport
// worker's constructor.
func NewPullWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	_ *PullDependencies,
) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	parentDeps := transport_pkg.ChildDeps()
	if parentDeps == nil {
		return nil, errors.New("pull worker requires parent transport deps via transport.ChildDeps(); transport worker must be registered/constructed first")
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
// Child workers always default to running; lifecycle is controlled via ShutdownRequested.
// Must be PURE — only uses the spec parameter, never dependencies.
func (w *PullWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[PullConfig]{}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	renderedConfig, err := config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	var cfg PullConfig
	if renderedConfig != "" {
		if err := yaml.Unmarshal([]byte(renderedConfig), &cfg); err != nil {
			return nil, fmt.Errorf("config unmarshal failed: %w", err)
		}
	}

	return &fsmv2.WrappedDesiredState[PullConfig]{
		Config: cfg,
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
// typed TDeps = *PullDependencies. Parent transport deps are consumed via the
// transport.ChildDeps() singleton populated by the transport worker during its
// own factory init, replacing the prior extraDeps["transport_deps"] seam.
func init() {
	register.Worker[PullConfig, PullStatus, *PullDependencies](workerType, NewPullWorker)
}
