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
// and manage protocol converter configurations in the UMH system.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A Protocol Converter (PC) in UMH connects external data sources/sinks to the
// unified namespace using templated configurations. "Deploying" a protocol
// converter means:
//
//   1. Creating a new configuration entry with a YAML anchor template
//   2. Setting IP and PORT as template variables
//   3. Generating a UUID based on the component name
//   4. Adding the configuration to the central store
//
// The action creates a minimal protocol converter that can later be enhanced
// with actual dataflow component configurations through edit actions.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// DeployProtocolConverterAction implements the Action interface for deploying a
// new Protocol Converter. All fields are immutable after construction to
// avoid race conditions.
type DeployProtocolConverterAction struct {
	configManager config.ConfigManager

	outboundChannel       chan *models.UMHMessage
	systemSnapshotManager *fsm.SnapshotManager // Snapshot Manager holds the latest system snapshot
	actionLogger          *zap.SugaredLogger

	userEmail string
	// Parsed request payload (only populated after Parse)
	payload models.ProtocolConverter

	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewDeployProtocolConverterAction returns an un-parsed action instance.
func NewDeployProtocolConverterAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *DeployProtocolConverterAction {
	return &DeployProtocolConverterAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
		systemSnapshotManager: systemSnapshotManager,
	}
}

// Parse implements the Action interface by extracting protocol converter configuration from the payload.
func (a *DeployProtocolConverterAction) Parse(payload interface{}) error {
	// Parse the payload to get the protocol converter configuration
	parsedPayload, err := ParseActionPayload[models.ProtocolConverter](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	a.payload = parsedPayload
	a.actionLogger.Debugf("Parsed DeployProtocolConverter action payload: name=%s, ip=%s, port=%d",
		a.payload.Name, a.payload.Connection.IP, a.payload.Connection.Port)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *DeployProtocolConverterAction) Validate() error {
	// Validate all required fields
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	if a.payload.Connection.IP == "" {
		return errors.New("missing required field Connection.IP")
	}

	if a.payload.Connection.Port == 0 {
		return errors.New("missing required field Connection.Port")
	}

	if err := config.ValidateComponentName(a.payload.Name); err != nil {
		return err
	}

	return nil
}

// Execute implements the Action interface by creating the protocol converter configuration.
func (a *DeployProtocolConverterAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeployProtocolConverter action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting deployment of protocol converter: "+a.payload.Name, a.outboundChannel, models.DeployProtocolConverter)

	// Create the protocol converter config with template and variables
	pcConfig := a.createProtocolConverterConfig()

	// currently, we canot reuse templates, so we need to create a new one
	pcConfig.ProtocolConverterServiceConfig.TemplateRef = pcConfig.Name

	// Add to configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Adding protocol converter to configuration...", a.outboundChannel, models.DeployProtocolConverter)

	err := a.configManager.AtomicAddProtocolConverter(ctx, pcConfig)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to add protocol converter: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.DeployProtocolConverter)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Generate the UUID for the response
	pcUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(a.payload.Name)

	// Create response with the filled UUID
	response := models.ProtocolConverter{
		UUID:       &pcUUID,
		Name:       a.payload.Name,
		Location:   a.payload.Location,
		Connection: a.payload.Connection,
		// ReadDFC, WriteDFC, and TemplateInfo are nil as they will be added later
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Waiting for protocol converter to be active...", a.outboundChannel, models.DeployProtocolConverter)

	// check against observedState
	if a.systemSnapshotManager != nil {
		errCode, err := a.waitForComponentToAppear()
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to wait for protocol converter to be active: %v", err)
			SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, errCode, nil, a.outboundChannel, models.DeployProtocolConverter, nil)

			return nil, nil, fmt.Errorf("%s", errorMsg)
		}
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Protocol converter successfully deployed and activated", a.outboundChannel, models.DeployProtocolConverter)

	return response, nil, nil
}

// createProtocolConverterConfig creates a ProtocolConverterConfig with templated configuration.
func (a *DeployProtocolConverterAction) createProtocolConverterConfig() config.ProtocolConverterConfig {
	// Create variables bundle - start empty to allow user variables first
	userVars := map[string]any{}

	// Add any additional user-supplied variables from TemplateInfo.Variables
	if a.payload.TemplateInfo != nil {
		for _, variable := range a.payload.TemplateInfo.Variables {
			userVars[variable.Label] = variable.Value
		}
	}

	// Enforce reserved connection variables after merging to prevent user overrides
	userVars["IP"] = a.payload.Connection.IP                                     // Keep IP as string
	userVars["PORT"] = strconv.FormatUint(uint64(a.payload.Connection.Port), 10) // Convert port to string

	variableBundle := variables.VariableBundle{
		User: userVars,
	}

	// Create template configuration with connection and placeholders for DFCs
	template := protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
		ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
			NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
				Target: "{{ .IP }}",   // Template variable for IP
				Port:   "{{ .PORT }}", // Template variable for PORT
			},
		},
		// DataflowComponent configs left empty initially - they will be configured later via edit actions
		DataflowComponentReadServiceConfig:  dataflowcomponentserviceconfig.DataflowComponentServiceConfig{},
		DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{},
	}

	// Create the spec with template and variables
	spec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
		Config:    template,
		Variables: variableBundle,
		Location:  convertIntMapToStringMap(a.payload.Location),
	}

	// Create the full config
	return config.ProtocolConverterConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            a.payload.Name,
			DesiredFSMState: "active", // Default to active state
		},
		ProtocolConverterServiceConfig: spec,
	}
}

// convertIntMapToStringMap converts map[int]string to map[string]string.
func convertIntMapToStringMap(intMap map[int]string) map[string]string {
	if intMap == nil {
		return nil
	}

	stringMap := make(map[string]string)
	for k, v := range intMap {
		stringMap[strconv.Itoa(k)] = v
	}

	return stringMap
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *DeployProtocolConverterAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *DeployProtocolConverterAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed payload - exposed primarily for testing purposes.
func (a *DeployProtocolConverterAction) GetParsedPayload() models.ProtocolConverter {
	return a.payload
}

// waitForComponentToAppear polls live FSM state until the new component
// becomes available or the timeout hits (→ delete unless ignoreHealthCheck).
// the function returns the error code and the error message via an error object
// the error code is a string that is sent to the frontend to allow it to determine if the action can be retried or not
// the error message is sent to the frontend to allow the user to see the error message.
func (a *DeployProtocolConverterAction) waitForComponentToAppear() (string, error) {
	ticker := time.NewTicker(constants.ActionTickerTime)
	defer ticker.Stop()

	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout)
	startTime := time.Now()
	timeoutDuration := constants.DataflowComponentWaitForActiveTimeout

	// Track last known blocking reason for timeout error message
	var lastStatusReason string

	for {
		elapsed := time.Since(startTime)
		remaining := timeoutDuration - elapsed
		remainingSeconds := int(remaining.Seconds())

		select {
		case <-timeout:
			stateMessage := Label("deploy", a.payload.Name) + "timeout reached. it did not become active in time. removing"
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage,
				a.outboundChannel, models.DeployProtocolConverter)

			ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
			defer cancel()

			err := a.configManager.AtomicDeleteProtocolConverter(ctx, dataflowcomponentserviceconfig.GenerateUUIDFromName(a.payload.Name))
			if err != nil {
				a.actionLogger.Errorf("failed to remove protocol converter %s: %v", a.payload.Name, err)

				return models.ErrRetryRollbackTimeout, fmt.Errorf("protocol converter '%s' failed to activate within timeout but could not be removed: %w. Please check system load and consider removing the component manually", a.payload.Name, err)
			}

			// Build timeout error message with blocking reason if available
			errorMsg := fmt.Sprintf("protocol converter '%s' was removed because it did not become active within the timeout period", a.payload.Name)
			if lastStatusReason != "" {
				errorMsg = fmt.Sprintf("protocol converter '%s' was removed because: %s", a.payload.Name, lastStatusReason)
			} else {
				errorMsg += ". Please check system load or component configuration and try again"
			}

			return models.ErrRetryRollbackTimeout, fmt.Errorf("%s", errorMsg)

		case <-ticker.C:
			// the snapshot manager holds the latest system snapshot which is asynchronously updated by the other goroutines
			// we need to get a deep copy of it to prevent race conditions
			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()
			if protocolConverterManager, exists := systemSnapshot.Managers[constants.ProtocolConverterManagerName]; exists {
				instances := protocolConverterManager.GetInstances()
				found := false

				for _, instance := range instances {
					curName := instance.ID
					if curName != a.payload.Name {
						continue
					}

					found = true

					// Check if the protocol converter is in an active state
					// Note: starting_failed_dfc_missing is a valid state for empty bridges (no DFCs configured yet)
					// This allows the deploy → edit workflow where deploy creates an empty bridge and edit adds DFCs later
					if instance.CurrentState == "active" || instance.CurrentState == "idle" || instance.CurrentState == "starting_failed_dfc_missing" {
						return "", nil
					}

					// Get more detailed status information from the protocol converter snapshot
					currentStateReason := "current state: " + instance.CurrentState

					// Cast the instance LastObservedState to a protocolconverter instance
					pcSnapshot, ok := instance.LastObservedState.(*protocolconverter.ProtocolConverterObservedStateSnapshot)
					if ok && pcSnapshot != nil && pcSnapshot.ServiceInfo.StatusReason != "" {
						// Use the raw status reason from FSM - frontend will handle enhancement
						lastStatusReason = pcSnapshot.ServiceInfo.StatusReason
						currentStateReason = pcSnapshot.ServiceInfo.StatusReason
					}

					stateMessage := RemainingPrefixSec(remainingSeconds) + currentStateReason
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.DeployProtocolConverter)
				}

				if !found {
					stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for it to appear in the config"
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.DeployProtocolConverter)
				}
			} else {
				stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for manager to initialise"
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					stateMessage, a.outboundChannel, models.DeployProtocolConverter)
			}
		}
	}
}
