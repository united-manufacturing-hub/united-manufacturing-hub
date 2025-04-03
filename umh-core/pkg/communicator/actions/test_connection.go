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

package actions

import (
	"context"
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"go.uber.org/zap"
)

type TestNetworkConnectionAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	Payload         models.TestNetworkConnectionPayload
}

// getUserEmail returns the user email of the action
func (t *TestNetworkConnectionAction) getUserEmail() string {
	return t.userEmail
}

// getUuid returns the UUID of the action
func (t *TestNetworkConnectionAction) getUuid() uuid.UUID {
	return t.actionUUID
}

// Parse parses the payload into the TestNetworkConnectionPayload type.
func (t *TestNetworkConnectionAction) Parse(actionPayload interface{}) (err error) {
	t.Payload, err = ParseActionPayload[models.TestNetworkConnectionPayload](actionPayload)
	return err
}

// Validate checks that IP, port, and type are valid.
func (t *TestNetworkConnectionAction) Validate() error {
	// Validate IP address
	if t.Payload.IP == "" {
		return fmt.Errorf("IP address cannot be empty")
	}
	_, err := url.Parse(t.Payload.IP)
	if err != nil {
		return fmt.Errorf("invalid IP address: %s", err.Error())
	}

	// Validate port
	if t.Payload.Port < 1 || t.Payload.Port > 65535 {
		return fmt.Errorf("invalid port: %d", t.Payload.Port)
	}

	// Validate connection type
	if t.Payload.Type != models.OpcuaServer && t.Payload.Type != models.GenericAsset && t.Payload.Type != models.ExternalMQTT {
		return fmt.Errorf("unsupported connection type: %s", t.Payload.Type)
	}

	return nil
}

// Execute creates an empty dataflowcomponent with name, uuid and connection field
func (t *TestNetworkConnectionAction) Execute() (result interface{}, actionContext map[string]interface{}, err error) {
	if !SendActionReply(t.instanceUUID, t.userEmail, t.actionUUID, models.ActionExecuting, "Creating empty dataflowcomponent...", t.outboundChannel, models.TestNetworkConnection) {
		return nil, nil, fmt.Errorf("error sending action reply")
	}

	// Generate a UUID for the dataflow component
	componentUUID := uuid.New()

	// Create a name for the dataflow component based on the connection type and target
	componentName := fmt.Sprintf("%s-%s-%d", t.Payload.Type, t.Payload.IP, t.Payload.Port)

	// Create the connection details as input configuration
	inputConfig := map[string]interface{}{
		"connection": map[string]interface{}{
			"ip":   t.Payload.IP,
			"port": t.Payload.Port,
			"type": t.Payload.Type,
		},
	}

	// Create the empty dataflow component using the correct DataFlowComponentConfig type
	emptyDfc := config.DataFlowComponentConfig{
		Name:         componentName,
		DesiredState: "stopped", // Default initial state
		VersionUUID:  componentUUID.String(),
		ServiceConfig: benthosserviceconfig.BenthosServiceConfig{
			Input:       inputConfig,
			MetricsPort: 4195,
			LogLevel:    "INFO",
		},
	}

	zap.S().Infof("Created empty dataflowcomponent: %v", emptyDfc)

	// Update the configuration with the new component
	err = t.updateConfigWithNewDFC(emptyDfc)
	if err != nil {
		return nil, nil, fmt.Errorf("error updating configuration: %s", err.Error())
	}

	if !SendActionReply(t.instanceUUID, t.userEmail, t.actionUUID, models.ActionExecuting, "Empty dataflowcomponent created and added to config successfully", t.outboundChannel, models.TestNetworkConnection) {
		return emptyDfc, nil, fmt.Errorf("error sending action reply")
	}

	//TODO: observe the dataflowcomponent and watch if the connection is successful

	return "Successfully created empty dataflowcomponent", nil, nil
}

// updateConfigWithNewDFC gets the current config, adds the new DFC, and writes it back
func (t *TestNetworkConnectionAction) updateConfigWithNewDFC(dfc config.DataFlowComponentConfig) error {
	// Create context for config operations
	ctx := context.Background()

	// Create a config manager
	configManager := config.NewFileConfigManager()

	// Get current configuration
	currentConfig, err := configManager.GetConfig(ctx, 0)
	if err != nil {
		zap.S().Errorf("Failed to get current configuration: %v", err)
		return err
	}

	// Clone the config to avoid modifying the original
	updatedConfig := currentConfig.Clone()

	// Add the new DataFlowComponent to the configuration
	updatedConfig.DataFlowComponents = append(updatedConfig.DataFlowComponents, dfc)

	// Write the updated configuration back to filesystem
	err = configManager.WriteConfig(ctx, updatedConfig)
	if err != nil {
		zap.S().Errorf("Failed to write updated configuration: %v", err)
		return err
	}

	zap.S().Infof("Successfully added new DataFlowComponent to configuration")
	return nil
}
