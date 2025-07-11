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

// Package actions contains implementations of the Action interface that delete
// stream processor configurations.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A Stream Processor (SP) in UMH processes data from the unified namespace
// using data models. The delete action removes an existing stream processor
// configuration from the system.
//
// The action follows the established deletion patterns and handles:
// - Deletion of standalone stream processors
// - Deletion of child stream processors (stops inheriting from root)
// - Deletion of root stream processors (only if no children depend on them)
// - Proper cleanup and health checking
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// DeleteStreamProcessorAction implements the Action interface for deleting
// stream processor configurations.
type DeleteStreamProcessorAction struct {
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager

	// Parsed request payload (only populated after Parse)
	streamProcessorUUID uuid.UUID
	name                string
	ignoreHealthCheck   bool

	// Runtime observation for health checks
	systemSnapshotManager *fsm.SnapshotManager

	actionLogger *zap.SugaredLogger
}

// NewDeleteStreamProcessorAction returns an un-parsed action instance.
func NewDeleteStreamProcessorAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *DeleteStreamProcessorAction {
	return &DeleteStreamProcessorAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		systemSnapshotManager: systemSnapshotManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface by extracting the stream processor UUID from the payload.
func (a *DeleteStreamProcessorAction) Parse(payload interface{}) error {
	// Parse the payload to get the UUID
	parsedPayload, err := ParseActionPayload[models.DeleteStreamProcessorPayload](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %v", err)
	}

	// Validate UUID is provided
	if parsedPayload.UUID == "" {
		return errors.New("missing required field UUID")
	}

	// Parse string UUID into UUID object
	streamProcessorUUID, err := uuid.Parse(parsedPayload.UUID)
	if err != nil {
		return fmt.Errorf("invalid UUID format: %v", err)
	}

	a.streamProcessorUUID = streamProcessorUUID

	a.actionLogger.Debugf("Parsed DeleteStreamProcessor action payload: UUID=%s", a.streamProcessorUUID)
	return nil
}

// Validate performs validation of the parsed payload.
func (a *DeleteStreamProcessorAction) Validate() error {
	// Validate UUID
	if a.streamProcessorUUID == uuid.Nil {
		return errors.New("missing or invalid stream processor UUID")
	}

	return nil
}

// Execute implements the Action interface by deleting the stream processor configuration.
func (a *DeleteStreamProcessorAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeleteStreamProcessor action")

	// Find the stream processor by UUID to get the name
	err := a.findStreamProcessorByUUID()
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to find stream processor: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.DeleteStreamProcessor)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Send confirmation that action is starting
	confirmationMessage := fmt.Sprintf("Starting deletion of stream processor %s", a.name)
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		confirmationMessage, a.outboundChannel, models.DeleteStreamProcessor)

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Deleting stream processor configuration...", a.outboundChannel, models.DeleteStreamProcessor)

	// Delete the stream processor configuration
	err = a.deleteStreamProcessor()
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to delete stream processor: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.DeleteStreamProcessor)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Await cleanup and perform health checks
	if a.systemSnapshotManager != nil && !a.ignoreHealthCheck {
		errCode, err := a.awaitCleanup()
		if err != nil {
			errorMsg := fmt.Sprintf("Failed during cleanup: %v", err)
			SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, errCode, nil, a.outboundChannel, models.DeleteStreamProcessor, nil)
			return nil, nil, fmt.Errorf("%s", errorMsg)
		}
	}

	response := map[string]any{
		"deleted_name": a.name,
	}

	return response, nil, nil
}

// findStreamProcessorByUUID finds the stream processor by UUID from the configuration and extracts the name.
func (a *DeleteStreamProcessorAction) findStreamProcessorByUUID() error {
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	// Get current configuration
	currentConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get current configuration: %w", err)
	}

	// Find the stream processor in the configuration
	for _, sp := range currentConfig.StreamProcessor {
		spID := GenerateUUIDFromName(sp.Name)
		if spID == a.streamProcessorUUID {
			a.name = sp.Name
			a.actionLogger.Debugf("Found stream processor: uuid=%s, name=%s", a.streamProcessorUUID, a.name)
			return nil
		}
	}

	return fmt.Errorf("stream processor with UUID %s not found", a.streamProcessorUUID)
}

// deleteStreamProcessor performs the actual deletion of the stream processor from the configuration.
func (a *DeleteStreamProcessorAction) deleteStreamProcessor() error {
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	// Delete the stream processor using the atomic operation
	err := a.configManager.AtomicDeleteStreamProcessor(ctx, a.name)
	if err != nil {
		return fmt.Errorf("failed to delete stream processor: %w", err)
	}

	return nil
}

// awaitCleanup waits for the stream processor to be removed from the system.
func (a *DeleteStreamProcessorAction) awaitCleanup() (string, error) {
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		fmt.Sprintf("Waiting for stream processor %s to be removed...", a.name),
		a.outboundChannel, models.DeleteStreamProcessor)

	return a.waitForComponentToBeRemoved()
}

// waitForComponentToBeRemoved polls live FSM state until the stream processor
// is removed from the system or the timeout hits.
func (a *DeleteStreamProcessorAction) waitForComponentToBeRemoved() (string, error) {
	ticker := time.NewTicker(constants.ActionTickerTime)
	defer ticker.Stop()
	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout)
	startTime := time.Now()
	timeoutDuration := constants.DataflowComponentWaitForActiveTimeout

	for {
		elapsed := time.Since(startTime)
		remaining := timeoutDuration - elapsed
		remainingSeconds := int(remaining.Seconds())

		select {
		case <-timeout:
			stateMessage := fmt.Sprintf("Stream processor '%s' deletion timeout reached. It may still be stopping in the background", a.name)
			return models.ErrRetryRollbackTimeout, fmt.Errorf("%s", stateMessage)

		case <-ticker.C:
			// Get a deep copy of the system snapshot to prevent race conditions
			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()
			if streamProcessorManager, exists := systemSnapshot.Managers[constants.StreamProcessorManagerName]; exists {
				instances := streamProcessorManager.GetInstances()
				found := false
				for _, instance := range instances {
					curName := instance.ID
					if curName == a.name {
						found = true
						break
					}
				}

				if !found {
					// Stream processor has been successfully removed
					stateMessage := RemainingPrefixSec(remainingSeconds) + "stream processor successfully removed"
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage,
						a.outboundChannel, models.DeleteStreamProcessor)
					return "", nil
				}

				stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for stream processor to be removed"
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					stateMessage, a.outboundChannel, models.DeleteStreamProcessor)

			} else {
				stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for stream processor manager to initialise"
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					stateMessage, a.outboundChannel, models.DeleteStreamProcessor)
			}
		}
	}
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *DeleteStreamProcessorAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *DeleteStreamProcessorAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetStreamProcessorUUID returns the stream processor UUID - exposed for testing purposes.
func (a *DeleteStreamProcessorAction) GetStreamProcessorUUID() uuid.UUID {
	return a.streamProcessorUUID
}

// GetParsedPayload returns the parsed payload - exposed primarily for testing purposes.
func (a *DeleteStreamProcessorAction) GetParsedPayload() models.DeleteStreamProcessorPayload {
	return models.DeleteStreamProcessorPayload{
		UUID: a.streamProcessorUUID.String(),
	}
}
