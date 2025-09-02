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

// Package actions contains implementations of the Action interface that mutate the
// UMH configuration or otherwise change the system state.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A *new* Data-Flow Component (DFC) in UMH is defined by a Benthos service
// configuration and materialised as an FSM instance.  "Deploying" therefore
// means **creating a new desired configuration entry** and **waiting until the
// FSM reports**
//
//   - state "active" **and**
//   - the *observed* configuration equals the *desired* one.
//
// If the component fails to reach `state=="active"` within
// `constants.DataflowComponentWaitForActiveTimeout`, the action *removes* the
// component again (unless the caller set `ignoreHealthCheck`).
//
// Runtime state observation
// -------------------------
// Just like EditDataflowComponentAction, the caller hands over a pointer
// `*fsm.SystemSnapshot` that is filled by the FSM event loop.  The action only
// takes *copies* under a read-lock (`GetSystemSnapshot`) to avoid blocking the
// writer.
//
// -----------------------------------------------------------------------------
// The concrete flow of a DeployDataflowComponentAction
// -----------------------------------------------------------------------------
//   1. **Parse** – extract name, type, payload and flags.
//   2. **Validate** – structural sanity checks and YAML parsing.
//   3. **Execute**
//        a.     Send ActionConfirmed.
//        b.     Translate the custom payload into a Benthos service config.
//        c.     Add a new DFC config via `configManager.AtomicAddDataflowcomponent`
//               (desired state = "active").
//        d.     Poll `systemSnapshot` until the instance reports `state==active`.
//        e.     If the poll times out → delete the component (unless
//               `ignoreHealthCheck`).
//
// All public methods below have Go-doc comments that repeat these key aspects in
// the exact location where a future maintainer will look for them.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6/s6_shared"
	"go.uber.org/zap"
)

// DeployDataflowComponentAction implements the Action interface for deploying a
// *new* Data-Flow Component.  All fields are *immutable* after construction to
// avoid race conditions – transient state lives in local variables only.
// -----------------------------------------------------------------------------.
type DeployDataflowComponentAction struct {
	configManager config.ConfigManager // abstraction over the central configuration store

	outboundChannel chan *models.UMHMessage // channel used to send progress events back to the UI

	// ─── Runtime observation & synchronisation ───────────────────────────────
	systemSnapshotManager *fsm.SnapshotManager // Snapshot Manager holds the latest system snapshot

	actionLogger *zap.SugaredLogger

	userEmail string // e-mail of the human that triggered the action

	name     string // human-readable component name
	metaType string // "custom" for now – future-proofing for other component kinds
	state    string // the desired state of the component

	// Parsed request payload (only populated after Parse)
	payload models.CDFCPayload

	actionUUID   uuid.UUID // unique ID of *this* action instance
	instanceUUID uuid.UUID // ID of the UMH instance this action operates on

	ignoreHealthCheck bool // if true → no delete on timeout
}

// NewDeployDataflowComponentAction returns an *un-parsed* action instance.
// Primarily used for dependency injection in unit tests – caller still needs to
// invoke Parse & Validate before Execute.
func NewDeployDataflowComponentAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *DeployDataflowComponentAction {
	return &DeployDataflowComponentAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		actionLogger:          logger.For(logger.ComponentCommunicator),
		systemSnapshotManager: systemSnapshotManager,
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
	topLevel, err := ParseDataflowComponentTopLevel(payload)
	if err != nil {
		return err
	}

	a.name = topLevel.Name
	a.metaType = topLevel.Meta.Type
	a.ignoreHealthCheck = topLevel.IgnoreHealthCheck

	a.state = topLevel.State
	if err := ValidateDataFlowComponentState(a.state); err != nil {
		return err
	}

	// Handle different component types
	switch a.metaType {
	case "custom":
		payload, err := ParseCustomDataFlowComponent(topLevel.Payload)
		if err != nil {
			return err
		}

		a.payload = payload
	case "protocolConverter", "dataBridge", "streamProcessor":
		return fmt.Errorf("component type %s not yet supported", a.metaType)
	default:
		return fmt.Errorf("unsupported component type: %s", a.metaType)
	}

	a.actionLogger.Debugf("Parsed DeployDataFlowComponent action payload: name=%s, type=%s", a.name, a.metaType)

	return nil
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

	// For custom type, validate the payload structure using shared function
	if a.metaType == "custom" {
		if err := ValidateCustomDataFlowComponentPayload(a.payload, true); err != nil {
			return err
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
// - Adding the component to the configuration with a desired state of "active".
func (a *DeployDataflowComponentAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeployDataflowComponent action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed, Label("deploy", a.name)+"starting", a.outboundChannel, models.DeployDataFlowComponent)

	// Parse and create Benthos configuration using shared function
	benthosConfig, err := CreateBenthosConfigFromCDFCPayload(a.payload, a.name)
	if err != nil {
		errMsg := Label("deploy", a.name) + err.Error()
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errMsg, a.outboundChannel, models.DeployDataFlowComponent)

		return nil, nil, fmt.Errorf("%s", errMsg)
	}

	// Create the DataFlowComponentConfig using shared function
	dfc := CreateDataFlowComponentConfig(a.name, a.state, benthosConfig)

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, Label("deploy", a.name)+"adding to configuration", a.outboundChannel, models.DeployDataFlowComponent)
	// Update the location in the configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	err = a.configManager.AtomicAddDataflowcomponent(ctx, dfc)
	if err != nil {
		errorMsg := Label("deploy", a.name) + fmt.Sprintf("failed to add dataflow component: %v.", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, a.outboundChannel, models.DeployDataFlowComponent)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// check against observedState as well
	if a.systemSnapshotManager != nil { // skipping this for the unit tests
		if a.ignoreHealthCheck {
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, Label("deploy", a.name)+"configuration updated; but ignoring the health check", a.outboundChannel, models.DeployDataFlowComponent)
		} else {
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, Label("deploy", a.name)+"configuration updated; waiting to become ready", a.outboundChannel, models.DeployDataFlowComponent)

			errCode, err := a.waitForComponentToBeReady(ctx)
			if err != nil {
				errorMsg := Label("deploy", a.name) + fmt.Sprintf("failed to wait for dataflow component to be ready: %v", err)
				// waitForComponentToBeReady gives us the error code, which we then forward to the frontend using the SendActionReplyV2 function
				// the error code is a string that can be used to identify the error reason
				// the main reason for this is to allow the frontend to determine whether it should offer a retry option or not
				SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure, errorMsg, errCode, nil, a.outboundChannel, models.DeployDataFlowComponent, nil)

				return nil, nil, fmt.Errorf("%s", errorMsg)
			}
		}
	}

	// return success message, but do not send it as this is done by the caller
	successMsg := Label("deploy", a.name) + "component successfully deployed"

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

// waitForComponentToBeReady polls live FSM state until the new component
// reaches the desired state or the timeout hits (→ delete unless ignoreHealthCheck).
// the function returns the error code and the error message via an error object
// the error code is a string that is sent to the frontend to allow it to determine if the action can be retried or not
// the error message is sent to the frontend to allow the user to see the error message.
func (a *DeployDataflowComponentAction) waitForComponentToBeReady(ctx context.Context) (string, error) {
	// checks the system snapshot
	// 1. waits for the instance to appear in the system snapshot
	// 2. takes the logs of the instance and sends them to the user in 1-second intervals
	// 3. waits for the instance to reach the desired state
	// 4. takes the residual logs of the instance and sends them to the user
	// 5. returns nil

	// we use those two variables below to store the incoming logs and send them to the user
	// logs is always updated with all existing logs
	// lastLogs is updated with the logs that have been sent to the user
	// this way we avoid sending the same log twice
	var (
		logs     []s6_shared.LogEntry
		lastLogs []s6_shared.LogEntry
	)

	ticker := time.NewTicker(constants.ActionTickerTime)
	defer ticker.Stop()

	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout)
	startTime := time.Now()
	timeoutDuration := constants.DataflowComponentWaitForActiveTimeout

	for {
		elapsed := time.Since(startTime)
		remaining := timeoutDuration - elapsed
		remainingSeconds := int(remaining.Seconds())

		select {
		case <-timeout:
			stateMessage := Label("deploy", a.name) + "timeout reached. it did not reach the desired state in time. removing"
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage,
				a.outboundChannel, models.DeployDataFlowComponent)
			// Create a fresh context for cleanup operation since the original ctx has timed out
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
			defer cleanupCancel()

			err := a.configManager.AtomicDeleteDataflowcomponent(cleanupCtx, dataflowcomponentserviceconfig.GenerateUUIDFromName(a.name))
			if err != nil {
				a.actionLogger.Errorf("failed to remove dataflowcomponent %s: %v", a.name, err)

				return models.ErrRetryRollbackTimeout, fmt.Errorf("dataflow component '%s' failed to reach desired state within timeout but could not be removed: %w. Please check system load and consider removing the component manually", a.name, err)
			}

			return models.ErrRetryRollbackTimeout, fmt.Errorf("dataflow component '%s' was removed because it did not reach the desired state within the timeout period. Please check system load or component configuration and try again", a.name)

		case <-ticker.C:
			// the snapshot manager holds the latest system snapshot which is asynchronously updated by the other goroutines
			// we need to get a deep copy of it to prevent race conditions
			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()
			if dataflowcomponentManager, exists := systemSnapshot.Managers[constants.DataflowcomponentManagerName]; exists {
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
						stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for state info"
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage,
							a.outboundChannel, models.DeployDataFlowComponent)

						continue
					}
					// Compare current state with the desired state
					var acceptedStates []string

					switch a.state {
					case dataflowcomponent.OperationalStateActive:
						acceptedStates = []string{dataflowcomponent.OperationalStateActive, dataflowcomponent.OperationalStateIdle}
					case dataflowcomponent.OperationalStateStopped:
						acceptedStates = []string{dataflowcomponent.OperationalStateStopped}
					}

					if slices.Contains(acceptedStates, instance.CurrentState) {
						stateMessage := RemainingPrefixSec(remainingSeconds) + fmt.Sprintf("completed. is in state '%s' with correct configuration", instance.CurrentState)
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage,
							a.outboundChannel, models.DeployDataFlowComponent)

						return "", nil
					}

					// currentStateReason contains more information on why the DFC is in its current state
					currentStateReason := dfcSnapshot.ServiceInfo.StatusReason

					stateMessage := RemainingPrefixSec(remainingSeconds) + currentStateReason
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage, a.outboundChannel, models.DeployDataFlowComponent)
					// send the benthos logs to the user
					logs = dfcSnapshot.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs

					// only send the logs that have not been sent yet
					if len(logs) > len(lastLogs) {
						lastLogs = SendLimitedLogs(logs, lastLogs, a.instanceUUID, a.userEmail, a.actionUUID, a.outboundChannel, models.DeployDataFlowComponent, remainingSeconds)
					}
					// CheckBenthosLogLinesForConfigErrors is used to detect fatal configuration errors that would cause
					// Benthos to enter a CrashLoop. When such errors are detected, we can immediately
					// abort the startup process rather than waiting for the full timeout period,
					// as these errors require configuration changes to resolve.
					if CheckBenthosLogLinesForConfigErrors(logs) {
						SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, Label("deploy", a.name)+"configuration error detected. Removing component...", a.outboundChannel, models.DeployDataFlowComponent)
						// Create a fresh context for cleanup operation since the original ctx may be expired or close to expiring
						cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
						defer cleanupCancel()

						err := a.configManager.AtomicDeleteDataflowcomponent(cleanupCtx, dataflowcomponentserviceconfig.GenerateUUIDFromName(a.name))
						if err != nil {
							a.actionLogger.Errorf("failed to remove dataflowcomponent %s: %v", a.name, err)

							return models.ErrConfigFileInvalid, fmt.Errorf("dataflow component '%s' has invalid configuration but could not be removed: %w. Please check your logs and consider removing the component manually", a.name, err)
						}

						return models.ErrConfigFileInvalid, fmt.Errorf("dataflow component '%s' was removed due to configuration errors. Please check the component logs, fix the configuration issues, and try deploying again", a.name)
					}
				}

				if !found {
					stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for it to appear in the config"
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.DeployDataFlowComponent)
				}
			} else {
				stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for manager to initialise"
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					stateMessage, a.outboundChannel, models.DeployDataFlowComponent)
			}
		}
	}
}
