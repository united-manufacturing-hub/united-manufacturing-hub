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

// Package actions contains implementations of the Action interface that edit
// data model configurations in the UMH system.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A Data Model in UMH defines the structure of data that flows through the system.
// "Editing" a data model means adding a new version to an existing configuration
// entry while preserving all previous versions to maintain data contracts.
//
// The action creates a new version of an existing data model configuration,
// incrementing the version number and preserving backward compatibility.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// EditDataModelAction implements the Action interface for editing an existing Data Model.
// All fields are immutable after construction to avoid race conditions.
type EditDataModelAction struct {
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager

	// Parsed request payload (only populated after Parse)
	payload models.EditDataModelPayload

	actionLogger *zap.SugaredLogger

	// Shared context for the entire action lifecycle (validate + execute)
	ctx    context.Context
	cancel context.CancelFunc
}

// NewEditDataModelAction returns an un-parsed action instance.
func NewEditDataModelAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *EditDataModelAction {
	// Create shared context with timeout for the entire action lifecycle
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)

	return &EditDataModelAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Parse implements the Action interface by extracting data model configuration from the payload.
func (a *EditDataModelAction) Parse(payload interface{}) error {
	// Parse the payload to get the data model configuration
	parsedPayload, err := ParseActionPayload[models.EditDataModelPayload](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %v", err)
	}

	a.payload = parsedPayload
	decodedStructure, err := base64.StdEncoding.DecodeString(a.payload.EncodedStructure)
	if err != nil {
		return fmt.Errorf("failed to decode data model version: %v", err)
	}

	var structure map[string]models.Field
	err = yaml.Unmarshal(decodedStructure, &structure)
	if err != nil {
		return fmt.Errorf("failed to unmarshal data model version: %v", err)
	}

	a.payload.Structure = structure

	a.actionLogger.Debugf("Parsed EditDataModel action payload: name=%s, description=%s",
		a.payload.Name, a.payload.Description)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *EditDataModelAction) Validate() error {
	// Validate all required fields
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	if len(a.payload.Structure) == 0 {
		return errors.New("missing required field Structure")
	}

	// Validate data model structure using our new validator
	validator := datamodel.NewValidator()

	// Convert models structure to config structure for validation
	configStructure := a.convertModelsFieldsToConfigFields(a.payload.Structure)

	dmVersion := config.DataModelVersion{
		Description: a.payload.Description,
		Structure:   configStructure,
	}

	// Use shared context for validation
	if err := validator.ValidateStructureOnly(a.ctx, dmVersion); err != nil {
		return fmt.Errorf("data model structure validation failed: %v", err)
	}

	return nil
}

// Execute implements the Action interface by creating a new version of the data model configuration.
func (a *EditDataModelAction) Execute() (interface{}, map[string]interface{}, error) {
	// Ensure context is cleaned up when action completes
	defer a.cancel()

	a.actionLogger.Info("Executing EditDataModel action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting to edit data model: "+a.payload.Name, a.outboundChannel, models.EditDataModel)

	// Convert models types to config types
	dmVersion := config.DataModelVersion{
		Structure: a.convertModelsFieldsToConfigFields(a.payload.Structure),
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Adding new version to data model configuration...", a.outboundChannel, models.EditDataModel)

	// Use shared context for execution
	err := a.configManager.AtomicEditDataModel(a.ctx, a.payload.Name, dmVersion)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to edit data model: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditDataModel)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Get the updated configuration to determine the new version number
	config, err := a.configManager.GetConfig(a.ctx, 0)
	if err != nil {
		a.actionLogger.Warnf("Failed to get config to determine new version number: %v", err)
		// Continue with execution, just use a placeholder version
	}

	// Find the new version number
	newVersion := uint64(0)
	if err == nil {
		for _, dmc := range config.DataModels {
			if dmc.Name == a.payload.Name {
				var maxVersion uint64 = 0
				for versionKey := range dmc.Versions {
					if strings.HasPrefix(versionKey, "v") {
						if versionNum, err := strconv.Atoi(versionKey[1:]); err == nil {
							if uint64(versionNum) > maxVersion {
								maxVersion = uint64(versionNum)
							}
						}
					}
				}
				newVersion = maxVersion
				break
			}
		}
	}

	if newVersion == 0 {
		errorMsg := "Failed to edit data model: new version number not found"
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditDataModel)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Create response with the data model information
	response := map[string]interface{}{
		"name":        a.payload.Name,
		"description": a.payload.Description,
		"structure":   a.payload.Structure,
		"version":     newVersion,
	}

	return response, nil, nil
}

// convertModelsFieldsToConfigFields converts models.Field map to config.Field map
func (a *EditDataModelAction) convertModelsFieldsToConfigFields(modelsFields map[string]models.Field) map[string]config.Field {
	if modelsFields == nil {
		return nil
	}

	configFields := make(map[string]config.Field)

	for key, modelsField := range modelsFields {
		var configModelRef *config.ModelRef
		if modelsField.ModelRef != nil {
			configModelRef = &config.ModelRef{
				Name:    modelsField.ModelRef.Name,
				Version: modelsField.ModelRef.Version,
			}
		}

		var subfields map[string]config.Field
		if modelsField.Subfields != nil {
			subfields = a.convertModelsFieldsToConfigFields(modelsField.Subfields)
		}

		configFields[key] = config.Field{
			PayloadShape: modelsField.PayloadShape,
			ModelRef:     configModelRef,
			Subfields:    subfields,
		}
	}

	return configFields
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *EditDataModelAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *EditDataModelAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed payload - exposed primarily for testing purposes.
func (a *EditDataModelAction) GetParsedPayload() models.EditDataModelPayload {
	return a.payload
}
