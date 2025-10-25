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

package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/protocol"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/basic"
)

// OrchestratorInterface coordinates synchronization between Frontend and Edge tiers.
//
// # Responsibilities
//
// The orchestrator manages the complete sync workflow:
//   - Building delta queries to find changes since last sync
//   - Encrypting and sending changes to the remote tier
//   - Receiving and decrypting incoming messages
//   - Applying changes to local storage
//   - Tracking sync progress via SyncStateInterface
//
// # Sync Flow
//
// Outbound sync (sending changes):
//  1. StartSync() fetches changes since last sync ID
//  2. Builds SyncMessage with operations
//  3. Encrypts payload (if crypto configured)
//  4. Sends via Transport
//  5. Marks changes as synced on success
//
// Inbound sync (receiving changes):
//  1. ProcessIncoming() receives encrypted RawMessage
//  2. Decrypts payload
//  3. Authorizes each operation (if authorizer configured)
//  4. Applies changes in a transaction
//  5. Updates sync state with remote's sync ID
//
// Periodic sync:
//  1. Tick() checks for pending changes
//  2. Initiates StartSync() if changes found
//  3. Polls for incoming messages
//  4. Processes messages via ProcessIncoming()
//
// # Error Handling
//
// All methods return errors for:
//   - Network failures (transport send/receive)
//   - Encryption/decryption failures
//   - Authorization failures
//   - Storage transaction failures
//
// The orchestrator tracks health status based on operation success/failure.
// Use GetStatus() to check current health and diagnose issues.
//
// # Thread Safety
//
// Implementations must be safe for concurrent calls from multiple goroutines.
// The FSM v2 Supervisor may call these methods from different threads.
type OrchestratorInterface interface {
	// StartSync initiates a synchronization cycle for the specified tier.
	//
	// This method fetches all changes since the last successful sync,
	// packages them into a SyncMessage, encrypts the payload, and sends
	// it to the remote tier via the Transport.
	//
	// The subscriptions parameter allows filtering what data to sync.
	// For MVP, we sync everything. Future versions can filter by model/field.
	//
	// The method is idempotent - if called multiple times with no new changes,
	// it returns immediately after detecting zero operations to sync.
	//
	// On success, changes are marked as synced in the SyncState, preventing
	// duplicate transmission. On failure, changes remain pending for retry.
	//
	// Returns an error if:
	//   - Unable to get delta query from sync state
	//   - Message serialization fails
	//   - Encryption fails (if crypto is configured)
	//   - Transport send fails
	//
	// Note: Failures to mark as synced are logged but do not fail the operation.
	// This prevents data loss at the cost of potential duplicate sends.
	StartSync(ctx context.Context, tier protocol.Tier, subscriptions []Subscription) error

	// ProcessIncoming handles incoming sync messages from the remote tier.
	//
	// This method:
	//  1. Decrypts the message payload (if crypto configured)
	//  2. Deserializes to SyncMessage
	//  3. Validates message is for this tier
	//  4. Authorizes each operation (if authorizer configured)
	//  5. Applies changes in a database transaction
	//  6. Updates sync state with remote's latest sync ID
	//
	// Authorization is optional but recommended. If an authorizer is configured,
	// each operation is checked via CanWriteTransaction(). Unauthorized operations
	// are skipped but don't fail the entire message.
	//
	// All operations in a message are applied atomically - either all succeed
	// or all are rolled back. This prevents partial application of sync messages.
	//
	// Returns an error if:
	//   - Decryption fails
	//   - Message deserialization fails
	//   - Message is for wrong tier
	//   - Transaction begin/commit fails
	//   - Any operation fails (after authorization)
	//
	// Note: Failures to update sync state are logged but do not fail the operation.
	// This prevents rejecting valid data at the cost of potential reprocessing.
	ProcessIncoming(ctx context.Context, msg protocol.RawMessage) error

	// Tick performs periodic synchronization operations.
	//
	// This method should be called regularly (e.g., every 100ms) by the FSM
	// Supervisor to maintain sync progress.
	//
	// On each tick, the method:
	//  1. Checks for pending changes via SyncState
	//  2. Initiates StartSync() if changes exist
	//  3. Polls Transport for incoming messages
	//  4. Processes messages via ProcessIncoming()
	//
	// The method is designed to be non-blocking and returns quickly:
	//   - Uses select with default for message polling
	//   - Respects context cancellation
	//   - Logs errors but continues operation
	//
	// Errors from StartSync() and ProcessIncoming() are logged but not returned.
	// This allows the supervisor to keep ticking even if individual operations fail.
	// The FSM will retry based on state transitions, not error returns.
	//
	// Returns an error only for:
	//   - Context cancellation
	//   - Critical failures that should stop the tick loop
	Tick(ctx context.Context) error

	// GetStatus returns the current synchronization status.
	//
	// The status includes:
	//   - Healthy: true if last sync succeeded, false if failed
	//   - LastSyncTime: when the last StartSync() completed
	//   - LastSyncError: the most recent error (nil if healthy)
	//   - PendingChangeCount: number of changes waiting to sync
	//   - EdgeSyncID: highest sync ID seen from Edge tier
	//   - FrontendSyncID: highest sync ID seen from Frontend tier
	//
	// This method is safe to call concurrently and does not block.
	// It provides a snapshot of current state for monitoring and debugging.
	//
	// Use this to:
	//   - Check if sync is functioning (Healthy field)
	//   - Diagnose sync failures (LastSyncError field)
	//   - Monitor sync progress (sync IDs increasing)
	//   - Detect stalled sync (PendingChangeCount not decreasing)
	GetStatus() SyncStatus
}

// Subscription defines what data a tier wants to sync.
// For MVP, we sync everything. Future versions can filter by model/field.
type Subscription struct {
	Model  string   // e.g., "container", "service", "*" for all
	Fields []string // e.g., ["status", "cpu"], empty for all fields
}

// SyncStatus represents the current state of synchronization.
type SyncStatus struct {
	Healthy           bool
	LastSyncTime      time.Time
	LastSyncError     error
	PendingChangeCount int
	EdgeSyncID        int64
	FrontendSyncID    int64
}


// DefaultOrchestrator is the standard implementation of OrchestratorInterface.
//
// It coordinates all components needed for synchronization:
//   - Transport: sends/receives encrypted messages
//   - Crypto: encrypts/decrypts message payloads (optional)
//   - Authorizer: validates write permissions (optional)
//   - SyncState: tracks sync progress and pending changes
//   - TriangularStore: CSE-specific storage (future)
//   - BasicStore: generic document storage
//
// The orchestrator delegates to these components rather than implementing
// their functionality directly. This separation allows:
//   - Testing with mocks (especially for transport and crypto)
//   - Swapping implementations (e.g., different storage backends)
//   - Optional features (crypto and authorizer can be nil)
//
// Thread Safety: All public methods are safe for concurrent use.
// Internal state (health, lastSyncTime) is protected by the orchestrator's
// single-threaded execution model - only called from FSM Supervisor.
type DefaultOrchestrator struct {
	transport       protocol.Transport
	crypto          protocol.Crypto
	authorizer      protocol.Authorizer
	syncState       SyncStateInterface
	triangularStore storage.TriangularStoreInterface
	basicStore      basic.Store
	tier            protocol.Tier
	remoteTier      protocol.Tier
	logger          *zap.SugaredLogger

	// Status tracking
	lastSyncTime  time.Time
	lastSyncError error
	healthy       bool
}

// NewDefaultOrchestrator creates a new sync orchestrator.
//
// Required parameters (panic if nil):
//   - transport: sends/receives messages to/from remote tier
//   - syncState: tracks sync progress and pending changes
//   - logger: structured logging
//
// Optional parameters (can be nil):
//   - crypto: encrypts/decrypts message payloads
//   - authorizer: validates write permissions for incoming changes
//   - triangularStore: CSE-specific storage (future feature)
//   - basicStore: generic document storage (required if syncing data)
//
// The tier parameter determines:
//   - Which tier this orchestrator represents (Frontend or Edge)
//   - Which tier is the remote (auto-computed as the opposite)
//   - Direction of sync operations
//
// Example:
//
//	orchestrator := NewDefaultOrchestrator(
//	    transport,      // WebSocket connection to relay
//	    crypto,         // E2E encryption using shared key
//	    authorizer,     // Permission validator
//	    syncState,      // Tracks sync IDs and pending changes
//	    nil,            // triangularStore (not used yet)
//	    basicStore,     // Document storage
//	    protocol.TierEdge,  // This is an Edge tier
//	    logger,
//	)
func NewDefaultOrchestrator(
	transport protocol.Transport,
	crypto protocol.Crypto,
	authorizer protocol.Authorizer,
	syncState SyncStateInterface,
	triangularStore storage.TriangularStoreInterface,
	basicStore basic.Store,
	tier protocol.Tier,
	logger *zap.SugaredLogger,
) *DefaultOrchestrator {
	if transport == nil {
		panic("transport must not be nil")
	}

	if syncState == nil {
		panic("syncState must not be nil")
	}

	if logger == nil {
		panic("logger must not be nil")
	}

	remoteTier := protocol.TierFrontend
	if tier == protocol.TierFrontend {
		remoteTier = protocol.TierEdge
	}

	return &DefaultOrchestrator{
		transport:       transport,
		crypto:          crypto,
		authorizer:      authorizer,
		syncState:       syncState,
		triangularStore: triangularStore,
		basicStore:      basicStore,
		tier:            tier,
		remoteTier:      remoteTier,
		logger:          logger,
		healthy:         true,
	}
}

// StartSync initiates a sync cycle for the specified tier.
func (o *DefaultOrchestrator) StartSync(ctx context.Context, tier protocol.Tier, subscriptions []Subscription) error {
	o.logger.Debugf("Starting sync for tier %s", tier)

	// Get changes since last sync
	deltaQuery, err := o.syncState.GetDeltaSince(tier)
	if err != nil {
		o.logger.Errorf("Failed to get delta query: %v", err)
		o.lastSyncError = err
		o.healthy = false
		return fmt.Errorf("failed to get delta query: %w", err)
	}

	// For MVP, we sync all triangular collections
	// Future: use subscriptions to filter
	collections := []string{
		"container_identity",
		"container_desired",
		"container_observed",
	}

	var operations []protocol.SyncOperation

	for _, collection := range collections {
		// Find documents matching delta query
		docs, err := o.basicStore.Find(ctx, collection, *deltaQuery)
		if err != nil {
			o.logger.Warnf("Failed to find documents in %s: %v", collection, err)
			continue
		}

		// Convert to sync operations
		for _, doc := range docs {
			syncID, _ := doc["_sync_id"].(int64)
			id, _ := doc["id"].(string)

			operations = append(operations, protocol.SyncOperation{
				Model:  collection,
				ID:     id,
				Action: "upsert",
				Data:   doc,
				SyncID: syncID,
			})
		}
	}

	if len(operations) == 0 {
		o.logger.Debug("No changes to sync")
		o.healthy = true
		o.lastSyncTime = time.Now()
		return nil
	}

	// Build sync message
	msg := protocol.SyncMessage{
		FromTier:   o.tier,
		ToTier:     o.remoteTier,
		SyncID:     o.getLatestSyncID(operations),
		Operations: operations,
		Timestamp:  time.Now().Unix(),
	}

	// Serialize message
	payload, err := json.Marshal(msg)
	if err != nil {
		o.logger.Errorf("Failed to marshal sync message: %v", err)
		o.lastSyncError = err
		o.healthy = false
		return fmt.Errorf("failed to marshal sync message: %w", err)
	}

	// Encrypt payload (if crypto is configured)
	if o.crypto != nil {
		encrypted, err := o.crypto.Encrypt(ctx, payload, string(o.remoteTier))
		if err != nil {
			o.logger.Errorf("Failed to encrypt sync message: %v", err)
			o.lastSyncError = err
			o.healthy = false
			return fmt.Errorf("failed to encrypt sync message: %w", err)
		}
		payload = encrypted
	}

	// Send via transport
	err = o.transport.Send(ctx, string(o.remoteTier), payload)
	if err != nil {
		o.logger.Errorf("Failed to send sync message: %v", err)
		o.lastSyncError = err
		o.healthy = false
		return fmt.Errorf("failed to send sync message: %w", err)
	}

	// Mark as synced on success
	err = o.syncState.MarkSynced(tier, msg.SyncID)
	if err != nil {
		o.logger.Errorf("Failed to mark as synced: %v", err)
		// Don't fail the sync, but log the error
	}

	o.logger.Infof("Successfully synced %d operations to %s (up to sync ID %d)",
		len(operations), o.remoteTier, msg.SyncID)

	o.healthy = true
	o.lastSyncTime = time.Now()
	o.lastSyncError = nil

	return nil
}

// ProcessIncoming handles incoming sync messages from the remote tier.
func (o *DefaultOrchestrator) ProcessIncoming(ctx context.Context, msg protocol.RawMessage) error {
	o.logger.Debugf("Processing incoming message from %s", msg.From)

	payload := msg.Payload

	// Decrypt if crypto is configured
	if o.crypto != nil {
		decrypted, err := o.crypto.Decrypt(ctx, payload, msg.From)
		if err != nil {
			o.logger.Errorf("Failed to decrypt message: %v", err)
			return fmt.Errorf("failed to decrypt message: %w", err)
		}
		payload = decrypted
	}

	// Deserialize message
	var syncMsg protocol.SyncMessage
	err := json.Unmarshal(payload, &syncMsg)
	if err != nil {
		o.logger.Errorf("Failed to unmarshal sync message: %v", err)
		return fmt.Errorf("failed to unmarshal sync message: %w", err)
	}

	// Validate message is for us
	if syncMsg.ToTier != o.tier {
		o.logger.Warnf("Received message for wrong tier: %s (we are %s)", syncMsg.ToTier, o.tier)
		return fmt.Errorf("message is for tier %s, not %s", syncMsg.ToTier, o.tier)
	}

	// Apply operations in a transaction
	tx, err := o.basicStore.BeginTx(ctx)
	if err != nil {
		o.logger.Errorf("Failed to begin transaction: %v", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err.Error() != "sql: transaction has already been committed or rolled back" {
			o.logger.Warnf("Failed to rollback transaction: %v", err)
		}
	}()

	for _, op := range syncMsg.Operations {
		// Authorize if authorizer is configured
		if o.authorizer != nil {
			authorized, err := o.authorizer.CanWriteTransaction(ctx, msg.From, op.Model, op.ID)
			if err != nil || !authorized {
				o.logger.Warnf("Unauthorized operation from %s: %s/%s", msg.From, op.Model, op.ID)
				continue
			}
		}

		// Apply operation
		switch op.Action {
		case "upsert":
			// Try to update first, if it doesn't exist, insert
			err = tx.Update(ctx, op.Model, op.ID, op.Data)
			if err != nil {
				// If document doesn't exist, insert it
				if errors.Is(err, basic.ErrNotFound) {
					_, err = tx.Insert(ctx, op.Model, op.Data)
					if err != nil {
						o.logger.Errorf("Failed to insert %s/%s: %v", op.Model, op.ID, err)
						return fmt.Errorf("failed to insert: %w", err)
					}
				} else {
					o.logger.Errorf("Failed to update %s/%s: %v", op.Model, op.ID, err)
					return fmt.Errorf("failed to update: %w", err)
				}
			}

		case "delete":
			err = tx.Delete(ctx, op.Model, op.ID)
			if err != nil {
				o.logger.Errorf("Failed to delete %s/%s: %v", op.Model, op.ID, err)
				return fmt.Errorf("failed to delete: %w", err)
			}

		default:
			o.logger.Warnf("Unknown operation action: %s", op.Action)
		}
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		o.logger.Errorf("Failed to commit transaction: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update sync state with remote's latest sync ID
	if o.tier == protocol.TierEdge {
		err = o.syncState.SetFrontendSyncID(syncMsg.SyncID)
	} else {
		err = o.syncState.SetEdgeSyncID(syncMsg.SyncID)
	}
	if err != nil {
		o.logger.Errorf("Failed to update sync ID: %v", err)
		// Don't fail the operation, but log the error
	}

	o.logger.Infof("Successfully applied %d operations from %s (sync ID %d)",
		len(syncMsg.Operations), syncMsg.FromTier, syncMsg.SyncID)

	return nil
}

// Tick performs periodic sync operations.
func (o *DefaultOrchestrator) Tick(ctx context.Context) error {
	// Check for pending changes
	pendingChanges, err := o.syncState.GetPendingChanges(o.tier)
	if err != nil {
		o.logger.Errorf("Failed to get pending changes: %v", err)
		return fmt.Errorf("failed to get pending changes: %w", err)
	}

	if len(pendingChanges) > 0 {
		o.logger.Debugf("Found %d pending changes, initiating sync", len(pendingChanges))

		// For MVP, sync everything
		subscriptions := []Subscription{
			{Model: "*", Fields: []string{}},
		}

		err = o.StartSync(ctx, o.tier, subscriptions)
		if err != nil {
			o.logger.Errorf("Sync failed: %v", err)
			// Don't return error - tick should continue even if sync fails
			// The supervisor will retry based on FSM state
		}
	}

	// Check for incoming messages
	msgChan, err := o.transport.Receive(ctx)
	if err != nil {
		o.logger.Errorf("Failed to start receive: %v", err)
		return fmt.Errorf("failed to start receive: %w", err)
	}

	select {
	case msg := <-msgChan:
		err = o.ProcessIncoming(ctx, msg)
		if err != nil {
			o.logger.Errorf("Failed to process incoming message: %v", err)
			// Don't return error - continue processing
		}

	case <-ctx.Done():
		return ctx.Err()

	default:
		// No messages, continue
	}

	return nil
}

// GetStatus returns the current sync status.
func (o *DefaultOrchestrator) GetStatus() SyncStatus {
	pendingChanges, _ := o.syncState.GetPendingChanges(o.tier)

	return SyncStatus{
		Healthy:            o.healthy,
		LastSyncTime:       o.lastSyncTime,
		LastSyncError:      o.lastSyncError,
		PendingChangeCount: len(pendingChanges),
		EdgeSyncID:         o.syncState.GetEdgeSyncID(),
		FrontendSyncID:     o.syncState.GetFrontendSyncID(),
	}
}

// getLatestSyncID returns the highest sync ID from the operations.
func (o *DefaultOrchestrator) getLatestSyncID(operations []protocol.SyncOperation) int64 {
	var maxSyncID int64
	for _, op := range operations {
		if op.SyncID > maxSyncID {
			maxSyncID = op.SyncID
		}
	}
	return maxSyncID
}