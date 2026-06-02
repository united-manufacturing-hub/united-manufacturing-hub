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

package fsmv2

import (
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// WorkerBase provides default implementations of the Worker interface methods
// DeriveDesiredState and GetInitialState, plus typed dependency access via BindDeps.
// Developers embed WorkerBase in their worker struct and implement only
// CollectObservedState (the worker-specific business logic).
//
// Type parameters:
//   - TConfig: the typed configuration struct (often embeds config.BaseUserSpec)
//   - TStatus: the typed observed status struct
//   - TDeps: the typed dependency set (typically a pointer to a concrete deps struct)
//
// WorkerBase implements DependencyProvider (GetDependenciesAny) but does NOT
// implement ActionProvider, MetricsProvider, GracefulShutdowner, or ChildrenViewConsumer.
// Workers that need those capabilities add them directly on their own struct.
type WorkerBase[TConfig any, TStatus any, TDeps any] struct {
	logger      deps.FSMLogger
	config      TConfig
	typedDeps   TDeps
	identity    deps.Identity
	mu          sync.RWMutex
	configReady bool
	initialized bool
}

// InitBase initializes the embedded WorkerBase with the worker's identity and
// framework dependencies. Returns the BaseDependencies instance so the caller
// can pass it to custom deps constructors that need access to the same logger,
// stateReader, and identity.
//
// Must be called in the worker constructor before any other WorkerBase method.
func (w *WorkerBase[TConfig, TStatus, TDeps]) InitBase(
	id deps.Identity,
	logger deps.FSMLogger,
	sr deps.StateReader,
) *deps.BaseDependencies {
	bd := deps.NewBaseDependencies(logger, sr, id)

	w.mu.Lock()
	w.identity = id
	w.logger = logger
	w.initialized = true
	w.mu.Unlock()

	return bd
}

// Config returns the TConfig cached from the last DeriveDesiredState call.
//
// Deprecated: scheduled for deletion in L3 (see L2b_SCOPE_MANIFEST). Production
// workers override DeriveDesiredState and never populate this cache, so Config()
// returns zero-value TConfig forever. Read config via fsmv2.ExtractConfig[T](desired)
// inside CollectObservedState instead.
func (w *WorkerBase[TConfig, TStatus, TDeps]) Config() TConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.config
}

// ConfigReady reports whether DeriveDesiredState has been called at least once.
//
// Deprecated: scheduled for deletion in L3 alongside Config(). Returns false
// forever for any worker that overrides DeriveDesiredState. See Config() godoc.
func (w *WorkerBase[TConfig, TStatus, TDeps]) ConfigReady() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.configReady
}

// DeriveDesiredState parses spec into a *WrappedDesiredState[TConfig].
// When spec is nil, returns a default desired state with an empty TConfig.
// When spec is a config.UserSpec, renders template variables, unmarshals
// YAML into TConfig, validates the lifecycle state when TConfig embeds
// config.BaseUserSpec (or implements GetState() string), and caches the config.
//
// Workers that need children specs must override this method in their own struct
// and either call w.WorkerBase.DeriveDesiredState(spec) first (then read the
// parsed TConfig from the returned WrappedDesiredState) or re-parse spec from
// scratch. Do NOT call w.Config() — see its godoc for why that path is dead.
func (w *WorkerBase[TConfig, TStatus, TDeps]) DeriveDesiredState(spec interface{}) (DesiredState, error) {
	if spec == nil {
		var cfg TConfig

		w.mu.Lock()
		w.config = cfg
		w.configReady = true
		w.mu.Unlock()

		return &WrappedDesiredState[TConfig]{
			Config: cfg,
		}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected config.UserSpec, got %T", spec)
	}

	renderedConfig, err := config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	var cfg TConfig
	if renderedConfig != "" {
		if err := yaml.Unmarshal([]byte(renderedConfig), &cfg); err != nil {
			return nil, fmt.Errorf("config unmarshal failed: %w", err)
		}
	}

	// Duck-type: validate and propagate the desired lifecycle state when
	// TConfig embeds config.BaseUserSpec or implements GetState() string.
	wds := &WrappedDesiredState[TConfig]{
		Config: cfg,
	}

	if g, ok := any(&cfg).(interface{ GetState() string }); ok {
		state := g.GetState()
		if state != "" {
			if err := config.ValidateDesiredState(state); err != nil {
				return nil, fmt.Errorf("invalid desired state: %w", err)
			}
		}

		wds.State = state
	}

	w.mu.Lock()
	w.config = cfg
	w.configReady = true
	w.mu.Unlock()

	return wds, nil
}

// GetInitialState returns the registered initial state for this worker type.
// Panics if InitBase was not called or if no state was registered for this worker type.
func (w *WorkerBase[TConfig, TStatus, TDeps]) GetInitialState() State[any, any] {
	w.mu.RLock()
	initialized := w.initialized
	workerType := w.identity.WorkerType
	w.mu.RUnlock()

	if !initialized {
		panic("WorkerBase.GetInitialState: InitBase was not called; call w.InitBase in your constructor")
	}

	s := LookupInitialState(workerType)
	if s == nil {
		panic(fmt.Sprintf("WorkerBase.GetInitialState: no initial state registered for worker type %q; call fsmv2.RegisterInitialState in your state package init()", workerType))
	}

	return s
}

// Identity returns the worker's identity as set by InitBase.
func (w *WorkerBase[TConfig, TStatus, TDeps]) Identity() deps.Identity {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.identity
}

// Logger returns the worker's logger as set by InitBase.
func (w *WorkerBase[TConfig, TStatus, TDeps]) Logger() deps.FSMLogger {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.logger
}

// BindDeps stores the typed deps payload. Call after InitBase in worker constructors.
// The stored value is returned by GetDependenciesAny to satisfy DependencyProvider.
func (w *WorkerBase[TConfig, TStatus, TDeps]) BindDeps(d TDeps) {
	w.mu.Lock()
	w.typedDeps = d
	w.mu.Unlock()
}

// GetDependenciesAny returns the typed deps bound via BindDeps as any.
// Satisfies the DependencyProvider interface. The supervisor uses this to pass
// deps to action Execute calls.
func (w *WorkerBase[TConfig, TStatus, TDeps]) GetDependenciesAny() any {
	w.mu.RLock()
	d := w.typedDeps
	w.mu.RUnlock()

	return d
}
