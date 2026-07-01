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
// and manage historian configurations in the UMH system.
//
// -----------------------------------------------------------------------------
// BUSINESS CONTEXT
// -----------------------------------------------------------------------------
// "Editing" a historian replaces the existing `historian:` section in
// config.yaml with new connection settings. Unlike deploy, edit expects the
// section to already exist — callers should use deploy-historian when creating
// it for the first time.
// -----------------------------------------------------------------------------

package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// EditHistorianAction implements the Action interface for updating the historian
// configuration in config.yaml. All fields are immutable after construction.
type EditHistorianAction struct {
	configManager   config.ConfigManager
	outboundChannel chan *models.UMHMessage
	actionLogger    *zap.SugaredLogger

	userEmail    string
	payload      config.HistorianConfig
	actionUUID   uuid.UUID
	instanceUUID uuid.UUID
}

// NewEditHistorianAction returns an un-parsed action instance.
func NewEditHistorianAction(
	userEmail string,
	actionUUID uuid.UUID,
	instanceUUID uuid.UUID,
	outboundChannel chan *models.UMHMessage,
	configManager config.ConfigManager,
) *EditHistorianAction {
	return &EditHistorianAction{
		userEmail:       userEmail,
		actionUUID:      actionUUID,
		instanceUUID:    instanceUUID,
		outboundChannel: outboundChannel,
		configManager:   configManager,
		actionLogger:    logger.For(logger.ComponentCommunicator),
	}
}

// Parse implements the Action interface.
func (a *EditHistorianAction) Parse(payload interface{}) error {
	parsed, err := ParseActionPayload[config.HistorianConfig](payload)
	if err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	a.payload = parsed

	a.actionLogger.Debugf("Parsed EditHistorian action payload: host=%s port=%d database=%s",
		a.payload.Host, a.payload.Port, a.payload.Database)

	return nil
}

// Validate implements the Action interface.
func (a *EditHistorianAction) Validate() error {
	return a.payload.Validate()
}

// getUserEmail implements the Action interface.
func (a *EditHistorianAction) getUserEmail() string {
	return a.userEmail
}

// getUuid implements the Action interface.
func (a *EditHistorianAction) getUuid() uuid.UUID {
	return a.actionUUID
}

// Execute implements the Action interface by overwriting the historian config.
func (a *EditHistorianAction) Execute() (interface{}, map[string]interface{}, error) {
	a.actionLogger.Info("Executing EditHistorian action")

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionConfirmed,
		"Starting edit of Historian", a.outboundChannel, models.EditHistorian)

	ctx, cancel := context.WithTimeout(context.Background(), constants.ActionTimeout)
	defer cancel()

	// Verify historian section already exists before overwriting.
	currentCfg, err := a.configManager.GetConfig(ctx, 0)
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to read current configuration: %v", err)
		SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, models.ErrConfigFileInvalid, nil, a.outboundChannel, models.EditHistorian, nil)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	if currentCfg.Historian == nil {
		errorMsg := "Historian is not configured; use deploy-historian to create it first"
		SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, models.ErrValidationFailed, nil, a.outboundChannel, models.EditHistorian, nil)

		return nil, nil, errors.New(errorMsg)
	}

	cfg := a.payload.WithDefaults()

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionExecuting,
		"Updating Historian configuration...", a.outboundChannel, models.EditHistorian)

	if err := a.configManager.AtomicSetHistorian(ctx, cfg); err != nil {
		errorMsg := fmt.Sprintf("Failed to update Historian configuration: %v", err)
		SendActionReplyV2(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedWithFailure,
			errorMsg, models.ErrRetryConfigWriteFailed, nil, a.outboundChannel, models.EditHistorian, nil)

		return nil, nil, fmt.Errorf("%s", errorMsg)
	}

	SendActionReply(a.instanceUUID, a.userEmail, a.actionUUID, models.ActionFinishedSuccessfull,
		"Historian updated successfully", a.outboundChannel, models.EditHistorian)

	return cfg, nil, nil
}

