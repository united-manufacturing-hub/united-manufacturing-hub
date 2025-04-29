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
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// DeployDataflowComponentAction implements the Action interface for deploying
// dataflow components to the UMH instance.
type DeployDataflowComponentAction struct {
	userEmail         string
	actionUUID        uuid.UUID
	instanceUUID      uuid.UUID
	outboundChannel   chan *models.UMHMessage
	configManager     config.ConfigManager
	systemSnapshot    *fsm.SystemSnapshot
	payload           models.CDFCPayload
	name              string
	metaType          string
	actionLogger      *zap.SugaredLogger
	ignoreHealthCheck bool
	systemMu          *sync.RWMutex
}

// NewDeployDataflowComponentAction creates a new DeployDataflowComponentAction with the provided parameters.
// This constructor is primarily used for testing to enable dependency injection, though it can be used
// in production code as well. It initializes the action with the necessary fields but doesn't
// populate the payload, name, or metaType fields which must be done via Parse.
func NewDeployDataflowComponentAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshot *fsm.SystemSnapshot, systemMu *sync.RWMutex) *DeployDataflowComponentAction {
	return &DeployDataflowComponentAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
		systemSnapshot:  systemSnapshot,
		systemMu:        systemMu,
	}
}

// Parse implements the Action interface by extracting dataflow component configuration from the payload.
// It handles the top-level structure parsing first to extract name and component type,
// then delegates to specialized parsing functions based on the component type.
//
// Currently supported types:
// - "custom": Custom dataflow components with Benthos configuration
//
// The function returns appropriate errors for missing required fields or unsupported component types.
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

	a.ignoreHealthCheck = topLevel.IgnoreHealthCheck

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

// parseCustomDataFlowComponent is a helper function that parses the custom dataflow component
// payload structure. It extracts the inputs, outputs, pipeline, and optional inject configurations.
//
// The function performs structure validation to ensure required sections exist, but delegates
// detailed validation to the Validate method.
func parseCustomDataFlowComponent(payload interface{}) (models.CDFCPayload, error) {
	// Define our intermediate struct to parse the nested payload

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

	// Only check that required top-level sections exist for parsing
	// Detailed validation will be done in Validate()
	_, ok = cdfcMap["inputs"].(map[string]interface{})
	if !ok {
		return models.CDFCPayload{}, errors.New("missing required field inputs")
	}

	_, ok = cdfcMap["outputs"].(map[string]interface{})
	if !ok {
		return models.CDFCPayload{}, errors.New("missing required field outputs")
	}

	// Use ParseActionPayload to convert the raw payload to our struct
	parsedPayload, err := ParseActionPayload[models.CustomDFCPayload](payload)
	if err != nil {
		return models.CDFCPayload{}, fmt.Errorf("failed to parse payload: %v", err)
	}

	cdfcParsed := parsedPayload.CustomDataFlowComponent

	// Create our return model
	cdfcPayload := models.CDFCPayload{
		Inputs: models.DfcDataConfig{
			Type: cdfcParsed.Inputs.Type,
			Data: cdfcParsed.Inputs.Data,
		},
		Outputs: models.DfcDataConfig{
			Type: cdfcParsed.Outputs.Type,
			Data: cdfcParsed.Outputs.Data,
		},
	}

	// Add inject data if present
	if cdfcParsed.Inject.Type != "" && cdfcParsed.Inject.Data != "" {
		cdfcPayload.Inject = models.DfcDataConfig{
			Type: cdfcParsed.Inject.Type,
			Data: cdfcParsed.Inject.Data,
		}
	}

	// Process the pipeline processors
	cdfcPayload.Pipeline = make(map[string]models.DfcDataConfig)
	for key, proc := range cdfcParsed.Pipeline.Processors {
		cdfcPayload.Pipeline[key] = models.DfcDataConfig{
			Type: proc.Type,
			Data: proc.Data,
		}
	}

	return cdfcPayload, nil
}

// Validate implements the Action interface by performing deeper validation of the parsed payload.
// For custom dataflow components, it validates:
// 1. Required fields exist (name, metaType, input/output configuration, pipeline)
// 2. All YAML content is valid by attempting to parse it
//
// The function returns detailed error messages for any validation failures, indicating
// exactly which field or YAML section is invalid.
func (a *DeployDataflowComponentAction) Validate() error {
	// Validate name and metatype were properly parsed
	if a.name == "" {
		return errors.New("missing required field Name")
	}

	if a.metaType == "" {
		return errors.New("missing required field Meta.Type")
	}

	// For custom type, validate the payload structure
	if a.metaType == "custom" {
		// Validate input fields
		if a.payload.Inputs.Type == "" {
			return errors.New("missing required field inputs.type")
		}
		if a.payload.Inputs.Data == "" {
			return errors.New("missing required field inputs.data")
		}

		// Validate output fields
		if a.payload.Outputs.Type == "" {
			return errors.New("missing required field outputs.type")
		}
		if a.payload.Outputs.Data == "" {
			return errors.New("missing required field outputs.data")
		}

		// Validate pipeline
		if len(a.payload.Pipeline) == 0 {
			return errors.New("missing required field pipeline.processors")
		}

		// Validate YAML in all components
		var temp map[string]interface{}

		// Validate Input YAML
		if err := yaml.Unmarshal([]byte(a.payload.Inputs.Data), &temp); err != nil {
			return fmt.Errorf("inputs.data is not valid YAML: %v", err)
		}

		// Validate Output YAML
		if err := yaml.Unmarshal([]byte(a.payload.Outputs.Data), &temp); err != nil {
			return fmt.Errorf("outputs.data is not valid YAML: %v", err)
		}

		// Validate pipeline processor YAML and fields
		for key, proc := range a.payload.Pipeline {
			if proc.Type == "" {
				return fmt.Errorf("missing required field pipeline.processors.%s.type", key)
			}
			if proc.Data == "" {
				return fmt.Errorf("missing required field pipeline.processors.%s.data", key)
			}

			// Check processor YAML
			if err := yaml.Unmarshal([]byte(proc.Data), &temp); err != nil {
				return fmt.Errorf("pipeline.processors.%s.data is not valid YAML: %v", key, err)
			}
		}

		// Validate inject data
		if a.payload.Inject.Type != "" && a.payload.Inject.Data != "" {
			if err := yaml.Unmarshal([]byte(a.payload.Inject.Data), &temp); err != nil {
				return fmt.Errorf("inject.data is not valid YAML: %v", err)
			}
		}
	}

	return nil
}

// Execute implements the Action interface by performing the actual deployment of the dataflow component.
// It follows the standard pattern for actions:
// 1. Sends ActionConfirmed to indicate the action is starting
// 2. Parses and normalizes all the configuration data
// 3. Creates a DataFlowComponentConfig and adds it to the system configuration
// 4. Sends ActionFinishedWithFailure if any error occurs
// 5. Returns a success message (not sending ActionFinishedSuccessfull as that's done by the caller)
//
// The function handles custom dataflow components by:
// - Converting YAML strings into structured configuration
// - Normalizing the Benthos configuration
// - Adding the component to the configuration with a desired state of "active"
func (a *DeployDataflowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeployDataflowComponent action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, "Starting deployment of dataflow component: "+a.name, a.outboundChannel, models.DeployDataFlowComponent)

	// Parse the input and output configurations
	benthosInput := make(map[string]interface{})
	benthosOutput := make(map[string]interface{})
	benthosYamlInject := make(map[string]interface{})

	// First try to use the Input data
	err := yaml.Unmarshal([]byte(a.payload.Inputs.Data), &benthosInput)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to parse input data: %s", err.Error())
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.DeployDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errMsg)
	}

	//parse the output data
	err = yaml.Unmarshal([]byte(a.payload.Outputs.Data), &benthosOutput)
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
			DesiredFSMState: "active",
		},
		DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
				Input:              normalizedConfig.Input,
				Pipeline:           normalizedConfig.Pipeline,
				Output:             normalizedConfig.Output,
				CacheResources:     normalizedConfig.CacheResources,
				RateLimitResources: normalizedConfig.RateLimitResources,
				Buffer:             normalizedConfig.Buffer,
			},
		},
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Adding dataflow component '"+a.name+"' to configuration...", a.outboundChannel, models.DeployDataFlowComponent)
	// Update the location in the configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()
	err = a.configManager.AtomicAddDataflowcomponent(ctx, dfc)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to add dataflow component: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.DeployDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// check against observedState as well
	if a.systemSnapshot != nil { // skipping this for the unit tests
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Configuration updated. Waiting for dataflow component '"+a.name+"' to start and become active...", a.outboundChannel, models.DeployDataFlowComponent)
		err = a.waitForComponentToBeActive()
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to wait for dataflow component to be active: %v", err)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.DeployDataFlowComponent)
			return nil, nil, fmt.Errorf("%s", errorMsg)
		}
	}

	// return success message, but do not send it as this is done by the caller
	successMsg := fmt.Sprintf("Successfully deployed dataflow component: %s", a.name)

	return successMsg, nil, nil
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *DeployDataflowComponentAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *DeployDataflowComponentAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed CDFCPayload - exposed primarily for testing purposes.
func (a *DeployDataflowComponentAction) GetParsedPayload() models.CDFCPayload {
	return a.payload
}

func (a *DeployDataflowComponentAction) waitForComponentToBeActive() error {
	// checks the system snapshot
	// 1. waits for the instance to appear in the system snapshot
	// 2. takes the logs of the instance and sends them to the user in 1-second intervals
	// 3. waits for the instance to be in state "active"
	// 4. takes the residual logs of the instance and sends them to the user
	// 5. returns nil

	// we use those two variables below to store the incoming logs and send them to the user
	// logs is always updated with all existing logs
	// lastLogs is updated with the logs that have been sent to the user
	// this way we avoid sending the same log twice
	var logs []s6.LogEntry
	var lastLogs []s6.LogEntry

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout)
	startTime := time.Now()
	timeoutDuration := constants.DataflowComponentWaitForActiveTimeout
	for {
		select {
		case <-timeout:
			if !a.ignoreHealthCheck {
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					"Timeout reached. Dataflow component did not become active in time. Removing component...",
					a.outboundChannel, models.DeployDataFlowComponent)
				ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
				defer cancel()
				err := a.configManager.AtomicDeleteDataflowcomponent(ctx, dataflowcomponentserviceconfig.GenerateUUIDFromName(a.name))
				if err != nil {
					a.actionLogger.Errorf("failed to remove dataflowcomponent %s: %v", a.name, err)
				}
				return fmt.Errorf("dataflow component '%s' was removed because it did not become active within the timeout period", a.name)
			}
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
				"Timeout reached. Dataflow component did not become active in time. You may want to check logs and remove it if needed.",
				a.outboundChannel, models.DeployDataFlowComponent)
			return nil
		case <-ticker.C:
			elapsed := time.Since(startTime)
			remaining := timeoutDuration - elapsed
			remainingSeconds := int(remaining.Seconds())

			if dataflowcomponentManager, exists := a.systemSnapshot.Managers[constants.DataflowcomponentManagerName]; exists {
				instances := dataflowcomponentManager.GetInstances()
				found := false
				for _, instance := range instances {
					// cast the instance LastObservedState to a dataflowcomponent instance
					curName := instance.ID
					if curName != a.name {
						continue
					}
					found = true
					dfcSnapshot, ok := instance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
					if !ok {
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
							fmt.Sprintf("Waiting for dataflow component state information (%ds remaining)...",
								remainingSeconds), a.outboundChannel, models.DeployDataFlowComponent)
						continue
					}
					if instance.CurrentState == "active" {
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
							"Dataflow component is now active! Deployment complete.",
							a.outboundChannel, models.DeployDataFlowComponent)
						return nil
					} else {
						stateMsg := fmt.Sprintf("Dataflow component is in state '%s' (waiting for 'active', %ds remaining)...",
							instance.CurrentState, remainingSeconds)
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
							stateMsg, a.outboundChannel, models.DeployDataFlowComponent)
						// send the benthos logs to the user
						logs = dfcSnapshot.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs
						// only send the logs that have not been sent yet
						if len(logs) > len(lastLogs) {
							for _, log := range logs[len(lastLogs):] {
								logTime := log.Timestamp.Format("15:04:05")
								SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
									fmt.Sprintf("[%s] %s", logTime, log.Content),
									a.outboundChannel, models.DeployDataFlowComponent)
							}
							lastLogs = logs
						}
					}
				}
				if !found {
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						fmt.Sprintf("Waiting for dataflow component to appear in system (%ds remaining)...",
							remainingSeconds), a.outboundChannel, models.DeployDataFlowComponent)
				}

			} else {
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					fmt.Sprintf("Waiting for dataflow component manager to initialize (%ds remaining)...",
						remainingSeconds), a.outboundChannel, models.DeployDataFlowComponent)
			}
		}
	}

}
