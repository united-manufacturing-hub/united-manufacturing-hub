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
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
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
	// DFCTypeBoth represents both read and write dataflow components being updated simultaneously.
	DFCTypeBoth DFCType = "both"
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
	case DFCTypeRead, DFCTypeWrite, DFCTypeBoth, DFCTypeEmpty:
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
	readDFCState   string // desired state for the read DFC ("active" or "stopped"; empty = default active)
	writeDFCState  string // desired state for the write DFC ("active" or "stopped"; empty = default active)

	readDFCPayload  *models.CDFCPayload
	writeDFCPayload *models.CDFCPayload

	vb []models.ProtocolConverterVariable

	// Desired DFC configs for comparison during health checks
	desiredReadDFCConfig  dataflowcomponentserviceconfig.DataflowComponentServiceConfig
	desiredWriteDFCConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig

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

	// Parse each DFC side independently.
	if pcPayload.ReadDFC != nil {
		readPayload := dfcToPayload(pcPayload.ReadDFC)
		a.readDFCPayload = &readPayload
		a.readDFCState = pcPayload.ReadDFC.State
		if pcPayload.ReadDFC.IgnoreErrors != nil {
			a.ignoreHealthCheck = *pcPayload.ReadDFC.IgnoreErrors
		}
	}
	if pcPayload.WriteDFC != nil {
		writePayload := dfcToPayload(pcPayload.WriteDFC)
		a.writeDFCPayload = &writePayload
		a.writeDFCState = pcPayload.WriteDFC.State
		if pcPayload.WriteDFC.IgnoreErrors != nil {
			a.ignoreHealthCheck = a.ignoreHealthCheck || *pcPayload.WriteDFC.IgnoreErrors
		}
	}

	// Determine dfcType by comparing incoming DFC configs against what is
	// currently deployed. Only DFCs that actually differ need to be stopped
	// and restarted.
	a.dfcType = a.deriveDFCType()

	if pcPayload.TemplateInfo != nil {
		a.vb = pcPayload.TemplateInfo.Variables
	} else {
		a.vb = make([]models.ProtocolConverterVariable, 0)
	}

	// Extract location
	if pcPayload.Location != nil {
		a.location = pcPayload.Location
	}

	a.connectionPort = strconv.Itoa(int(pcPayload.Connection.Port))
	a.connectionIP = pcPayload.Connection.IP

	a.actionLogger.Debugf("Parsed EditProtocolConverter action payload: uuid=%s, name=%s, dfcType=%s, readDFCState=%s, writeDFCState=%s",
		a.protocolConverterUUID, a.name, a.dfcType, a.readDFCState, a.writeDFCState)

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

	if err := validateDFCPayloadAndState(a.readDFCPayload, a.readDFCState, "read"); err != nil {
		return err
	}
	if err := validateDFCPayloadAndState(a.writeDFCPayload, a.writeDFCState, "write"); err != nil {
		return err
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
		readBenthosConfig  *dataflowcomponentserviceconfig.BenthosConfig
		writeBenthosConfig *dataflowcomponentserviceconfig.BenthosConfig
		err                error
	)

	if a.readDFCPayload != nil {
		cfg, buildErr := CreateBenthosConfigFromCDFCPayload(*a.readDFCPayload, a.name)
		if buildErr != nil {
			errorMsg := fmt.Sprintf("Failed to create read DFC Benthos configuration: %v", buildErr)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, a.outboundChannel, models.EditProtocolConverter)

			return nil, nil, fmt.Errorf("%s", errorMsg)
		}
		readBenthosConfig = &cfg
	}

	if a.writeDFCPayload != nil {
		cfg, buildErr := CreateBenthosConfigFromCDFCPayload(*a.writeDFCPayload, a.name)
		if buildErr != nil {
			errorMsg := fmt.Sprintf("Failed to create write DFC Benthos configuration: %v", buildErr)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, a.outboundChannel, models.EditProtocolConverter)

			return nil, nil, fmt.Errorf("%s", errorMsg)
		}
		writeBenthosConfig = &cfg
	}

	if a.dfcType != DFCTypeEmpty {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			fmt.Sprintf("Updating protocol converter configuration with %s DFC...", a.dfcType.String()),
			a.outboundChannel, models.EditProtocolConverter)
	} else {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			"Updating protocol converter configuration (connection and location only)...",
			a.outboundChannel, models.EditProtocolConverter)
	}

	// Apply mutations to create new spec
	newSpec, atomicEditUUID, desiredPCState, err := a.applyMutation(readBenthosConfig, writeBenthosConfig)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to apply configuration mutation: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Store the atomic edit UUID for use in rollback operations
	a.atomicEditUUID = atomicEditUUID

	var oldConfig config.ProtocolConverterConfig

	// For DFC modifications, stop the affected DFCs first to ensure a clean redeployment.
	// This prevents stale errors from a previously degraded DFC from blocking the new deployment.
	if a.dfcType != DFCTypeEmpty && a.systemSnapshotManager != nil && !a.ignoreHealthCheck {
		var stopErr error
		oldConfig, stopErr = a.stopAndAwaitDFCs(atomicEditUUID, newSpec)
		if stopErr != nil {
			errorMsg := fmt.Sprintf("Failed to stop DFCs before edit: %v", stopErr)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, a.outboundChannel, models.EditProtocolConverter)

			return nil, nil, fmt.Errorf("%s", errorMsg)
		}

		// Persist the actual new config (DFCs will start fresh from stopped state)
		persistCtx, persistCancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
		defer persistCancel()

		_, err = a.configManager.AtomicEditProtocolConverter(persistCtx, atomicEditUUID, newSpec)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to persist new configuration: %v", err)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, a.outboundChannel, models.EditProtocolConverter)

			return nil, nil, fmt.Errorf("%s", errorMsg)
		}
	} else {
		// No DFC changes or no health check — just persist directly
		oldConfig, err = a.persistConfig(atomicEditUUID, newSpec)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to persist configuration changes: %v", err)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, a.outboundChannel, models.EditProtocolConverter)

			return nil, nil, fmt.Errorf("%s", errorMsg)
		}
	}

	// Await rollout and perform health checks
	if a.systemSnapshotManager != nil && !a.ignoreHealthCheck {
		errCode, err := a.awaitRollout(oldConfig, desiredPCState)
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
// Returns the new spec, the atomic edit UUID, the derived PC desired state, and any error.
func (a *EditProtocolConverterAction) applyMutation(readBenthosConfig, writeBenthosConfig *dataflowcomponentserviceconfig.BenthosConfig) (config.ProtocolConverterConfig, uuid.UUID, string, error) {
	// Get current configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	currentConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		return config.ProtocolConverterConfig{}, uuid.Nil, "", fmt.Errorf("failed to get current configuration: %w", err)
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
		return config.ProtocolConverterConfig{}, uuid.Nil, "", fmt.Errorf("protocol converter with UUID %s not found", a.protocolConverterUUID)
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
			return config.ProtocolConverterConfig{}, uuid.Nil, "", fmt.Errorf("root template %s not found for child protocol converter %s",
				targetPC.ProtocolConverterServiceConfig.TemplateRef, targetPC.Name)
		}

		// Apply mutations to the root, but keep child's variables
		instanceToModify = rootPC
		atomicEditUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(rootPC.Name)
	} else {
		// Root or stand-alone: apply mutations directly
		instanceToModify = targetPC
		atomicEditUUID = a.protocolConverterUUID
	}

	// Add the new variables and preserve existing variables
	newVB = make(map[string]any)

	// First copy existing variables (provides defaults)
	if targetPC.ProtocolConverterServiceConfig.Variables.User != nil {
		maps.Copy(newVB, targetPC.ProtocolConverterServiceConfig.Variables.User)
	}

	// Then add new variables (overwrites with updated values)
	for _, variable := range a.vb {
		newVB[variable.Label] = variable.Value
	}

	// As the BuildRuntimeConfig function always adds location and location_path to the user variables,
	// we need to remove them from the variables here to avoid that they end up in the config file
	delete(newVB, "location")
	delete(newVB, "location_path")

	instanceToModify.ProtocolConverterServiceConfig.Variables.User = newVB

	// Apply read DFC config if provided
	if readBenthosConfig != nil {
		readDFCServiceConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: *readBenthosConfig,
		}
		a.desiredReadDFCConfig = readDFCServiceConfig
		instanceToModify.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = readDFCServiceConfig
	}

	// Apply write DFC config if provided
	if writeBenthosConfig != nil {
		writeDFCServiceConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: *writeBenthosConfig,
		}
		a.desiredWriteDFCConfig = writeDFCServiceConfig
		instanceToModify.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = writeDFCServiceConfig
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

	// Only update the per-DFC desired states if the user provided new values.
	if a.readDFCState != "" {
		instanceToModify.ProtocolConverterServiceConfig.ReadDFCDesiredState = a.readDFCState
	}
	if a.writeDFCState != "" {
		instanceToModify.ProtocolConverterServiceConfig.WriteDFCDesiredState = a.writeDFCState
	}

	// The PC is always active so that the connection monitor stays alive.
	// Individual DFC states are tracked separately; the bridge (and its
	// connection monitor) is only torn down when the bridge itself is removed.
	instanceToModify.DesiredFSMState = protocolconverter.OperationalStateActive

	return instanceToModify, atomicEditUUID, instanceToModify.DesiredFSMState, nil
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

// stopAndAwaitDFCs stops the affected DFCs before applying a new configuration.
// This ensures a clean redeployment: any previously degraded DFC is stopped first,
// then started fresh with the new config, avoiding stale errors blocking progress.
// Returns the original (pre-edit) config for rollback purposes.
func (a *EditProtocolConverterAction) stopAndAwaitDFCs(atomicEditUUID uuid.UUID, newSpec config.ProtocolConverterConfig) (config.ProtocolConverterConfig, error) {
	// Build a stop variant: same new config but affected DFCs set to stopped.
	stopSpec := newSpec
	switch a.dfcType {
	case DFCTypeRead:
		stopSpec.ProtocolConverterServiceConfig.ReadDFCDesiredState = dataflowcomponent.OperationalStateStopped
	case DFCTypeWrite:
		stopSpec.ProtocolConverterServiceConfig.WriteDFCDesiredState = dataflowcomponent.OperationalStateStopped
	case DFCTypeBoth:
		stopSpec.ProtocolConverterServiceConfig.ReadDFCDesiredState = dataflowcomponent.OperationalStateStopped
		stopSpec.ProtocolConverterServiceConfig.WriteDFCDesiredState = dataflowcomponent.OperationalStateStopped
	}

	oldConfig, err := a.persistConfig(atomicEditUUID, stopSpec)
	if err != nil {
		return config.ProtocolConverterConfig{}, fmt.Errorf("failed to persist stop config: %w", err)
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Stopping affected data flow components for clean redeployment...",
		a.outboundChannel, models.EditProtocolConverter)

	ticker := time.NewTicker(constants.ActionTickerTime)
	defer ticker.Stop()

	// Use half the normal timeout for the stop phase, leaving enough time for the start phase.
	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout / 2)

	for {
		select {
		case <-timeout:
			a.actionLogger.Warnf("timeout waiting for DFCs to stop, continuing with deployment")

			return oldConfig, nil
		case <-ticker.C:
			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()

			pcManager, exists := systemSnapshot.Managers[constants.ProtocolConverterManagerName]
			if !exists {
				continue
			}

			for _, instance := range pcManager.GetInstances() {
				if instance.ID != a.name {
					continue
				}

				pcSnapshot, ok := instance.LastObservedState.(*protocolconverter.ProtocolConverterObservedStateSnapshot)
				if !ok {
					continue
				}

				readStopped := true
				writeStopped := true

				if a.dfcType == DFCTypeRead || a.dfcType == DFCTypeBoth {
					readStopped = pcSnapshot.ServiceInfo.DataflowComponentReadFSMState == dataflowcomponent.OperationalStateStopped
				}

				if a.dfcType == DFCTypeWrite || a.dfcType == DFCTypeBoth {
					writeStopped = pcSnapshot.ServiceInfo.DataflowComponentWriteFSMState == dataflowcomponent.OperationalStateStopped
				}

				if readStopped && writeStopped {
					a.actionLogger.Infof("DFCs stopped successfully, proceeding with new config")

					return oldConfig, nil
				}

				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					fmt.Sprintf("Waiting for DFCs to stop (read: %s, write: %s)...",
						pcSnapshot.ServiceInfo.DataflowComponentReadFSMState,
						pcSnapshot.ServiceInfo.DataflowComponentWriteFSMState),
					a.outboundChannel, models.EditProtocolConverter)
			}
		}
	}
}

// awaitRollout waits for the protocol converter to reach the desired state and performs health checks.
// Returns error code and error message for proper error handling in the caller.
//
// It polls live FSM state until the protocol converter reaches the desired state or the timeout hits.
// Unlike deploy operations, this method does not remove the component on timeout since it's an edit operation.
// The function returns the error code and the error message via an error object.
// The error code is a string that is sent to the frontend to allow it to determine if the action can be retried or not.
// The error message is sent to the frontend to allow the user to see the error message.
func (a *EditProtocolConverterAction) awaitRollout(pcConfig config.ProtocolConverterConfig, desiredPCState string) (string, error) {
	SendActionReply(
		a.instanceUUID,
		a.userEmail,
		a.actionUUID,
		models.ActionExecuting,
		fmt.Sprintf(
			"Waiting for protocol converter %s to be %s...",
			a.name,
			desiredPCState,
		),
		a.outboundChannel,
		models.EditProtocolConverter,
	)

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

			_, err := a.configManager.AtomicEditProtocolConverter(ctx, a.atomicEditUUID, pcConfig)
			if err != nil {
				a.actionLogger.Errorf("Failed to rollback to previous configuration: %v", err)
				stateMessage := fmt.Sprintf("Protocol converter '%s' edit timeout reached. It did not become %s in time. Rolling back to previous configuration failed: %v", a.name, desiredPCState, err)

				return models.ErrRetryRollbackTimeout, fmt.Errorf("%s", stateMessage)
			} else {
				stateMessage := fmt.Sprintf("Protocol converter '%s' edit timeout reached. It did not become %s in time. Rolled back to previous configuration", a.name, desiredPCState)

				return models.ErrRetryRollbackTimeout, fmt.Errorf("%s", stateMessage)
			}

		case <-ticker.C:
			// Get a deep copy of the system snapshot to prevent race conditions
			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()

			protocolConverterManager, exists := systemSnapshot.Managers[constants.ProtocolConverterManagerName]
			if !exists {
				SendActionReply(
					a.instanceUUID,
					a.userEmail,
					a.actionUUID,
					models.ActionExecuting,
					RemainingPrefixSec(remainingSeconds)+"waiting for protocol converter manager to initialise",
					a.outboundChannel,
					models.EditProtocolConverter,
				)

				continue
			}

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
					SendActionReply(
						a.instanceUUID,
						a.userEmail,
						a.actionUUID,
						models.ActionExecuting,
						RemainingPrefixSec(remainingSeconds)+"waiting for state info of protocol converter instance",
						a.outboundChannel,
						models.EditProtocolConverter,
					)

					continue
				}

				found = true
				currentStateReason := "current state: " + instance.CurrentState

				if a.dfcType == DFCTypeEmpty {
					// For empty DFC type (connection/location/state update only)
					// Only check the nmap port when activating; when stopping, nmap is also
					// stopped so it will never update to the new port.
					if desiredPCState != protocolconverter.OperationalStateStopped {
						nmapPort := strconv.FormatUint(
							uint64(pcSnapshot.ServiceInfo.ConnectionObservedState.ServiceInfo.NmapObservedState.ObservedNmapServiceConfig.Port),
							10,
						)

						if nmapPort != a.connectionPort {
							currentStateReason = "waiting for nmap to connect to port " + a.connectionPort
							SendActionReply(
								a.instanceUUID,
								a.userEmail,
								a.actionUUID,
								models.ActionExecuting,
								RemainingPrefixSec(remainingSeconds)+currentStateReason,
								a.outboundChannel,
								models.EditProtocolConverter,
							)

							continue
						}
					}

					// Check if the protocol converter has reached the desired state
					hasReachedDesiredState := false

					switch desiredPCState {
					case protocolconverter.OperationalStateActive:

						hasReachedDesiredState = slices.Contains(
							[]string{
								protocolconverter.OperationalStateActive,
								protocolconverter.OperationalStateIdle,
								protocolconverter.OperationalStateStartingFailedDFCMissing,
							},
							instance.CurrentState,
						)
					case protocolconverter.OperationalStateStopped:
						hasReachedDesiredState = instance.CurrentState == protocolconverter.OperationalStateStopped
					}

					if !hasReachedDesiredState {
						currentStateReason = fmt.Sprintf(
							"waiting for state to become %s (current: %s)",
							desiredPCState,
							instance.CurrentState,
						)
						SendActionReply(
							a.instanceUUID,
							a.userEmail,
							a.actionUUID,
							models.ActionExecuting,
							RemainingPrefixSec(remainingSeconds)+currentStateReason,
							a.outboundChannel,
							models.EditProtocolConverter,
						)

						continue
					}

					return "", nil
				}

				// When desired state is "stopped", the Benthos process is not running so
				// compareProtocolConverterDFCConfig always returns false (observed Input is nil).
				// Check the state first to avoid an infinite loop in this case.
				if desiredPCState == protocolconverter.OperationalStateStopped {
					if instance.CurrentState == protocolconverter.OperationalStateStopped {
						SendActionReply(
							a.instanceUUID,
							a.userEmail,
							a.actionUUID,
							models.ActionExecuting,
							RemainingPrefixSec(remainingSeconds)+"protocol converter successfully stopped",
							a.outboundChannel,
							models.EditProtocolConverter,
						)

						return "", nil
					}

					SendActionReply(
						a.instanceUUID,
						a.userEmail,
						a.actionUUID,
						models.ActionExecuting,
						RemainingPrefixSec(remainingSeconds)+fmt.Sprintf(
							"waiting for state to become stopped (current: %s)",
							instance.CurrentState,
						),
						a.outboundChannel,
						models.EditProtocolConverter,
					)

					continue
				}

				// Verify that the protocol converter has applied the desired DFC configuration.
				// We compare the desired DFC config with the observed DFC configuration
				// in the protocol converter snapshot.
				if !a.compareProtocolConverterDFCConfig(pcSnapshot) {
					SendActionReply(
						a.instanceUUID,
						a.userEmail,
						a.actionUUID,
						models.ActionExecuting,
						RemainingPrefixSec(remainingSeconds)+fmt.Sprintf(
							"%s DFC config not yet applied. State: %s, Status reason: %s",
							a.dfcType.String(),
							instance.CurrentState,
							pcSnapshot.ServiceInfo.StatusReason,
						),
						a.outboundChannel,
						models.EditProtocolConverter,
					)

					continue
				}

				// Check if the protocol converter has reached the desired state
				// For "active" state: accept "active" or "idle"
				// For "stopped" state: accept only "stopped"
				hasReachedDesiredState := false

				a.actionLogger.Errorf("desiredPCState empty: %s, currentState empty: %s", desiredPCState, instance.CurrentState)
				switch desiredPCState {
				case protocolconverter.OperationalStateActive:

					hasReachedDesiredState = slices.Contains(
						[]string{
							protocolconverter.OperationalStateActive,
							protocolconverter.OperationalStateIdle,
						},
						instance.CurrentState,
					)
				case protocolconverter.OperationalStateStopped:
					hasReachedDesiredState = instance.CurrentState == protocolconverter.OperationalStateStopped
				}

				if hasReachedDesiredState {
					terminal := map[string]string{
						protocolconverter.OperationalStateActive:  "activated",
						protocolconverter.OperationalStateStopped: "stopped",
					}[desiredPCState]
					SendActionReply(
						a.instanceUUID,
						a.userEmail,
						a.actionUUID,
						models.ActionExecuting,
						RemainingPrefixSec(remainingSeconds)+fmt.Sprintf(
							"protocol converter successfully %s with state '%s', %s DFC configuration verified",
							terminal,
							instance.CurrentState,
							a.dfcType.String(),
						),
						a.outboundChannel,
						models.EditProtocolConverter,
					)

					return "", nil
				}

				// Get the current state reason for more detailed information
				if pcSnapshot != nil && pcSnapshot.ServiceInfo.StatusReason != "" {
					currentStateReason = pcSnapshot.ServiceInfo.StatusReason
				}

				// send the benthos logs to the user; for DFCTypeBoth, prefer write logs since
				// read logs are also included below via the merged slice.

				switch a.dfcType {
				case DFCTypeWrite, DFCTypeBoth:
					logs = pcSnapshot.ServiceInfo.DataflowComponentWriteObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs
				default:
					logs = pcSnapshot.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs
				}

				// only send the logs that have not been sent yet
				if len(logs) > len(lastLogs) {
					lastLogs = SendLimitedLogs(logs, lastLogs, a.instanceUUID, a.userEmail, a.actionUUID, a.outboundChannel, models.EditProtocolConverter, remainingSeconds)

				}

				// CheckBenthosLogLinesForConfigErrors is used to detect fatal configuration errors that would cause
				// Benthos to enter a CrashLoop. When such errors are detected, we can immediately
				// abort the startup process rather than waiting for the full timeout period,
				// as these errors require configuration changes to resolve.
				if CheckBenthosLogLinesForConfigErrors(logs) {
					SendActionReply(
						a.instanceUUID,
						a.userEmail,
						a.actionUUID,
						models.ActionExecuting,
						Label("edit", a.name)+"configuration error detected. Rolling back...",
						a.outboundChannel,
						models.EditProtocolConverter,
					)

					ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
					defer cancel()

					a.actionLogger.Infof("rolling back to previous configuration with user variables: %v", pcConfig.ProtocolConverterServiceConfig.Variables.User)

					_, err := a.configManager.AtomicEditProtocolConverter(ctx, a.atomicEditUUID, pcConfig)
					if err != nil {
						a.actionLogger.Errorf("failed to roll back protocol converter %s: %v", a.name, err)

						return models.ErrConfigFileInvalid, fmt.Errorf("protocol converter '%s' has invalid configuration but could not be rolled back: %w. Please check your logs and consider manually restoring the previous configuration", a.name, err)
					}

					return models.ErrConfigFileInvalid, fmt.Errorf("protocol converter '%s' was rolled back to its previous configuration due to configuration errors. Please check the component logs, fix the configuration issues, and try editing again", a.name)
				}

				SendActionReply(
					a.instanceUUID,
					a.userEmail,
					a.actionUUID,
					models.ActionExecuting,
					RemainingPrefixSec(remainingSeconds)+currentStateReason,
					a.outboundChannel,
					models.EditProtocolConverter,
				)
			}

			if !found {
				SendActionReply(
					a.instanceUUID,
					a.userEmail,
					a.actionUUID,
					models.ActionExecuting,
					RemainingPrefixSec(remainingSeconds)+"waiting for protocol converter to appear in the system",
					a.outboundChannel,
					models.EditProtocolConverter,
				)
			}
		}
	}
}

// compareProtocolConverterDFCConfig compares the desired DFC configuration with the observed
// DFC configuration in the protocol converter snapshot.
func (a *EditProtocolConverterAction) compareProtocolConverterDFCConfig(pcSnapshot *protocolconverter.ProtocolConverterObservedStateSnapshot) bool {
	if pcSnapshot == nil {
		return false
	}

	switch a.dfcType {
	case DFCTypeEmpty:
		return true
	case DFCTypeRead:
		return a.compareSingleDFCConfig(pcSnapshot, DFCTypeRead)
	case DFCTypeWrite:
		return a.compareSingleDFCConfig(pcSnapshot, DFCTypeWrite)
	case DFCTypeBoth:
		return a.compareSingleDFCConfig(pcSnapshot, DFCTypeRead) &&
			a.compareSingleDFCConfig(pcSnapshot, DFCTypeWrite)
	default:
		return false
	}
}

// compareSingleDFCConfig compares a single DFC (read or write) against its observed state.
func (a *EditProtocolConverterAction) compareSingleDFCConfig(pcSnapshot *protocolconverter.ProtocolConverterObservedStateSnapshot, dfcType DFCType) bool {
	var (
		desiredState      string
		observedFSMState  string
		observedDFCConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig
		presenceField     interface{} // the field that indicates the observed config is available
		excludeInput      bool        // true for write DFC (input is auto-generated)
	)

	switch dfcType {
	case DFCTypeRead:
		desiredState = a.readDFCState
		observedFSMState = pcSnapshot.ServiceInfo.DataflowComponentReadFSMState
		obs := pcSnapshot.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ObservedBenthosServiceConfig
		presenceField = obs.Input
		observedDFCConfig = observedBenthosToServiceConfig(obs)
	case DFCTypeWrite:
		desiredState = a.writeDFCState
		observedFSMState = pcSnapshot.ServiceInfo.DataflowComponentWriteFSMState
		obs := pcSnapshot.ServiceInfo.DataflowComponentWriteObservedState.ServiceInfo.BenthosObservedState.ObservedBenthosServiceConfig
		presenceField = obs.Output
		observedDFCConfig = observedBenthosToServiceConfig(obs)
		excludeInput = true
	default:
		return false
	}

	// When DFC is being stopped, check FSM state instead of Benthos config.
	if desiredState == protocolconverter.OperationalStateStopped {
		return observedFSMState == protocolconverter.OperationalStateStopped
	}

	if presenceField == nil {
		return false
	}

	renderedDesiredConfig, err := a.renderDesiredDFCConfig(pcSnapshot, dfcType)
	if err != nil {
		a.actionLogger.Errorf("failed to render desired %s DFC config: %v", dfcType, err)
		return false
	}

	// Exclude the auto-generated field (output for read, input for write) from comparison.
	if excludeInput {
		observedDFCConfig.BenthosConfig.Input = nil
		renderedDesiredConfig.BenthosConfig.Input = nil
	} else {
		observedDFCConfig.BenthosConfig.Output = nil
		renderedDesiredConfig.BenthosConfig.Output = nil
	}

	a.actionLogger.Debugf("observed %s DFC config: %+v", dfcType, observedDFCConfig)
	a.actionLogger.Debugf("rendered desired %s DFC config: %+v", dfcType, renderedDesiredConfig)

	return dataflowcomponentserviceconfig.NewComparator().ConfigsEqual(observedDFCConfig, renderedDesiredConfig)
}

// observedBenthosToServiceConfig converts an observed BenthosServiceConfig to a
// DataflowComponentServiceConfig for comparison with the desired config.
func observedBenthosToServiceConfig(obs benthosserviceconfig.BenthosServiceConfig) dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
			Input:              obs.Input,
			Pipeline:           obs.Pipeline,
			Output:             obs.Output,
			CacheResources:     obs.CacheResources,
			RateLimitResources: obs.RateLimitResources,
			Buffer:             obs.Buffer,
		},
	}
}

// renderDesiredDFCConfig renders the template variables in the desired DFC config
// using the actual runtime values from the protocol converter observed state.
// dfcTypeToReturn specifies which side (read or write) to return after rendering.
func (a *EditProtocolConverterAction) renderDesiredDFCConfig(pcSnapshot *protocolconverter.ProtocolConverterObservedStateSnapshot, dfcTypeToReturn DFCType) (dataflowcomponentserviceconfig.DataflowComponentServiceConfig, error) {
	if dfcTypeToReturn != DFCTypeRead && dfcTypeToReturn != DFCTypeWrite {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("invalid DFC type for rendering: %s", dfcTypeToReturn.String())
	}

	// Get the observed spec config
	specConfig := pcSnapshot.ObservedProtocolConverterSpecConfig

	// Create a deep copy to avoid mutating the original observed state
	var modifiedSpec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec

	err := deepcopy.Copy(&modifiedSpec, &specConfig)
	if err != nil {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("failed to deep copy spec config: %w", err)
	}

	// Apply whichever desired DFC configs are set, so the runtime render sees the full picture.
	if a.desiredReadDFCConfig.BenthosConfig.Input != nil {
		modifiedSpec.Config.DataflowComponentReadServiceConfig = a.desiredReadDFCConfig
	}

	if a.desiredWriteDFCConfig.BenthosConfig.Output != nil {
		modifiedSpec.Config.DataflowComponentWriteServiceConfig = a.desiredWriteDFCConfig
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

	switch dfcTypeToReturn {
	case DFCTypeRead:
		return runtimeConfig.DataflowComponentReadServiceConfig, nil
	case DFCTypeWrite:
		return runtimeConfig.DataflowComponentWriteServiceConfig, nil
	default:
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("invalid DFC type: %s", dfcTypeToReturn.String())
	}
}

// dfcToPayload converts a ProtocolConverterDFC to the internal CDFCPayload representation.
func dfcToPayload(dfc *models.ProtocolConverterDFC) models.CDFCPayload {
	return models.CDFCPayload{
		Inputs:   models.DfcDataConfig{Data: dfc.Inputs.Data, Type: dfc.Inputs.Type},
		Pipeline: convertPipelineToMap(dfc.Pipeline),
		Outputs:  models.DfcDataConfig{Data: dfc.Outputs.Data, Type: dfc.Outputs.Type},
		Inject:   extractInjectFromRawYAML(dfc.RawYAML),
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

// deriveDFCType compares the incoming DFC payloads against the currently
// deployed config and returns a DFCType that only includes the sides that
// actually differ. If no config is deployed yet (first edit) or the config
// cannot be read, it falls back to payload presence.
func (a *EditProtocolConverterAction) deriveDFCType() DFCType {
	hasRead := a.readDFCPayload != nil
	hasWrite := a.writeDFCPayload != nil

	if !hasRead && !hasWrite {
		return DFCTypeEmpty
	}

	// Try to fetch the current config for diff comparison.
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	currentConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		a.actionLogger.Debugf("Cannot read current config for diff, falling back to payload presence: %v", err)
		return dfcTypeFromPresence(hasRead, hasWrite)
	}

	// Find the protocol converter in the current config.
	var currentPC *config.ProtocolConverterConfig
	for i, pc := range currentConfig.ProtocolConverter {
		if dataflowcomponentserviceconfig.GenerateUUIDFromName(pc.Name) == a.protocolConverterUUID {
			currentPC = &currentConfig.ProtocolConverter[i]
			break
		}
	}
	if currentPC == nil {
		a.actionLogger.Debugf("Protocol converter %s not found in current config, falling back to payload presence", a.protocolConverterUUID)
		return dfcTypeFromPresence(hasRead, hasWrite)
	}

	readChanged := hasRead && a.dfcPayloadDiffers(*a.readDFCPayload, a.readDFCState,
		currentPC.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig,
		currentPC.ProtocolConverterServiceConfig.ReadDFCDesiredState)
	writeChanged := hasWrite && a.dfcPayloadDiffers(*a.writeDFCPayload, a.writeDFCState,
		currentPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig,
		currentPC.ProtocolConverterServiceConfig.WriteDFCDesiredState)

	derived := dfcTypeFromPresence(readChanged, writeChanged)
	a.actionLogger.Debugf("Derived dfcType=%s (readChanged=%v, writeChanged=%v)", derived, readChanged, writeChanged)

	return derived
}

// dfcPayloadDiffers checks whether an incoming DFC payload differs from the
// persisted config in config.yaml. This is intentionally simpler than
// compareSingleDFCConfig, which compares rendered templates against runtime
// observed state from the FSM snapshot. Here we only need to know whether the
// user sent something new relative to what is already on disk.
func (a *EditProtocolConverterAction) dfcPayloadDiffers(
	payload models.CDFCPayload,
	incomingState string,
	deployedConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	deployedState string,
) bool {
	// State change counts as a diff.
	if incomingState != "" && incomingState != deployedState {
		return true
	}

	incomingBenthos, err := CreateBenthosConfigFromCDFCPayload(payload, a.name)
	if err != nil {
		a.actionLogger.Debugf("Cannot build benthos config for diff, treating as changed: %v", err)
		return true
	}

	incomingDFCConfig := dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
		BenthosConfig: incomingBenthos,
	}

	return !deployedConfig.Equal(incomingDFCConfig)
}

// dfcTypeFromPresence returns a DFCType based on boolean flags for read/write.
func dfcTypeFromPresence(read, write bool) DFCType {
	switch {
	case read && write:
		return DFCTypeBoth
	case read:
		return DFCTypeRead
	case write:
		return DFCTypeWrite
	default:
		return DFCTypeEmpty
	}
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
// Returns the read DFC payload when available, otherwise the write DFC payload.
func (a *EditProtocolConverterAction) GetParsedPayload() models.CDFCPayload {
	if a.readDFCPayload != nil {
		return *a.readDFCPayload
	}

	if a.writeDFCPayload != nil {
		return *a.writeDFCPayload
	}

	return models.CDFCPayload{}
}

// GetProtocolConverterUUID returns the protocol converter UUID - exposed for testing purposes.
func (a *EditProtocolConverterAction) GetProtocolConverterUUID() uuid.UUID {
	return a.protocolConverterUUID
}

// GetDFCType returns the DFC type (read/write) - exposed for testing purposes.
func (a *EditProtocolConverterAction) GetDFCType() string {
	return a.dfcType.String()
}

// validateDFCPayloadAndState validates a pre-parsed CDFCPayload and its state string.
func validateDFCPayloadAndState(payload *models.CDFCPayload, state string, label string) error {
	if payload != nil {
		if err := ValidateCustomDataFlowComponentPayload(*payload, false); err != nil {
			return fmt.Errorf("invalid %s DFC configuration: %w", label, err)
		}
	}
	if state != "" {
		if err := ValidateDataFlowComponentState(state); err != nil {
			return fmt.Errorf("invalid %s DFC state: %w", label, err)
		}
	}
	return nil
}

// validateProtocolConverterDFC validates a ProtocolConverterDFC's state and configuration.
// Returns nil if dfc is nil (nothing to validate).
func validateProtocolConverterDFC(dfc *models.ProtocolConverterDFC, label string) error {
	if dfc == nil {
		return nil
	}
	if dfc.State != "" {
		if err := ValidateDataFlowComponentState(dfc.State); err != nil {
			return fmt.Errorf("invalid %s DFC state: %w", label, err)
		}
	}
	payload := dfcToPayload(dfc)
	if err := ValidateCustomDataFlowComponentPayload(payload, false); err != nil {
		return fmt.Errorf("invalid %s DFC configuration: %w", label, err)
	}
	return nil
}
