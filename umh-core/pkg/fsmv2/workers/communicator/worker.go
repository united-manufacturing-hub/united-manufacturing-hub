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
// # FSM v2 Pattern
//
// This package follows the FSM v2 pattern:
//   - worker.go: Implements Worker interface (CollectObservedState, DeriveDesiredState)
//   - config.go: CommunicatorConfig and CommunicatorStatus types
//   - state/*.go: Defines state machine states and transitions
//
// # States and Transitions
//
// State flow:
//
//	Stopped → Syncing ↔ Recovering → Stopped
//
// TransportWorker runs as a child when the parent is in Syncing or Recovering.
//
// Actions by state:
//   - Syncing: Monitors child health. Transitions to Recovering when any child is unhealthy.
//   - Recovering: Waits for children to recover. Transitions to Syncing when all children are healthy.
//   - Stopped: Transitions to Syncing on start, or emits SignalNeedsRemoval on shutdown.
package communicator

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

const workerTypeName = "communicator"

// CommunicatorWorker implements the FSM v2 Worker interface for channel-based synchronization.
type CommunicatorWorker struct {
	fsmv2.WorkerBase[CommunicatorConfig, CommunicatorStatus, *CommunicatorDependencies]
}

// NewCommunicatorWorker creates a new Channel-based Communicator worker in Stopped state.
// The supervisor sets HierarchyPath on identity before instantiation; tests inject a
// transport via transportParam (the factory path passes nil  -  transport is owned by
// the TransportWorker child, ENG-4264).
func NewCommunicatorWorker(
	identity depspkg.Identity,
	transportParam types.Transport,
	logger depspkg.FSMLogger,
	stateReader depspkg.StateReader,
) (*CommunicatorWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerTypeName
	}

	w := &CommunicatorWorker{}
	bd := w.InitBase(identity, logger, stateReader)

	dependencies := NewCommunicatorDependencies(transportParam, bd)
	w.BindDeps(dependencies)

	return w, nil
}

// GetDependencies returns the typed CommunicatorDependencies.
// Panics with a clear message if BindDeps was not called before this worker is used.
func (w *CommunicatorWorker) GetDependencies() *CommunicatorDependencies {
	raw := w.GetDependenciesAny()

	d, ok := raw.(*CommunicatorDependencies)
	if !ok || d == nil {
		panic("CommunicatorWorker: GetDependencies called before BindDeps")
	}

	return d
}

// CollectObservedState returns the current observed state of the communicator.
// Returns NewObservation; the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically.
func (w *CommunicatorWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	d := w.GetDependencies()

	d.MetricsRecorder().SetGauge(depspkg.GaugeConsecutiveErrors, float64(d.GetConsecutiveErrors()))

	status := CommunicatorStatus{
		DegradedEnteredAt: d.GetDegradedEnteredAt(),
	}

	return fsmv2.NewObservation(status), nil
}

// DeriveDesiredState parses UserSpec.Config YAML into typed WrappedDesiredState[CommunicatorConfig].
func (w *CommunicatorWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[CommunicatorConfig]{
			State: fsmv2types.DesiredStateRunning,
			Config: CommunicatorConfig{
				Timeout: httpTransport.LongPollingDuration + httpTransport.LongPollingBuffer,
			},
		}, nil
	}

	userSpec, ok := spec.(fsmv2types.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	renderedConfig, err := fsmv2types.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	var commSpec CommunicatorConfig
	if err := yaml.Unmarshal([]byte(renderedConfig), &commSpec); err != nil {
		return nil, fmt.Errorf("config parse failed: %w", err)
	}

	if commSpec.Timeout == 0 {
		commSpec.Timeout = httpTransport.LongPollingDuration + httpTransport.LongPollingBuffer
	}

	return &fsmv2.WrappedDesiredState[CommunicatorConfig]{
		State:  commSpec.GetState(),
		Config: commSpec,
	}, nil
}

func init() {
	register.Worker[CommunicatorConfig, CommunicatorStatus, *CommunicatorDependencies](workerTypeName,
		func(id depspkg.Identity, logger depspkg.FSMLogger, sr depspkg.StateReader) (fsmv2.Worker, error) {
			// ChannelProvider must be set via global singleton before factory is called (will panic if not set).
			// Transport creation and auth are handled by TransportWorker (ENG-4264).
			return NewCommunicatorWorker(id, nil, logger, sr)
		})
}
