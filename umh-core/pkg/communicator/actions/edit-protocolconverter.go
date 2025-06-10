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
	// Parse the payload directly as a complete ProtocolConverter object
	pcPayload, err := ParseActionPayload[models.ProtocolConverter](payload)
	if err != nil {
		return fmt.Errorf("failed to parse protocol converter payload: %v", err)
	}

	// Extract UUID
	if pcPayload.UUID == nil {
		return errors.New("missing required field UUID")
	}
	a.protocolConverterUUID = *pcPayload.UUID
	a.name = pcPayload.Name

	// Determine which DFC is being updated and convert it to CDFCPayload
	var dfcToUpdate *models.ProtocolConverterDFC
	if pcPayload.ReadDFC != nil {
		a.dfcType = "read"
		dfcToUpdate = pcPayload.ReadDFC
	} else if pcPayload.WriteDFC != nil {
		a.dfcType = "write"
		dfcToUpdate = pcPayload.WriteDFC
	} else {
		return errors.New("no DFC configuration found in payload (readDFC or writeDFC required)")
	}

	// Convert ProtocolConverterDFC to CDFCPayload for internal processing
	a.dfcPayload = models.CDFCPayload{
		Inputs:   models.DfcDataConfig{Data: dfcToUpdate.Inputs.Data, Type: dfcToUpdate.Inputs.Type},
		Pipeline: convertPipelineToMap(dfcToUpdate.Pipeline),
		// Set default outputs since ProtocolConverterDFC doesn't have outputs
		Outputs: models.DfcDataConfig{Data: "", Type: ""},
	}

	// Handle optional fields
	if dfcToUpdate.IgnoreErrors != nil {
		a.dfcPayload.IgnoreErrors = *dfcToUpdate.IgnoreErrors
	}

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

	if err := ValidateCustomDataFlowComponentPayload(a.dfcPayload, false); err != nil {
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

	// Extract connection info from the updated config
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

	// Extract location from config
	location := make(map[int]string)
	if len(targetPC.ProtocolConverterServiceConfig.Location) > 0 {
		for k, v := range targetPC.ProtocolConverterServiceConfig.Location {
			var intKey int
			if _, err := fmt.Sscanf(k, "%d", &intKey); err == nil {
				location[intKey] = v
			}
		}
	}

	// Build ReadDFC if present
	var readDFC *models.ProtocolConverterDFC
	if readDFCConfig := targetPC.ProtocolConverterServiceConfig.Template.DataflowComponentReadServiceConfig; len(readDFCConfig.BenthosConfig.Input) > 0 {
		var err error
		readDFC, err = a.buildProtocolConverterDFCFromConfig(readDFCConfig)
		if err != nil {
			a.actionLogger.Warnf("Failed to build read DFC: %v", err)
		}
	}

	// Build WriteDFC if present
	var writeDFC *models.ProtocolConverterDFC
	if writeDFCConfig := targetPC.ProtocolConverterServiceConfig.Template.DataflowComponentWriteServiceConfig; len(writeDFCConfig.BenthosConfig.Input) > 0 {
		var err error
		writeDFC, err = a.buildProtocolConverterDFCFromConfig(writeDFCConfig)
		if err != nil {
			a.actionLogger.Warnf("Failed to build write DFC: %v", err)
		}
	}

	// Create meta information
	meta := &models.ProtocolConverterMeta{
		ProcessingMode: a.determineProcessingMode(readDFC),
		Protocol:       a.determineProtocol(readDFC),
	}

	response := models.ProtocolConverter{
		UUID:     &a.protocolConverterUUID,
		Name:     targetPC.Name,
		Location: location,
		Connection: models.ProtocolConverterConnection{
			IP:   ip,
			Port: port,
		},
		ReadDFC:  readDFC,
		WriteDFC: writeDFC,
		Meta:     meta,
		// TemplateInfo can be added later if needed
		TemplateInfo: nil,
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

// buildProtocolConverterDFCFromConfig converts a dataflow component service config
// into the models.ProtocolConverterDFC format expected by the API using the shared function.
func (a *EditProtocolConverterAction) buildProtocolConverterDFCFromConfig(dfcConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig) (*models.ProtocolConverterDFC, error) {
	if len(dfcConfig.BenthosConfig.Input) == 0 {
		// No DFC configuration present
		return nil, nil
	}

	// Use the shared function to build the common DFC properties
	commonPayload, err := BuildCommonDataFlowComponentPropertiesFromConfig(dfcConfig, a.actionLogger)
	if err != nil {
		return nil, err
	}

	// Convert the common payload to ProtocolConverterDFC format
	dfc := &models.ProtocolConverterDFC{
		Inputs:   commonPayload.CDFCProperties.Inputs,
		Pipeline: commonPayload.CDFCProperties.Pipeline,
		RawYAML:  commonPayload.CDFCProperties.RawYAML,
	}

	return dfc, nil
}

// determineProcessingMode analyzes the pipeline processors in readDFC only
// to determine the appropriate processing mode based on the business rules.
func (a *EditProtocolConverterAction) determineProcessingMode(readDFC *models.ProtocolConverterDFC) string {
	// Only look at readDFC as requested
	if readDFC == nil {
		return "no_dfc"
	}

	processors := readDFC.Pipeline.Processors

	// If more than one processor, return custom
	if len(processors) > 1 {
		return "custom"
	}

	// If exactly one processor, check its type
	if len(processors) == 1 {
		// Get the first (and only) processor from the map
		for _, processor := range processors {
			switch processor.Type {
			case "nodered_js":
				return "nodered_js"
			case "tag_processor":
				return "tag_processor"
			default:
				return "custom"
			}
		}
	}

	// No processors found, fall back to custom
	return "custom"
}

// determineProtocol analyzes the input processors to determine the protocol
func (a *EditProtocolConverterAction) determineProtocol(readDFC *models.ProtocolConverterDFC) string {
	if readDFC == nil {
		return "generic"
	}

	input := readDFC.Inputs

	return input.Type
}

// convertPipelineToMap converts CommonDataFlowComponentPipelineConfig to map[string]DfcDataConfig
func convertPipelineToMap(pipeline models.CommonDataFlowComponentPipelineConfig) map[string]models.DfcDataConfig {
	result := make(map[string]models.DfcDataConfig)
	for key, processor := range pipeline.Processors {
		result[key] = models.DfcDataConfig{
			Data: processor.Data,
			Type: processor.Type,
		}
	}
	return result
}
