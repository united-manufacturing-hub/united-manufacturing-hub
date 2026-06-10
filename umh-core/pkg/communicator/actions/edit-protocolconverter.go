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

// Package actions contains implementations of the Action interface that edit
// protocol converter configurations, particularly for adding dataflow components
// to existing protocol converters.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// An existing Protocol Converter (PC) in UMH starts as a basic connection template.
// The edit action allows adding actual dataflow component configurations (read/write)
// to the protocol converter, effectively making it functional for data processing.
//
// The action follows a pattern similar to deploy-dataflowcomponent but operates
// on an existing protocol converter configuration instead of creating a new one.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter/runtime_config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
	"go.uber.org/zap"
)

// DFCType represents the type of dataflow component configuration.
type DFCType string

const (
	// DFCTypeRead represents a read dataflow component.
	DFCTypeRead DFCType = "read"
	// DFCTypeWrite represents a write dataflow component.
	DFCTypeWrite DFCType = "write"
	// DFCTypeBoth represents both read and write dataflow components being updated simultaneously.
	DFCTypeBoth DFCType = "both"
	// DFCTypeEmpty represents no dataflow component (connection/location update only).
	DFCTypeEmpty DFCType = "empty"
)

// String returns the string representation of the DFCType.
func (d DFCType) String() string {
	return string(d)
}

// IsValid checks if the DFCType has a valid value.
func (d DFCType) IsValid() bool {
	switch d {
	case DFCTypeRead, DFCTypeWrite, DFCTypeBoth, DFCTypeEmpty:
		return true
	default:
		return false
	}
}

// EditProtocolConverterAction implements the Action interface for editing
// protocol converter configurations, particularly for adding DFC configurations.
type EditProtocolConverterAction struct {
	configManager config.ConfigManager

	fsmLogger deps.FSMLogger
	// lastRenderErr holds the most recent renderDesiredDFCConfig error so the
	// awaitRollout timeout message can surface the real cause instead of just
	// "did not become active in time". It is sticky: compareSingleDFCConfig
	// clears it only when a later render succeeds, so ticks that never reach
	// a render (for example while Benthos restarts) keep the captured cause.
	lastRenderErr error

	outboundChannel chan *models.UMHMessage
	location        map[int]string

	// Runtime observation for health checks
	systemSnapshotManager *fsm.SnapshotManager

	actionLogger *zap.SugaredLogger

	// readDFCSvcCfg is the built+validated read DFC service config, set in Parse.
	// nil means no read DFC was provided in the request.
	readDFCSvcCfg *dataflowcomponentserviceconfig.DataflowComponentServiceConfig
	// writeDFCInput is the raw template-string form of the write config, persisted to the spec.
	// nil means no write DFC was provided.
	writeDFCInput *dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput

	userEmail      string
	name           string // protocol converter name (optional for updates)
	dfcType        DFCType
	connectionPort string
	connectionIP   string
	readDFCState   string // desired state for the read DFC ("active" or "stopped"; empty = default active)
	writeDFCState  string // desired state for the write DFC ("active" or "stopped"; empty = default active)

	templateVars []models.ProtocolConverterVariable

	actionUUID   uuid.UUID
	instanceUUID uuid.UUID

	// Parsed request payload (only populated after Parse)
	protocolConverterUUID uuid.UUID

	// Atomic edit UUID used for configuration updates and rollbacks
	atomicEditUUID uuid.UUID

	// tickInterval overrides the awaitRollout poll interval. Zero means
	// constants.ActionTickerTime; tests inject a shorter interval so the
	// fail-fast specs do not wait on the 1s production ticker.
	tickInterval time.Duration

	ignoreHealthCheck bool
}

// NewEditProtocolConverterAction returns an un-parsed action instance.
func NewEditProtocolConverterAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *EditProtocolConverterAction {
	al := logger.For(logger.ComponentCommunicator)

	return &EditProtocolConverterAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		systemSnapshotManager: systemSnapshotManager,
		actionLogger:          al,
		fsmLogger:             deps.NewFSMLogger(al),
	}
}

// Parse implements the Action interface by extracting the protocol converter UUID and
// dataflow component configuration from the payload.
func (a *EditProtocolConverterAction) Parse(payload interface{}) error {
	a.ignoreHealthCheck = false

	// Parse the payload directly as a complete ProtocolConverter object
	pcPayload, err := ParseActionPayload[models.ProtocolConverter](payload)
	if err != nil {
		return fmt.Errorf("failed to parse protocol converter payload: %w", err)
	}

	// Extract UUID
	if pcPayload.UUID == nil {
		return errors.New("missing required field UUID")
	}

	a.protocolConverterUUID = *pcPayload.UUID
	a.name = pcPayload.Name

	// Resolve user variables first — they are needed to render the InputTopics template.
	if pcPayload.TemplateInfo != nil {
		a.templateVars = pcPayload.TemplateInfo.Variables
	} else {
		a.templateVars = make([]models.ProtocolConverterVariable, 0)
	}

	// Parse and build each DFC side independently.
	// Both sides follow the same pattern: wire format → built/rendered config → state + flags.
	if pcPayload.ReadDFC != nil {
		svcCfg, err := buildReadDFCServiceConfig(dfcToPayload(pcPayload.ReadDFC), pcPayload.Name)
		if err != nil {
			return fmt.Errorf("failed to build read DFC configuration: %w", err)
		}
		a.readDFCSvcCfg = &svcCfg
		a.readDFCState = pcPayload.ReadDFC.State
		if pcPayload.ReadDFC.IgnoreErrors != nil {
			a.ignoreHealthCheck = *pcPayload.ReadDFC.IgnoreErrors
		}
	}
	if pcPayload.WriteDFCPayload != nil {
		input := pcPayload.WriteDFCPayload.DataflowComponentWriteConfigInput
		a.writeDFCInput = &input
		a.writeDFCState = pcPayload.WriteDFCPayload.State
		// OR-merge: if either DFC requests skipping errors, skip the health-check wait
		// for the entire PC. See deploy-protocolconverter.go for the rationale.
		if pcPayload.WriteDFCPayload.IgnoreErrors != nil {
			a.ignoreHealthCheck = a.ignoreHealthCheck || *pcPayload.WriteDFCPayload.IgnoreErrors
		}
	}

	// Extract location
	if pcPayload.Location != nil {
		a.location = pcPayload.Location
	}

	a.connectionPort = strconv.Itoa(int(pcPayload.Connection.Port))
	a.connectionIP = pcPayload.Connection.IP

	// Determine dfcType by comparing incoming DFC configs against what is
	// currently deployed. Only DFCs that actually differ need redeployment.
	// Must run AFTER connectionPort/IP assigned since deriveDFCType reads them.
	a.dfcType = a.deriveDFCType()

	a.actionLogger.Debugf("Parsed EditProtocolConverter action payload: uuid=%s, name=%s, dfcType=%s, readDFCState=%s, writeDFCState=%s",
		a.protocolConverterUUID, a.name, a.dfcType, a.readDFCState, a.writeDFCState)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *EditProtocolConverterAction) Validate() error {
	// Validate UUID and DFC type
	if a.protocolConverterUUID == uuid.Nil {
		return errors.New("missing or invalid protocol converter UUID")
	}

	if err := config.ValidateComponentName(a.name); err != nil {
		return err
	}

	// Validate read DFC state — validate independently of whether config was provided,
	// so state-only edits (readDFCSvcCfg == nil) are also checked.
	if a.readDFCState != "" {
		if err := ValidateDataFlowComponentState(a.readDFCState); err != nil {
			return fmt.Errorf("invalid read DFC state: %w", err)
		}
	}

	// Validate write DFC state
	if err := validateWriteDFCConfig(a.writeDFCInput, a.writeDFCState); err != nil {
		return err
	}

	return nil
}

// Execute implements the Action interface by updating the protocol converter configuration
// with the provided dataflow component configuration.
func (a *EditProtocolConverterAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing EditProtocolConverter action")

	// Send confirmation that action is starting
	var confirmationMessage string
	if a.dfcType == DFCTypeEmpty {
		confirmationMessage = fmt.Sprintf("Starting edit of protocol converter %s to update connection and location", a.protocolConverterUUID)
	} else {
		confirmationMessage = fmt.Sprintf("Starting edit of protocol converter %s to add %s DFC", a.protocolConverterUUID, a.dfcType.String())
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		confirmationMessage, a.outboundChannel, models.EditProtocolConverter)

	var err error

	if a.dfcType != DFCTypeEmpty {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			fmt.Sprintf("Updating protocol converter configuration with %s DFC...", a.dfcType.String()),
			a.outboundChannel, models.EditProtocolConverter)
	} else {
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			"Updating protocol converter configuration (connection and location only)...",
			a.outboundChannel, models.EditProtocolConverter)
	}

	// Apply mutations to create new spec
	newSpec, atomicEditUUID, desiredPCState, err := a.applyMutation()
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to apply configuration mutation: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)
		a.fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", err, "edit_protocol_converter_apply_mutation_failed",
			deps.String("new pcConfig", newSpec.String()))

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Store the atomic edit UUID for use in rollback operations
	a.atomicEditUUID = atomicEditUUID

	oldConfig, err := a.persistConfig(atomicEditUUID, newSpec)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to persist configuration changes: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.EditProtocolConverter)
		a.fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", err, "edit_protocol_converter_persist_config_failed",
			deps.String("new pcConfig", newSpec.String()),
			deps.String("old pcConfig", oldConfig.String()))

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Await rollout and perform health checks
	if a.systemSnapshotManager != nil && !a.ignoreHealthCheck {
		errCode, err := a.awaitRollout(oldConfig, desiredPCState)
		if err != nil {
			errorMsg := fmt.Sprintf("Failed during rollout: %v", err)
			SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
				errorMsg, errCode, nil, a.outboundChannel, models.EditProtocolConverter, nil)
			// awaitRollout abort paths returning these codes already fired
			// their own Sentry event scoped to the abort reason; the
			// render-failure events deliberately carry only the bridge name,
			// UUID and render error. Re-reporting here would attach the full
			// old/new config, whose user variables can hold credentials.
			if errCode != models.ErrConfigFileInvalid && errCode != models.ErrRetryRollbackTimeout {
				a.fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", err, "edit_protocol_converter_rollout_failed",
					deps.String("new pcConfig", newSpec.String()),
					deps.String("old pcConfig", oldConfig.String()))
			}

			return nil, nil, fmt.Errorf("%s", errorMsg)
		}

		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
			"Protocol converter successfully updated", a.outboundChannel, models.EditProtocolConverter)
	}

	newUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(a.name)
	response := map[string]any{
		"uuid": newUUID,
	}

	return response, nil, nil
}

// applyMutation analyzes the current configuration and applies the necessary mutations
// to create the new protocol converter specification. It handles child/root relationships,
// variable merging, and DFC configuration updates.
// Returns the new spec, the atomic edit UUID, the derived PC desired state, and any error.
// Read/write DFC configs are read from a.readDFCSvcCfg and a.writeDFCInput respectively.
func (a *EditProtocolConverterAction) applyMutation() (config.ProtocolConverterConfig, uuid.UUID, string, error) {
	// Get current configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	currentConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		return config.ProtocolConverterConfig{}, uuid.Nil, "", fmt.Errorf("failed to get current configuration: %w", err)
	}

	// Find the protocol converter in the configuration
	var targetPC config.ProtocolConverterConfig

	found := false

	for _, pc := range currentConfig.ProtocolConverter {
		pcID := dataflowcomponentserviceconfig.GenerateUUIDFromName(pc.Name)
		if pcID == a.protocolConverterUUID {
			targetPC = pc
			found = true

			break
		}
	}

	if !found {
		return config.ProtocolConverterConfig{}, uuid.Nil, "", fmt.Errorf("protocol converter with UUID %s not found", a.protocolConverterUUID)
	}

	// Currently, we cannot reuse templates, so we need to create a new one
	targetPC.ProtocolConverterServiceConfig.TemplateRef = a.name
	targetPC.Name = a.name

	// Determine which instance to modify and which UUID to use for atomic operation.
	// TemplateRef is always set to Name (line above), so stand-alone is the only case.
	var (
		instanceToModify = targetPC
		atomicEditUUID   = a.protocolConverterUUID
		newVB            map[string]any
	)

	// Add the new variables and preserve existing variables
	newVB = make(map[string]any)

	// First copy existing variables (provides defaults)
	if targetPC.ProtocolConverterServiceConfig.Variables.User != nil {
		maps.Copy(newVB, targetPC.ProtocolConverterServiceConfig.Variables.User)
	}

	// Then add new variables (overwrites with updated values)
	for _, variable := range a.templateVars {
		newVB[variable.Label] = variable.Value
	}

	// As the BuildRuntimeConfig function always adds location and location_path to the user variables,
	// we need to remove them from the variables here to avoid that they end up in the config file
	delete(newVB, "location")
	delete(newVB, "location_path")

	instanceToModify.ProtocolConverterServiceConfig.Variables.User = newVB

	// Apply read DFC config if provided.
	if a.readDFCSvcCfg != nil {
		instanceToModify.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = *a.readDFCSvcCfg
	}

	// Apply write DFC config when the Output field was explicitly included in the payload
	// (non-nil, even if empty). A nil Output means a state-only change; the existing
	// config is preserved. An explicitly empty Output ({}) clears the write DFC output.
	if a.writeDFCInput != nil && a.writeDFCInput.Output != nil {
		instanceToModify.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = *a.writeDFCInput
	}

	// Add the connection details to the template
	instanceToModify.ProtocolConverterServiceConfig.Config.ConnectionServiceConfig = newIPPortConnectionTemplate()

	instanceToModify.ProtocolConverterServiceConfig.Location = convertIntMapToStringMap(a.location)

	// Update the connection details of the protocol converter (IP and PORT variables).
	// Only overwrite when non-empty: an edit that omits connection details should
	// preserve the existing values rather than blanking them out.
	if instanceToModify.ProtocolConverterServiceConfig.Variables.User != nil {
		if a.connectionIP != "" {
			instanceToModify.ProtocolConverterServiceConfig.Variables.User["IP"] = a.connectionIP
		}

		if a.connectionPort != "" && a.connectionPort != "0" {
			instanceToModify.ProtocolConverterServiceConfig.Variables.User["PORT"] = a.connectionPort
		}
	}

	// Only update the per-DFC desired states if the user provided new values.
	if a.readDFCState != "" {
		instanceToModify.ProtocolConverterServiceConfig.ReadDFCDesiredState = a.readDFCState
	}

	if a.writeDFCState != "" {
		instanceToModify.ProtocolConverterServiceConfig.WriteDFCDesiredState = a.writeDFCState
	}

	// The PC is always active so that the connection monitor stays alive.
	// Individual DFC states are tracked separately; the bridge (and its
	// connection monitor) is only torn down when the bridge itself is removed.
	instanceToModify.DesiredFSMState = protocolconverter.OperationalStateActive

	return instanceToModify, atomicEditUUID, instanceToModify.DesiredFSMState, nil
}

// persistConfig performs the atomic configuration update operation.
// Returns the old configuration for potential rollback operations.
func (a *EditProtocolConverterAction) persistConfig(atomicEditUUID uuid.UUID, newSpec config.ProtocolConverterConfig) (config.ProtocolConverterConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	oldConfig, err := a.configManager.AtomicEditProtocolConverter(ctx, atomicEditUUID, newSpec)
	if err != nil {
		return config.ProtocolConverterConfig{}, fmt.Errorf("failed to update protocol converter: %w", err)
	}

	// deep copy the old config therefore setup a full config
	// this may seem hacky but like that we can reuse the Clone() function
	// and we do not need to implement a custom Clone() function for the ProtocolConverterConfig
	fullConfig := config.FullConfig{
		ProtocolConverter: []config.ProtocolConverterConfig{oldConfig},
	}

	copiedConfig := fullConfig.Clone()
	oldConfig = copiedConfig.ProtocolConverter[0]
	// remove the location and location_path from the user variables
	// Check if User map exists before trying to delete from it
	if oldConfig.ProtocolConverterServiceConfig.Variables.User != nil {
		delete(oldConfig.ProtocolConverterServiceConfig.Variables.User, "location")
		delete(oldConfig.ProtocolConverterServiceConfig.Variables.User, "location_path")
	}

	return oldConfig, nil
}

// awaitRollout waits for the protocol converter to reach the desired state and performs health checks.
// Returns error code and error message for proper error handling in the caller.
//
// It polls live FSM state until the protocol converter reaches the desired state or the timeout hits.
// Unlike deploy operations, this method does not remove the component on timeout since it's an edit operation.
// The function returns the error code and the error message via an error object.
// The error code is a string that is sent to the frontend to allow it to determine if the action can be retried or not.
// The error message is sent to the frontend to allow the user to see the error message.
func (a *EditProtocolConverterAction) awaitRollout(pcConfig config.ProtocolConverterConfig, desiredPCState string) (string, error) {
	SendActionReply(
		a.instanceUUID,
		a.userEmail,
		a.actionUUID,
		models.ActionExecuting,
		fmt.Sprintf(
			"Waiting for protocol converter %s to be %s...",
			a.name,
			desiredPCState,
		),
		a.outboundChannel,
		models.EditProtocolConverter,
	)

	tickInterval := a.tickInterval
	if tickInterval == 0 {
		tickInterval = constants.ActionTickerTime
	}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout)
	startTime := time.Now()
	timeoutDuration := constants.DataflowComponentWaitForActiveTimeout

	var (
		logs     []s6.LogEntry
		lastLogs []s6.LogEntry

		// Fail-fast on persistent render failures (ENG-5103): a deterministic
		// render error repeats identically every tick; abort after a few
		// consecutive identical failures instead of burning the full timeout.
		prevRenderErrMsg     string
		identicalRenderFails int
	)

	const maxIdenticalRenderFails = 3

	for {
		elapsed := time.Since(startTime)
		remaining := timeoutDuration - elapsed
		remainingSeconds := int(remaining.Seconds())

		select {
		case <-timeout:
			// rollback to previous configuration
			rollbackErr := a.rollbackEdit(pcConfig)
			if rollbackErr != nil {
				a.actionLogger.Errorf("Failed to rollback to previous configuration: %v", rollbackErr)
				stateMessage := fmt.Sprintf("Protocol converter '%s' edit timeout reached. It did not become %s in time. Rolling back to previous configuration failed: %v", a.name, desiredPCState, rollbackErr)
				a.fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", rollbackErr, "edit_protocol_converter_rollback_failed",
					deps.String("pcConfig", pcConfig.String()))

				return models.ErrRetryRollbackTimeout, fmt.Errorf("%s", stateMessage)
			}

			stateMessage := fmt.Sprintf("Protocol converter '%s' edit timeout reached. It did not become %s in time. Rolled back to previous configuration", a.name, desiredPCState)
			if a.lastRenderErr != nil {
				stateMessage += fmt.Sprintf(" (root cause: %v)", a.lastRenderErr)
			}
			a.fsmLogger.SentryWarn(deps.FeatureDisableReadFlows, "", "edit_protocol_converter_rollback_on_timeout",
				deps.String("pcConfig", pcConfig.String()),
				deps.String("desiredPCState", desiredPCState),
			)

			return models.ErrRetryRollbackTimeout, fmt.Errorf("%s", stateMessage)

		case <-ticker.C:
			// Get a deep copy of the system snapshot to prevent race conditions
			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()

			protocolConverterManager, exists := systemSnapshot.Managers[constants.ProtocolConverterManagerName]
			if !exists {
				SendActionReply(
					a.instanceUUID,
					a.userEmail,
					a.actionUUID,
					models.ActionExecuting,
					RemainingPrefixSec(remainingSeconds)+"waiting for protocol converter manager to initialise",
					a.outboundChannel,
					models.EditProtocolConverter,
				)

				continue
			}

			instances := protocolConverterManager.GetInstances()
			found := false

			for _, instance := range instances {
				curName := instance.ID
				if curName != a.name {
					continue
				}

				// Cast the instance LastObservedState to a protocolconverter instance
				pcSnapshot, ok := instance.LastObservedState.(*protocolconverter.ProtocolConverterObservedStateSnapshot)
				if !ok {
					SendActionReply(
						a.instanceUUID,
						a.userEmail,
						a.actionUUID,
						models.ActionExecuting,
						RemainingPrefixSec(remainingSeconds)+"waiting for state info of protocol converter instance",
						a.outboundChannel,
						models.EditProtocolConverter,
					)

					continue
				}

				found = true
				currentStateReason := "current state: " + instance.CurrentState

				if a.dfcType == DFCTypeEmpty {
					// For empty DFC type (connection/location/state update only)
					// Only check the nmap port when activating; when stopping, nmap is also
					// stopped so it will never update to the new port.
					if desiredPCState != protocolconverter.OperationalStateStopped {
						nmapPort := strconv.FormatUint(
							uint64(pcSnapshot.ServiceInfo.ConnectionObservedState.ServiceInfo.NmapObservedState.ObservedNmapServiceConfig.Port),
							10,
						)

						if nmapPort != a.connectionPort {
							currentStateReason = "waiting for nmap to connect to port " + a.connectionPort
							SendActionReply(
								a.instanceUUID,
								a.userEmail,
								a.actionUUID,
								models.ActionExecuting,
								RemainingPrefixSec(remainingSeconds)+currentStateReason,
								a.outboundChannel,
								models.EditProtocolConverter,
							)

							continue
						}
					}

					// Check if the protocol converter has reached the desired state
					hasReachedDesiredState := false

					switch desiredPCState {
					case protocolconverter.OperationalStateActive:
						hasReachedDesiredState = slices.Contains(
							[]string{
								protocolconverter.OperationalStateActive,
								protocolconverter.OperationalStateIdle,
								protocolconverter.OperationalStateStartingFailedDFCMissing,
							},
							instance.CurrentState,
						)
					case protocolconverter.OperationalStateStopped:
						hasReachedDesiredState = instance.CurrentState == protocolconverter.OperationalStateStopped
					}

					if !hasReachedDesiredState {
						currentStateReason = fmt.Sprintf(
							"waiting for state to become %s (current: %s)",
							desiredPCState,
							instance.CurrentState,
						)
						SendActionReply(
							a.instanceUUID,
							a.userEmail,
							a.actionUUID,
							models.ActionExecuting,
							RemainingPrefixSec(remainingSeconds)+currentStateReason,
							a.outboundChannel,
							models.EditProtocolConverter,
						)

						continue
					}

					return "", nil
				}

				// When desired state is "stopped", the Benthos process is not running so
				// compareProtocolConverterDFCConfig always returns false (observed Input is nil).
				// Check the state first to avoid an infinite loop in this case.
				if desiredPCState == protocolconverter.OperationalStateStopped {
					if instance.CurrentState == protocolconverter.OperationalStateStopped {
						SendActionReply(
							a.instanceUUID,
							a.userEmail,
							a.actionUUID,
							models.ActionExecuting,
							RemainingPrefixSec(remainingSeconds)+"protocol converter successfully stopped",
							a.outboundChannel,
							models.EditProtocolConverter,
						)

						return "", nil
					}

					SendActionReply(
						a.instanceUUID,
						a.userEmail,
						a.actionUUID,
						models.ActionExecuting,
						RemainingPrefixSec(remainingSeconds)+fmt.Sprintf(
							"waiting for state to become stopped (current: %s)",
							instance.CurrentState,
						),
						a.outboundChannel,
						models.EditProtocolConverter,
					)

					continue
				}

				// Verify that the protocol converter has applied the desired DFC configuration.
				// We compare the desired DFC config with the observed DFC configuration
				// in the protocol converter snapshot.
				matched, renderErr := a.compareProtocolConverterDFCConfig(pcSnapshot)

				// Any tick whose comparison runs without a render failure —
				// it succeeded, or it failed for a non-render reason such as
				// Benthos still restarting — breaks the consecutive streak.
				// Ticks that skip the comparison entirely (manager, instance
				// or state info missing from the snapshot) leave the streak
				// untouched, so identical failures spanning such gaps still
				// count as consecutive.
				if renderErr == nil {
					prevRenderErrMsg = ""
					identicalRenderFails = 0
				}

				if !matched {
					if renderErr != nil {
						if renderErr.Error() == prevRenderErrMsg {
							identicalRenderFails++
						} else {
							prevRenderErrMsg = renderErr.Error()
							identicalRenderFails = 1
						}

						if identicalRenderFails >= maxIdenticalRenderFails {
							SendActionReply(
								a.instanceUUID,
								a.userEmail,
								a.actionUUID,
								models.ActionExecuting,
								Label("edit", a.name)+"persistent render failure detected. Rolling back...",
								a.outboundChannel,
								models.EditProtocolConverter,
							)

							rollbackErr := a.rollbackEdit(pcConfig)
							if rollbackErr != nil {
								a.actionLogger.Errorf("failed to roll back protocol converter %s: %v", a.name, rollbackErr)
								a.fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", rollbackErr, "edit_protocol_converter_render_failure_rollback_failed",
									deps.String("protocolConverter", a.name),
									deps.String("protocolConverterUUID", a.protocolConverterUUID.String()),
									deps.String("renderErr", renderErr.Error()))

								return models.ErrRetryRollbackTimeout, fmt.Errorf("protocol converter '%s' has a persistent render failure (%w) but could not be rolled back: %w. Please check your logs and consider manually restoring the previous configuration", a.name, renderErr, rollbackErr)
							}

							a.fsmLogger.SentryWarn(deps.FeatureDisableReadFlows, "", "edit_protocol_converter_render_failure_rolled_back",
								deps.String("protocolConverter", a.name),
								deps.String("protocolConverterUUID", a.protocolConverterUUID.String()),
								deps.String("renderErr", renderErr.Error()))

							return models.ErrConfigFileInvalid, fmt.Errorf("protocol converter '%s' was rolled back to its previous configuration after repeated render failures: %w. Please fix the configuration and try editing again", a.name, renderErr)
						}
					}

					SendActionReply(
						a.instanceUUID,
						a.userEmail,
						a.actionUUID,
						models.ActionExecuting,
						RemainingPrefixSec(remainingSeconds)+fmt.Sprintf(
							"%s DFC config not yet applied. State: %s, Status reason: %s",
							a.dfcType.String(),
							instance.CurrentState,
							pcSnapshot.ServiceInfo.StatusReason,
						),
						a.outboundChannel,
						models.EditProtocolConverter,
					)

					continue
				}

				// Check if the protocol converter has reached the desired state
				// For "active" state: accept "active" or "idle"
				// For "stopped" state: accept only "stopped"
				hasReachedDesiredState := false

				switch desiredPCState {
				case protocolconverter.OperationalStateActive:
					hasReachedDesiredState = slices.Contains(
						[]string{
							protocolconverter.OperationalStateActive,
							protocolconverter.OperationalStateIdle,
						},
						instance.CurrentState,
					)
				case protocolconverter.OperationalStateStopped:
					hasReachedDesiredState = instance.CurrentState == protocolconverter.OperationalStateStopped
				}

				if hasReachedDesiredState {
					terminal := map[string]string{
						protocolconverter.OperationalStateActive:  "activated",
						protocolconverter.OperationalStateStopped: "stopped",
					}[desiredPCState]
					SendActionReply(
						a.instanceUUID,
						a.userEmail,
						a.actionUUID,
						models.ActionExecuting,
						RemainingPrefixSec(remainingSeconds)+fmt.Sprintf(
							"protocol converter successfully %s with state '%s', %s DFC configuration verified",
							terminal,
							instance.CurrentState,
							a.dfcType.String(),
						),
						a.outboundChannel,
						models.EditProtocolConverter,
					)

					return "", nil
				}

				// Get the current state reason for more detailed information
				if pcSnapshot != nil && pcSnapshot.ServiceInfo.StatusReason != "" {
					currentStateReason = pcSnapshot.ServiceInfo.StatusReason
				}

				// send the benthos logs to the user; for DFCTypeBoth, prefer write logs since
				// read logs are also included below via the merged slice.

				switch a.dfcType {
				case DFCTypeWrite, DFCTypeBoth:
					logs = pcSnapshot.ServiceInfo.DataflowComponentWriteObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs
				default:
					logs = pcSnapshot.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.BenthosLogs
				}

				// only send the logs that have not been sent yet
				if len(logs) > len(lastLogs) {
					lastLogs = SendLimitedLogs(logs, lastLogs, a.instanceUUID, a.userEmail, a.actionUUID, a.outboundChannel, models.EditProtocolConverter, remainingSeconds)
				}

				// CheckBenthosLogLinesForConfigErrors is used to detect fatal configuration errors that would cause
				// Benthos to enter a CrashLoop. When such errors are detected, we can immediately
				// abort the startup process rather than waiting for the full timeout period,
				// as these errors require configuration changes to resolve.
				if CheckBenthosLogLinesForConfigErrors(logs) {
					SendActionReply(
						a.instanceUUID,
						a.userEmail,
						a.actionUUID,
						models.ActionExecuting,
						Label("edit", a.name)+"configuration error detected. Rolling back...",
						a.outboundChannel,
						models.EditProtocolConverter,
					)

					a.actionLogger.Infof("rolling back to previous configuration with user variables: %v", pcConfig.ProtocolConverterServiceConfig.Variables.User)

					err := a.rollbackEdit(pcConfig)
					if err != nil {
						a.actionLogger.Errorf("failed to roll back protocol converter %s: %v", a.name, err)
						a.fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", err, "edit_protocol_converter_config_error_rollback_failed",
							deps.String("pcConfig", pcConfig.String()))

						return models.ErrConfigFileInvalid, fmt.Errorf("protocol converter '%s' has invalid configuration but could not be rolled back: %w. Please check your logs and consider manually restoring the previous configuration", a.name, err)
					}

					a.fsmLogger.SentryWarn(deps.FeatureDisableReadFlows, "", "edit_protocol_converter_config_error_rolled_back",
						deps.String("pcConfig", pcConfig.String()))

					return models.ErrConfigFileInvalid, fmt.Errorf("protocol converter '%s' was rolled back to its previous configuration due to configuration errors. Please check the component logs, fix the configuration issues, and try editing again", a.name)
				}

				SendActionReply(
					a.instanceUUID,
					a.userEmail,
					a.actionUUID,
					models.ActionExecuting,
					RemainingPrefixSec(remainingSeconds)+currentStateReason,
					a.outboundChannel,
					models.EditProtocolConverter,
				)
			}

			if !found {
				SendActionReply(
					a.instanceUUID,
					a.userEmail,
					a.actionUUID,
					models.ActionExecuting,
					RemainingPrefixSec(remainingSeconds)+"waiting for protocol converter to appear in the system",
					a.outboundChannel,
					models.EditProtocolConverter,
				)
			}
		}
	}
}

// rollbackEdit restores the pre-edit configuration via the same
// AtomicEditProtocolConverter call that persisted the edit. Every awaitRollout
// abort path shares it so a change to the rollback semantics lands in one
// place; the error codes and Sentry events stay at the call sites because
// they intentionally differ per abort reason.
func (a *EditProtocolConverterAction) rollbackEdit(pcConfig config.ProtocolConverterConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	_, err := a.configManager.AtomicEditProtocolConverter(ctx, a.atomicEditUUID, pcConfig)

	return err
}

// compareProtocolConverterDFCConfig compares the desired DFC configuration with the observed
// DFC configuration in the protocol converter snapshot.
// It returns whether the configurations match and, when the mismatch was caused
// by a render failure, the render error.
func (a *EditProtocolConverterAction) compareProtocolConverterDFCConfig(pcSnapshot *protocolconverter.ProtocolConverterObservedStateSnapshot) (bool, error) {
	if pcSnapshot == nil {
		return false, nil
	}

	switch a.dfcType {
	case DFCTypeEmpty:
		return true, nil
	case DFCTypeRead:
		return a.compareSingleDFCConfig(pcSnapshot, DFCTypeRead)
	case DFCTypeWrite:
		return a.compareSingleDFCConfig(pcSnapshot, DFCTypeWrite)
	case DFCTypeBoth:
		readMatched, readRenderErr := a.compareSingleDFCConfig(pcSnapshot, DFCTypeRead)
		if !readMatched {
			return false, readRenderErr
		}

		return a.compareSingleDFCConfig(pcSnapshot, DFCTypeWrite)
	default:
		return false, nil
	}
}

// compareSingleDFCConfig compares a single DFC (read or write) against its observed state.
// It returns whether the configurations match and, when the mismatch was caused
// by a render failure, the render error.
func (a *EditProtocolConverterAction) compareSingleDFCConfig(pcSnapshot *protocolconverter.ProtocolConverterObservedStateSnapshot, dfcType DFCType) (bool, error) {
	var (
		desiredState      string
		observedFSMState  string
		observedDFCConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig
		presenceField     interface{} // the field that indicates the observed config is available
		excludeInput      bool        // true for write DFC (input is auto-generated)
	)

	switch dfcType {
	case DFCTypeRead:
		desiredState = a.readDFCState
		observedFSMState = pcSnapshot.ServiceInfo.DataflowComponentReadFSMState
		obs := pcSnapshot.ServiceInfo.DataflowComponentReadObservedState.ServiceInfo.BenthosObservedState.ObservedBenthosServiceConfig
		presenceField = obs.Input
		observedDFCConfig = observedBenthosToServiceConfig(obs)
	case DFCTypeWrite:
		desiredState = a.writeDFCState
		observedFSMState = pcSnapshot.ServiceInfo.DataflowComponentWriteFSMState
		obs := pcSnapshot.ServiceInfo.DataflowComponentWriteObservedState.ServiceInfo.BenthosObservedState.ObservedBenthosServiceConfig
		presenceField = obs.Output
		observedDFCConfig = observedBenthosToServiceConfig(obs)
		excludeInput = true
	default:
		return false, nil
	}

	// When DFC is being stopped, check FSM state instead of Benthos config.
	if desiredState == protocolconverter.OperationalStateStopped {
		return observedFSMState == protocolconverter.OperationalStateStopped, nil
	}

	if presenceField == nil {
		return false, nil
	}

	renderedDesiredConfig, err := a.renderDesiredDFCConfig(pcSnapshot, dfcType)
	if err != nil {
		a.actionLogger.Errorf("failed to render desired %s DFC config: %v", dfcType, err)
		a.lastRenderErr = err

		return false, err
	}

	// The render succeeded: clear the sticky root cause so a stale render
	// error cannot leak into a later timeout message.
	a.lastRenderErr = nil

	// Exclude the auto-generated field (output for read, input for write) from comparison.
	if excludeInput {
		observedDFCConfig.BenthosConfig.Input = nil
		renderedDesiredConfig.BenthosConfig.Input = nil
	} else {
		observedDFCConfig.BenthosConfig.Output = nil
		renderedDesiredConfig.BenthosConfig.Output = nil
	}

	a.actionLogger.Debugf("observed %s DFC config: %+v", dfcType, observedDFCConfig)
	a.actionLogger.Debugf("rendered desired %s DFC config: %+v", dfcType, renderedDesiredConfig)

	return dataflowcomponentserviceconfig.NewComparator().ConfigsEqual(observedDFCConfig, renderedDesiredConfig), nil
}

// renderDesiredDFCConfig renders the template variables in the desired DFC config
// using the actual runtime values from the protocol converter observed state.
// dfcTypeToReturn specifies which side (read or write) to return after rendering.
func (a *EditProtocolConverterAction) renderDesiredDFCConfig(pcSnapshot *protocolconverter.ProtocolConverterObservedStateSnapshot, dfcTypeToReturn DFCType) (dataflowcomponentserviceconfig.DataflowComponentServiceConfig, error) {
	if dfcTypeToReturn != DFCTypeRead && dfcTypeToReturn != DFCTypeWrite {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("invalid DFC type for rendering: %s", dfcTypeToReturn.String())
	}

	// Get the observed spec config
	specConfig := pcSnapshot.ObservedProtocolConverterSpecConfig

	// Create a deep copy to avoid mutating the original observed state
	var modifiedSpec protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec

	err := deepcopy.Copy(&modifiedSpec, &specConfig)
	if err != nil {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("failed to deep copy spec config: %w", err)
	}

	// Apply whichever desired DFC configs are set, so the runtime render sees the full picture.
	if a.readDFCSvcCfg != nil {
		modifiedSpec.Config.DataflowComponentReadServiceConfig = *a.readDFCSvcCfg
	}

	if a.writeDFCInput != nil && a.writeDFCInput.HasOutput() {
		modifiedSpec.Config.DataflowComponentWriteServiceConfig = *a.writeDFCInput
	}

	// Render with the edit's own template variables overlaid, the same merge
	// applyMutation persists. The observed spec lags the persisted edit by one
	// or more control-loop cycles (multiple seconds under CPU pressure), so
	// an edit that introduces a new variable referenced by its DFC would fail
	// this verification render with a missingkey error, identically every
	// tick, until the snapshot catches up, and the fail-fast abort would roll
	// back a valid edit. Variables the edit does not carry keep their
	// observed values.
	if len(a.templateVars) > 0 {
		mergedVars := make(map[string]any)

		if modifiedSpec.Variables.User != nil {
			maps.Copy(mergedVars, modifiedSpec.Variables.User)
		}

		for _, variable := range a.templateVars {
			mergedVars[variable.Label] = variable.Value
		}

		delete(mergedVars, "location")
		delete(mergedVars, "location_path")

		modifiedSpec.Variables.User = mergedVars
	}

	systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()

	agentLocation := convertIntMapToStringMap(systemSnapshot.CurrentConfig.Agent.Location)

	pcName := a.name

	runtimeConfig, err := runtime_config.BuildRuntimeConfig(
		modifiedSpec,
		agentLocation,
		nil, // TODO: add global vars
		runtime_config.BridgedByPlaceholder,
		pcName,
	)
	if err != nil {
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("failed to build runtime config: %w", err)
	}

	switch dfcTypeToReturn {
	case DFCTypeRead:
		return runtimeConfig.DataflowComponentReadServiceConfig, nil
	case DFCTypeWrite:
		return runtimeConfig.DataflowComponentWriteServiceConfig, nil
	default:
		return dataflowcomponentserviceconfig.DataflowComponentServiceConfig{}, fmt.Errorf("invalid DFC type: %s", dfcTypeToReturn.String())
	}
}

// deriveDFCType compares the incoming DFC payloads against the currently
// deployed config and returns a DFCType that only includes the sides that
// actually differ. If no config is deployed yet (first edit) or the config
// cannot be read, it falls back to payload presence.
func (a *EditProtocolConverterAction) deriveDFCType() DFCType {
	hasRead := a.readDFCSvcCfg != nil
	hasWrite := a.writeDFCInput != nil

	if !hasRead && !hasWrite {
		return DFCTypeEmpty
	}

	// Try to fetch the current config for diff comparison.
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	currentConfig, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		a.actionLogger.Debugf("Cannot read current config for diff, falling back to payload presence: %v", err)

		return dfcTypeFromPresence(hasRead, hasWrite)
	}

	// Find the protocol converter in the current config.
	var currentPC *config.ProtocolConverterConfig

	for i, pc := range currentConfig.ProtocolConverter {
		if dataflowcomponentserviceconfig.GenerateUUIDFromName(pc.Name) == a.protocolConverterUUID {
			currentPC = &currentConfig.ProtocolConverter[i]

			break
		}
	}

	if currentPC == nil {
		a.actionLogger.Debugf("Protocol converter %s not found in current config, falling back to payload presence", a.protocolConverterUUID)

		return dfcTypeFromPresence(hasRead, hasWrite)
	}

	// Any variable change (IP, PORT, or custom vars like baudRate) forces redeploy of all
	// present DFCs because template rendering uses all variables. Keys absent in the deployed
	// config are skipped — treating nil as changed would cause spurious redeploys on PCs
	// created before a variable was standard (fmt.Sprint(nil) → "<nil>" ≠ any real value).
	deployedVars := currentPC.ProtocolConverterServiceConfig.Variables.User

	incomingVars := make(map[string]any, len(a.templateVars)+2)
	for _, v := range a.templateVars {
		incomingVars[v.Label] = v.Value
	}
	if a.connectionIP != "" {
		incomingVars["IP"] = a.connectionIP
	}
	if a.connectionPort != "" && a.connectionPort != "0" {
		incomingVars["PORT"] = a.connectionPort
	}

	connectionChanged := false
	for k, incomingVal := range incomingVars {
		if deployedVal, ok := deployedVars[k]; ok {
			if fmt.Sprint(deployedVal) != fmt.Sprint(incomingVal) {
				a.actionLogger.Debugf("Variable %q changed (%v → %v), all present DFCs need redeploy", k, deployedVal, incomingVal)
				connectionChanged = true
				break
			}
		}
	}
	if connectionChanged {
		return dfcTypeFromPresence(hasRead, hasWrite)
	}

	// Check if read or write payload is different from the deployed config.
	readChanged := hasRead && a.readDFCSvcCfgDiffers(*a.readDFCSvcCfg, a.readDFCState,
		currentPC.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig,
		currentPC.ProtocolConverterServiceConfig.ReadDFCDesiredState)
	writeChanged := hasWrite && a.writeDFCConfigDiffers(*a.writeDFCInput, a.writeDFCState,
		currentPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig,
		currentPC.ProtocolConverterServiceConfig.WriteDFCDesiredState)
	derived := dfcTypeFromPresence(readChanged, writeChanged)
	a.actionLogger.Debugf("Derived dfcType=%s (readChanged=%v, writeChanged=%v)", derived, readChanged, writeChanged)

	return derived
}

// readDFCSvcCfgDiffers checks whether an incoming read DFC service config differs from the
// persisted config in config.yaml. This is intentionally simpler than
// compareSingleDFCConfig, which compares rendered templates against runtime
// observed state from the FSM snapshot. Here we only need to know whether the
// user sent something new relative to what is already on disk.
func (a *EditProtocolConverterAction) readDFCSvcCfgDiffers(
	incoming dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	incomingState string,
	deployedConfig dataflowcomponentserviceconfig.DataflowComponentServiceConfig,
	deployedState string,
) bool {
	// State change counts as a diff, but only when both sides are populated.
	// On freshly upgraded systems the deployed config may not have the
	// ReadDFCDesiredState/WriteDFCDesiredState fields yet (they unmarshal to "").
	// Treating "" != "active" as a diff would force an unnecessary redeploy
	// on every first edit after upgrade.
	if incomingState != "" && deployedState != "" && incomingState != deployedState {
		return true
	}
	return !deployedConfig.Equal(incoming)
}

// writeDFCConfigDiffers reports whether the incoming typed write config differs from
// what is persisted in config.yaml.
func (a *EditProtocolConverterAction) writeDFCConfigDiffers(
	incoming dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput,
	incomingState string,
	deployedConfig dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput,
	deployedState string,
) bool {
	if incomingState != "" && deployedState != "" && incomingState != deployedState {
		return true
	}
	// Normalize nil maps to empty maps before comparison: YAML round-trip converts
	// nil maps (Output/Buffer) to empty maps, and reflect.DeepEqual treats them as
	// different, causing spurious drift detection on every reconcile.
	return !reflect.DeepEqual(normalizeWriteConfigInput(incoming), normalizeWriteConfigInput(deployedConfig))
}

// normalizeWriteConfigInput replaces nil map fields with empty maps so that
// reflect.DeepEqual comparisons are not tripped up by YAML round-trip nil→{} coercions.
func normalizeWriteConfigInput(c dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput) dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput {
	if c.Output == nil {
		c.Output = map[string]any{}
	}
	if c.Buffer == nil {
		c.Buffer = map[string]any{}
	}
	// Mirror the default applied by ToDataflowComponentServiceConfig so that a config
	// stored without an explicit JS snippet compares equal to one that was just parsed
	// from a payload carrying the default "return msg;".
	if c.ProcessingNoderedJS == "" {
		c.ProcessingNoderedJS = "return msg;"
	}
	return c
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *EditProtocolConverterAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *EditProtocolConverterAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the built read DFC service config - exposed primarily for testing purposes.
func (a *EditProtocolConverterAction) GetParsedPayload() *dataflowcomponentserviceconfig.DataflowComponentServiceConfig {
	return a.readDFCSvcCfg
}

// GetIgnoreHealthCheck returns whether health-check errors are suppressed - exposed primarily for testing.
func (a *EditProtocolConverterAction) GetIgnoreHealthCheck() bool {
	return a.ignoreHealthCheck
}

// GetProtocolConverterUUID returns the protocol converter UUID - exposed for testing purposes.
func (a *EditProtocolConverterAction) GetProtocolConverterUUID() uuid.UUID {
	return a.protocolConverterUUID
}

// GetDFCType returns the DFC type (read/write) - exposed for testing purposes.
func (a *EditProtocolConverterAction) GetDFCType() string {
	return a.dfcType.String()
}

// GetDesiredWriteDFCConfig returns the raw write DFC input config - exposed for testing purposes.
func (a *EditProtocolConverterAction) GetDesiredWriteDFCConfig() *dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput {
	return a.writeDFCInput
}
