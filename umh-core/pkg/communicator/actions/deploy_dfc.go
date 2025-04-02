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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	benthosserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflow"
)

// deployDFCConfigManager extends config.ConfigManager with WriteConfig method
type deployDFCConfigManager interface {
	config.ConfigManager
	WriteConfig(ctx context.Context, config config.FullConfig) error
}

// DeployDFCAction implements the Action interface for deploying a data flow component
type DeployDFCAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	payload         *AddDataflowcomponentRequestPayload
	configManager   deployDFCConfigManager
}

// NewDeployDFCAction creates a new DeployDFCAction with default config manager
func NewDeployDFCAction(userEmail string, actionUUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage) *DeployDFCAction {
	return &DeployDFCAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   config.NewFileConfigManager(),
	}
}

// WithConfigManager allows setting a custom config manager for testing
func (d *DeployDFCAction) WithConfigManager(configManager deployDFCConfigManager) *DeployDFCAction {
	d.configManager = configManager
	return d
}

// AddDataflowcomponentRequestPayload represents the payload for adding a data flow component
type AddDataflowcomponentRequestPayload struct {
	// IgnoreHealthCheck flag to ignore health check
	IgnoreHealthCheck bool `json:"ignoreHealthCheck,omitempty"`
	// Meta contains metadata about the component
	Meta struct {
		// Type of the component (e.g., "custom", "data-bridge", "protocol-converter", "stream-processor")
		Type string `json:"type"`
		// AdditionalMetadata contains additional key-value pairs
		AdditionalMetadata map[string]interface{} `json:"additionalMetadata,omitempty"`
	} `json:"meta"`
	// Name of the component
	Name string `json:"name"`
	// Payload contains the specific configuration for the component type
	Payload map[string]interface{} `json:"payload"`
}

// Parse implements the Action interface
func (d *DeployDFCAction) Parse(payload interface{}) error {
	zap.S().Debug("Parsing DeployDFCAction payload")

	// Initialize the payload struct
	d.payload = &AddDataflowcomponentRequestPayload{}

	// Convert the payload to a struct using json marshaling/unmarshaling
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	err = json.Unmarshal(payloadBytes, d.payload)
	if err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return nil
}

// Validate implements the Action interface
func (d *DeployDFCAction) Validate() error {
	// Check if payload is present
	if d.payload == nil {
		return errors.New("payload is required")
	}

	// Validate required fields
	if d.payload.Name == "" {
		return errors.New("name is required")
	}

	if d.payload.Meta.Type == "" {
		return errors.New("meta.type is required")
	}

	// Validate payload based on component type
	switch d.payload.Meta.Type {
	case "custom", "data-bridge", "protocol-converter", "stream-processor":
		// Valid component types
	default:
		return fmt.Errorf("invalid component type: %s", d.payload.Meta.Type)
	}

	return nil
}

// Execute implements the Action interface
func (d *DeployDFCAction) Execute() (interface{}, map[string]interface{}, error) {
	zap.S().Info("Executing DeployDFCAction")

	// Send confirmation that action is starting
	SendActionReplyWithAdditionalContext(d.instanceUUID, d.userEmail, d.actionUUID, models.ActionConfirmed, "Starting deployment of data flow component", d.outboundChannel, models.DeployDataFlowComponent, nil)

	// Send progress update
	SendActionReplyWithAdditionalContext(d.instanceUUID, d.userEmail, d.actionUUID, models.ActionExecuting, fmt.Sprintf("Deploying data flow component: %s", d.payload.Name), d.outboundChannel, models.DeployDataFlowComponent, nil)

	// Create a context with timeout for config operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get the current configuration
	currentConfig, err := d.configManager.GetConfig(ctx, 0)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to get current configuration: %s", err)
		SendActionReplyWithAdditionalContext(d.instanceUUID, d.userEmail, d.actionUUID, models.ActionFinishedWithFailure, errorMsg, d.outboundChannel, models.DeployDataFlowComponent, nil)
		return nil, nil, fmt.Errorf("failed to get current configuration: %w", err)
	}

	// Create a DFC configuration from the payload
	dfcConfig, err := d.createDFCConfig()
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to create data flow component configuration: %s", err)
		SendActionReplyWithAdditionalContext(d.instanceUUID, d.userEmail, d.actionUUID, models.ActionFinishedWithFailure, errorMsg, d.outboundChannel, models.DeployDataFlowComponent, nil)
		return nil, nil, fmt.Errorf("failed to create data flow component configuration: %w", err)
	}

	// Add the component to Benthos services in the configuration
	// Create a new BenthosConfig instance for the data flow component
	newBenthosConfig := config.BenthosConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            dfcConfig.Name,
			DesiredFSMState: dfcConfig.DesiredState,
		},
		BenthosServiceConfig: dfcConfig.ServiceConfig,
	}

	// Add the new BenthosConfig to the FullConfig
	currentConfig.Benthos = append(currentConfig.Benthos, newBenthosConfig)

	// Write the updated configuration back to disk
	err = d.configManager.WriteConfig(ctx, currentConfig)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to write updated configuration: %s", err)
		SendActionReplyWithAdditionalContext(d.instanceUUID, d.userEmail, d.actionUUID, models.ActionFinishedWithFailure, errorMsg, d.outboundChannel, models.DeployDataFlowComponent, nil)
		return nil, nil, fmt.Errorf("failed to write updated configuration: %w", err)
	}

	// Success message
	message := fmt.Sprintf("Successfully deployed data flow component: %s", d.payload.Name)
	metadata := map[string]interface{}{
		"name": d.payload.Name,
		"uuid": dfcConfig.VersionUUID,
	}

	// Send the success message
	SendActionReplyWithAdditionalContext(d.instanceUUID, d.userEmail, d.actionUUID, models.ActionFinishedSuccessfull, "Data flow component deployment completed", d.outboundChannel, models.DeployDataFlowComponent, metadata)

	return message, metadata, nil
}

// createDFCConfig creates a DataFlowComponentConfig from the payload
func (d *DeployDFCAction) createDFCConfig() (*dataflow.DataFlowComponentConfig, error) {
	// Generate a UUID for the version if not already specified
	versionUUID := uuid.New().String()

	// Create the basic DFC config
	dfcConfig := &dataflow.DataFlowComponentConfig{
		Name:         d.payload.Name,
		DesiredState: "active", // Default to active state
		VersionUUID:  versionUUID,
	}

	// Process component-specific configuration based on type
	switch d.payload.Meta.Type {
	case "custom":
		// For custom type, extract the Benthos service config directly
		serviceConfigBytes, err := json.Marshal(d.payload.Payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal custom component payload: %w", err)
		}

		err = json.Unmarshal(serviceConfigBytes, &dfcConfig.ServiceConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal custom component payload: %w", err)
		}

	case "data-bridge":
		// Extract bridge-specific configuration
		var bridgeConfig struct {
			Bridge struct {
				InputType        string `json:"inputType"`
				OutputType       string `json:"outputType"`
				DataContractName string `json:"dataContractName"`
			} `json:"bridge"`
		}

		bridgeConfigBytes, err := json.Marshal(d.payload.Payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal bridge config: %w", err)
		}

		err = json.Unmarshal(bridgeConfigBytes, &bridgeConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal bridge config: %w", err)
		}

		// Create a simple bridge configuration
		dfcConfig.ServiceConfig = benthosserviceconfig.BenthosServiceConfig{
			Input: map[string]interface{}{
				bridgeConfig.Bridge.InputType: map[string]interface{}{},
			},
			Output: map[string]interface{}{
				bridgeConfig.Bridge.OutputType: map[string]interface{}{},
			},
		}

	case "protocol-converter":
		// Extract protocol converter specific configuration
		var pcConfig struct {
			ProtocolConverter struct {
				Inputs struct {
					Type string `json:"type"`
					Data string `json:"data"`
				} `json:"inputs"`
				Pipeline struct {
					Processors map[string]struct {
						Type string `json:"type"`
						Data string `json:"data"`
					} `json:"processors"`
				} `json:"pipeline"`
			} `json:"protocolConverter"`
		}

		pcConfigBytes, err := json.Marshal(d.payload.Payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal protocol converter config: %w", err)
		}

		err = json.Unmarshal(pcConfigBytes, &pcConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal protocol converter config: %w", err)
		}

		// Create a protocol converter configuration
		dfcConfig.ServiceConfig = benthosserviceconfig.BenthosServiceConfig{
			Input: map[string]interface{}{
				pcConfig.ProtocolConverter.Inputs.Type: map[string]interface{}{},
			},
			Pipeline: map[string]interface{}{
				"processors": []map[string]interface{}{},
			},
		}

	case "stream-processor":
		// Extract stream processor specific configuration
		var spConfig struct {
			StreamProcessor struct {
				// Add specific fields if needed
			} `json:"streamProcessor"`
		}

		spConfigBytes, err := json.Marshal(d.payload.Payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal stream processor config: %w", err)
		}

		err = json.Unmarshal(spConfigBytes, &spConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal stream processor config: %w", err)
		}

		// Create a stream processor configuration
		dfcConfig.ServiceConfig = benthosserviceconfig.BenthosServiceConfig{
			// Configure appropriate defaults for stream processor
		}

	default:
		return nil, fmt.Errorf("unsupported component type: %s", d.payload.Meta.Type)
	}

	return dfcConfig, nil
}

// getUserEmail implements the Action interface
func (d *DeployDFCAction) getUserEmail() string {
	return d.userEmail
}

// getUuid implements the Action interface
func (d *DeployDFCAction) getUuid() uuid.UUID {
	return d.actionUUID
}
