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
// protocol converter configurations, particularly for adding dataflow components
// to existing protocol converters.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// An existing Protocol Converter (PC) in UMH starts as a basic connection template.
// The edit action allows adding actual dataflow component configurations (read/write)
// to the protocol converter, effectively making it functional for data processing.
//
// The action follows a pattern similar to deploy-dataflowcomponent but operates
// on an existing protocol converter configuration instead of creating a new one.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// EditProtocolConverterAction implements the Action interface for editing
// protocol converter configurations, particularly for adding DFC configurations.
type EditProtocolConverterAction struct {
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager

	// Parsed request payload (only populated after Parse)
	protocolConverterUUID uuid.UUID
	name                  string // protocol converter name (optional for updates)
	dfcPayload            models.CDFCPayload
	dfcType               string // "read" or "write"
	ignoreHealthCheck     bool

	// Runtime observation for health checks
	systemSnapshotManager *fsm.SnapshotManager

	actionLogger *zap.SugaredLogger
}

// NewEditProtocolConverterAction returns an un-parsed action instance.
func NewEditProtocolConverterAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *EditProtocolConverterAction {
	return &EditProtocolConverterAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		systemSnapshotManager: systemSnapshotManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface by extracting the protocol converter UUID and
// dataflow component configuration from the payload.
func (a *EditProtocolConverterAction) Parse(payload interface{}) error {
	// Parse the top-level structure to get UUID and DFC configuration
	topLevel, err := ParseDataflowComponentTopLevel(payload)
	if err != nil {
		return fmt.Errorf("failed to parse top level payload: %v", err)
	}

	a.name = topLevel.Name
	a.ignoreHealthCheck = topLevel.IgnoreHealthCheck

	// Extract UUID from the DFC name - this assumes the name format includes the protocol converter UUID
	// For edit operations, we need both the PC UUID and the DFC configuration
	// The payload should include the protocol converter UUID in the payload section
	type EditPCPayload struct {
		UUID    string      `json:"uuid"`
		DFCType string      `json:"dfcType"` // "read" or "write"
		DFC     interface{} `json:"dfc"`     // the actual DFC configuration
	}

	// Parse the nested payload to get UUID and DFC config
	editPayload, err := ParseActionPayload[EditPCPayload](topLevel.Payload)
	if err != nil {
		return fmt.Errorf("failed to parse edit protocol converter payload: %v", err)
	}

	// Validate required fields
	if editPayload.UUID == "" {
		return errors.New("missing required field UUID")
	}

	pcUUID, err := uuid.Parse(editPayload.UUID)
	if err != nil {
		return fmt.Errorf("invalid UUID format: %v", err)
	}
	a.protocolConverterUUID = pcUUID

	if editPayload.DFCType == "" {
		return errors.New("missing required field dfcType")
	}
	if editPayload.DFCType != "read" && editPayload.DFCType != "write" {
		return fmt.Errorf("invalid dfcType: %s, must be 'read' or 'write'", editPayload.DFCType)
	}
	a.dfcType = editPayload.DFCType

	dfcPayload, err := ParseCustomDataFlowComponent(editPayload.DFC)
	if err != nil {
		return fmt.Errorf("failed to parse custom dataflow component: %v", err)
	}
	a.dfcPayload = dfcPayload

	a.actionLogger.Debugf("Parsed EditProtocolConverter action payload: uuid=%s, name=%s, dfcType=%s",
		a.protocolConverterUUID, a.name, a.dfcType)
	return nil
}

// Validate performs validation of the parsed payload.
func (a *EditProtocolConverterAction) Validate() error {
	// Validate UUID and DFC type
	if a.protocolConverterUUID == uuid.Nil {
		return errors.New("missing or invalid protocol converter UUID")
	}

	if a.dfcType == "" {
		return errors.New("missing required field dfcType")
	}

	if err := ValidateCustomDataFlowComponentPayload(a.dfcPayload); err != nil {
		return fmt.Errorf("invalid dataflow component configuration: %v", err)
	}

	return nil
}

// Execute implements the Action interface by updating the protocol converter configuration
// with the provided dataflow component configuration.
func (a *EditProtocolConverterAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing EditProtocolConverter action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		fmt.Sprintf("Starting edit of protocol converter %s to add %s DFC", a.protocolConverterUUID, a.dfcType),
		a.outboundChannel, models.EditProtocolConverter)

	// Convert the DFC payload to BenthosConfig
	benthosConfig, err := CreateBenthosConfigFromCDFCPayload(a.dfcPayload, a.name)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to create Benthos configuration: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		fmt.Sprintf("Updating protocol converter configuration with %s DFC...", a.dfcType),
		a.outboundChannel, models.EditProtocolConverter)

	// Get current configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	currentConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to get current configuration: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Find the protocol converter in the configuration
	var targetPC config.ProtocolConverterConfig
	found := false
	for _, pc := range currentConfig.ProtocolConverter {
		pcID := dataflowcomponentserviceconfig.GenerateUUIDFromName(pc.Name)
		if pcID == a.protocolConverterUUID {
			targetPC = pc
			found = true
			break
		}
	}

	if !found {
		errorMsg := fmt.Sprintf("Protocol converter with UUID %s not found", a.protocolConverterUUID)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Update the appropriate DFC configuration based on dfcType
	dfcServiceConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: benthosConfig,
	}

	switch a.dfcType {
	case "read":
		targetPC.ProtocolConverterServiceConfig.Template.DataflowComponentReadServiceConfig = dfcServiceConfig
	case "write":
		targetPC.ProtocolConverterServiceConfig.Template.DataflowComponentWriteServiceConfig = dfcServiceConfig
	default:
		errorMsg := fmt.Sprintf("Invalid DFC type: %s", a.dfcType)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Update the protocol converter using atomic operation
	oldConfig, err := a.configManager.AtomicEditProtocolConverter(ctx, a.protocolConverterUUID, targetPC)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to update protocol converter: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// TODO: Health check waiting logic similar to deploy-dataflowcomponent
	// if a.systemSnapshotManager != nil && !a.ignoreHealthCheck {
	//     // Wait for protocol converter to be active
	// }

	// Create response with updated protocol converter - extract IP and PORT from variables
	var ip string
	var port uint32

	if ipVar, exists := targetPC.ProtocolConverterServiceConfig.Variables.User["IP"]; exists {
		if ipStr, ok := ipVar.(string); ok {
			ip = ipStr
		}
	}

	if portVar, exists := targetPC.ProtocolConverterServiceConfig.Variables.User["PORT"]; exists {
		if portStr, ok := portVar.(string); ok {
			// PORT is stored as string, need to convert
			if portInt, parseErr := fmt.Sscanf(portStr, "%d", &port); parseErr != nil || portInt != 1 {
				a.actionLogger.Warnf("Failed to parse PORT variable as integer: %v", portStr)
			}
		}
	}

	response := models.ProtocolConverter{
		UUID: &a.protocolConverterUUID,
		Name: targetPC.Name,
		Connection: models.ProtocolConverterConnection{
			IP:   ip,
			Port: port,
		},
		// Additional fields would be populated from the updated config
		// ReadDFC, WriteDFC, TemplateInfo etc. could be populated here
	}

	_ = oldConfig // Suppress unused variable warning

	return response, nil, nil
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *EditProtocolConverterAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *EditProtocolConverterAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed DFC payload - exposed primarily for testing purposes.
func (a *EditProtocolConverterAction) GetParsedPayload() models.CDFCPayload {
	return a.dfcPayload
}

// GetProtocolConverterUUID returns the protocol converter UUID - exposed for testing purposes.
func (a *EditProtocolConverterAction) GetProtocolConverterUUID() uuid.UUID {
	return a.protocolConverterUUID
}

// GetDFCType returns the DFC type (read/write) - exposed for testing purposes.
func (a *EditProtocolConverterAction) GetDFCType() string {
	return a.dfcType
}
