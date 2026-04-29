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

package exampleparent

import (
	"context"
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"

	// Blank import for side effects: registers the "Stopped" initial state
	// via fsmv2.RegisterInitialState in state/state_stopped.go init(). WorkerBase's
	// GetInitialState looks up the state from the registry, so the state
	// package must be loaded whenever the worker is imported — otherwise the
	// registry lookup returns nil and the supervisor panics at first tick.
	// This import is safe because state/ depends on snapshot/ (not on the
	// worker package), so no import cycle is introduced.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/state"
)

// WorkerTypeName is the canonical worker-type identifier for the exampleparent
// worker, used in config YAML and CSE storage.
const WorkerTypeName = "exampleparent"

// Compile-time interface check: ParentWorker implements fsmv2.Worker.
var _ fsmv2.Worker = (*ParentWorker)(nil)

// ParentWorker implements the FSM Worker interface for the exampleparent
// worker. Children are emitted by RenderChildren (children.go) from each
// state.Next return; the supervisor reconciles the actual child set against
// the canonical children-set per PR2 P2.4 NextResult.Children semantics.
type ParentWorker struct {
	fsmv2.WorkerBase[ExampleparentConfig, ExampleparentStatus]
	deps *ParentDependencies
}

// NewParentWorker creates a new exampleparent worker. The dependencies
// parameter is optional: when nil the constructor provisions fresh
// ParentDependencies around the framework logger/stateReader/identity.
//
// Returns fsmv2.Worker to align with the register.Worker constructor contract
// used by transport/push/pull and the other example workers.
func NewParentWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	dependencies *ParentDependencies,
) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = WorkerTypeName
	}

	if dependencies == nil {
		dependencies = NewParentDependencies(logger, stateReader, identity)
	}

	w := &ParentWorker{deps: dependencies}
	w.InitBase(identity, logger, stateReader)

	// Children are emitted by the canonical RenderChildren in children.go,
	// invoked from each state.Next via WorkerSnapshot[ExampleparentConfig,
	// ExampleparentStatus]. The supervisor reads the resulting children set
	// from NextResult.Children (PR2 P2.4 discriminator). The supervisor then
	// merges config.Merge(parentUserSpec.Variables, spec.UserSpec.Variables)
	// at spawn time; per-child DEVICE_ID is injected by RenderChildren.
	return w, nil
}

// GetDependencies returns the typed parent dependencies. Used by tests and by
// external callers that need to observe worker state.
func (w *ParentWorker) GetDependencies() *ParentDependencies {
	return w.deps
}

// GetDependenciesAny returns the custom ParentDependencies. Overrides
// WorkerBase's default which returns *BaseDependencies. Required by the
// architecture test: custom deps must be visible to the supervisor.
func (w *ParentWorker) GetDependenciesAny() any {
	return w.deps
}

// CollectObservedState snapshots the parent's runtime state. The parent has no
// worker-specific status fields — the state machine drives transitions from
// framework-level children health counts and parent-mapped state, both of
// which the collector injects automatically after COS returns.
func (w *ParentWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Track state changes so dependencies.StateTracker can expose
	// time-in-state to any test hook that consumes it. The supervisor
	// separately records state transitions for framework metrics.
	//
	// First-tick startup (no prior observation) is the expected ErrNotFound
	// case; we silently skip the state-tracker update. Other errors
	// (e.g. JSON deserialization failures, context cancellation, storage
	// corruption) are logged as warnings but never propagated — COS must
	// not fail the supervisor tick on a state-tracker bookkeeping miss.
	stateReader := w.deps.GetStateReader()
	if stateReader != nil {
		var previous fsmv2.Observation[ExampleparentStatus]

		err := stateReader.LoadObservedTyped(ctx, w.Identity().WorkerType, w.Identity().ID, &previous)
		switch {
		case err == nil:
			if previous.State != "" {
				w.deps.GetStateTracker().RecordStateChange(previous.State)
			}
		case errors.Is(err, persistence.ErrNotFound):
			// Expected on first tick — no prior observation persisted yet.
		default:
			w.deps.GetLogger().SentryWarn(deps.FeatureExamples, w.Identity().HierarchyPath,
				"LoadObservedTyped failed in CollectObservedState",
				deps.String("worker_id", w.Identity().ID),
				deps.Err(err))
		}
	}

	return fsmv2.NewObservation(ExampleparentStatus{}), nil
}

// init registers the exampleparent worker via the generic register.Worker
// helper with typed TDeps = *ParentDependencies. Callers that want to inject
// custom dependencies can publish them via register.SetDeps before the
// supervisor starts; otherwise the constructor provisions fresh
// ParentDependencies.
func init() {
	register.Worker[ExampleparentConfig, ExampleparentStatus, *ParentDependencies](WorkerTypeName, NewParentWorker)
}
