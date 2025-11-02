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
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

// WorkerType identifies this worker in the FSM v2 system.
// It is used to distinguish communicator workers from other worker types
// (e.g., supervisor workers) in logging, metrics, and supervision contexts.
const WorkerType = "communicator"

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
// This enables actions to communicate results to states without coupling.
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
	*fsmv2.BaseWorker[*CommunicatorRegistry]
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
//	worker := NewCommunicatorWorker(
//	    "communicator-1",
//	    "Communicator Worker",
//	    httpTransport,
//	    logger,
//	)
//	supervisor := fsmv2.NewSupervisor(worker, logger)
//	supervisor.Start(ctx)
func NewCommunicatorWorker(
	id string,
	name string,
	transportParam transport.Transport,
	logger *zap.SugaredLogger,
) *CommunicatorWorker {
	registry := NewCommunicatorRegistry(transportParam, logger)

	return &CommunicatorWorker{
		BaseWorker: fsmv2.NewBaseWorker(registry),
		identity: fsmv2.Identity{
			ID:         id,
			Name:       name,
			WorkerType: WorkerType,
		},
		logger: logger,
	}
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
	observed := snapshot.CommunicatorObservedState{
		CollectedAt: time.Now(),
	}

	// TODO: Implement proper state collection based on transport state
	// This will need to query the transport for current authentication status
	// and sync health, rather than relying on previousAction pattern

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
func (w *CommunicatorWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &snapshot.CommunicatorDesiredState{
		Registry: w.GetRegistry(),
	}, nil
}

// GetInitialState returns the state the FSM should start in.
//
// The communicator always starts in StoppedState. The FSM will transition
// through Authenticating → Authenticated → Syncing based on observed state.
func (w *CommunicatorWorker) GetInitialState() state.BaseCommunicatorState {
	return &state.StoppedState{}
}
