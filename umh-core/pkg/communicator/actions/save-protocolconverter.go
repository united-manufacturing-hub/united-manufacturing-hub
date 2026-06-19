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
// SaveProtocolConverter exists solely for the first-time bridge deployment
// experience. It covers the specific edge case where a user clicks
// "Save & Deploy" for the first time and that initial deployment then fails:
// the bridge configuration must not be lost. To guarantee that, the action
// persists the new configuration without waiting for the bridge to reach its
// desired state, and without rolling the config back on failure. Everything
// after this initial save (later edits, restarts, etc.) goes through the normal
// paths and is unaffected.
//
// Contrast with DeployProtocolConverter, which adds the config AND blocks until
// the FSM reaches the desired state, deleting the config again on timeout. The
// deployment status is observed separately through the normal FSM status feed.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// SaveProtocolConverterAction implements the Action interface for saving a
// Protocol Converter configuration without deploying it. All fields are
// immutable after construction to avoid race conditions.
type SaveProtocolConverterAction struct {
	configManager config.ConfigManager

	outboundChannel chan *models.UMHMessage
	actionLogger    *zap.SugaredLogger
	fsmLogger       deps.FSMLogger

	userEmail string
	// Parsed request payload (only populated after Parse)
	payload models.ProtocolConverter

	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewSaveProtocolConverterAction returns an un-parsed action instance. It is the
// test-facing constructor; production builds the struct directly in
// newActionFromPayload so it can pass the caller's prepared loggers.
func NewSaveProtocolConverterAction(userEmail string, actionUUID uuid.UUID, instanceUUID uuid.UUID, outboundChannel chan *models.UMHMessage, configManager config.ConfigManager) *SaveProtocolConverterAction {
	al := logger.For(logger.ComponentCommunicator)
	return &SaveProtocolConverterAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    al,
		fsmLogger:       deps.NewFSMLogger(al),
	}
}

// Parse implements the Action interface by extracting protocol converter configuration from the payload.
func (a *SaveProtocolConverterAction) Parse(payload interface{}) error {
	parsedPayload, err := ParseActionPayload[models.ProtocolConverter](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	a.payload = parsedPayload

	a.actionLogger.Debugf("Parsed SaveProtocolConverter action payload: name=%s, ip=%s, port=%d",
		a.payload.Name, a.payload.Connection.IP, a.payload.Connection.Port)

	return nil
}

// Validate performs validation of the parsed payload.
func (a *SaveProtocolConverterAction) Validate() error {
	if a.payload.Name == "" {
		return errors.New("missing required field Name")
	}

	if a.payload.Connection.IP == "" {
		return errors.New("missing required field Connection.IP")
	}

	if a.payload.Connection.Port == 0 {
		return errors.New("missing required field Connection.Port")
	}

	if err := config.ValidateComponentName(a.payload.Name); err != nil {
		return err
	}

	if err := validateReadProtocolConverterDFC(a.payload.ReadDFC); err != nil {
		return err
	}
	if w := a.payload.WriteDFCPayload; w != nil {
		if err := validateWriteDFCConfig(&w.DataflowComponentWriteConfigInput, w.State); err != nil {
			return err
		}
	}

	return nil
}

// Execute implements the Action interface by persisting the protocol converter
// configuration. Unlike DeployProtocolConverter it does not wait for the bridge
// to reach its desired state and never rolls the config back, so the
// configuration survives a failed deployment.
func (a *SaveProtocolConverterAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing SaveProtocolConverter action")

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting save of protocol converter: "+a.payload.Name, a.outboundChannel, models.SaveProtocolConverter)

	// Build the protocol converter config (template + variables).
	pcConfig, err := buildProtocolConverterConfig(a.payload)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to create protocol converter configuration: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.SaveProtocolConverter)
		a.fsmLogger.SentryError(deps.FeatureDeploymentSaveConfig, "", err, "save_protocol_converter_create_config_failed",
			deps.String("name", a.payload.Name))

		return nil, nil, fmt.Errorf("failed to create protocol converter configuration: %w", err)
	}

	// currently, we cannot reuse templates, so we need to create a new one
	pcConfig.ProtocolConverterServiceConfig.TemplateRef = pcConfig.Name

	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Saving protocol converter configuration...", a.outboundChannel, models.SaveProtocolConverter)

	// First-time deployment only: create the new bridge. If one with this name
	// already exists, AtomicAddProtocolConverter fails with a duplicate-name
	// error - by design, since later updates go through the dedicated edit
	// action, not this save path.
	if err := a.configManager.AtomicAddProtocolConverter(ctx, pcConfig); err != nil {
		errorMsg := fmt.Sprintf("Failed to save protocol converter: %v", err)
		SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, a.outboundChannel, models.SaveProtocolConverter)
		a.fsmLogger.SentryError(deps.FeatureDeploymentSaveConfig, "", err, "save_protocol_converter_save_failed",
			deps.String("pcConfig", pcConfig.String()))

		return nil, nil, fmt.Errorf("failed to save protocol converter: %w", err)
	}

	pcUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(a.payload.Name)

	response := models.ProtocolConverter{
		UUID:       &pcUUID,
		Name:       a.payload.Name,
		Location:   a.payload.Location,
		Connection: a.payload.Connection,
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Protocol converter configuration was saved successfully", a.outboundChannel, models.SaveProtocolConverter)

	return response, nil, nil
}

// getUserEmail implements the Action interface by returning the user email associated with this action.
func (a *SaveProtocolConverterAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface by returning the UUID of this action.
func (a *SaveProtocolConverterAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// GetParsedPayload returns the parsed payload - exposed primarily for testing purposes.
func (a *SaveProtocolConverterAction) GetParsedPayload() models.ProtocolConverter {
	return a.payload
}
