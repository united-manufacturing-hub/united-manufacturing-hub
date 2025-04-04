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
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type DeployDataFlowComponentAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager
	systemSnapshot  *fsm.SystemSnapshot
	name            string
	payload         models.CDFCPayload
	metaType        string
}

// exposed for testing purposes
func NewDeployDataFlowComponentAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshot *fsm.SystemSnapshot) *DeployDataFlowComponentAction {
	return &DeployDataFlowComponentAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		systemSnapshot:  systemSnapshot,
	}
}

func (a *DeployDataFlowComponentAction) Parse(payload interface{}) error {
	zap.S().Debug("Parsing DeployDataFlowComponent action payload")

	// First parse the top level structure
	type TopLevelPayload struct {
		Name string `json:"name"`
		Meta struct {
			Type string `json:"type"`
		} `json:"meta"`
		IgnoreHealthCheck bool        `json:"ignoreHealthCheck"`
		Payload           interface{} `json:"payload"`
	}

	// Parse the top level payload
	var topLevel TopLevelPayload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	if err := json.Unmarshal(payloadBytes, &topLevel); err != nil {
		return fmt.Errorf("failed to unmarshal top level payload: %v", err)
	}

	a.name = topLevel.Name
	if a.name == "" {
		return errors.New("missing required field Name")
	}

	// Store the meta type
	a.metaType = topLevel.Meta.Type
	if a.metaType == "" {
		return errors.New("missing required field Meta.Type")
	}

	// Handle different component types
	switch a.metaType {
	case "custom":
		// Parse the nested custom data flow component payload
		var customPayloadMap map[string]interface{}
		nestedPayloadBytes, err := json.Marshal(topLevel.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal nested payload: %v", err)
		}

		if err := json.Unmarshal(nestedPayloadBytes, &customPayloadMap); err != nil {
			return fmt.Errorf("failed to unmarshal nested payload: %v", err)
		}

		// Extract the customDataFlowComponent section
		cdfc, ok := customPayloadMap["customDataFlowComponent"]
		if !ok {
			return errors.New("missing customDataFlowComponent in payload")
		}

		// Convert to the expected structure
		cdfcMap, ok := cdfc.(map[string]interface{})
		if !ok {
			return errors.New("customDataFlowComponent is not a valid object")
		}

		// Now map the incoming structure to our expected structure
		var cdfcPayload models.CDFCPayload

		// Handle inputs -> Input
		inputs, ok := cdfcMap["inputs"].(map[string]interface{})
		if !ok {
			return errors.New("missing required field inputs")
		}

		inputType, ok := inputs["type"].(string)
		if !ok || inputType == "" {
			return errors.New("missing required field inputs.type")
		}

		inputData, ok := inputs["data"].(string)
		if !ok || inputData == "" {
			return errors.New("missing required field inputs.data")
		}

		cdfcPayload.Input = models.DfcDataConfig{
			Type: inputType,
			Data: inputData,
		}

		// Handle outputs -> Output
		outputs, ok := cdfcMap["outputs"].(map[string]interface{})
		if !ok {
			return errors.New("missing required field outputs")
		}

		outputType, ok := outputs["type"].(string)
		if !ok || outputType == "" {
			return errors.New("missing required field outputs.type")
		}

		outputData, ok := outputs["data"].(string)
		if !ok || outputData == "" {
			return errors.New("missing required field outputs.data")
		}

		cdfcPayload.Output = models.DfcDataConfig{
			Type: outputType,
			Data: outputData,
		}

		// Handle pipeline
		pipeline, ok := cdfcMap["pipeline"].(map[string]interface{})
		if !ok {
			return errors.New("missing required field pipeline")
		}

		processors, ok := pipeline["processors"].(map[string]interface{})
		if !ok || len(processors) == 0 {
			return errors.New("missing required field pipeline.processors")
		}

		cdfcPayload.Pipeline = make(map[string]models.DfcDataConfig)

		// Process each processor
		for key, proc := range processors {
			processor, ok := proc.(map[string]interface{})
			if !ok {
				return fmt.Errorf("processor %s is not a valid object", key)
			}

			procType, ok := processor["type"].(string)
			if !ok || procType == "" {
				return fmt.Errorf("missing required field pipeline.processors.%s.type", key)
			}

			procData, ok := processor["data"].(string)
			if !ok || procData == "" {
				return fmt.Errorf("missing required field pipeline.processors.%s.data", key)
			}

			cdfcPayload.Pipeline[key] = models.DfcDataConfig{
				Type: procType,
				Data: procData,
			}
		}

		// Validate YAML in Input and Output
		var temp map[string]interface{}
		if err = yaml.Unmarshal([]byte(cdfcPayload.Input.Data), &temp); err != nil {
			return fmt.Errorf("inputs.data is not valid YAML: %v", err)
		}
		if err = yaml.Unmarshal([]byte(cdfcPayload.Output.Data), &temp); err != nil {
			return fmt.Errorf("outputs.data is not valid YAML: %v", err)
		}

		// Validate pipeline processors
		for k, v := range cdfcPayload.Pipeline {
			// Check if processor data is valid YAML
			if err = yaml.Unmarshal([]byte(v.Data), &temp); err != nil {
				return fmt.Errorf("pipeline.processors.%s.data is not valid YAML: %v", k, err)
			}
		}

		a.payload = cdfcPayload
	case "protocolConverter", "dataBridge", "streamProcessor":
		return fmt.Errorf("component type %s not yet supported", a.metaType)
	default:
		return fmt.Errorf("unsupported component type: %s", a.metaType)
	}

	zap.S().Infof("Parsed DeployDataFlowComponent action payload: name=%s, type=%s", a.name, a.metaType)
	return nil
}

func (a *DeployDataFlowComponentAction) Validate() error {
	return nil
}

func (a *DeployDataFlowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	zap.S().Info("Executing DeployDataFlowComponent action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, "Starting DeployDataFlowComponent action", a.outboundChannel, models.DeployDataFlowComponent)

	// convert the action payload to a config.DataFlowComponentConfig
	// Create pipeline as a map with processors array
	pipeline := make(map[string]interface{})
	var processorsArray []interface{}

	// Sort the keys to ensure processors are added in the right order
	var keys []string
	for k := range a.payload.Pipeline {
		keys = append(keys, k)
	}
	// Simple string sort works for numeric keys like "0", "1", etc.
	sort.Strings(keys)

	// Add processors to the array in order
	for _, k := range keys {
		v := a.payload.Pipeline[k]
		// Unmarshal each processor data to a map
		var processorData map[string]interface{}
		err := yaml.Unmarshal([]byte(v.Data), &processorData)
		if err != nil {
			zap.S().Errorf("Error unmarshalling processor data for %s: %v", k, err)
			return nil, nil, fmt.Errorf("failed to parse processor %s: %v", k, err)
		}

		processorsArray = append(processorsArray, processorData)
	}

	// Add the processors array to the pipeline map
	pipeline["processors"] = processorsArray

	// Parse input data
	var inputData map[string]interface{}
	err := yaml.Unmarshal([]byte(a.payload.Input.Data), &inputData)
	if err != nil {
		zap.S().Errorf("Error unmarshalling input data: %v", err)
		return nil, nil, fmt.Errorf("failed to parse input data: %v", err)
	}

	// Parse output data
	var outputData map[string]interface{}
	err = yaml.Unmarshal([]byte(a.payload.Output.Data), &outputData)
	if err != nil {
		zap.S().Errorf("Error unmarshalling output data: %v", err)
		return nil, nil, fmt.Errorf("failed to parse output data: %v", err)
	}

	serviceConfig := benthosserviceconfig.BenthosServiceConfig{
		Pipeline:    pipeline,
		Input:       inputData,
		Output:      outputData,
		MetricsPort: 9090,
		LogLevel:    "debug",
	}
	dfcConfig := config.DataFlowComponentConfig{
		Name:          a.name,
		DesiredState:  "active",
		ServiceConfig: serviceConfig,
	}

	// Add the data flow component to the config
	err = a.configManager.AtomicAddDataFlowComponent(context.Background(), dfcConfig)
	if err != nil {
		return nil, nil, err
	}

	// observe the system snapshot to check if the data flow component is deployed
	err = a.validateDeployment()
	if err != nil {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, "Data flow component not deployed", a.outboundChannel, models.DeployDataFlowComponent)
		return nil, nil, err
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedSuccessfull, "Data flow component deployed successfully", a.outboundChannel, models.DeployDataFlowComponent)

	return nil, nil, nil
}

func (a *DeployDataFlowComponentAction) validateDeployment() error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-ticker.C:
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Validating data flow component deployment...", a.outboundChannel, models.DeployDataFlowComponent)
			// check if the data flow component is deployed
			managers := a.systemSnapshot.Managers
			for _, manager := range managers {
				if manager.GetName() == logger.ComponentDataFlowComponentManager+logger.ComponentCore {
					instances := manager.GetInstances()
					for _, instance := range instances {
						if instance.ID == a.name {
							zap.S().Info("Data flow component deployed")
							//check state
							if instance.CurrentState == "active" {
								return nil
							}
						}
					}
				}
			}
		case <-timeout:
			return fmt.Errorf("data flow component not deployed after 10 seconds")
		}
	}
}

func (a *DeployDataFlowComponentAction) getUserEmail() string {
	return a.userEmail
}

func (a *DeployDataFlowComponentAction) getUuid() uuid.UUID {
	return a.actionUUID
}
