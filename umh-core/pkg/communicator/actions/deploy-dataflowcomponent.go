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

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type DeployDataflowComponentAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager
	systemSnapshot  *fsm.SystemSnapshot
	payload         models.CDFCPayload
	name            string
	metaType        string
	actionLogger    *zap.SugaredLogger
}

// exposed for testing purposed
func NewDeployDataflowComponentAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *DeployDataflowComponentAction {
	return &DeployDataflowComponentAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicatorActions),
	}
}

func (a *DeployDataflowComponentAction) Parse(payload interface{}) error {
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
		payload, err := parseCustomDataFlowComponent(topLevel.Payload)
		if err != nil {
			return err
		}
		a.payload = payload
	case "protocolConverter", "dataBridge", "streamProcessor":
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, "component type not supported", a.outboundChannel, models.DeployDataFlowComponent)
		return fmt.Errorf("component type %s not yet supported", a.metaType)
	default:
		return fmt.Errorf("unsupported component type: %s", a.metaType)
	}

	a.actionLogger.Debugf("Parsed DeployDataFlowComponent action payload: name=%s, type=%s", a.name, a.metaType)
	return nil
}

func (a *DeployDataflowComponentAction) Validate() error {
	// no validation needed anymore because here, only parsing problem can happen
	// and they are caught in the Parse()
	return nil
}

func parseCustomDataFlowComponent(payload interface{}) (models.CDFCPayload, error) {
	// Parse the nested custom data flow component payload
	var customPayloadMap map[string]interface{}
	nestedPayloadBytes, err := json.Marshal(payload)
	if err != nil {
		return models.CDFCPayload{}, fmt.Errorf("failed to marshal nested payload: %v", err)
	}

	if err := json.Unmarshal(nestedPayloadBytes, &customPayloadMap); err != nil {
		return models.CDFCPayload{}, fmt.Errorf("failed to unmarshal nested payload: %v", err)
	}

	// Extract the customDataFlowComponent section
	cdfc, ok := customPayloadMap["customDataFlowComponent"]
	if !ok {
		return models.CDFCPayload{}, errors.New("missing customDataFlowComponent in payload")
	}

	// Convert to the expected structure
	cdfcMap, ok := cdfc.(map[string]interface{})
	if !ok {
		return models.CDFCPayload{}, errors.New("customDataFlowComponent is not a valid object")
	}

	// Now map the incoming structure to our expected structure
	var cdfcPayload models.CDFCPayload

	// Handle inputs -> Input
	inputs, ok := cdfcMap["inputs"].(map[string]interface{})
	if !ok {
		return models.CDFCPayload{}, errors.New("missing required field inputs")
	}

	inputType, ok := inputs["type"].(string)
	if !ok || inputType == "" {
		return models.CDFCPayload{}, errors.New("missing required field inputs.type")
	}

	inputData, ok := inputs["data"].(string)
	if !ok || inputData == "" {
		return models.CDFCPayload{}, errors.New("missing required field inputs.data")
	}

	cdfcPayload.Input = models.DfcDataConfig{
		Type: inputType,
		Data: inputData,
	}

	// Handle outputs -> Output
	outputs, ok := cdfcMap["outputs"].(map[string]interface{})
	if !ok {
		return models.CDFCPayload{}, errors.New("missing required field outputs")
	}

	outputType, ok := outputs["type"].(string)
	if !ok || outputType == "" {
		return models.CDFCPayload{}, errors.New("missing required field outputs.type")
	}

	outputData, ok := outputs["data"].(string)
	if !ok || outputData == "" {
		return models.CDFCPayload{}, errors.New("missing required field outputs.data")
	}

	cdfcPayload.Output = models.DfcDataConfig{
		Type: outputType,
		Data: outputData,
	}

	// Handle inject data if present
	inject, ok := cdfcMap["inject"].(map[string]interface{})
	if ok {
		injectType, ok := inject["type"].(string)
		if !ok || injectType == "" {
			return models.CDFCPayload{}, errors.New("missing required field inject.type")
		}

		injectData, ok := inject["data"].(string)
		if !ok || injectData == "" {
			return models.CDFCPayload{}, errors.New("missing required field inject.data")
		}

		cdfcPayload.Inject = models.DfcDataConfig{
			Type: injectType,
			Data: injectData,
		}

		// Validate YAML in inject data
		var temp map[string]interface{}
		if err = yaml.Unmarshal([]byte(cdfcPayload.Inject.Data), &temp); err != nil {
			return models.CDFCPayload{}, fmt.Errorf("inject.data is not valid YAML: %v", err)
		}
	}

	// Handle pipeline
	pipeline, ok := cdfcMap["pipeline"].(map[string]interface{})
	if !ok {
		return models.CDFCPayload{}, errors.New("missing required field pipeline")
	}

	processors, ok := pipeline["processors"].(map[string]interface{})
	if !ok || len(processors) == 0 {
		return models.CDFCPayload{}, errors.New("missing required field pipeline.processors")
	}

	cdfcPayload.Pipeline = make(map[string]models.DfcDataConfig)

	// Process each processor
	for key, proc := range processors {
		processor, ok := proc.(map[string]interface{})
		if !ok {
			return models.CDFCPayload{}, fmt.Errorf("processor %s is not a valid object", key)
		}

		procType, ok := processor["type"].(string)
		if !ok || procType == "" {
			return models.CDFCPayload{}, fmt.Errorf("missing required field pipeline.processors.%s.type", key)
		}

		procData, ok := processor["data"].(string)
		if !ok || procData == "" {
			return models.CDFCPayload{}, fmt.Errorf("missing required field pipeline.processors.%s.data", key)
		}

		cdfcPayload.Pipeline[key] = models.DfcDataConfig{
			Type: procType,
			Data: procData,
		}
	}

	// Validate YAML in Input and Output
	var temp map[string]interface{}
	if err = yaml.Unmarshal([]byte(cdfcPayload.Input.Data), &temp); err != nil {
		return models.CDFCPayload{}, fmt.Errorf("inputs.data is not valid YAML: %v", err)
	}
	if err = yaml.Unmarshal([]byte(cdfcPayload.Output.Data), &temp); err != nil {
		return models.CDFCPayload{}, fmt.Errorf("outputs.data is not valid YAML: %v", err)
	}

	// Validate pipeline processors
	for k, v := range cdfcPayload.Pipeline {
		// Check if processor data is valid YAML
		if err = yaml.Unmarshal([]byte(v.Data), &temp); err != nil {
			return models.CDFCPayload{}, fmt.Errorf("pipeline.processors.%s.data is not valid YAML: %v", k, err)
		}
	}

	return cdfcPayload, nil
}

func (a *DeployDataflowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeployDataflowComponent action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, "Starting DeployDataflowComponent", a.outboundChannel, models.DeployDataFlowComponent)

	// Parse the input and output configurations
	benthosInput := make(map[string]interface{})
	benthosOutput := make(map[string]interface{})
	benthosYamlInject := make(map[string]interface{})

	// First try to use the Input data
	err := yaml.Unmarshal([]byte(a.payload.Input.Data), &benthosInput)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to parse input data: %s", err.Error())
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.DeployDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errMsg)
	}

	//parse the output data
	err = yaml.Unmarshal([]byte(a.payload.Output.Data), &benthosOutput)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to parse output data: %s", err.Error())
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.DeployDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errMsg)
	}

	//parse the inject data
	err = yaml.Unmarshal([]byte(a.payload.Inject.Data), &benthosYamlInject)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to parse inject data: %s", err.Error())
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.DeployDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errMsg)
	}

	//parse the cache resources, rate limit resources and buffer from the inject data
	cacheResources, ok := benthosYamlInject["cache_resources"].([]interface{})
	if !ok {
		cacheResources = []interface{}{}
	}

	rateLimitResources, ok := benthosYamlInject["rate_limit_resources"].([]interface{})
	if !ok {
		rateLimitResources = []interface{}{}
	}

	buffer, ok := benthosYamlInject["buffer"].(map[string]interface{})
	if !ok {
		buffer = map[string]interface{}{}
	}

	benthosCacheResources := make([]map[string]interface{}, len(cacheResources))
	for i, resource := range cacheResources {
		resourceMap, ok := resource.(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("cache resource %d is not a valid object", i)
		}
		benthosCacheResources[i] = resourceMap
	}

	benthosRateLimitResources := make([]map[string]interface{}, len(rateLimitResources))
	for i, resource := range rateLimitResources {
		resourceMap, ok := resource.(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("rate limit resource %d is not a valid object", i)
		}
		benthosRateLimitResources[i] = resourceMap
	}

	benthosBuffer := make(map[string]interface{})
	for key, value := range buffer {
		benthosBuffer[key] = value
	}

	// Convert pipeline data to Benthos pipeline configuration
	benthosPipeline := map[string]interface{}{
		"processors": []interface{}{},
	}

	if len(a.payload.Pipeline) > 0 {
		// Convert each processor configuration in the pipeline
		processors := []interface{}{}

		for processorName, processor := range a.payload.Pipeline {
			var procConfig map[string]interface{}
			err := yaml.Unmarshal([]byte(processor.Data), &procConfig)
			if err != nil {
				errMsg := fmt.Sprintf("Failed to parse pipeline processor %s: %s", processorName, err.Error())
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.DeployDataFlowComponent)
				return nil, nil, fmt.Errorf("%s", errMsg)
			}

			// Add processor to the list
			processors = append(processors, procConfig)
		}

		benthosPipeline["processors"] = processors
	}

	// Create the Benthos service config
	benthosConfig := benthosserviceconfig.BenthosServiceConfig{
		Input:              benthosInput,
		Output:             benthosOutput,
		Pipeline:           benthosPipeline,
		CacheResources:     benthosCacheResources,
		RateLimitResources: benthosRateLimitResources,
		Buffer:             benthosBuffer,
	}

	// Normalize the config
	normalizedConfig := benthosserviceconfig.NormalizeBenthosConfig(benthosConfig)

	// Create the DataFlowComponentConfig
	dfc := config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            a.name,
			DesiredFSMState: "running",
		},
		DataFlowComponentConfig: dataflowcomponentconfig.DataFlowComponentConfig{
			BenthosConfig: dataflowcomponentconfig.BenthosConfig{
				Input:              normalizedConfig.Input,
				Pipeline:           normalizedConfig.Pipeline,
				Output:             normalizedConfig.Output,
				CacheResources:     normalizedConfig.CacheResources,
				RateLimitResources: normalizedConfig.RateLimitResources,
				Buffer:             normalizedConfig.Buffer,
			},
		},
	}

	// Update the location in the configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()
	err = a.configManager.AtomicAddDataflowcomponent(ctx, dfc)
	if err != nil {
		errorMsg := fmt.Sprintf("failed to add dataflowcomponent: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.DeployDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Send success reply
	successMsg := fmt.Sprintf("Successfully deployed data flow component: %s", a.name)
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedSuccessfull, successMsg, a.outboundChannel, models.DeployDataFlowComponent)

	return nil, nil, nil
}

func (a *DeployDataflowComponentAction) getUserEmail() string {
	return a.userEmail
}

func (a *DeployDataflowComponentAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// exposed for testing purposes
func (a *DeployDataflowComponentAction) GetParsedPayload() models.CDFCPayload {
	return a.payload
}
