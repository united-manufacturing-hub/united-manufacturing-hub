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
// Getting a data contract returns one dataContractsV2 entry with every version
// and its structure as base64-encoded YAML, in the same shape get-datamodel
// returns. The status message only carries version names, so pages that
// start from an existing structure, such as the new version page, fetch it
// here.
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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// GetDataContractAction implements the Action interface for retrieving a data
// contract. All fields are immutable after construction to avoid race conditions.
type GetDataContractAction struct {
	configManager config.ConfigManager

	outboundChannel chan *models.UMHMessage

	actionLogger *zap.SugaredLogger
	userEmail    string

	// Parsed request payload (only populated after Parse)
	payload models.GetDataContractPayload

	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewGetDataContractAction returns an un-parsed action instance.
func NewGetDataContractAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *GetDataContractAction {
	return &GetDataContractAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface by extracting the contract name from the payload.
func (a *GetDataContractAction) Parse(payload interface{}) error {
	parsedPayload, err := ParseActionPayload[models.GetDataContractPayload](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	a.payload = parsedPayload
	a.actionLogger.Debugf("Parsed GetDataContract action payload: name=%s", a.payload.Name)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *GetDataContractAction) Validate() error {
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	return nil
}

// Execute implements the Action interface by reading the contract from the config.
func (a *GetDataContractAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing GetDataContract action")

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting to retrieve data contract: "+a.payload.Name, a.outboundChannel, models.GetDataContract)

	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	fullConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to get configuration: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.GetDataContract)

		return nil, nil, errors.New(errorMsg)
	}

	var found *config.DataContractV2Config

	for i := range fullConfig.DataContractsV2 {
		if fullConfig.DataContractsV2[i].Name == a.payload.Name {
			found = &fullConfig.DataContractsV2[i]

			break
		}
	}

	if found == nil {
		errorMsg := fmt.Sprintf("Data contract with name %q not found", a.payload.Name)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.GetDataContract)

		return nil, nil, errors.New(errorMsg)
	}

	versions := make(map[string]models.GetDataModelVersion, len(found.Versions))

	for versionKey, version := range found.Versions {
		structure := convertConfigFieldsToModelsFieldsForContract(version.Structure)

		yamlData, err := yaml.Marshal(structure)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to marshal data contract structure for version %s: %v", versionKey, err)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, a.outboundChannel, models.GetDataContract)

			return nil, nil, errors.New(errorMsg)
		}

		versions[versionKey] = models.GetDataModelVersion{
			EncodedStructure: base64.StdEncoding.EncodeToString(yamlData),
			Structure:        structure,
		}
	}

	return models.GetDataContractResponse{
		Name:        found.Name,
		Description: found.Description,
		Versions:    versions,
	}, nil, nil
}

// convertConfigFieldsToModelsFieldsForContract converts a config structure
// into the wire structure, the reverse of
// convertModelsFieldsToConfigFieldsForContract. _refModel targets are kept as
// references, not expanded.
func convertConfigFieldsToModelsFieldsForContract(configFields map[string]config.Field) map[string]models.Field {
	if configFields == nil {
		return nil
	}

	modelsFields := make(map[string]models.Field, len(configFields))

	for key, configField := range configFields {
		var modelRef *models.ModelRef
		if configField.ModelRef != nil {
			modelRef = &models.ModelRef{
				Name:    configField.ModelRef.Name,
				Version: configField.ModelRef.Version,
			}
		}

		modelsFields[key] = models.Field{
			PayloadShape: configField.PayloadShape,
			ModelRef:     modelRef,
			Subfields:    convertConfigFieldsToModelsFieldsForContract(configField.Subfields),
			Relational:   configRelationalToModels(configField.Relational),
		}
	}

	return modelsFields
}

func (a *GetDataContractAction) getUserEmail() string {
	return a.userEmail
}

func (a *GetDataContractAction) getUuid() uuid.UUID {
	return a.actionUUID
}
