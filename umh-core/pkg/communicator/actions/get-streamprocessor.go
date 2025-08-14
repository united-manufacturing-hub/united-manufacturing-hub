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
// server.  Unlike the *edit* and *delete* actions, **GetStreamProcessor** is
// **read-only** – it aggregates configuration *and* runtime state for one
// Stream Processor (SP) so that the frontend can render a human-friendly
// representation.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
//   - A **stream processor UUID** is the deterministic ID derived from a SP name via
//     `dataflowcomponentserviceconfig.GenerateUUIDFromName`.  The frontend knows
//     only this UUID when requesting stream processor details.
//   - The action therefore needs to translate UUID → runtime instance name →
//     observed Stream Processor configuration → API response schema.
//   - All information is fetched from a *single* snapshot of the FSM runtime so
//     the result is **self-consistent** even while the system keeps running.
//
// -----------------------------------------------------------------------------
// HIGH-LEVEL FLOW
// -----------------------------------------------------------------------------
//  1. **Parse** – store the requested UUID (no heavy work here).
//  2. **Validate** – check that the UUID is valid.
//  3. **Execute**
//     a. Copy the shared `*fsm.SystemSnapshot` under the read-lock.
//     b. Find the live SP instance whose deterministic UUID matches the request.
//     c. Extract the stream processor configuration from the observed state.
//     d. Convert the configuration into the *public* UMH API schema.
//     e. Send progress messages if the instance is missing.
//     f. Return the assembled `models.StreamProcessor` object.
//
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// GetStreamProcessorAction returns metadata and configuration for the
// requested Stream Processor **UUID**.  The action never blocks the FSM
// writer goroutine – instead it holds the read‑lock only while making a deep
// copy of the snapshot.
//
// All fields are immutable after Parse so that callers can safely pass the
// struct between goroutines when needed.
// ----------------------------------------------------------------------------

type GetStreamProcessorAction struct {
	configManager config.ConfigManager // currently unused but kept for symmetry

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager

	// ─── Utilities ──────────────────────────────────────────────────────────
	actionLogger *zap.SugaredLogger

	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	// ─── Parsed request payload ─────────────────────────────────────────────
	payload models.GetStreamProcessorPayload
}

// NewGetStreamProcessorAction creates a new GetStreamProcessorAction with the provided parameters.
// This constructor is primarily used for testing to enable dependency injection, though it can be used
// in production code as well. It initializes the action with the necessary fields but doesn't
// populate the payload field which must be done via Parse.
func NewGetStreamProcessorAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *GetStreamProcessorAction {
	return &GetStreamProcessorAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		systemSnapshotManager: systemSnapshotManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
	}
}

// Parse stores the UUID we should resolve.  The heavy lifting
// happens later in Execute.
func (a *GetStreamProcessorAction) Parse(ctx context.Context, payload interface{}) (err error) {
	a.actionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetStreamProcessorPayload](payload)
	a.actionLogger.Info("Payload parsed, uuid: ", a.payload.UUID)

	return err
}

// Validate validates the parsed payload, checking that required fields are present
// and properly formatted.
func (a *GetStreamProcessorAction) Validate(ctx context.Context) error {
	// Check if UUID is the zero value (empty UUID)
	if a.payload.UUID == uuid.Nil {
		return errors.New("uuid must be set to retrieve stream processor")
	}

	return nil
}

// Execute retrieves the stream processor configuration from the system snapshot and returns it.
func (a *GetStreamProcessorAction) Execute(ctx context.Context) (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing GetStreamProcessor action")

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting stream processor retrieval", a.outboundChannel, models.GetStreamProcessor)

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Retrieving stream processor configuration...", a.outboundChannel, models.GetStreamProcessor)

	// Get the current config to read the stream processor configuration
	currentConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to get current config: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.GetStreamProcessor)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Look for the stream processor in the config by matching UUID
	var (
		foundStreamProcessor     *config.StreamProcessorConfig
		foundStreamProcessorName string
	)

	for _, sp := range currentConfig.StreamProcessor {
		// Generate the deterministic UUID from the stream processor name
		generatedUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(sp.Name)
		if generatedUUID == a.payload.UUID {
			foundStreamProcessor = &sp
			foundStreamProcessorName = sp.Name

			break
		}
	}

	if foundStreamProcessor == nil {
		errorMsg := fmt.Sprintf("Stream processor with UUID %s not found", a.payload.UUID)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.GetStreamProcessor)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Building stream processor response...", a.outboundChannel, models.GetStreamProcessor)

	// Build the response from the config
	streamProcessor := a.buildStreamProcessorFromConfig(foundStreamProcessor, foundStreamProcessorName)

	a.actionLogger.Info("Stream processor retrieved successfully")

	return streamProcessor, nil, nil
}

// buildStreamProcessorFromConfig constructs a models.StreamProcessor from the config data.
func (a *GetStreamProcessorAction) buildStreamProcessorFromConfig(streamProcessorConfig *config.StreamProcessorConfig, instanceName string) *models.StreamProcessor {
	_ = instanceName // TODO: use instanceName if needed for instance-specific configuration
	// Build location map from agent location - convert map[int]string to map[int]string
	locationMap := make(map[int]string)

	if streamProcessorConfig.StreamProcessorServiceConfig.Location != nil {
		// Convert the location map[string]string to map[int]string
		for key, value := range streamProcessorConfig.StreamProcessorServiceConfig.Location {
			intKey, err := strconv.Atoi(key)
			if err == nil {
				locationMap[intKey] = value
			}
		}
	}

	// Build model reference
	modelRef := models.StreamProcessorModelRef{
		Name:    streamProcessorConfig.StreamProcessorServiceConfig.Config.Model.Name,
		Version: streamProcessorConfig.StreamProcessorServiceConfig.Config.Model.Version,
	}

	// Build encoded config - encode the YAML representation
	var encodedConfig string

	spConfig := streamProcessorConfig.StreamProcessorServiceConfig.Config
	// remove the model reference from the config and encode the rest (sources and mapping)
	toEncode := map[string]interface{}{
		"sources": spConfig.Sources,
		"mapping": spConfig.Mapping,
	}

	configData, err := yaml.Marshal(toEncode)
	if err == nil {
		encodedConfig = base64.StdEncoding.EncodeToString(configData)
	}

	// Build template info if template ref is set
	var templateInfo *models.StreamProcessorTemplateInfo

	if streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef != "" {
		// Check if this is a templated instance
		isTemplated := streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef != streamProcessorConfig.Name

		// Build variables from the StreamProcessorServiceConfig
		variables := make([]models.StreamProcessorVariable, 0)

		if streamProcessorConfig.StreamProcessorServiceConfig.Variables.User != nil {
			for label, value := range streamProcessorConfig.StreamProcessorServiceConfig.Variables.User {
				variables = append(variables, models.StreamProcessorVariable{
					Label: label,
					Value: fmt.Sprintf("%v", value),
				})
			}
		}

		// Generate root UUID from template reference
		rootUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(streamProcessorConfig.StreamProcessorServiceConfig.TemplateRef)

		templateInfo = &models.StreamProcessorTemplateInfo{
			IsTemplated: isTemplated,
			Variables:   variables,
			RootUUID:    rootUUID,
		}
	}

	// Create the stream processor response
	streamProcessor := &models.StreamProcessor{
		UUID:          &a.payload.UUID,
		Name:          streamProcessorConfig.Name,
		Location:      locationMap,
		Model:         modelRef,
		EncodedConfig: encodedConfig,
		Config:        nil, // This is marked as not used in the action
		TemplateInfo:  templateInfo,
	}

	return streamProcessor
}

// getUserEmail returns the user email for this action.
func (a *GetStreamProcessorAction) getUserEmail() string {
	return a.userEmail
}

// getUuid returns the action UUID for this action.
func (a *GetStreamProcessorAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed payload - exposed for testing purposes.
func (a *GetStreamProcessorAction) GetParsedPayload() models.GetStreamProcessorPayload {
	return a.payload
}
