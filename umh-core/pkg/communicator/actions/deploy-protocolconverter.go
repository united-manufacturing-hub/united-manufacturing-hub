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

// Package actions contains implementations of the Action interface that create
// and manage protocol converter configurations in the UMH system.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// A Protocol Converter (PC) in UMH connects external data sources/sinks to the
// unified namespace using templated configurations. "Deploying" a protocol
// converter means:
//
//   1. Creating a new configuration entry with a YAML anchor template
//   2. Setting IP and PORT as template variables
//   3. Generating a UUID based on the component name
//   4. Adding the configuration to the central store
//
// The action creates a minimal protocol converter that can later be enhanced
// with actual dataflow component configurations through edit actions.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// DeployProtocolConverterAction implements the Action interface for deploying a
// new Protocol Converter. All fields are immutable after construction to
// avoid race conditions.
type DeployProtocolConverterAction struct {
	configManager config.ConfigManager

	outboundChannel       chan *models.UMHMessage
	systemSnapshotManager *fsm.SnapshotManager // Snapshot Manager holds the latest system snapshot
	actionLogger          *zap.SugaredLogger
	fsmLogger             deps.FSMLogger

	userEmail string
	// Parsed request payload (only populated after Parse)
	payload models.ProtocolConverter

	actionUUID        uuid.UUID
	instanceUUID      uuid.UUID
	ignoreHealthCheck bool
}

// NewDeployProtocolConverterAction returns an un-parsed action instance.
func NewDeployProtocolConverterAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager, systemSnapshotManager *fsm.SnapshotManager) *DeployProtocolConverterAction {
	al := logger.For(logger.ComponentCommunicator)

	return &DeployProtocolConverterAction{
		userEmail:             userEmail,
		actionUUID:            actionUUID,
		instanceUUID:          instanceUUID,
		outboundChannel:       outboundChannel,
		configManager:         configManager,
		actionLogger:          al,
		fsmLogger:             deps.NewFSMLogger(al),
		systemSnapshotManager: systemSnapshotManager,
	}
}

// Parse implements the Action interface by extracting protocol converter configuration from the payload.
func (a *DeployProtocolConverterAction) Parse(payload interface{}) error {
	a.ignoreHealthCheck = false

	// Parse the payload to get the protocol converter configuration
	parsedPayload, err := ParseActionPayload[models.ProtocolConverter](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	a.payload = parsedPayload

	if parsedPayload.ReadDFC != nil && parsedPayload.ReadDFC.IgnoreErrors != nil {
		a.ignoreHealthCheck = *parsedPayload.ReadDFC.IgnoreErrors
	}
	// OR-merge: if either DFC requests skipping errors, skip the health-check wait for
	// the entire PC. This means a write DFC with IgnoreErrors=true also suppresses the
	// read DFC health check. This is intentional: the frontend sets IgnoreErrors when it
	// cannot guarantee a DFC will reach a healthy state (e.g. experimental outputs).
	if parsedPayload.WriteDFCPayload != nil && parsedPayload.WriteDFCPayload.IgnoreErrors != nil {
		a.ignoreHealthCheck = a.ignoreHealthCheck || *parsedPayload.WriteDFCPayload.IgnoreErrors
	}

	a.actionLogger.Debugf("Parsed DeployProtocolConverter action payload: name=%s, ip=%s, port=%d",
		a.payload.Name, a.payload.Connection.IP, a.payload.Connection.Port)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *DeployProtocolConverterAction) Validate() error {
	// Validate all required fields
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	// A bridge that inherits the historian connection derives its health check from the
	// shared historian configuration, so it does not require a user-supplied host/port.
	isHistorian := a.payload.WriteDFCPayload != nil &&
		inheritsHistorianConnection(a.payload.WriteDFCPayload.Destination)

	if !isHistorian {
		if a.payload.Connection.IP == "" {
			return errors.New("missing required field Connection.IP")
		}

		if a.payload.Connection.Port == 0 {
			return errors.New("missing required field Connection.Port")
		}
	}

	if err := config.ValidateComponentName(a.payload.Name); err != nil {
		return err
	}

	// Always validate the read DFC, even if it's nil (ignoreErrors will handle it)
	if err := validateReadProtocolConverterDFC(a.payload.ReadDFC); err != nil {
		return err
	}

	// Validate the write DFC, if present. Might not exist for read-only bridges
	if w := a.payload.WriteDFCPayload; w != nil {
		if err := validateWriteDFCConfig(&w.DataflowComponentWriteConfigInput, w.State); err != nil {
			return err
		}
	}

	return nil
}

// Execute implements the Action interface by creating the protocol converter configuration.
func (a *DeployProtocolConverterAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing DeployProtocolConverter action")

	// Send confirmation that action is starting
	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting deployment of Bridge: "+a.payload.Name, a.outboundChannel, models.DeployProtocolConverter)

	// Create the protocol converter config with template and variables
	pcConfig, err := a.createProtocolConverterConfig()
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to create protocol converter configuration: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.DeployProtocolConverter)
		a.fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", err, "deploy_protocol_converter_create_config_failed",
			deps.String("name", a.payload.Name))

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// currently, we canot reuse templates, so we need to create a new one
	pcConfig.ProtocolConverterServiceConfig.TemplateRef = pcConfig.Name

	// Add to configuration
	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Adding Bridge to configuration...", a.outboundChannel, models.DeployProtocolConverter)

	err = a.configManager.AtomicAddProtocolConverter(ctx, pcConfig)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to add Bridge: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.DeployProtocolConverter)
		a.fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", err, "deploy_protocol_converter_add_failed",
			deps.String("pcConfig", pcConfig.String()))

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	// Generate the UUID for the response
	pcUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(a.payload.Name)

	// Create response with the filled UUID
	response := models.ProtocolConverter{
		UUID:       &pcUUID,
		Name:       a.payload.Name,
		Location:   a.payload.Location,
		Connection: a.payload.Connection,
		// ReadDFC, WriteDFC, and TemplateInfo are nil as they will be added later
	}

	SendActionReplyV2(
		a.instanceUUID,
		a.userEmail,
		a.actionUUID,
		models.ActionExecuting,
		fmt.Sprintf(
			"Waiting for Bridge to be %s...",
			pcConfig.DesiredFSMState,
		),
		"",
		map[string]interface{}{"uuid": pcUUID.String()},
		a.outboundChannel,
		models.DeployProtocolConverter,
		nil,
	)

	// check against observedState
	if a.systemSnapshotManager != nil && !a.ignoreHealthCheck {
		errCode, err := a.waitForComponentToAppear(pcConfig.DesiredFSMState)
		if err != nil {
			// err is already a self-contained user message, so send it as-is
			// instead of wrapping it again.
			// Config is kept (no rollback), so return the UUID. The frontend uses
			// it to offer fixing the persisted bridge from the editing view.
			SendActionReplyV2(
				a.instanceUUID,
				a.userEmail,
				a.actionUUID,
				models.ActionFinishedWithFailure,
				err.Error(),
				errCode,
				map[string]interface{}{"uuid": pcUUID.String()},
				a.outboundChannel,
				models.DeployProtocolConverter,
				nil,
			)
			a.fsmLogger.SentryError(deps.FeatureDisableReadFlows, "", err, "deploy_protocol_converter_wait_failed",
				deps.String("pcConfig", pcConfig.String()),
				deps.String("desiredState", pcConfig.DesiredFSMState))

			return nil, nil, err
		}
	}

	var successMsg string
	if a.ignoreHealthCheck {
		successMsg = fmt.Sprintf(
			`Bridge deployed (health check skipped; state '%s' not verified)`,
			pcConfig.DesiredFSMState,
		)
	} else {
		successMsg = fmt.Sprintf(
			`Bridge was successfully deployed and reached the expected state '%s'`,
			pcConfig.DesiredFSMState,
		)
	}

	SendActionReply(
		a.instanceUUID,
		a.userEmail,
		a.actionUUID,
		models.ActionExecuting,
		successMsg,
		a.outboundChannel,
		models.DeployProtocolConverter,
	)

	return response, nil, nil
}

// createProtocolConverterConfig creates a ProtocolConverterConfig with templated configuration.
func (a *DeployProtocolConverterAction) createProtocolConverterConfig() (config.ProtocolConverterConfig, error) {
	return buildProtocolConverterConfig(a.payload)
}

// buildProtocolConverterConfig builds a ProtocolConverterConfig with templated
// configuration from a parsed protocol converter payload.
func buildProtocolConverterConfig(payload models.ProtocolConverter) (config.ProtocolConverterConfig, error) {
	userVars := buildUserScope(payload.TemplateInfo)
	userVars["IP"] = payload.Connection.IP
	userVars["PORT"] = strconv.FormatUint(uint64(payload.Connection.Port), 10)

	// Historian bridges get an Nmap target resolved from the shared historian section; all
	// others get the standard {{ .IP }}/{{ .PORT }} template fed by the connection variables.
	var writeDestination dataflowcomponentserviceconfig.WriteConfigDestination
	if payload.WriteDFCPayload != nil {
		writeDestination = payload.WriteDFCPayload.Destination
	}

	tmpl := protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
		ConnectionServiceConfig:             connectionTemplateForWriteOutput(writeDestination),
		DataflowComponentReadServiceConfig:  dataflowcomponentserviceconfig.DataflowComponentServiceConfig{},
		DataflowComponentWriteServiceConfig: dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{},
	}

	if payload.ReadDFC != nil {
		readSvcCfg, err := buildReadDFCServiceConfig(dfcToPayload(payload.ReadDFC), payload.Name)
		if err != nil {
			return config.ProtocolConverterConfig{}, err
		}

		tmpl.DataflowComponentReadServiceConfig = readSvcCfg
	}

	if w := payload.WriteDFCPayload; w != nil {
		tmpl.DataflowComponentWriteServiceConfig = w.DataflowComponentWriteConfigInput
	}

	var readDFCDesiredState, writeDFCDesiredState string
	if payload.ReadDFC != nil {
		readDFCDesiredState = payload.ReadDFC.State
	}

	if payload.WriteDFCPayload != nil {
		writeDFCDesiredState = payload.WriteDFCPayload.State
	}

	spec := protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
		Config:               tmpl,
		Variables:            variables.VariableBundle{User: userVars},
		Location:             convertIntMapToStringMap(payload.Location),
		ReadDFCDesiredState:  readDFCDesiredState,
		WriteDFCDesiredState: writeDFCDesiredState,
	}

	return config.ProtocolConverterConfig{
		FSMInstanceConfig: config.FSMInstanceConfig{
			Name:            payload.Name,
			DesiredFSMState: protocolconverter.OperationalStateActive,
		},
		ProtocolConverterServiceConfig: spec,
	}, nil
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *DeployProtocolConverterAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *DeployProtocolConverterAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed payload - exposed primarily for testing purposes.
func (a *DeployProtocolConverterAction) GetParsedPayload() models.ProtocolConverter {
	return a.payload
}

// waitForComponentToAppear polls live FSM state until the new component
// reaches the desired state or the timeout hits (→ delete unless ignoreHealthCheck).
// the function returns the error code and the error message via an error object
// the error code is a string that is sent to the frontend to allow it to determine if the action can be retried or not
// the error message is sent to the frontend to allow the user to see the error message.
func (a *DeployProtocolConverterAction) waitForComponentToAppear(desiredState string) (string, error) {
	ticker := time.NewTicker(constants.ActionTickerTime)
	defer ticker.Stop()

	timeout := time.After(constants.DataflowComponentWaitForActiveTimeout)
	startTime := time.Now()
	timeoutDuration := constants.DataflowComponentWaitForActiveTimeout

	// Track last known blocking reason for timeout error message
	var lastStatusReason string

	for {
		elapsed := time.Since(startTime)
		remaining := timeoutDuration - elapsed
		remainingSeconds := int(remaining.Seconds())

		select {
		case <-timeout:
			stateMessage := Label("deploy", a.payload.Name) + "timeout reached"
			SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting, stateMessage,
				a.outboundChannel, models.DeployProtocolConverter)

			// Config is kept on failure so the user does not lose it. The bridge
			// can be fixed from the editing view.
			errorMsg := fmt.Sprintf("Bridge '%s' did not reach state '%s' within the timeout period", a.payload.Name, desiredState)
			if lastStatusReason != "" {
				errorMsg = fmt.Sprintf("Bridge '%s' did not become healthy: %s", a.payload.Name, lastStatusReason)
			} else {
				errorMsg += ". Please check system load or component configuration and try again"
			}

			a.fsmLogger.SentryWarn(deps.FeatureDisableReadFlows, "", "deploy_protocol_converter_timeout",
				deps.String("name", a.payload.Name),
				deps.String("desiredState", desiredState),
				deps.String("lastStatusReason", lastStatusReason))

			return models.ErrDeployTimeout, fmt.Errorf("%s", errorMsg)

		case <-ticker.C:
			// the snapshot manager holds the latest system snapshot which is asynchronously updated by the other goroutines
			// we need to get a deep copy of it to prevent race conditions
			systemSnapshot := a.systemSnapshotManager.GetDeepCopySnapshot()
			if protocolConverterManager, exists := systemSnapshot.Managers[constants.ProtocolConverterManagerName]; exists {
				instances := protocolConverterManager.GetInstances()
				found := false

				for _, instance := range instances {
					curName := instance.ID
					if curName != a.payload.Name {
						continue
					}

					found = true

					// Check if the protocol converter has reached the desired state
					// For "active" state: accept "active", "idle", or "starting_failed_dfc_missing" (empty bridges)
					// For "stopped" state: accept only "stopped"
					var acceptedStates []string

					switch desiredState {
					case dataflowcomponent.OperationalStateActive:
						// Note: starting_failed_dfc_missing is a valid state for empty bridges (no DFCs configured yet)
						// This allows the deploy → edit workflow where deploy creates an empty bridge and edit adds DFCs later
						acceptedStates = []string{
							protocolconverter.OperationalStateActive,
							protocolconverter.OperationalStateIdle,
							protocolconverter.OperationalStateStartingFailedDFCMissing,
						}
					case dataflowcomponent.OperationalStateStopped:
						acceptedStates = []string{
							protocolconverter.OperationalStateStopped,
						}
					}

					for _, acceptedState := range acceptedStates {
						if instance.CurrentState == acceptedState {
							return "", nil
						}
					}

					// Get more detailed status information from the protocol converter snapshot
					currentStateReason := "current state: " + instance.CurrentState

					// Cast the instance LastObservedState to a protocolconverter instance
					pcSnapshot, ok := instance.LastObservedState.(*protocolconverter.ProtocolConverterObservedStateSnapshot)
					if ok && pcSnapshot != nil && pcSnapshot.ServiceInfo.StatusReason != "" {
						// Use the raw status reason from FSM - frontend will handle enhancement
						lastStatusReason = pcSnapshot.ServiceInfo.StatusReason
						currentStateReason = pcSnapshot.ServiceInfo.StatusReason
					}

					stateMessage := RemainingPrefixSec(remainingSeconds) + currentStateReason
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.DeployProtocolConverter)
				}

				if !found {
					stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for it to appear in the config"
					SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
						stateMessage, a.outboundChannel, models.DeployProtocolConverter)
				}
			} else {
				stateMessage := RemainingPrefixSec(remainingSeconds) + "waiting for manager to initialise"
				SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
					stateMessage, a.outboundChannel, models.DeployProtocolConverter)
			}
		}
	}
}
