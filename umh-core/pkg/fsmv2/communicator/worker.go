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

// Package communicator implements the CSE Communicator FSM worker for
// bidirectional synchronization between Edge and Frontend tiers.
//
// # Architecture
//
// The Communicator FSM manages the complete lifecycle of CSE sync:
//  1. Authentication: Obtain JWT token from relay server
//  2. Synchronization: Bidirectional data sync with remote tier
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
//   - Syncing: SyncAction performs delta sync operations
//
// # Integration with CSE Sync
//
// The worker delegates sync operations to csesync.OrchestratorInterface:
//   - Authentication: Handled directly by AuthenticateAction
//   - Delta sync: Delegated to orchestrator.Tick()
//   - Message processing: Delegated to orchestrator.ProcessIncoming()
//
// This separation keeps the FSM focused on state management while the
// orchestrator handles sync protocol details.
package communicator

import (
	"context"
	"time"

	"go.uber.org/zap"

	csesync "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/sync"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// CommunicatorWorker implements the FSM v2 Worker interface for CSE synchronization.
//
// It manages the lifecycle of bidirectional sync between Edge and Frontend tiers,
// handling authentication, connection management, and sync operations.
//
// # Responsibilities
//
// State collection (CollectObservedState):
//   - Returns shared CommunicatorObservedState
//   - Updated by actions (AuthenticateAction sets JWT token)
//   - Read by states to determine transitions
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
//   - orchestrator: Handles sync protocol operations
//   - relayURL: Relay server endpoint for authentication
//   - instanceUUID: Identifies this Edge instance
//   - authToken: Pre-shared secret for authentication
//   - logger: Structured logging
//
// These dependencies are passed to actions when created by states.
type CommunicatorWorker struct {
	identity  fsmv2.Identity
	relayURL  string

	// Dependencies for creating actions
	orchestrator  csesync.OrchestratorInterface
	instanceUUID  string
	authToken     string
	observedState *CommunicatorObservedState
	logger        *zap.SugaredLogger
}

// NewCommunicatorWorker creates a new CSE Communicator worker.
//
// Parameters:
//   - id: Unique identifier for this worker instance
//   - relayURL: Relay server endpoint (e.g., "wss://relay.example.com")
//   - orchestrator: Handles sync protocol operations (must not be nil)
//   - instanceUUID: Identifies this Edge instance (from config)
//   - authToken: Pre-shared secret for relay authentication (from config)
//   - logger: Structured logging (must not be nil)
//
// The worker is created in Stopped state. Call supervisor.Start() to begin
// the authentication and sync lifecycle.
//
// Example:
//
//	worker := NewCommunicatorWorker(
//	    "communicator-1",
//	    "wss://relay.umh.app",
//	    orchestrator,
//	    "550e8400-e29b-41d4-a716-446655440000",
//	    "secret-auth-token",
//	    logger,
//	)
//	supervisor := fsmv2.NewSupervisor(worker, logger)
//	supervisor.Start(ctx)
func NewCommunicatorWorker(
	id string,
	relayURL string,
	orchestrator csesync.OrchestratorInterface,
	instanceUUID string,
	authToken string,
	logger *zap.SugaredLogger,
) *CommunicatorWorker {
	return &CommunicatorWorker{
		identity: fsmv2.Identity{
			ID:   id,
			Name: "CSE-Communicator",
		},
		relayURL:      relayURL,
		orchestrator:  orchestrator,
		instanceUUID:  instanceUUID,
		authToken:     authToken,
		observedState: &CommunicatorObservedState{},
		logger:        logger,
	}
}

// CollectObservedState returns the current observed state of the communicator.
//
// This method is called periodically by the FSM v2 Supervisor to determine
// what state the system is in. The returned state is used to decide which
// state transitions are possible.
//
// The observed state includes:
//   - IsAuthenticated: Whether we have a valid JWT token
//   - JWTToken: The current JWT token (empty if not authenticated)
//   - CollectedAt: Timestamp of this observation
//
// Actions (AuthenticateAction, SyncAction) update the shared observedState,
// and this method returns it to the supervisor.
//
// This method never returns an error for the communicator worker.
func (w *CommunicatorWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	// Return the shared observedState that actions update
	w.observedState.CollectedAt = time.Now()
	return w.observedState, nil
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
	return &CommunicatorDesiredState{
		shutdownRequested: false,
	}, nil
}

// GetInitialState returns the state the FSM should start in.
//
// The communicator always starts in StoppedState. The FSM will transition
// through Authenticating → Authenticated → Syncing based on observed state.
func (w *CommunicatorWorker) GetInitialState() fsmv2.State {
	return &StoppedState{Worker: w}
}
