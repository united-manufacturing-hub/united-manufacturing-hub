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

// Package actions houses *imperative* operations that mutate the configuration
// or runtime state of an UMH instance.  This file contains the implementation
// for deleting a Protocol Converter.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A Protocol Converter is an FSM‑managed component that handles protocol conversion.
// Deleting such a component is a two‑step affair:
//
//   1. Remove the component **configuration** from the central store via
//      `configManager.AtomicDeleteProtocolConverter`.
//   2. Wait briefly for the runtime to process the deletion.
//
// The Action's contract mirrors the other mutate‑type actions:
//   * A progress message is emitted for each significant milestone.
//   * A simple 2-second wait is used instead of complex FSM observation.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// DeleteProtocolConverterAction removes a single Protocol Converter identified by UUID.
//
// The struct only contains *immutable* data required throughout the whole
// lifecycle of a deletion.  Any value that changes during execution is local to
// the respective method to avoid lock contention and race conditions.
// ----------------------------------------------------------------------------
type DeleteProtocolConverterAction struct {
	configManager config.ConfigManager // abstraction over config store

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage // channel for progress events

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager

	// ─── Utilities ──────────────────────────────────────────────────────────
	actionLogger *zap.SugaredLogger
	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail    string    // used for feedback messages
	actionUUID   uuid.UUID // unique ID of *this* action instance
	instanceUUID uuid.UUID // ID of the UMH instance we operate on

	// ─── Business data ──────────────────────────────────────────────────────
	componentUUID uuid.UUID // the component slated for deletion

}

// NewDeleteProtocolConverterAction returns an *empty* action instance prepared
// for unit tests or real execution.  The caller still needs to Parse and
// Validate before calling Execute.
func NewDeleteProtocolConverterAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *DeleteProtocolConverterAction {
	return &DeleteProtocolConverterAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		systemSnapshotManager: systemSnapshotManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
	}
}

// Parse extracts the component UUID from the user‑supplied JSON payload.
// Shape errors (missing or malformed UUID) are detected here so that Validate
// can remain trivial.
func (a *DeleteProtocolConverterAction) Parse(payload interface{}) error {
	// Parse the payload to get the UUID
	parsedPayload, err := ParseActionPayload[models.DeleteProtocolConverterPayload](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %v", err)
	}

	a.componentUUID = parsedPayload.UUID
	a.actionLogger.Debugf("Parsed DeleteProtocolConverter action payload: UUID=%s", a.componentUUID)

	return nil
}

// Validate performs only *existence* checks because all heavy‑weight work has
// already happened in Parse.
func (a *DeleteProtocolConverterAction) Validate() error {
	// UUID validation is already done in Parse, so there's not much additional validation needed
	if a.componentUUID == uuid.Nil {
		return errors.New("component UUID is missing or invalid")
	}

	return nil
}

// Execute removes the configuration entry and then waits briefly for the system
// to process the deletion.
//
// Progress is streamed via `outboundChannel` so that a human operator can watch
// the deletion in real time.
func (a *DeleteProtocolConverterAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeleteProtocolConverter action")

	// ─── 1  Tell the UI we are about to start ──────────────────────────────
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, "Starting deletion of protocol converter with UUID: "+a.componentUUID.String(), a.outboundChannel, models.DeleteProtocolConverter)

	// ─── 2  Remove the config atomically ───────────────────────────────────-
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Removing protocol converter from configuration...", a.outboundChannel, models.DeleteProtocolConverter)
	err := a.configManager.AtomicDeleteProtocolConverter(ctx, a.componentUUID)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to delete protocol converter: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.DeleteProtocolConverter)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// ─── 3  Observe the runtime until the FSM forgets the instance ─────────
	if a.systemSnapshotManager != nil { // skipping this for the unit tests

		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Configuration updated. Waiting for protocol converter to be fully removed from the system...", a.outboundChannel, models.DeleteProtocolConverter)
		err = a.waitForComponentToBeRemoved()
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to wait for protocol converter to be removed: %v", err)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.DeleteProtocolConverter)
			return nil, nil, fmt.Errorf("%s", errorMsg)
		}
	}

	// ─── 4  Tell the caller we are done (caller will send FinishedSuccessful) ──
	successMsg := fmt.Sprintf("Successfully deleted protocol converter with UUID: %s", a.componentUUID)

	return successMsg, nil, nil
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *DeleteProtocolConverterAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *DeleteProtocolConverterAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetComponentUUID returns the UUID of the component to be deleted - exposed primarily for testing purposes.
func (a *DeleteProtocolConverterAction) GetComponentUUID() uuid.UUID {
	return a.componentUUID
}

func (a *DeleteProtocolConverterAction) waitForComponentToBeRemoved() error {
	//check the system snapshot and waits for the instance to be removed
	ticker := time.NewTicker(constants.ActionTickerTime)
	defer ticker.Stop()
	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout)
	startTime := time.Now()
	timeoutDuration := constants.DataflowComponentWaitForActiveTimeout

	// try to find the component name for better logging
	componentName := a.componentUUID.String() // Default to using UUID if name not found
	// the snapshot manager holds the latest system snapshot which is asynchronously updated by the other goroutines
	// we need to get a deep copy of it to prevent race conditions
	systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()
	if protocolConverterManager, exists := systemSnapshot.Managers[constants.ProtocolConverterManagerName]; exists {
		for _, inst := range protocolConverterManager.GetInstances() {
			if dataflowcomponentserviceconfig.GenerateUUIDFromName(inst.ID) == a.componentUUID {
				componentName = inst.ID
				break
			}
		}
	}

	for {
		select {
		case <-timeout:
			return fmt.Errorf("protocol converter %s was not removed within the timeout period", componentName)
		case <-ticker.C:
			elapsed := time.Since(startTime)
			remaining := timeoutDuration - elapsed
			remainingSeconds := int(remaining.Seconds())

			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
				fmt.Sprintf("Verifying removal of protocol converter '%s' (%ds remaining)...",
					componentName, remainingSeconds), a.outboundChannel, models.DeleteProtocolConverter)

			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()

			removed := true
			if mgr, ok := systemSnapshot.Managers[constants.ProtocolConverterManagerName]; ok {
				for _, inst := range mgr.GetInstances() {
					if dataflowcomponentserviceconfig.GenerateUUIDFromName(inst.ID) == a.componentUUID {
						removed = false
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
							fmt.Sprintf("Component '%s' still exists in state '%s'. Waiting for removal (%ds remaining)...",
								inst.ID, inst.CurrentState, remainingSeconds), a.outboundChannel, models.DeleteProtocolConverter)
						break
					}
				}
			}
			if removed {
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					fmt.Sprintf("Protocol converter '%s' has been successfully removed from the system.", componentName),
					a.outboundChannel, models.DeleteProtocolConverter)
				return nil
			}
		}
	}
}
