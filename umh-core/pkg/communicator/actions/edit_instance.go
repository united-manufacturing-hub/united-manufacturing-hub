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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// EditInstanceAction implements the Action interface for editing instance properties.
// Currently, it supports updating the location hierarchy (enterprise, site, area, line, workCell).
type EditInstanceAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	location        *models.EditInstanceLocationModel
	configManager   config.ConfigManager
	actionLogger    *zap.SugaredLogger
}

// NewEditInstanceAction creates a new EditInstanceAction with the provided parameters.
// This constructor is primarily used for testing purposes to enable dependency injection.
// It initializes the action with the necessary fields but doesn't populate the location field
// which must be done via Parse or SetLocation.
func NewEditInstanceAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *EditInstanceAction {
	return &EditInstanceAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicatorActions),
	}
}

// Parse implements the Action interface by extracting location information from the payload.
// It accepts a location structure containing enterprise (required) and optional site,
// area, line, and workCell fields.
//
// The function handles the case where no location is provided by leaving the location field nil,
// which is valid and indicates no location change is requested.
func (a *EditInstanceAction) Parse(payload interface{}) error {
	a.actionLogger.Debug("Parsing EditInstance action payload")

	// Convert the payload to a map
	payloadMap, ok := payload.(map[string]interface{})
	if !ok {
		return errors.New("invalid payload format, expected map")
	}

	// Check if we have a location field
	locationData, ok := payloadMap["location"]
	if !ok {
		// No location provided, which is fine since it's optional
		return nil
	}

	// Parse location data
	locationMap, ok := locationData.(map[string]interface{})
	if !ok {
		return errors.New("invalid location format, expected map")
	}

	// Extract enterprise (required)
	enterprise, ok := locationMap["enterprise"].(string)
	if !ok || enterprise == "" {
		return errors.New("missing or invalid enterprise in location")
	}

	// Create location object
	location := &models.EditInstanceLocationModel{
		Enterprise: enterprise,
	}

	// Extract optional fields
	if site, ok := locationMap["site"].(string); ok && site != "" {
		location.Site = &site
	}
	if area, ok := locationMap["area"].(string); ok && area != "" {
		location.Area = &area
	}
	if line, ok := locationMap["line"].(string); ok && line != "" {
		location.Line = &line
	}
	if workCell, ok := locationMap["workCell"].(string); ok && workCell != "" {
		location.WorkCell = &workCell
	}

	a.location = location
	return nil
}

// Validate implements the Action interface by checking if the parsed data meets
// the business requirements. For EditInstanceAction, it verifies that if a location
// is provided, the enterprise field is not empty.
//
// If the location field is nil (no location update requested), validation passes.
func (a *EditInstanceAction) Validate() error {
	// If location is provided, validate that enterprise is not empty
	if a.location != nil && a.location.Enterprise == "" {
		return errors.New("enterprise cannot be empty when location is provided")
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
//
// For EditInstanceAction, if no location update is requested (location is nil),
// it returns early with a message indicating no changes were made.
func (a *EditInstanceAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Debug("Executing EditInstance action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, "Starting EditInstance action", a.outboundChannel, models.EditInstance)

	// If we don't have any location to update, return early
	if a.location == nil {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "No location changes requested", a.outboundChannel, models.EditInstance)
		return "No changes were made to the instance", nil, nil
	}

	// Send progress update
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Updating instance location", a.outboundChannel, models.EditInstance)

	// Update the location in the configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()
	err := a.configManager.AtomicSetLocation(ctx, *a.location)
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

// GetLocation returns the location - for testing
func (a *EditInstanceAction) GetLocation() *models.EditInstanceLocationModel {
	return a.location
}

// SetLocation sets the location - for testing
func (a *EditInstanceAction) SetLocation(location *models.EditInstanceLocationModel) {
	a.location = location
}
