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
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// EditDataflowComponentAction implements the Action interface for editing
// existing dataflow components in the UMH instance.
type EditDataflowComponentAction struct {
	userEmail         string
	actionUUID        uuid.UUID
	instanceUUID      uuid.UUID
	outboundChannel   chan *models.UMHMessage
	configManager     config.ConfigManager
	payload           models.CDFCPayload
	name              string
	metaType          string
	oldComponentUUID  uuid.UUID
	newComponentUUID  uuid.UUID
	systemSnapshot    *fsm.SystemSnapshot
	ignoreHealthCheck bool
	actionLogger      *zap.SugaredLogger
	dfc               config.DataFlowComponentConfig
	oldConfig         config.DataFlowComponentConfig
	systemMu          *sync.RWMutex
}

// NewEditDataflowComponentAction creates a new EditDataflowComponentAction with the provided parameters.
// This constructor is primarily used for testing to enable dependency injection, though it can be used
// in production code as well. It initializes the action with the necessary fields but doesn't
// populate the payload fields which must be done via Parse.
func NewEditDataflowComponentAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshot *fsm.SystemSnapshot, systemMu *sync.RWMutex) *EditDataflowComponentAction {
	return &EditDataflowComponentAction{
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
// It handles the top-level structure parsing first to extract name, UUID, component type, and other fields,
// then delegates to specialized parsing functions based on the component type.
//
// Currently supported types:
// - "custom": Custom dataflow components with Benthos configuration
//
// The function returns appropriate errors for missing required fields or unsupported component types.
func (a *EditDataflowComponentAction) Parse(payload interface{}) error {
	//First parse the top level structure
	type TopLevelPayload struct {
		Name string `json:"name"`
		Meta struct {
			Type string `json:"type"`
		} `json:"meta"`
		Payload           interface{} `json:"payload"`
		UUID              string      `json:"uuid"`
		IgnoreHealthCheck bool        `json:"ignoreHealthCheck"`
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

	//set the new component UUID by the name
	a.newComponentUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(a.name)

	a.oldComponentUUID, err = uuid.Parse(topLevel.UUID)
	if err != nil {
		return fmt.Errorf("invalid UUID format: %v", err)
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
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, "component type not supported", a.outboundChannel, models.EditDataFlowComponent)
		return fmt.Errorf("component type %s not yet supported", a.metaType)
	default:
		return fmt.Errorf("unsupported component type: %s", a.metaType)
	}

	a.actionLogger.Debugf("Parsed EditDataFlowComponent action payload: name=%s, type=%s, UUID=%s", a.name, a.metaType, a.oldComponentUUID)
	return nil
}

// Validate implements the Action interface by performing deeper validation of the parsed payload.
// For custom dataflow components, it validates:
// 1. Required fields exist (name, metaType, UUID, input/output configuration, pipeline)
// 2. All YAML content is valid by attempting to parse it
//
// The function returns detailed error messages for any validation failures, indicating
// exactly which field or YAML section is invalid.
func (a *EditDataflowComponentAction) Validate() error {
	// Validate UUID was properly parsed
	if a.oldComponentUUID == uuid.Nil {
		return errors.New("component UUID is missing or invalid")
	}

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

// Execute implements the Action interface by performing the actual editing of the dataflow component.
// It follows the standard pattern for actions:
// 1. Sends ActionConfirmed to indicate the action is starting
// 2. Parses and normalizes all the configuration data
// 3. Creates a DataFlowComponentConfig and updates it in the system configuration
// 4. Sends ActionFinishedWithFailure if any error occurs
// 5. Returns a success message (not sending ActionFinishedSuccessfull as that's done by the caller)
//
// The function handles custom dataflow components by:
// - Converting YAML strings into structured configuration
// - Normalizing the Benthos configuration
// - Updating the component in the configuration with a desired state of "active"
func (a *EditDataflowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing EditDataflowComponent action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, "Starting edit of dataflow component: "+a.name, a.outboundChannel, models.EditDataFlowComponent)

	// Parse the input and output configurations
	benthosInput := make(map[string]interface{})
	benthosOutput := make(map[string]interface{})
	benthosYamlInject := make(map[string]interface{})

	// First try to use the Input data
	err := yaml.Unmarshal([]byte(a.payload.Inputs.Data), &benthosInput)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to parse input data: %s", err.Error())
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.EditDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errMsg)
	}

	//parse the output data
	err = yaml.Unmarshal([]byte(a.payload.Outputs.Data), &benthosOutput)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to parse output data: %s", err.Error())
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.EditDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errMsg)
	}

	//parse the inject data
	if a.payload.Inject.Data != "" {
		err = yaml.Unmarshal([]byte(a.payload.Inject.Data), &benthosYamlInject)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to parse inject data: %s", err.Error())
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.EditDataFlowComponent)
			return nil, nil, fmt.Errorf("%s", errMsg)
		}
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
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.EditDataFlowComponent)
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

	a.dfc = dfc

	// Update the component in the configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Updating dataflow component '"+a.name+"' configuration...", a.outboundChannel, models.EditDataFlowComponent)
	a.oldConfig, err = a.configManager.AtomicEditDataflowcomponent(ctx, a.oldComponentUUID, dfc)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to edit dataflow component: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.EditDataFlowComponent)
		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// check against observedState as well
	if a.systemSnapshot != nil { // skipping this for the unit tests
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Configuration updated. Waiting for dataflow component '"+a.name+"' to become active...", a.outboundChannel, models.EditDataFlowComponent)
		err = a.waitForComponentToBeActive()
		if err != nil {
			errorMsg := fmt.Sprintf("Failed to wait for dataflow component to be active: %v", err)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.EditDataFlowComponent)
			return nil, nil, fmt.Errorf("%s", errorMsg)
		}
	}

	// return success message, but do not send it as this is done by the caller
	successMsg := fmt.Sprintf("Successfully edited dataflow component: %s", a.name)

	return successMsg, nil, nil
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *EditDataflowComponentAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *EditDataflowComponentAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed CDFCPayload - exposed primarily for testing purposes.
func (a *EditDataflowComponentAction) GetParsedPayload() models.CDFCPayload {
	return a.payload
}

// GetComponentUUID returns the UUID of the component being edited - exposed primarily for testing purposes.
func (a *EditDataflowComponentAction) GetComponentUUID() uuid.UUID {
	return a.oldComponentUUID
}

// GetSystemSnapshot returns a deep copy of the system snapshot to avoid the caller having to handle locking
func (a *EditDataflowComponentAction) GetSystemSnapshot() fsm.SystemSnapshot {
	a.systemMu.RLock()
	defer a.systemMu.RUnlock()

	if a.systemSnapshot == nil {
		return fsm.SystemSnapshot{}
	}

	var snapshotCopy fsm.SystemSnapshot
	err := deepcopy.Copy(&snapshotCopy, a.systemSnapshot)
	if err != nil {
		sentry.ReportIssue(err, sentry.IssueTypeError, a.actionLogger)
	}

	return snapshotCopy
}

func (a *EditDataflowComponentAction) waitForComponentToBeActive() error {
	// checks the system snapshot
	// 1. waits for the component to appear in the system snapshot (relevant for changed name)
	// 2. waits for the component to be active
	// 3. waits for the component to have the correct config
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout)
	startTime := time.Now()
	timeoutDuration := constants.DataflowComponentWaitForActiveTimeout

	for {
		select {
		case <-timeout:
			if !a.ignoreHealthCheck {
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Timeout reached. Dataflow component did not become active in time. Rolling back to previous configuration...", a.outboundChannel, models.EditDataFlowComponent)
				ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
				defer cancel()
				_, err := a.configManager.AtomicEditDataflowcomponent(ctx, a.oldComponentUUID, a.oldConfig)
				if err != nil {
					a.actionLogger.Errorf("failed to roll back dataflowcomponent %s: %v", a.name, err)
				}
				return fmt.Errorf("dataflowcomponent %s was not active in time and was rolled back to the old config", a.name)
			}
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Timeout reached. Dataflow component did not become active in time. Consider removing it or checking logs for errors.", a.outboundChannel, models.EditDataFlowComponent)
			return nil
		case <-ticker.C:
			elapsed := time.Since(startTime)
			remaining := timeoutDuration - elapsed
			remainingSeconds := int(remaining.Seconds())

			systemSnapshot := a.GetSystemSnapshot()
			if dataflowcomponentManager, exists := systemSnapshot.Managers[constants.DataflowcomponentManagerName]; exists {
				instances := dataflowcomponentManager.GetInstances()
				found := false
				for _, instance := range instances {
					if dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID) == a.newComponentUUID {
						found = true
						dfcSnapshot, ok := instance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
						if !ok {
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
								fmt.Sprintf("Waiting for dataflow component state info (%ds remaining)...", remainingSeconds), a.outboundChannel, models.EditDataFlowComponent)
							continue
						}

						if instance.CurrentState != "active" {
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
								fmt.Sprintf("Dataflow component is in state '%s' (waiting for 'active', %ds remaining)...",
									instance.CurrentState, remainingSeconds), a.outboundChannel, models.EditDataFlowComponent)
							continue
						} else {
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "Dataflow component is active.", a.outboundChannel, models.EditDataFlowComponent)
						}
						// check if the config is correct
						if !dataflowcomponentserviceconfig.NewComparator().ConfigsEqual(&dfcSnapshot.Config, &a.dfc.DataFlowComponentServiceConfig) {
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
								fmt.Sprintf("Dataflow component is active but config changes haven't applied yet (%ds remaining)...",
									remainingSeconds), a.outboundChannel, models.EditDataFlowComponent)
						} else {
							SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
								"Dataflow component is active with correct configuration. Edit complete.", a.outboundChannel, models.EditDataFlowComponent)
							return nil
						}
					}
				}
				if !found {
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						fmt.Sprintf("Waiting for dataflow component to appear in system (%ds remaining)...",
							remainingSeconds), a.outboundChannel, models.EditDataFlowComponent)
				}
			} else {
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					fmt.Sprintf("Waiting for dataflow component manager (%ds remaining)...",
						remainingSeconds), a.outboundChannel, models.EditDataFlowComponent)
			}
		}
	}
}
