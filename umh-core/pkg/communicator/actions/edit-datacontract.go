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
// Editing a data contract appends its next version to the dataContractsV2
// entry. Versions are append-only: the new one is one above the highest
// existing v<number>, and no existing version changes, because flows and
// bridges write to a version's address (_<name>_<version>).
//
// The description is left as it is; the new version page does not edit it.
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

// EditDataContractAction implements the Action interface for appending the next
// version to a data contract. All fields are immutable after construction to
// avoid race conditions.
type EditDataContractAction struct {

	// Parsed request payload (only populated after Parse)
	payload models.EditDataContractPayload

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

// NewEditDataContractAction returns an un-parsed action instance.
func NewEditDataContractAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *EditDataContractAction {
	// Create shared context with timeout for the entire action lifecycle
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)

	return &EditDataContractAction{
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

// Parse implements the Action interface by extracting the new version from the payload.
func (a *EditDataContractAction) Parse(payload interface{}) error {
	parsedPayload, err := ParseActionPayload[models.EditDataContractPayload](payload)
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

	a.actionLogger.Debugf("Parsed EditDataContract action payload: name=%s", a.payload.Name)

	return nil
}

// Validate checks that the contract exists and that the new structure is valid.
func (a *EditDataContractAction) Validate() error {
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	if len(a.payload.Structure) == 0 {
		return errors.New("missing required field Structure")
	}

	currentConfig, err := a.configManager.GetConfig(a.ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to get current config for validation: %w", err)
	}

	found := false

	for _, contract := range currentConfig.DataContractsV2 {
		if contract.Name == a.payload.Name {
			found = true

			break
		}
	}

	if !found {
		return fmt.Errorf("data contract with name %q not found", a.payload.Name)
	}

	dcVersion := config.DataModelVersion{
		Structure: convertModelsFieldsToConfigFieldsForContract(a.payload.Structure),
	}

	validator := datamodel.NewValidator()
	if err := validator.ValidateWithReferences(a.ctx, dcVersion, referenceableDataContracts(currentConfig), currentConfig.PayloadShapes); err != nil {
		return fmt.Errorf("data contract validation failed: %w", err)
	}

	return nil
}

// Execute implements the Action interface by appending the new version to the config.
func (a *EditDataContractAction) Execute() (interface{}, map[string]interface{}, error) {
	// Ensure context is cleaned up when action completes
	defer a.cancel()

	a.actionLogger.Info("Executing EditDataContract action")

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting to add a version to data contract: "+a.payload.Name, a.outboundChannel, models.EditDataContract)

	dcVersion := config.DataModelVersion{
		Structure: convertModelsFieldsToConfigFieldsForContract(a.payload.Structure),
	}

	// Safety validation before writing to config
	validator := datamodel.NewValidator()
	if err := validator.ValidateStructureOnly(a.ctx, dcVersion); err != nil {
		errorMsg := fmt.Sprintf("Final validation failed before adding the version: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditDataContract)

		return nil, nil, errors.New(errorMsg)
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Adding the new version to the configuration...", a.outboundChannel, models.EditDataContract)

	versionKey, err := a.configManager.AtomicAddDataContractV2Version(a.ctx, a.payload.Name, dcVersion)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to add the version: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditDataContract)

		return nil, nil, errors.New(errorMsg)
	}

	response := map[string]interface{}{
		"name":     a.payload.Name,
		"version":  versionKey,
		"contract": "_" + a.payload.Name + "_" + versionKey,
	}

	return response, nil, nil
}

func (a *EditDataContractAction) getUserEmail() string {
	return a.userEmail
}

func (a *EditDataContractAction) getUuid() uuid.UUID {
	return a.actionUUID
}

func (a *EditDataContractAction) getParsedPayload() interface{} {
	return a.payload
}
