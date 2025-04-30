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

// Package actions contains *imperative* building-blocks executed by the UMH API
// server.  Unlike the *edit* and *delete* actions, **GetDataFlowComponent** is
// **read-only** – it aggregates configuration *and* runtime state for one or
// more Data-Flow Components (DFCs) so that the frontend can render a
// human-friendly representation.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
//   - A **version UUID** is the deterministic ID derived from a DFC name via
//     `dataflowcomponentserviceconfig.GenerateUUIDFromName`.  The frontend knows
//     only these UUIDs when requesting component details.
//   - The action therefore needs to translate UUID → runtime instance name →
//     observed Benthos configuration → API response schema.
//   - All information is fetched from a *single* snapshot of the FSM runtime so
//     the result is **self-consistent** even while the system keeps running.
//
// -----------------------------------------------------------------------------
// HIGH-LEVEL FLOW
// -----------------------------------------------------------------------------
//  1. **Parse** – store the list of requested UUIDs (no heavy work here).
//  2. **Validate** – no-op because Parse already guarantees structural
//     correctness.
//  3. **Execute**
//     a. Copy the shared `*fsm.SystemSnapshot` under the read-lock.
//     b. Iterate over all live DFC instances and pick those whose deterministic
//     UUID appears in the request.
//     c. Convert each Benthos config into the *public* UMH API schema.
//     d. Send progress messages whenever partial data is returned or an
//     instance is missing.
//     e. Return the assembled `models.GetDataflowcomponentResponse` object.
//
// -----------------------------------------------------------------------------

package actions

import (
	"fmt"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
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

// GetDataFlowComponentAction returns metadata and Benthos configuration for the
// requested list of DFC **version UUIDs**.  The action never blocks the FSM
// writer goroutine – instead it holds the read‑lock only while making a deep
// copy of the snapshot.
//
// All fields are immutable after Parse so that callers can safely pass the
// struct between goroutines when needed.
// ----------------------------------------------------------------------------

type GetDataFlowComponentAction struct {
	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager // currently unused but kept for symmetry

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshot *fsm.SystemSnapshot
	systemMu       *sync.RWMutex

	// ─── Parsed request payload ─────────────────────────────────────────────
	payload models.GetDataflowcomponentRequestSchemaJson

	// ─── Utilities ──────────────────────────────────────────────────────────
	actionLogger *zap.SugaredLogger
}

// NewGetDataFlowComponentAction creates a new GetDataFlowComponentAction with the provided parameters.
// This constructor is primarily used for testing to enable dependency injection, though it can be used
// in production code as well. It initializes the action with the necessary fields but doesn't
// populate the payload field which must be done via Parse.
func NewGetDataFlowComponentAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshot *fsm.SystemSnapshot, systemMu *sync.RWMutex) *GetDataFlowComponentAction {
	return &GetDataFlowComponentAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		systemSnapshot:  systemSnapshot,
		actionLogger:    logger.For(logger.ComponentCommunicator),
		systemMu:        systemMu,
	}
}

// Parse stores the list of version UUIDs we should resolve.  The heavy lifting
// happens later in Execute.
func (a *GetDataFlowComponentAction) Parse(payload interface{}) (err error) {
	a.actionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetDataflowcomponentRequestSchemaJson](payload)
	a.actionLogger.Info("Payload parsed, uuids: ", a.payload.VersionUUIDs)
	return err
}

// Validate is a no‑op because the request schema does not require additional
// semantic checks beyond JSON deserialization.
func (a *GetDataFlowComponentAction) Validate() error {
	return nil
}

// buildDataFlowComponentDataFromSnapshot converts the *observed* FSM snapshot
// into a `config.DataFlowComponentConfig`.  The helper lives outside the action
// struct to keep Execute shorter.
func buildDataFlowComponentDataFromSnapshot(instance fsm.FSMInstanceSnapshot, log *zap.SugaredLogger) (config.DataFlowComponentConfig, error) {
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

// GetSystemSnapshot returns a deep copy of the system snapshot to avoid the caller having to handle locking
func (a *GetDataFlowComponentAction) GetSystemSnapshot() fsm.SystemSnapshot {
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

// Execute aggregates Benthos configs for the requested UUIDs and converts them
// into the public API schema defined in `models.GetDataflowcomponentResponse`.
func (a *GetDataFlowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing the action")
	numUUIDs := len(a.payload.VersionUUIDs)

	dataFlowComponents := []config.DataFlowComponentConfig{}
	// Get the DataFlowComponent
	a.actionLogger.Debugf("Getting the DataFlowComponent")

	// ─── 1  Take a consistent snapshot of the runtime ───────────────────────
	systemSnapshot := a.GetSystemSnapshot()
	if dataflowcomponentManager, exists := systemSnapshot.Managers[constants.DataflowcomponentManagerName]; exists {
		a.actionLogger.Debugf("Dataflowcomponent manager found, getting the dataflowcomponent")
		instances := dataflowcomponentManager.GetInstances()
		foundComponents := 0
		for _, instance := range instances {
			currentUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID).String()
			if slices.Contains(a.payload.VersionUUIDs, currentUUID) {
				a.actionLogger.Debugf("Adding %s to the response", instance.ID)
				dfc, err := buildDataFlowComponentDataFromSnapshot(*instance, a.actionLogger)
				if err != nil {
					a.actionLogger.Warnf("Failed to build dataflowcomponent data: %v", err)
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						fmt.Sprintf("Warning: Failed to retrieve data for component '%s': %v",
							instance.ID, err), a.outboundChannel, models.GetDataFlowComponent)
					continue
				}
				dataFlowComponents = append(dataFlowComponents, dfc)
				foundComponents++
			}

		}
		if foundComponents < numUUIDs {
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
				fmt.Sprintf("Found %d of %d requested components. Some components might not exist in the system.",
					foundComponents, numUUIDs), a.outboundChannel, models.GetDataFlowComponent)
		} else {
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
				"Dataflow component manager not found. No components will be returned.",
				a.outboundChannel, models.GetDataFlowComponent)
		}

	}

	// ─── 2  Build the public response object ────────────────────────────────
	a.actionLogger.Info("Building the response")
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		fmt.Sprintf("Processing configurations for %d dataflow components...",
			len(dataFlowComponents)), a.outboundChannel, models.GetDataFlowComponent)
	response := models.GetDataflowcomponentResponse{}
	for _, component := range dataFlowComponents {
		// build the payload
		dfc_payload := models.CommonDataFlowComponentCDFCPropertiesPayload{}
		tagValue := "" // the benthos image tag is not used anymore in UMH Core
		dfc_payload.CDFCProperties.BenthosImageTag = &models.CommonDataFlowComponentBenthosImageTagConfig{
			Tag: &tagValue,
		}
		dfc_payload.CDFCProperties.IgnoreErrors = nil
		//fill the inputs, outputs, pipeline and rawYAML
		// Convert the BenthosConfig input to CommonDataFlowComponentInputConfig
		inputData, err := yaml.Marshal(component.DataFlowComponentServiceConfig.BenthosConfig.Input)
		if err != nil {
			a.actionLogger.Warnf("Failed to marshal input data: %v", err)
		}
		dfc_payload.CDFCProperties.Inputs = models.CommonDataFlowComponentInputConfig{
			Data: string(inputData),
			Type: "benthos", // Default type for benthos inputs
		}

		// Convert the BenthosConfig output to CommonDataFlowComponentOutputConfig
		outputData, err := yaml.Marshal(component.DataFlowComponentServiceConfig.BenthosConfig.Output)
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
		if pipeline, ok := component.DataFlowComponentServiceConfig.BenthosConfig.Pipeline["processors"].([]interface{}); ok {
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
		if threadsVal, ok := component.DataFlowComponentServiceConfig.BenthosConfig.Pipeline["threads"]; ok {
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
		if len(component.DataFlowComponentServiceConfig.BenthosConfig.CacheResources) > 0 {
			rawYAMLMap["cache_resources"] = component.DataFlowComponentServiceConfig.BenthosConfig.CacheResources
		}

		// Add rate limit resources if present
		if len(component.DataFlowComponentServiceConfig.BenthosConfig.RateLimitResources) > 0 {
			rawYAMLMap["rate_limit_resources"] = component.DataFlowComponentServiceConfig.BenthosConfig.RateLimitResources
		}

		// Add buffer if present
		if len(component.DataFlowComponentServiceConfig.BenthosConfig.Buffer) > 0 {
			rawYAMLMap["buffer"] = component.DataFlowComponentServiceConfig.BenthosConfig.Buffer
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

		response[dataflowcomponentserviceconfig.GenerateUUIDFromName(component.FSMInstanceConfig.Name).String()] = models.GetDataflowcomponentResponseContent{
			CreationTime: 0,
			Creator:      "",
			Meta: models.CommonDataFlowComponentMeta{
				Type: "custom",
			},
			Name:      component.Name,
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

// GetParsedVersionUUIDs returns the parsed request version UUIDs - exposed primarily for testing purposes.
func (a *GetDataFlowComponentAction) GetParsedVersionUUIDs() models.GetDataflowcomponentRequestSchemaJson {
	return a.payload
}
