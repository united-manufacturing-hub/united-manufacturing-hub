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
	"fmt"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type GetDataFlowComponentAction struct {
	userEmail       string
	actionUUID      uuid.UUID
	instanceUUID    uuid.UUID
	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager
	systemSnapshot  *fsm.SystemSnapshot
	payload         models.GetDataflowcomponentRequestSchemaJson
	actionLogger    *zap.SugaredLogger
}

func (a *GetDataFlowComponentAction) Parse(payload interface{}) (err error) {
	a.actionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetDataflowcomponentRequestSchemaJson](payload)
	a.actionLogger.Info("Payload parsed, uuids: ", a.payload.VersionUUIDs)
	return err
}

// validation step is empty here
func (a *GetDataFlowComponentAction) Validate() error {
	return nil
}

func buildDataFlowComponentDataFromSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.SugaredLogger) (config.DataFlowComponentConfig, error) {
	dfcData := config.DataFlowComponentConfig{}

	log.Info("Building dataflowcomponent data from snapshot", zap.String("instanceID", instance.ID))

	if instance.LastObservedState != nil {
		// Try to cast to the right type
		observedState := instance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
		dfcData.DataFlowComponentConfig = observedState.Config
		dfcData.FSMInstanceConfig.Name = instance.ID
		dfcData.FSMInstanceConfig.DesiredFSMState = instance.DesiredState

	} else {
		log.Warn("No observed state found for dataflowcomponent", zap.String("instanceID", instance.ID))
		return config.DataFlowComponentConfig{}, fmt.Errorf("no observed state found for dataflowcomponent")
	}

	return dfcData, nil
}

func (a *GetDataFlowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing the action")
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, "getting the dataflowcomponent", a.outboundChannel, models.GetDataFlowComponent)

	dataFlowComponents := []config.DataFlowComponentConfig{}
	// Get the DataFlowComponent
	a.actionLogger.Info("Getting the DataFlowComponent")

	// Get the config
	// a.actionLogger.Info("Getting the config")
	// ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	// defer cancel()
	// curConfig, err := a.configManager.GetConfig(ctx, a.systemSnapshot.Tick)
	// if err != nil {
	// 	return nil, nil, err
	// }
	// for _, component := range curConfig.DataFlow {
	// 	cur_uuid := dataflowcomponentconfig.GenerateUUIDFromName(component.Name).String()
	// 	a.actionLogger.Info("Checking if ", cur_uuid, " is in ", a.payload.VersionUUIDs)
	// 	if slices.Contains(a.payload.VersionUUIDs, cur_uuid) {
	// 		a.actionLogger.Info("Adding ", component.Name, " to the response")
	// 		dataFlowComponents = append(dataFlowComponents, component)
	// 	}
	// }

	if dataflowcomponentManager, exists := a.systemSnapshot.Managers[constants.DataflowcomponentManagerName]; exists {
		a.actionLogger.Info("Dataflowcomponent manager found, getting the dataflowcomponent")
		instances := dataflowcomponentManager.GetInstances()
		for _, instance := range instances {
			dfc, err := buildDataFlowComponentDataFromSnapshot(instance, a.actionLogger)
			if err != nil {
				a.actionLogger.Warnf("Failed to build dataflowcomponent data: %v", err)
				continue
			}
			dataFlowComponents = append(dataFlowComponents, dfc)
		}
	}

	// build the response
	a.actionLogger.Info("Building the response")
	response := models.GetDataflowcomponentResponse{}
	for _, component := range dataFlowComponents {
		// build the payload
		dfc_payload := models.CommonDataFlowComponentCDFCPropertiesPayload{}
		tagValue := "not-used"
		dfc_payload.CDFCProperties.BenthosImageTag = &models.CommonDataFlowComponentBenthosImageTagConfig{
			Tag: &tagValue,
		}
		dfc_payload.CDFCProperties.IgnoreErrors = nil
		//fill the inputs, outputs, pipeline and rawYAML
		// Convert the BenthosConfig input to CommonDataFlowComponentInputConfig
		inputData, err := yaml.Marshal(component.DataFlowComponentConfig.BenthosConfig.Input)
		if err != nil {
			a.actionLogger.Warnf("Failed to marshal input data: %v", err)
		}
		dfc_payload.CDFCProperties.Inputs = models.CommonDataFlowComponentInputConfig{
			Data: string(inputData),
			Type: "benthos", // Default type for benthos inputs
		}

		// Convert the BenthosConfig output to CommonDataFlowComponentOutputConfig
		outputData, err := yaml.Marshal(component.DataFlowComponentConfig.BenthosConfig.Output)
		if err != nil {
			a.actionLogger.Warnf("Failed to marshal output data: %v", err)
		}
		dfc_payload.CDFCProperties.Outputs = models.CommonDataFlowComponentOutputConfig{
			Data: string(outputData),
			Type: "benthos", // Default type for benthos outputs
		}

		// Convert the BenthosConfig pipeline to CommonDataFlowComponentPipelineConfig
		processors := models.CommonDataFlowComponentPipelineConfigProcessors{}

		// Extract processors from the pipeline if they exist
		if pipeline, ok := component.DataFlowComponentConfig.BenthosConfig.Pipeline["processors"].([]interface{}); ok {
			for i, proc := range pipeline {
				procData, err := yaml.Marshal(proc)
				if err != nil {
					a.actionLogger.Warnf("Failed to marshal processor data: %v", err)
					continue
				}
				// Use index as processor name if not specified
				procName := fmt.Sprintf("processor_%d", i)
				processors[procName] = struct {
					Data string `json:"data" yaml:"data" mapstructure:"data"`
					Type string `json:"type" yaml:"type" mapstructure:"type"`
				}{
					Data: string(procData),
					Type: "bloblang", // Default type for benthos processors
				}
			}
		}

		// Set threads value if present in the pipeline
		var threads *int
		if threadsVal, ok := component.DataFlowComponentConfig.BenthosConfig.Pipeline["threads"]; ok {
			if t, ok := threadsVal.(int); ok {
				threads = &t
			}
		}

		dfc_payload.CDFCProperties.Pipeline = models.CommonDataFlowComponentPipelineConfig{
			Processors: processors,
			Threads:    threads,
		}

		// Create RawYAML from the cache_resources, rate_limit_resources, and buffer
		rawYAMLMap := map[string]interface{}{}

		// Add cache resources if present
		if len(component.DataFlowComponentConfig.BenthosConfig.CacheResources) > 0 {
			rawYAMLMap["cache_resources"] = component.DataFlowComponentConfig.BenthosConfig.CacheResources
		}

		// Add rate limit resources if present
		if len(component.DataFlowComponentConfig.BenthosConfig.RateLimitResources) > 0 {
			rawYAMLMap["rate_limit_resources"] = component.DataFlowComponentConfig.BenthosConfig.RateLimitResources
		}

		// Add buffer if present
		if len(component.DataFlowComponentConfig.BenthosConfig.Buffer) > 0 {
			rawYAMLMap["buffer"] = component.DataFlowComponentConfig.BenthosConfig.Buffer
		}

		// Only create rawYAML if we have any data
		if len(rawYAMLMap) > 0 {
			rawYAMLData, err := yaml.Marshal(rawYAMLMap)
			if err != nil {
				a.actionLogger.Warnf("Failed to marshal rawYAML data: %v", err)
			} else {
				dfc_payload.CDFCProperties.RawYAML = &models.CommonDataFlowComponentRawYamlConfig{
					Data: string(rawYAMLData),
				}
			}
		}

		response[dataflowcomponentconfig.GenerateUUIDFromName(component.FSMInstanceConfig.Name).String()] = models.GetDataflowcomponentResponseContent{
			CreationTime: 0,
			Creator:      "",
			Meta: models.CommonDataFlowComponentMeta{
				Type: "custom",
			},
			Name:      component.FSMInstanceConfig.Name,
			ParentDFC: nil,
			Payload:   dfc_payload,
		}
	}

	// Send the success message
	//SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedSuccessfull, response, a.outboundChannel, models.GetDataFlowComponent)

	a.actionLogger.Info("Response built, returning, response: ", response)
	return response, nil, nil
}

func (a *GetDataFlowComponentAction) getUserEmail() string {
	return a.userEmail
}

func (a *GetDataFlowComponentAction) getUuid() uuid.UUID {
	return a.actionUUID
}
