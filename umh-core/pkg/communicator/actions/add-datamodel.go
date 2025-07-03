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

// Package actions contains implementations of the Action interface that create
// and manage data model configurations in the UMH system.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A Data Model in UMH defines the structure of data that flows through the system.
// "Adding" a data model means creating a new configuration entry with a name,
// description, and structure definition.
//
// The action creates a new data model configuration that can be used for
// data validation and structure enforcement throughout the system.
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

// AddDataModelAction implements the Action interface for adding a new Data Model.
// All fields are immutable after construction to avoid race conditions.
type AddDataModelAction struct {
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager

	// Parsed request payload (only populated after Parse)
	payload models.AddDataModelPayload

	actionLogger *zap.SugaredLogger
}

// NewAddDataModelAction returns an un-parsed action instance.
func NewAddDataModelAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *AddDataModelAction {
	return &AddDataModelAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface by extracting data model configuration from the payload.
func (a *AddDataModelAction) Parse(payload interface{}) error {
	// Parse the payload to get the data model configuration
	parsedPayload, err := ParseActionPayload[models.AddDataModelPayload](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %v", err)
	}

	a.payload = parsedPayload
	a.actionLogger.Debugf("Parsed AddDataModel action payload: name=%s, description=%s",
		a.payload.Name, a.payload.Description)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *AddDataModelAction) Validate() error {
	// Validate all required fields
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	if len(a.payload.Structure) == 0 {
		return errors.New("missing required field Structure")
	}

	return nil
}

// Execute implements the Action interface by creating the data model configuration.
func (a *AddDataModelAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing AddDataModel action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting to add data model: "+a.payload.Name, a.outboundChannel, models.AddDataModel)

	// Convert models types to config types
	dmVersion := config.DataModelVersion{
		Description: a.payload.Description,
		Structure:   a.convertModelsFieldsToConfigFields(a.payload.Structure),
	}

	// Add to configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Adding data model to configuration...", a.outboundChannel, models.AddDataModel)

	err := a.configManager.AtomicAddDataModel(ctx, a.payload.Name, dmVersion)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to add data model: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.AddDataModel)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Create response with the data model information
	response := map[string]interface{}{
		"name":        a.payload.Name,
		"description": a.payload.Description,
		"structure":   a.payload.Structure,
		"version":     1, // First version is always 1
	}

	return response, nil, nil
}

// convertModelsFieldsToConfigFields converts models.Field map to config.Field map
func (a *AddDataModelAction) convertModelsFieldsToConfigFields(modelsFields map[string]models.Field) map[string]config.Field {
	configFields := make(map[string]config.Field)

	for key, modelsField := range modelsFields {
		configFields[key] = config.Field{
			Type:        modelsField.Type,
			ModelRef:    modelsField.ModelRef,
			Subfields:   a.convertModelsFieldsToConfigFields(modelsField.Subfields),
			Description: modelsField.Description,
			Unit:        modelsField.Unit,
		}
	}

	return configFields
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *AddDataModelAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *AddDataModelAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed payload - exposed primarily for testing purposes.
func (a *AddDataModelAction) GetParsedPayload() models.AddDataModelPayload {
	return a.payload
}
