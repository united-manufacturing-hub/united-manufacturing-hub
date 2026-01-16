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
//     MUST: Transport must not be nil throughout worker lifetime
//     WHY:  All actions depend on transport for HTTP communication
//     ENFORCED: NewCommunicatorWorker validates transport parameter
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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
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

	// Read consecutive error count from dependencies
	consecutiveErrors := deps.GetConsecutiveErrors()

	observed := snapshot.CommunicatorObservedState{
		CollectedAt:       time.Now(),
		JWTToken:          jwtToken,
		JWTExpiry:         jwtExpiry,
		MessagesReceived:  messagesReceived,
		Authenticated:     authenticated,
		ConsecutiveErrors: consecutiveErrors,
	}

	return observed, nil
}

// DeriveDesiredState determines what state the communicator should be in.
//
// For MVP, this always returns "not shutdown" - the communicator should
// always be running and syncing. Future versions may derive desired state
// from configuration (e.g., enable/disable sync).
//
// The spec parameter is reserved for future use and currently ignored.
//
// This method never returns an error for the communicator worker.
func (w *CommunicatorWorker) DeriveDesiredState(spec interface{}) (fsmv2types.DesiredState, error) {
	// For now, communicator uses simple desired state without children
	// Future: may populate ChildrenSpecs for sub-components
	return fsmv2types.DesiredState{
		State:         "running",
		ChildrenSpecs: nil,
	}, nil
}

// GetInitialState returns the state the FSM should start in.
//
// The communicator always starts in StoppedState. The FSM will transition
// through Authenticating → Authenticated → Syncing based on observed state.
func (w *CommunicatorWorker) GetInitialState() state.BaseCommunicatorState {
	return &state.StoppedState{}
}
