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

// Package application provides a generic root supervisor for FSMv2.
// The application supervisor dynamically creates children based on YAML configuration,
// allowing any registered worker type to be instantiated as a child.
package application

import (
	"context"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"

	// Blank import for side effects: registers the initial state via
	// fsmv2.RegisterInitialState in state/stopped_state.go init(). WorkerBase's
	// GetInitialState looks up the state from the registry, so the state
	// package must be loaded whenever the worker is imported — otherwise the
	// registry lookup returns nil and the supervisor panics at first tick.
	// This import is safe because state/ depends on snapshot/ (not on the
	// worker package), so no import cycle is introduced.
	_ "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/state"
)

// WorkerTypeName is the canonical worker-type identifier for the application
// worker, used in config YAML and CSE storage. Exported to mirror the
// persistence worker's pattern and allow cmd/main.go callers to reference it
// without hardcoding the string.
const WorkerTypeName = "application"

const workerType = WorkerTypeName

// Compile-time interface check: ApplicationWorker implements fsmv2.Worker.
var _ fsmv2.Worker = (*ApplicationWorker)(nil)

// ApplicationWorker is a generic application worker that parses YAML configuration
// to dynamically discover and create child workers. It doesn't hardcode child
// types - any registered worker type can be instantiated as a child.
//
// This implements the "passthrough pattern" where the application supervisor simply
// passes through ChildrenSpecs from the YAML config without knowing about
// specific child implementations.
type ApplicationWorker struct {
	fsmv2.WorkerBase[snapshot.ApplicationConfig, snapshot.ApplicationStatus]
	id   string
	name string
}

// NewApplicationWorker creates a new application worker. Returns nil when
// id or name is empty — callers must surface that as a construction error.
// A nil logger is replaced with a no-op logger so unit tests can construct
// workers without wiring a full logger stack; production callers should pass
// the supervisor's configured logger.
func NewApplicationWorker(id, name string, logger deps.FSMLogger, stateReader deps.StateReader) *ApplicationWorker {
	if id == "" || name == "" {
		return nil
	}

	if logger == nil {
		logger = deps.NewNopFSMLogger()
	}

	w := &ApplicationWorker{
		id:   id,
		name: name,
	}
	w.InitBase(deps.Identity{
		ID:         id,
		Name:       name,
		WorkerType: workerType,
	}, logger, stateReader)

	return w
}

// CollectObservedState returns the current observed state of the application
// supervisor. Returns fsmv2.NewObservation — the collector fills CollectedAt,
// framework metrics, action history, ChildrenView, and children counts
// automatically after COS returns.
func (w *ApplicationWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return fsmv2.NewObservation(snapshot.ApplicationStatus{
		ID:   w.id,
		Name: w.name,
	}), nil
}

// childrenConfig is the structure for parsing children from YAML.
type childrenConfig struct {
	Children []config.ChildSpec `yaml:"children"`
}

// DeriveDesiredState parses the YAML configuration to extract children
// specifications and wraps the result in *fsmv2.WrappedDesiredState so it
// satisfies the WorkerBase TConfig/TStatus shape while preserving the
// application worker's passthrough children semantics.
func (w *ApplicationWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	cfg := snapshot.ApplicationConfig{Name: w.name}

	if spec == nil {
		// Byte-equivalent with canonical RenderChildren on the empty case:
		// canonical returns []ChildSpec{}, so the DDS path emits the same
		// authoritative "zero children right now" sentinel here. Pre-PR2-
		// boundary this branch left ChildrenSpecs as the zero value (nil),
		// producing a divergence with the canonical path that was
		// operationally moot today (state.Next mirror always emits non-nil
		// for application; the discriminator at reconciliation.go never
		// falls through to the DDS path in normal operation) but still a
		// drift the PR2 boundary closes for symmetry with the discriminator
		// nil-vs-empty contract.
		return &fsmv2.WrappedDesiredState[snapshot.ApplicationConfig]{
			Config:        cfg,
			ChildrenSpecs: []config.ChildSpec{},
		}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected config.UserSpec, got %T", spec)
	}

	var childrenCfg childrenConfig
	if userSpec.Config != "" {
		if err := yaml.Unmarshal([]byte(userSpec.Config), &childrenCfg); err != nil {
			return nil, fmt.Errorf("failed to parse children config: %w", err)
		}
	}

	// §4-C exception (application YAML passthrough): the application's
	// contract is "every child the user declared runs". Default-on is the
	// load-bearing semantic for this worker; the regular §4-C zero-value-
	// false rule is opted out of here. Callers that explicitly want a
	// stopped child must use the worker-specific ShutdownRequested signal,
	// not the per-tick Enabled flag. RenderChildren (children.go) emits
	// the same default-on result so the legacy DDS path and the post-P2.2
	// renderChildren-in-state.Next path stay bit-for-bit identical.
	//
	// To avoid the silent-override trap (user writes `enabled: false` in
	// YAML, application overrides to true, user debugs in confusion), we
	// reject any user-supplied `enabled:` key at parse time. Detection
	// uses a parallel parse into a map shape because yaml.Unmarshal into
	// a typed struct cannot distinguish "field absent" from "field set to
	// zero value." The application contract is enforced explicitly: users
	// declare WHICH children exist; the framework decides whether each
	// runs.
	if userSpec.Config != "" {
		var rawCfg struct {
			Children []map[string]any `yaml:"children"`
		}
		if err := yaml.Unmarshal([]byte(userSpec.Config), &rawCfg); err != nil {
			return nil, fmt.Errorf("failed to re-parse children config for enabled-key check: %w", err)
		}
		for i, raw := range rawCfg.Children {
			if _, hasEnabled := raw["enabled"]; hasEnabled {
				return nil, fmt.Errorf("application children[%d] has user-supplied `enabled:` field; not permitted (§4-C YAML-passthrough exception): the application worker forces all declared children to run. Remove the `enabled:` key from your YAML, or use the ShutdownRequested signal at runtime if you need to stop a specific child", i)
			}
		}
	}

	for i := range childrenCfg.Children {
		childrenCfg.Children[i].Enabled = true
	}

	return &fsmv2.WrappedDesiredState[snapshot.ApplicationConfig]{
		Config:        cfg,
		ChildrenSpecs: childrenCfg.Children,
	}, nil
}

// GetDependenciesAny implements fsmv2.DependencyProvider. The application
// worker has no typed dependencies today, so it returns nil.
func (w *ApplicationWorker) GetDependenciesAny() any {
	return nil
}

// SupervisorConfig contains configuration for creating an application supervisor.
type SupervisorConfig struct {
	Store              storage.TriangularStoreInterface
	Logger             deps.FSMLogger
	Dependencies       map[string]any // Injected into child workers via deps parameter
	ID                 string
	Name               string
	YAMLConfig         string        // Raw YAML containing children specifications
	TickInterval       time.Duration // Defaults to 100ms
	EnableTraceLogging bool          // Verbose lifecycle event logging for debugging
}

// NewApplicationSupervisor creates a supervisor with an application worker already added.
// Child workers are created automatically via reconcileChildren() based on ChildrenSpecs.
func NewApplicationSupervisor(cfg SupervisorConfig) (*supervisor.Supervisor[fsmv2.Observation[snapshot.ApplicationStatus], *fsmv2.WrappedDesiredState[snapshot.ApplicationConfig]], error) {
	tickInterval := cfg.TickInterval
	if tickInterval == 0 {
		tickInterval = 100 * time.Millisecond
	}

	sup := supervisor.NewSupervisor[
		fsmv2.Observation[snapshot.ApplicationStatus],
		*fsmv2.WrappedDesiredState[snapshot.ApplicationConfig],
	](supervisor.Config{
		WorkerType:         workerType,
		Store:              cfg.Store,
		Logger:             cfg.Logger,
		TickInterval:       tickInterval,
		UserSpec:           config.UserSpec{Config: cfg.YAMLConfig},
		EnableTraceLogging: cfg.EnableTraceLogging,
		Dependencies:       cfg.Dependencies,
	})

	appIdentity := deps.Identity{
		ID:            cfg.ID,
		Name:          cfg.Name,
		WorkerType:    workerType,
		HierarchyPath: fmt.Sprintf("%s(%s)", cfg.ID, workerType),
	}

	appWorker := NewApplicationWorker(cfg.ID, cfg.Name, cfg.Logger, nil)

	// Application workers need explicit AddWorker; child workers are created via reconcileChildren
	err := sup.AddWorker(appIdentity, appWorker)
	if err != nil {
		return nil, err
	}

	return sup, nil
}

// init registers the application worker via the generic register.Worker helper
// with TDeps = register.NoDeps. The application worker takes no custom
// dependencies beyond the standard framework (Identity, FSMLogger,
// StateReader), so GetDeps returns the Go-native zero value and no
// register.SetDeps wiring is needed at cmd/main.go.
//
// The factory closure ignores the zero-value NoDeps and constructs the
// worker with just id/name/logger/stateReader, matching the existing
// NewApplicationWorker signature introduced in C11.
func init() {
	register.Worker[snapshot.ApplicationConfig, snapshot.ApplicationStatus, register.NoDeps](
		WorkerTypeName,
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ register.NoDeps) (fsmv2.Worker, error) {
			w := NewApplicationWorker(id.ID, id.Name, logger, stateReader)
			if w == nil {
				return nil, fmt.Errorf("NewApplicationWorker returned nil for id=%q name=%q", id.ID, id.Name)
			}
			return w, nil
		},
	)
}
