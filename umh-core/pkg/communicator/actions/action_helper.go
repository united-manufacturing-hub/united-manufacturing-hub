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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

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
		return config.DataFlowComponentConfig{}, fmt.Errorf("no observed state found for dataflowcomponent")
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

	//fill the inputs, outputs, pipeline and rawYAML
	// Convert the BenthosConfig input to CommonDataFlowComponentInputConfig
	inputData, err := yaml.Marshal(dfcConfig.BenthosConfig.Input)
	if err != nil {
		log.Warnf("Failed to marshal input data: %v", err)
		return models.CommonDataFlowComponentCDFCPropertiesPayload{}, err
	}
	dfc_payload.CDFCProperties.Inputs = models.CommonDataFlowComponentInputConfig{
		Data: string(inputData),
		Type: "benthos", // Default type for benthos inputs
	}

	// Convert the BenthosConfig output to CommonDataFlowComponentOutputConfig
	outputData, err := yaml.Marshal(dfcConfig.BenthosConfig.Output)
	if err != nil {
		log.Warnf("Failed to marshal output data: %v", err)
		return models.CommonDataFlowComponentCDFCPropertiesPayload{}, err
	}
	dfc_payload.CDFCProperties.Outputs = models.CommonDataFlowComponentOutputConfig{
		Data: string(outputData),
		Type: "benthos", // Default type for benthos outputs
	}

	// Convert the BenthosConfig pipeline to CommonDataFlowComponentPipelineConfig
	processors := models.CommonDataFlowComponentPipelineConfigProcessors{}

	// Extract processors from the pipeline if they exist
	if pipeline, ok := dfcConfig.BenthosConfig.Pipeline["processors"].([]interface{}); ok {
		for i, proc := range pipeline {
			procData, err := yaml.Marshal(proc)
			if err != nil {
				log.Warnf("Failed to marshal processor data: %v", err)
				continue
			}
			// Use index as processor name to allow sorting in the frontend
			procName := fmt.Sprintf("%d", i)
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

// ConsumeOutboundMessages processes messages from the outbound channel
// This method is used for testing purposes to consume messages that would normally be sent to the user
func ConsumeOutboundMessages(outboundChannel chan *models.UMHMessage, messages *[]*models.UMHMessage, logMessages bool) {
	for msg := range outboundChannel {
		*messages = append(*messages, msg)
		decodedMessage, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
		if err != nil {
			zap.S().Error("error decoding message", zap.Error(err))
			continue
		}
		if logMessages {
			zap.S().Info("received message", decodedMessage.Payload)
		}

	}
}

// SendLimitedLogs sends a maximum of 10 logs to the user and a message about remaining logs.
// Returns the updated lastLogs array that includes all logs, even those not sent.
func SendLimitedLogs(
	logs []s6.LogEntry,
	lastLogs []s6.LogEntry,
	instanceUUID uuid.UUID,
	userEmail string,
	actionUUID uuid.UUID,
	outboundChannel chan *models.UMHMessage,
	actionType models.ActionType,
	remainingSeconds int) []s6.LogEntry {

	if len(logs) <= len(lastLogs) {
		return lastLogs
	}

	maxLogsToSend := 10
	logsToSend := logs[len(lastLogs):]
	remainingLogs := len(logsToSend) - maxLogsToSend

	// Send at most maxLogsToSend logs
	end := min(len(logsToSend), maxLogsToSend)

	for _, log := range logsToSend[:end] {
		stateMessage := RemainingPrefixSec(remainingSeconds) + "received log line: " + log.Content
		SendActionReply(instanceUUID, userEmail, actionUUID, models.ActionExecuting,
			stateMessage,
			outboundChannel, actionType)
	}

	// Send message about remaining logs if any
	if remainingLogs > 0 {
		stateMessage := RemainingPrefixSec(remainingSeconds) + fmt.Sprintf("%d remaining logs not displayed", remainingLogs)
		SendActionReply(instanceUUID, userEmail, actionUUID, models.ActionExecuting,
			stateMessage,
			outboundChannel, actionType)
	}

	// Return updated lastLogs to include all logs we've seen, even if not all were sent
	return logs
}

// RemainingPrefixSec formats d (assumed ≤20 s) as "[left: NN s] ".
func RemainingPrefixSec(dSeconds int) string {
	return fmt.Sprintf("[left: %02d s] ", dSeconds) // fixed 15-rune prefix
}

// High-level label for one-off (non-polling) messages.
//
//	action = "deploy", "edit" …
//	name   = human name of the component
//
// → "deploy(foo): "
func Label(action, name string) string {
	return fmt.Sprintf("%s(%s): ", action, name)
}
