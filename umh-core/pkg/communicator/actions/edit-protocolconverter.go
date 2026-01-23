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
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter/runtime_config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"go.uber.org/zap"
)

// DFCType represents the type of dataflow component configuration.
type DFCType string

const (
	// DFCTypeRead represents a read dataflow component.
	DFCTypeRead DFCType = "read"
	// DFCTypeWrite represents a write dataflow component.
	DFCTypeWrite DFCType = "write"
	// DFCTypeEmpty represents no dataflow component (connection/location update only).
	DFCTypeEmpty DFCType = "empty"
)

// String returns the string representation of the DFCType.
func (d DFCType) String() string {
	return string(d)
}

// IsValid checks if the DFCType has a valid value.
func (d DFCType) IsValid() bool {
	switch d {
	case DFCTypeRead, DFCTypeWrite, DFCTypeEmpty:
		return true
	default:
		return false
	}
}

// EditProtocolConverterAction implements the Action interface for editing
// protocol converter configurations, particularly for adding DFC configurations.
type EditProtocolConverterAction struct {
	configManager config.ConfigManager

	outboundChannel chan *models.UMHMessage
	location        map[int]string

	// Runtime observation for health checks
	systemSnapshotManager *fsm.SnapshotManager

	actionLogger   *zap.SugaredLogger
	userEmail      string
	name           string // protocol converter name (optional for updates)
	dfcType        DFCType
	connectionPort string
	connectionIP   string

	dfcPayload models.CDFCPayload

	vb []models.ProtocolConverterVariable

	// Desired DFC config for comparison during health checks
	desiredDFCConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig

	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	// Parsed request payload (only populated after Parse)
	protocolConverterUUID uuid.UUID

	// Atomic edit UUID used for configuration updates and rollbacks
	atomicEditUUID uuid.UUID

	ignoreHealthCheck bool
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
		return fmt.Errorf("failed to parse protocol converter payload: %w", err)
	}

	// Extract UUID
	if pcPayload.UUID == nil {
		return errors.New("missing required field UUID")
	}

	a.protocolConverterUUID = *pcPayload.UUID
	a.name = pcPayload.Name

	// Determine which DFC is being updated and convert it to CDFCPayload
	var dfcToUpdate *models.ProtocolConverterDFC

	switch {
	case pcPayload.ReadDFC != nil:
		a.dfcType = DFCTypeRead
		dfcToUpdate = pcPayload.ReadDFC
	case pcPayload.WriteDFC != nil:
		a.dfcType = DFCTypeWrite
		dfcToUpdate = pcPayload.WriteDFC
	default:
		a.dfcType = DFCTypeEmpty
		dfcToUpdate = &models.ProtocolConverterDFC{}
	}

	if pcPayload.TemplateInfo != nil {
		a.vb = pcPayload.TemplateInfo.Variables
	} else {
		a.vb = make([]models.ProtocolConverterVariable, 0)
	}

	// Convert map[string]any to []ProtocolConverterVariable for consistency
	if pcPayload.Variables != nil {
		for key, value := range pcPayload.Variables {
			a.vb = append(a.vb, models.ProtocolConverterVariable{
				Label: key,
				Value: value,
			})
		}
	}

	// Extract location
	if pcPayload.Location != nil {
		a.location = pcPayload.Location
	}

	a.connectionPort = strconv.Itoa(int(pcPayload.Connection.Port))
	a.connectionIP = pcPayload.Connection.IP

	// Convert ProtocolConverterDFC to CDFCPayload for internal processing
	if a.dfcType != DFCTypeEmpty {
		a.dfcPayload = models.CDFCPayload{
			Inputs:   models.DfcDataConfig{Data: dfcToUpdate.Inputs.Data, Type: dfcToUpdate.Inputs.Type},
			Pipeline: convertPipelineToMap(dfcToUpdate.Pipeline),
			// Set default outputs since ProtocolConverterDFC doesn't have outputs
			Outputs: models.DfcDataConfig{Data: "", Type: ""},
			// Extract Inject data from RawYAML to support CacheResources, RateLimitResources, and Buffer
			Inject: extractInjectFromRawYAML(dfcToUpdate.RawYAML),
		}
		// Handle optional fields
		if dfcToUpdate.IgnoreErrors != nil {
			a.ignoreHealthCheck = *dfcToUpdate.IgnoreErrors
		}
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

	if err := config.ValidateComponentName(a.name); err != nil {
		return err
	}

	if a.dfcType != DFCTypeEmpty {
		if err := ValidateCustomDataFlowComponentPayload(a.dfcPayload, false); err != nil {
			return fmt.Errorf("invalid dataflow component configuration: %w", err)
		}
	}

	return nil
}

// Execute implements the Action interface by updating the protocol converter configuration
// with the provided dataflow component configuration.
func (a *EditProtocolConverterAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing EditProtocolConverter action")

	// Send confirmation that action is starting
	var confirmationMessage string
	if a.dfcType == DFCTypeEmpty {
		confirmationMessage = fmt.Sprintf("Starting edit of protocol converter %s to update connection and location", a.protocolConverterUUID)
	} else {
		confirmationMessage = fmt.Sprintf("Starting edit of protocol converter %s to add %s DFC", a.protocolConverterUUID, a.dfcType.String())
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		confirmationMessage, a.outboundChannel, models.EditProtocolConverter)

	var (
		benthosConfig dataflowcomponentserviceconfig.BenthosConfig
		err           error
	)

	// Only create Benthos config if we have a DFC to configure

	if a.dfcType != DFCTypeEmpty {
		// Convert the DFC payload to BenthosConfig
		benthosConfig, err = CreateBenthosConfigFromCDFCPayload(a.dfcPayload, a.name)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to create Benthos configuration: %v", err)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, a.outboundChannel, models.EditProtocolConverter)

			return nil, nil, fmt.Errorf("%s", errorMsg)
		}

		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			fmt.Sprintf("Updating protocol converter configuration with %s DFC...", a.dfcType.String()),
			a.outboundChannel, models.EditProtocolConverter)
	} else {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			"Updating protocol converter configuration (connection and location only)...",
			a.outboundChannel, models.EditProtocolConverter)
	}

	// Apply mutations to create new spec
	newSpec, atomicEditUUID, err := a.applyMutation(benthosConfig)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to apply configuration mutation: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Store the atomic edit UUID for use in rollback operations
	a.atomicEditUUID = atomicEditUUID

	// Persist the configuration changes
	oldConfig, err := a.persistConfig(atomicEditUUID, newSpec)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to persist configuration changes: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Await rollout and perform health checks
	if a.systemSnapshotManager != nil && !a.ignoreHealthCheck {
		errCode, err := a.awaitRollout(oldConfig)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed during rollout: %v", err)
			SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, errCode, nil, a.outboundChannel, models.EditProtocolConverter, nil)

			return nil, nil, fmt.Errorf("%s", errorMsg)
		}

		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			"Protocol converter successfully updated", a.outboundChannel, models.EditProtocolConverter)
	}

	newUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(a.name)
	response := map[string]any{
		"uuid": newUUID,
	}

	return response, nil, nil
}

// applyMutation analyzes the current configuration and applies the necessary mutations
// to create the new protocol converter specification. It handles child/root relationships,
// variable merging, and DFC configuration updates.
// Returns the new spec and the atomic edit UUID to use for persistence.
func (a *EditProtocolConverterAction) applyMutation(benthosConfig dataflowcomponentserviceconfig.BenthosConfig) (config.ProtocolConverterConfig, uuid.UUID, error) {
	// Get current configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	currentConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		return config.ProtocolConverterConfig{}, uuid.Nil, fmt.Errorf("failed to get current configuration: %w", err)
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
		return config.ProtocolConverterConfig{}, uuid.Nil, fmt.Errorf("protocol converter with UUID %s not found", a.protocolConverterUUID)
	}

	// Currently, we cannot reuse templates, so we need to create a new one
	targetPC.ProtocolConverterServiceConfig.TemplateRef = a.name
	targetPC.Name = a.name

	// Classify the protocol converter instance
	isChild := targetPC.ProtocolConverterServiceConfig.TemplateRef != "" &&
		targetPC.ProtocolConverterServiceConfig.TemplateRef != targetPC.Name

	// Determine which instance to modify and which UUID to use for atomic operation
	var (
		instanceToModify config.ProtocolConverterConfig
		atomicEditUUID   uuid.UUID
		newVB            map[string]any
	)

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
			return config.ProtocolConverterConfig{}, uuid.Nil, fmt.Errorf("root template %s not found for child protocol converter %s",
				targetPC.ProtocolConverterServiceConfig.TemplateRef, targetPC.Name)
		}

		// Apply mutations to the root, but keep child's variables
		instanceToModify = rootPC
		atomicEditUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(rootPC.Name)

		// Add the new variables and preserve existing child variables
		newVB = make(map[string]any)

		// First copy existing variables (provides defaults)
		if targetPC.ProtocolConverterServiceConfig.Variables.User != nil {
			maps.Copy(newVB, targetPC.ProtocolConverterServiceConfig.Variables.User)
		}

		// Then add new variables (overwrites with updated values)
		for _, variable := range a.vb {
			newVB[variable.Label] = variable.Value
		}
	} else {
		// Root or stand-alone: apply mutations directly
		instanceToModify = targetPC
		atomicEditUUID = a.protocolConverterUUID

		// Add the variables and keep the existing variables
		newVB = make(map[string]any)

		// First copy existing variables (provides defaults)
		if targetPC.ProtocolConverterServiceConfig.Variables.User != nil {
			maps.Copy(newVB, targetPC.ProtocolConverterServiceConfig.Variables.User)
		}

		// Then add new variables (overwrites with updated values)
		for _, variable := range a.vb {
			newVB[variable.Label] = variable.Value
		}
	}

	// As the BuildRuntimeConfig function always adds location and location_path to the user variables,
	// we need to remove them from the variables here to avoid that they end up in the config file
	delete(newVB, "location")
	delete(newVB, "location_path")

	instanceToModify.ProtocolConverterServiceConfig.Variables.User = newVB

	// Update the appropriate DFC configuration based on dfcType
	var dfcServiceConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig
	if a.dfcType != DFCTypeEmpty {
		dfcServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: benthosConfig,
		}
		// Store the desired DFC config for health check comparison
		a.desiredDFCConfig = dfcServiceConfig
	}

	// Add the connection details to the template
	instanceToModify.ProtocolConverterServiceConfig.Config.ConnectionServiceConfig = connectionserviceconfig.ConnectionServiceConfigTemplate{
		NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
			Target: "{{ .IP }}",
			Port:   "{{ .PORT }}",
		},
	}

	// Update the location of the protocol converter (convert the map to a string map)
	locationMap := make(map[string]string)
	for k, v := range a.location {
		locationMap[strconv.Itoa(k)] = v
	}

	instanceToModify.ProtocolConverterServiceConfig.Location = locationMap

	// Update the connection details of the protocol converter (IP and PORT variables)
	if instanceToModify.ProtocolConverterServiceConfig.Variables.User != nil {
		instanceToModify.ProtocolConverterServiceConfig.Variables.User["IP"] = a.connectionIP
		instanceToModify.ProtocolConverterServiceConfig.Variables.User["PORT"] = a.connectionPort
	}

	switch a.dfcType {
	case DFCTypeRead:
		instanceToModify.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dfcServiceConfig
	case DFCTypeWrite:
		instanceToModify.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dfcServiceConfig
	case DFCTypeEmpty:
		// For empty dfcType, we only update connection, location, and name - no DFC configuration
		// The connection, location, and name updates are already handled above
	default:
		return config.ProtocolConverterConfig{}, uuid.Nil, fmt.Errorf("invalid DFC type: %s", a.dfcType.String())
	}

	return instanceToModify, atomicEditUUID, nil
}

// persistConfig performs the atomic configuration update operation.
// Returns the old configuration for potential rollback operations.
func (a *EditProtocolConverterAction) persistConfig(atomicEditUUID uuid.UUID, newSpec config.ProtocolConverterConfig) (config.ProtocolConverterConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	oldConfig, err := a.configManager.AtomicEditProtocolConverter(ctx, atomicEditUUID, newSpec)
	if err != nil {
		return config.ProtocolConverterConfig{}, fmt.Errorf("failed to update protocol converter: %w", err)
	}

	// deep copy the old config therefore setup a full config
	// this may seem hacky but like that we can reuse the Clone() function
	// and we do not need to implement a custom Clone() function for the ProtocolConverterConfig
	fullConfig := config.FullConfig{
		ProtocolConverter: []config.ProtocolConverterConfig{oldConfig},
	}

	copiedConfig := fullConfig.Clone()
	oldConfig = copiedConfig.ProtocolConverter[0]
	// remove the location and location_path from the user variables
	// Check if User map exists before trying to delete from it
	if oldConfig.ProtocolConverterServiceConfig.Variables.User != nil {
		delete(oldConfig.ProtocolConverterServiceConfig.Variables.User, "location")
		delete(oldConfig.ProtocolConverterServiceConfig.Variables.User, "location_path")
	}

	return oldConfig, nil
}

// awaitRollout waits for the protocol converter to become active and performs health checks.
// Returns error code and error message for proper error handling in the caller.
func (a *EditProtocolConverterAction) awaitRollout(oldConfig config.ProtocolConverterConfig) (string, error) {
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		fmt.Sprintf("Waiting for protocol converter %s to be active...", a.name),
		a.outboundChannel, models.EditProtocolConverter)

	return a.waitForComponentToBeActive(oldConfig)
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

	var (
		logs     []s6.LogEntry
		lastLogs []s6.LogEntry
	)

	for {
		elapsed := time.Since(startTime)
		remaining := timeoutDuration - elapsed
		remainingSeconds := int(remaining.Seconds())

		select {
		case <-timeout:
			// rollback to previous configuration
			ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
			defer cancel()

			_, err := a.configManager.AtomicEditProtocolConverter(ctx, a.atomicEditUUID, oldConfig)
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
						stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for state info of protocol converter instance"
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
							stateMessage, a.outboundChannel, models.EditProtocolConverter)

						continue
					}

					found = true

					currentStateReason := "current state: " + instance.CurrentState

					if a.dfcType != DFCTypeEmpty {
						// Verify that the protocol converter has applied the desired DFC configuration.
						// We compare the desired DFC config with the observed DFC configuration
						// in the protocol converter snapshot.
						if !a.compareProtocolConverterDFCConfig(pcSnapshot) {
							stateMessage := RemainingPrefixSec(remainingSeconds) + fmt.Sprintf("%s DFC config not yet applied. State: %s, Status reason: %s", a.dfcType.String(), instance.CurrentState, pcSnapshot.ServiceInfo.StatusReason)
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
								stateMessage, a.outboundChannel, models.EditProtocolConverter)

							continue
						}

						// Check if the protocol converter is in an active state
						if instance.CurrentState == "active" || instance.CurrentState == "idle" {
							stateMessage := RemainingPrefixSec(remainingSeconds) + fmt.Sprintf("protocol converter successfully activated with state '%s', %s DFC configuration verified", instance.CurrentState, a.dfcType.String())
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage,
								a.outboundChannel, models.EditProtocolConverter)

							return "", nil
						}

						// Get the current state reason for more detailed information
						if pcSnapshot != nil && pcSnapshot.ServiceInfo.StatusReason != "" {
							currentStateReason = pcSnapshot.ServiceInfo.StatusReason
						}

						// send the benthos logs to the user
						logs = pcSnapshot.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs

						// only send the logs that have not been sent yet
						if len(logs) > len(lastLogs) {
							lastLogs = SendLimitedLogs(logs, lastLogs, a.instanceUUID, a.userEmail, a.actionUUID, a.outboundChannel, models.EditProtocolConverter, remainingSeconds)
						}

						// CheckBenthosLogLinesForConfigErrors is used to detect fatal configuration errors that would cause
						// Benthos to enter a CrashLoop. When such errors are detected, we can immediately
						// abort the startup process rather than waiting for the full timeout period,
						// as these errors require configuration changes to resolve.
						if CheckBenthosLogLinesForConfigErrors(logs) {
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, Label("edit", a.name)+"configuration error detected. Rolling back...", a.outboundChannel, models.EditProtocolConverter)

							ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
							defer cancel()

							a.actionLogger.Infof("rolling back to previous configuration with user variables: %v", oldConfig.ProtocolConverterServiceConfig.Variables.User)

							_, err := a.configManager.AtomicEditProtocolConverter(ctx, a.atomicEditUUID, oldConfig)
							if err != nil {
								a.actionLogger.Errorf("failed to roll back protocol converter %s: %v", a.name, err)

								return models.ErrConfigFileInvalid, fmt.Errorf("protocol converter '%s' has invalid configuration but could not be rolled back: %w. Please check your logs and consider manually restoring the previous configuration", a.name, err)
							}

							return models.ErrConfigFileInvalid, fmt.Errorf("protocol converter '%s' was rolled back to its previous configuration due to configuration errors. Please check the component logs, fix the configuration issues, and try editing again", a.name)
						}
					} else {
						if strconv.FormatUint(uint64(pcSnapshot.ServiceInfo.ConnectionObservedState.ServiceInfo.NmapObservedState.ObservedNmapServiceConfig.Port), 10) != a.connectionPort {
							currentStateReason = "waiting for nmap to connect to port " + a.connectionPort
						} else {
							return "", nil
						}
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
	if a.dfcType != DFCTypeRead {
		// For write DFC and empty DFC, just return true for now since write DFC is not implemented
		// and empty DFC doesn't have any DFC configuration to compare
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
// using the actual runtime values from the protocol converter observed state.
func (a *EditProtocolConverterAction) renderDesiredDFCConfig(pcSnapshot *protocolconverter.ProtocolConverterObservedStateSnapshot) (dataflowcomponentserviceconfig.DataflowComponentServiceConfig, error) {
	// Get the observed spec config
	specConfig := pcSnapshot.ObservedProtocolConverterSpecConfig

	// Create a deep copy to avoid mutating the original observed state
	var modifiedSpec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec

	err := deepcopy.Copy(&modifiedSpec, &specConfig)
	if err != nil {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("failed to deep copy spec config: %w", err)
	}

	// Now safely modify the copy without affecting the original
	switch a.dfcType {
	case DFCTypeRead:
		modifiedSpec.Config.DataflowComponentReadServiceConfig = a.desiredDFCConfig
	case DFCTypeWrite:
		modifiedSpec.Config.DataflowComponentWriteServiceConfig = a.desiredDFCConfig
	default:
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("invalid DFC type: %s", a.dfcType.String())
	}

	systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()

	agentLocation := convertIntMapToStringMap(systemSnapshot.CurrentConfig.Agent.Location)

	pcName := a.name

	runtimeConfig, err := runtime_config.BuildRuntimeConfig(
		modifiedSpec,
		agentLocation,
		nil,             // TODO: add global vars
		"unimplemented", // TODO: add node name
		pcName,
	)
	if err != nil {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("failed to build runtime config: %w", err)
	}

	// Return the appropriate DFC config
	switch a.dfcType {
	case DFCTypeRead:
		return runtimeConfig.DataflowComponentReadServiceConfig, nil
	case DFCTypeWrite:
		return runtimeConfig.DataflowComponentWriteServiceConfig, nil
	default:
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("invalid DFC type: %s", a.dfcType.String())
	}
}

// convertPipelineToMap converts CommonDataFlowComponentPipelineConfig to map[string]DfcDataConfig.
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

// extractInjectFromRawYAML extracts the inject data from the RawYAML field to support
// CacheResources, RateLimitResources, and Buffer in protocol converter DFCs.
func extractInjectFromRawYAML(rawYAML *models.CommonDataFlowComponentRawYamlConfig) string {
	if rawYAML == nil {
		return ""
	}

	return rawYAML.Data
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
	return a.dfcType.String()
}
