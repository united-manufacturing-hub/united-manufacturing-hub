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

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// DeployProtocolConverterAction implements the Action interface for deploying a
// new Protocol Converter. All fields are immutable after construction to
// avoid race conditions.
type DeployProtocolConverterAction struct {
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager

	// Parsed request payload (only populated after Parse)
	payload models.ProtocolConverter

	actionLogger *zap.SugaredLogger
}

// NewDeployProtocolConverterAction returns an un-parsed action instance.
func NewDeployProtocolConverterAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *DeployProtocolConverterAction {
	return &DeployProtocolConverterAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface by extracting protocol converter configuration from the payload.
func (a *DeployProtocolConverterAction) Parse(payload interface{}) error {
	// Parse the payload to get the protocol converter configuration
	parsedPayload, err := ParseActionPayload[models.ProtocolConverter](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %v", err)
	}

	// Validate required fields
	if parsedPayload.Name == "" {
		return errors.New("missing required field Name")
	}

	if parsedPayload.Connection.IP == "" {
		return errors.New("missing required field Connection.IP")
	}

	if parsedPayload.Connection.Port == 0 {
		return errors.New("missing required field Connection.Port")
	}

	a.payload = parsedPayload
	a.actionLogger.Debugf("Parsed DeployProtocolConverter action payload: name=%s, ip=%s, port=%d",
		a.payload.Name, a.payload.Connection.IP, a.payload.Connection.Port)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *DeployProtocolConverterAction) Validate() error {
	// Basic validation was done in Parse, additional validation can be added here
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	if a.payload.Connection.IP == "" {
		return errors.New("missing required field Connection.IP")
	}

	if a.payload.Connection.Port == 0 {
		return errors.New("missing required field Connection.Port")
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

	return response, nil, nil
}

// createProtocolConverterConfig creates a ProtocolConverterConfig with templated configuration
func (a *DeployProtocolConverterAction) createProtocolConverterConfig() config.ProtocolConverterConfig {
	// Create variables bundle with IP and PORT in the User namespace
	variableBundle := variables.VariableBundle{
		User: map[string]any{
			"IP":   a.payload.Connection.IP,
			"PORT": fmt.Sprintf("%d", a.payload.Connection.Port),
		},
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
		Template:  template,
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

// convertIntMapToStringMap converts map[int]string to map[string]string
func convertIntMapToStringMap(intMap map[int]string) map[string]string {
	if intMap == nil {
		return nil
	}

	stringMap := make(map[string]string)
	for k, v := range intMap {
		stringMap[fmt.Sprintf("%d", k)] = v
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
