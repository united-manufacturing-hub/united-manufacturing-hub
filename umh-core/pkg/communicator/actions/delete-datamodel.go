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
// data model configurations from the UMH system.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A Data Model in UMH defines the structure of data that flows through the system.
// "Deleting" a data model means removing an existing configuration entry by name.
//
// The action removes the data model configuration from the system, making it
// unavailable for data validation and structure enforcement.
// -----------------------------------------------------------------------------

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

// DeleteDataModelAction implements the Action interface for deleting an existing Data Model.
// All fields are immutable after construction to avoid race conditions.
type DeleteDataModelAction struct {
	configManager config.ConfigManager

	outboundChannel chan *models.UMHMessage

	actionLogger *zap.SugaredLogger
	userEmail    string

	// Parsed request payload (only populated after Parse)
	payload models.DeleteDataModelPayload

	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewDeleteDataModelAction returns an un-parsed action instance.
func NewDeleteDataModelAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *DeleteDataModelAction {
	return &DeleteDataModelAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface by extracting data model name from the payload.
func (a *DeleteDataModelAction) Parse(payload interface{}) error {
	// Parse the payload to get the data model name
	parsedPayload, err := ParseActionPayload[models.DeleteDataModelPayload](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	a.payload = parsedPayload
	a.actionLogger.Debugf("Parsed DeleteDataModel action payload: name=%s", a.payload.Name)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *DeleteDataModelAction) Validate() error {
	// Validate required fields
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	return nil
}

// Execute implements the Action interface by deleting the data model configuration.
func (a *DeleteDataModelAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeleteDataModel action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting to delete data model: "+a.payload.Name, a.outboundChannel, models.DeleteDataModel)

	// Delete from configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Removing data model from configuration...", a.outboundChannel, models.DeleteDataModel)

	err := a.configManager.AtomicDeleteDataModel(ctx, a.payload.Name)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to delete data model: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.DeleteDataModel)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Create response with the deletion confirmation
	response := map[string]interface{}{
		"name":    a.payload.Name,
		"deleted": true,
	}

	return response, nil, nil
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *DeleteDataModelAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *DeleteDataModelAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed payload - exposed primarily for testing purposes.
func (a *DeleteDataModelAction) GetParsedPayload() models.DeleteDataModelPayload {
	return a.payload
}
