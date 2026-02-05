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

// Package communicator implements the Channel Communicator FSM worker for
// bidirectional message exchange between Edge and Backend tiers.
//
// # Architecture
//
// The Communicator FSM manages the complete lifecycle of channel-based sync:
//  1. Authentication: Obtain JWT token from relay server
//  2. Synchronization: Push/pull messages via HTTP transport
//  3. Connection management: Handle network failures and reconnects
//
// # FSM v2 Pattern
//
// This package follows the FSM v2 pattern:
//   - worker.go: Implements Worker interface (CollectObservedState, DeriveDesiredState)
//   - states.go: Defines state machine states and transitions
//   - action_*.go: Idempotent actions executed during state transitions
//   - models.go: Observed and desired state structures
//
// The FSM v2 Supervisor manages the worker, calling CollectObservedState
// periodically and executing actions when state transitions occur.
//
// # States and Transitions
//
// State flow:
//
//	Stopped → Authenticating → Authenticated → Syncing → Syncing (loop)
//	   ↓            ↓               ↓             ↓
//	  Error ← ─── Error ← ─────── Error ← ───── Error
//
// Actions by state:
//   - Authenticating: AuthenticateAction obtains JWT token
//   - Syncing: SyncAction performs push/pull operations via HTTP
//
// # Integration with Channel Protocol
//
// The worker operates using a 2-goroutine pattern with channels:
//
// Inbound flow (backend → edge):
//   - Pull goroutine: HTTPTransport.Pull() → inboundChan
//   - Messages arrive from backend and are queued for local processing
//
// Outbound flow (edge → backend):
//   - Push goroutine: outboundChan → HTTPTransport.Push()
//   - Messages from edge are batched and sent to backend
//
// HTTPTransport handles:
//   - HTTP POST/GET operations to relay server
//   - JWT token management and refresh
//   - Network error handling and retries
//
// This channel-based approach eliminates:
//   - CSE delta sync and conflict resolution
//   - Distributed state synchronization
//   - Bidirectional merge logic
package communicator

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
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
	logger *zap.SugaredLogger,
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
		if err := stateReader.LoadObservedTyped(ctx, deps.GetWorkerType(), deps.GetWorkerID(), &prev); err == nil {
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
		RelayURL:     commSpec.RelayURL,
		InstanceUUID: commSpec.InstanceUUID,
		AuthToken:    commSpec.AuthToken,
		Timeout:      commSpec.Timeout,
	}, nil
}

// GetInitialState returns StoppedState as the initial FSM state.
func (w *CommunicatorWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	if err := factory.RegisterWorkerType[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](
		func(id depspkg.Identity, logger *zap.SugaredLogger, stateReader depspkg.StateReader, _ map[string]any) fsmv2.Worker {
			// ChannelProvider must be set via global singleton before factory is called (will panic if not set).
			// Transport is created lazily by AuthenticateAction.
			commDeps := NewCommunicatorDependencies(nil, logger, stateReader, id)

			worker, err := NewCommunicatorWorker(id.ID, id.Name, nil, logger, stateReader)
			if err != nil {
				panic(fmt.Sprintf("failed to create communicator worker: %v", err))
			}
			worker.BaseWorker = helpers.NewBaseWorker(commDeps)

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(fmt.Sprintf("failed to register communicator worker: %v", err))
	}
}
