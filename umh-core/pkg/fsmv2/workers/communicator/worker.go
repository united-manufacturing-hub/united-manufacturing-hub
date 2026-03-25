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

// Package communicator implements the CommunicatorWorker, a parent orchestrator
// that manages bidirectional message exchange between Edge and Backend tiers.
//
// # Architecture
//
// CommunicatorWorker delegates all transport operations to a TransportWorker child.
// TransportWorker handles authentication, push, pull, backoff, and transport reset.
// CommunicatorWorker monitors child health and manages lifecycle transitions.
//
// Channel sharing: Both communicator and transport packages use a ChannelProvider
// singleton to supply inbound and outbound message channels. Call
// communicator.SetChannelProvider() and transport.SetChannelProvider() before
// starting the supervisor.
//
// # Worker API v2
//
// This package uses the WorkerBase[TConfig, TStatus] API with custom overrides:
//   - Custom CollectObservedState: reads previous metrics from CSE for accumulation
//   - Custom DeriveDesiredState: constructs TransportWorker child specs
//   - RegisterInitialState pattern: breaks circular import with state package
//
// # States and Transitions
//
// State flow:
//
//	Stopped → Syncing ↔ Recovering → Stopped
//
// TransportWorker runs as a child when the parent is in Syncing or Recovering.
package communicator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// registeredInitialState holds the worker-specific initial state, set by the state
// package via RegisterInitialState. This avoids a circular import (worker -> state -> worker).
var registeredInitialState fsmv2.State[any, any]

// RegisterInitialState sets the worker-specific initial state.
// Called from the state package's init() to break the circular import.
func RegisterInitialState(s fsmv2.State[any, any]) {
	registeredInitialState = s
}

// CommunicatorWorker implements the FSMv2 Worker interface using the WorkerBase API.
type CommunicatorWorker struct {
	deps *CommunicatorDependencies
	fsmv2.WorkerBase[CommunicatorConfig, CommunicatorStatus]
}

// NewCommunicatorWorker creates a new communicator worker with the standard framework dependencies.
func NewCommunicatorWorker(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	w := &CommunicatorWorker{}
	w.InitBase(id, logger, sr)

	// Share WorkerBase's BaseDependencies to avoid dual-instance metrics divergence.
	baseDeps := w.WorkerBase.GetDependenciesAny().(*deps.BaseDependencies)
	w.deps = NewCommunicatorDependencies(baseDeps)

	return w, nil
}

// GetDependencies returns the typed communicator dependencies.
func (w *CommunicatorWorker) GetDependencies() *CommunicatorDependencies {
	return w.deps
}

// GetDependenciesAny returns the worker's dependencies for action execution.
// Overrides WorkerBase.GetDependenciesAny to return *CommunicatorDependencies
// instead of *deps.BaseDependencies.
func (w *CommunicatorWorker) GetDependenciesAny() any {
	return w.deps
}

// CollectObservedState returns the current observed state of the communicator.
// Constructs WrappedObservedState manually instead of using WrapStatus because
// the communicator accumulates metrics across ticks by reading previous state from CSE.
func (w *CommunicatorWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.deps

	consecutiveErrors := d.GetConsecutiveErrors()
	degradedEnteredAt := d.GetDegradedEnteredAt()

	var prevWorkerMetrics deps.Metrics

	sr := d.GetStateReader()
	if sr != nil {
		var prev fsmv2.WrappedObservedState[CommunicatorStatus]
		if err := sr.LoadObservedTyped(ctx, d.GetWorkerType(), d.GetWorkerID(), &prev); err != nil {
			d.GetLogger().Debug("observed_state_load_failed", deps.Err(err))
		} else {
			prevWorkerMetrics = prev.Metrics.Worker
		}
	}

	newWorkerMetrics := prevWorkerMetrics

	if newWorkerMetrics.Counters == nil {
		newWorkerMetrics.Counters = make(map[string]int64)
	}

	if newWorkerMetrics.Gauges == nil {
		newWorkerMetrics.Gauges = make(map[string]float64)
	}

	tickMetrics := d.MetricsRecorder().Drain()

	for name, delta := range tickMetrics.Counters {
		newWorkerMetrics.Counters[name] += delta
	}

	for name, value := range tickMetrics.Gauges {
		newWorkerMetrics.Gauges[name] = value
	}

	newWorkerMetrics.Gauges[string(deps.GaugeConsecutiveErrors)] = float64(consecutiveErrors)

	metricsContainer := deps.MetricsContainer{
		Worker: newWorkerMetrics,
	}

	if fm := d.GetFrameworkState(); fm != nil {
		metricsContainer.Framework = *fm
	}

	status := CommunicatorStatus{
		ConsecutiveErrors: consecutiveErrors,
		DegradedEnteredAt: degradedEnteredAt,
	}

	observed := fsmv2.WrappedObservedState[CommunicatorStatus]{
		CollectedAt:     time.Now(),
		Status:          status,
		MetricsEmbedder: deps.MetricsEmbedder{Metrics: metricsContainer},
	}

	if actionHistory := d.GetActionHistory(); actionHistory != nil {
		observed.LastActionResults = actionHistory
	}

	return observed, nil
}

// DeriveDesiredState parses UserSpec.Config YAML into typed CommunicatorConfig
// and constructs the TransportWorker child spec.
func (w *CommunicatorWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[CommunicatorConfig]{
			BaseDesiredState: config.BaseDesiredState{
				State: config.DesiredStateRunning,
			},
			ChildrenSpecs: makeTransportChildSpec(config.UserSpec{}),
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

	var cfg CommunicatorConfig
	if err := yaml.Unmarshal([]byte(renderedConfig), &cfg); err != nil {
		return nil, fmt.Errorf("config parse failed: %w", err)
	}

	if cfg.Timeout == 0 {
		// Default to LongPollingDuration + buffer to prevent premature action timeouts
		cfg.Timeout = httpTransport.LongPollingDuration + httpTransport.LongPollingBuffer
	}

	desiredState := config.DesiredStateRunning
	if s := cfg.GetState(); s != "" {
		desiredState = s
	}

	return &fsmv2.WrappedDesiredState[CommunicatorConfig]{
		BaseDesiredState: config.BaseDesiredState{
			State: desiredState,
		},
		Config:        cfg,
		ChildrenSpecs: makeTransportChildSpec(userSpec),
	}, nil
}

// makeTransportChildSpec creates the ChildSpec for the TransportWorker child.
// TransportWorker handles authentication, push, and pull operations.
// ChildStartStates includes both Syncing and Recovering so the child remains
// running during error recovery and does not restart on parent state oscillation.
func makeTransportChildSpec(parentSpec config.UserSpec) []config.ChildSpec {
	return []config.ChildSpec{{
		Name:             "transport",
		WorkerType:       "transport",
		UserSpec:         parentSpec,
		ChildStartStates: []string{"Syncing", "Recovering"},
	}}
}

// GetInitialState returns the worker-specific StoppedState that knows how to
// transition to SyncingState. Falls back to WorkerBase's default if
// the state package hasn't registered itself.
func (w *CommunicatorWorker) GetInitialState() fsmv2.State[any, any] {
	if registeredInitialState != nil {
		return registeredInitialState
	}

	return w.WorkerBase.GetInitialState()
}

func init() {
	register.Worker[CommunicatorConfig, CommunicatorStatus]("communicator", NewCommunicatorWorker)
}
