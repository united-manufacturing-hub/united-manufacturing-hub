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
	"errors"
	"fmt"
	"sync"

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
// Deprecated: read config via fsmv2.ExtractConfig[T](desired) inside
// CollectObservedState instead. Only the base DeriveDesiredState writes this
// cache. An override that calls the base first still fills it; an override
// that parses the spec on its own leaves it empty, and Config() then returns
// the zero value. No production code calls Config().
func (w *WorkerBase[TConfig, TStatus, TDeps]) Config() TConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.config
}

// ConfigReady reports whether DeriveDesiredState has filled the config cache
// at least once.
//
// Deprecated: see Config(). The cache is filled the same way, so
// ConfigReady() stays false only for a worker whose override never calls the
// base DeriveDesiredState. No production code calls ConfigReady().
func (w *WorkerBase[TConfig, TStatus, TDeps]) ConfigReady() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.configReady
}

// DeriveDesiredState parses spec into a *WrappedDesiredState[TConfig].
// When spec is nil, returns a default desired state with an empty TConfig.
// When spec is a config.UserSpec, loads the YAML into TConfig without
// rendering it, renders every config.Template field, validates the lifecycle
// state when TConfig embeds config.BaseUserSpec (or implements GetState()
// string), and caches the config. Templates render against the names a
// parent hands down when UserSpec.Names carries at least one entry; a nil
// or empty Names builds the names from the config's own fields, the config
// values a parent hands down, and the user variables.
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

	cfg, err := config.LoadUserSpec[TConfig](spec)
	if err != nil {
		return nil, fmt.Errorf("failed to load user spec: %w", err)
	}

	fields, err := config.EntryFields(userSpec.Config, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to read the config fields: %w", err)
	}

	// One channel for variables: the spec's variables bundle when it carries
	// one, otherwise the entry's own top-level variables mapping, which
	// EntryFields drops from the fields. Both carrying variables is a config
	// mistake, because the two would silently hide each other.
	entryVariables, entryHasVariables, err := config.EntryVariables(userSpec.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to read the entry's variables: %w", err)
	}

	variables := userSpec.Variables

	switch {
	case entryHasVariables && len(variables) > 0:
		return nil, &config.ConfigError{
			Field: "variables",
			Err:   errors.New("the entry declares a variables section while the spec also carries a variables bundle"),
		}
	case entryHasVariables:
		variables = entryVariables
	}

	names := userSpec.Names
	if len(names) == 0 {
		names, err = config.BuildNames(config.Entry{
			Fields:    fields,
			Variables: variables,
			Config:    userSpec.ConfigValues,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build the template names: %w", err)
		}
	}

	if err := config.RenderTemplates(&cfg, names); err != nil {
		return nil, fmt.Errorf("failed to render the template fields: %w", err)
	}

	// The state is read by duck-typing, so this works for any TConfig that
	// embeds config.BaseUserSpec or implements GetState() string. Validation
	// rejects illegal user-supplied state values; lifecycle itself is driven
	// by the FSM.
	if g, ok := any(&cfg).(interface{ GetState() string }); ok {
		state := g.GetState()
		if state != "" {
			if err := config.ValidateDesiredState(state); err != nil {
				return nil, &config.ConfigError{
					Field: "state",
					Err:   fmt.Errorf("invalid desired state: unknown value %q: %w", state, err),
				}
			}
		}
	}

	wds := &WrappedDesiredState[TConfig]{
		Config: cfg,
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
//
// Do not override it. The supervisor and the collector type-assert its result.
// SetFrameworkState and SetActionHistory, which let a worker read
// GetFrameworkState and GetActionHistory during CollectObservedState, reach
// only a deps that has those methods. Worker-metric accumulation needs a
// MetricsRecorder method. A TDeps that embeds *deps.BaseDependencies has all
// three. A struct{} TDeps has none, so that worker records no counters of its
// own; a nil result behaves the same. Framework metrics and action history
// reach the Observation whatever TDeps is.
func (w *WorkerBase[TConfig, TStatus, TDeps]) GetDependenciesAny() any {
	w.mu.RLock()
	d := w.typedDeps
	w.mu.RUnlock()

	return d
}
