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

// Package actions contains shared functionality for parsing dataflow component configurations
// that can be used across different action types (dataflow components and protocol converters).

package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// DataflowComponentTopLevelPayload represents the top-level structure for dataflow component payloads.
type DataflowComponentTopLevelPayload struct {
	Payload interface{} `json:"payload"`
	Name    string      `json:"name"`
	Meta    struct {
		Type string `json:"type"`
	} `json:"meta"`
	State             string `json:"state"`
	IgnoreHealthCheck bool   `json:"ignoreHealthCheck"`
}

// ParseDataflowComponentTopLevel parses the top-level payload structure for dataflow components.
func ParseDataflowComponentTopLevel(payload interface{}) (DataflowComponentTopLevelPayload, error) {
	var topLevel DataflowComponentTopLevelPayload

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return DataflowComponentTopLevelPayload{}, fmt.Errorf("failed to marshal payload: %w", err)
	}

	err = json.Unmarshal(payloadBytes, &topLevel)
	if err != nil {
		return DataflowComponentTopLevelPayload{}, fmt.Errorf("failed to unmarshal top level payload: %w", err)
	}

	// Validate required fields
	if topLevel.Name == "" {
		return DataflowComponentTopLevelPayload{}, errors.New("missing required field Name")
	}

	return topLevel, nil
}

// ParseCustomDataFlowComponent is a helper function that parses the custom dataflow component
// payload structure. It extracts the inputs, outputs, pipeline, and optional inject configurations.
func ParseCustomDataFlowComponent(payload interface{}) (models.CDFCPayload, error) {
	// Parse the nested custom data flow component payload
	var customPayloadMap map[string]interface{}

	nestedPayloadBytes, err := json.Marshal(payload)
	if err != nil {
		return models.CDFCPayload{}, fmt.Errorf("failed to marshal nested payload: %w", err)
	}

	err = json.Unmarshal(nestedPayloadBytes, &customPayloadMap)
	if err != nil {
		return models.CDFCPayload{}, fmt.Errorf("failed to unmarshal nested payload: %w", err)
	}

	// Extract the customDataFlowComponent section
	cdfc, exists := customPayloadMap["customDataFlowComponent"]
	if !exists {
		return models.CDFCPayload{}, errors.New("missing customDataFlowComponent in payload")
	}

	// Convert to the expected structure
	cdfcMap, isValidMap := cdfc.(map[string]interface{})
	if !isValidMap {
		return models.CDFCPayload{}, errors.New("customDataFlowComponent is not a valid object")
	}

	// Only check that required top-level sections exist for parsing
	// Detailed validation will be done in Validate()
	_, hasInputs := cdfcMap["inputs"].(map[string]interface{})
	if !hasInputs {
		return models.CDFCPayload{}, errors.New("missing required field inputs")
	}

	_, hasOutputs := cdfcMap["outputs"].(map[string]interface{})
	if !hasOutputs {
		return models.CDFCPayload{}, errors.New("missing required field outputs")
	}

	// Use ParseActionPayload to convert the raw payload to our struct
	parsedPayload, err := ParseActionPayload[models.CustomDFCPayload](payload)
	if err != nil {
		return models.CDFCPayload{}, fmt.Errorf("failed to parse payload: %w", err)
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
		Inject: cdfcParsed.Inject.Data,
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

// ValidateCustomDataFlowComponentPayload validates the structure and YAML content of a CDFCPayload.
func ValidateCustomDataFlowComponentPayload(payload models.CDFCPayload, validateOutput bool) error {
	// Validate input fields
	if payload.Inputs.Type == "" {
		return errors.New("missing required field inputs.type")
	}

	if payload.Inputs.Data == "" {
		return errors.New("missing required field inputs.data")
	}

	// Validate output fields
	if validateOutput {
		if payload.Outputs.Type == "" {
			return errors.New("missing required field outputs.type")
		}

		if payload.Outputs.Data == "" {
			return errors.New("missing required field outputs.data")
		}
	}

	// Validate pipeline
	if len(payload.Pipeline) == 0 {
		return errors.New("missing required field pipeline.processors")
	}

	// Validate YAML in all components
	var temp map[string]interface{}

	// Validate Input YAML
	err := yaml.Unmarshal([]byte(payload.Inputs.Data), &temp)
	if err != nil {
		return fmt.Errorf("inputs.data is not valid YAML: %w", err)
	}

	// Validate Output YAML
	err = yaml.Unmarshal([]byte(payload.Outputs.Data), &temp)
	if err != nil {
		return fmt.Errorf("outputs.data is not valid YAML: %w", err)
	}

	// Validate pipeline processor YAML and fields
	for key, proc := range payload.Pipeline {
		if proc.Type == "" {
			return fmt.Errorf("missing required field pipeline.processors.%s.type", key)
		}

		if proc.Data == "" {
			return fmt.Errorf("missing required field pipeline.processors.%s.data", key)
		}

		// Check processor YAML
		err := yaml.Unmarshal([]byte(proc.Data), &temp)
		if err != nil {
			return fmt.Errorf("pipeline.processors.%s.data is not valid YAML: %w", key, err)
		}
	}

	// Validate inject data
	if payload.Inject != "" {
		err := yaml.Unmarshal([]byte(payload.Inject), &temp)
		if err != nil {
			return fmt.Errorf("inject.data is not valid YAML: %w", err)
		}
	}

	return nil
}

// CreateBenthosConfigFromCDFCPayload converts a CDFCPayload into a normalized BenthosConfig.
func CreateBenthosConfigFromCDFCPayload(payload models.CDFCPayload, componentName string) (dataflowcomponentserviceconfig.BenthosConfig, error) {
	// Parse YAML configurations
	benthosInput := make(map[string]interface{})
	benthosOutput := make(map[string]interface{})
	benthosYamlInject := make(map[string]interface{})

	// Parse input data
	err := yaml.Unmarshal([]byte(payload.Inputs.Data), &benthosInput)
	if err != nil {
		return dataflowcomponentserviceconfig.BenthosConfig{}, fmt.Errorf("failed to parse input data: %s", err.Error())
	}

	// Parse output data
	err = yaml.Unmarshal([]byte(payload.Outputs.Data), &benthosOutput)
	if err != nil {
		return dataflowcomponentserviceconfig.BenthosConfig{}, fmt.Errorf("failed to parse output data: %s", err.Error())
	}

	// Parse inject data
	err = yaml.Unmarshal([]byte(payload.Inject), &benthosYamlInject)
	if err != nil {
		return dataflowcomponentserviceconfig.BenthosConfig{}, fmt.Errorf("failed to parse inject data: %s", err.Error())
	}

	// Parse cache resources, rate limit resources and buffer from the inject data
	cacheResources, exists := benthosYamlInject["cache_resources"].([]interface{})
	if !exists {
		cacheResources = []interface{}{}
	}

	rateLimitResources, exists := benthosYamlInject["rate_limit_resources"].([]interface{})
	if !exists {
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
			return dataflowcomponentserviceconfig.BenthosConfig{}, fmt.Errorf("cache resource %d is not a valid object", i)
		}

		benthosCacheResources[i] = resourceMap
	}

	benthosRateLimitResources := make([]map[string]interface{}, len(rateLimitResources))
	for i, resource := range rateLimitResources {
		resourceMap, ok := resource.(map[string]interface{})
		if !ok {
			return dataflowcomponentserviceconfig.BenthosConfig{}, fmt.Errorf("rate limit resource %d is not a valid object", i)
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

	if len(payload.Pipeline) > 0 {
		// Convert each processor configuration in the pipeline
		processors := []interface{}{}

		// Check if we have numeric keys (0, 1, 2, ...) and use them to preserve order
		hasNumericKeys := CheckIfOrderedNumericKeys(payload.Pipeline)

		if hasNumericKeys {
			// Process in numeric order
			for i := range len(payload.Pipeline) {
				processorName := strconv.Itoa(i)

				processor := payload.Pipeline[processorName]

				var procConfig map[string]interface{}

				err := yaml.Unmarshal([]byte(processor.Data), &procConfig)
				if err != nil {
					return dataflowcomponentserviceconfig.BenthosConfig{}, fmt.Errorf("failed to parse pipeline processor %s: %s", processorName, err.Error())
				}

				// Add processor to the list
				processors = append(processors, procConfig)
			}
		}

		if !hasNumericKeys {
			return dataflowcomponentserviceconfig.BenthosConfig{}, errors.New("at least one processor with a non-numerous key was found")
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

	// Return as DataflowComponentServiceConfig BenthosConfig
	return dataflowcomponentserviceconfig.BenthosConfig{
		Input:              normalizedConfig.Input,
		Pipeline:           normalizedConfig.Pipeline,
		Output:             normalizedConfig.Output,
		CacheResources:     normalizedConfig.CacheResources,
		RateLimitResources: normalizedConfig.RateLimitResources,
		Buffer:             normalizedConfig.Buffer,
	}, nil
}

// CreateDataFlowComponentConfig creates a DataFlowComponentConfig from a normalized BenthosConfig.
func CreateDataFlowComponentConfig(name string, state string, benthosConfig dataflowcomponentserviceconfig.BenthosConfig) config.DataFlowComponentConfig {
	return config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: state,
		},
		DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: benthosConfig,
		},
	}
}

// BuildDataFlowComponentDataFromSnapshot converts the *observed* FSM snapshot
// into a `config.DataFlowComponentConfig`.  This function can be used by both
// get-dataflowcomponent and get-protocol-converter actions.
//
// The function extracts the configuration data from a dataflow component FSM instance
// and builds a complete DataFlowComponentConfig structure that can be used to construct
// API responses.
func BuildDataFlowComponentDataFromSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.SugaredLogger) (config.DataFlowComponentConfig, error) {
	dfcData := config.DataFlowComponentConfig{}

	log.Infow("Building dataflowcomponent data from snapshot", "instanceID", instance.ID)

	if instance.LastObservedState != nil {
		// Try to cast to the right type
		observedState, ok := instance.LastObservedState.(*dataflowcomponent.DataflowComponentObservedStateSnapshot)
		if !ok {
			log.Errorw("Observed state is of unexpected type", "instanceID", instance.ID)

			return config.DataFlowComponentConfig{}, fmt.Errorf("invalid observed state type for dataflowcomponent %s", instance.ID)
		}

		dfcData.DataFlowComponentServiceConfig = observedState.Config
		dfcData.Name = instance.ID
		dfcData.DesiredFSMState = instance.DesiredState
	} else {
		log.Warnw("No observed state found for dataflowcomponent", "instanceID", instance.ID)

		return config.DataFlowComponentConfig{}, errors.New("no observed state found for dataflowcomponent")
	}

	return dfcData, nil
}

// BuildCommonDataFlowComponentPropertiesFromConfig converts a DataFlowComponentServiceConfig
// into the CommonDataFlowComponentCDFCPropertiesPayload format expected by the API.
// This function can be used by both get-dataflowcomponent and get-protocol-converter actions.
//
// The function extracts inputs, outputs, pipeline, and rawYAML from the Benthos configuration
// and builds the complete CDFC properties structure that can be used to construct API responses.
func BuildCommonDataFlowComponentPropertiesFromConfig(dfcConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig, log *zap.SugaredLogger) (models.CommonDataFlowComponentCDFCPropertiesPayload, error) {
	// build the payload
	dfc_payload := models.CommonDataFlowComponentCDFCPropertiesPayload{}
	tagValue := "" // the benthos image tag is not used anymore in UMH Core
	dfc_payload.CDFCProperties.BenthosImageTag = &models.CommonDataFlowComponentBenthosImageTagConfig{
		Tag: &tagValue,
	}
	dfc_payload.CDFCProperties.IgnoreErrors = nil

	// fill the inputs, outputs, pipeline and rawYAML
	// Convert the BenthosConfig input to CommonDataFlowComponentInputConfig
	inputData, err := yaml.Marshal(dfcConfig.BenthosConfig.Input)
	if err != nil {
		log.Warnf("Failed to marshal input data: %v", err)

		return models.CommonDataFlowComponentCDFCPropertiesPayload{}, err
	}

	// Determine input type by looking at the input structure
	inputType := "benthos" // Default type
	for key := range dfcConfig.BenthosConfig.Input {
		inputType = key
		if inputType == "input" {
			if innerMap, ok := dfcConfig.BenthosConfig.Input["input"].(map[string]interface{}); ok {
				for key := range innerMap {
					inputType = key

					break
				}
			}
		}

		break
	}

	dfc_payload.CDFCProperties.Inputs = models.CommonDataFlowComponentInputConfig{
		Data: string(inputData),
		Type: inputType,
	}

	// Convert the BenthosConfig output to CommonDataFlowComponentOutputConfig
	outputData, err := yaml.Marshal(dfcConfig.BenthosConfig.Output)
	if err != nil {
		log.Warnf("Failed to marshal output data: %v", err)

		return models.CommonDataFlowComponentCDFCPropertiesPayload{}, err
	}

	// Determine output type by looking at the output structure
	outputType := "benthos" // Default type
	for key := range dfcConfig.BenthosConfig.Output {
		outputType = key

		break
	}

	dfc_payload.CDFCProperties.Outputs = models.CommonDataFlowComponentOutputConfig{
		Data: string(outputData),
		Type: outputType,
	}

	// Convert the BenthosConfig pipeline to CommonDataFlowComponentPipelineConfig
	processors := models.CommonDataFlowComponentPipelineConfigProcessors{}

	// Extract processors from the pipeline if they exist
	if pipeline, hasPipeline := dfcConfig.BenthosConfig.Pipeline["processors"].([]interface{}); hasPipeline {
		for procIndex, proc := range pipeline {
			procData, err := yaml.Marshal(proc)
			if err != nil {
				log.Warnf("Failed to marshal processor data: %v", err)

				continue
			}

			// Determine processor type by looking at the processor structure
			processorType := "bloblang" // Default type

			if procMap, isValidProcessor := proc.(map[string]interface{}); isValidProcessor {
				// Get the first key in the processor map as the type
				for key := range procMap {
					processorType = key

					break
				}
			}

			// Use index as processor name to allow sorting in the frontend
			procName := strconv.Itoa(procIndex)
			processors[procName] = struct {
				Data string `json:"data" mapstructure:"data" yaml:"data"`
				Type string `json:"type" mapstructure:"type" yaml:"type"`
			}{
				Data: string(procData),
				Type: processorType,
			}
		}
	}

	// Set threads value if present in the pipeline
	var threads *int
	if threadsVal, ok := dfcConfig.BenthosConfig.Pipeline["threads"]; ok {
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
	if len(dfcConfig.BenthosConfig.CacheResources) > 0 {
		rawYAMLMap["cache_resources"] = dfcConfig.BenthosConfig.CacheResources
	}

	// Add rate limit resources if present
	if len(dfcConfig.BenthosConfig.RateLimitResources) > 0 {
		rawYAMLMap["rate_limit_resources"] = dfcConfig.BenthosConfig.RateLimitResources
	}

	// Add buffer if present
	if len(dfcConfig.BenthosConfig.Buffer) > 0 {
		rawYAMLMap["buffer"] = dfcConfig.BenthosConfig.Buffer
	}

	// Only create rawYAML if we have any data
	if len(rawYAMLMap) > 0 {
		rawYAMLData, err := yaml.Marshal(rawYAMLMap)
		if err != nil {
			log.Warnf("Failed to marshal rawYAML data: %v", err)
		} else {
			dfc_payload.CDFCProperties.RawYAML = &models.CommonDataFlowComponentRawYamlConfig{
				Data: string(rawYAMLData),
			}
		}
	}

	return dfc_payload, nil
}

// ValidateComponentName validates that a component name contains only valid characters
// and is not empty. Valid characters are letters (a-z, A-Z), numbers (0-9), and hyphens (-).
func ValidateComponentName(name string) error {
	if name == "" {
		return errors.New("name cannot be empty")
	}

	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' {
			return errors.New("name can only contain letters (a-z, A-Z) and numbers (0-9) and hyphens (-)")
		}
	}

	return nil
}

func ValidateDataFlowComponentState(state string) error {
	if state != dataflowcomponent.OperationalStateStopped && state != dataflowcomponent.OperationalStateActive {
		return fmt.Errorf("invalid state: %s", state)
	}

	return nil
}
