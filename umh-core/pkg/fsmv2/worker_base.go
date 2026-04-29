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

// WorkerBase provides default implementations for the Worker interface.
// Developers embed this in their worker struct and only implement
// CollectObservedState (the business logic).
//
// WorkerBase does NOT implement any optional capability interface (BW1).
// It implements: Worker (2 of 3 methods: DeriveDesiredState, GetInitialState).
// The developer must implement CollectObservedState.
// It also implements DependencyProvider (1 method).
type WorkerBase[TConfig any, TStatus any] struct {
	logger        deps.FSMLogger
	config        TConfig
	baseDeps      *deps.BaseDependencies
	postParseHook func(*TConfig) error
	identity      deps.Identity
	mu            sync.RWMutex
	configReady   bool
	initialized   bool
}

// InitBase initializes the embedded WorkerBase with framework dependencies.
// Returns the BaseDependencies instance that the collector reads from. Workers
// that construct custom dependencies MUST use this returned pointer — do not call
// deps.NewBaseDependencies separately, as a separate instance is invisible to the collector.
func (w *WorkerBase[TConfig, TStatus]) InitBase(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) *deps.BaseDependencies {
	bd := deps.NewBaseDependencies(logger, sr, id)

	w.mu.Lock()
	w.identity = id
	w.logger = logger
	w.baseDeps = bd
	w.initialized = true
	w.mu.Unlock()

	return bd
}

// SetPostParseHook registers a hook called after config parsing in DeriveDesiredState.
// The hook receives a pointer to the parsed config and may modify or validate it.
// Must be called in the constructor, before any DeriveDesiredState call.
func (w *WorkerBase[TConfig, TStatus]) SetPostParseHook(hook func(*TConfig) error) {
	w.postParseHook = hook
}

// Config returns the cached TConfig from the last DeriveDesiredState call.
// Returns zero-value TConfig before first DDS (BW4).
func (w *WorkerBase[TConfig, TStatus]) Config() TConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.config
}

// ConfigReady returns true after the first successful DeriveDesiredState call.
func (w *WorkerBase[TConfig, TStatus]) ConfigReady() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.configReady
}

// DeriveDesiredState parses UserSpec, renders templates, unmarshals into TConfig,
// caches the config (write-lock), and wraps in WrappedDesiredState.
func (w *WorkerBase[TConfig, TStatus]) DeriveDesiredState(spec interface{}) (DesiredState, error) {
	if spec == nil {
		var cfg TConfig
		if err := w.runPostParseHook(&cfg); err != nil {
			return nil, err
		}

		w.mu.Lock()
		w.config = cfg
		w.configReady = true
		w.mu.Unlock()

		wds := &WrappedDesiredState[TConfig]{
			Config: cfg,
		}

		return wds, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
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

	if err := w.runPostParseHook(&cfg); err != nil {
		return nil, err
	}

	w.mu.Lock()
	w.config = cfg
	w.configReady = true
	w.mu.Unlock()

	wds := &WrappedDesiredState[TConfig]{
		Config: cfg,
	}

	return wds, nil
}

func (w *WorkerBase[TConfig, TStatus]) runPostParseHook(cfg *TConfig) error {
	if w.postParseHook != nil {
		if err := w.postParseHook(cfg); err != nil {
			return fmt.Errorf("post-parse hook failed: %w", err)
		}
	}
	return nil
}

// GetInitialState returns the registered initial state for this worker type.
// Panics if no state is registered — call fsmv2.RegisterInitialState in
// the state package init() function.
func (w *WorkerBase[TConfig, TStatus]) GetInitialState() State[any, any] {
	if !w.initialized {
		panic("WorkerBase.GetInitialState: InitBase was not called — ensure your constructor calls w.InitBase(id, logger, sr)")
	}
	wt := w.identity.WorkerType
	s := LookupInitialState(wt)
	if s == nil {
		panic(fmt.Sprintf("WorkerBase.GetInitialState: no initial state registered for worker type %q — call fsmv2.RegisterInitialState in your state package init()", wt))
	}
	return s
}

// Identity returns the worker's identity.
func (w *WorkerBase[TConfig, TStatus]) Identity() deps.Identity {
	return w.identity
}

// Logger returns the worker's logger.
func (w *WorkerBase[TConfig, TStatus]) Logger() deps.FSMLogger {
	return w.logger
}

// GetDependenciesAny returns the base dependencies. Satisfies DependencyProvider.
func (w *WorkerBase[TConfig, TStatus]) GetDependenciesAny() any {
	return w.baseDeps
}
