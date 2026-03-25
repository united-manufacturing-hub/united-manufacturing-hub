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
	"time"

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
	logger      deps.FSMLogger
	stateReader deps.StateReader
	config      TConfig
	baseDeps    *deps.BaseDependencies
	identity    deps.Identity
	mu          sync.RWMutex
	configReady bool
	initialized bool
}

// InitBase initializes the embedded WorkerBase with framework dependencies.
// Returns the BaseDependencies instance that WrapStatus reads from. Workers
// that construct custom dependencies MUST use this returned pointer — do not call
// deps.NewBaseDependencies separately, as a separate instance is invisible to WrapStatus.
func (w *WorkerBase[TConfig, TStatus]) InitBase(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) *deps.BaseDependencies {
	w.identity = id
	w.logger = logger
	w.stateReader = sr
	bd := deps.NewBaseDependencies(logger, sr, id)

	w.mu.Lock()
	w.baseDeps = bd
	w.initialized = true
	w.mu.Unlock()

	return bd
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

// WrapStatus constructs a WrappedObservedState from the developer's TStatus.
// Sets CollectedAt to time.Now() (BW2), copies framework metrics and action history from baseDeps (BW3).
// Safe to call on uninitialized WorkerBase (returns observation with zero metrics, no panic).
func (w *WorkerBase[TConfig, TStatus]) WrapStatus(status TStatus) ObservedState {
	obs := WrappedObservedState[TStatus]{
		CollectedAt: time.Now(),
		Status:      status,
	}

	w.mu.RLock()
	initialized := w.initialized
	bd := w.baseDeps
	w.mu.RUnlock()

	if initialized && bd != nil {
		if fm := bd.GetFrameworkState(); fm != nil {
			obs.Metrics.Framework = *fm
		}

		actionHistory := bd.GetActionHistory()
		if actionHistory != nil {
			obs.LastActionResults = actionHistory
		}

		if recorder := bd.MetricsRecorder(); recorder != nil {
			drained := recorder.Drain()
			obs.Metrics.Worker = deps.Metrics{
				Counters: drained.Counters,
				Gauges:   drained.Gauges,
			}
		}
	}

	return obs
}

// DeriveDesiredState parses UserSpec, renders templates, unmarshals into TConfig,
// caches the config (write-lock), and wraps in WrappedDesiredState.
func (w *WorkerBase[TConfig, TStatus]) DeriveDesiredState(spec interface{}) (DesiredState, error) {
	if spec == nil {
		w.mu.Lock()
		w.configReady = true
		w.mu.Unlock()

		return &WrappedDesiredState[TConfig]{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
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

	var cfg TConfig
	if renderedConfig != "" {
		if err := yaml.Unmarshal([]byte(renderedConfig), &cfg); err != nil {
			return nil, fmt.Errorf("config unmarshal failed: %w", err)
		}
	}

	// Extract state from config if it implements StateGetter (e.g., embeds BaseUserSpec).
	desiredState := config.DesiredStateRunning
	if sg, ok := any(&cfg).(config.StateGetter); ok {
		if s := sg.GetState(); s != "" {
			desiredState = s
		}
	}

	w.mu.Lock()
	w.config = cfg
	w.configReady = true
	w.mu.Unlock()

	return &WrappedDesiredState[TConfig]{
		BaseDesiredState: config.BaseDesiredState{State: desiredState},
		Config:           cfg,
	}, nil
}

// GetInitialState returns the registered initial state for this worker type.
// Panics if no state is registered — call fsmv2.RegisterInitialState in
// the state package init() function.
func (w *WorkerBase[TConfig, TStatus]) GetInitialState() State[any, any] {
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
