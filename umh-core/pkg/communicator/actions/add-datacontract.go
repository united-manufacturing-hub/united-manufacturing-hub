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

// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A Data Contract is the merged concept that supersedes data models: one entry
// holds a name and every version of the structure it enforces.
//
// Adding one writes a single entry to the dataContractsV2 section, starting at
// v1. Unlike add-datamodel it creates nothing in the dataModels or
// dataContracts sections, so the two concepts never write into each other.
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

// AddDataContractAction implements the Action interface for adding a data contract.
// All fields are immutable after construction to avoid race conditions.
type AddDataContractAction struct {

	// Parsed request payload (only populated after Parse)
	payload models.AddDataContractPayload

	configManager config.ConfigManager

	// Shared context for the entire action lifecycle (validate + execute)
	ctx context.Context

	outboundChannel chan *models.UMHMessage

	actionLogger *zap.SugaredLogger

	cancel       context.CancelFunc
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewAddDataContractAction returns an un-parsed action instance.
func NewAddDataContractAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *AddDataContractAction {
	// Create shared context with timeout for the entire action lifecycle
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)

	return &AddDataContractAction{
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

// Parse implements the Action interface by extracting the data contract from the payload.
func (a *AddDataContractAction) Parse(payload interface{}) error {
	parsedPayload, err := ParseActionPayload[models.AddDataContractPayload](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	a.payload = parsedPayload

	decodedStructure, err := base64.StdEncoding.DecodeString(a.payload.EncodedStructure)
	if err != nil {
		return fmt.Errorf("failed to decode data contract structure: %w", err)
	}

	var structure map[string]models.Field

	err = yaml.Unmarshal(decodedStructure, &structure)
	if err != nil {
		return fmt.Errorf("failed to unmarshal data contract structure: %w", err)
	}

	a.payload.Structure = structure

	a.actionLogger.Debugf("Parsed AddDataContract action payload: name=%s, description=%s",
		a.payload.Name, a.payload.Description)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *AddDataContractAction) Validate() error {
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	// The contract address and the PostgreSQL tables derived from it are named
	// after the contract, so the same 53 character ceiling applies here.
	if len(a.payload.Name) > maxContractLen {
		return fmt.Errorf("data contract name %q is %d characters; use %d or fewer, because PostgreSQL truncates the table name at 63 bytes", a.payload.Name, len(a.payload.Name), maxContractLen)
	}

	if len(a.payload.Structure) == 0 {
		return errors.New("missing required field Structure")
	}

	dcVersion := config.DataModelVersion{
		Structure: convertModelsFieldsToConfigFieldsForContract(a.payload.Structure),
	}

	currentConfig, err := a.configManager.GetConfig(a.ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get current config for validation: %w", err)
	}

	if err := config.CheckLegacyAddressFree(currentConfig, a.payload.Name); err != nil {
		return err
	}

	validator := datamodel.NewValidator()
	if err := validator.ValidateWithReferences(a.ctx, dcVersion, referenceableDataContracts(currentConfig), currentConfig.PayloadShapes); err != nil {
		return fmt.Errorf("data contract validation failed: %w", err)
	}

	return nil
}

// Execute implements the Action interface by writing the data contract to the config.
func (a *AddDataContractAction) Execute() (interface{}, map[string]interface{}, error) {
	// Ensure context is cleaned up when action completes
	defer a.cancel()

	a.actionLogger.Info("Executing AddDataContract action")

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting to add data contract: "+a.payload.Name, a.outboundChannel, models.AddDataContract)

	dcVersion := config.DataModelVersion{
		Structure: convertModelsFieldsToConfigFieldsForContract(a.payload.Structure),
	}

	// Safety validation before writing to config
	validator := datamodel.NewValidator()
	if err := validator.ValidateStructureOnly(a.ctx, dcVersion); err != nil {
		errorMsg := fmt.Sprintf("Final validation failed before adding data contract: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.AddDataContract)

		return nil, nil, errors.New(errorMsg)
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Adding data contract to configuration...", a.outboundChannel, models.AddDataContract)

	err := a.configManager.AtomicAddDataContractV2(a.ctx, a.payload.Name, dcVersion, a.payload.Description)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to add data contract: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.AddDataContract)

		return nil, nil, errors.New(errorMsg)
	}

	return fmt.Sprintf("Data contract %s added successfully", a.payload.Name), nil, nil
}

func (a *AddDataContractAction) getUserEmail() string {
	return a.userEmail
}

func (a *AddDataContractAction) getUuid() uuid.UUID {
	return a.actionUUID
}

func (a *AddDataContractAction) getParsedPayload() interface{} {
	return a.payload
}

// referenceableDataContracts returns the dataContractsV2 entries in the form
// the validator resolves _refModel targets against. They resolve against the
// data contracts section, not the data models section, so a contract can only
// reference another contract.
func referenceableDataContracts(currentConfig config.FullConfig) map[string]config.DataModelsConfig {
	referenceable := make(map[string]config.DataModelsConfig, len(currentConfig.DataContractsV2))
	for _, contract := range currentConfig.DataContractsV2 {
		referenceable[contract.Name] = config.DataModelsConfig{
			Name:        contract.Name,
			Description: contract.Description,
			Versions:    contract.Versions,
		}
	}

	return referenceable
}

// convertModelsFieldsToConfigFieldsForContract converts a wire structure into
// the config structure. It mirrors the data model conversion because both carry
// the same field shape over the wire.
func convertModelsFieldsToConfigFieldsForContract(modelsFields map[string]models.Field) map[string]config.Field {
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
			subfields = convertModelsFieldsToConfigFieldsForContract(modelsField.Subfields)
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
