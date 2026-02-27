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
//   - state/*.go: Defines state machine states and transitions
//   - action/*.go: Deprecated actions retained for architecture test compliance (ENG-4265)
//   - snapshot/snapshot.go: Observed and desired state structures
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
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// CommunicatorWorker implements the FSM v2 Worker interface for channel-based synchronization.
type CommunicatorWorker struct {
	*helpers.BaseWorker[*CommunicatorDependencies]
}

// NewCommunicatorWorker creates a new Channel-based Communicator worker in Stopped state.
func NewCommunicatorWorker(
	id string,
	name string,
	transportParam transport.Transport,
	logger depspkg.FSMLogger,
	stateReader depspkg.StateReader,
) (*CommunicatorWorker, error) {
	workerType, err := storage.DeriveWorkerType[snapshot.CommunicatorObservedState]()
	if err != nil {
		return nil, fmt.Errorf("failed to derive worker type: %w", err)
	}

	identity := depspkg.Identity{
		ID:         id,
		Name:       name,
		WorkerType: workerType,
		// HierarchyPath is set by the supervisor when adding workers via factory.
	}

	dependencies := NewCommunicatorDependencies(transportParam, logger, stateReader, identity)

	return &CommunicatorWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
	}, nil
}

// TODO(ENG-4265): Remove deprecated fields (JWTToken, JWTExpiry, Authenticated,
// AuthenticatedUUID, MessagesReceived, ConsecutiveErrors, IsBackpressured) from
// CommunicatorObservedState. These are now tracked by TransportWorker.

// CollectObservedState returns the current observed state of the communicator.
func (w *CommunicatorWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	deps := w.GetDependencies()

	jwtToken := deps.GetJWTToken()
	jwtExpiry := deps.GetJWTExpiry()

	pulledMessages := deps.GetPulledMessages()

	var messagesReceived []transport.UMHMessage
	if pulledMessages != nil {
		messagesReceived = make([]transport.UMHMessage, len(pulledMessages))
		for i, msg := range pulledMessages {
			if msg != nil {
				messagesReceived[i] = *msg
			}
		}
	}

	authenticated := jwtToken != "" && !time.Now().After(jwtExpiry)

	consecutiveErrors := deps.GetConsecutiveErrors()
	degradedEnteredAt := deps.GetDegradedEnteredAt()

	lastErrorType := deps.GetLastErrorType()
	lastRetryAfter := deps.GetLastRetryAfter()
	lastAuthAttemptAt := deps.GetLastAuthAttemptAt()
	lastErrorAt := deps.RetryTracker().LastError().OccurredAt

	var prevWorkerMetrics depspkg.Metrics

	stateReader := deps.GetStateReader()
	if stateReader != nil {
		var prev snapshot.CommunicatorObservedState
		if err := stateReader.LoadObservedTyped(ctx, deps.GetWorkerType(), deps.GetWorkerID(), &prev); err != nil {
			deps.GetLogger().Debug("observed_state_load_failed", depspkg.Err(err))
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

	tickMetrics := deps.MetricsRecorder().Drain()

	for name, delta := range tickMetrics.Counters {
		newWorkerMetrics.Counters[name] += delta
	}

	for name, value := range tickMetrics.Gauges {
		newWorkerMetrics.Gauges[name] = value
	}

	newWorkerMetrics.Gauges[string(depspkg.GaugeConsecutiveErrors)] = float64(consecutiveErrors)

	authenticatedUUID := deps.GetAuthenticatedUUID()

	metricsContainer := depspkg.MetricsContainer{
		Worker: newWorkerMetrics,
	}

	if fm := deps.GetFrameworkState(); fm != nil {
		metricsContainer.Framework = *fm
	}

	observed := snapshot.CommunicatorObservedState{
		CollectedAt:       time.Now(),
		JWTToken:          jwtToken,
		JWTExpiry:         jwtExpiry,
		AuthenticatedUUID: authenticatedUUID,
		MessagesReceived:  messagesReceived,
		Authenticated:     authenticated,
		ConsecutiveErrors: consecutiveErrors,
		DegradedEnteredAt: degradedEnteredAt,
		LastErrorType:     lastErrorType,
		LastRetryAfter:    lastRetryAfter,
		LastErrorAt:       lastErrorAt,
		LastAuthAttemptAt: lastAuthAttemptAt,
		IsBackpressured:   deps.IsBackpressured(),
		MetricsEmbedder:   depspkg.MetricsEmbedder{Metrics: metricsContainer},
	}

	return observed, nil
}

// DeriveDesiredState parses UserSpec.Config YAML into typed CommunicatorDesiredState.
func (w *CommunicatorWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &snapshot.CommunicatorDesiredState{
			BaseDesiredState: fsmv2types.BaseDesiredState{
				State: "running",
			},
			ChildrenSpecs: makeTransportChildSpec(fsmv2types.UserSpec{}),
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

	var commSpec CommunicatorUserSpec
	if err := yaml.Unmarshal([]byte(renderedConfig), &commSpec); err != nil {
		return nil, fmt.Errorf("config parse failed: %w", err)
	}

	if commSpec.Timeout == 0 {
		// Default to LongPollingDuration + buffer to prevent premature action timeouts
		commSpec.Timeout = httpTransport.LongPollingDuration + httpTransport.LongPollingBuffer
	}

	return &snapshot.CommunicatorDesiredState{
		BaseDesiredState: fsmv2types.BaseDesiredState{
			State: commSpec.GetState(),
		},
		RelayURL:      commSpec.RelayURL,
		InstanceUUID:  commSpec.InstanceUUID,
		AuthToken:     commSpec.AuthToken,
		Timeout:       commSpec.Timeout,
		ChildrenSpecs: makeTransportChildSpec(userSpec),
	}, nil
}

// makeTransportChildSpec creates the ChildSpec for the TransportWorker child.
// TransportWorker handles authentication, push, and pull operations.
// ChildStartStates includes both Syncing and Recovering so the child remains
// running during error recovery and does not restart on parent state oscillation.
func makeTransportChildSpec(parentSpec fsmv2types.UserSpec) []fsmv2types.ChildSpec {
	return []fsmv2types.ChildSpec{{
		Name:             "transport",
		WorkerType:       "transport",
		UserSpec:         parentSpec,
		ChildStartStates: []string{"Syncing", "Recovering"},
	}}
}

// GetInitialState returns StoppedState as the initial FSM state.
func (w *CommunicatorWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	if err := factory.RegisterWorkerType[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](
		func(id depspkg.Identity, logger depspkg.FSMLogger, stateReader depspkg.StateReader, _ map[string]any) fsmv2.Worker {
			// ChannelProvider must be set via global singleton before factory is called (will panic if not set).
			// Transport creation and auth are handled by TransportWorker (ENG-4264).
			commDeps := NewCommunicatorDependencies(nil, logger, stateReader, id)

			return &CommunicatorWorker{
				BaseWorker: helpers.NewBaseWorker(commDeps),
			}
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(fmt.Sprintf("failed to register communicator worker: %v", err))
	}
}
