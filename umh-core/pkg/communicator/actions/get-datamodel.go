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

// Package actions contains implementations of the Action interface that retrieve
// data model configurations from the UMH system.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A Data Model in UMH defines the structure of data that flows through the system.
// "Getting" a data model means retrieving all versions of an existing configuration
// entry by name, including the structure definitions as base64-encoded strings.
//
// The action retrieves a data model configuration with all its versions,
// providing access to the complete evolution history of the data model.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/configmanager"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// GetDataModelAction implements the Action interface for retrieving an existing Data Model.
// All fields are immutable after construction to avoid race conditions.
type GetDataModelAction struct {
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	outboundChannel chan *models.UMHMessage
	configManager   configmanager.ConfigManager

	// Parsed request payload (only populated after Parse)
	payload models.GetDataModelPayload

	actionLogger *zap.SugaredLogger
}

// NewGetDataModelAction returns an un-parsed action instance.
func NewGetDataModelAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager configmanager.ConfigManager) *GetDataModelAction {
	return &GetDataModelAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface by extracting data model name from the payload.
func (a *GetDataModelAction) Parse(payload interface{}) error {
	// Parse the payload to get the data model name
	parsedPayload, err := ParseActionPayload[models.GetDataModelPayload](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %v", err)
	}

	a.payload = parsedPayload
	a.actionLogger.Debugf("Parsed GetDataModel action payload: name=%s", a.payload.Name)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *GetDataModelAction) Validate() error {
	// Validate required fields
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	return nil
}

// Execute implements the Action interface by retrieving the data model configuration.
func (a *GetDataModelAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing GetDataModel action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting to retrieve data model: "+a.payload.Name, a.outboundChannel, models.GetDataModel)

	// Get configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Retrieving data model from configuration...", a.outboundChannel, models.GetDataModel)

	fullConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to get configuration: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.GetDataModel)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Find the data model by name
	var foundDataModel *config.DataModelsConfig
	for _, dmc := range fullConfig.DataModels {
		if dmc.Name == a.payload.Name {
			foundDataModel = &dmc
			break
		}
	}

	if foundDataModel == nil {
		errorMsg := fmt.Sprintf("Data model with name %q not found", a.payload.Name)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.GetDataModel)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Convert all versions to response format with base64-encoded structures
	responseVersions := make(map[string]models.GetDataModelVersion)
	for versionKey, versionData := range foundDataModel.Versions {
		// Convert config.Field to models.Field recursively
		modelsStructure := a.convertConfigFieldsToModelsFields(versionData.Structure)

		// Marshal the structure to YAML
		yamlData, err := yaml.Marshal(modelsStructure)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to marshal data model structure for version %s: %v", versionKey, err)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, a.outboundChannel, models.GetDataModel)
			return nil, nil, fmt.Errorf("%s", errorMsg)
		}

		// Base64 encode the YAML
		encodedStructure := base64.StdEncoding.EncodeToString(yamlData)

		responseVersions[versionKey] = models.GetDataModelVersion{
			Description:      versionData.Description,
			EncodedStructure: encodedStructure,
			Structure:        modelsStructure,
		}
	}

	// Create response with all versions
	response := models.GetDataModelResponse{
		Name:        foundDataModel.Name,
		Description: foundDataModel.Description,
		Versions:    responseVersions,
	}

	return response, nil, nil
}

// convertConfigFieldsToModelsFields converts config.Field map to models.Field map recursively
func (a *GetDataModelAction) convertConfigFieldsToModelsFields(configFields map[string]config.Field) map[string]models.Field {
	modelsFields := make(map[string]models.Field)

	for key, configField := range configFields {
		modelsFields[key] = models.Field{
			Type:        configField.Type,
			ModelRef:    configField.ModelRef,
			Subfields:   a.convertConfigFieldsToModelsFields(configField.Subfields),
			Description: configField.Description,
			Unit:        configField.Unit,
		}
	}

	return modelsFields
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *GetDataModelAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *GetDataModelAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed payload - exposed primarily for testing purposes.
func (a *GetDataModelAction) GetParsedPayload() models.GetDataModelPayload {
	return a.payload
}
