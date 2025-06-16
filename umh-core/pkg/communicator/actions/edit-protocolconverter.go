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
	"maps"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
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
	vb                    []models.ProtocolConverterVariable
	ignoreHealthCheck     bool

	// Runtime observation for health checks
	systemSnapshotManager *fsm.SnapshotManager

	// Desired DFC config for comparison during health checks
	desiredDFCConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig

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

	if pcPayload.TemplateInfo != nil {
		a.vb = pcPayload.TemplateInfo.Variables
	} else {
		a.vb = make([]models.ProtocolConverterVariable, 0)
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
		a.ignoreHealthCheck = *dfcToUpdate.IgnoreErrors
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

	// currently, we canot reuse templates, so we need to create a new one
	targetPC.ProtocolConverterServiceConfig.TemplateRef = targetPC.Name

	if !found {
		errorMsg := fmt.Sprintf("Protocol converter with UUID %s not found", a.protocolConverterUUID)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Classify the protocol converter instance
	isChild := targetPC.ProtocolConverterServiceConfig.TemplateRef != "" &&
		targetPC.ProtocolConverterServiceConfig.TemplateRef != targetPC.Name

	// Determine which instance to modify and which UUID to use for atomic operation
	var instanceToModify config.ProtocolConverterConfig
	var atomicEditUUID uuid.UUID
	var newVB map[string]any

	if isChild {
		// Find the root instance
		var rootPC config.ProtocolConverterConfig
		rootFound := false
		for _, pc := range currentConfig.ProtocolConverter {
			if pc.Name == targetPC.ProtocolConverterServiceConfig.TemplateRef &&
				pc.ProtocolConverterServiceConfig.TemplateRef == pc.Name {
				rootPC = pc
				rootFound = true
				break
			}
		}

		if !rootFound {
			errorMsg := fmt.Sprintf("Root template %s not found for child protocol converter %s",
				targetPC.ProtocolConverterServiceConfig.TemplateRef, targetPC.Name)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, a.outboundChannel, models.EditProtocolConverter)
			return nil, nil, fmt.Errorf("%s", errorMsg)
		}

		// Apply mutations to the root, but keep child's variables
		instanceToModify = rootPC
		atomicEditUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(rootPC.Name)

		// preserve existing child variables
		maps.Copy(newVB, targetPC.ProtocolConverterServiceConfig.Variables.User)
	} else {
		// Root or stand-alone: apply mutations directly
		instanceToModify = targetPC
		atomicEditUUID = a.protocolConverterUUID

		// add the variables and keep the existing variables
		newVB = make(map[string]any)
		for _, variable := range a.vb {
			newVB[variable.Label] = variable.Value
		}
		maps.Copy(newVB, targetPC.ProtocolConverterServiceConfig.Variables.User)
	}

	// as the BuildRuntimeConfig function always adds location and location_path to the user variables,
	// we need to remove them from the variables here to avoid that they end up in the config file
	delete(newVB, "location")
	delete(newVB, "location_path")

	instanceToModify.ProtocolConverterServiceConfig.Variables.User = newVB

	// Update the appropriate DFC configuration based on dfcType
	dfcServiceConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: benthosConfig,
	}

	// Store the desired DFC config for health check comparison
	a.desiredDFCConfig = dfcServiceConfig

	// add the connection details to the template
	instanceToModify.ProtocolConverterServiceConfig.Config.ConnectionServiceConfig = connectionserviceconfig.ConnectionServiceConfigTemplate{
		NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
			Target: "{{ .IP }}",
			Port:   "{{ .PORT }}",
		},
	}

	switch a.dfcType {
	case "read":
		instanceToModify.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dfcServiceConfig
	case "write":
		instanceToModify.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dfcServiceConfig
	default:
		errorMsg := fmt.Sprintf("Invalid DFC type: %s", a.dfcType)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Update the protocol converter using atomic operation with the correct UUID
	oldConfig, err := a.configManager.AtomicEditProtocolConverter(ctx, atomicEditUUID, instanceToModify)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to update protocol converter: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Health check waiting logic similar to deploy-dataflowcomponent
	if a.systemSnapshotManager != nil && !a.ignoreHealthCheck {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			fmt.Sprintf("Waiting for protocol converter %s to be active...", a.name),
			a.outboundChannel, models.EditProtocolConverter)

		errCode, err := a.waitForComponentToBeActive(oldConfig)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to wait for protocol converter to be active: %v", err)
			SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, errCode, nil, a.outboundChannel, models.EditProtocolConverter, nil)
			return nil, nil, fmt.Errorf("%s", errorMsg)
		}

		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			"Protocol converter successfully updated and activated", a.outboundChannel, models.EditProtocolConverter)
	}

	return "Successfully updated protocol converter", nil, nil
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

// waitForComponentToBeActive polls live FSM state until the protocol converter
// becomes active or the timeout hits. Unlike deploy operations, this method
// does not remove the component on timeout since it's an edit operation.
// The function returns the error code and the error message via an error object.
// The error code is a string that is sent to the frontend to allow it to determine if the action can be retried or not.
// The error message is sent to the frontend to allow the user to see the error message.
func (a *EditProtocolConverterAction) waitForComponentToBeActive(oldConfig config.ProtocolConverterConfig) (string, error) {
	ticker := time.NewTicker(constants.ActionTickerTime)
	defer ticker.Stop()
	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout)
	startTime := time.Now()
	timeoutDuration := constants.DataflowComponentWaitForActiveTimeout

	for {
		elapsed := time.Since(startTime)
		remaining := timeoutDuration - elapsed
		remainingSeconds := int(remaining.Seconds())

		select {
		case <-timeout:

			// rollback to previous configuration
			ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
			defer cancel()
			_, err := a.configManager.AtomicEditProtocolConverter(ctx, a.protocolConverterUUID, oldConfig)
			if err != nil {
				a.actionLogger.Errorf("Failed to rollback to previous configuration: %v", err)
				stateMessage := fmt.Sprintf("Protocol converter '%s' edit timeout reached. It did not become active in time. Rolling back to previous configuration failed: %v", a.name, err)
				return models.ErrRetryRollbackTimeout, fmt.Errorf("%s", stateMessage)
			} else {
				stateMessage := fmt.Sprintf("Protocol converter '%s' edit timeout reached. It did not become active in time. Rolled back to previous configuration", a.name)
				return models.ErrRetryRollbackTimeout, fmt.Errorf("%s", stateMessage)
			}

		case <-ticker.C:
			// Get a deep copy of the system snapshot to prevent race conditions
			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()
			if protocolConverterManager, exists := systemSnapshot.Managers[constants.ProtocolConverterManagerName]; exists {
				instances := protocolConverterManager.GetInstances()
				found := false
				for _, instance := range instances {
					curName := instance.ID
					if curName != a.name {
						continue
					}

					// Cast the instance LastObservedState to a protocolconverter instance
					pcSnapshot, ok := instance.LastObservedState.(*protocolconverter.ProtocolConverterObservedStateSnapshot)
					if !ok {
						stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for state info"
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
							stateMessage, a.outboundChannel, models.EditProtocolConverter)
						continue
					}

					found = true

					// Verify that the protocol converter has applied the desired DFC configuration.
					// We compare the desired DFC config with the observed DFC configuration
					// in the protocol converter snapshot.
					if !a.compareProtocolConverterDFCConfig(pcSnapshot) {
						stateMessage := RemainingPrefixSec(remainingSeconds) + fmt.Sprintf("%s DFC config not yet applied. Status reason: %s", a.dfcType, pcSnapshot.ServiceInfo.StatusReason)
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
							stateMessage, a.outboundChannel, models.EditProtocolConverter)
						continue
					}

					// Check if the protocol converter is in an active state
					if instance.CurrentState == "active" {
						stateMessage := RemainingPrefixSec(remainingSeconds) + fmt.Sprintf("protocol converter successfully activated with state '%s', %s DFC configuration verified", instance.CurrentState, a.dfcType)
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage,
							a.outboundChannel, models.EditProtocolConverter)
						return "", nil
					}

					// Get the current state reason for more detailed information
					currentStateReason := fmt.Sprintf("current state: %s", instance.CurrentState)
					if pcSnapshot != nil && pcSnapshot.ServiceInfo.StatusReason != "" {
						currentStateReason = pcSnapshot.ServiceInfo.StatusReason
					}

					stateMessage := RemainingPrefixSec(remainingSeconds) + currentStateReason
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.EditProtocolConverter)
				}

				if !found {
					stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for protocol converter to appear in the system"
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.EditProtocolConverter)
				}

			} else {
				stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for protocol converter manager to initialise"
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					stateMessage, a.outboundChannel, models.EditProtocolConverter)
			}
		}
	}
}

// compareProtocolConverterDFCConfig compares the desired DFC configuration with the observed
// DFC configuration in the protocol converter snapshot. Currently only checks the read DFC
// since write DFC is not yet implemented.
func (a *EditProtocolConverterAction) compareProtocolConverterDFCConfig(pcSnapshot *protocolconverter.ProtocolConverterObservedStateSnapshot) bool {
	if pcSnapshot == nil {
		return false
	}

	// Only check read DFC for now since write DFC is not yet implemented
	if a.dfcType != "read" {
		// For write DFC, just return true for now since it's not implemented
		return true
	}

	// Check if the observed Benthos config is available for read DFC
	if pcSnapshot.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ObservedBenthosServiceConfig.Input == nil {
		return false
	}

	// Convert observed Benthos config to DFC config format for comparison
	observedBenthosConfig := pcSnapshot.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ObservedBenthosServiceConfig
	observedDFCConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
			Input:              observedBenthosConfig.Input,
			Pipeline:           observedBenthosConfig.Pipeline,
			Output:             observedBenthosConfig.Output,
			CacheResources:     observedBenthosConfig.CacheResources,
			RateLimitResources: observedBenthosConfig.RateLimitResources,
			Buffer:             observedBenthosConfig.Buffer,
		},
	}

	// render the desired DFC config template
	// We need to render the template with the actual protocol converter variables
	// to compare it properly with the observed config
	renderedDesiredConfig, err := a.renderDesiredDFCConfig(pcSnapshot)
	if err != nil {
		a.actionLogger.Errorf("failed to render desired DFC config: %v", err)
		return false
	}

	// do not compare the output since it will be automatically generated
	observedDFCConfig.BenthosConfig.Output = nil
	renderedDesiredConfig.BenthosConfig.Output = nil

	// log the observed and desired DFC configs
	a.actionLogger.Debugf("observed DFC config: %+v", observedDFCConfig)
	a.actionLogger.Debugf("rendered desired DFC config: %+v", renderedDesiredConfig)

	// Compare with the rendered desired DFC config
	return dataflowcomponentserviceconfig.NewComparator().ConfigsEqual(observedDFCConfig, renderedDesiredConfig)
}

// renderDesiredDFCConfig renders the template variables in the desired DFC config
// using the actual runtime values from the protocol converter observed state
func (a *EditProtocolConverterAction) renderDesiredDFCConfig(pcSnapshot *protocolconverter.ProtocolConverterObservedStateSnapshot) (dataflowcomponentserviceconfig.DataflowComponentServiceConfig, error) {
	// Get the observed spec config to extract the variables
	specConfig := pcSnapshot.ObservedProtocolConverterSpecConfig

	// Build the variable scope from the spec config's variables
	// This includes user, global, and internal variables that were used to render the observed config
	variableScope := specConfig.Variables.Flatten()

	// Render the desired DFC config using the same variables that were used for the observed config
	renderedConfig, err := config.RenderTemplate(a.desiredDFCConfig, variableScope)
	if err != nil {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("failed to render desired DFC config: %w", err)
	}

	return renderedConfig, nil
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
