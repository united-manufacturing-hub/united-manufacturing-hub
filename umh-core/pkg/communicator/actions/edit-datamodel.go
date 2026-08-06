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
// Additionally, it automatically creates a corresponding data contract for the
// new version with the naming pattern _{modelName}_{version}.
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

// EditDataModelAction implements the Action interface for editing an existing Data Model.
// All fields are immutable after construction to avoid race conditions.
type EditDataModelAction struct {
	configManager config.ConfigManager

	// Shared context for the entire action lifecycle (validate + execute)
	ctx context.Context

	outboundChannel chan *models.UMHMessage

	actionLogger *zap.SugaredLogger

	cancel context.CancelFunc

	// Parsed request payload (only populated after Parse)
	payload models.EditDataModelPayload

	userEmail string

	// expectedVersionKey is the version key Validate computed and ran
	// CheckAdditive against. Execute passes it back to the config manager as
	// a compare-and-swap guard, so a version that another concurrent edit
	// wrote in between gets caught instead of silently becoming the
	// unchecked predecessor of this one.
	expectedVersionKey string
	actionUUID         uuid.UUID
	instanceUUID       uuid.UUID
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
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	a.payload = parsedPayload

	decodedStructure, err := base64.StdEncoding.DecodeString(a.payload.EncodedStructure)
	if err != nil {
		return fmt.Errorf("failed to decode data model version: %w", err)
	}

	var structure map[string]models.Field

	err = yaml.Unmarshal(decodedStructure, &structure)
	if err != nil {
		return fmt.Errorf("failed to unmarshal data model version: %w", err)
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
		Structure: configStructure,
	}

	// Get all existing data models and payload shapes for validation
	currentConfig, err := a.configManager.GetConfig(a.ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get current config for validation: %w", err)
	}

	// Convert existing data models to the format expected by the validator
	allDataModels := make(map[string]config.DataModelsConfig)
	for _, dataModel := range currentConfig.DataModels {
		allDataModels[dataModel.Name] = dataModel
	}

	// Validate with references and payload shapes (handles cases with no references gracefully)
	if err := validator.ValidateWithReferences(a.ctx, dmVersion, allDataModels, currentConfig.PayloadShapes); err != nil {
		return fmt.Errorf("data model validation failed: %w", err)
	}

	existing, exists := allDataModels[a.payload.Name]
	if !exists {
		return fmt.Errorf("data model %q not found", a.payload.Name)
	}

	if err := validator.ValidateVersionKeys(existing.Versions); err != nil {
		return err
	}

	keys := make([]string, 0, len(existing.Versions))
	for versionKey := range existing.Versions {
		keys = append(keys, versionKey)
	}

	next, err := config.NextMinor(keys)
	if err != nil {
		return err
	}

	// A model with no versions yet has nothing to be additive over: skip the
	// check and let the write proceed to v1_0.
	if len(keys) > 0 {
		predecessor, err := previousMinorOf(existing.Versions, next)
		if err != nil {
			return err
		}

		changes, err := datamodel.CheckAdditive(a.ctx, predecessor, dmVersion, allDataModels, currentConfig.PayloadShapes)
		if err != nil {
			return fmt.Errorf("cannot check version %s of data model %q: %w", next, a.payload.Name, err)
		}

		if len(changes) > 0 {
			return errors.New(datamodel.FormatBreakingChanges(a.payload.Name, next.String(), changes))
		}
	}

	a.expectedVersionKey = next.String()

	return nil
}

// previousMinorOf returns the version the candidate must be additive over: the
// highest existing minor of the candidate's major. Versions are immutable and
// additivity is transitive, so comparing against the immediate predecessor is
// equivalent to comparing against all of them.
func previousMinorOf(versions map[string]config.DataModelVersion, next config.Version) (config.DataModelVersion, error) {
	var (
		best      config.Version
		bestKey   string
		haveMatch bool
	)

	for key := range versions {
		parsed, err := config.ParseVersion(key)
		if err != nil {
			return config.DataModelVersion{}, err
		}

		if parsed.Major != next.Major {
			continue
		}

		if !haveMatch || parsed.Compare(best) > 0 {
			best, bestKey, haveMatch = parsed, key, true
		}
	}

	if !haveMatch {
		return config.DataModelVersion{}, fmt.Errorf("no existing version of major %d to compare against", next.Major)
	}

	return versions[bestKey], nil
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

	// Safety validation before editing the data model
	validator := datamodel.NewValidator()
	if err := validator.ValidateStructureOnly(a.ctx, dmVersion); err != nil {
		errorMsg := fmt.Sprintf("Final validation failed before editing data model: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditDataModel)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Adding new version to data model configuration...", a.outboundChannel, models.EditDataModel)

	versionStr, err := a.configManager.AtomicAddDataModelVersionWithContract(
		a.ctx, a.payload.Name, dmVersion, a.payload.Description, a.expectedVersionKey)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to edit data model: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditDataModel)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	dataContractName := config.DataContractNameFor(a.payload.Name, versionStr)

	a.actionLogger.Infof("Successfully created data contract %s for data model %s version %s", dataContractName, a.payload.Name, versionStr)
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Data contract created successfully", a.outboundChannel, models.EditDataModel)

	// Create response with the data model information
	response := map[string]interface{}{
		"name":        a.payload.Name,
		"description": a.payload.Description,
		"structure":   a.payload.Structure,
		"version":     versionStr,
		"dataContract": map[string]interface{}{
			"name":  dataContractName,
			"model": fmt.Sprintf("%s:%s", a.payload.Name, versionStr),
		},
	}

	return response, nil, nil
}

// convertModelsFieldsToConfigFields converts models.Field map to config.Field map.
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
			Relational:   modelsRelationalToConfig(modelsField.Relational),
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
