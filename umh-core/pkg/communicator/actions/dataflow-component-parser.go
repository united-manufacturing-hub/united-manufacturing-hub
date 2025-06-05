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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"gopkg.in/yaml.v3"
)

// DataflowComponentTopLevelPayload represents the top-level structure for dataflow component payloads
type DataflowComponentTopLevelPayload struct {
	Name string `json:"name"`
	Meta struct {
		Type string `json:"type"`
	} `json:"meta"`
	IgnoreHealthCheck bool        `json:"ignoreHealthCheck"`
	Payload           interface{} `json:"payload"`
}

// ParseDataflowComponentTopLevel parses the top-level payload structure for dataflow components
func ParseDataflowComponentTopLevel(payload interface{}) (DataflowComponentTopLevelPayload, error) {
	var topLevel DataflowComponentTopLevelPayload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return DataflowComponentTopLevelPayload{}, fmt.Errorf("failed to marshal payload: %v", err)
	}

	if err := json.Unmarshal(payloadBytes, &topLevel); err != nil {
		return DataflowComponentTopLevelPayload{}, fmt.Errorf("failed to unmarshal top level payload: %v", err)
	}

	// Validate required fields
	if topLevel.Name == "" {
		return DataflowComponentTopLevelPayload{}, errors.New("missing required field Name")
	}

	if topLevel.Meta.Type == "" {
		return DataflowComponentTopLevelPayload{}, errors.New("missing required field Meta.Type")
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

// ValidateCustomDataFlowComponentPayload validates the structure and YAML content of a CDFCPayload
func ValidateCustomDataFlowComponentPayload(payload models.CDFCPayload) error {
	// Validate input fields
	if payload.Inputs.Type == "" {
		return errors.New("missing required field inputs.type")
	}
	if payload.Inputs.Data == "" {
		return errors.New("missing required field inputs.data")
	}

	// Validate output fields
	if payload.Outputs.Type == "" {
		return errors.New("missing required field outputs.type")
	}
	if payload.Outputs.Data == "" {
		return errors.New("missing required field outputs.data")
	}

	// Validate pipeline
	if len(payload.Pipeline) == 0 {
		return errors.New("missing required field pipeline.processors")
	}

	// Validate YAML in all components
	var temp map[string]interface{}

	// Validate Input YAML
	if err := yaml.Unmarshal([]byte(payload.Inputs.Data), &temp); err != nil {
		return fmt.Errorf("inputs.data is not valid YAML: %v", err)
	}

	// Validate Output YAML
	if err := yaml.Unmarshal([]byte(payload.Outputs.Data), &temp); err != nil {
		return fmt.Errorf("outputs.data is not valid YAML: %v", err)
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
		if err := yaml.Unmarshal([]byte(proc.Data), &temp); err != nil {
			return fmt.Errorf("pipeline.processors.%s.data is not valid YAML: %v", key, err)
		}
	}

	// Validate inject data
	if payload.Inject != "" {
		if err := yaml.Unmarshal([]byte(payload.Inject), &temp); err != nil {
			return fmt.Errorf("inject.data is not valid YAML: %v", err)
		}
	}

	return nil
}

// CreateBenthosConfigFromCDFCPayload converts a CDFCPayload into a normalized BenthosConfig
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
				processorName := fmt.Sprintf("%d", i)

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
			return dataflowcomponentserviceconfig.BenthosConfig{}, fmt.Errorf("at least one processor with a non-numerous key was found")
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

// CreateDataFlowComponentConfig creates a DataFlowComponentConfig from a normalized BenthosConfig
func CreateDataFlowComponentConfig(name string, benthosConfig dataflowcomponentserviceconfig.BenthosConfig) config.DataFlowComponentConfig {
	return config.DataFlowComponentConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            name,
			DesiredFSMState: "active",
		},
		DataFlowComponentServiceConfig: dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
			BenthosConfig: benthosConfig,
		},
	}
}
