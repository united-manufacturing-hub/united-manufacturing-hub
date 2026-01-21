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

// =============================================================================
// COMMUNICATOR INVARIANTS
// =============================================================================
//
// C1: Authentication precedence
//     MUST: Cannot sync before authentication completes
//     WHY:  JWT token required for all sync operations
//     ENFORCED: State machine transitions (Stopped → Authenticating → Syncing)
//
// C2: Token expiry handling
//     MUST: Check JWT expiry before every sync operation
//     WHY:  Expired tokens cause 401 errors and sync failures
//     ENFORCED: SyncAction checks JWTExpiry before transport.Push/Pull
//
// C3: Transport lifecycle
//     MUST: Transport must be set before syncing operations
//     WHY:  All sync actions depend on transport for HTTP communication
//     ENFORCED: AuthenticateAction creates transport on first execution
//     GUARANTEE: After AuthenticateAction.Execute() runs, transport is non-nil
//     IMPLICATION: SyncAction, ResetTransportAction can assume transport is non-nil
//                  (they only execute after authentication has completed)
//
// C4: Shutdown check priority
//     MUST: Check shutdown signal before processing any action
//     WHY:  Prevents partial operations during graceful shutdown
//     ENFORCED: All states check signal in Next() before returning actions
//
// C5: Syncing state loop
//     MUST: SyncingState returns self on success (loops indefinitely)
//     WHY:  Continuous bidirectional sync is the primary operational mode
//     ENFORCED: SyncingState.Next() returns (self, SignalNone, SyncAction)
//
// Safety layers:
//   - Layer 1: Type system (Go's strong typing, interface contracts)
//   - Layer 2: Runtime checks (state machine transitions, nil validation)
//   - Layer 3: Tests (verify invariants hold under all conditions)
//   - Layer 4: Monitoring (detect violations in production via status messages)
//
// =============================================================================

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// CommunicatorWorker implements the FSM v2 Worker interface for channel-based synchronization.
//
// It manages the lifecycle of bidirectional message exchange between Edge and Backend tiers,
// handling authentication, connection management, and HTTP push/pull operations.
//
// # Responsibilities
//
// State collection (CollectObservedState):
//   - Returns shared CommunicatorObservedState
//   - Updated by actions (AuthenticateAction sets JWT token)
//   - Read by states to determine transitions
//   - Monitors inbound/outbound channel queue sizes
//
// State derivation (DeriveDesiredState):
//   - Always returns "not shutdown" for MVP
//   - Future: may derive desired state from config
//
// Initial state (GetInitialState):
//   - Returns StoppedState
//   - FSM starts in Stopped and transitions to Authenticating
//
// # Shared State Pattern
//
// The worker shares CommunicatorObservedState between actions and states:
//   - Actions write to observedState (e.g., SetJWTToken)
//   - States read from observedState (e.g., IsAuthenticated)
//   - Worker returns observedState in CollectObservedState
//
// Actions communicate results to states without coupling.
//
// # Dependencies
//
// The worker is constructed with:
//   - inboundChan: Receives messages from backend (via HTTP pull)
//   - outboundChan: Sends messages to backend (via HTTP push)
//   - transport: HTTPTransport for push/pull operations
//   - relayURL: Relay server endpoint for authentication
//   - instanceUUID: Identifies this Edge instance
//   - authToken: Pre-shared secret for authentication
//   - logger: Structured logging
//
// These dependencies are passed to actions when created by states.
type CommunicatorWorker struct {
	*helpers.BaseWorker[*CommunicatorDependencies]
	identity fsmv2.Identity

	// Temporary State
	// Temporary state can be a local logger, or some HTTP client, or some local services
	// like filesystem, etc.
	// But everything that configures it or that needs to be exposed should be in observed and desired state
	logger *zap.SugaredLogger
}

// NewCommunicatorWorker creates a new Channel-based Communicator worker.
//
// Parameters:
//   - id: Unique identifier for this worker instance
//   - name: Human-readable name for this worker
//   - transport: HTTP transport for push/pull operations
//   - logger: Structured logging (must not be nil)
//
// The worker is created in Stopped state. Call supervisor.Start() to begin
// the authentication and sync lifecycle.
//
// Example:
//
//	httpTransport := transport.NewHTTPTransport("https://relay.umh.app")
//	worker, err := NewCommunicatorWorker(
//	    "communicator-1",
//	    "Communicator Worker",
//	    httpTransport,
//	    logger,
//	)
//	if err != nil {
//	    return fmt.Errorf("failed to create communicator worker: %w", err)
//	}
//	supervisor := fsmv2.NewSupervisor(worker, logger)
//	supervisor.Start(ctx)
func NewCommunicatorWorker(
	id string,
	name string,
	transportParam transport.Transport,
	logger *zap.SugaredLogger,
	stateReader fsmv2.StateReader,
) (*CommunicatorWorker, error) {
	workerType, err := storage.DeriveWorkerType[snapshot.CommunicatorObservedState]()
	if err != nil {
		return nil, fmt.Errorf("failed to derive worker type: %w", err)
	}

	// Create identity first so it can be used for dependency logging
	identity := fsmv2.Identity{
		ID:         id,
		Name:       name,
		WorkerType: workerType,
		// HierarchyPath will be empty here - it's set by the supervisor when adding
		// workers via factory. For directly constructed workers, fallback logging is used.
	}

	dependencies := NewCommunicatorDependencies(transportParam, logger, stateReader, identity)

	return &CommunicatorWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
	}, nil
}

// CollectObservedState returns the current observed state of the communicator.
//
// This method is called periodically by the FSM v2 Supervisor to determine
// what state the system is in. The returned state is used to decide which
// state transitions are possible.
//
// The observed state includes:
//   - CollectedAt: Timestamp of this observation
//
// Actions (AuthenticateAction, SyncAction) update the shared observedState,
// and this method returns it to the supervisor.
//
// Metrics accumulation:
//   - Reads previous metrics from the state store
//   - Gets this tick's results from dependencies
//   - Accumulates metrics (totals, counts, latency averages)
//   - Clears per-tick results for next cycle
//
// This method never returns an error for the communicator worker.
func (w *CommunicatorWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	deps := w.GetDependencies()

	// Read JWT token and expiry from dependencies
	jwtToken := deps.GetJWTToken()
	jwtExpiry := deps.GetJWTExpiry()

	// Read pulled messages from dependencies
	pulledMessages := deps.GetPulledMessages()

	// Convert []*transport.UMHMessage to []transport.UMHMessage for the snapshot
	var messagesReceived []transport.UMHMessage
	if pulledMessages != nil {
		messagesReceived = make([]transport.UMHMessage, len(pulledMessages))
		for i, msg := range pulledMessages {
			if msg != nil {
				messagesReceived[i] = *msg
			}
		}
	}

	// Determine authentication status: authenticated if token is present and not expired
	authenticated := jwtToken != "" && !time.Now().After(jwtExpiry)

	// Read consecutive error count and degraded entry time from dependencies
	consecutiveErrors := deps.GetConsecutiveErrors()
	degradedEnteredAt := deps.GetDegradedEnteredAt()

	// Read error type tracking fields from dependencies (for intelligent backoff)
	lastErrorType := deps.GetLastErrorType()
	lastRetryAfter := deps.GetLastRetryAfter()
	lastAuthAttemptAt := deps.GetLastAuthAttemptAt()

	// Get previous metrics from store (persisted across restarts)
	var prevMetrics fsmv2.Metrics

	stateReader := deps.GetStateReader()
	if stateReader != nil {
		var prev snapshot.CommunicatorObservedState
		if err := stateReader.LoadObservedTyped(ctx, deps.GetWorkerType(), deps.GetWorkerID(), &prev); err == nil {
			prevMetrics = prev.Metrics
		}
	}

	// Initialize metrics from previous (for accumulation)
	newMetrics := prevMetrics

	if newMetrics.Counters == nil {
		newMetrics.Counters = make(map[string]int64)
	}

	if newMetrics.Gauges == nil {
		newMetrics.Gauges = make(map[string]float64)
	}

	// Drain this tick's buffered metrics from MetricsRecorder
	tickMetrics := deps.Metrics().Drain()

	// Accumulate counters (add deltas to cumulative)
	for name, delta := range tickMetrics.Counters {
		newMetrics.Counters[name] += delta
	}

	// Set gauges (overwrite with latest values)
	for name, value := range tickMetrics.Gauges {
		newMetrics.Gauges[name] = value
	}

	// Also set consecutive errors as a gauge
	newMetrics.Gauges[string(metrics.GaugeConsecutiveErrors)] = float64(consecutiveErrors)

	// Clear per-tick results from legacy tracking (for backward compatibility with tests)
	// // TODO: wtf what backwards compatbitlity? remove it!
	deps.ClearSyncResults()

	// Read authenticated UUID from dependencies
	authenticatedUUID := deps.GetAuthenticatedUUID()

	observed := snapshot.CommunicatorObservedState{
		CollectedAt:       time.Now(),
		JWTToken:          jwtToken,
		JWTExpiry:         jwtExpiry,
		AuthenticatedUUID: authenticatedUUID,
		MessagesReceived:  messagesReceived,
		Authenticated:     authenticated,
		ConsecutiveErrors: consecutiveErrors,
		DegradedEnteredAt: degradedEnteredAt,
		// Error type tracking fields for intelligent backoff
		LastErrorType:     lastErrorType,
		LastRetryAfter:    lastRetryAfter,
		LastAuthAttemptAt: lastAuthAttemptAt,
		// Use MetricsEmbedder for both worker and framework metrics
		MetricsEmbedder: fsmv2.MetricsEmbedder{Metrics: newMetrics},
	}

	return observed, nil
}

// DeriveDesiredState determines what state the communicator should be in.
//
// This method parses the UserSpec.Config YAML into typed configuration
// and returns a typed CommunicatorDesiredState with all configuration fields populated.
// States access these typed fields directly (NO re-parsing of OriginalUserSpec).
//
// The spec parameter should be a fsmv2types.UserSpec containing the communicator config.
//
// Returns an error if config parsing fails.
func (w *CommunicatorWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &snapshot.CommunicatorDesiredState{
			BaseDesiredState: fsmv2types.BaseDesiredState{
				State: "running",
			},
		}, nil
	}

	// Cast spec to UserSpec to access Config and Variables
	userSpec, ok := spec.(fsmv2types.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	// Render the config template using the variables
	renderedConfig, err := fsmv2types.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	// Parse the rendered config into CommunicatorUserSpec
	var commSpec CommunicatorUserSpec
	if err := yaml.Unmarshal([]byte(renderedConfig), &commSpec); err != nil {
		return nil, fmt.Errorf("config parse failed: %w", err)
	}

	// Set default timeout if not specified
	if commSpec.Timeout == 0 {
		commSpec.Timeout = 10 * time.Second
	}

	// Return typed CommunicatorDesiredState - ALL fields preserved through marshal/unmarshal!
	// States access typed fields directly (NO re-parsing of OriginalUserSpec)
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

// GetInitialState returns the state the FSM should start in.
//
// The communicator always starts in StoppedState. The FSM will transition
// through Authenticating → Authenticated → Syncing based on observed state.
func (w *CommunicatorWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	// Register both worker and supervisor factories atomically.
	if err := factory.RegisterWorkerType[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](
		func(id fsmv2.Identity, logger *zap.SugaredLogger, stateReader fsmv2.StateReader, _ map[string]any) fsmv2.Worker {
			// Phase 1: ChannelProvider MUST be set via global singleton BEFORE factory is called.
			// The singleton is read by NewCommunicatorDependencies and will panic if not set.
			// DO NOT extract channelProvider from deps - singleton is THE ONLY way.

			// Phase 2: UUID is available via ObservedState.AuthenticatedUUID
			// No callback injection needed - consumers poll ObservedState instead.

			// Create dependencies WITHOUT transport (created lazily by AuthenticateAction).
			// NewCommunicatorDependencies will read channels from the global singleton.
			// It will panic if the singleton is not set - this is intentional.
			commDeps := NewCommunicatorDependencies(nil, logger, stateReader, id)

			worker, err := NewCommunicatorWorker(id.ID, id.Name, nil, logger, stateReader)
			if err != nil {
				panic(fmt.Sprintf("failed to create communicator worker: %v", err))
			}
			// Replace dependencies with the ones we created
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
