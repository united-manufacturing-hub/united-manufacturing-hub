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
	"context"
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
	logger           deps.FSMLogger
	stateReader      deps.StateReader
	config           TConfig
	baseDeps         *deps.BaseDependencies
	postParseHook    func(*TConfig) error
	childSpecFactory func(TConfig, config.UserSpec) []config.ChildSpec
	identity         deps.Identity
	mu               sync.RWMutex
	configReady      bool
	initialized      bool
}

// InitBase initializes the embedded WorkerBase with framework dependencies.
// Returns the BaseDependencies instance that WrapStatus reads from. Workers
// that construct custom dependencies MUST use this returned pointer — do not call
// deps.NewBaseDependencies separately, as a separate instance is invisible to WrapStatus.
func (w *WorkerBase[TConfig, TStatus]) InitBase(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) *deps.BaseDependencies {
	bd := deps.NewBaseDependencies(logger, sr, id)

	w.mu.Lock()
	w.identity = id
	w.logger = logger
	w.stateReader = sr
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

// SetChildSpecsFactory registers a factory that produces child specifications
// from the parsed config and raw UserSpec. The raw spec is needed so children
// can run their own DeriveDesiredState with template variables intact.
// Called after the post-parse hook in DeriveDesiredState.
// Must be called in the constructor, before any DeriveDesiredState call.
func (w *WorkerBase[TConfig, TStatus]) SetChildSpecsFactory(factory func(TConfig, config.UserSpec) []config.ChildSpec) {
	w.childSpecFactory = factory
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

// Deprecated: Use fsmv2.NewObservation(status) instead. The collector now handles
// CollectedAt, framework metrics, action history, and metric accumulation automatically
// after CollectObservedState returns (post-COS wrapping).
//
// WrapStatus constructs an Observation from the developer's TStatus.
// Sets CollectedAt to time.Now() (BW2), copies framework metrics and action history from baseDeps (BW3).
// Safe to call on uninitialized WorkerBase (returns observation with zero metrics, no panic).
func (w *WorkerBase[TConfig, TStatus]) WrapStatus(status TStatus) ObservedState {
	obs := Observation[TStatus]{
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

// Deprecated: Use fsmv2.NewObservation(status) instead. The collector now handles
// metric accumulation (load previous from CSE, drain recorder, merge) automatically
// after CollectObservedState returns (post-COS wrapping).
//
// WrapStatusAccumulated constructs an Observation with cross-tick metric accumulation.
// Unlike WrapStatus (current-tick only), this reads the previous observed state from CSE,
// merges counters (additive) and gauges (replace), then drains the current tick's metrics on top.
//
// Use for workers that accumulate metrics across ticks (communicator, transport push/pull).
// Workers without cross-tick metrics should use WrapStatus instead.
//
// Calls MetricsRecorder().Drain() which is destructive — calling this method
// twice in the same tick yields zero deltas on the second call.
func (w *WorkerBase[TConfig, TStatus]) WrapStatusAccumulated(ctx context.Context, status TStatus) ObservedState {
	obs := Observation[TStatus]{
		CollectedAt: time.Now(),
		Status:      status,
	}

	w.mu.RLock()
	initialized := w.initialized
	bd := w.baseDeps
	w.mu.RUnlock()

	if !initialized || bd == nil {
		return obs
	}

	if fm := bd.GetFrameworkState(); fm != nil {
		obs.Metrics.Framework = *fm
	}

	if ah := bd.GetActionHistory(); ah != nil {
		obs.LastActionResults = ah
	}

	// Step 1: Load previous state FIRST (before drain).
	// Drain is destructive — if we drain first and the CSE read then fails,
	// the drained deltas are lost and the cumulative counter resets to zero.
	var prevWorkerMetrics deps.Metrics
	if sr := bd.GetStateReader(); sr != nil && ctx.Err() == nil {
		var prev Observation[TStatus]
		if err := sr.LoadObservedTyped(ctx, bd.GetWorkerType(), bd.GetWorkerID(), &prev); err == nil {
			prevWorkerMetrics = prev.Metrics.Worker
		} else if w.logger != nil {
			w.logger.Debug("WrapStatusAccumulated: could not load previous state, starting fresh: " + err.Error())
		}
	}

	if prevWorkerMetrics.Counters == nil {
		prevWorkerMetrics.Counters = make(map[string]int64)
	}
	if prevWorkerMetrics.Gauges == nil {
		prevWorkerMetrics.Gauges = make(map[string]float64)
	}

	// Step 2: Drain AFTER CSE read — if CSE failed, deltas are still correct.
	if recorder := bd.MetricsRecorder(); recorder != nil {
		drained := recorder.Drain()
		for name, delta := range drained.Counters {
			prevWorkerMetrics.Counters[name] += delta
		}
		for name, value := range drained.Gauges {
			prevWorkerMetrics.Gauges[name] = value
		}
	}

	obs.Metrics.Worker = prevWorkerMetrics

	return obs
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
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
			Config:           cfg,
		}
		w.populateChildrenSpecs(wds, cfg, config.UserSpec{})

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

	wds := &WrappedDesiredState[TConfig]{
		BaseDesiredState: config.BaseDesiredState{State: desiredState},
		Config:           cfg,
	}
	w.populateChildrenSpecs(wds, cfg, userSpec)

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

func (w *WorkerBase[TConfig, TStatus]) populateChildrenSpecs(wds *WrappedDesiredState[TConfig], cfg TConfig, spec config.UserSpec) {
	if w.childSpecFactory != nil {
		wds.ChildrenSpecs = w.childSpecFactory(cfg, spec)
	}
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
