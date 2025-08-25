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

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
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
	configManager config.ConfigManager // currently unused but kept for symmetry

	// ─── Plumbing ────────────────────────────────────────────────────────────
	outboundChannel chan *models.UMHMessage

	// ─── Runtime observation ────────────────────────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager

	// ─── Utilities ──────────────────────────────────────────────────────────
	actionLogger *zap.SugaredLogger

	// ─── Request metadata ────────────────────────────────────────────────────
	userEmail string

	// ─── Parsed request payload ─────────────────────────────────────────────
	payload models.GetDataflowcomponentRequestSchemaJson

	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewGetDataFlowComponentAction creates a new GetDataFlowComponentAction with the provided parameters.
// This constructor is primarily used for testing to enable dependency injection, though it can be used
// in production code as well. It initializes the action with the necessary fields but doesn't
// populate the payload field which must be done via Parse.
func NewGetDataFlowComponentAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *GetDataFlowComponentAction {
	return &GetDataFlowComponentAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		systemSnapshotManager: systemSnapshotManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
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

func (a *GetDataFlowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing the action")
	numUUIDs := len(a.payload.VersionUUIDs)

	dataFlowComponents := []config.DataFlowComponentConfig{}
	// Get the DataFlowComponent
	a.actionLogger.Debugf("Getting the DataFlowComponent")

	// the snapshot manager holds the latest system snapshot which is asynchronously updated by the other goroutines
	// we need to get a deep copy of it to prevent race conditions
	systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()

	if dataflowcomponentManager, exists := systemSnapshot.Managers[constants.DataflowcomponentManagerName]; exists {
		a.actionLogger.Debugf("Dataflowcomponent manager found, getting the dataflowcomponent")

		instances := dataflowcomponentManager.GetInstances()
		foundComponents := 0

		for _, instance := range instances {
			currentUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID).String()
			if slices.Contains(a.payload.VersionUUIDs, currentUUID) {
				a.actionLogger.Debugf("Adding %s to the response", instance.ID)

				dfc, err := BuildDataFlowComponentDataFromSnapshot(*instance, a.actionLogger)
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
		}
	}

	// ─── 2  Build the public response object ────────────────────────────────
	a.actionLogger.Info("Building the response")
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		fmt.Sprintf("Processing configurations for %d dataflow components...",
			len(dataFlowComponents)), a.outboundChannel, models.GetDataFlowComponent)
	response := models.GetDataflowcomponentResponse{}

	for _, component := range dataFlowComponents {
		// build the payload using shared function
		dfc_payload, err := BuildCommonDataFlowComponentPropertiesFromConfig(component.DataFlowComponentServiceConfig, a.actionLogger)
		if err != nil {
			a.actionLogger.Warnf("Failed to build CDFC properties: %v", err)
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
				fmt.Sprintf("Warning: Failed to build properties for component '%s': %v",
					component.Name, err), a.outboundChannel, models.GetDataFlowComponent)

			continue
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
			State:     component.DesiredFSMState,
		}
	}

	// Send the success message
	// SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedSuccessfull, response, a.outboundChannel, models.GetDataFlowComponent)

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
