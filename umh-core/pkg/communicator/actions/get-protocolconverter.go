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
// server.  Unlike the *edit* and *delete* actions, **GetProtocolConverter** is
// **read-only** – it aggregates configuration *and* runtime state for one
// Protocol Converter (PC) so that the frontend can render a human-friendly
// representation.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
//   - A **protocol converter UUID** is the deterministic ID derived from a PC name via
//     `dataflowcomponentserviceconfig.GenerateUUIDFromName`.  The frontend knows
//     only this UUID when requesting protocol converter details.
//   - The action therefore needs to translate UUID → runtime instance name →
//     observed Protocol Converter configuration → API response schema.
//   - All information is fetched from a *single* snapshot of the FSM runtime so
//     the result is **self-consistent** even while the system keeps running.
//
// -----------------------------------------------------------------------------
// HIGH-LEVEL FLOW
// -----------------------------------------------------------------------------
//  1. **Parse** – store the requested UUID (no heavy work here).
//  2. **Validate** – no-op because Parse already guarantees structural
//     correctness.
//  3. **Execute**
//     a. Copy the shared `*fsm.SystemSnapshot` under the read-lock.
//     b. Find the live PC instance whose deterministic UUID matches the request.
//     c. Extract both read and write DFC configurations from the observed state.
//     d. Convert each Benthos config into the *public* UMH API schema.
//     e. Send progress messages if the instance is missing.
//     f. Return the assembled `models.ProtocolConverter` object.
//
// -----------------------------------------------------------------------------

package actions

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// GetProtocolConverterAction returns metadata and Benthos configuration for the
// requested Protocol Converter **UUID**.  The action never blocks the FSM
// writer goroutine – instead it holds the read‑lock only while making a deep
// copy of the snapshot.
//
// All fields are immutable after Parse so that callers can safely pass the
// struct between goroutines when needed.
// ----------------------------------------------------------------------------

type GetProtocolConverterAction struct {

	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail    string
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage
	configManager   config.ConfigManager // currently unused but kept for symmetry

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager

	// ─── Parsed request payload ─────────────────────────────────────────────
	payload models.GetProtocolConverterPayload

	// ─── Utilities ──────────────────────────────────────────────────────────
	actionLogger *zap.SugaredLogger
}

// NewGetProtocolConverterAction creates a new GetProtocolConverterAction with the provided parameters.
// This constructor is primarily used for testing to enable dependency injection, though it can be used
// in production code as well. It initializes the action with the necessary fields but doesn't
// populate the payload field which must be done via Parse.
func NewGetProtocolConverterAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *GetProtocolConverterAction {
	return &GetProtocolConverterAction{
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
func (a *GetProtocolConverterAction) Parse(payload interface{}) (err error) {
	a.actionLogger.Info("Parsing the payload")
	a.payload, err = ParseActionPayload[models.GetProtocolConverterPayload](payload)
	a.actionLogger.Info("Payload parsed, uuid: ", a.payload.UUID)
	return err
}

// Validate is a no‑op because the request schema does not require additional
// semantic checks beyond JSON deserialization.
func (a *GetProtocolConverterAction) Validate() error {
	return nil
}

// determineProcessingMode analyzes the pipeline processors in readDFC only
// to determine the appropriate processing mode based on the business rules.
func determineProcessingMode(readDFC *models.ProtocolConverterDFC) string {
	// Only look at readDFC as requested
	if readDFC == nil {
		return "no_dfc"
	}

	processors := readDFC.Pipeline.Processors

	// If more than one processor, return custom
	if len(processors) > 1 {
		return "custom"
	}

	// If exactly one processor, check its type
	if len(processors) == 1 {
		// Get the first (and only) processor from the map
		for _, processor := range processors {
			switch processor.Type {
			case "nodered_js":
				return "nodered_js"
			case "tag_processor":
				return "tag_processor"
			default:
				return "custom"
			}
		}
	}

	// No processors found, fall back to custom
	return "custom"
}

// determineProtocol analyzes the input processors to determine the protocol
func determineProtocol(readDFC *models.ProtocolConverterDFC) string {
	if readDFC == nil {
		return "generic"
	}

	input := readDFC.Inputs

	return input.Type
}

// buildProtocolConverterDFCFromConfig converts a dataflow component service config
// into the models.ProtocolConverterDFC format expected by the API using the shared function.
func buildProtocolConverterDFCFromConfig(dfcConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig, a *GetProtocolConverterAction) (*models.ProtocolConverterDFC, error) {
	if len(dfcConfig.BenthosConfig.Input) == 0 {
		// No DFC configuration present
		return nil, nil
	}

	// Use the shared function to build the common DFC properties
	commonPayload, err := BuildCommonDataFlowComponentPropertiesFromConfig(dfcConfig, a.actionLogger)
	if err != nil {
		return nil, err
	}

	// Convert the common payload to ProtocolConverterDFC format
	dfc := &models.ProtocolConverterDFC{
		Inputs:   commonPayload.CDFCProperties.Inputs,
		Pipeline: commonPayload.CDFCProperties.Pipeline,
		RawYAML:  commonPayload.CDFCProperties.RawYAML,
	}

	return dfc, nil
}

func (a *GetProtocolConverterAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing the action")

	// Get the Protocol Converter
	a.actionLogger.Debugf("Getting the Protocol Converter")

	// the snapshot manager holds the latest system snapshot which is asynchronously updated by the other goroutines
	// we need to get a deep copy of it to prevent race conditions
	systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()

	if protocolconverterManager, exists := systemSnapshot.Managers[constants.ProtocolConverterManagerName]; exists {
		a.actionLogger.Debugf("Protocol converter manager found, getting the protocol converter")
		instances := protocolconverterManager.GetInstances()

		for _, instance := range instances {
			currentUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID)
			if currentUUID == a.payload.UUID {
				a.actionLogger.Debugf("Found protocol converter %s", instance.ID)

				// Try to cast to the right type
				observedState, ok := instance.LastObservedState.(*protocolconverter.ProtocolConverterObservedStateSnapshot)
				if !ok {
					a.actionLogger.Errorw("Observed state is of unexpected type", "instanceID", instance.ID)
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						fmt.Sprintf("Warning: Invalid observed state for protocol converter '%s'", instance.ID),
						a.outboundChannel, models.GetProtocolConverter)
					return nil, nil, fmt.Errorf("invalid observed state type for protocol converter %s", instance.ID)
				}

				if observedState == nil {
					a.actionLogger.Warnw("No observed state found for protocol converter", "instanceID", instance.ID)
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						fmt.Sprintf("Warning: No observed state found for protocol converter '%s'", instance.ID),
						a.outboundChannel, models.GetProtocolConverter)
					return nil, nil, fmt.Errorf("no observed state found for protocol converter %s", instance.ID)
				}

				// Extract connection info from variables
				var ip string
				var port uint32

				config := observedState.ObservedProtocolConverterTemplateConfig
				if config.ConnectionServiceConfig.NmapTemplate.Target != "" {
					ip = config.ConnectionServiceConfig.NmapTemplate.Target
				}
				if config.ConnectionServiceConfig.NmapTemplate.Port != "" {
					portInt, err := strconv.ParseUint(config.ConnectionServiceConfig.NmapTemplate.Port, 10, 32)
					if err != nil {
						a.actionLogger.Warnw("Failed to parse port number", "port", config.ConnectionServiceConfig.NmapTemplate.Port, "error", err)
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
							fmt.Sprintf("Warning: Invalid port number '%s' for protocol converter '%s'", config.ConnectionServiceConfig.NmapTemplate.Port, instance.ID),
							a.outboundChannel, models.GetProtocolConverter)
						return nil, nil, fmt.Errorf("invalid port number '%s' for protocol converter %s: %v", config.ConnectionServiceConfig.NmapTemplate.Port, instance.ID, err)
					}
					port = uint32(portInt)
				}

				// Build ReadDFC if present
				var readDFC *models.ProtocolConverterDFC
				if readDFCConfig := config.DataflowComponentReadServiceConfig; len(readDFCConfig.BenthosConfig.Input) > 0 {
					var err error
					readDFC, err = buildProtocolConverterDFCFromConfig(readDFCConfig, a)
					if err != nil {
						a.actionLogger.Warnf("Failed to build read DFC: %v", err)
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
							fmt.Sprintf("Warning: Failed to build read DFC for protocol converter '%s': %v", instance.ID, err),
							a.outboundChannel, models.GetProtocolConverter)
					}
				}

				// Build WriteDFC if present
				var writeDFC *models.ProtocolConverterDFC
				if writeDFCConfig := config.DataflowComponentWriteServiceConfig; len(writeDFCConfig.BenthosConfig.Input) > 0 {
					var err error
					writeDFC, err = buildProtocolConverterDFCFromConfig(writeDFCConfig, a)
					if err != nil {
						a.actionLogger.Warnf("Failed to build write DFC: %v", err)
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
							fmt.Sprintf("Warning: Failed to build write DFC for protocol converter '%s': %v", instance.ID, err),
							a.outboundChannel, models.GetProtocolConverter)
					}
				}

				// Location is stored in the config spec, not in the observed runtime state
				// We need to access the protocol converter from the system snapshot to get the config
				location := make(map[int]string)
				if pcManager, exists := systemSnapshot.Managers[constants.ProtocolConverterManagerName]; exists {
					if pcInstances := pcManager.GetInstances(); pcInstances != nil {
						for _, pcInstance := range pcInstances {
							if pcInstance.ID == instance.ID {
								// Extract location from system snapshot config
								if pcConfigs := systemSnapshot.CurrentConfig.ProtocolConverter; pcConfigs != nil {
									for _, pcConfig := range pcConfigs {
										if pcConfig.Name == instance.ID {
											// Convert location from string map to int map (reverse of deploy operation)
											if len(pcConfig.ProtocolConverterServiceConfig.Location) > 0 {
												for k, v := range pcConfig.ProtocolConverterServiceConfig.Location {
													var intKey int
													if _, err := fmt.Sscanf(k, "%d", &intKey); err == nil {
														location[intKey] = v
													}
												}
											}
											break
										}
									}
								}
								break
							}
						}
					}
				}

				// Create meta information
				meta := &models.ProtocolConverterMeta{
					ProcessingMode: determineProcessingMode(readDFC),
					Protocol:       determineProtocol(readDFC),
				}

				// Build the response
				response := models.ProtocolConverter{
					UUID:     &currentUUID,
					Name:     instance.ID,
					Location: location,
					Connection: models.ProtocolConverterConnection{
						IP:   ip,
						Port: port,
					},
					ReadDFC:  readDFC,
					WriteDFC: writeDFC,
					Meta:     meta,
					// TemplateInfo can be added later if needed
					TemplateInfo: nil,
				}

				a.actionLogger.Info("Protocol converter found and built, returning response")
				return response, nil, nil
			}
		}
	}

	// Protocol converter not found
	errorMsg := fmt.Sprintf("Protocol converter with UUID %s not found", a.payload.UUID)
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
		errorMsg, a.outboundChannel, models.GetProtocolConverter)
	return nil, nil, fmt.Errorf("%s", errorMsg)
}

func (a *GetProtocolConverterAction) getUserEmail() string {
	return a.userEmail
}

func (a *GetProtocolConverterAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed request payload - exposed primarily for testing purposes.
func (a *GetProtocolConverterAction) GetParsedPayload() models.GetProtocolConverterPayload {
	return a.payload
}
