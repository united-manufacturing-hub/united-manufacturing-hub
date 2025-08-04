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

// Package actions contains implementations of the Action interface that create
// and manage stream processor configurations in the UMH system.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A Stream Processor (SP) in UMH processes data from the unified namespace using
// templated configurations. "Deploying" a stream processor means:
//
//   1. Creating a new configuration entry with template variables
//   2. Setting model reference, sources, and mappings
//   3. Generating a UUID based on the component name
//   4. Adding the configuration to the central store
//
// The action creates a stream processor that transforms data from multiple
// sources into a structured data model format.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// DeployStreamProcessorAction implements the Action interface for deploying a
// new Stream Processor. All fields are immutable after construction to
// avoid race conditions.
type DeployStreamProcessorAction struct {

	// Parsed request payload (only populated after Parse)
	payload models.StreamProcessor

	configManager config.ConfigManager

	outboundChannel       chan *models.UMHMessage
	systemSnapshotManager *fsm.SnapshotManager // Snapshot Manager holds the latest system snapshot

	actionLogger      *zap.SugaredLogger
	userEmail         string
	actionUUID        uuid.UUID
	instanceUUID      uuid.UUID
	ignoreHealthCheck bool
}

// NewDeployStreamProcessorAction returns an un-parsed action instance.
func NewDeployStreamProcessorAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *DeployStreamProcessorAction {
	return &DeployStreamProcessorAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
		systemSnapshotManager: systemSnapshotManager,
	}
}

// Parse implements the Action interface by extracting stream processor configuration from the payload.
func (a *DeployStreamProcessorAction) Parse(payload interface{}) error {
	// Parse the payload to get the stream processor configuration
	parsedPayload, err := ParseActionPayload[models.StreamProcessor](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %v", err)
	}

	a.payload = parsedPayload

	// Decode the base64-encoded config
	decodedConfig, err := base64.StdEncoding.DecodeString(a.payload.EncodedConfig)
	if err != nil {
		return fmt.Errorf("failed to decode stream processor config: %v", err)
	}

	var config models.StreamProcessorConfig
	err = yaml.Unmarshal(decodedConfig, &config)
	if err != nil {
		return fmt.Errorf("failed to unmarshal stream processor config: %v", err)
	}

	a.payload.Config = &config

	a.ignoreHealthCheck = a.payload.IgnoreHealthCheck

	a.actionLogger.Debugf("Parsed DeployStreamProcessor action payload: name=%s, model=%s:%s",
		a.payload.Name, a.payload.Model.Name, a.payload.Model.Version)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *DeployStreamProcessorAction) Validate() error {
	// Validate all required fields
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	if a.payload.Model.Name == "" {
		return errors.New("missing required field Model.Name")
	}

	if a.payload.Model.Version == "" {
		return errors.New("missing required field Model.Version")
	}

	if err := ValidateComponentName(a.payload.Name); err != nil {
		return err
	}

	return nil
}

// Execute implements the Action interface by creating the stream processor configuration.
func (a *DeployStreamProcessorAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeployStreamProcessor action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting deployment of stream processor: "+a.payload.Name, a.outboundChannel, models.DeployStreamProcessor)

	// Create the stream processor config with template and variables
	spConfig := a.createStreamProcessorConfig()

	// Currently, we cannot reuse templates, so we need to create a new one
	spConfig.StreamProcessorServiceConfig.TemplateRef = spConfig.Name

	// Add to configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Adding stream processor to configuration...", a.outboundChannel, models.DeployStreamProcessor)

	err := a.configManager.AtomicAddStreamProcessor(ctx, spConfig)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to add stream processor: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.DeployStreamProcessor)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Generate the UUID for the response
	spUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(a.payload.Name)

	// Create response with the filled UUID
	response := models.StreamProcessor{
		UUID:          &spUUID,
		Name:          a.payload.Name,
		Location:      a.payload.Location,
		Model:         a.payload.Model,
		EncodedConfig: a.payload.EncodedConfig,
		Config:        a.payload.Config,
		// TemplateInfo is nil as it will be populated later if needed
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Waiting for stream processor to be active...", a.outboundChannel, models.DeployStreamProcessor)

	// Check against observedState if snapshot manager is available
	if a.systemSnapshotManager != nil && !a.ignoreHealthCheck {
		errCode, err := a.waitForComponentToAppear()
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to wait for stream processor to be active: %v", err)
			SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, errCode, nil, a.outboundChannel, models.DeployStreamProcessor, nil)
			return nil, nil, fmt.Errorf("%s", errorMsg)
		}
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Stream processor successfully deployed and activated", a.outboundChannel, models.DeployStreamProcessor)

	return response, nil, nil
}

// createStreamProcessorConfig creates a StreamProcessorConfig with templated configuration
func (a *DeployStreamProcessorAction) createStreamProcessorConfig() config.StreamProcessorConfig {
	// Create variables bundle starting with any user-supplied variables
	userVars := make(map[string]any)

	// Add any additional user-supplied variables from TemplateInfo.Variables
	if a.payload.TemplateInfo != nil {
		for _, variable := range a.payload.TemplateInfo.Variables {
			userVars[variable.Label] = variable.Value
		}
	}

	variableBundle := variables.VariableBundle{
		User: userVars,
	}

	// Create template configuration
	template := streamprocessorserviceconfig.StreamProcessorServiceConfigTemplate{
		Model: streamprocessorserviceconfig.ModelRef{
			Name:    a.payload.Model.Name,
			Version: a.payload.Model.Version,
		},
		Sources: streamprocessorserviceconfig.SourceMapping(a.payload.Config.Sources),
		Mapping: convertStreamProcessorMappingToInterface(a.payload.Config.Mapping),
	}

	// Convert location map from int keys to string keys
	locationMap := make(map[string]string)
	if a.payload.Location != nil {
		for k, v := range a.payload.Location {
			locationMap[fmt.Sprintf("%d", k)] = v
		}
	}

	// Create the spec with template and variables
	spec := streamprocessorserviceconfig.StreamProcessorServiceConfigSpec{
		Config:    template,
		Variables: variableBundle,
		Location:  locationMap,
	}

	// Create the full config
	return config.StreamProcessorConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            a.payload.Name,
			DesiredFSMState: "active", // Default to active state
		},
		StreamProcessorServiceConfig: spec,
	}
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *DeployStreamProcessorAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *DeployStreamProcessorAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed payload - exposed primarily for testing purposes.
func (a *DeployStreamProcessorAction) GetParsedPayload() models.StreamProcessor {
	return a.payload
}

// waitForComponentToAppear polls live FSM state until the new component
// becomes available or the timeout hits (→ delete unless ignoreHealthCheck).
// The function returns the error code and the error message via an error object
// The error code is a string that is sent to the frontend to allow it to determine if the action can be retried or not
// The error message is sent to the frontend to allow the user to see the error message
func (a *DeployStreamProcessorAction) waitForComponentToAppear() (string, error) {
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
			stateMessage := Label("deploy", a.payload.Name) + "timeout reached. it did not become active in time. removing"
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage,
				a.outboundChannel, models.DeployStreamProcessor)

			ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
			defer cancel()
			err := a.configManager.AtomicDeleteStreamProcessor(ctx, a.payload.Name)
			if err != nil {
				a.actionLogger.Errorf("failed to remove stream processor %s: %v", a.payload.Name, err)
				return models.ErrRetryRollbackTimeout, fmt.Errorf("stream processor '%s' failed to activate within timeout but could not be removed: %v. Please check system load and consider removing the component manually", a.payload.Name, err)
			}
			return models.ErrRetryRollbackTimeout, fmt.Errorf("stream processor '%s' was removed because it did not become active within the timeout period. Please check system load or component configuration and try again", a.payload.Name)

		case <-ticker.C:
			// The snapshot manager holds the latest system snapshot which is asynchronously updated by other goroutines
			// We need to get a deep copy of it to prevent race conditions
			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()
			if streamProcessorManager, exists := systemSnapshot.Managers[constants.StreamProcessorManagerName]; exists {
				instances := streamProcessorManager.GetInstances()
				found := false
				for _, instance := range instances {
					curName := instance.ID
					if curName != a.payload.Name {
						continue
					}

					found = true

					// Check if the stream processor is in an active state
					if instance.CurrentState == "active" || instance.CurrentState == "idle" {
						return "", nil
					}

					// Get more detailed status information from the stream processor snapshot
					currentStateReason := fmt.Sprintf("current state: %s", instance.CurrentState)

					// Cast the instance LastObservedState to a streamprocessor instance
					spSnapshot, ok := instance.LastObservedState.(*streamprocessor.ObservedStateSnapshot)
					if ok && spSnapshot != nil && spSnapshot.ServiceInfo.StatusReason != "" {
						currentStateReason = spSnapshot.ServiceInfo.StatusReason
					}

					stateMessage := RemainingPrefixSec(remainingSeconds) + currentStateReason
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.DeployStreamProcessor)
				}

				if !found {
					stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for it to appear in the config"
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.DeployStreamProcessor)
				}

			} else {
				stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for manager to initialise"
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					stateMessage, a.outboundChannel, models.DeployStreamProcessor)
			}
		}
	}
}
