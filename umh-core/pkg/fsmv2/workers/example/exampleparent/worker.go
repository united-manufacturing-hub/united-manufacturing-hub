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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"

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

const workerType = WorkerTypeName

// defaultChildConfig is the fallback UserSpec.Config template handed to each
// child when the parent's ExampleparentConfig.ChildConfig is empty. It
// references the parent's inherited user variables (IP, PORT) plus the
// per-child DEVICE_ID injected by the child-spec factory.
const defaultChildConfig = `address: {{ .IP }}:{{ .PORT }}
device: {{ .DEVICE_ID }}`

// Compile-time interface check: ParentWorker implements fsmv2.Worker.
var _ fsmv2.Worker = (*ParentWorker)(nil)

// ParentWorker implements the FSM Worker interface for the exampleparent
// worker. It declares children via SetChildSpecsFactory; the supervisor
// reconciles the actual child set against the declared specs.
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
		identity.WorkerType = workerType
	}

	if dependencies == nil {
		dependencies = NewParentDependencies(logger, stateReader, identity)
	}

	w := &ParentWorker{deps: dependencies}
	w.InitBase(identity, logger, stateReader)

	// Declare children via the child-spec factory. The factory receives the
	// parsed ExampleparentConfig plus the raw UserSpec (so parent user
	// variables can be propagated to each child). The supervisor merges
	// config.Merge(parentUserSpec.Variables, spec.UserSpec.Variables) again at
	// spawn time; pre-merging here keeps the declared specs self-describing
	// without changing the merge result.
	w.SetChildSpecsFactory(buildChildSpecs)

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
	stateReader := w.deps.GetStateReader()
	if stateReader != nil {
		var previous fsmv2.Observation[ExampleparentStatus]

		err := stateReader.LoadObservedTyped(ctx, w.Identity().WorkerType, w.Identity().ID, &previous)
		if err == nil && previous.State != "" {
			w.deps.GetStateTracker().RecordStateChange(previous.State)
		}
	}

	return fsmv2.NewObservation(ExampleparentStatus{}), nil
}

// buildChildSpecs constructs the ChildSpec slice for the parent's declared
// children. It runs as the WorkerBase child-spec factory callback (pure
// function of its inputs).
//
// Each child inherits the parent's user-namespace variables (IP, PORT,
// CONNECTION_NAME, ...) merged with a per-child DEVICE_ID. The supervisor
// re-applies config.Merge(parentVars, childVars) at spawn time; pre-merging
// here keeps the declared specs self-describing and preserves the merge
// invariant observed by integration tests.
func buildChildSpecs(cfg ExampleparentConfig, spec config.UserSpec) []config.ChildSpec {
	if cfg.ChildrenCount == 0 {
		return nil
	}

	childWorkerType := cfg.GetChildWorkerType()

	childConfigTemplate := cfg.ChildConfig
	if childConfigTemplate == "" {
		childConfigTemplate = defaultChildConfig
	}

	specs := make([]config.ChildSpec, cfg.ChildrenCount)

	for i := range cfg.ChildrenCount {
		childVars := map[string]any{}
		for k, v := range spec.Variables.User {
			childVars[k] = v
		}

		childVars["DEVICE_ID"] = fmt.Sprintf("device-%d", i)

		specs[i] = config.ChildSpec{
			Name:       fmt.Sprintf("child-%d", i),
			WorkerType: childWorkerType,
			UserSpec: config.UserSpec{
				Config: childConfigTemplate,
				Variables: config.VariableBundle{
					User: childVars,
				},
			},
			ChildStartStates: []string{"TryingToStart", "Running"},
		}
	}

	return specs
}

// init registers the exampleparent worker via the generic register.Worker
// helper with typed TDeps = *ParentDependencies. Callers that want to inject
// custom dependencies can publish them via register.SetDeps before the
// supervisor starts; otherwise the constructor provisions fresh
// ParentDependencies.
func init() {
	register.Worker[ExampleparentConfig, ExampleparentStatus, *ParentDependencies](WorkerTypeName, NewParentWorker)
}
