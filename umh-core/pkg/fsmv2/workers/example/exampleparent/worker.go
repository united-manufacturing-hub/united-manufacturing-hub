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

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// workerType is the registered type name for this worker.
const workerType = "exampleparent"

// ParentWorker implements the FSM v2 Worker interface for parent-child relationships.
type ParentWorker struct {
	deps *ParentDependencies
	fsmv2.WorkerBase[ExampleparentConfig, ExampleparentStatus, *ParentDependencies]
}

// NewParentWorker creates a new example parent worker.
func NewParentWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*ParentWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	w := &ParentWorker{}
	baseDeps := w.InitBase(identity, logger, stateReader)
	w.deps = NewParentDependencies(baseDeps)
	w.BindDeps(w.deps)

	return w, nil
}

// CollectObservedState returns the current observed state of the parent worker.
// Returns NewObservation; the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically.
func (w *ParentWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return fsmv2.NewObservation(ExampleparentStatus{}), nil
}

// DeriveDesiredState parses the user spec and returns the desired state.
// The parent populates WrappedDesiredState.ChildrenSpecs via RenderChildren so
// the supervisor's DDS-derived children-set path emits the canonical ChildSpec
// values authoritatively (the renderChildren-in-state.Next path is a nil-stub
// pending PR4-C, so DDS is the single source of truth in this commit).
//
// Children-spec emission preserves the §4-C/F4⊕G1 invariant via NewChildSpec,
// which sets Enabled: true explicitly on each ChildSpec.
func (w *ParentWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		// Byte-equivalent with canonical RenderChildren(nil) = []ChildSpec{}.
		// The DDS path emits the same authoritative "zero children right now"
		// sentinel as the canonical path.
		return &fsmv2.WrappedDesiredState[ExampleparentConfig]{
			ChildrenSpecs: []config.ChildSpec{},
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

	var cfg ExampleparentConfig
	if renderedConfig != "" {
		if err := yaml.Unmarshal([]byte(renderedConfig), &cfg); err != nil {
			return nil, fmt.Errorf("config unmarshal failed: %w", err)
		}
	}

	if cfg.ChildrenCount == 0 {
		// Same byte-equivalence with canonical RenderChildren as the spec==nil
		// branch above. If a user shrinks ChildrenCount from N>0 to 0, the
		// supervisor needs an explicit non-nil [] sentinel to despawn existing
		// children authoritatively; nil here would re-route through fallback
		// and leak the prior tick's children-set on parents whose mirror also
		// returns nil (option-a, exampleparent).
		return &fsmv2.WrappedDesiredState[ExampleparentConfig]{
			Config:        cfg,
			ChildrenSpecs: []config.ChildSpec{},
		}, nil
	}

	// RenderChildren (children.go) is the canonical children-set emitter and
	// the single source of truth for ChildSpec construction.
	childrenSpecs := RenderChildren(&cfg)

	return &fsmv2.WrappedDesiredState[ExampleparentConfig]{
		Config:        cfg,
		ChildrenSpecs: childrenSpecs,
	}, nil
}

func init() {
	register.Worker[ExampleparentConfig, ExampleparentStatus, *ParentDependencies](
		workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			return NewParentWorker(id, logger, sr)
		},
	)
}
