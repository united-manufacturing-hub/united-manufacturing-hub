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

package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// EditInstanceAction implements the Action interface for editing instance properties.
// Currently, it supports updating the location hierarchy using the generic location format
// where map keys represent hierarchy levels (example: 0=enterprise, 1=site, etc.)
type EditInstanceAction struct {
	configManager         config.ConfigManager
	outboundChannel       chan *models.UMHMessage
	actionLogger          *zap.SugaredLogger
	systemSnapshotManager *fsm.SnapshotManager

	// Parsed request payload (populated after Parse)
	payload      models.EditInstanceLocationModel
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewEditInstanceAction creates a new EditInstanceAction with the provided parameters.
// This constructor is primarily used for testing purposes to enable dependency injection.
// It initializes the action with the necessary fields but doesn't populate the location field
// which must be done via Parse or SetLocation.
func NewEditInstanceAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *EditInstanceAction {
	return &EditInstanceAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
		systemSnapshotManager: systemSnapshotManager,
	}
}

// Parse implements the Action interface by extracting location information from the payload.
func (a *EditInstanceAction) Parse(payload interface{}) error {
	a.actionLogger.Debug("Parsing EditInstance action payload")

	parsedPayload, err := ParseActionPayload[models.EditInstanceLocationModel](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	a.payload = parsedPayload

	a.actionLogger.Debugf("Parsed EditInstance action payload: location=%v", a.payload.Location)

	return nil
}

// Validate implements the Action interface by checking if the parsed data meets
// the business requirements. For EditInstanceAction, it verifies that the location
// is provided and that the enterprise field (level 0) exists and is not empty.
func (a *EditInstanceAction) Validate() error {
	if len(a.payload.Location) == 0 {
		return errors.New("location is required")
	}

	if enterprise, exists := a.payload.Location[0]; !exists || enterprise == "" {
		return errors.New("enterprise (level 0) is required and cannot be empty")
	}

	return nil
}

// Execute implements the Action interface by performing the actual instance update.
// It follows the standard pattern for actions:
// 1. Sends ActionConfirmed to indicate the action is starting
// 2. Sends ActionExecuting with progress updates
// 3. Performs the configuration update using the configManager
// 4. Sends ActionFinishedWithFailure if an error occurs
// 5. Returns a success message (not sending ActionFinishedSuccessfull as that's done by the caller)
func (a *EditInstanceAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Debug("Executing EditInstance action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, "Starting EditInstance action", a.outboundChannel, models.EditInstance)

	// Send progress update
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Updating instance location", a.outboundChannel, models.EditInstance)

	// Update the location in the configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	err := a.configManager.AtomicSetLocation(ctx, a.payload.Location)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to update instance location: %s", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.EditInstance)

		return nil, nil, fmt.Errorf("failed to update instance location: %w", err)
	}

	// we can be sure that the location is updated in the config if the error is nil

	// TODO: check against observedState as well

	return "Successfully updated instance location", nil, nil
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *EditInstanceAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *EditInstanceAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// Methods added for testing purposes

// GetParsedPayload returns the parsed payload - for testing.
func (a *EditInstanceAction) GetParsedPayload() models.EditInstanceLocationModel {
	return a.payload
}

// SetParsedPayload sets the parsed payload - for testing.
func (a *EditInstanceAction) SetParsedPayload(payload models.EditInstanceLocationModel) {
	a.payload = payload
}
