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
// stream processor configurations.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// An existing Stream Processor (SP) in UMH processes data from the unified
// namespace using data models. The edit action allows updating the model
// reference, sources mapping, field mappings, and variables of an existing
// stream processor configuration.
//
// The action follows a pattern similar to edit-protocolconverter but operates
// on stream processor configurations instead of protocol converter configurations.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// EditStreamProcessorAction implements the Action interface for editing
// stream processor configurations.
type EditStreamProcessorAction struct {
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager

	// Parsed request payload (only populated after Parse)
	streamProcessorUUID uuid.UUID
	name                string // stream processor name
	model               models.StreamProcessorModelRef
	sources             map[string]string
	mapping             map[string]interface{}
	vb                  []models.StreamProcessorVariable
	ignoreHealthCheck   bool
	location            map[int]string

	// Runtime observation for health checks
	systemSnapshotManager *fsm.SnapshotManager

	// Atomic edit UUID used for configuration updates and rollbacks
	atomicEditUUID uuid.UUID

	actionLogger *zap.SugaredLogger
}

// NewEditStreamProcessorAction returns an un-parsed action instance.
func NewEditStreamProcessorAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *EditStreamProcessorAction {
	return &EditStreamProcessorAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		systemSnapshotManager: systemSnapshotManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface by extracting the stream processor UUID and
// configuration from the payload.
func (a *EditStreamProcessorAction) Parse(payload interface{}) error {
	// Parse the payload directly as a complete StreamProcessor object
	spPayload, err := ParseActionPayload[models.StreamProcessor](payload)
	if err != nil {
		return fmt.Errorf("failed to parse stream processor payload: %v", err)
	}

	// Extract UUID
	if spPayload.UUID == nil {
		return errors.New("missing required field UUID")
	}
	a.streamProcessorUUID = *spPayload.UUID
	a.name = spPayload.Name
	a.model = spPayload.Model

	// Decode the base64-encoded config
	decodedConfig, err := base64.StdEncoding.DecodeString(spPayload.EncodedConfig)
	if err != nil {
		return fmt.Errorf("failed to decode stream processor config: %v", err)
	}

	var config models.StreamProcessorConfig
	err = yaml.Unmarshal(decodedConfig, &config)
	if err != nil {
		return fmt.Errorf("failed to unmarshal stream processor config: %v", err)
	}

	a.sources = config.Sources
	a.mapping = convertStreamProcessorMappingToInterface(config.Mapping)

	if spPayload.TemplateInfo != nil {
		a.vb = spPayload.TemplateInfo.Variables
	} else {
		a.vb = make([]models.StreamProcessorVariable, 0)
	}

	// Extract location
	if spPayload.Location != nil {
		a.location = spPayload.Location
	}

	a.actionLogger.Debugf("Parsed EditStreamProcessor action payload: uuid=%s, name=%s, model=%s:%s",
		a.streamProcessorUUID, a.name, a.model.Name, a.model.Version)
	return nil
}

// Validate performs validation of the parsed payload.
func (a *EditStreamProcessorAction) Validate() error {
	// Validate UUID
	if a.streamProcessorUUID == uuid.Nil {
		return errors.New("missing or invalid stream processor UUID")
	}

	if err := ValidateStreamProcessorName(a.name); err != nil {
		return err
	}

	if a.model.Name == "" {
		return errors.New("missing required field Model.Name")
	}

	if a.model.Version == "" {
		return errors.New("missing required field Model.Version")
	}

	return nil
}

// Execute implements the Action interface by updating the stream processor configuration
// with the provided configuration.
func (a *EditStreamProcessorAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing EditStreamProcessor action")

	// Send confirmation that action is starting
	confirmationMessage := fmt.Sprintf("Starting edit of stream processor %s", a.streamProcessorUUID)
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		confirmationMessage, a.outboundChannel, models.EditStreamProcessor)

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Updating stream processor configuration...", a.outboundChannel, models.EditStreamProcessor)

	// Apply mutations to create new spec
	newSpec, atomicEditUUID, err := a.applyMutation()
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to apply configuration mutation: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditStreamProcessor)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Store the atomic edit UUID for use in rollback operations
	a.atomicEditUUID = atomicEditUUID

	// Persist the configuration changes
	oldConfig, err := a.persistConfig(atomicEditUUID, newSpec)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to persist configuration changes: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditStreamProcessor)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Await rollout and perform health checks
	if a.systemSnapshotManager != nil && !a.ignoreHealthCheck {
		errCode, err := a.awaitRollout(oldConfig)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed during rollout: %v", err)
			SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, errCode, nil, a.outboundChannel, models.EditStreamProcessor, nil)
			return nil, nil, fmt.Errorf("%s", errorMsg)
		}

		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			"Stream processor successfully updated", a.outboundChannel, models.EditStreamProcessor)
	}

	newUUID := generateUUIDFromName(a.name)
	response := map[string]any{
		"uuid": newUUID,
	}

	return response, nil, nil
}

// applyMutation analyzes the current configuration and applies the necessary mutations
// to create the new stream processor specification. It handles child/root relationships,
// variable merging, and configuration updates.
// Returns the new spec and the atomic edit UUID to use for persistence.
func (a *EditStreamProcessorAction) applyMutation() (config.StreamProcessorConfig, uuid.UUID, error) {
	// Get current configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	currentConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		return config.StreamProcessorConfig{}, uuid.Nil, fmt.Errorf("failed to get current configuration: %w", err)
	}

	// Find the stream processor in the configuration
	var targetSP config.StreamProcessorConfig
	found := false
	for _, sp := range currentConfig.StreamProcessor {
		spID := generateUUIDFromName(sp.Name)
		if spID == a.streamProcessorUUID {
			targetSP = sp
			found = true
			break
		}
	}

	if !found {
		return config.StreamProcessorConfig{}, uuid.Nil, fmt.Errorf("stream processor with UUID %s not found", a.streamProcessorUUID)
	}

	// Currently, we cannot reuse templates, so we need to create a new one
	targetSP.StreamProcessorServiceConfig.TemplateRef = a.name
	targetSP.Name = a.name

	// Classify the stream processor instance
	isChild := targetSP.StreamProcessorServiceConfig.TemplateRef != "" &&
		targetSP.StreamProcessorServiceConfig.TemplateRef != targetSP.Name

	// Determine which instance to modify and which UUID to use for atomic operation
	var instanceToModify config.StreamProcessorConfig
	var atomicEditUUID uuid.UUID
	var newVB map[string]any

	if isChild {
		// Find the root instance
		var rootSP config.StreamProcessorConfig
		rootFound := false
		for _, sp := range currentConfig.StreamProcessor {
			if sp.Name == targetSP.StreamProcessorServiceConfig.TemplateRef &&
				sp.StreamProcessorServiceConfig.TemplateRef == sp.Name {
				rootSP = sp
				rootFound = true
				break
			}
		}

		if !rootFound {
			return config.StreamProcessorConfig{}, uuid.Nil, fmt.Errorf("root template %s not found for child stream processor %s",
				targetSP.StreamProcessorServiceConfig.TemplateRef, targetSP.Name)
		}

		// Apply mutations to the root, but keep child's variables
		instanceToModify = rootSP
		atomicEditUUID = generateUUIDFromName(rootSP.Name)

		// Add the new variables and preserve existing child variables
		newVB = make(map[string]any)
		for _, variable := range a.vb {
			newVB[variable.Label] = variable.Value
		}
		maps.Copy(newVB, targetSP.StreamProcessorServiceConfig.Variables.User)
	} else {
		// Root or stand-alone: apply mutations directly
		instanceToModify = targetSP
		atomicEditUUID = a.streamProcessorUUID

		// Add the variables and keep the existing variables
		newVB = make(map[string]any)
		for _, variable := range a.vb {
			newVB[variable.Label] = variable.Value
		}
		maps.Copy(newVB, targetSP.StreamProcessorServiceConfig.Variables.User)
	}

	// As the BuildRuntimeConfig function always adds location and location_path to the user variables,
	// we need to remove them from the variables here to avoid that they end up in the config file
	delete(newVB, "location")
	delete(newVB, "location_path")

	instanceToModify.StreamProcessorServiceConfig.Variables.User = newVB

	// Update the configuration
	instanceToModify.StreamProcessorServiceConfig.Config.Model = streamprocessorserviceconfig.ModelRef{
		Name:    a.model.Name,
		Version: a.model.Version,
	}
	instanceToModify.StreamProcessorServiceConfig.Config.Sources = streamprocessorserviceconfig.SourceMapping(a.sources)
	instanceToModify.StreamProcessorServiceConfig.Config.Mapping = a.mapping

	// Update the location of the stream processor (convert the map to a string map)
	locationMap := make(map[string]string)
	for k, v := range a.location {
		locationMap[strconv.Itoa(k)] = v
	}
	instanceToModify.StreamProcessorServiceConfig.Location = locationMap

	return instanceToModify, atomicEditUUID, nil
}

// persistConfig performs the atomic configuration update operation.
// Returns the old configuration for potential rollback operations.
func (a *EditStreamProcessorAction) persistConfig(atomicEditUUID uuid.UUID, newSpec config.StreamProcessorConfig) (config.StreamProcessorConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	// Get the old config before applying changes (for rollback purposes)
	configSnapshot, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		return config.StreamProcessorConfig{}, fmt.Errorf("failed to get current config: %w", err)
	}

	// Find the old stream processor configuration
	var oldConfig config.StreamProcessorConfig
	found := false
	for _, sp := range configSnapshot.StreamProcessor {
		if sp.Name == newSpec.Name {
			oldConfig = sp
			found = true
			break
		}
	}

	if !found {
		return config.StreamProcessorConfig{}, fmt.Errorf("stream processor %q not found", newSpec.Name)
	}

	// Apply the configuration changes
	returnedOldConfig, err := a.configManager.AtomicEditStreamProcessor(ctx, newSpec)
	if err != nil {
		return config.StreamProcessorConfig{}, fmt.Errorf("failed to update stream processor: %w", err)
	}

	// Deep copy the old config using the Clone function
	// This may seem hacky but allows us to reuse the Clone() function
	// and we do not need to implement a custom Clone() function for the StreamProcessorConfig
	fullConfig := config.FullConfig{
		StreamProcessor: []config.StreamProcessorConfig{returnedOldConfig},
	}

	copiedConfig := fullConfig.Clone()
	oldConfig = copiedConfig.StreamProcessor[0]

	// Remove the location and location_path from the user variables
	// Check if User map exists before trying to delete from it
	if oldConfig.StreamProcessorServiceConfig.Variables.User != nil {
		delete(oldConfig.StreamProcessorServiceConfig.Variables.User, "location")
		delete(oldConfig.StreamProcessorServiceConfig.Variables.User, "location_path")
	}

	return oldConfig, nil
}

// awaitRollout waits for the stream processor to become active and performs health checks.
// Returns error code and error message for proper error handling in the caller.
func (a *EditStreamProcessorAction) awaitRollout(oldConfig config.StreamProcessorConfig) (string, error) {
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		fmt.Sprintf("Waiting for stream processor %s to be active...", a.name),
		a.outboundChannel, models.EditStreamProcessor)

	return a.waitForComponentToBeActive(oldConfig)
}

// waitForComponentToBeActive polls live FSM state until the stream processor
// becomes active or the timeout hits. Unlike deploy operations, this method
// does not remove the component on timeout since it's an edit operation.
// The function returns the error code and the error message via an error object.
// The error code is a string that is sent to the frontend to allow it to determine if the action can be retried or not.
// The error message is sent to the frontend to allow the user to see the error message.
func (a *EditStreamProcessorAction) waitForComponentToBeActive(oldConfig config.StreamProcessorConfig) (string, error) {
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
			// Rollback to previous configuration
			ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
			defer cancel()
			_, err := a.configManager.AtomicEditStreamProcessor(ctx, oldConfig)
			if err != nil {
				a.actionLogger.Errorf("Failed to rollback to previous configuration: %v", err)
				stateMessage := fmt.Sprintf("Stream processor '%s' edit timeout reached. It did not become active in time. Rolling back to previous configuration failed: %v", a.name, err)
				return models.ErrRetryRollbackTimeout, fmt.Errorf("%s", stateMessage)
			} else {
				stateMessage := fmt.Sprintf("Stream processor '%s' edit timeout reached. It did not become active in time. Rolled back to previous configuration", a.name)
				return models.ErrRetryRollbackTimeout, fmt.Errorf("%s", stateMessage)
			}

		case <-ticker.C:
			// Get a deep copy of the system snapshot to prevent race conditions
			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()
			if streamProcessorManager, exists := systemSnapshot.Managers[constants.StreamProcessorManagerName]; exists {
				instances := streamProcessorManager.GetInstances()
				found := false
				for _, instance := range instances {
					curName := instance.ID
					if curName != a.name {
						continue
					}

					found = true
					currentStateReason := fmt.Sprintf("current state: %s", instance.CurrentState)

					// Check if the stream processor is in an active state
					if instance.CurrentState == "active" || instance.CurrentState == "idle" {
						stateMessage := RemainingPrefixSec(remainingSeconds) + fmt.Sprintf("stream processor successfully activated with state '%s'", instance.CurrentState)
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage,
							a.outboundChannel, models.EditStreamProcessor)
						return "", nil
					}

					stateMessage := RemainingPrefixSec(remainingSeconds) + currentStateReason
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.EditStreamProcessor)
				}

				if !found {
					stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for stream processor to appear in the system"
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.EditStreamProcessor)
				}

			} else {
				stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for stream processor manager to initialise"
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					stateMessage, a.outboundChannel, models.EditStreamProcessor)
			}
		}
	}
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *EditStreamProcessorAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *EditStreamProcessorAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetStreamProcessorUUID returns the stream processor UUID - exposed for testing purposes.
func (a *EditStreamProcessorAction) GetStreamProcessorUUID() uuid.UUID {
	return a.streamProcessorUUID
}

// GetParsedPayload returns the parsed payload - exposed primarily for testing purposes.
func (a *EditStreamProcessorAction) GetParsedPayload() models.StreamProcessor {
	// Marshal the config back to base64 for response
	configData, err := yaml.Marshal(models.StreamProcessorConfig{
		Sources: a.sources,
		Mapping: convertInterfaceToStreamProcessorMapping(a.mapping),
	})
	if err != nil {
		a.actionLogger.Errorf("Failed to marshal stream processor config: %v", err)
		return models.StreamProcessor{}
	}
	encodedConfig := base64.StdEncoding.EncodeToString(configData)

	return models.StreamProcessor{
		UUID:          &a.streamProcessorUUID,
		Name:          a.name,
		Location:      a.location,
		Model:         a.model,
		EncodedConfig: encodedConfig,
	}
}
