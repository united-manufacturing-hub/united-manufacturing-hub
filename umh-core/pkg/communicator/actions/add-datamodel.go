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
// data validation and structure enforcement throughout the system. Additionally,
// it automatically creates a corresponding data contract that binds the data
// model to storage and processing policies, following the naming convention
// of prefixing the data model name with an underscore (e.g., "Temperature"
// becomes "_Temperature").
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
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

	// Shared context for the entire action lifecycle (validate + execute)
	ctx    context.Context
	cancel context.CancelFunc
}

// NewAddDataModelAction returns an un-parsed action instance.
func NewAddDataModelAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *AddDataModelAction {
	// Create shared context with timeout for the entire action lifecycle
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)

	return &AddDataModelAction{
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
func (a *AddDataModelAction) Parse(payload interface{}) error {
	// Parse the payload to get the data model configuration
	parsedPayload, err := ParseActionPayload[models.AddDataModelPayload](payload)
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

	// Validate data model structure using our new validator
	validator := datamodel.NewValidator()

	// Convert models structure to config structure for validation
	configStructure := a.convertModelsFieldsToConfigFields(a.payload.Structure)

	dmVersion := config.DataModelVersion{
		Structure: configStructure,
	}

	// Use shared context for validation
	if err := validator.ValidateStructureOnly(a.ctx, dmVersion); err != nil {
		return fmt.Errorf("data model structure validation failed: %v", err)
	}

	return nil
}

// Execute implements the Action interface by creating the data model configuration.
func (a *AddDataModelAction) Execute() (interface{}, map[string]interface{}, error) {
	// Ensure context is cleaned up when action completes
	defer a.cancel()

	a.actionLogger.Info("Executing AddDataModel action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting to add data model: "+a.payload.Name, a.outboundChannel, models.AddDataModel)

	// Convert models types to config types
	dmVersion := config.DataModelVersion{
		Structure: a.convertModelsFieldsToConfigFields(a.payload.Structure),
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Adding data model to configuration...", a.outboundChannel, models.AddDataModel)

	// Use shared context for execution
	err := a.configManager.AtomicAddDataModel(a.ctx, a.payload.Name, dmVersion, a.payload.Description)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to add data model: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.AddDataModel)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Creating data contract for data model...", a.outboundChannel, models.AddDataModel)

	// Automatically create a data contract for the newly added data model
	dataContractName := "_" + a.payload.Name + "_v1" // Data contract names start with underscore, first version is always v1
	dataContract := config.DataContractsConfig{
		Name: dataContractName,
		Model: &config.ModelRef{
			Name:    a.payload.Name,
			Version: "v1", // First version is always v1
		},
	}

	dataContractErr := a.configManager.AtomicAddDataContract(a.ctx, dataContract)
	if dataContractErr != nil {
		// Log the error but don't fail the entire operation since the data model was successfully added
		a.actionLogger.Warnf("Failed to automatically create data contract for data model %s: %v", a.payload.Name, dataContractErr)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			fmt.Sprintf("Data model added successfully, but failed to create data contract: %v", dataContractErr), a.outboundChannel, models.AddDataModel)
	} else {
		a.actionLogger.Infof("Successfully created data contract %s for data model %s", dataContractName, a.payload.Name)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			"Data contract created successfully", a.outboundChannel, models.AddDataModel)
	}

	// Create response with the data model information
	response := map[string]interface{}{
		"name":        a.payload.Name,
		"description": a.payload.Description,
		"structure":   a.payload.Structure,
		"version":     1, // First version is always 1
		"dataContract": map[string]interface{}{
			"name":  dataContractName,
			"model": a.payload.Name + ":v1",
			"status": func() string {
				if dataContractErr != nil {
					return "failed"
				}
				return "created"
			}(),
		},
	}

	return response, nil, nil
}

// convertModelsFieldsToConfigFields converts models.Field map to config.Field map
func (a *AddDataModelAction) convertModelsFieldsToConfigFields(modelsFields map[string]models.Field) map[string]config.Field {
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
